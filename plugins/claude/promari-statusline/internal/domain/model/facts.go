package model

import "time"

// Facts is everything the status line learns from outside the session's own
// report. Each fact is optional: a source that is missing, slow or broken
// leaves its fact absent and its chips disappear; it never breaks the line.
type Facts struct {
	Git        Optional[Git]
	Pull       Optional[PullRequest]
	Reviews    Optional[ReviewQueue]
	Spend      Optional[Spend]
	Codex      Optional[CodexLimits]
	Transcript Optional[Transcript]
	Todos      Optional[Todos]
	Track      Optional[Track]
	Incident   Optional[Incident]
	// Latest is the newest released version of Claude Code.
	Latest  Optional[string]
	Account Optional[string]
	Machine Machine
}

// Git is the state of the working tree.
type Git struct {
	Branch  string
	Changed int
	Ahead   int
	Behind  int
	Stashes int
	// Staged, Untracked and Conflicts break Changed down: files staged for the
	// next commit, files git does not track, and files with merge conflicts.
	Staged    int
	Untracked int
	Conflicts int
	// Inserted and Deleted are the lines changed against HEAD, staged or not.
	Inserted int
	Deleted  int
	// Operation is the operation in progress (rebase, merge, cherry-pick,
	// revert, bisect), or "".
	Operation string
	// CommitsToday are the commits on HEAD made since midnight; FixesToday
	// those that fix or revert.
	CommitsToday int
	FixesToday   int
	// AICommitsToday are the commits of today that an AI co-authored.
	AICommitsToday int
	// Changes are the files changed against HEAD with their lines, and
	// DebtAdded the lines added to source files that mark a debt (TODO, FIXME,
	// HACK, XXX), DebtRemoved those deleted.
	Changes     []FileChange
	DebtAdded   int
	DebtRemoved int
	// MocksAdded are the lines added to test files that make a test double;
	// DepsAdded the lines added to manifests that declare a dependency.
	MocksAdded int
	DepsAdded  int
	// SkipsAdded are the lines added to test files that skip a test;
	// AssertsAdded and AssertsRemoved the assertions added and removed.
	SkipsAdded     int
	AssertsAdded   int
	AssertsRemoved int
	// DefaultBranch is the branch origin's HEAD points at, or "" when unknown.
	DefaultBranch string
	// BranchStart is when the first commit of the branch that the default
	// branch does not have was made, and OldestUnpushed when the oldest commit
	// not pushed yet was made; zero when there is none.
	BranchStart    time.Time
	OldestUnpushed time.Time
	// TodayAdded and TodayDeleted are the lines today's commits changed, and
	// VagueToday the commits whose subject says nothing about the change.
	TodayAdded   int
	TodayDeleted int
	VagueToday   int
	// LargestToday is the most lines one commit of today changed, and
	// ConventionalToday the commits of today whose subject starts with a
	// Conventional Commits type.
	LargestToday      int
	ConventionalToday int
	// BehindDefault are the commits of the default branch the branch does not
	// have yet.
	BehindDefault int
	// ConflictMarkers are conflict markers added to files; Junk are changed or
	// untracked paths that do not belong in a repository, JunkStaged those of
	// them staged.
	ConflictMarkers int
	Junk            int
	JunkStaged      int
	// Rules are the coding rules of a training course that the added lines of
	// the working tree break.
	Rules Violations
	// RulesUnchecked are the added lines past model.MaxLintedLines, left
	// unchecked.
	RulesUnchecked int
	// MergedBranches are the local branches merged into the default branch and
	// not deleted.
	MergedBranches int
	// Streak is the run of days with the user's commits up to today, and
	// LongestStreak the longest in the last 60 days.
	Streak        int
	LongestStreak int
	// LastCommit is zero when the branch has no commit.
	LastCommit time.Time
}

// ReviewDecision is the review state of a pull request, as GitHub names it.
type ReviewDecision string

// The review decisions that are shown.
const (
	ReviewApproved         ReviewDecision = "APPROVED"
	ReviewChangesRequested ReviewDecision = "CHANGES_REQUESTED"
	ReviewRequired         ReviewDecision = "REVIEW_REQUIRED"
)

// PullRequest is the pull request of the current branch and its checks.
type PullRequest struct {
	Number  int            `json:"number"`
	Passed  int            `json:"passed"`
	Failed  int            `json:"failed"`
	Pending int            `json:"pending"`
	Review  ReviewDecision `json:"review,omitzero"`
	// Additions, Deletions and Files are the size of the change.
	Additions int `json:"additions,omitzero"`
	Deletions int `json:"deletions,omitzero"`
	Files     int `json:"files,omitzero"`
	// Created is zero when unknown.
	Created   time.Time `json:"created,omitzero"`
	Draft     bool      `json:"draft,omitzero"`
	Conflicts bool      `json:"conflicts,omitzero"`
	// Assignees, Reviewers and Reviews are the people assigned, the reviews
	// requested and the reviews given.
	Assignees int `json:"assignees,omitzero"`
	Reviewers int `json:"reviewers,omitzero"`
	Reviews   int `json:"reviews,omitzero"`
	// PeopleKnown is true when the three above were read: a pull request read
	// by an older version, or reported by Claude Code, knows none of them, and
	// unknown is not none.
	PeopleKnown bool `json:"people_known,omitzero"`
}

// Size returns the lines the pull request changes.
func (p PullRequest) Size() int { return p.Additions + p.Deletions }

// Amount is a sum of money as its source printed it, and its value.
type Amount struct {
	Text  string
	Value float64
}

// Spend is the estimated spending across sessions (from ccusage). It is an
// estimate priced at API rates, unlike the session cost Claude Code reports.
type Spend struct {
	Today Optional[Amount]
	Block Optional[Amount]
	// BlockLeft is the time left in the billing block, as printed and as a
	// duration.
	BlockLeftText string
	BlockLeft     time.Duration
	BurnPerHour   Optional[Amount]
}

// EstimatedBlock projects the block's total at the current burn rate. ok is
// false unless the block, its remaining time and the burn rate are all known.
func (s Spend) EstimatedBlock() (float64, bool) {
	block, hasBlock := s.Block.Get()
	burn, hasBurn := s.BurnPerHour.Get()
	if !hasBlock || !hasBurn || burn.Value == 0 || s.BlockLeft <= 0 {
		return 0, false
	}
	return block.Value + burn.Value*s.BlockLeft.Hours(), true
}

// CodexLimits are the usage windows Codex last reported.
type CodexLimits struct {
	Primary   Optional[CodexWindow] `json:"primary,omitzero"`
	Secondary Optional[CodexWindow] `json:"secondary,omitzero"`
	Balance   Optional[float64]     `json:"balance,omitzero"`
	// SeenAt is when Codex wrote the limits.
	SeenAt time.Time `json:"seen_at"`
}

// CodexWindow is one Codex usage window.
type CodexWindow struct {
	UsedPct       float64   `json:"used_pct"`
	WindowMinutes float64   `json:"window_minutes"`
	ResetsAt      time.Time `json:"resets_at,omitzero"`
}

// ToolStats counts the tool calls of the session.
type ToolStats struct {
	Total  int         `json:"total"`
	Errors int         `json:"errors"`
	Top    []ToolCount `json:"top"`
}

// ToolCount is how often one tool was called.
type ToolCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ErrorRate returns the share of tool calls that failed, in percent. ok is
// false when no tool was called: zero calls is not a zero error rate.
func (t ToolStats) ErrorRate() (float64, bool) {
	if t.Total == 0 {
		return 0, false
	}
	return float64(t.Errors) / float64(t.Total) * percent, true
}

// Todos is the progress of the session's to-do list.
type Todos struct {
	Done  int
	Total int
	// Doing is the item in progress, or "".
	Doing string
}

// Track is the song that is playing.
type Track struct {
	Title  string `json:"title"`
	Artist string `json:"artist,omitzero"`
	Paused bool   `json:"paused,omitzero"`
}

// Incident is an ongoing problem on the API's status page.
type Incident struct {
	Indicator   string `json:"indicator"`
	Description string `json:"description,omitzero"`
}

// Severe reports whether the incident is a major outage.
func (i Incident) Severe() bool { return i.Indicator == "major" || i.Indicator == "critical" }

// Machine is the state of the computer the session runs on. Every reading is
// optional; a platform without the tool to take it leaves it absent.
type Machine struct {
	TerminalStart Optional[time.Time]
	// Load is the one-minute load average.
	Load       Optional[float64]
	CPUs       int
	FreeMemory Optional[float64] // bytes
	FreeDisk   Optional[float64] // bytes
	Battery    Optional[Battery]
	// Sessions is the number of running Claude Code processes.
	Sessions int
}

// Battery is the charge of the battery.
type Battery struct {
	Percent int
	// OnPower is true while the machine is plugged in.
	OnPower bool
}

// LowBatteryPct is the charge below which a battery is nearly empty.
const LowBatteryPct = 20

// Low reports whether the battery is nearly empty and not charging.
func (b Battery) Low() bool { return b.Percent < LowBatteryPct && !b.OnPower }
