// Package usage reads plan usage from caches a status line writes. Hooks never
// receive `rate_limits`; only the status line does, so this reads the copies a
// status line caches in ~/.cache/claude-rate-limits.json (Claude) and
// ~/.cache/codex-rate-statusline.json (Codex). Stale caches and windows past
// their reset time are ignored.
package usage

import (
	"encoding/json"
	"os"
	"time"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/repository"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/fsutil"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/pkg/fp"
)

// Reader implements repository.UsageReader. It reads the caches and hands
// the raw values to the domain (model.NewPressure), which applies the
// thresholds.
type Reader struct {
	cfg   model.PressureSettings
	clock repository.Clock
}

var _ repository.UsageReader = (*Reader)(nil)

// New builds a reader.
func New(cfg model.PressureSettings, clock repository.Clock) *Reader {
	return &Reader{cfg: cfg, clock: clock}
}

func (r *Reader) now() time.Time { return r.clock.Now() }

func (r *Reader) maxAge() time.Duration { return time.Duration(r.cfg.MaxAgeHours * float64(time.Hour)) }

func readJSON(path string, v any) error {
	raw, err := os.ReadFile(fsutil.ExpandHome(path))
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

type window struct {
	UsedPercentage *float64 `json:"used_percentage"`
	UsedPercent    *float64 `json:"used_percent"`
	ResetsAt       *float64 `json:"resets_at"`
}

func (r *Reader) value(w *window) fp.Option[float64] {
	if w == nil || (w.ResetsAt != nil && time.Unix(int64(*w.ResetsAt), 0).Before(r.now())) {
		return fp.None[float64]()
	}
	if w.UsedPercentage != nil {
		return fp.Some(*w.UsedPercentage)
	}
	if w.UsedPercent != nil {
		return fp.Some(*w.UsedPercent)
	}
	return fp.None[float64]()
}

// Claude reads the Claude plan pressure.
func (r *Reader) Claude() model.Pressure {
	var cache struct {
		TS float64 `json:"ts"`
		RL struct {
			FiveHour *window `json:"five_hour"`
			SevenDay *window `json:"seven_day"`
		} `json:"rl"`
	}
	if readJSON(r.cfg.ClaudeRateFile, &cache) != nil || r.now().Sub(time.Unix(int64(cache.TS), 0)) > r.maxAge() {
		return model.Pressure{}
	}
	return model.NewPressure(r.value(cache.RL.FiveHour), r.value(cache.RL.SevenDay), r.cfg)
}

// Codex reads the Codex primary window; unknown counts as available.
func (r *Reader) Codex() model.CodexQuota {
	path := fsutil.ExpandHome(r.cfg.CodexRateFile)
	info, err := os.Stat(path)
	if err != nil || r.now().Sub(info.ModTime()) > r.maxAge() {
		return model.CodexQuota{Available: true}
	}
	var cache struct {
		RL struct {
			Primary *window `json:"primary"`
		} `json:"rl"`
	}
	if readJSON(path, &cache) != nil {
		return model.CodexQuota{Available: true}
	}
	return model.NewCodexQuota(r.value(cache.RL.Primary), r.cfg)
}
