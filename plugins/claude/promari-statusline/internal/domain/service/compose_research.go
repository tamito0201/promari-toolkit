package service

import (
	"time"

	"promari-statusline/internal/domain/model"
)

// unit は研究由来の指標の単位。チップの書式を決め、契約ファイル
// （contracts/research-measurements.json）では名前で書かれる。
type unit uint8

const (
	unitNumber unit = iota
	unitSeconds
	unitPercent
	unitRatio
	unitDecimal
)

// String は契約ファイルでの単位の名前を返す。
func (u unit) String() string {
	var name string
	switch u {
	case unitNumber:
		name = "number"
	case unitSeconds:
		name = "seconds"
	case unitPercent:
		name = "percent"
	case unitRatio:
		name = "ratio"
	case unitDecimal:
		name = "decimal"
	}
	return name
}

// researchField は指標1件: 集計のキー、チップのラベル、単位。
type researchField struct {
	key, label string
	unit       unit
}

// researchSection は研究由来の指標の分類1つ: 契約ファイルでの分類名と、表示順の指標。
type researchSection struct {
	category string
	fields   []researchField
}

var (
	latencySection = researchSection{"Latency", []researchField{
		{"turnSamples", "Samples", unitNumber},
		{"turnMean", "Mean", unitSeconds},
		{"turnMin", "Min", unitSeconds},
		{"turnMax", "Max", unitSeconds},
		{"turnP95", "P95", unitSeconds},
		{"turnP99", "P99", unitSeconds},
		{"turnSD", "StdDev", unitSeconds},
		{"turnCV", "CV", unitRatio},
		{"turnTail", "Tail", unitRatio},
	}}
	toolsSection = researchSection{"Tools", []researchField{
		{"observedCalls", "Calls", unitNumber},
		{"toolCompleted", "Results", unitNumber},
		{"toolFailed", "Errors", unitNumber},
		{"toolSkipped", "Skipped", unitNumber},
		{"toolPending", "Pending", unitNumber},
		{"toolUnpaired", "Unpaired", unitNumber},
		{"toolEvicted", "Evicted", unitNumber},
		{"toolMissingID", "MissingID", unitNumber},
		{"toolUntimed", "Untimed", unitNumber},
		{"toolTimeSamples", "Timed", unitNumber},
		{"toolTimeMean", "RoundMean", unitSeconds},
		{"toolTimeP50", "RoundP50", unitSeconds},
		{"toolTimeP95", "RoundP95", unitSeconds},
		{"toolTimeMax", "RoundMax", unitSeconds},
		// MainErr は主会話の対応済み結果に対するエラー率。全ツール呼び出しに対する
		// ErrRate（compose_money.go の perf の分類）とは分母が違うので名前を分ける。
		{"toolErrorRate", "MainErr", unitPercent},
		{"toolErrorLow", "WilsonLow", unitPercent},
		{"toolErrorHigh", "WilsonHigh", unitPercent},
	}}
	diversitySection = researchSection{"Diversity", []researchField{
		{"toolKinds", "Kinds", unitNumber},
		{"toolEntropy", "Entropy", unitDecimal},
		{"toolEffective", "Effective", unitDecimal},
		{"toolDominance", "Dominant", unitPercent},
		{"toolTransitions", "Transitions", unitNumber},
		{"toolSwitch", "Switch", unitPercent},
	}}
	budgetSection = researchSection{"Budget", []researchField{
		{"promptSamples", "Samples", unitNumber},
		{"promptMean", "MeanTok", unitNumber},
		{"promptMedian", "P50Tok", unitNumber},
		{"promptP95", "P95Tok", unitNumber},
		{"promptMax", "MaxTok", unitNumber},
		{"promptCV", "TokenCV", unitRatio},
		{"promptTop10", "Top10", unitPercent},
		{"inputPerRequest", "InputReq", unitNumber},
		{"outputPerRequest", "OutputReq", unitNumber},
		{"sessionCacheShare", "Cached", unitPercent},
	}}
	evidenceSection = researchSection{"Evidence", []researchField{
		{"testDecided", "TestKnown", unitNumber},
		{"testUnknown", "TestUnknown", unitNumber},
		{"testPassRate", "TestPass", unitPercent},
		{"buildUnknown", "BuildUnknown", unitNumber},
		{"buildPassRate", "BuildPass", unitPercent},
		{"hookErrorRate", "HookError", unitPercent},
		{"editErrorRate", "EditError", unitPercent},
	}}
	// researchSections は研究由来の指標の全分類。契約ファイルはこの順に並ぶ。
	researchSections = []researchSection{latencySection, toolsSection, diversitySection, budgetSection, evidenceSection}
)

// chips は研究由来の記述統計を、実測値があるときだけ表示する。
// 論文由来の合否閾値は作らず、全指標を中立色で扱う。
func (s researchSection) chips(v *View) []model.Chip {
	t, ok := v.Facts.Transcript.Get()
	if !ok || t.Cursor.Format != model.TranscriptFormat {
		return nil
	}
	m := measurements{}
	m.research(&t)
	var chips []model.Chip
	for _, f := range s.fields {
		if value, known := m[f.key].Value.Get(); known {
			chips = append(chips, chip(model.ToneInfo, f.label+" "+researchValue(value, f.unit)))
		}
	}
	return chips
}

func researchValue(value float64, u unit) string {
	var text string
	switch u {
	case unitSeconds:
		if value < 1 {
			text = fixed(value*float64(time.Second/time.Millisecond), 1) + "ms"
		} else {
			text = brief(time.Duration(value * float64(time.Second)))
		}
	case unitPercent:
		text = fixed(value, 1) + "%"
	case unitRatio:
		text = "×" + fixed(value, 2)
	case unitDecimal:
		text = fixed(value, 2)
	case unitNumber:
		text = tokens(value)
	}
	return text
}

// research は現在の形式で読んだ会話記録だけを集計する。旧形式の集計には研究由来の
// 項目が無く、ゼロと区別できないため、読み直されるまで何も出さない。
func (m measurements) research(t *model.Transcript) {
	if t.Cursor.Format != model.TranscriptFormat {
		return
	}
	m.turnDistribution(t)
	o := t.Observations.View()
	m.toolObservations(&o)
	m.diversity(&o)
	m.tokenDistribution(t)
	m.evidence(t)
}

// observed は観測範囲 basis から算出した値を記録する。
func (m measurements) observed(key string, value float64, basis model.Basis) {
	basis.Kind = model.BasisObserved
	m[key] = Measurement{Value: model.Some(value), Basis: basis}
}

// waiting は観測が minimum 件に満たない値を、件数だけ記録する。
func (m measurements) waiting(key string, n, minimum int) {
	m[key] = Measurement{Basis: model.Basis{Kind: model.BasisWaiting, N: n, Required: minimum}}
}

func (m measurements) turnDistribution(t *model.Transcript) {
	d := model.Describe(t.Turns)
	m.number("turnSamples", float64(d.N))
	if d.N == 0 {
		return
	}
	recent := model.Basis{Scope: model.ScopeRecentTurns, N: d.N, Window: model.MaxTurnSamples}
	for key, value := range map[string]float64{"turnMean": d.Mean, "turnMin": d.Min, "turnMax": d.Max, "turnSD": d.SD} {
		m.observed(key, value, recent)
	}
	m.tailMetrics("turnP95", "turnP99", d, model.MaxTurnSamples)
	if d.N >= 2 && d.Mean > 0 {
		m.observed("turnCV", d.CV, model.Basis{Scope: model.ScopeTurnSpread, N: d.N})
	}
	if d.N >= model.P95Samples && d.P50 > 0 {
		m.observed("turnTail", d.P95/d.P50, model.Basis{Scope: model.ScopeTailRatio, N: d.N})
	}
}

// tailMetrics は裾の分位点を、観測が足りるときだけ記録する。window は観測窓の上限件数。
// p99key が空なら p95 だけを記録する。
func (m measurements) tailMetrics(p95key, p99key string, d model.Distribution, window int) {
	ranked := model.Basis{Scope: model.ScopeNearestRank, N: d.N, Window: window}
	m.waiting(p95key, d.N, model.P95Samples)
	if d.N >= model.P95Samples {
		m.observed(p95key, d.P95, ranked)
	}
	if p99key == "" {
		return
	}
	m.waiting(p99key, d.N, model.P99Samples)
	if d.N >= model.P99Samples {
		m.observed(p99key, d.P99, ranked)
	}
}

func (m measurements) toolObservations(o *model.ToolObservationsView) {
	for key, n := range map[string]int{
		"observedCalls": o.Calls, "toolCompleted": o.Completed, "toolFailed": o.Failed,
		"toolSkipped": o.Skipped, "toolPending": o.Pending, "toolUnpaired": o.Unpaired,
		"toolEvicted": o.Evicted, "toolMissingID": o.MissingID, "toolUntimed": o.Untimed,
	} {
		m.number(key, float64(n))
	}
	if low, high, ok := model.Wilson95(o.Failed, o.Completed); ok {
		paired := model.Basis{Scope: model.ScopePairedResults, N: o.Completed}
		for key, value := range map[string]float64{"toolErrorRate": float64(o.Failed) / float64(o.Completed) * percent, "toolErrorLow": low, "toolErrorHigh": high} {
			m.observed(key, value, paired)
		}
	}
	d := model.Describe(o.Seconds)
	m.number("toolTimeSamples", float64(d.N))
	if d.N == 0 {
		return
	}
	trips := model.Basis{Scope: model.ScopeRoundTrips, N: d.N, Window: model.MaxTurnSamples}
	for key, value := range map[string]float64{"toolTimeMean": d.Mean, "toolTimeP50": d.P50, "toolTimeMax": d.Max} {
		m.observed(key, value, trips)
	}
	m.tailMetrics("toolTimeP95", "", d, model.MaxTurnSamples)
}

func (m measurements) diversity(o *model.ToolObservationsView) {
	m.number("toolTransitions", float64(o.Transitions))
	if o.Transitions > 0 {
		m.observed("toolSwitch", float64(o.Switches)/float64(o.Transitions)*percent, model.Basis{Scope: model.ScopeAdjacentCalls, N: o.Transitions})
	}
	if o.OtherNames > 0 {
		for _, key := range []string{"toolKinds", "toolEntropy", "toolEffective", "toolDominance"} {
			m[key] = Measurement{Basis: model.Basis{Kind: model.BasisTooManyKinds, Limit: model.ToolKindsLimit}}
		}
		return
	}
	d := model.ToolDiversity(o.Names)
	if d.N == 0 {
		return
	}
	named := model.Basis{Scope: model.ScopeNamedCalls, N: d.N}
	for key, value := range map[string]float64{"toolKinds": float64(d.Kinds), "toolEntropy": d.Entropy, "toolEffective": d.Effective, "toolDominance": d.Dominance} {
		m.observed(key, value, named)
	}
}

// topShareMinimum は上位10%の占有率を出す最少の区間数: 10件で上位1件になる。
const topShareMinimum = 10

func (m measurements) tokenDistribution(t *model.Transcript) {
	d := model.Describe(t.Trace.TurnTokens)
	m.number("promptSamples", float64(d.N))
	if d.N > 0 {
		closed := model.Basis{Scope: model.ScopeClosedPrompts, N: d.N, Window: model.MaxTurnTokensKept}
		for key, value := range map[string]float64{"promptMean": d.Mean, "promptMedian": d.P50, "promptMax": d.Max} {
			m.observed(key, value, closed)
		}
		m.tailMetrics("promptP95", "", d, model.MaxTurnTokensKept)
		if d.N >= 2 && d.Mean > 0 {
			m.observed("promptCV", d.CV, model.Basis{Scope: model.ScopePromptSpread, N: d.N})
		}
		if d.N >= topShareMinimum && d.Mean > 0 {
			m.observed("promptTop10", d.Top10, model.Basis{Scope: model.ScopeTopShare, N: d.N})
		}
	}
	if t.UsageRequests > 0 {
		responses := model.Basis{Scope: model.ScopeUsageResponses, N: t.UsageRequests}
		m.observed("inputPerRequest", t.Tokens.AllInput()/float64(t.UsageRequests), responses)
		m.observed("outputPerRequest", t.Tokens.Output/float64(t.UsageRequests), responses)
	}
	if share, ok := t.Tokens.CachedShare(); ok {
		m.observed("sessionCacheShare", share, model.Basis{Scope: model.ScopeAllInput, N: t.UsageRequests})
	}
}

func (m measurements) evidence(t *model.Transcript) {
	q := t.Quality
	known := q.Tests.Passed + q.Tests.Failed
	m.number("testDecided", float64(known))
	m.number("testUnknown", float64(q.Tests.Runs-known))
	m.number("buildUnknown", float64(q.Builds.Runs-q.Builds.Passed-q.Builds.Failed))
	for _, r := range []struct {
		key           string
		events, total int
	}{
		{"testPassRate", q.Tests.Passed, known},
		{"buildPassRate", q.Builds.Passed, q.Builds.Passed + q.Builds.Failed},
		{"hookErrorRate", t.HookErrors, t.Hooks},
		{"editErrorRate", q.EditFailures, q.Edits},
	} {
		if r.total > 0 {
			m.observed(r.key, float64(r.events)/float64(r.total)*percent, model.Basis{Scope: model.ScopeRecognizedRuns, N: r.total})
		}
	}
}
