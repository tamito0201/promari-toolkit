package vcs_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform/platformtest"
	"promari-statusline/internal/infrastructure/vcs"
)

var t0 = time.Date(2026, 10, 3, 4, 9, 0, 0, time.UTC)

var errExit = errors.New("exit status 128")

func TestGit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cmds map[string]platformtest.Result
		want model.Git
		err  error
	}{
		{
			"a branch with everything to report",
			map[string]platformtest.Result{
				"git --no-optional-locks -C /work branch --show-current":                            {Out: "develop\n"},
				"git --no-optional-locks -C /work status --porcelain":                               {Out: " M a.go\n?? b.go\n\n"},
				"git --no-optional-locks -C /work stash list":                                       {Out: "stash@{0}: x\nstash@{1}: y\n"},
				"git --no-optional-locks -C /work rev-list --left-right --count @{upstream}...HEAD": {Out: "16\t3\n"},
				"git --no-optional-locks -C /work log -1 --format=%ct":                              {Out: "1759464540\n"},
			},
			model.Git{Branch: "develop", Changed: 2, Stashes: 2, Behind: 16, Ahead: 3, LastCommit: time.Unix(1759464540, 0)},
			nil,
		},
		{
			"a new branch: no upstream, no commit, nothing changed",
			map[string]platformtest.Result{
				"git --no-optional-locks -C /work branch --show-current":                            {Out: "feature\n"},
				"git --no-optional-locks -C /work rev-list --left-right --count @{upstream}...HEAD": {Err: errExit},
				"git --no-optional-locks -C /work log -1 --format=%ct":                              {Err: errExit},
			},
			model.Git{Branch: "feature"},
			nil,
		},
		{
			"counts that are not numbers are left at zero",
			map[string]platformtest.Result{
				"git --no-optional-locks -C /work branch --show-current":                            {Out: "feature\n"},
				"git --no-optional-locks -C /work rev-list --left-right --count @{upstream}...HEAD": {Out: "x y\n"},
				"git --no-optional-locks -C /work log -1 --format=%ct":                              {Out: "yesterday\n"},
			},
			model.Git{Branch: "feature"},
			nil,
		},
		{"a detached HEAD has no branch", map[string]platformtest.Result{"git --no-optional-locks -C /work branch --show-current": {Out: "\n"}}, model.Git{}, repository.ErrNone},
		{"not a repository, or no git", nil, model.Git{}, repository.ErrNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.Cmds = tt.cmds
			got, err := vcs.Git{Sys: sys}.Git(context.Background(), "/work")
			if got != tt.want || !errors.Is(err, tt.err) {
				t.Errorf("Git() = %+v, %v; want %+v, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestGitHub(t *testing.T) {
	t.Parallel()
	const command = "gh pr view --json number,reviewDecision,statusCheckRollup"
	tests := []struct {
		name string
		out  platformtest.Result
		want model.PullRequest
		err  error
	}{
		{
			"checks of every kind",
			platformtest.Result{Out: `{"number":2996,"reviewDecision":"CHANGES_REQUESTED","statusCheckRollup":[
				{"conclusion":"SUCCESS"},{"conclusion":"neutral"},{"conclusion":"SKIPPED"},{"state":"SUCCESS"},
				{"conclusion":"FAILURE"},{"conclusion":"CANCELLED"},{"conclusion":"TIMED_OUT"},{"conclusion":"ACTION_REQUIRED"},{"state":"ERROR"},{"state":"FAILURE"},
				{"conclusion":"","state":"PENDING"},{"status":"IN_PROGRESS"}]}`},
			model.PullRequest{Number: 2996, Passed: 4, Failed: 6, Pending: 2, Review: model.ReviewChangesRequested},
			nil,
		},
		{"no checks and no review", platformtest.Result{Out: `{"number":7,"reviewDecision":"","statusCheckRollup":[]}`}, model.PullRequest{Number: 7}, nil},
		{
			"members of an unexpected type are skipped, the rest is kept",
			platformtest.Result{Out: `{"number":7,"reviewDecision":null,"statusCheckRollup":"none"}`},
			model.PullRequest{Number: 7},
			nil,
		},
		{"no pull request for the branch", platformtest.Result{Err: errExit}, model.PullRequest{}, repository.ErrNone},
		{"output that is not JSON", platformtest.Result{Out: "no pull requests found"}, model.PullRequest{}, repository.ErrNone},
		{"no number", platformtest.Result{Out: `{"reviewDecision":"APPROVED"}`}, model.PullRequest{}, repository.ErrNone},
		{"a number of the wrong type", platformtest.Result{Out: `{"number":"7"}`}, model.PullRequest{}, repository.ErrNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.Cmds[command] = tt.out
			got, err := vcs.GitHub{Sys: sys}.PullRequest(context.Background(), "/work", "develop")
			if got != tt.want || !errors.Is(err, tt.err) {
				t.Errorf("PullRequest() = %+v, %v; want %+v, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

// A git that is killed while it holds index.lock leaves the lock behind, and
// the user's next commit fails. No git the status line starts may take it.
func TestGitNeverTakesTheIndexLock(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	sys.Cmds["git --no-optional-locks -C /work branch --show-current"] = platformtest.Result{Out: "develop\n"}
	if _, err := (vcs.Git{Sys: sys}).Git(t.Context(), "/work"); err != nil {
		t.Fatal(err)
	}
	calls := sys.Calls()
	if len(calls) == 0 {
		t.Fatal("git was not run")
	}
	for _, call := range calls {
		if !strings.HasPrefix(call, "git --no-optional-locks ") {
			t.Errorf("%q may take index.lock", call)
		}
	}
}
