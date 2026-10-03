package model

import "promari-model-router/pkg/fp"

// Pressure is the Claude plan usage read from the status-line cache.
type Pressure struct {
	Known    bool
	High     bool
	FiveHour fp.Option[float64]
	SevenDay fp.Option[float64]
}

// CodexQuota says whether Codex has room left.
type CodexQuota struct {
	Available bool
	Used      fp.Option[float64]
}

// NewPressure interprets the raw plan usage against the thresholds: usage is
// high when a known window is at or above its threshold. The adapter only
// reads the cache; the judgement is the domain's.
func NewPressure(fiveHour, sevenDay fp.Option[float64], cfg PressureSettings) Pressure {
	high := func(o fp.Option[float64], threshold float64) bool {
		v, ok := o.Get()
		return ok && v >= threshold
	}
	return Pressure{
		Known: true, FiveHour: fiveHour, SevenDay: sevenDay,
		High: high(fiveHour, cfg.FiveHourHigh) || high(sevenDay, cfg.SevenDayHigh),
	}
}

// NewCodexQuota interprets the raw Codex usage: Codex is available unless the
// known usage is at or above the block threshold (unknown counts as available).
func NewCodexQuota(used fp.Option[float64], cfg PressureSettings) CodexQuota {
	v, ok := used.Get()
	return CodexQuota{Available: !ok || v < cfg.CodexBlockPercent, Used: used}
}
