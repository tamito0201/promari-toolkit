package model

import "time"

const (
	// historyWindow is how far back the usage of a rate window is remembered.
	historyWindow = 3 * time.Hour
	// historyMax bounds the remembered points.
	historyMax = 200
	// forecastMinSpan is the shortest history a growth rate is trusted over.
	forecastMinSpan = 10 * time.Minute
	// forecastMax is longer than any window: an exhaustion further away than
	// this can never come before a reset, and would not fit a Duration.
	forecastMax = 30 * 24 * time.Hour
)

// RatePoint is the usage of the rolling windows at one moment.
type RatePoint struct {
	At       time.Time         `json:"at"`
	FiveHour Optional[float64] `json:"five_hour,omitzero"`
	SevenDay Optional[float64] `json:"seven_day,omitzero"`
}

// RateHistory is the recent usage of the rolling windows, oldest first.
type RateHistory []RatePoint

// Record appends the usage seen now and drops what is too old to matter.
func (h RateHistory) Record(l RateLimits, now time.Time) RateHistory {
	point := RatePoint{At: now}
	if w, ok := l.FiveHour.Get(); ok {
		point.FiveHour = Some(w.UsedPct)
	}
	if w, ok := l.SevenDay.Get(); ok {
		point.SevenDay = Some(w.UsedPct)
	}
	out := make(RateHistory, 0, len(h)+1)
	for _, p := range h {
		if now.Sub(p.At) < historyWindow {
			out = append(out, p)
		}
	}
	out = append(out, point)
	if n := len(out); n > historyMax {
		out = out[n-historyMax:]
	}
	return out
}

// Forecast says a rate window will be exhausted before it resets.
type Forecast struct {
	Label string
	// In is the time left until the window reaches 100 %.
	In time.Duration
}

// Forecasts returns the windows that, at the pace of the recorded history,
// reach 100 % before they reset.
func (h RateHistory) Forecasts(l RateLimits, now time.Time) []Forecast {
	var out []Forecast
	for _, c := range []struct {
		label  string
		window Optional[RateWindow]
		pick   func(RatePoint) Optional[float64]
	}{
		{"5h", l.FiveHour, func(p RatePoint) Optional[float64] { return p.FiveHour }},
		{"7d", l.SevenDay, func(p RatePoint) Optional[float64] { return p.SevenDay }},
	} {
		w, ok := c.window.Get()
		if !ok || w.ResetsAt.IsZero() {
			continue
		}
		if in, ok := h.exhaustion(c.pick, w.UsedPct); ok && now.Add(in).Before(w.ResetsAt) {
			out = append(out, Forecast{Label: c.label, In: in})
		}
	}
	return out
}

// exhaustion extrapolates the growth between the oldest and the newest point
// of one window to 100 %.
func (h RateHistory) exhaustion(pick func(RatePoint) Optional[float64], current float64) (time.Duration, bool) {
	var first, last RatePoint
	var firstPct, lastPct float64
	n := 0
	for _, p := range h {
		pct, ok := pick(p).Get()
		if !ok {
			continue
		}
		if n == 0 {
			first, firstPct = p, pct
		}
		last, lastPct = p, pct
		n++
	}
	elapsed, grown := last.At.Sub(first.At), lastPct-firstPct
	if n < 2 || elapsed < forecastMinSpan || grown <= 0 {
		return 0, false
	}
	left := (percent - current) / (grown / elapsed.Seconds())
	if left >= forecastMax.Seconds() {
		return 0, false
	}
	return seconds(left), true
}
