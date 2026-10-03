// Package vcs reads the working tree with git and its pull request with the
// GitHub CLI.
package vcs

import (
	"context"
	"path/filepath"
	"slices"
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
	git := model.Git{Branch: branch, Stashes: strings.Count(run("stash", "list"), "\n")}
	countStatus(&git, run("status", "--porcelain"))
	git.Inserted, git.Deleted = shortStat(run("diff", "--shortstat", "HEAD"))
	git.Operation = g.operation(strings.TrimSpace(run("rev-parse", "--absolute-git-dir")))
	if n, err := strconv.Atoi(strings.TrimSpace(run("rev-list", "--count", "--since=midnight", "HEAD"))); err == nil {
		git.CommitsToday = n
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

// conflictCodes are the two-letter states of `git status --porcelain` for an
// unmerged path.
var conflictCodes = []string{"DD", "AU", "UD", "UA", "DU", "AA", "UU"}

// countStatus counts the files of `git status --porcelain`: every file, the
// staged, the untracked and the conflicted.
func countStatus(git *model.Git, porcelain string) {
	for line := range strings.Lines(porcelain) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		git.Changed++
		// The state is the first two characters; the newline is not one of them.
		if line = strings.TrimRight(line, "\r\n"); len(line) < len("XY") {
			continue
		}
		code := line[:len("XY")]
		switch {
		case code == "??":
			git.Untracked++
		case slices.Contains(conflictCodes, code):
			git.Conflicts++
		case code[0] != ' ':
			git.Staged++
		}
	}
}

// shortStat reads the lines inserted and deleted from `git diff --shortstat`:
// " 3 files changed, 120 insertions(+), 30 deletions(-)". Either part is left
// out when it is zero.
func shortStat(out string) (inserted, deleted int) {
	for part := range strings.SplitSeq(strings.TrimSpace(out), ",") {
		fields := strings.Fields(part)
		if len(fields) < 2 {
			continue
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		switch {
		case strings.HasPrefix(fields[1], "insertion"):
			inserted = n
		case strings.HasPrefix(fields[1], "deletion"):
			deleted = n
		}
	}
	return inserted, deleted
}

// operations are the operations git leaves a mark for in its directory while
// they are in progress, in the order they are looked for.
var operations = []struct{ mark, name string }{
	{"rebase-merge", "rebase"},
	{"rebase-apply", "rebase"},
	{"MERGE_HEAD", "merge"},
	{"CHERRY_PICK_HEAD", "cherry-pick"},
	{"REVERT_HEAD", "revert"},
	{"BISECT_LOG", "bisect"},
}

// operation returns the operation in progress in a git directory, or "".
func (g Git) operation(gitDir string) string {
	if gitDir == "" {
		return ""
	}
	for _, op := range operations {
		if _, err := g.Sys.ModTime(filepath.Join(gitDir, op.mark)); err == nil {
			return op.name
		}
	}
	return ""
}
