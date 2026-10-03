package model

import (
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"
)

// The habits of a team's repository, as a training course for new engineers
// teaches them: work on a short branch named "kind/topic", never on the default
// branch, push within the day, commit in steps of 20 to 50 lines with a
// message that says what changed, keep build output out of the repository,
// resolve conflicts at once, delete merged branches and review within 30
// minutes. The numbers below are that course's.
const (
	// BranchTooOld is how long a branch may live before it is no longer a
	// micro-branch: a task is cut to two or three hours.
	BranchTooOld = 3 * time.Hour
	// UnpushedTooOld is how long a commit may wait to be pushed: push within
	// the day.
	UnpushedTooOld = 24 * time.Hour
	// ReviewTooSlow is how long a review request may wait for its review.
	ReviewTooSlow = 30 * time.Minute
	// The lines per commit of a standard step: fewer is a small fix, more than
	// LargeCommit a large change.
	SmallCommit = 10
	LargeCommit = 100
	// The commits per day of a standard pace.
	CommitsPerDayLow  = 2
	CommitsPerDayHigh = 5
	// SelfSolveLimit is how long a newcomer works on a failure alone before
	// asking: the course's manual says to try alone for about 15 minutes, its
	// team exercise to ask after 30.
	SelfSolveLimit = 15 * time.Minute
	// OldBranchAfter is how long a local branch may go without a commit
	// before it is a candidate to abandon: the KPI books' two weeks without
	// progress, and their "abandonments to be actioned".
	OldBranchAfter = 14 * 24 * time.Hour
	// FetchTooOld is how long the remote may go unread before the counts read
	// from it (behind, merged) are no longer current: the KPI books ask for
	// measures no more than a day old.
	FetchTooOld = 24 * time.Hour
	// SteadyStreak is the run of days with commits that marks a steady
	// contribution.
	SteadyStreak = 3
)

// branchName matches a branch named "kind/topic": the kind of change, then
// what it is about.
var branchName = regexp.MustCompile(`^(?:feature|feat|fix|bugfix|hotfix|refactor|docs|doc|chore|test|tests|ci|perf|style|build|release|epic|spike|experiment)/[A-Za-z0-9][A-Za-z0-9._/-]{2,}$`)

// vagueTopics are topics that say nothing about the work.
var vagueTopics = []string{"test", "tmp", "temp", "wip", "work", "update", "fix", "change", "changes", "new", "branch", "aaa", "hoge", "foo"}

// GoodBranchName reports whether a branch is named "kind/topic" with a topic
// that says something.
func GoodBranchName(branch string) bool {
	if !branchName.MatchString(branch) {
		return false
	}
	_, topic, _ := strings.Cut(branch, "/")
	return !slices.Contains(vagueTopics, strings.ToLower(topic))
}

// IsDefaultBranch reports whether a branch is the repository's default one, or
// main or master when the default is unknown.
func IsDefaultBranch(branch, defaultBranch string) bool {
	if defaultBranch != "" {
		return branch == defaultBranch
	}
	return branch == "main" || branch == "master"
}

// vagueSubject matches a commit subject that does not say what changed.
var vagueSubject = regexp.MustCompile(`(?i)^\s*(?:(?:fix|fixes|fixed|update|updates|updated|change|changes|modify|edit|wip|tmp|temp|test|commit|minor|misc|refactor|cleanup|aaa+|\.+|-+|修正|変更|更新|対応|追加|作業中|テスト|コミット|とりあえず)[\s.!。]*|.{1,3})$`)

// IsVagueCommit reports whether a commit's subject says nothing about what
// changed: one word, or three characters or fewer. A subject with a
// conventional prefix is judged by what follows the prefix.
func IsVagueCommit(subject string) bool {
	subject = strings.TrimSpace(subject)
	if kind, rest, found := strings.Cut(subject, ":"); found && len(kind) <= 20 && !strings.ContainsAny(kind, " ") {
		subject = strings.TrimSpace(rest)
	}
	return vagueSubject.MatchString(subject)
}

// conflictMarker matches a line git writes into a file with a conflict.
var conflictMarker = regexp.MustCompile(`^(?:<{7}|={7}|>{7})(?:\s|$)`)

// MarksConflict reports whether a line is a conflict marker left in a file.
func MarksConflict(line string) bool { return conflictMarker.MatchString(line) }

// junkPath matches build output, editor state, dependencies and secrets that
// do not belong in a repository.
var junkPath = regexp.MustCompile(`(?:^|/)(?:bin|obj|\.vs|\.idea|node_modules|__pycache__|\.pytest_cache|TestResults)(?:/|$)|(?:^|/)\.DS_Store$|\.(?:user|suo|pyc|log|swp)$|(?:^|/)\.env(?:\.[^/]*)?$|(?:^|/)Thumbs\.db$`)

// IsJunkPath reports whether a path is something that should not be added.
// git status puts a slash after an untracked directory ("obj/") and quotes a
// path with special characters.
func IsJunkPath(path string) bool { return junkPath.MatchString(strings.Trim(path, `"`)) }

// CommitSize returns the lines per commit of today's commits, and ok false
// when no commit of today changed a line.
func (g Git) CommitSize() (float64, bool) {
	if g.CommitsToday == 0 || g.TodayAdded+g.TodayDeleted == 0 {
		return 0, false
	}
	return float64(g.TodayAdded+g.TodayDeleted) / float64(g.CommitsToday), true
}

// AddDeleteRatio returns the lines added per line deleted today. ok is false
// when no line was deleted.
func (g Git) AddDeleteRatio() (float64, bool) {
	if g.TodayDeleted == 0 {
		return 0, false
	}
	return float64(g.TodayAdded) / float64(g.TodayDeleted), true
}

// Streaks returns the run of days with commits that ends today (or yesterday,
// when today has none yet) and the longest run, from the days that had
// commits, newest or oldest first.
func Streaks(days []string, today time.Time) (current, longest int) {
	set := map[string]bool{}
	for _, d := range days {
		set[d] = true
	}
	const layout = "2006-01-02"
	day := today
	if !set[day.Format(layout)] {
		day = day.AddDate(0, 0, -1)
	}
	for set[day.Format(layout)] {
		current++
		day = day.AddDate(0, 0, -1)
	}
	sorted := slices.Sorted(maps.Keys(set))
	run := 0
	var prev time.Time
	for _, d := range sorted {
		at, err := time.Parse(layout, d)
		if err != nil {
			continue
		}
		if !prev.IsZero() && at.Sub(prev) == 24*time.Hour {
			run++
		} else {
			run = 1
		}
		longest = max(longest, run)
		prev = at
	}
	return current, longest
}

// conventionalSubject matches a subject that starts with a Conventional
// Commits type: feat, fix, docs and the others, with an optional scope.
var conventionalSubject = regexp.MustCompile(`^(?:feat|fix|docs|style|refactor|perf|test|chore|build|ci|revert)(?:\([^)]*\))?!?: \S`)

// revertSubject matches the subject of a commit that reverts another.
var revertSubject = regexp.MustCompile(`(?i)^(?:Revert "|revert(?:\([^)]*\))?!?:)`)

// IsRevertCommit reports whether a commit's subject says it reverts another:
// work that came back, the books' returned goods.
func IsRevertCommit(subject string) bool { return revertSubject.MatchString(subject) }

// IsConventionalCommit reports whether a subject follows Conventional Commits.
func IsConventionalCommit(subject string) bool { return conventionalSubject.MatchString(subject) }

// ReviewQueue is the pull requests that wait for the user's review.
type ReviewQueue struct {
	Count int `json:"count"`
	// Oldest is when the oldest of them was opened.
	Oldest time.Time `json:"oldest,omitzero"`
}

// History is what the commits of a branch say, as opposed to the working
// tree: today's commits, the branch's age, what waits to be pushed, the merged
// branches left behind and the user's run of days with commits. It changes
// only when commits are made or fetched, so it is read less often than the
// working tree.
type History struct {
	CommitsToday      int `json:"commits_today,omitzero"`
	FixesToday        int `json:"fixes_today,omitzero"`
	AICommitsToday    int `json:"ai_commits_today,omitzero"`
	VagueToday        int `json:"vague_today,omitzero"`
	ConventionalToday int `json:"conventional_today,omitzero"`
	TodayAdded        int `json:"today_added,omitzero"`
	TodayDeleted      int `json:"today_deleted,omitzero"`
	LargestToday      int `json:"largest_today,omitzero"`

	DefaultBranch  string    `json:"default_branch,omitzero"`
	BranchStart    time.Time `json:"branch_start,omitzero"`
	OldestUnpushed time.Time `json:"oldest_unpushed,omitzero"`
	BehindDefault  int       `json:"behind_default,omitzero"`
	MergedBranches int       `json:"merged_branches,omitzero"`
	Streak         int       `json:"streak,omitzero"`
	LongestStreak  int       `json:"longest_streak,omitzero"`

	RevertsToday  int       `json:"reverts_today,omitzero"`
	OldBranches   int       `json:"old_branches,omitzero"`
	SwitchesToday int       `json:"switches_today,omitzero"`
	FetchedAt     time.Time `json:"fetched_at,omitzero"`
}

// Apply copies the history into the state of the working tree.
func (h History) Apply(g *Git) {
	g.CommitsToday, g.FixesToday, g.AICommitsToday = h.CommitsToday, h.FixesToday, h.AICommitsToday
	g.VagueToday, g.ConventionalToday = h.VagueToday, h.ConventionalToday
	g.TodayAdded, g.TodayDeleted, g.LargestToday = h.TodayAdded, h.TodayDeleted, h.LargestToday
	g.DefaultBranch, g.BranchStart, g.OldestUnpushed = h.DefaultBranch, h.BranchStart, h.OldestUnpushed
	g.BehindDefault, g.MergedBranches = h.BehindDefault, h.MergedBranches
	g.Streak, g.LongestStreak = h.Streak, h.LongestStreak
	g.RevertsToday, g.OldBranches, g.SwitchesToday, g.FetchedAt = h.RevertsToday, h.OldBranches, h.SwitchesToday, h.FetchedAt
}
