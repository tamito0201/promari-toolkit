package model

import (
	"iter"
	"math"
	"regexp"
	"time"
)

const percent = 100.0

// Session is what Claude Code reports about the running session on one render.
// Every part is optional: the first render carries only a few fields, and the
// rate limits appear after the first response.
type Session struct {
	// Reported is false when Claude Code sent nothing usable.
	Reported bool

	ID             string
	PromptID       string
	Name           string
	TranscriptPath string
	Dir            string
	Repo           string
	Version        string

	Model    string
	Effort   string
	Thinking bool
	Fast     bool
	Style    string

	Cost     Cost
	Context  ContextWindow
	Limits   RateLimits
	Cache    PromptCache
	Over200k bool
}

// fileSafe matches an id that can be part of a file name: no path separator,
// no glob metacharacter, no leading dot.
var fileSafe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)

// Key returns the session id when it can name a file. The id arrives from
// outside the process and ends up in a path, so anything else is refused.
func (s *Session) Key() (string, bool) {
	return s.ID, fileSafe.MatchString(s.ID)
}

// WorkDir returns the directory the session works in.
func (s *Session) WorkDir() string {
	if s.Dir == "" {
		return "."
	}
	return s.Dir
}

// Cost is what the session has cost and produced so far.
type Cost struct {
	TotalUSD     Optional[float64]
	Wall         time.Duration
	API          time.Duration
	LinesAdded   int
	LinesRemoved int
}

// ContextWindow is the model's context as Claude Code reports it.
type ContextWindow struct {
	Size    float64
	UsedPct Optional[float64]
	// Current is the tokens in the window now (input, cache writes, cache reads).
	Current     float64
	TotalInput  float64
	TotalOutput float64
}

// ContextUsage is how full the context window is.
type ContextUsage struct {
	Pct    float64
	Used   float64
	Remain float64
	Size   float64
}

// Usage returns how full the window is; ok is false until Claude Code reports
// both its size and the used percentage.
func (c ContextWindow) Usage() (ContextUsage, bool) {
	pct, ok := c.UsedPct.Get()
	if !ok || c.Size == 0 {
		return ContextUsage{}, false
	}
	used := c.Current
	if used == 0 {
		used = math.RoundToEven(c.Size * pct / percent)
	}
	return ContextUsage{Pct: pct, Used: used, Remain: max(0, c.Size-used), Size: c.Size}, true
}

// RateLimits are the subscription's usage windows.
type RateLimits struct {
	FiveHour Optional[RateWindow] `json:"five_hour,omitzero"`
	SevenDay Optional[RateWindow] `json:"seven_day,omitzero"`
	Spend    Optional[RateWindow] `json:"spend_limit,omitzero"`
}

// RateWindow is one usage window.
type RateWindow struct {
	UsedPct float64 `json:"used_pct"`
	// ResetsAt is zero when Claude Code did not say when the window resets.
	ResetsAt time.Time `json:"resets_at,omitzero"`
}

// The lengths of the rolling windows.
const (
	FiveHours = 5 * time.Hour
	SevenDays = 7 * 24 * time.Hour
)

// LabelledWindow is a window with its label and length. Length is zero for a
// window that does not roll (the spend limit).
type LabelledWindow struct {
	Label  string
	Length time.Duration
	Window RateWindow
}

// Empty reports whether no window is known.
func (l RateLimits) Empty() bool {
	return !l.FiveHour.Present() && !l.SevenDay.Present() && !l.Spend.Present()
}

// All yields the known windows in display order.
func (l RateLimits) All() iter.Seq[LabelledWindow] {
	return func(yield func(LabelledWindow) bool) {
		for _, c := range []struct {
			label  string
			length time.Duration
			window Optional[RateWindow]
		}{{"5h", FiveHours, l.FiveHour}, {"7d", SevenDays, l.SevenDay}, {"Spend", 0, l.Spend}} {
			if w, ok := c.window.Get(); ok && !yield(LabelledWindow{Label: c.label, Length: c.length, Window: w}) {
				return
			}
		}
	}
}

const (
	// paceMinElapsedPct keeps the pace quiet at the start of a window, where a
	// single request makes it jump.
	paceMinElapsedPct = 5.0
)

// Pace returns how far usage runs ahead of the window's elapsed time: 2 means
// twice the share of the window is used than has passed. ok is false when the
// window does not roll, has no reset time, has barely started or is unused.
func (w RateWindow) Pace(now time.Time, length time.Duration) (pace float64, ok bool) {
	if length <= 0 || w.ResetsAt.IsZero() {
		return 0, false
	}
	left := w.ResetsAt.Sub(now)
	if left <= 0 || left >= length {
		return 0, false
	}
	elapsedPct := float64(length-left) / float64(length) * percent
	if elapsedPct < paceMinElapsedPct || w.UsedPct <= 0 {
		return 0, false
	}
	return w.UsedPct / elapsedPct, true
}

// PromptCache is the state of the prompt cache.
type PromptCache struct {
	HitRatio      Optional[float64]
	Misses        int
	LastMissCause string
	// ExpiresAt is zero when unknown.
	ExpiresAt     time.Time
	RecacheTokens float64
}
