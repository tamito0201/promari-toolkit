package vcs

import (
	"context"
	"encoding/json/v2"
	"strconv"
	"strings"
	"sync"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform"
	"promari-statusline/pkg/jsonx"
)

// The most items asked for in one list: enough for anyone's own work, few
// enough to answer within ghTimeout.
const (
	issueLimit = 200
	pullLimit  = 100
	runLimit   = 30
)

var _ repository.WorkloadReader = GitHub{}

// Workload implements repository.WorkloadReader. It asks gh five questions at
// once, each in its own goroutine writing its own part: the assigned issues,
// the open pull requests, the merged and the abandoned ones, and the runs of
// the default branch. A question that fails leaves its part empty; when all
// fail (no gh, no login, no remote) there is no workload to show.
func (g GitHub) Workload(ctx context.Context, dir string) (model.Workload, error) {
	now := g.Sys.Now()
	since := now.Add(-model.FlowWindow).Format("2006-01-02")
	var (
		issues                    []model.IssueItem
		open, merged              []model.PullItem
		abandoned                 int
		runs                      []model.RunItem
		okIssues, okOpen, okFlows bool
		wg                        sync.WaitGroup
	)
	wg.Go(func() {
		issues, okIssues = g.issues(ctx, dir)
	})
	wg.Go(func() {
		open, okOpen = g.pulls(ctx, dir, "--state", "open", "--json", "number,updatedAt,createdAt,isDraft,reviewDecision")
	})
	wg.Go(func() {
		merged, okFlows = g.pulls(ctx, dir, "--state", "merged", "--search", "merged:>="+since, "--json", "number,createdAt,mergedAt,reviews")
	})
	wg.Go(func() {
		closed, ok := g.pulls(ctx, dir, "--state", "closed", "--search", "is:unmerged closed:>="+since, "--json", "number")
		if ok {
			abandoned = len(closed)
		}
	})
	wg.Go(func() { runs = g.runs(ctx, dir) })
	wg.Wait()
	if !okIssues && !okOpen && !okFlows {
		return model.Workload{}, repository.ErrNone
	}
	var w model.Workload
	w.AddIssues(issues, now)
	w.AddOpen(open, now)
	w.AddMerged(merged)
	w.Abandoned = abandoned
	w.AddRuns(runs)
	w.IssuesCapped = len(issues) == issueLimit
	w.MergedCapped = len(merged) == pullLimit
	return w, nil
}

// list runs gh and decodes the JSON array it prints.
func (g GitHub) list(ctx context.Context, dir string, args ...string) ([]jsonx.Object, bool) {
	out, err := g.Sys.Run(ctx, platform.Cmd{Name: "gh", Args: args, Dir: dir, Timeout: ghTimeout})
	var list []jsonx.Object
	if err != nil || json.Unmarshal([]byte(out), &list) != nil {
		return nil, false
	}
	return list, true
}

func (g GitHub) issues(ctx context.Context, dir string) ([]model.IssueItem, bool) {
	list, ok := g.list(ctx, dir, "issue", "list", "--assignee", "@me", "--state", "open", "--limit", strconv.Itoa(issueLimit),
		"--json", "number,createdAt,updatedAt,milestone,labels")
	items := make([]model.IssueItem, 0, len(list))
	for _, o := range list {
		it := model.IssueItem{
			Number:  int(jsonx.Or[float64](o, "number")),
			Created: parseTime(jsonx.Or[string](o, "createdAt")),
			Updated: parseTime(jsonx.Or[string](o, "updatedAt")),
		}
		if milestone, ok := jsonx.Get[jsonx.Object](o, "milestone"); ok {
			it.Due = parseTime(jsonx.Or[string](milestone, "dueOn"))
		}
		for _, label := range jsonx.Or[[]jsonx.Object](o, "labels") {
			it.Labels = append(it.Labels, jsonx.Or[string](label, "name"))
		}
		items = append(items, it)
	}
	return items, ok
}

func (g GitHub) pulls(ctx context.Context, dir string, args ...string) ([]model.PullItem, bool) {
	list, ok := g.list(ctx, dir, append([]string{"pr", "list", "--author", "@me", "--limit", strconv.Itoa(pullLimit)}, args...)...)
	items := make([]model.PullItem, 0, len(list))
	for _, o := range list {
		it := model.PullItem{
			Number:           int(jsonx.Or[float64](o, "number")),
			Created:          parseTime(jsonx.Or[string](o, "createdAt")),
			Updated:          parseTime(jsonx.Or[string](o, "updatedAt")),
			Merged:           parseTime(jsonx.Or[string](o, "mergedAt")),
			Draft:            jsonx.Or[bool](o, "isDraft"),
			ChangesRequested: jsonx.Or[string](o, "reviewDecision") == "CHANGES_REQUESTED",
		}
		// gh lists the reviews oldest first; the author's own comments on
		// the pull request are reviews too, and are not a reviewer's verdict.
		for _, review := range jsonx.Or[[]jsonx.Object](o, "reviews") {
			state := jsonx.Or[string](review, "state")
			if state != "APPROVED" && state != "CHANGES_REQUESTED" {
				continue
			}
			it.Reviewed, it.FirstPass = true, state == "APPROVED"
			break
		}
		items = append(items, it)
	}
	return items, ok
}

// runs reads the last runs of the default branch, found from origin/HEAD.
func (g GitHub) runs(ctx context.Context, dir string) []model.RunItem {
	head := strings.TrimSpace(runner(ctx, g.Sys, dir)("symbolic-ref", "--short", "refs/remotes/origin/HEAD"))
	branch, found := strings.CutPrefix(head, "origin/")
	if !found || branch == "" {
		return nil
	}
	list, _ := g.list(ctx, dir, "run", "list", "--branch", branch, "--limit", strconv.Itoa(runLimit), "--json", "workflowName,conclusion,createdAt")
	items := make([]model.RunItem, 0, len(list))
	for _, o := range list {
		items = append(items, model.RunItem{
			Workflow:   jsonx.Or[string](o, "workflowName"),
			Conclusion: jsonx.Or[string](o, "conclusion"),
			Created:    parseTime(jsonx.Or[string](o, "createdAt")),
		})
	}
	return items
}

// parseTime reads an RFC 3339 time, zero when there is none.
func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
