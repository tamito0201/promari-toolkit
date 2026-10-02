// Package statusline is the entry point Claude Code talks to: it decodes the
// session report on standard input, asks the use case for the lines, and
// writes them with ANSI colours.
package statusline

import (
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/pkg/jsonx"
)

// Decode turns the JSON Claude Code writes to the status line into a Session.
//
// The format belongs to Claude Code and grows with its versions, so nothing
// is required and every member is read on its own: one that is missing or has
// changed its type is left out, and the rest of the status line still shows.
// An empty object, or input that is not an object, is a session that reported
// nothing.
func Decode(raw []byte) model.Session {
	in, ok := jsonx.Parse(raw)
	if !ok || len(in) == 0 {
		return model.Session{}
	}
	workspace := jsonx.Child(in, "workspace")
	dir := jsonx.Or[string](workspace, "current_dir")
	if dir == "" {
		dir = jsonx.Or[string](in, "cwd")
	}
	return model.Session{
		Reported:       true,
		ID:             jsonx.Or[string](in, "session_id"),
		PromptID:       jsonx.Or[string](in, "prompt_id"),
		Name:           jsonx.Or[string](in, "session_name"),
		TranscriptPath: jsonx.Or[string](in, "transcript_path"),
		Dir:            dir,
		Repo:           jsonx.Or[string](jsonx.Child(workspace, "repo"), "name"),
		Version:        jsonx.Or[string](in, "version"),
		Model:          jsonx.Or[string](jsonx.Child(in, "model"), "display_name"),
		Effort:         jsonx.Or[string](jsonx.Child(in, "effort"), "level"),
		Thinking:       jsonx.Or[bool](jsonx.Child(in, "thinking"), "enabled"),
		Fast:           jsonx.Or[bool](in, "fast_mode"),
		Style:          jsonx.Or[string](jsonx.Child(in, "output_style"), "name"),
		Cost:           cost(jsonx.Child(in, "cost")),
		Context:        contextWindow(jsonx.Child(in, "context_window")),
		Limits:         limits(jsonx.Child(in, "rate_limits")),
		Cache:          cache(jsonx.Child(in, "prompt_cache")),
		Over200k:       jsonx.Or[bool](in, "exceeds_200k_tokens"),
	}
}

// optional reads a number that may be absent; zero is a value.
func optional(obj jsonx.Object, name string) model.Optional[float64] {
	if v, ok := jsonx.Get[float64](obj, name); ok {
		return model.Some(v)
	}
	return model.Optional[float64]{}
}

func milliseconds(obj jsonx.Object, name string) time.Duration {
	return time.Duration(jsonx.Or[float64](obj, name) * float64(time.Millisecond))
}

// unix reads a Unix time in seconds; a missing or zero member is no time.
func unix(obj jsonx.Object, name string) time.Time {
	seconds := jsonx.Or[float64](obj, name)
	if seconds <= 0 {
		return time.Time{}
	}
	return time.Unix(0, int64(seconds*float64(time.Second)))
}

func cost(obj jsonx.Object) model.Cost {
	return model.Cost{
		TotalUSD:     optional(obj, "total_cost_usd"),
		Wall:         milliseconds(obj, "total_duration_ms"),
		API:          milliseconds(obj, "total_api_duration_ms"),
		LinesAdded:   int(jsonx.Or[float64](obj, "total_lines_added")),
		LinesRemoved: int(jsonx.Or[float64](obj, "total_lines_removed")),
	}
}

func contextWindow(obj jsonx.Object) model.ContextWindow {
	current := jsonx.Child(obj, "current_usage")
	return model.ContextWindow{
		Size:    jsonx.Or[float64](obj, "context_window_size"),
		UsedPct: optional(obj, "used_percentage"),
		Current: jsonx.Or[float64](current, "input_tokens") +
			jsonx.Or[float64](current, "cache_creation_input_tokens") +
			jsonx.Or[float64](current, "cache_read_input_tokens"),
		TotalInput:  jsonx.Or[float64](obj, "total_input_tokens"),
		TotalOutput: jsonx.Or[float64](obj, "total_output_tokens"),
	}
}

func limits(obj jsonx.Object) model.RateLimits {
	return model.RateLimits{
		FiveHour: window(jsonx.Child(obj, "five_hour")),
		SevenDay: window(jsonx.Child(obj, "seven_day")),
		Spend:    window(jsonx.Child(obj, "spend_limit")),
	}
}

// window reads one rate window; one without a percentage is not a window.
func window(obj jsonx.Object) model.Optional[model.RateWindow] {
	used, ok := jsonx.Get[float64](obj, "used_percentage")
	if !ok {
		return model.Optional[model.RateWindow]{}
	}
	return model.Some(model.RateWindow{UsedPct: used, ResetsAt: unix(obj, "resets_at")})
}

func cache(obj jsonx.Object) model.PromptCache {
	return model.PromptCache{
		HitRatio:      optional(obj, "hit_ratio"),
		Misses:        int(jsonx.Or[float64](obj, "misses")),
		LastMissCause: jsonx.Or[string](obj, "last_miss_cause"),
		ExpiresAt:     unix(obj, "expires_at"),
		RecacheTokens: jsonx.Or[float64](obj, "recache_tokens_if_cold"),
	}
}
