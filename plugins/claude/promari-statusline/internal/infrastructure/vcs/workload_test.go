package vcs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform/platformtest"
	"promari-statusline/internal/infrastructure/vcs"
)

const (
	cmdIssues    = "gh issue list --assignee @me --state open --limit 200 --json number,createdAt,updatedAt,milestone,labels"
	cmdOpen      = "gh pr list --author @me --limit 100 --state open --json number,updatedAt,createdAt,isDraft,reviewDecision"
	cmdMerged    = "gh pr list --author @me --limit 100 --state merged --search merged:>=2026-09-19 --json number,createdAt,mergedAt,reviews"
	cmdAbandoned = "gh pr list --author @me --limit 100 --state closed --search is:unmerged closed:>=2026-09-19 --json number"
	cmdHead      = "git --no-optional-locks -C /work symbolic-ref --short refs/remotes/origin/HEAD"
	cmdRuns      = "gh run list --branch develop --limit 30 --json workflowName,conclusion,createdAt"
)

func TestWorkload(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	sys.Cmds[cmdIssues] = platformtest.Result{Out: `[
		{"number":7,"createdAt":"2026-09-01T00:00:00Z","updatedAt":"2026-09-01T00:00:00Z","milestone":{"dueOn":"2026-10-01T00:00:00Z"},"labels":[{"name":"P1"}]},
		{"number":8,"createdAt":"2026-10-03T00:00:00Z","updatedAt":"2026-10-03T00:00:00Z","milestone":null,"labels":[]}]`}
	sys.Cmds[cmdOpen] = platformtest.Result{Out: `[{"number":3,"updatedAt":"2026-10-03T00:00:00Z","isDraft":true,"reviewDecision":"CHANGES_REQUESTED"}]`}
	sys.Cmds[cmdMerged] = platformtest.Result{Out: `[
		{"number":1,"createdAt":"2026-10-02T00:00:00Z","mergedAt":"2026-10-02T02:00:00Z","reviews":[{"state":"COMMENTED"},{"state":"APPROVED"}]},
		{"number":2,"createdAt":"2026-10-02T00:00:00Z","mergedAt":"2026-10-02T04:00:00Z","reviews":[{"state":"CHANGES_REQUESTED"},{"state":"APPROVED"}]}]`}
	sys.Cmds[cmdAbandoned] = platformtest.Result{Out: `[{"number":4}]`}
	sys.Cmds[cmdHead] = platformtest.Result{Out: "origin/develop\n"}
	sys.Cmds[cmdRuns] = platformtest.Result{Out: `[{"workflowName":"CI","conclusion":"failure","createdAt":"2026-10-03T03:00:00Z"}]`}
	got, err := vcs.GitHub{Sys: sys}.Workload(context.Background(), "/work")
	want := model.Workload{
		Issues: 2, Overdue: 1, MostLate: 2, LatestIssue: 7, Undated: 1, Stale: 1, Urgent: 1,
		Open: 1, Drafts: 1, MyTurn: 1, Merged: 2, LeadP50: 3 * time.Hour, Reviewed: 2, FirstPass: 1, Abandoned: 1,
		Red: 1, RedWorkflow: "CI", RedSince: time.Date(2026, 10, 3, 3, 0, 0, 0, time.UTC),
	}
	if err != nil || got != want {
		t.Errorf("Workload() = %+v, %v\nwant %+v", got, err, want)
	}
}

func TestWorkloadWithoutGitHub(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	sys.Cmds[cmdIssues] = platformtest.Result{Err: errExit}
	_, err := vcs.GitHub{Sys: sys}.Workload(context.Background(), "/work")
	if !errors.Is(err, repository.ErrNone) {
		t.Errorf("a repository gh cannot read has no workload: %v", err)
	}
}

func TestPullRequestRoundsAndCI(t *testing.T) {
	t.Parallel()
	const command = "gh pr view --json number,reviewDecision,statusCheckRollup,additions,deletions,changedFiles,createdAt,isDraft,mergeable,assignees,reviewRequests,latestReviews,reviews"
	tests := []struct {
		name   string
		out    string
		rounds int
		ci     time.Duration
	}{
		{
			"two rounds of changes, checks that took six minutes, a status without times",
			`{"number":7,"reviews":[{"state":"CHANGES_REQUESTED"},{"state":"COMMENTED"},{"state":"CHANGES_REQUESTED"},{"state":"APPROVED"}],
				"statusCheckRollup":[
					{"conclusion":"SUCCESS","startedAt":"2026-10-03T03:00:00Z","completedAt":"2026-10-03T03:04:00Z"},
					{"conclusion":"SKIPPED","startedAt":"2026-10-03T03:01:00Z","completedAt":"2026-10-03T03:06:00Z"},
					{"state":"SUCCESS"},
					{"conclusion":"SUCCESS","startedAt":"0001-01-01T00:00:00Z","completedAt":"0001-01-01T00:00:00Z"}]}`,
			2, 6 * time.Minute,
		},
		{
			"a check still running has no duration yet",
			`{"number":7,"statusCheckRollup":[{"conclusion":"SUCCESS","startedAt":"2026-10-03T03:00:00Z","completedAt":"2026-10-03T03:04:00Z"},{"status":"IN_PROGRESS","startedAt":"2026-10-03T03:01:00Z"}]}`,
			0, 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.Cmds[command] = platformtest.Result{Out: tt.out}
			got, err := vcs.GitHub{Sys: sys}.PullRequest(context.Background(), "/work", "x")
			if err != nil || got.Rounds != tt.rounds || got.CIDuration != tt.ci {
				t.Errorf("PullRequest() = rounds %d, CI %v, %v; want %d, %v", got.Rounds, got.CIDuration, err, tt.rounds, tt.ci)
			}
		})
	}
}
