package service

import (
	"math"
	"slices"
	"time"

	"promari-statusline/internal/domain/model"
)

const (
	contextBarCells = 10
	windowBarCells  = 5
	// incidentCharacters bounds the description of an incident.
	incidentCharacters = 30
	// staleAfter is the age from which a remembered value shows its date.
	staleAfter = 6 * time.Hour

	// Usage running ahead of the window by paceShow is shown, by paceBad in red.
	paceShow = 1.05
	paceBad  = 1.5

	// Codex names its windows by their length in minutes.
	codexWeekMinutes     = 10_000
	codexFiveHourMinutes = 240
)

// alertChips shows an ongoing incident of the API. A major outage is an alarm.
func alertChips(v *View) []model.Chip {
	incident, ok := v.Facts.Incident.Get()
	if !ok || incident.Indicator == "" || incident.Indicator == "none" {
		return nil
	}
	tone := model.ToneCaution
	if incident.Severe() {
		tone = model.ToneDanger
	}
	description := []rune(incident.Description)
	description = description[:min(len(description), incidentCharacters)]
	c := chip(tone, "🌐 API "+incident.Indicator+": "+string(description))
	return []model.Chip{alarmIf(incident.Severe(), c)}
}

// forecastChips warns about rate windows that run out before they reset.
func forecastChips(v *View) []model.Chip {
	chips := make([]model.Chip, 0, len(v.Forecasts))
	for _, f := range v.Forecasts {
		chips = append(chips, chip(model.ToneDanger, f.Label+" 枯渇まで "+span(f.In)+" (reset前)").Alarmed())
	}
	return chips
}

// contextChips shows how full the context window is and when it will be full.
func contextChips(v *View) []model.Chip {
	usage, ok := v.Usage.Get()
	if !ok {
		if v.Session.Reported {
			return []model.Chip{chip(model.ToneMuted, "初回応答待ち")}
		}
		return nil
	}
	tone := severity(usage.Pct, warnPct, badPct)
	c := slices.Concat(model.Chip(bar(usage.Pct, contextBarCells)), model.Chip{
		space(),
		text(tone, fixed(usage.Pct, 0)+"%"),
		text(model.TonePlain, " "+tokens(usage.Used)+"/"+tokens(usage.Size)+" "),
		text(tone, "残 "+tokens(usage.Remain)),
	})
	if activity, ok := v.Activity.Get(); ok {
		if eta, ok := activity.ETA(usage.Remain); ok {
			c = append(c, space(), text(tone, "⏳ ETA "+span(eta)))
		}
	}
	return []model.Chip{alarmIf(usage.Pct >= v.alarmAt(), c)}
}

// claudeChips shows the subscription's usage windows in one chip.
func claudeChips(v *View) []model.Chip {
	c := model.Chip{{Text: "⚡ Claude", Tone: model.ToneBrand, Bold: true}}
	for w := range v.Limits.All() {
		c = append(c, space())
		c = append(c, claudeWindow(v, w)...)
	}
	if len(c) == 1 {
		return nil
	}
	if !v.LimitsSeen.IsZero() && v.Now.Sub(v.LimitsSeen) > staleAfter {
		c = append(c, text(model.ToneMuted, " "+monthDay(v.LimitsSeen)))
	}
	return []model.Chip{c}
}

// claudeWindow shows one window: its label, a bar, the percentage, the time
// until it resets, and how far usage runs ahead of the clock.
func claudeWindow(v *View, w model.LabelledWindow) model.Chip {
	pct := w.Window.UsedPct
	c := windowChip(w.Label, pct, w.Window.ResetsAt, v.Now)
	if pace, ok := w.Window.Pace(v.Now, w.Length); ok && pace >= paceShow {
		tone := model.ToneCaution
		if pace >= paceBad {
			tone = model.ToneDanger
		}
		c = append(c, space(), text(tone, "Pace ×"+fixed(pace, 1)))
	}
	return alarmIf(pct >= v.alarmAt(), c)
}

// windowChip is the part every usage window shares.
func windowChip(label string, pct float64, resetsAt, now time.Time) model.Chip {
	c := slices.Concat(model.Chip{text(model.ToneMuted, label), space()}, model.Chip(bar(pct, windowBarCells)), model.Chip{
		space(),
		text(severity(pct, warnPct, badPct), fixed(pct, 0)+"%"),
	})
	if !resetsAt.IsZero() {
		c = append(c, space(), text(model.ToneMuted, "🔄 "+until(resetsAt, now)))
	}
	return c
}

// codexChips shows the usage windows Codex last reported, in one chip.
func codexChips(v *View) []model.Chip {
	codex, ok := v.Facts.Codex.Get()
	if !ok {
		return nil
	}
	c := model.Chip{{Text: "🤖 Codex", Tone: model.ToneNote, Bold: true}}
	for _, slot := range []struct {
		prefix string
		window model.Optional[model.CodexWindow]
	}{{"", codex.Primary}, {"2:", codex.Secondary}} {
		w, ok := slot.window.Get()
		if !ok {
			continue
		}
		part := windowChip(slot.prefix+codexLabel(w.WindowMinutes), w.UsedPct, w.ResetsAt, v.Now)
		c = append(c, space())
		c = append(c, alarmIf(w.UsedPct >= v.alarmAt(), part)...)
	}
	if balance, ok := codex.Balance.Get(); ok {
		c = append(c, space(), text(model.ToneMoney, "💳 Bal $"+grouped(math.Trunc(balance))))
	}
	if len(c) == 1 {
		return nil // a header with nothing to say
	}
	if !codex.SeenAt.IsZero() && v.Now.Sub(codex.SeenAt) > staleAfter {
		c = append(c, space(), text(model.ToneMuted, monthDay(codex.SeenAt)))
	}
	return []model.Chip{c}
}

func codexLabel(minutes float64) string {
	switch {
	case minutes >= codexWeekMinutes:
		return "7d"
	case minutes >= codexFiveHourMinutes:
		return "5h"
	default:
		return plain(minutes) + "m"
	}
}
