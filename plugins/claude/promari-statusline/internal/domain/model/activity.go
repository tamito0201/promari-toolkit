package model

import (
	"math"
	"time"
)

const (
	// A context that shrinks below compactDrop of the last sample, from above
	// compactMinTokens, was compacted.
	compactDrop      = 0.6
	compactMinTokens = 100_000
	// maxSamples bounds the history the growth rate is measured over.
	maxSamples = 20
	// IdleGap is the longest pause between two renders that still counts as
	// work. The renders themselves are the activity sensor: Claude Code redraws
	// the status line while a session is busy.
	IdleGap = 5 * time.Minute
	// etaMinSpan is the shortest history a growth rate is trusted over.
	etaMinSpan = time.Minute
	// etaMax hides an estimate too far away to act on.
	etaMax = 12 * time.Hour
	// DeepStreak is the shortest streak that counts as deep work: the time it
	// takes, on average, to get back to an interrupted task (23 min 15 s in
	// Mark, Gudith and Klocke, "The Cost of Interrupted Work", CHI 2008). A
	// streak shorter than that ends before the work it resumed was back in
	// full swing.
	DeepStreak = 23 * time.Minute
)

// Activity is what one session remembers between renders: how its context
// grew, how often it was compacted, how many prompts it took, and how long it
// worked and idled. It is an aggregate with the session id as its identity.
type Activity struct {
	Samples     []Sample `json:"samples"`
	Compactions int      `json:"compactions"`
	Turns       int      `json:"turns"`
	LastPrompt  string   `json:"last_prompt,omitzero"`
	// LastSeen is zero until the first render of the session.
	LastSeen      time.Time `json:"last_seen,omitzero"`
	WorkedSeconds float64   `json:"worked_seconds"`
	IdledSeconds  float64   `json:"idled_seconds"`
	StreakStart   time.Time `json:"streak_start,omitzero"`
	// Breaks are the pauses longer than IdleGap.
	Breaks int `json:"breaks,omitzero"`
	// DeepSeconds is the time worked in finished streaks of DeepStreak or
	// longer; LongestSeconds the longest finished streak.
	DeepSeconds    float64 `json:"deep_seconds,omitzero"`
	LongestSeconds float64 `json:"longest_seconds,omitzero"`
}

// Sample is the context size at one moment.
type Sample struct {
	At     time.Time `json:"at"`
	Tokens float64   `json:"tokens"`
}

// Observe records one render: the context size, the prompt being answered and
// the time since the previous render.
func (a *Activity) Observe(used float64, promptID string, now time.Time) {
	a.sample(used, now)
	if promptID != "" && promptID != a.LastPrompt {
		a.LastPrompt = promptID
		a.Turns++
	}
	switch gap := now.Sub(a.LastSeen); {
	case a.LastSeen.IsZero():
		a.StreakStart = now
	case gap > IdleGap:
		a.IdledSeconds += gap.Seconds()
		a.endStreak(a.LastSeen.Sub(a.StreakStart))
		a.Breaks++
		a.StreakStart = now
	default:
		a.WorkedSeconds += max(0, gap.Seconds())
	}
	a.LastSeen = now
}

func (a *Activity) sample(used float64, now time.Time) {
	if n := len(a.Samples); n > 0 && a.Samples[n-1].Tokens > compactMinTokens && used < a.Samples[n-1].Tokens*compactDrop {
		a.Compactions++
		a.Samples = nil // the growth rate is measured anew after a compaction
	}
	if n := len(a.Samples); n == 0 || a.Samples[n-1].Tokens != used {
		a.Samples = append(a.Samples, Sample{At: now, Tokens: used})
	}
	if n := len(a.Samples); n > maxSamples {
		a.Samples = a.Samples[n-maxSamples:]
	}
}

// ETA estimates the time until the context is full, from the growth between
// the oldest and the newest sample. ok is false when the history is too short,
// the context is not growing, or the estimate is too far away to act on.
func (a *Activity) ETA(remain float64) (eta time.Duration, ok bool) {
	if len(a.Samples) < 2 {
		return 0, false
	}
	first, last := a.Samples[0], a.Samples[len(a.Samples)-1]
	elapsed, grown := last.At.Sub(first.At), last.Tokens-first.Tokens
	if elapsed < etaMinSpan || grown <= 0 {
		return 0, false
	}
	// Compared in seconds first: a context that barely grows gives an estimate
	// too large for a Duration.
	left := remain / (grown / elapsed.Seconds())
	if left >= etaMax.Seconds() {
		return 0, false
	}
	return seconds(left), true
}

// endStreak records a finished streak of the given length.
func (a *Activity) endStreak(length time.Duration) {
	a.LongestSeconds = max(a.LongestSeconds, length.Seconds())
	if length >= DeepStreak {
		a.DeepSeconds += length.Seconds()
	}
}

// Deep returns the time worked in streaks of DeepStreak or longer, the
// current streak included once it is that long.
func (a *Activity) Deep(now time.Time) time.Duration {
	deep := seconds(a.DeepSeconds)
	if streak := a.Streak(now); streak >= DeepStreak {
		deep += streak
	}
	return deep
}

// Longest returns the longest streak, the current one included.
func (a *Activity) Longest(now time.Time) time.Duration {
	return max(seconds(a.LongestSeconds), a.Streak(now))
}

// Worked returns the time the session spent working.
func (a *Activity) Worked() time.Duration { return seconds(a.WorkedSeconds) }

// Idled returns the time the session spent idle.
func (a *Activity) Idled() time.Duration { return seconds(a.IdledSeconds) }

// Streak returns the time worked since the session last came back from idle.
func (a *Activity) Streak(now time.Time) time.Duration { return now.Sub(a.StreakStart) }

// seconds converts seconds to a Duration, rounded to the nanosecond: a rate
// computed in floating point is a hair off the value it stands for, and
// truncating 3599.9999999 s would show an hour as 59 minutes.
func seconds(s float64) time.Duration { return time.Duration(math.Round(s * float64(time.Second))) }
