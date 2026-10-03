package model_test

import (
	"encoding/json/v2"
	"slices"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
)

var t0 = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

func TestOptional(t *testing.T) {
	t.Parallel()
	type record struct {
		Kept    model.Optional[float64] `json:"kept"`
		Omitted model.Optional[float64] `json:"omitted,omitzero"`
	}
	tests := []struct {
		name string
		in   record
		want string
	}{
		{"absent is null, or left out with omitzero", record{}, `{"kept":null}`},
		{"zero is a value, not absent", record{Kept: model.Some(0.0), Omitted: model.Some(0.0)}, `{"kept":0,"omitted":0}`},
		{"a value", record{Kept: model.Some(1.5)}, `{"kept":1.5}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data, err := json.Marshal(tt.in)
			if err != nil || string(data) != tt.want {
				t.Fatalf("Marshal = %s, %v; want %s", data, err, tt.want)
			}
			var back record
			if err := json.Unmarshal(data, &back); err != nil || back != tt.in {
				t.Errorf("Unmarshal(%s) = %+v, %v; want %+v", data, back, err, tt.in)
			}
		})
	}
	t.Run("a value of the wrong type is an error and leaves the optional absent", func(t *testing.T) {
		t.Parallel()
		var r record
		if err := json.Unmarshal([]byte(`{"kept":"x"}`), &r); err == nil || r.Kept.Present() {
			t.Errorf("Unmarshal = %+v, %v; want an error and an absent value", r, err)
		}
	})
	t.Run("Or", func(t *testing.T) {
		t.Parallel()
		if got := (model.Optional[int]{}).Or(7); got != 7 {
			t.Errorf("absent.Or(7) = %d", got)
		}
		if got := model.Some(0).Or(7); got != 0 {
			t.Errorf("Some(0).Or(7) = %d; zero is a value", got)
		}
	})
}

func TestSessionKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		id   string
		want bool
	}{
		{"3f2a-41b0_x.1", true},
		{"", false},
		{"../etc/passwd", false},
		{"a/b", false},
		{".hidden", false},
		{"a*b", false},
		{"a b", false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			s := model.Session{ID: tt.id}
			if _, ok := s.Key(); ok != tt.want {
				t.Errorf("Key(%q) ok = %v, want %v", tt.id, ok, tt.want)
			}
		})
	}
}

func TestSessionWorkDir(t *testing.T) {
	t.Parallel()
	for dir, want := range map[string]string{"": ".", "/work": "/work"} {
		t.Run(dir, func(t *testing.T) {
			t.Parallel()
			s := model.Session{Dir: dir}
			if got := s.WorkDir(); got != want {
				t.Errorf("WorkDir() = %q, want %q", got, want)
			}
		})
	}
}

func TestContextUsage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		window model.ContextWindow
		want   model.ContextUsage
		ok     bool
	}{
		{"nothing reported", model.ContextWindow{}, model.ContextUsage{}, false},
		{"a size without a percentage", model.ContextWindow{Size: 200_000}, model.ContextUsage{}, false},
		{"a percentage without a size", model.ContextWindow{UsedPct: model.Some(10.0)}, model.ContextUsage{}, false},
		{
			"zero percent is known usage",
			model.ContextWindow{Size: 200_000, UsedPct: model.Some(0.0)},
			model.ContextUsage{Pct: 0, Used: 0, Remain: 200_000, Size: 200_000},
			true,
		},
		{
			"the reported tokens win over the percentage",
			model.ContextWindow{Size: 200_000, UsedPct: model.Some(42.0), Current: 84_500},
			model.ContextUsage{Pct: 42, Used: 84_500, Remain: 115_500, Size: 200_000},
			true,
		},
		{
			"without tokens the percentage gives them",
			model.ContextWindow{Size: 200_000, UsedPct: model.Some(42.0)},
			model.ContextUsage{Pct: 42, Used: 84_000, Remain: 116_000, Size: 200_000},
			true,
		},
		{
			"more tokens than the window leaves nothing, not less than nothing",
			model.ContextWindow{Size: 1000, UsedPct: model.Some(100.0), Current: 1200},
			model.ContextUsage{Pct: 100, Used: 1200, Remain: 0, Size: 1000},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got, ok := tt.window.Usage(); got != tt.want || ok != tt.ok {
				t.Errorf("Usage() = %+v, %v; want %+v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestRateLimitsAll(t *testing.T) {
	t.Parallel()
	window := model.Some(model.RateWindow{UsedPct: 1})
	tests := []struct {
		name   string
		limits model.RateLimits
		want   []string
	}{
		{"none", model.RateLimits{}, nil},
		{"in display order", model.RateLimits{FiveHour: window, SevenDay: window, Spend: window}, []string{"5h", "7d", "Spend"}},
		{"only the known ones", model.RateLimits{SevenDay: window}, []string{"7d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got []string
			for w := range tt.limits.All() {
				got = append(got, w.Label)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("All() = %v, want %v", got, tt.want)
			}
			if empty := tt.limits.Empty(); empty != (len(tt.want) == 0) {
				t.Errorf("Empty() = %v with windows %v", empty, tt.want)
			}
		})
	}
	t.Run("stops when the consumer does", func(t *testing.T) {
		t.Parallel()
		n := 0
		for range (model.RateLimits{FiveHour: window, SevenDay: window}).All() {
			n++
			break
		}
		if n != 1 {
			t.Errorf("visited %d windows after break, want 1", n)
		}
	})
}

func TestRateWindowPace(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		window model.RateWindow
		length time.Duration
		want   float64
		ok     bool
	}{
		{"half the window gone, all of it used", model.RateWindow{UsedPct: 100, ResetsAt: t0.Add(150 * time.Minute)}, model.FiveHours, 2, true},
		{"usage level with the clock", model.RateWindow{UsedPct: 50, ResetsAt: t0.Add(150 * time.Minute)}, model.FiveHours, 1, true},
		{"a window that does not roll", model.RateWindow{UsedPct: 50, ResetsAt: t0.Add(time.Hour)}, 0, 0, false},
		{"no reset time", model.RateWindow{UsedPct: 50}, model.FiveHours, 0, false},
		{"already reset", model.RateWindow{UsedPct: 50, ResetsAt: t0.Add(-time.Minute)}, model.FiveHours, 0, false},
		{"a reset further away than the window is long", model.RateWindow{UsedPct: 50, ResetsAt: t0.Add(6 * time.Hour)}, model.FiveHours, 0, false},
		{"the window has barely started", model.RateWindow{UsedPct: 50, ResetsAt: t0.Add(299 * time.Minute)}, model.FiveHours, 0, false},
		{"nothing used", model.RateWindow{UsedPct: 0, ResetsAt: t0.Add(150 * time.Minute)}, model.FiveHours, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got, ok := tt.window.Pace(t0, tt.length); got != tt.want || ok != tt.ok {
				t.Errorf("Pace() = %v, %v; want %v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestActivityObserve(t *testing.T) {
	t.Parallel()
	type step struct {
		after  time.Duration
		used   float64
		prompt string
	}
	tests := []struct {
		name        string
		steps       []step
		samples     int
		compactions int
		turns       int
		worked      time.Duration
		idled       time.Duration
		streak      time.Duration
	}{
		{"the first render starts the streak", []step{{0, 1000, "p1"}}, 1, 0, 1, 0, 0, 0},
		{
			"renders close together are work",
			[]step{{0, 1000, "p1"}, {time.Minute, 2000, "p1"}, {4 * time.Minute, 3000, "p2"}},
			3, 0, 2, 5 * time.Minute, 0, 5 * time.Minute,
		},
		{
			"exactly the idle gap is still work",
			[]step{{0, 1000, ""}, {model.IdleGap, 1000, ""}},
			1, 0, 0, model.IdleGap, 0, model.IdleGap,
		},
		{
			"a longer pause is idle and restarts the streak",
			[]step{{0, 1000, ""}, {model.IdleGap + time.Second, 1000, ""}, {time.Minute, 1500, ""}},
			2, 0, 0, time.Minute, model.IdleGap + time.Second, time.Minute,
		},
		{
			"an unchanged context adds no sample",
			[]step{{0, 1000, ""}, {time.Second, 1000, ""}, {time.Second, 1000, ""}},
			1, 0, 0, 2 * time.Second, 0, 2 * time.Second,
		},
		{
			"a large context that shrinks was compacted, and the history starts over",
			[]step{{0, 150_000, ""}, {time.Second, 160_000, ""}, {time.Second, 60_000, ""}},
			1, 1, 0, 2 * time.Second, 0, 2 * time.Second,
		},
		{
			"a small context that shrinks was not compacted",
			[]step{{0, 90_000, ""}, {time.Second, 30_000, ""}},
			2, 0, 0, time.Second, 0, time.Second,
		},
		{
			"a clock that moved back adds no negative work",
			[]step{{0, 1000, ""}, {-time.Hour, 1000, ""}},
			1, 0, 0, 0, 0, -time.Hour,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var a model.Activity
			now := t0
			for _, s := range tt.steps {
				now = now.Add(s.after)
				a.Observe(s.used, s.prompt, now)
			}
			if len(a.Samples) != tt.samples || a.Compactions != tt.compactions || a.Turns != tt.turns {
				t.Errorf("samples, compactions, turns = %d, %d, %d; want %d, %d, %d",
					len(a.Samples), a.Compactions, a.Turns, tt.samples, tt.compactions, tt.turns)
			}
			if a.Worked() != tt.worked || a.Idled() != tt.idled {
				t.Errorf("worked, idled = %v, %v; want %v, %v", a.Worked(), a.Idled(), tt.worked, tt.idled)
			}
			if tt.streak >= 0 && a.Streak(now) != now.Sub(a.StreakStart) {
				t.Errorf("Streak() = %v, want %v", a.Streak(now), now.Sub(a.StreakStart))
			}
		})
	}
	t.Run("only the newest samples are kept", func(t *testing.T) {
		t.Parallel()
		var a model.Activity
		for i := range 30 {
			a.Observe(float64(1000+i), "", t0.Add(time.Duration(i)*time.Second))
		}
		if len(a.Samples) != 20 || a.Samples[0].Tokens != 1010 {
			t.Errorf("kept %d samples starting at %v, want 20 starting at 1010", len(a.Samples), a.Samples[0].Tokens)
		}
	})
	t.Run("survives a round trip through JSON", func(t *testing.T) {
		t.Parallel()
		var a model.Activity
		a.Observe(1000, "p1", t0)
		a.Observe(2000, "p2", t0.Add(time.Minute))
		data, err := json.Marshal(a)
		if err != nil {
			t.Fatal(err)
		}
		var back model.Activity
		if err := json.Unmarshal(data, &back); err != nil {
			t.Fatal(err)
		}
		if back.Turns != 2 || back.Worked() != time.Minute || !back.LastSeen.Equal(a.LastSeen) || len(back.Samples) != 2 {
			t.Errorf("round trip = %+v, want %+v", back, a)
		}
	})
}

func TestActivityETA(t *testing.T) {
	t.Parallel()
	samples := func(elapsed time.Duration, from, to float64) []model.Sample {
		return []model.Sample{{At: t0, Tokens: from}, {At: t0.Add(elapsed), Tokens: to}}
	}
	tests := []struct {
		name    string
		samples []model.Sample
		remain  float64
		want    time.Duration
		ok      bool
	}{
		{"1000 tokens a minute, 60000 left", samples(time.Minute, 0, 1000), 60_000, time.Hour, true},
		{"one sample is no history", samples(time.Minute, 0, 1000)[:1], 60_000, 0, false},
		{"a history shorter than a minute", samples(59*time.Second, 0, 1000), 60_000, 0, false},
		{"a context that does not grow", samples(time.Minute, 1000, 1000), 60_000, 0, false},
		{"a context that shrank", samples(time.Minute, 2000, 1000), 60_000, 0, false},
		{"twelve hours away is too far", samples(time.Minute, 0, 1000), 720_000, 0, false},
		{"just under twelve hours", samples(time.Minute, 0, 1000), 719_000, 719 * time.Minute, true},
		{"growth so slow the estimate would not fit a Duration", samples(24*time.Hour, 0, 1e-9), 1e9, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := model.Activity{Samples: tt.samples}
			if got, ok := a.ETA(tt.remain); got != tt.want || ok != tt.ok {
				t.Errorf("ETA() = %v, %v; want %v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestRateHistoryRecord(t *testing.T) {
	t.Parallel()
	limits := model.RateLimits{FiveHour: model.Some(model.RateWindow{UsedPct: 10})}
	t.Run("keeps three hours and records only the known windows", func(t *testing.T) {
		t.Parallel()
		history := model.RateHistory{{At: t0.Add(-3 * time.Hour)}, {At: t0.Add(-179 * time.Minute)}}.Record(limits, t0)
		if len(history) != 2 || !history[0].At.Equal(t0.Add(-179*time.Minute)) {
			t.Fatalf("history = %+v, want the point 179 minutes old and the new one", history)
		}
		if last := history[1]; last.FiveHour.Or(-1) != 10 || last.SevenDay.Present() {
			t.Errorf("new point = %+v, want five_hour 10 and no seven_day", last)
		}
	})
	t.Run("keeps at most 200 points", func(t *testing.T) {
		t.Parallel()
		var history model.RateHistory
		for i := range 250 {
			history = history.Record(limits, t0.Add(time.Duration(i)*time.Second))
		}
		if len(history) != 200 || !history[0].At.Equal(t0.Add(50*time.Second)) {
			t.Errorf("kept %d points from %v, want 200 from second 50", len(history), history[0].At)
		}
	})
}

func TestRateHistoryForecasts(t *testing.T) {
	t.Parallel()
	point := func(at time.Duration, five float64) model.RatePoint {
		return model.RatePoint{At: t0.Add(at), FiveHour: model.Some(five)}
	}
	window := func(pct float64, resetsIn time.Duration) model.RateLimits {
		return model.RateLimits{FiveHour: model.Some(model.RateWindow{UsedPct: pct, ResetsAt: t0.Add(resetsIn)})}
	}
	tests := []struct {
		name    string
		history model.RateHistory
		limits  model.RateLimits
		want    []model.Forecast
	}{
		{
			"10 points in 10 minutes from 50: full in 50 minutes, before the reset in two hours",
			model.RateHistory{point(-10*time.Minute, 40), point(0, 50)},
			window(50, 2*time.Hour),
			[]model.Forecast{{Label: "5h", In: 50 * time.Minute}},
		},
		{"the reset comes first", model.RateHistory{point(-10*time.Minute, 40), point(0, 50)}, window(50, 30*time.Minute), nil},
		{"a history shorter than ten minutes", model.RateHistory{point(-9*time.Minute, 40), point(0, 50)}, window(50, 2*time.Hour), nil},
		{"usage that does not grow", model.RateHistory{point(-10*time.Minute, 50), point(0, 50)}, window(50, 2*time.Hour), nil},
		{"one point", model.RateHistory{point(0, 50)}, window(50, 2*time.Hour), nil},
		{"no reset time", model.RateHistory{point(-10*time.Minute, 40), point(0, 50)}, model.RateLimits{FiveHour: model.Some(model.RateWindow{UsedPct: 50})}, nil},
		{"a window that is not known", model.RateHistory{point(-10*time.Minute, 40), point(0, 50)}, model.RateLimits{}, nil},
		{"growth too slow to ever matter", model.RateHistory{point(-10*time.Minute, 50), point(0, 50.000001)}, window(50, 2*time.Hour), nil},
		{
			"points without this window are skipped",
			model.RateHistory{point(-10*time.Minute, 40), {At: t0.Add(-5 * time.Minute)}, point(0, 50)},
			window(50, 2*time.Hour),
			[]model.Forecast{{Label: "5h", In: 50 * time.Minute}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.history.Forecasts(tt.limits, t0); !slices.Equal(got, tt.want) {
				t.Errorf("Forecasts() = %v, want %v", got, tt.want)
			}
		})
	}
	t.Run("the seven-day window is forecast from its own points", func(t *testing.T) {
		t.Parallel()
		history := model.RateHistory{
			{At: t0.Add(-20 * time.Minute), SevenDay: model.Some(80.0)},
			{At: t0, SevenDay: model.Some(90.0)},
		}
		limits := model.RateLimits{SevenDay: model.Some(model.RateWindow{UsedPct: 90, ResetsAt: t0.Add(24 * time.Hour)})}
		want := []model.Forecast{{Label: "7d", In: 20 * time.Minute}}
		if got := history.Forecasts(limits, t0); !slices.Equal(got, want) {
			t.Errorf("Forecasts() = %v, want %v", got, want)
		}
	})
}

func TestFacts(t *testing.T) {
	t.Parallel()
	amount := func(v float64) model.Optional[model.Amount] { return model.Some(model.Amount{Value: v}) }
	t.Run("EstimatedBlock", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			spend model.Spend
			want  float64
			ok    bool
		}{
			{"block plus burn for the time left", model.Spend{Block: amount(10), BurnPerHour: amount(4), BlockLeft: 90 * time.Minute}, 16, true},
			{"no block", model.Spend{BurnPerHour: amount(4), BlockLeft: time.Hour}, 0, false},
			{"no burn rate", model.Spend{Block: amount(10), BlockLeft: time.Hour}, 0, false},
			{"a burn rate of zero", model.Spend{Block: amount(10), BurnPerHour: amount(0), BlockLeft: time.Hour}, 0, false},
			{"no time left", model.Spend{Block: amount(10), BurnPerHour: amount(4)}, 0, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if got, ok := tt.spend.EstimatedBlock(); got != tt.want || ok != tt.ok {
					t.Errorf("EstimatedBlock() = %v, %v; want %v, %v", got, ok, tt.want, tt.ok)
				}
			})
		}
	})
	t.Run("ErrorRate", func(t *testing.T) {
		t.Parallel()
		if rate, ok := (model.ToolStats{Total: 200, Errors: 5}).ErrorRate(); rate != 2.5 || !ok {
			t.Errorf("ErrorRate() = %v, %v; want 2.5, true", rate, ok)
		}
		if _, ok := (model.ToolStats{}).ErrorRate(); ok {
			t.Error("no tool call has no error rate; got one")
		}
	})
	t.Run("Severe", func(t *testing.T) {
		t.Parallel()
		for indicator, want := range map[string]bool{"minor": false, "major": true, "critical": true, "none": false} {
			if got := (model.Incident{Indicator: indicator}).Severe(); got != want {
				t.Errorf("Severe(%s) = %v, want %v", indicator, got, want)
			}
		}
	})
	t.Run("Low", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			battery model.Battery
			want    bool
		}{
			{model.Battery{Percent: 19}, true},
			{model.Battery{Percent: 20}, false},
			{model.Battery{Percent: 5, OnPower: true}, false},
		}
		for _, tt := range tests {
			if got := tt.battery.Low(); got != tt.want {
				t.Errorf("Low(%+v) = %v, want %v", tt.battery, got, tt.want)
			}
		}
	})
}

func TestChip(t *testing.T) {
	t.Parallel()
	chip := model.Chip{{Text: "Bat ", Tone: model.ToneMuted}, {Text: "9%", Tone: model.ToneDanger}}
	if got := chip.Text(); got != "Bat 9%" {
		t.Errorf("Text() = %q", got)
	}
	alarmed := chip.Alarmed()
	if !alarmed[0].Alarm || !alarmed[1].Alarm || alarmed[1].Tone != model.ToneDanger {
		t.Errorf("Alarmed() = %+v, want every span an alarm with its tone kept", alarmed)
	}
	if chip[0].Alarm {
		t.Error("Alarmed() changed the chip it was called on")
	}
}
