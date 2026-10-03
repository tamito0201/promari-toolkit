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
	readDiff(&git, run("diff", "HEAD", "--numstat", "--patch", "--unified=0", "--no-color", "--no-ext-diff", "--no-renames"))
	git.Operation = g.operation(strings.TrimSpace(run("rev-parse", "--absolute-git-dir")))
	for subject := range strings.Lines(run("log", "--since=midnight", "--format=%s", "HEAD")) {
		git.CommitsToday++
		if model.IsFixCommit(subject) {
			git.FixesToday++
		}
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

// readDiff reads `git diff --numstat --patch --unified=0`: first a line per
// file ("added<TAB>deleted<TAB>path", "-" for a binary file), then the patch,
// whose added and deleted lines are searched for the markers of a debt. One run of git
// gives both.
func readDiff(git *model.Git, out string) {
	file := ""
	inPatch := false
	for line := range strings.Lines(out) {
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "diff --git "):
			inPatch = true
		case !inPatch:
			if change, ok := numStat(line); ok {
				git.Changes = append(git.Changes, change)
				git.Inserted += change.Added
				git.Deleted += change.Deleted
			}
		case strings.HasPrefix(line, "+++ "):
			file = strings.TrimPrefix(strings.TrimPrefix(line, "+++ "), "b/")
		case strings.HasPrefix(line, "--- "):
			// The old name of the file; the new one follows.
		case !model.IsSourcePath(file) || !model.MarksDebt(line):
		case strings.HasPrefix(line, "+"):
			git.DebtAdded++
		case strings.HasPrefix(line, "-"):
			git.DebtRemoved++
		}
	}
}

// numStatFields are the fields of a line of `git diff --numstat`.
const numStatFields = 3

// numStat reads one line of `git diff --numstat`. A binary file changes no
// lines that can be counted.
func numStat(line string) (model.FileChange, bool) {
	fields := strings.SplitN(line, "\t", numStatFields)
	if len(fields) != numStatFields {
		return model.FileChange{}, false
	}
	added, errAdded := strconv.Atoi(fields[0])
	deleted, errDeleted := strconv.Atoi(fields[1])
	if fields[0] == "-" && fields[1] == "-" {
		return model.FileChange{Path: fields[2]}, true
	}
	if errAdded != nil || errDeleted != nil {
		return model.FileChange{}, false
	}
	return model.FileChange{Path: fields[2], Added: added, Deleted: deleted}, true
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
