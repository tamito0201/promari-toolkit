package usecase

import (
	"context"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/domain/service"
)

// freshFor is how long a recorded session report still describes a session at
// work. A report older than this is shown, but the limits it carries are not
// made into a forecast: a stale value is not a new measurement.
const freshFor = 2 * time.Minute

// SnapshotDeps is what TakeSnapshot depends on. It reads what the status line
// reads and writes nothing of the status line's own: no activity, no
// remembered limits, no post for the other sessions.
type SnapshotDeps struct {
	Clock      repository.Clock
	Sources    Sources
	Inputs     repository.InputArchive
	Activities repository.ActivityStore
	Limits     repository.RateLimitMemory
	Peers      repository.PeerBoard
	Switches   repository.Switches
	// Decode turns a recorded report into a session, as the status line does.
	Decode func(raw []byte) model.Session
}

// TakeSnapshot builds the dashboard: every category of the status line, for
// the session that drew last, without the layout into lines.
type TakeSnapshot struct {
	deps SnapshotDeps
}

// NewTakeSnapshot returns the use case. It panics when a dependency is missing.
func NewTakeSnapshot(deps SnapshotDeps) *TakeSnapshot {
	mustBeWired("SnapshotDeps", deps)
	return &TakeSnapshot{deps: deps}
}

// Headline holds the few numbers the dashboard shows large. A number that is
// not known is absent, never zero.
type Headline struct {
	ContextPct model.Optional[float64]
	FiveHour   model.Optional[model.RateWindow]
	SevenDay   model.Optional[model.RateWindow]
	SessionUSD model.Optional[float64]
	TodayUSD   model.Optional[float64]
	Running    int
}

// Snapshot is the dashboard at one moment.
type Snapshot struct {
	At time.Time
	// InputAt is when the session report was recorded; zero when no session
	// has drawn yet.
	InputAt time.Time
	// Live is true while the report is recent enough to describe a session at
	// work.
	Live     bool
	Session  model.Session
	Headline Headline
	Bands    []Band
	Groups   []model.Group
	// グラフは記録済みの観測だけを返す。閲覧を新しい測定にしない。
	RateHistory    model.RateHistory
	CodexHistory   []model.CodexRatePoint
	ContextHistory []model.Sample
	LimitsAt       time.Time
	Measurements   map[string]Measurement
}

// Band names a band of the status line for a reader: a short English name and
// the question the band answers.
type Band struct {
	Band     model.Band
	Name     string
	Question string
}

// Measurement is one number of the dashboard. A number that was not observed
// holds no value, apart from a zero that was; Basis says what the value rests
// on, or why there is none.
type Measurement struct {
	Value model.Optional[float64]
	Basis model.Basis
}

// bands copies the bands of the domain into the use case's terms.
func bands() []Band {
	infos := service.Bands()
	out := make([]Band, 0, len(infos))
	for _, b := range infos {
		out = append(out, Band{Band: b.Band, Name: b.Name, Question: b.Question})
	}
	return out
}

// measurements copies the numbers of the domain into the use case's terms.
func measurements(v *service.View) map[string]Measurement {
	numbers := service.Measurements(v)
	out := make(map[string]Measurement, len(numbers))
	for key, m := range numbers {
		out[key] = Measurement{Value: m.Value, Basis: m.Basis}
	}
	return out
}

// Execute takes the snapshot. It never fails: what cannot be read is left out.
func (u *TakeSnapshot) Execute(ctx context.Context) Snapshot {
	d := u.deps
	now := d.Clock.Now()
	raw, inputAt, err := d.Inputs.LastInput()
	if err != nil {
		raw, inputAt = nil, time.Time{}
	}
	session := d.Decode(raw)
	live := !inputAt.IsZero() && now.Sub(inputAt) < freshFor

	view := service.View{Now: now, Session: session, AlarmAll: d.Switches.BlinkDemo()}
	if usage, ok := session.Context.Usage(); ok {
		view.Usage = model.Some(usage)
		if key, ok := session.Key(); ok {
			view.Activity = model.Some(d.Activities.Load(key))
		}
	}
	view.Facts, _ = collector{sources: d.Sources, clock: d.Clock}.gather(ctx, RenderRequest{Session: session, Raw: raw})
	account := view.Facts.Account.Or("")
	view.Limits, view.LimitsSeen, view.Forecasts = u.limits(account, session.Limits, inputAt, live, now)
	roster := d.Peers.Roster().Of(account)
	key, _ := session.Key()
	view.Peers, view.Running = roster.Others(key), len(roster)

	return Snapshot{
		At:             now,
		InputAt:        inputAt,
		Live:           live,
		Session:        session,
		Headline:       headline(&view),
		Bands:          bands(),
		Groups:         service.Compose(&view),
		RateHistory:    d.Limits.History(account),
		CodexHistory:   view.Facts.Codex.Or(model.CodexLimits{}).History,
		ContextHistory: view.Activity.Or(model.Activity{}).Samples,
		LimitsAt:       view.LimitsSeen,
		Measurements:   measurements(&view),
	}
}

// limits returns the rate limits to show: those of the recorded report while
// it is fresh, with a forecast from the remembered history and the report
// (added at the time it was measured, and not saved); otherwise the remembered
// ones with the time they were seen, and no forecast.
func (u *TakeSnapshot) limits(account string, reported model.RateLimits, inputAt time.Time, live bool, now time.Time) (model.RateLimits, time.Time, []model.Forecast) {
	memory := u.deps.Limits
	if !reported.Empty() && live {
		return reported, time.Time{}, memory.History(account).Record(reported, inputAt).Forecasts(reported, now)
	}
	last, seen, err := memory.Last(account)
	if err != nil || now.Sub(seen) >= limitsKept {
		if !reported.Empty() {
			return reported, inputAt, nil
		}
		return model.RateLimits{}, time.Time{}, nil
	}
	return last, seen, nil
}

// headline picks the numbers shown large from a view.
func headline(v *service.View) Headline {
	h := Headline{SessionUSD: v.Session.Cost.TotalUSD, Running: v.Running}
	if usage, ok := v.Usage.Get(); ok {
		h.ContextPct = model.Some(usage.Pct)
	}
	h.FiveHour, h.SevenDay = v.Limits.FiveHour, v.Limits.SevenDay
	if spend, ok := v.Facts.Spend.Get(); ok {
		if today, ok := spend.Today.Get(); ok {
			h.TodayUSD = model.Some(today.Value)
		}
	}
	return h
}
