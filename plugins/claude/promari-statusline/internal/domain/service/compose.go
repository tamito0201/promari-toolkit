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
	band  model.Band
	title string
	tone  model.Tone
	chips func(*View) []model.Chip
}

// sections lists the categories in display order, one line each, from the
// top: what needs action now, then what is glanced at most often, then the
// details read only when something looks off. The order is fixed (2026-10-08):
//
//  1. Alerts — outages and exhaustion, shown only when they happen.
//  2. Limits — context and the Claude and Codex windows: whether the session
//     can go on at all.
//  3. Money — what it costs and how fast it burns.
//  4. Where the work stands — the session's own work, what is due, and the
//     session and its peers.
//  5. How well it goes — KPI, performance, cache, tokens, agents, quality,
//     rules, trace and habits.
//  6. Surroundings — environment, machine, meta and music.
//  7. Git and the pull request — the branch context changes least often, so
//     it anchors the bottom, Git on the very last line (2026-10-08 指示).
//     They stay in the WORK band: the position is about reading order, not
//     about which question they answer.
//
// A new category is one more entry here, in the band it belongs to.
func sections() []section {
	return []section{
		// 1. Alerts
		{model.BandAlerts, "🚨 Alert", model.ToneDanger, alertChips},
		{model.BandAlerts, "📉 Forecast", model.ToneDanger, forecastChips},
		// 2. Limits
		{model.BandLimits, "🧠 Context", model.ToneAccent, contextChips},
		{model.BandLimits, "", model.TonePlain, claudeChips}, // the chip carries its own ⚡ Claude header
		{model.BandLimits, "", model.TonePlain, codexChips},  // the chip carries its own 🤖 Codex header
		// 3. Money
		{model.BandMoney, "💰 Cost", model.ToneMoney, costChips},
		{model.BandMoney, "🔥 Burn", model.ToneDanger, burnChips},
		// 4. Where the work stands
		{model.BandWork, "🔧 Work", model.ToneNote, workChips},
		{model.BandWork, "⏰ Due", model.ToneCaution, dueChips},
		{model.BandWork, "🔖 Session", model.ToneNote, sessionChips},
		{model.BandWork, "👥 Sessions", model.ToneInfo, peerChips},
		// 5. How well it goes
		{model.BandMetrics, "📈 KPI", model.ToneGood, kpiChips},
		{model.BandMetrics, "🚀 Perf", model.ToneNote, perfChips},
		{model.BandMetrics, "📦 Cache", model.ToneNote, cacheChips},
		{model.BandMetrics, "📊 Tokens", model.ToneInfo, tokenChips},
		{model.BandMetrics, "🤝 Agent", model.ToneAccent, agentChips},
		{model.BandMetrics, "🧪 Quality", model.ToneGood, qualityChips},
		{model.BandMetrics, "📏 Rules", model.ToneCaution, rulesChips},
		{model.BandMetrics, "🧬 Trace", model.ToneInfo, traceChips},
		{model.BandMetrics, "🎓 Habits", model.ToneGood, habitsChips},
		// 研究由来の記述統計。実測できる範囲を個別に表示する。
		{model.BandMetrics, "📐 Latency", model.ToneInfo, latencySection.chips},
		{model.BandMetrics, "🧰 Tools", model.ToneInfo, toolsSection.chips},
		{model.BandMetrics, "🎲 Diversity", model.ToneInfo, diversitySection.chips},
		{model.BandMetrics, "🧮 Budget", model.ToneInfo, budgetSection.chips},
		{model.BandMetrics, "🔬 Evidence", model.ToneInfo, evidenceSection.chips},
		// 6. Surroundings
		{model.BandSurroundings, "🧭 Env", model.ToneAccent, envChips},
		{model.BandSurroundings, "💻 System", model.ToneInfo, systemChips},
		{model.BandSurroundings, "🧾 Meta", model.ToneMuted, metaChips},
		{model.BandSurroundings, "🎵 Music", model.ToneAccent, musicChips},
		// 7. Git last (PR just above it)
		{model.BandWork, "🔀 PR", model.ToneInfo, pullChips},
		{model.BandWork, "🌿 Git", model.ToneGood, gitChips},
	}
}

// BandInfo names a band for a reader: a short English name and the question
// the band answers.
type BandInfo struct {
	Band     model.Band
	Name     string
	Question string
}

// Bands returns the bands in display order.
func Bands() []BandInfo {
	return []BandInfo{
		{model.BandAlerts, "ALERTS", "いま手を打つべきこと"},
		{model.BandLimits, "LIMITS", "このまま続けられるか"},
		{model.BandMoney, "MONEY", "いくらかかっているか"},
		{model.BandWork, "WORK", "作業はどこまで進んだか"},
		{model.BandMetrics, "METRICS", "うまく進んでいるか"},
		{model.BandSurroundings, "SURROUNDINGS", "どんな環境で動いているか"},
	}
}

// Compose turns a view into the groups of the status line, in display order.
func Compose(v *View) []model.Group {
	var groups []model.Group
	for _, s := range sections() {
		if chips := s.chips(v); len(chips) > 0 {
			groups = append(groups, model.Group{Title: s.title, Tone: s.tone, Chips: chips, Band: s.band})
		}
	}
	return groups
}
