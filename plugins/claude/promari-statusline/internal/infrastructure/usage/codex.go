package usage

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform"
	"promari-statusline/pkg/jsonx"
)

// rateLimitsKey is the member Codex writes its usage windows under.
const (
	rateLimitsKey = "rate_limits"
	// 長いログでも返却する履歴を制限する。表示範囲の判定は元の観測時刻を使う。
	codexHistoryMax = 200
)

// Codex reads the usage windows from the newest Codex session log. Codex has
// no command that prints them; it writes them into the log of each session.
type Codex struct {
	Sys platform.System
}

var _ repository.CodexReader = Codex{}

// Codex implements repository.CodexReader.
func (c Codex) Codex(_ context.Context) (model.CodexLimits, error) {
	log, modified, ok := c.newestLog()
	if !ok {
		return model.CodexLimits{}, repository.ErrNone
	}
	data, err := c.Sys.ReadFile(log)
	if err != nil {
		return model.CodexLimits{}, fmt.Errorf("read the codex log: %w", err)
	}
	limits, ok := lastRateLimits(data)
	if !ok {
		return model.CodexLimits{}, repository.ErrNone
	}
	limits.SeenAt = modified
	return limits, nil
}

// newestLog returns the session log that was written last.
func (c Codex) newestLog() (path string, modified time.Time, ok bool) {
	pattern := filepath.Join(c.Sys.HomeDir(), ".codex", "sessions", "*", "*", "*", "*.jsonl")
	for _, candidate := range c.Sys.Glob(pattern) {
		if t, err := c.Sys.ModTime(candidate); err == nil && (!ok || t.After(modified)) {
			path, modified, ok = candidate, t, true
		}
	}
	return path, modified, ok
}

// lastRateLimits returns the usage windows of the last log line that has any.
func lastRateLimits(log []byte) (limits model.CodexLimits, ok bool) {
	marker := []byte(`"` + rateLimitsKey + `"`)
	var history []model.CodexRatePoint
	for line := range bytes.Lines(log) {
		if !bytes.Contains(line, marker) {
			continue
		}
		var entry any
		if err := json.Unmarshal(bytes.TrimSpace(line), &entry); err != nil {
			continue
		}
		if found, has := findRateLimits(entry); has {
			limits, ok = found, true
			if point, measured := codexPoint(entry, found); measured {
				history = append(history, point)
			}
		}
	}
	slices.SortFunc(history, func(a, b model.CodexRatePoint) int { return a.At.Compare(b.At) })
	limits.History = history[max(0, len(history)-codexHistoryMax):]
	return limits, ok
}

// ファイル更新時刻を観測時刻にしない。時刻や時間枠がない行はグラフに載せない。
func codexPoint(entry any, limits model.CodexLimits) (model.CodexRatePoint, bool) {
	obj, _ := entry.(map[string]any)
	stamp, _ := obj["timestamp"].(string)
	at, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil || !limits.Primary.Present() && !limits.Secondary.Present() {
		return model.CodexRatePoint{}, false
	}
	return model.CodexRatePoint{At: at, Primary: limits.Primary, Secondary: limits.Secondary}, true
}

// findRateLimits searches a decoded log line, depth first, for an object with
// a non-empty rate_limits member. Members are visited in the order of their
// names, so the same line always gives the same answer.
func findRateLimits(v any) (model.CodexLimits, bool) {
	switch v := v.(type) {
	case map[string]any:
		if limits, ok := v[rateLimitsKey].(map[string]any); ok && len(limits) > 0 {
			return toLimits(limits), true
		}
		for _, name := range slices.Sorted(maps.Keys(v)) {
			if limits, ok := findRateLimits(v[name]); ok {
				return limits, true
			}
		}
	case []any:
		for _, item := range v {
			if limits, ok := findRateLimits(item); ok {
				return limits, true
			}
		}
	}
	return model.CodexLimits{}, false
}

func toLimits(raw map[string]any) model.CodexLimits {
	// raw was decoded from JSON, so it always encodes again.
	data, _ := json.Marshal(raw)
	obj, _ := jsonx.Parse(data)
	limits := model.CodexLimits{
		Primary:   window(jsonx.Child(obj, "primary")),
		Secondary: window(jsonx.Child(obj, "secondary")),
	}
	credits := jsonx.Child(obj, "credits")
	if balance, ok := jsonx.Get[float64](credits, "balance"); ok {
		limits.Balance = model.Some(balance)
	} else if text, ok := jsonx.Get[string](credits, "balance"); ok {
		// Codex has written the balance as a number and as a string.
		if balance, err := strconv.ParseFloat(text, 64); err == nil {
			limits.Balance = model.Some(balance)
		}
	}
	return limits
}

// window reads one usage window; one without a percentage is not a window.
func window(obj jsonx.Object) model.Optional[model.CodexWindow] {
	used, ok := jsonx.Get[float64](obj, "used_percent")
	if !ok {
		return model.Optional[model.CodexWindow]{}
	}
	w := model.CodexWindow{UsedPct: used, WindowMinutes: jsonx.Or[float64](obj, "window_minutes")}
	if resets := jsonx.Or[float64](obj, "resets_at"); resets > 0 {
		w.ResetsAt = time.Unix(int64(resets), 0)
	}
	return model.Some(w)
}
