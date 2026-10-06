package usecase_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"promari-statusline/internal/application/usecase"
	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
)

// archive is a recorded session report.
type archive struct {
	raw []byte
	at  time.Time
	err error
}

func (a archive) LastInput() ([]byte, time.Time, error) { return a.raw, a.at, a.err }

// decodeTo returns a decoder that turns any report into s, and no report into
// an empty session, as the status line's decoder does.
func decodeTo(s model.Session) func([]byte) model.Session {
	return func(raw []byte) model.Session {
		if len(raw) == 0 {
			return model.Session{}
		}
		return s
	}
}

func snapshot(w *world, m *memory, in archive, s model.Session) usecase.Snapshot {
	u := usecase.NewTakeSnapshot(usecase.SnapshotDeps{
		Clock: clock(t0), Sources: w.sources(), Inputs: in, Activities: m, Limits: m, Peers: m, Switches: m,
		Decode: decodeTo(s),
	})
	return u.Execute(context.Background())
}

// titles lists the categories of a snapshot with their bands.
func titles(s usecase.Snapshot) []string {
	out := make([]string, 0, len(s.Groups))
	for _, g := range s.Groups {
		title := g.Title
		if title == "" && len(g.Chips) > 0 {
			title = strings.Fields(g.Chips[0].Text())[1]
		}
		out = append(out, title)
	}
	return out
}

func has(s usecase.Snapshot, part string) bool {
	for _, g := range s.Groups {
		for _, c := range g.Chips {
			if strings.Contains(g.Title+" "+c.Text(), part) {
				return true
			}
		}
	}
	return false
}

func TestSnapshotWritesNothing(t *testing.T) {
	t.Parallel()
	m := newMemory()
	s := session()
	s.Limits = model.RateLimits{FiveHour: model.Some(model.RateWindow{UsedPct: 50, ResetsAt: t0.Add(time.Hour)})}
	w := &world{err: repository.ErrNone, codex: model.CodexLimits{}}
	snapshot(w, m, archive{raw: []byte(`{}`), at: t0.Add(-10 * time.Second)}, s)
	switch {
	case len(m.activities) != 0:
		t.Errorf("an activity was saved: %v", m.activities)
	case !m.limits.Empty() || len(m.history) != 0:
		t.Errorf("limits were remembered (%+v) or recorded (%v): a dashboard must not measure", m.limits, m.history)
	case len(m.claude) != 0 || len(m.codex) != 0:
		t.Errorf("usage was posted for other tools: %+v %+v", m.claude, m.codex)
	case len(m.posts) != 0:
		t.Errorf("the session was posted for the others: %+v", m.posts)
	case m.input != nil || m.runs != nil:
		t.Errorf("the render record was overwritten")
	}
}

func TestSnapshotOfALiveSession(t *testing.T) {
	t.Parallel()
	m := newMemory()
	// 40 % twenty minutes ago and 50 % in the report: 10 % per 19.5 minutes,
	// 100 % in about 1h38m, before the reset in three hours.
	m.history = model.RateHistory{{At: t0.Add(-20 * time.Minute), FiveHour: model.Some(40.0)}}
	m.others = model.Roster{{Key: "other", Name: "another session", At: t0}}
	s := session()
	s.Cost.TotalUSD = model.Some(1.25)
	s.Limits = model.RateLimits{FiveHour: model.Some(model.RateWindow{UsedPct: 50, ResetsAt: t0.Add(3 * time.Hour)})}
	w := &world{err: repository.ErrNone, spend: model.Spend{Today: model.Some(model.Amount{Text: "45.67", Value: 45.67})}}
	w.err = nil
	snap := snapshot(w, m, archive{raw: []byte(`{}`), at: t0.Add(-30 * time.Second)}, s)

	if !snap.Live || !snap.InputAt.Equal(t0.Add(-30*time.Second)) || !snap.At.Equal(t0) {
		t.Errorf("live %v, input at %v, at %v", snap.Live, snap.InputAt, snap.At)
	}
	if !has(snap, "5h 枯渇まで") {
		t.Errorf("a live report is forecast from the history: %v", titles(snap))
	}
	h := snap.Headline
	if pct, _ := h.ContextPct.Get(); pct != 42 {
		t.Errorf("context %v", h.ContextPct)
	}
	if five, _ := h.FiveHour.Get(); five.UsedPct != 50 || h.SevenDay.Present() {
		t.Errorf("windows %+v %+v", h.FiveHour, h.SevenDay)
	}
	if usd, _ := h.SessionUSD.Get(); usd != 1.25 {
		t.Errorf("session cost %v", h.SessionUSD)
	}
	if today, _ := h.TodayUSD.Get(); today != 45.67 {
		t.Errorf("today %v", h.TodayUSD)
	}
	if h.Running != 1 || !has(snap, "another session") {
		t.Errorf("running %d; the other session is listed: %v", h.Running, has(snap, "another session"))
	}
	if len(snap.Bands) != 6 || snap.Bands[0].Name != "ALERTS" {
		t.Errorf("bands %+v", snap.Bands)
	}
	for _, g := range snap.Groups {
		if g.Band < model.BandAlerts || g.Band > model.BandSurroundings {
			t.Errorf("%q has no band: %d", g.Title, g.Band)
		}
	}
}

func TestSnapshotOfAStaleReport(t *testing.T) {
	t.Parallel()
	reported := model.RateLimits{FiveHour: model.Some(model.RateWindow{UsedPct: 50, ResetsAt: t0.Add(time.Hour)})}
	s := session()
	s.Limits = reported
	stale := archive{raw: []byte(`{}`), at: t0.Add(-time.Hour)}

	t.Run("the remembered limits win over a stale report, and nothing is forecast", func(t *testing.T) {
		t.Parallel()
		m := newMemory()
		m.limits, m.limitsAt = model.RateLimits{FiveHour: model.Some(model.RateWindow{UsedPct: 70})}, t0.Add(-5*time.Minute)
		m.history = model.RateHistory{{At: t0.Add(-80 * time.Minute), FiveHour: model.Some(10.0)}}
		snap := snapshot(&world{err: repository.ErrNone}, m, stale, s)
		if snap.Live {
			t.Error("an hour-old report is live")
		}
		if five, _ := snap.Headline.FiveHour.Get(); five.UsedPct != 70 {
			t.Errorf("5h %v, want the remembered 70", snap.Headline.FiveHour)
		}
		if has(snap, "枯渇まで") {
			t.Error("a stale value was forecast")
		}
	})
	t.Run("without remembered limits, the stale report is shown with its time", func(t *testing.T) {
		t.Parallel()
		snap := snapshot(&world{err: repository.ErrNone}, newMemory(), stale, s)
		if five, _ := snap.Headline.FiveHour.Get(); five.UsedPct != 50 {
			t.Errorf("5h %v, want the reported 50", snap.Headline.FiveHour)
		}
		if has(snap, "枯渇まで") {
			t.Error("a stale value was forecast")
		}
	})
	t.Run("a stale report without limits and nothing remembered shows no limits", func(t *testing.T) {
		t.Parallel()
		snap := snapshot(&world{err: repository.ErrNone}, newMemory(), stale, session())
		if snap.Headline.FiveHour.Present() || has(snap, "⚡ Claude") {
			t.Errorf("limits out of nowhere: %v", titles(snap))
		}
	})
	t.Run("week-old remembered limits are not shown", func(t *testing.T) {
		t.Parallel()
		m := newMemory()
		m.limits, m.limitsAt = reported, t0.Add(-8*24*time.Hour)
		if snap := snapshot(&world{err: repository.ErrNone}, m, stale, session()); snap.Headline.FiveHour.Present() {
			t.Errorf("week-old limits: %v", snap.Headline.FiveHour)
		}
	})
}

func TestSnapshotBeforeAnySessionDrew(t *testing.T) {
	t.Parallel()
	snap := snapshot(&world{err: repository.ErrNone}, newMemory(), archive{err: repository.ErrNone}, session())
	if snap.Live || !snap.InputAt.IsZero() || snap.Session.Reported {
		t.Errorf("live %v, input at %v, session %+v", snap.Live, snap.InputAt, snap.Session)
	}
	if snap.Headline.ContextPct.Present() {
		t.Errorf("context out of nowhere: %v", snap.Headline.ContextPct)
	}
	if len(snap.Groups) == 0 {
		t.Error("the machine's facts are shown without a session")
	}
}

func TestSnapshotShowsTheActivityOfTheSession(t *testing.T) {
	t.Parallel()
	m := newMemory()
	var a model.Activity
	a.Observe(42, "p0", t0.Add(-30*time.Minute))
	a.Observe(42, "p1", t0.Add(-time.Minute))
	m.activities["s1"] = a
	snap := snapshot(&world{err: repository.ErrNone}, m, archive{raw: []byte(`{}`), at: t0}, session())
	if !has(snap, "🔥 Burn") {
		t.Errorf("the stored activity is not shown: %v", titles(snap))
	}
}

func TestSnapshotNeedsADecoder(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "Decode") {
			t.Errorf("recovered %v, want a panic naming Decode", r)
		}
	}()
	m := newMemory()
	usecase.NewTakeSnapshot(usecase.SnapshotDeps{
		Clock: clock(t0), Sources: (&world{}).sources(), Inputs: archive{}, Activities: m, Limits: m, Peers: m, Switches: m,
	})
}

// 繰り返し閲覧しても観測時刻や履歴件数を変えず、古い入力を追加しない。
func TestSnapshotReturnsRecordedHistoryWithoutMeasuring(t *testing.T) {
	t.Parallel()
	for _, age := range []time.Duration{time.Second, time.Hour} {
		t.Run(age.String(), func(t *testing.T) {
			t.Parallel()
			m := newMemory()
			observed := t0.Add(-2 * time.Hour)
			m.history = model.RateHistory{{At: observed, FiveHour: model.Some(0.0)}}
			m.activities["s1"] = model.Activity{Samples: []model.Sample{{At: observed, Tokens: 42}}}
			s := session()
			w := &world{codex: model.CodexLimits{History: []model.CodexRatePoint{
				{At: observed, Primary: model.Some(model.CodexWindow{UsedPct: 0, WindowMinutes: 10080})},
			}}}
			s.Limits = model.RateLimits{FiveHour: model.Some(model.RateWindow{UsedPct: 50})}
			for range 2 {
				snap := snapshot(w, m, archive{raw: []byte(`{}`), at: t0.Add(-age)}, s)
				if len(snap.CodexHistory) != 1 || !snap.CodexHistory[0].At.Equal(observed) || snap.CodexHistory[0].Primary.Or(model.CodexWindow{UsedPct: -1}).UsedPct != 0 {
					t.Errorf("Codex の元の観測が変わった: %+v", snap.CodexHistory)
				}
				if len(snap.RateHistory) != 1 || !snap.RateHistory[0].At.Equal(observed) || snap.RateHistory[0].FiveHour.Or(-1) != 0 || snap.RateHistory[0].SevenDay.Present() {
					t.Errorf("記録済みのレート履歴が変わった: %+v", snap.RateHistory)
				}
				if len(snap.ContextHistory) != 1 || !snap.ContextHistory[0].At.Equal(observed) || snap.ContextHistory[0].Tokens != 42 {
					t.Errorf("記録済みのコンテキスト履歴が変わった: %+v", snap.ContextHistory)
				}
			}
			if len(m.history) != 1 || len(m.activities["s1"].Samples) != 1 {
				t.Error("ダッシュボードの閲覧が履歴に保存された")
			}
		})
	}
}
