package service

import (
	"math"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
)

func TestMeasurementsKeepZeroAndDoNotInventMissingFacts(t *testing.T) {
	t.Parallel()
	if got := Measurements(&View{}); len(got) != 0 {
		t.Fatalf("未観測の指標を生成した: %+v", got)
	}
	v := &View{Facts: model.Facts{
		Git: model.Some(model.Git{}), Transcript: model.Some(model.Transcript{}), Todos: model.Some(model.Todos{}),
	}}
	m := Measurements(v)
	for _, key := range []string{"changed", "staged", "inserted", "deleted", "rules", "rulesUnchecked", "edited", "untested", "tools", "hooks", "tests", "builds", "todoDone", "todoTotal"} {
		if n, ok := m[key].Value.Get(); !ok || n != 0 {
			t.Errorf("実測ゼロ %s = %+v", key, m[key])
		}
	}
	for _, key := range []string{"costLine", "focus", "linesHour", "toolErrors", "turnP50", "turnP90", "thinking", "autonomy", "intervention", "exploreEdit"} {
		if m[key].Value.Present() {
			t.Errorf("分母または観測のない %s を算出した", key)
		}
	}
}

func TestMeasurementsCalculateFromFacts(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 6, 3, 0, 0, 0, time.UTC)
	v := &View{Now: now, Session: model.Session{
		Cost:  model.Cost{TotalUSD: model.Some(12.0), Wall: 2 * time.Hour, API: time.Hour, LinesAdded: 60},
		Cache: model.PromptCache{HitRatio: model.Some(0.5), WriteTokens: 100},
	}, Activity: model.Some(model.Activity{
		LastSeen: now, StreakStart: now.Add(-time.Hour), WorkedSeconds: 3600, IdledSeconds: 3600, Turns: 3,
	}), Facts: model.Facts{
		Spend: model.Some(model.Spend{Today: model.Some(model.Amount{Value: 20}), Block: model.Some(model.Amount{Value: 10}), BurnPerHour: model.Some(model.Amount{Value: 2}), BlockLeft: time.Hour}),
		Transcript: model.Some(model.Transcript{
			Tokens: model.TokenTotals{Input: 10, CacheWrite: 20, CacheRead: 70, Output: 40, Thinking: 10},
			Tools:  model.ToolStats{Total: 8, Errors: 2}, Prompts: 4, Denials: 1, Interrupts: 1,
			Turns: []float64{1, 2, 3, 4, 5}, Trace: model.Trace{Explores: 6}, Quality: model.Quality{Edits: 2},
		}),
		Machine: model.Machine{Load: model.Some(1.5), CPUs: 8, FreeMemory: model.Some(1024.0), FreeDisk: model.Some(2048.0), Battery: model.Some(model.Battery{Percent: 90})},
	}}
	want := map[string]float64{
		"sessionCost": 12, "sessionBurn": 6, "costLine": 0.2, "costTurn": 4, "wall": 7200, "parallel": 0.5,
		"active": 3600, "idle": 3600, "deep": 3600, "longest": 3600, "turns": 3, "focus": 50, "linesHour": 60,
		"todayCost": 20, "blockCost": 10, "blockEstimate": 12, "burnRate": 2,
		"cacheHit": 50, "cacheSaved": 45, "cacheWrite": 100, "inputTokens": 100, "outputTokens": 40,
		"toolErrors": 25, "thinking": 25, "autonomy": 2, "intervention": 50, "exploreEdit": 3,
		"turnP50": 3, "turnP90": 5, "cpuLoad": 1.5, "cores": 8, "freeMemory": 1024, "freeDisk": 2048, "battery": 90,
	}
	m := Measurements(v)
	for key, expected := range want {
		if actual, ok := m[key].Value.Get(); !ok || math.Abs(actual-expected) > 1e-9 {
			t.Errorf("%s = %+v, want %v", key, m[key], expected)
		}
	}
	// 追加行が0でも、十分な稼働時間を観測した生産性は実測0。
	v.Session.Cost.LinesAdded = 0
	v.Session.Cost.TotalUSD = model.Some(0.0)
	m = Measurements(v)
	if m["linesHour"].Value.Or(-1) != 0 || m["costTurn"].Value.Or(-1) != 0 || m["costLine"].Value.Present() {
		t.Errorf("ゼロと未定義の区別が崩れた: %+v", m)
	}
}

func TestMeasurementsWaitForEnoughActivityAndFreezeAtObservation(t *testing.T) {
	t.Parallel()
	observed := time.Date(2026, 10, 6, 3, 0, 0, 0, time.UTC)
	for _, worked := range []time.Duration{0, time.Minute, 15 * time.Minute, 16 * time.Minute} {
		a := model.Activity{LastSeen: observed, StreakStart: observed.Add(-worked), WorkedSeconds: worked.Seconds()}
		v := &View{Now: observed.Add(3 * time.Hour), Activity: model.Some(a)}
		m := Measurements(v)
		if m["focus"].Value.Present() != (worked > focusMinObserved) || m["linesHour"].Value.Present() != (worked > linesMinWorked) {
			t.Errorf("観測時間 %s の最小条件が崩れた", worked)
		}
		if m["longest"].Value.Or(-1) != worked.Seconds() || m["deep"].Value.Or(-1) != 0 {
			t.Errorf("閲覧中に作業時間を増やした: %+v", m)
		}
	}
	if got := activityTime(&model.Activity{}, observed); !got.Equal(observed) {
		t.Errorf("観測時刻なし: %s", got)
	}
	if m := Measurements(&View{Activity: model.Some(model.Activity{})}); m["active"].Value.Present() {
		t.Error("未記録の活動を実測0として扱った")
	}
}

func TestMeasurementsDistinguishInactiveBillingBlock(t *testing.T) {
	t.Parallel()
	m := Measurements(&View{Facts: model.Facts{Spend: model.Some(model.Spend{Inactive: true})}})
	for _, key := range []string{"blockCost", "blockEstimate", "burnRate"} {
		if m[key].Value.Present() || m[key].Basis.Kind != model.BasisNoBlock {
			t.Errorf("稼働枠なしを費用0と混同した: %s = %+v", key, m[key])
		}
	}
}
