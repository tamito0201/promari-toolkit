package model

import (
	"math"
	"slices"
	"time"
)

const (
	// MaxTurnSamples bounds the turn durations kept: enough for stable
	// percentiles, small enough to store on every read.
	MaxTurnSamples = 200
	// MaxFilesKept bounds the files remembered as edited.
	MaxFilesKept = 1000
	// p90 is the percentile shown as the slow end of the turns.
	p90 = 0.9
	// median is the percentile shown as the typical turn.
	median = 0.5
)

// Transcript is what a session's transcript records: every request to the
// model with its tokens, every tool call, the human prompts and what happened
// to them. Claude Code reports most of the session on standard input, but only
// for the moment; the transcript holds the whole session.
type Transcript struct {
	// Tools summarises ToolCounts: the total, the failures and the most called.
	Tools ToolStats `json:"tools"`
	// ToolCounts are every tool called, in the order first called.
	ToolCounts []ToolCount `json:"tool_counts,omitzero"`
	// Tokens are summed over every request of the session, the main
	// conversation's and its subagents'.
	Tokens TokenTotals `json:"tokens"`
	// Requests is the number of responses of the model; Side counts those of
	// subagents.
	Requests     int `json:"requests"`
	SideRequests int `json:"side_requests,omitzero"`
	// Models are the models that answered, with how many responses each.
	Models map[string]int `json:"models,omitzero"`
	// Prompts are the prompts a human typed, not the tool results and reminders
	// that also arrive as user messages.
	Prompts int `json:"prompts"`
	// Interrupts are responses the human stopped; Denials are tool calls the
	// human or a permission rule refused.
	Interrupts int `json:"interrupts,omitzero"`
	Denials    int `json:"denials,omitzero"`
	// Refusals and Truncated are responses that stopped on a refusal or on the
	// output limit.
	Refusals  int `json:"refusals,omitzero"`
	Truncated int `json:"truncated,omitzero"`
	// WebSearches and WebFetches are the server tools the model used.
	WebSearches int `json:"web_searches,omitzero"`
	WebFetches  int `json:"web_fetches,omitzero"`
	// ThinkingSeconds is the time the model spent thinking.
	ThinkingSeconds float64 `json:"thinking_seconds,omitzero"`
	// Turns are the durations of the last turns, in seconds, oldest first: the
	// time from a prompt to the end of its answer, as Claude Code measured it.
	Turns []float64 `json:"turns,omitzero"`
	// Compactions are the compactions of the context; LastCompaction is zero
	// before the first.
	Compactions    int       `json:"compactions,omitzero"`
	LastCompaction time.Time `json:"last_compaction,omitzero"`
	// Files are the files the session edited or wrote.
	Files []string `json:"files,omitzero"`
	// PermissionMode is the last permission mode the session switched to
	// (default, acceptEdits, plan, auto, bypassPermissions), or "".
	PermissionMode string `json:"permission_mode,omitzero"`
	// Hooks are the hooks that ran; HookErrors those that failed or were
	// cancelled.
	Hooks      int `json:"hooks,omitzero"`
	HookErrors int `json:"hook_errors,omitzero"`
	// Queued are the prompts typed while the agent was still working.
	Queued int `json:"queued,omitzero"`
	// Diagnostics are the batches of editor diagnostics (errors, warnings)
	// handed to the model.
	Diagnostics int `json:"diagnostics,omitzero"`
	// Quality is what the tool calls say about the quality of the work.
	Quality Quality `json:"quality,omitzero"`
	// Trace is how the agent works: exploring, editing, reading again.
	Trace Trace `json:"trace,omitzero"`
	// Started is when the transcript's first entry was written.
	Started time.Time `json:"started,omitzero"`
	// Cursor is where the next read continues.
	Cursor TranscriptCursor `json:"cursor"`
}

// TranscriptCursor is how far a transcript has been read. A transcript only
// grows, so the next read starts at Offset; a shorter file is a new one.
type TranscriptCursor struct {
	Offset int64 `json:"offset"`
	// Format is the TranscriptFormat the reading was made with. A reading made
	// with another is read again from the start: continuing it would leave what
	// the new format counts uncounted for every line already read.
	Format int `json:"format,omitzero"`
	// Recent are the ids of the last responses counted, newest last. Claude
	// Code writes a response as several entries, one per block, each repeating
	// its usage, and the entries of two responses can interleave.
	Recent []string `json:"recent,omitzero"`
}

// TranscriptFormat is the version of what a reading of a transcript counts. It
// goes up whenever a reading counts something new (2: the quality of the work; 3: the trace and the claims).
const TranscriptFormat = 3

// recentKept is how many response ids the cursor remembers.
const recentKept = 16

// Counted reports whether the response with the given id was counted, and
// remembers it as counted when it was not. An empty id is never counted twice
// because it is never remembered: each entry without one is a response.
func (c *TranscriptCursor) Counted(id string) bool {
	if id == "" {
		return false
	}
	if slices.Contains(c.Recent, id) {
		return true
	}
	c.Recent = append(c.Recent, id)
	if n := len(c.Recent); n > recentKept {
		c.Recent = slices.Clone(c.Recent[n-recentKept:])
	}
	return false
}

// TokenTotals are tokens summed over requests.
type TokenTotals struct {
	Input      float64 `json:"input"`
	CacheWrite float64 `json:"cache_write"`
	// CacheWrite1h is the part of CacheWrite written with the one-hour lifetime.
	CacheWrite1h float64 `json:"cache_write_1h,omitzero"`
	CacheRead    float64 `json:"cache_read"`
	Output       float64 `json:"output"`
	// Thinking is the part of Output the model spent thinking.
	Thinking float64 `json:"thinking,omitzero"`
}

// AllInput returns every input token: fresh, written to the cache and read
// from it.
func (t TokenTotals) AllInput() float64 { return t.Input + t.CacheWrite + t.CacheRead }

// CachedShare returns the share of the input read from the cache, in percent.
// ok is false before any input.
func (t TokenTotals) CachedShare() (float64, bool) {
	all := t.AllInput()
	if all == 0 {
		return 0, false
	}
	return t.CacheRead / all * percent, true
}

// ThinkingShare returns the share of the output spent thinking, in percent.
// ok is false before any output.
func (t TokenTotals) ThinkingShare() (float64, bool) {
	if t.Output == 0 {
		return 0, false
	}
	return t.Thinking / t.Output * percent, true
}

// CountTools adds tool calls, in the order first called, and failed results,
// and summarises them again in Tools. Equal counts keep the order of the first
// call.
func (t *Transcript) CountTools(calls []ToolCount, failed int) {
	for _, c := range calls {
		if i := slices.IndexFunc(t.ToolCounts, func(k ToolCount) bool { return k.Name == c.Name }); i >= 0 {
			t.ToolCounts[i].Count += c.Count
		} else {
			t.ToolCounts = append(t.ToolCounts, c)
		}
	}
	stats := ToolStats{Errors: t.Tools.Errors + failed}
	for _, c := range t.ToolCounts {
		stats.Total += c.Count
	}
	top := slices.Clone(t.ToolCounts)
	slices.SortStableFunc(top, func(a, b ToolCount) int { return b.Count - a.Count })
	stats.Top = top[:min(len(top), TopTools)]
	t.Tools = stats
}

// TopTools is how many tools the status line names.
const TopTools = 3

// AddTurn records the duration of a turn, keeping the last MaxTurnSamples.
func (t *Transcript) AddTurn(d time.Duration) {
	t.Turns = append(t.Turns, d.Seconds())
	if n := len(t.Turns); n > MaxTurnSamples {
		t.Turns = slices.Clone(t.Turns[n-MaxTurnSamples:])
	}
}

// AddFile records a file the session edited; a file already known, or one
// past MaxFilesKept, is not added again.
func (t *Transcript) AddFile(path string) {
	if path == "" || len(t.Files) >= MaxFilesKept || slices.Contains(t.Files, path) {
		return
	}
	t.Files = append(t.Files, path)
}

// TurnTimes are the durations of the turns: the last one and the median and
// 90th percentile of those kept.
type TurnTimes struct {
	Last, P50, P90 time.Duration
}

// TurnTimes returns the durations of the turns; ok is false before the first
// turn ended.
func (t *Transcript) TurnTimes() (TurnTimes, bool) {
	if len(t.Turns) == 0 {
		return TurnTimes{}, false
	}
	sorted := slices.Sorted(slices.Values(t.Turns))
	return TurnTimes{
		Last: seconds(t.Turns[len(t.Turns)-1]),
		P50:  seconds(quantile(sorted, median)),
		P90:  seconds(quantile(sorted, p90)),
	}, true
}

// quantile returns the q-quantile of sorted values by the nearest-rank method
// (the ceil(q·n)-th value): a value that was observed, never one between two.
func quantile(sorted []float64, q float64) float64 {
	rank := int(math.Ceil(q*float64(len(sorted)))) - 1
	return sorted[min(max(rank, 0), len(sorted)-1)]
}

// Autonomy returns the tool calls per human prompt: how many steps the agent
// takes on its own for each thing it is asked. ok is false before a prompt.
func (t *Transcript) Autonomy() (float64, bool) {
	if t.Prompts == 0 {
		return 0, false
	}
	return float64(t.Tools.Total) / float64(t.Prompts), true
}

// Interventions returns the interrupts and denials per human prompt, in
// percent: how often the human had to step in. ok is false before a prompt.
func (t *Transcript) Interventions() (float64, bool) {
	if t.Prompts == 0 {
		return 0, false
	}
	return float64(t.Interrupts+t.Denials) / float64(t.Prompts) * percent, true
}
