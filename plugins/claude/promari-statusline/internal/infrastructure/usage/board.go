package usage

import (
	"bytes"
	"encoding/json/v2"
	"fmt"
	"path/filepath"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform"
)

// The files of the board. Their paths and member names are a contract with
// the tools that read them (promari-model-router reads both, to advise on plan
// usage from a hook), not this plugin's own cache: they stay in ~/.cache
// whatever XDG_CACHE_HOME says, and a member is never renamed.
//
//	claude-rate-limits.json     {"ts": 1790971649.05, "rl": {"five_hour": {"used_percentage": 29, "resets_at": 1790985000}, "seven_day": {…}}}
//	codex-rate-statusline.json  {"rl": {"primary": {"used_percent": 100, "window_minutes": 10080, "resets_at": 1791055716}, "secondary": {…}}, "mtime": 1790827230.99}
//
// Times are Unix seconds. A window that is not known is left out, and so is a
// reset time that is not known.
const (
	boardDir        = ".cache"
	claudeBoardFile = "claude-rate-limits.json"
	codexBoardFile  = "codex-rate-statusline.json"
)

// codexRefresh is how often an unchanged Codex file is written again. Its
// readers take the file's modification time as the moment the status line
// last looked, so the file has to keep moving while the content stands still.
const codexRefresh = time.Minute

// Board writes the plan usage where other tools on this machine look for it.
type Board struct {
	Sys platform.System
}

var _ repository.UsageBoard = Board{}

type claudeBoard struct {
	TS float64       `json:"ts"`
	RL claudeWindows `json:"rl"`
}

type claudeWindows struct {
	FiveHour *claudeWindow `json:"five_hour,omitzero"`
	SevenDay *claudeWindow `json:"seven_day,omitzero"`
}

type claudeWindow struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at,omitzero"`
}

type codexBoard struct {
	RL    codexWindows `json:"rl"`
	Mtime float64      `json:"mtime"`
}

type codexWindows struct {
	Primary   *codexWindow `json:"primary,omitzero"`
	Secondary *codexWindow `json:"secondary,omitzero"`
}

type codexWindow struct {
	UsedPercent   float64 `json:"used_percent"`
	WindowMinutes float64 `json:"window_minutes,omitzero"`
	ResetsAt      int64   `json:"resets_at,omitzero"`
}

// PostClaude implements repository.UsageBoard.
func (b Board) PostClaude(l model.RateLimits, at time.Time) error {
	window := func(w model.Optional[model.RateWindow]) *claudeWindow {
		v, ok := w.Get()
		if !ok {
			return nil
		}
		return &claudeWindow{UsedPercentage: v.UsedPct, ResetsAt: unix(v.ResetsAt)}
	}
	data, err := json.Marshal(claudeBoard{
		TS: seconds(at),
		RL: claudeWindows{FiveHour: window(l.FiveHour), SevenDay: window(l.SevenDay)},
	})
	if err != nil {
		return fmt.Errorf("encode the Claude usage: %w", err)
	}
	return b.Sys.WriteFile(b.path(claudeBoardFile), data, platform.Private)
}

// PostCodex implements repository.UsageBoard. A render comes several times a
// second while Claude answers and Codex's usage changes only when Codex runs,
// so an unchanged file is written again once a minute, not on every render.
func (b Board) PostCodex(l model.CodexLimits, at time.Time) error {
	window := func(w model.Optional[model.CodexWindow]) *codexWindow {
		v, ok := w.Get()
		if !ok {
			return nil
		}
		return &codexWindow{UsedPercent: v.UsedPct, WindowMinutes: v.WindowMinutes, ResetsAt: unix(v.ResetsAt)}
	}
	data, err := json.Marshal(codexBoard{
		RL:    codexWindows{Primary: window(l.Primary), Secondary: window(l.Secondary)},
		Mtime: seconds(l.SeenAt),
	})
	if err != nil {
		return fmt.Errorf("encode the Codex usage: %w", err)
	}
	path := b.path(codexBoardFile)
	if posted, err := b.Sys.ReadFile(path); err == nil && bytes.Equal(posted, data) {
		if written, err := b.Sys.ModTime(path); err == nil && at.Sub(written) < codexRefresh {
			return nil
		}
	}
	return b.Sys.WriteFile(path, data, platform.Private)
}

func (b Board) path(file string) string {
	return filepath.Join(b.Sys.HomeDir(), boardDir, file)
}

// unix returns t in Unix seconds, and zero for the zero time (a time that is
// not known), which the encoder leaves out.
func unix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// seconds returns t in Unix seconds with its milliseconds.
func seconds(t time.Time) float64 {
	const millisPerSecond = 1000
	if t.IsZero() {
		return 0
	}
	return float64(t.UnixMilli()) / millisPerSecond
}
