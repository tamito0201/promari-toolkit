package service

import (
	"time"

	"promari-statusline/internal/domain/model"
)

// View is everything one render knows.
type View struct {
	Now     time.Time
	Session model.Session
	// Limits are the rate limits to show: the session's own, or the ones
	// remembered from an earlier render.
	Limits model.RateLimits
	// LimitsSeen is when remembered limits were seen; zero when they came with
	// this render.
	LimitsSeen time.Time
	Usage      model.Optional[model.ContextUsage]
	// Activity is absent when the session has no usable id; its counters are
	// then unknown, not zero.
	Activity  model.Optional[model.Activity]
	Forecasts []model.Forecast
	Facts     model.Facts
	// AlarmAll makes every usage chip an alarm, to check that blinking works.
	AlarmAll bool
	// Peers are the other sessions running on this machine.
	Peers model.Roster
	// Running is the number of sessions running, this one included; zero when
	// unknown.
	Running int
}

// alarmPct is the usage at which a chip becomes an alarm.
const alarmPct = 90.0

// alarmAt returns the usage percentage from which a chip is an alarm.
func (v *View) alarmAt() float64 {
	if v.AlarmAll {
		return 0
	}
	return alarmPct
}

// alarmIf marks the chip as an alarm when on is true.
func alarmIf(on bool, c model.Chip) model.Chip {
	if on {
		return c.Alarmed()
	}
	return c
}

// section is one category of the status line: its title and the chips it
// builds from a view. A section without chips is left out.
type section struct {
	title string
	tone  model.Tone
	chips func(*View) []model.Chip
}

// sections lists the categories in display order, the most urgent first:
// outages and exhaustion, then what is left, what it costs, how the work
// goes, and last the surroundings. A new category is one more entry here.
func sections() []section {
	return []section{
		{"🚨 Alert", model.ToneDanger, alertChips},
		{"📉 Forecast", model.ToneDanger, forecastChips},
		{"🧠 Context", model.ToneAccent, contextChips},
		{"", model.TonePlain, claudeChips}, // the chip carries its own ⚡ Claude header
		{"", model.TonePlain, codexChips},  // the chip carries its own 🤖 Codex header
		{"💰 Cost", model.ToneMoney, costChips},
		{"🔥 Burn", model.ToneDanger, burnChips},
		{"📈 KPI", model.ToneGood, kpiChips},
		{"🚀 Perf", model.ToneNote, perfChips},
		{"📦 Cache", model.ToneNote, cacheChips},
		{"📊 Tokens", model.ToneInfo, tokenChips},
		{"🔧 Work", model.ToneNote, workChips},
		{"🌿 Git", model.ToneGood, gitChips},
		{"🔀 PR", model.ToneInfo, pullChips},
		{"🔖 Session", model.ToneNote, sessionChips},
		{"👥 Sessions", model.ToneInfo, peerChips},
		{"🧭 Env", model.ToneAccent, envChips},
		{"💻 System", model.ToneInfo, systemChips},
		{"🧾 Meta", model.ToneMuted, metaChips},
		{"🎵 Music", model.ToneAccent, musicChips},
	}
}

// Compose turns a view into the groups of the status line, in display order.
func Compose(v *View) []model.Group {
	var groups []model.Group
	for _, s := range sections() {
		if chips := s.chips(v); len(chips) > 0 {
			groups = append(groups, model.Group{Title: s.title, Tone: s.tone, Chips: chips})
		}
	}
	return groups
}
