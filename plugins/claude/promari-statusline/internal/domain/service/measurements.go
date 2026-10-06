package service

import (
	"time"

	"promari-statusline/internal/domain/model"
)

// Measurement は表示文に依存しない集計値。未観測は値を持たず、実測ゼロと区別する。
// 値の根拠（観測範囲・件数・表示しない理由）は Basis に構造のまま持ち、文言は表示側が組み立てる。
type Measurement struct {
	Value model.Optional[float64]
	Basis model.Basis
}

// Measurements は画面の全数値を同じ事実から集計する。観測や保存は行わない。
func Measurements(v *View) map[string]Measurement {
	m := measurements{}
	m.money(v)
	m.activity(v)
	m.transcript(v)
	if t, ok := v.Facts.Transcript.Get(); ok {
		m.research(&t)
	}
	m.git(v)
	m.machine(v)
	if todo, ok := v.Facts.Todos.Get(); ok {
		m.number("todoDone", float64(todo.Done))
		m.number("todoTotal", float64(todo.Total))
	}
	if hit, ok := v.Session.Cache.HitRatio.Get(); ok {
		m.number("cacheHit", hit*percent)
		m.number("cacheSaved", hit*cacheSaving)
		m.number("cacheWrite", v.Session.Cache.WriteTokens)
	}
	return m
}

type measurements map[string]Measurement

func (m measurements) number(key string, value float64) {
	m[key] = Measurement{Value: model.Some(value)}
}

func (m measurements) ratio(key string, numerator, denominator float64) {
	if denominator > 0 {
		m.number(key, numerator/denominator)
	}
}

func (m measurements) money(v *View) {
	cost := v.Session.Cost
	if total, ok := cost.TotalUSD.Get(); ok {
		m.number("sessionCost", total)
		m.ratio("sessionBurn", total, cost.Wall.Hours())
		m.ratio("costLine", total, float64(cost.LinesAdded))
		if a, ok := v.Activity.Get(); ok {
			m.ratio("costTurn", total, float64(a.Turns))
		}
	}
	if cost.Wall > 0 {
		m.number("wall", cost.Wall.Seconds())
		m.ratio("parallel", cost.API.Seconds(), cost.Wall.Seconds())
	}
	if spend, ok := v.Facts.Spend.Get(); ok {
		for key, value := range map[string]model.Optional[model.Amount]{
			"todayCost": spend.Today, "blockCost": spend.Block, "burnRate": spend.BurnPerHour,
		} {
			if amount, known := value.Get(); known {
				m.number(key, amount.Value)
			}
		}
		if estimate, known := spend.EstimatedBlock(); known {
			m.number("blockEstimate", estimate)
		}
		if spend.Inactive {
			for _, key := range []string{"blockCost", "blockEstimate", "burnRate"} {
				m[key] = Measurement{Basis: model.Basis{Kind: model.BasisNoBlock}}
			}
		}
	}
}

func (m measurements) activity(v *View) {
	a, ok := v.Activity.Get()
	if !ok || a.LastSeen.IsZero() {
		return
	}
	m.number("active", a.WorkedSeconds)
	m.number("idle", a.IdledSeconds)
	m.number("turns", float64(a.Turns))
	m.number("deep", a.Deep(activityTime(&a, v.Now)).Seconds())
	m.number("longest", a.Longest(activityTime(&a, v.Now)).Seconds())
	if observed := a.Worked() + a.Idled(); observed > focusMinObserved {
		m.ratio("focus", a.WorkedSeconds*percent, observed.Seconds())
	}
	if a.Worked() > linesMinWorked {
		m.ratio("linesHour", float64(v.Session.Cost.LinesAdded), a.Worked().Hours())
	}
}

// 閲覧時刻まで作業時間を伸ばさず、最後の実測で止める。
func activityTime(a *model.Activity, now time.Time) time.Time {
	if !a.LastSeen.IsZero() && a.LastSeen.Before(now) {
		return a.LastSeen
	}
	return now
}

func (m measurements) transcript(v *View) {
	t, ok := v.Facts.Transcript.Get()
	if !ok {
		return
	}
	for key, value := range map[string]float64{
		"inputTokens": t.Tokens.AllInput(), "outputTokens": t.Tokens.Output,
		"requests": float64(t.Requests), "tools": float64(t.Tools.Total),
		"edited": float64(len(t.Files)), "prompts": float64(t.Prompts), "hooks": float64(t.Hooks),
		"untested": float64(len(t.Quality.Unverified)), "tests": float64(t.Quality.Tests.Runs),
		"builds": float64(t.Quality.Builds.Runs), "explores": float64(t.Trace.Explores),
		"edits": float64(t.Quality.Edits),
	} {
		m.number(key, value)
	}
	m.ratio("toolErrors", float64(t.Tools.Errors)*percent, float64(t.Tools.Total))
	m.ratio("thinking", t.Tokens.Thinking*percent, t.Tokens.Output)
	m.ratio("autonomy", float64(t.Tools.Total), float64(t.Prompts))
	m.ratio("intervention", float64(t.Interrupts+t.Denials)*percent, float64(t.Prompts))
	m.ratio("exploreEdit", float64(t.Trace.Explores), float64(t.Quality.Edits))
	if times, known := t.TurnTimes(); known {
		m.number("turnP50", times.P50.Seconds())
		m.number("turnP90", times.P90.Seconds())
	}
}

func (m measurements) git(v *View) {
	g, ok := v.Facts.Git.Get()
	if !ok {
		return
	}
	for key, value := range map[string]int{
		"changed": g.Changed, "staged": g.Staged, "inserted": g.Inserted, "deleted": g.Deleted,
		"rules": g.Rules.Total(), "rulesUnchecked": g.RulesUnchecked,
		"commitStreak": g.Streak, "conventional": g.ConventionalToday, "commitsToday": g.CommitsToday,
	} {
		m.number(key, float64(value))
	}
}

func (m measurements) machine(v *View) {
	host := v.Facts.Machine
	for key, value := range map[string]model.Optional[float64]{
		"cpuLoad": host.Load, "freeMemory": host.FreeMemory, "freeDisk": host.FreeDisk,
	} {
		if n, ok := value.Get(); ok {
			m.number(key, n)
		}
	}
	if host.CPUs > 0 {
		m.number("cores", float64(host.CPUs))
	}
	if battery, ok := host.Battery.Get(); ok {
		m.number("battery", float64(battery.Percent))
	}
}
