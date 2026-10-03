package model

import "time"

// Facts is everything the status line learns from outside the session's own
// report. Each fact is optional: a source that is missing, slow or broken
// leaves its fact absent and its chips disappear; it never breaks the line.
type Facts struct {
	Git      Optional[Git]
	Pull     Optional[PullRequest]
	Spend    Optional[Spend]
	Codex    Optional[CodexLimits]
	Tools    Optional[ToolStats]
	Todos    Optional[Todos]
	Track    Optional[Track]
	Incident Optional[Incident]
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
}

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
