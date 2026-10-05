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

// sections lists the categories in display order, one line each, from the
// top: what needs action now, then what is glanced at most often, then the
// details read only when something looks off. The order is fixed (2026-10-06):
//
//  1. Alerts — outages and exhaustion, shown only when they happen.
//  2. Limits — context and the Claude and Codex windows: whether the session
//     can go on at all.
//  3. Money — what it costs and how fast it burns.
//  4. Where the work stands — branch, pull request, the session's own work,
//     what is due, and the session and its peers.
//  5. How well it goes — KPI, performance, cache, tokens, agents, quality,
//     rules, trace and habits.
//  6. Surroundings — environment, machine, meta and music.
//
// A new category is one more entry here, in the band it belongs to.
func sections() []section {
	return []section{
		// 1. Alerts
		{"🚨 Alert", model.ToneDanger, alertChips},
		{"📉 Forecast", model.ToneDanger, forecastChips},
		// 2. Limits
		{"🧠 Context", model.ToneAccent, contextChips},
		{"", model.TonePlain, claudeChips}, // the chip carries its own ⚡ Claude header
		{"", model.TonePlain, codexChips},  // the chip carries its own 🤖 Codex header
		// 3. Money
		{"💰 Cost", model.ToneMoney, costChips},
		{"🔥 Burn", model.ToneDanger, burnChips},
		// 4. Where the work stands
		{"🌿 Git", model.ToneGood, gitChips},
		{"🔀 PR", model.ToneInfo, pullChips},
		{"🔧 Work", model.ToneNote, workChips},
		{"⏰ Due", model.ToneCaution, dueChips},
		{"🔖 Session", model.ToneNote, sessionChips},
		{"👥 Sessions", model.ToneInfo, peerChips},
		// 5. How well it goes
		{"📈 KPI", model.ToneGood, kpiChips},
		{"🚀 Perf", model.ToneNote, perfChips},
		{"📦 Cache", model.ToneNote, cacheChips},
		{"📊 Tokens", model.ToneInfo, tokenChips},
		{"🤝 Agent", model.ToneAccent, agentChips},
		{"🧪 Quality", model.ToneGood, qualityChips},
		{"📏 Rules", model.ToneCaution, rulesChips},
		{"🧬 Trace", model.ToneInfo, traceChips},
		{"🎓 Habits", model.ToneGood, habitsChips},
		// 6. Surroundings
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
