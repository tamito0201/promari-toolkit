package vcs

import (
	"context"
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

var _ repository.PullRequestReader = GitHub{}

// PullRequest implements repository.PullRequestReader. gh reads the branch
// from the working tree, so only dir is passed on.
func (g GitHub) PullRequest(ctx context.Context, dir, _ string) (model.PullRequest, error) {
	out, err := g.Sys.Run(ctx, platform.Cmd{
		Name:    "gh",
		Args:    []string{"pr", "view", "--json", "number,reviewDecision,statusCheckRollup,additions,deletions,changedFiles,createdAt,isDraft,mergeable"},
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
	}
	if created, err := time.Parse(time.RFC3339, jsonx.Or[string](view, "createdAt")); err == nil {
		pull.Created = created
	}
	for _, check := range jsonx.Or[[]jsonx.Object](view, "statusCheckRollup") {
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
	return pull, nil
}
