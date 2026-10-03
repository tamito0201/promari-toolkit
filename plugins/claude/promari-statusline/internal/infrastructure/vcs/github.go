package vcs

import (
	"context"
	"encoding/json/v2"
	"slices"
	"strings"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform"
	"promari-statusline/pkg/jsonx"
)

// ghTimeout bounds the GitHub CLI, which asks the network.
const ghTimeout = 4 * time.Second

// GitHub reads the pull request of a branch by running gh.
type GitHub struct {
	Sys platform.System
}

var (
	_ repository.PullRequestReader = GitHub{}
	_ repository.ReviewQueueReader = GitHub{}
)

// ReviewQueue implements repository.ReviewQueueReader.
func (g GitHub) ReviewQueue(ctx context.Context, dir string) (model.ReviewQueue, error) {
	out, err := g.Sys.Run(ctx, platform.Cmd{
		Name:    "gh",
		Args:    []string{"pr", "list", "--search", "review-requested:@me", "--json", "createdAt"},
		Dir:     dir,
		Timeout: ghTimeout,
	})
	var list []jsonx.Object
	if err != nil || json.Unmarshal([]byte(out), &list) != nil || len(list) == 0 {
		return model.ReviewQueue{}, repository.ErrNone
	}
	queue := model.ReviewQueue{Count: len(list)}
	for _, pr := range list {
		if at, err := time.Parse(time.RFC3339, jsonx.Or[string](pr, "createdAt")); err == nil && (queue.Oldest.IsZero() || at.Before(queue.Oldest)) {
			queue.Oldest = at
		}
	}
	return queue, nil
}

// PullRequest implements repository.PullRequestReader. gh reads the branch
// from the working tree, so only dir is passed on.
func (g GitHub) PullRequest(ctx context.Context, dir, _ string) (model.PullRequest, error) {
	out, err := g.Sys.Run(ctx, platform.Cmd{
		Name:    "gh",
		Args:    []string{"pr", "view", "--json", "number,reviewDecision,statusCheckRollup,additions,deletions,changedFiles,createdAt,isDraft,mergeable,assignees,reviewRequests,latestReviews,reviews"},
		Dir:     dir,
		Timeout: ghTimeout,
	})
	if err != nil {
		// gh exits with an error for a branch without a pull request as well as
		// for a missing login; neither has a pull request to show.
		return model.PullRequest{}, repository.ErrNone
	}
	view, ok := jsonx.Parse([]byte(out))
	number, hasNumber := jsonx.Get[float64](view, "number")
	if !ok || !hasNumber || number <= 0 {
		return model.PullRequest{}, repository.ErrNone
	}
	pull := model.PullRequest{
		Number:    int(number),
		Review:    model.ReviewDecision(jsonx.Or[string](view, "reviewDecision")),
		Additions: int(jsonx.Or[float64](view, "additions")),
		Deletions: int(jsonx.Or[float64](view, "deletions")),
		Files:     int(jsonx.Or[float64](view, "changedFiles")),
		Draft:     jsonx.Or[bool](view, "isDraft"),
		Conflicts: jsonx.Or[string](view, "mergeable") == "CONFLICTING",
		Assignees: len(jsonx.Or[[]jsonx.Object](view, "assignees")),
		Reviewers: len(jsonx.Or[[]jsonx.Object](view, "reviewRequests")),
		Reviews:   len(jsonx.Or[[]jsonx.Object](view, "latestReviews")),
		// gh names every member it was asked for, empty or not.
		PeopleKnown: view["assignees"] != nil && view["reviewRequests"] != nil,
	}
	if created, err := time.Parse(time.RFC3339, jsonx.Or[string](view, "createdAt")); err == nil {
		pull.Created = created
	}
	for _, review := range jsonx.Or[[]jsonx.Object](view, "reviews") {
		if jsonx.Or[string](review, "state") == "CHANGES_REQUESTED" {
			pull.Rounds++
		}
	}
	var first, last time.Time
	for _, check := range jsonx.Or[[]jsonx.Object](view, "statusCheckRollup") {
		// A check run has its times; a commit status has none and is left out
		// of the duration.
		if at, err := time.Parse(time.RFC3339, jsonx.Or[string](check, "startedAt")); err == nil && at.Year() > 1 && (first.IsZero() || at.Before(first)) {
			first = at
		}
		if at, err := time.Parse(time.RFC3339, jsonx.Or[string](check, "completedAt")); err == nil && at.Year() > 1 && at.After(last) {
			last = at
		}
		conclusion := strings.ToUpper(jsonx.Or[string](check, "conclusion"))
		state := strings.ToUpper(jsonx.Or[string](check, "state"))
		switch {
		case slices.Contains([]string{"SUCCESS", "NEUTRAL", "SKIPPED"}, conclusion) || state == "SUCCESS":
			pull.Passed++
		case slices.Contains([]string{"FAILURE", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED"}, conclusion) ||
			slices.Contains([]string{"FAILURE", "ERROR"}, state):
			pull.Failed++
		default:
			pull.Pending++
		}
	}
	if pull.Pending == 0 && !first.IsZero() && last.After(first) {
		pull.CIDuration = last.Sub(first)
	}
	return pull, nil
}
