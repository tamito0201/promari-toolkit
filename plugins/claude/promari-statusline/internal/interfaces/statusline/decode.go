// Package statusline is the entry point Claude Code talks to: it decodes the
// session report on standard input, asks the use case for the lines, and
// writes them with ANSI colours.
package statusline

import (
	"strings"
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
		ModelID:        jsonx.Or[string](jsonx.Child(in, "model"), "id"),
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
		SpendUSD:       spendMoney(jsonx.Child(jsonx.Child(in, "rate_limits"), "spend_limit")),
		ProjectDir:     jsonx.Or[string](workspace, "project_dir"),
		AddedDirs:      len(jsonx.Or[[]string](workspace, "added_dirs")),
		GitWorktree:    jsonx.Or[string](workspace, "git_worktree"),
		Worktree:       worktree(jsonx.Child(in, "worktree")),
		Vim:            jsonx.Or[string](jsonx.Child(in, "vim"), "mode"),
		Agent:          jsonx.Or[string](jsonx.Child(in, "agent"), "name"),
		PR:             pullRequest(jsonx.Child(in, "pr")),
	}
}

// spendMoney reads the dollars of a spend limit, which arrive apart from its
// percentage and may stay absent.
func spendMoney(obj jsonx.Object) model.Optional[model.SpendMoney] {
	money := model.SpendMoney{Used: optional(obj, "used_usd"), Limit: optional(obj, "limit_usd"), Period: jsonx.Or[string](obj, "period")}
	if !money.Used.Present() && !money.Limit.Present() {
		return model.Optional[model.SpendMoney]{}
	}
	return model.Some(money)
}

// worktree reads the worktree session; one without a name is none.
func worktree(obj jsonx.Object) model.Optional[model.Worktree] {
	name := jsonx.Or[string](obj, "name")
	if name == "" {
		return model.Optional[model.Worktree]{}
	}
	return model.Some(model.Worktree{
		Name:           name,
		Branch:         jsonx.Or[string](obj, "branch"),
		OriginalBranch: jsonx.Or[string](obj, "original_branch"),
	})
}

// pullRequest reads the pull request Claude Code found; one without a number
// is none.
func pullRequest(obj jsonx.Object) model.Optional[model.SessionPR] {
	number := int(jsonx.Or[float64](obj, "number"))
	if number <= 0 {
		return model.Optional[model.SessionPR]{}
	}
	return model.Some(model.SessionPR{
		Number:       number,
		ReviewState:  jsonx.Or[string](obj, "review_state"),
		MergeRequest: jsonx.Or[string](obj, "kind") == "mr",
	})
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
	fresh := jsonx.Or[float64](current, "input_tokens")
	written := jsonx.Or[float64](current, "cache_creation_input_tokens")
	read := jsonx.Or[float64](current, "cache_read_input_tokens")
	return model.ContextWindow{
		Size:        jsonx.Or[float64](obj, "context_window_size"),
		UsedPct:     optional(obj, "used_percentage"),
		Current:     fresh + written + read,
		Fresh:       fresh,
		Written:     written,
		Read:        read,
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
	c := model.PromptCache{
		HitRatio:      optional(obj, "hit_ratio"),
		Misses:        int(jsonx.Or[float64](obj, "misses")),
		LastMissCause: missCause(obj),
		ExpiresAt:     unix(obj, "expires_at"),
		RecacheTokens: jsonx.Or[float64](obj, "recache_tokens_if_cold"),
		TTL:           jsonx.Or[string](obj, "ttl"),
		Requests:      int(jsonx.Or[float64](obj, "requests")),
		Rebuilds:      int(jsonx.Or[float64](obj, "expected_rebuilds")),
		WriteTokens:   jsonx.Or[float64](obj, "cache_write_tokens"),
		MissTokens:    jsonx.Or[float64](obj, "miss_recache_tokens"),
	}
	if warm, ok := jsonx.Get[bool](obj, "warm"); ok {
		c.Warm = model.Some(warm)
	}
	if observed, ok := jsonx.Get[bool](obj, "caching_observed"); ok {
		c.Observed = model.Some(observed)
	}
	c.LastMissAt = unix(obj, "last_miss_at")
	if causes := jsonx.Or[map[string]float64](obj, "miss_causes"); len(causes) > 0 {
		c.MissCauses = make(map[string]int, len(causes))
		for cause, n := range causes {
			c.MissCauses[cause] = int(n)
		}
	}
	return c
}

// missCause names the causes of the last miss. Claude Code reports them as an
// object with a list of causes; a version that reported a plain string is read
// as it was.
func missCause(obj jsonx.Object) string {
	if cause, ok := jsonx.Get[string](obj, "last_miss_cause"); ok {
		return cause
	}
	return strings.Join(jsonx.Or[[]string](jsonx.Child(obj, "last_miss_cause"), "causes"), "+")
}
