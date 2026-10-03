// Package vcs reads the working tree with git and its pull request with the
// GitHub CLI.
package vcs

import (
	"context"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform"
)

// Git reads the working tree by running git. History reads what the commits
// say; it is nil in tests, where the history is read uncached.
type Git struct {
	Sys     platform.System
	History repository.HistoryReader
}

// History reads what the commits of a branch say by running git.
type History struct {
	Sys platform.System
}

var (
	_ repository.GitReader     = Git{}
	_ repository.HistoryReader = History{}
)

// runner returns a function that runs git in a directory and returns its
// output.
func runner(ctx context.Context, sys platform.System, dir string) func(args ...string) string {
	return func(args ...string) string {
		// --no-optional-locks: `git status` refreshes the index when it can, and
		// takes index.lock to do so. A status line runs several times a second
		// and its git is killed when the timeout passes; killed with the lock
		// held, it leaves an index.lock that stops the user's next commit.
		//
		// git exits with an error where there is simply nothing to count (a
		// branch without an upstream), so only the output is read.
		out, _ := sys.Run(ctx, platform.Cmd{Name: "git", Args: append([]string{"--no-optional-locks", "-C", dir}, args...)})
		return out
	}
}

// History implements repository.HistoryReader.
func (h History) History(ctx context.Context, dir, branch string) (model.History, error) {
	run := runner(ctx, h.Sys, dir)
	var history model.History
	var wg sync.WaitGroup
	wg.Go(func() {
		countCommits(&history, run("log", "--since=midnight", "--numstat", "--format=%x1e%s%x1f%(trailers:key=Co-authored-by,valueonly,separator=%x2C)%x1f", "HEAD"))
	})
	wg.Go(func() { readHabits(&history, branch, run, h.Sys.Now()) })
	wg.Go(func() {
		history.SwitchesToday = countSwitches(run("log", "-g", "--date=unix", "--format=%gd%x09%gs", "-n", reflogLimit, "HEAD"), h.Sys.Now())
	})
	wg.Go(func() { history.FetchedAt = h.fetchedAt(dir, run) })
	wg.Wait()
	return history, nil
}

// reflogLimit bounds the reflog entries read for today's branch switches.
const reflogLimit = "300"

// countSwitches counts today's branch switches in the reflog of HEAD: one
// entry per line, "HEAD@{<unix time>}<TAB><message>", newest first.
func countSwitches(out string, now time.Time) int {
	y, m, d := now.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	n := 0
	for line := range strings.Lines(out) {
		selector, message, ok := strings.Cut(strings.TrimRight(line, "\r\n"), "\t")
		at, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(selector, "HEAD@{"), "}"), 10, 64)
		if !ok || err != nil {
			continue
		}
		if time.Unix(at, 0).Before(midnight) {
			break
		}
		if strings.HasPrefix(message, "checkout: moving from ") {
			n++
		}
	}
	return n
}

// fetchedAt returns when the remote was last fetched: the time of FETCH_HEAD
// in the repository's common git directory, shared by its worktrees.
func (h History) fetchedAt(dir string, run func(args ...string) string) time.Time {
	common := strings.TrimSpace(run("rev-parse", "--git-common-dir"))
	if common == "" {
		return time.Time{}
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(dir, common)
	}
	at, err := h.Sys.ModTime(filepath.Join(common, "FETCH_HEAD"))
	if err != nil {
		return time.Time{}
	}
	return at
}

// Git implements repository.GitReader. Only the branch is required: a count
// that git cannot give (no upstream, no commit yet) is left at zero.
func (g Git) Git(ctx context.Context, dir string) (model.Git, error) {
	run := runner(ctx, g.Sys, dir)
	branch := strings.TrimSpace(run("branch", "--show-current"))
	if branch == "" {
		return model.Git{}, repository.ErrNone
	}
	// The questions are independent of each other, so they are asked at once:
	// a status line waits for the slowest instead of the sum. Each goroutine
	// writes fields of its own.
	git := model.Git{Branch: branch}
	var wg sync.WaitGroup
	wg.Go(func() { git.Stashes = strings.Count(run("stash", "list"), "\n") })
	wg.Go(func() { countStatus(&git, run("status", "--porcelain")) })
	wg.Go(func() {
		readDiff(&git, run("diff", "HEAD", "--numstat", "--patch", "--unified=0", "--no-color", "--no-ext-diff", "--no-renames"))
	})
	wg.Go(func() { git.Operation = g.operation(strings.TrimSpace(run("rev-parse", "--absolute-git-dir"))) })
	wg.Go(func() {
		// "behind<TAB>ahead" for upstream...HEAD.
		if counts := strings.Fields(run("rev-list", "--left-right", "--count", "@{upstream}...HEAD")); len(counts) == 2 {
			behind, errBehind := strconv.Atoi(counts[0])
			ahead, errAhead := strconv.Atoi(counts[1])
			if errBehind == nil && errAhead == nil {
				git.Behind, git.Ahead = behind, ahead
			}
		}
	})
	wg.Go(func() {
		if committed, err := strconv.ParseInt(strings.TrimSpace(run("log", "-1", "--format=%ct")), 10, 64); err == nil {
			git.LastCommit = time.Unix(committed, 0)
		}
	})
	var history model.History
	wg.Go(func() {
		var reader repository.HistoryReader = History{Sys: g.Sys}
		if g.History != nil {
			reader = g.History
		}
		// A history that cannot be read leaves its counts at zero, as a git
		// that cannot count does.
		history, _ = reader.History(ctx, dir, branch)
	})
	wg.Wait()
	history.Apply(&git)
	return git, nil
}

// readHabits reads what the habits of a team's repository are measured by:
// the default branch, how old the branch and its unpushed commits are, the
// merged branches left behind and the user's run of days with commits.
func readHabits(git *model.History, branch string, run func(args ...string) string, now time.Time) {
	git.DefaultBranch = strings.TrimPrefix(strings.TrimSpace(run("symbolic-ref", "--short", "refs/remotes/origin/HEAD")), "origin/")
	var wg sync.WaitGroup
	var start, unpushed time.Time
	var upstream bool
	if git.DefaultBranch != "" && !model.IsDefaultBranch(branch, git.DefaultBranch) {
		wg.Go(func() { start = oldest(run("log", "--format=%ct", "origin/"+git.DefaultBranch+"..HEAD")) })
		wg.Go(func() {
			if n, err := strconv.Atoi(strings.TrimSpace(run("rev-list", "--count", "HEAD..origin/"+git.DefaultBranch))); err == nil {
				git.BehindDefault = n
			}
		})
		wg.Go(func() {
			git.OldBranches = countOld(run("for-each-ref", "--format=%(refname:short)%09%(committerdate:unix)", "refs/heads"), branch, git.DefaultBranch, now)
		})
		wg.Go(func() {
			git.MergedBranches = countMerged(run("branch", "--merged", git.DefaultBranch, "--format=%(refname:short)"), branch, git.DefaultBranch)
		})
	}
	wg.Go(func() {
		upstream = strings.TrimSpace(run("rev-parse", "--abbrev-ref", "@{upstream}")) != ""
		if upstream {
			unpushed = oldest(run("log", "--format=%ct", "@{upstream}..HEAD"))
		}
	})
	wg.Go(func() {
		if email := strings.TrimSpace(run("config", "user.email")); email != "" {
			git.Streak, git.LongestStreak = model.Streaks(strings.Fields(run("log", "--since=60.days", "--author="+email, "--format=%cs", "HEAD")), now)
		}
	})
	wg.Wait()
	git.BranchStart = start
	git.OldestUnpushed = unpushed
	if !upstream {
		// A branch never pushed: every commit of its own waits to be pushed.
		git.OldestUnpushed = start
	}
}

// oldest returns the earliest of commit times, one Unix time per line, or the
// zero time when there is none.
func oldest(out string) time.Time {
	var first time.Time
	for field := range strings.FieldsSeq(out) {
		if at, err := strconv.ParseInt(field, 10, 64); err == nil && (first.IsZero() || time.Unix(at, 0).Before(first)) {
			first = time.Unix(at, 0)
		}
	}
	return first
}

// countOld counts the local branches whose last commit is older than
// model.OldBranchAfter, the current one and the default left out. One branch
// per line: "<name><TAB><unix time>".
func countOld(out, current, defaultBranch string, now time.Time) int {
	n := 0
	for line := range strings.Lines(out) {
		name, unix, ok := strings.Cut(strings.TrimRight(line, "\r\n"), "\t")
		at, err := strconv.ParseInt(unix, 10, 64)
		if !ok || err != nil || name == current || name == defaultBranch {
			continue
		}
		if now.Sub(time.Unix(at, 0)) > model.OldBranchAfter {
			n++
		}
	}
	return n
}

// countMerged counts the branches merged into the default branch, the current
// one and the default left out.
func countMerged(out, current, defaultBranch string) int {
	n := 0
	for name := range strings.FieldsSeq(out) {
		if name != current && name != defaultBranch {
			n++
		}
	}
	return n
}

// commitFields are the parts of a record of today's commits: the subject, the
// co-authors and the lines of each file.
const commitFields = 3

// countCommits counts today's commits from `git log` records: a record
// separator, the subject and the co-authors each ended by a unit separator,
// then the lines of each file (--numstat). A subject or a trailer cannot hold
// either separator.
func countCommits(git *model.History, out string) {
	for record := range strings.SplitSeq(out, "\x1e") {
		fields := strings.SplitN(record, "\x1f", commitFields)
		if len(fields) != commitFields {
			continue
		}
		subject, coauthors, stat := fields[0], fields[1], fields[2]
		git.CommitsToday++
		if model.IsFixCommit(subject) {
			git.FixesToday++
		}
		if model.IsRevertCommit(subject) {
			git.RevertsToday++
		}
		if model.IsAICoauthor(coauthors) {
			git.AICommitsToday++
		}
		if model.IsVagueCommit(subject) {
			git.VagueToday++
		}
		if model.IsConventionalCommit(subject) {
			git.ConventionalToday++
		}
		size := 0
		for line := range strings.Lines(stat) {
			if change, ok := numStat(strings.TrimRight(line, "\r\n")); ok {
				git.TodayAdded += change.Added
				git.TodayDeleted += change.Deleted
				size += change.Lines()
			}
		}
		git.LargestToday = max(git.LargestToday, size)
	}
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
		if path := line[len("XY "):]; len(line) > len("XY ") && model.IsJunkPath(path) {
			git.Junk++
			if code[0] != ' ' && code != "??" {
				git.JunkStaged++
			}
		}
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
	var lint model.Lint
	defer func() {
		lint.Done()
		git.Rules, git.RulesUnchecked = lint.Found, lint.Skipped
	}()
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
			lint.File(file)
		case strings.HasPrefix(line, "@@"):
			lint.Gap()
		case strings.HasPrefix(line, "--- "):
			// The old name of the file; the new one follows.
		case strings.HasPrefix(line, "+"):
			if model.MarksConflict(line[1:]) {
				git.ConflictMarkers++
			}
			readAdded(git, file, line[1:])
			lint.Added(line[1:])
		case strings.HasPrefix(line, "-"):
			lint.Gap()
			readRemoved(git, file, line[1:])
		}
	}
}

// readAdded counts what an added line brings: a debt, a test double, a
// dependency.
func readAdded(git *model.Git, file, line string) {
	if model.IsTestPath(file) && model.IsSourcePath(file) {
		if model.MarksSkip(line) {
			git.SkipsAdded++
		}
		if model.MarksAssert(line) {
			git.AssertsAdded++
		}
	}
	switch {
	case model.IsSourcePath(file) && model.MarksDebt(line):
		git.DebtAdded++
	case model.IsTestPath(file) && model.IsSourcePath(file) && model.MarksMock(line):
		git.MocksAdded++
	case model.AddsDependency(file, line):
		git.DepsAdded++
	}
}

// readRemoved counts what a deleted line takes away: a debt, an assertion.
func readRemoved(git *model.Git, file, line string) {
	if !model.IsSourcePath(file) {
		return
	}
	if model.MarksDebt(line) {
		git.DebtRemoved++
	}
	if model.IsTestPath(file) && model.MarksAssert(line) {
		git.AssertsRemoved++
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
