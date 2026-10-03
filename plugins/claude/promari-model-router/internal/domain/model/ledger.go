package model

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// EventKind names a ledger event.
type EventKind string

// Ledger events.
const (
	EventSessionStart   EventKind = "session_start"
	EventModelSwitch    EventKind = "model_switch"
	EventPrompt         EventKind = "prompt"
	EventSubagent       EventKind = "subagent"
	EventSubagentResult EventKind = "subagent_result"
	EventError          EventKind = "error"
)

// Entry is one immutable ledger row. Prompt text is never stored: only a short
// SHA-256 prefix and the character count.
type Entry struct {
	At              time.Time
	Event           EventKind
	SessionID       string
	ToolUseID       string
	SessionModel    string
	SessionSource   SessionSource
	Class           Class
	Confidence      int
	Margin          int
	Danger          bool
	Codex           CodexKind
	Continuation    bool
	Lang            Language
	PromptChars     int
	PromptSHA       string
	Advised         bool
	PressureHigh    bool
	Action          Action
	Reason          string
	Target          Tier
	SubagentType    string
	Requested       string
	Resolved        string
	Mismatch        Mismatch
	Status          SubagentStatus
	TotalTokens     int
	InputTokens     int
	OutputTokens    int
	CacheReadTokens int
	DurationMS      int
	Nested          bool
	Detail          string
}

// Failure is a failure recorded outside the ledger: the hook that could not
// write the ledger, or the launcher that could not start the binary.
type Failure struct {
	At      time.Time
	Message string
}

// PromptDigest is the first 12 hex digits of the prompt's SHA-256.
func PromptDigest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:12]
}

// EntryOption configures an Entry (functional options keep call sites short
// and the struct immutable after construction).
type EntryOption func(*Entry)

// NewEntry builds an entry.
func NewEntry(at time.Time, event EventKind, opts ...EntryOption) Entry {
	e := Entry{At: at, Event: event}
	for _, opt := range opts {
		opt(&e)
	}
	return e
}

// WithSession sets the session and its observed model.
func WithSession(id string, sm SessionModel) EntryOption {
	return func(e *Entry) { e.SessionID, e.SessionModel, e.SessionSource = id, sm.Model, sm.Source }
}

// WithClassification copies the classification fields.
func WithClassification(c Classification) EntryOption {
	return func(e *Entry) {
		e.Class, e.Confidence, e.Margin = c.Class, c.Confidence, c.Margin
		e.Danger, e.Codex, e.Continuation, e.Lang = c.Danger, c.Codex, c.Continuation, c.Lang
		e.PromptChars = c.Chars
	}
}

// WithDecision copies the routing decision.
func WithDecision(d Decision) EntryOption {
	return func(e *Entry) {
		e.Action, e.Reason, e.Target = d.Action, d.Reason, d.Target
		e.SubagentType, e.Requested = d.SubagentType, d.Requested
		if e.Class == ClassNone {
			e.Class = d.Class
		}
	}
}

// WithPrompt records the digest of a prompt.
func WithPrompt(text string) EntryOption {
	return func(e *Entry) { e.PromptSHA = PromptDigest(text) }
}

// WithDetail records free-form detail (a trace, the previous model).
func WithDetail(detail string) EntryOption {
	return func(e *Entry) { e.Detail = detail }
}

// WithToolUse records the Agent call the entry belongs to and whether it was
// started from inside another subagent.
func WithToolUse(toolUseID string, nested bool) EntryOption {
	return func(e *Entry) { e.ToolUseID, e.Nested = toolUseID, nested }
}

// WithAdvice records whether the prompt was advised and whether the plan
// usage was high at the time.
func WithAdvice(advised, pressureHigh bool) EntryOption {
	return func(e *Entry) { e.Advised, e.PressureHigh = advised, pressureHigh }
}

// WithOutcome copies what PostToolUse reported for an Agent call.
func WithOutcome(o SubagentOutcome) EntryOption {
	return func(e *Entry) {
		e.ToolUseID, e.SubagentType, e.Requested, e.Resolved = o.ToolUseID, o.SubagentType, o.Requested, o.Resolved
		e.Mismatch, e.Status, e.TotalTokens = o.Mismatch(), o.Status, o.TotalTokens
		e.InputTokens, e.OutputTokens, e.CacheReadTokens, e.DurationMS = o.InputTokens, o.OutputTokens, o.CacheReadTokens, o.DurationMS
	}
}

// WithFailure records where a hook failed and the (already shortened) detail.
func WithFailure(where, detail string) EntryOption {
	return func(e *Entry) { e.Reason, e.Detail = where, detail }
}

// SubagentStatus is the status PostToolUse reports for an Agent call.
type SubagentStatus string

// Subagent statuses the router interprets; any other value is a failure.
const (
	StatusCompleted     SubagentStatus = "completed"
	StatusAsyncLaunched SubagentStatus = "async_launched"
)

// Succeeded reports whether the subagent finished its work.
func (s SubagentStatus) Succeeded() bool { return s == StatusCompleted }

// Background reports a subagent started in the background: PostToolUse runs
// at launch, so the result carries no usage data yet.
func (s SubagentStatus) Background() bool { return s == StatusAsyncLaunched }

// Mismatch says whether the model that ran differs from the one requested.
// It is a value (never a shared pointer) with an explicit unknown state.
type Mismatch int8

// Mismatch states.
const (
	MismatchUnknown Mismatch = iota // either side is missing
	MismatchNo
	MismatchYes
)

// Known reports whether both sides were observed.
func (m Mismatch) Known() bool { return m != MismatchUnknown }

// Yes reports an observed mismatch.
func (m Mismatch) Yes() bool { return m == MismatchYes }

// SubagentOutcome is what PostToolUse reports for an Agent call.
type SubagentOutcome struct {
	ToolUseID       string
	SubagentType    string
	Requested       string
	Resolved        string
	Status          SubagentStatus
	TotalTokens     int
	InputTokens     int
	OutputTokens    int
	CacheReadTokens int
	DurationMS      int
}

// Mismatch reports whether the model that ran differs from the one requested.
// It is unknown when either side is missing.
func (o SubagentOutcome) Mismatch() Mismatch {
	switch {
	case o.Requested == "" || o.Resolved == "":
		return MismatchUnknown
	case TierOf(o.Requested) != TierOf(o.Resolved):
		return MismatchYes
	default:
		return MismatchNo
	}
}
