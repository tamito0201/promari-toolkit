// Package vcs reads the working tree with git and its pull request with the
// GitHub CLI.
package vcs

import (
	"context"
	"strconv"
	"strings"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform"
)

// Git reads the working tree by running git.
type Git struct {
	Sys platform.System
}

var _ repository.GitReader = Git{}

// Git implements repository.GitReader. Only the branch is required: a count
// that git cannot give (no upstream, no commit yet) is left at zero.
func (g Git) Git(ctx context.Context, dir string) (model.Git, error) {
	run := func(args ...string) string {
		// --no-optional-locks: `git status` refreshes the index when it can, and
		// takes index.lock to do so. A status line runs several times a second
		// and its git is killed when the timeout passes; killed with the lock
		// held, it leaves an index.lock that stops the user's next commit.
		//
		// git exits with an error where there is simply nothing to count (a
		// branch without an upstream), so only the output is read.
		out, _ := g.Sys.Run(ctx, platform.Cmd{Name: "git", Args: append([]string{"--no-optional-locks", "-C", dir}, args...)})
		return out
	}
	branch := strings.TrimSpace(run("branch", "--show-current"))
	if branch == "" {
		return model.Git{}, repository.ErrNone
	}
	git := model.Git{
		Branch:  branch,
		Changed: nonBlankLines(run("status", "--porcelain")),
		Stashes: strings.Count(run("stash", "list"), "\n"),
	}
	// "behind<TAB>ahead" for upstream...HEAD.
	if counts := strings.Fields(run("rev-list", "--left-right", "--count", "@{upstream}...HEAD")); len(counts) == 2 {
		behind, errBehind := strconv.Atoi(counts[0])
		ahead, errAhead := strconv.Atoi(counts[1])
		if errBehind == nil && errAhead == nil {
			git.Behind, git.Ahead = behind, ahead
		}
	}
	if committed, err := strconv.ParseInt(strings.TrimSpace(run("log", "-1", "--format=%ct")), 10, 64); err == nil {
		git.LastCommit = time.Unix(committed, 0)
	}
	return git, nil
}

func nonBlankLines(s string) int {
	n := 0
	for line := range strings.Lines(s) {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}
