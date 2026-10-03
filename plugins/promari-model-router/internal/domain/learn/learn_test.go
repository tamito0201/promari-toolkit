package learn_test

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"promari-model-router/internal/domain/learn"
	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/service"
	"promari-model-router/internal/infrastructure/settings"
)

// approx compares floats up to rounding noise.
var approx = cmpopts.EquateApprox(0, 1e-12)

// fixtures load the shipped TOML (defaults, lexicon, tiers) through the real
// settings loader, with HOME pointed at an empty directory so no user or
// project override applies.
func fixtures(t *testing.T) (*service.Lexicon, model.TierTable, model.Settings) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_PROJECT_DIR", t.TempDir())
	st, _, problems := settings.Load("")
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	return settings.Lexicon(st), settings.Tiers(), st
}

func TestFitIsotonic(t *testing.T) {
	tests := []struct {
		name  string
		pairs []learn.Pair
		want  model.Isotonic
	}{
		{
			name: "no pairs give an empty map",
			want: model.Isotonic{},
		},
		{
			name:  "already monotone stays",
			pairs: []learn.Pair{{0.1, false}, {0.9, true}},
			want:  model.Isotonic{X: []float64{0.1, 0.9}, Y: []float64{0, 1}},
		},
		{
			name:  "a violation is pooled",
			pairs: []learn.Pair{{0.2, true}, {0.4, false}, {0.8, true}},
			want:  model.Isotonic{X: []float64{0.3, 0.8}, Y: []float64{0.5, 1}},
		},
		{
			name:  "unsorted input is sorted first and a cascade pools three",
			pairs: []learn.Pair{{0.3, false}, {0.1, true}, {0.2, true}},
			want:  model.Isotonic{X: []float64{0.2}, Y: []float64{2.0 / 3}},
		},
		{
			name:  "equal means are not pooled",
			pairs: []learn.Pair{{0.1, true}, {0.2, true}},
			want:  model.Isotonic{X: []float64{0.1, 0.2}, Y: []float64{1, 1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, learn.FitIsotonic(tt.pairs), approx); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestECE(t *testing.T) {
	tests := []struct {
		name  string
		pairs []learn.Pair
		bins  int
		want  float64
	}{
		{"no pairs have no error", nil, 10, 0},
		{"a certain hit in the top bin is calibrated", []learn.Pair{{1, true}}, 10, 0},
		{"one bin: accuracy 0.5 against confidence 0.8", []learn.Pair{{0.8, true}, {0.8, false}}, 10, 0.3},
		{"two bins are weighted by their size", []learn.Pair{{0.2, false}, {0.9, true}}, 10, 0.15},
		{"empty bins are skipped", []learn.Pair{{0.05, false}}, 4, 0.05},
		// 0.79 falls in bin 7 and 0.81 in bin 8 of ten: (0.21 + 0.81) / 2.
		{"neighbouring bins are not merged", []learn.Pair{{0.79, true}, {0.81, false}}, 10, 0.51},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, learn.ECE(tt.pairs, tt.bins), approx); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestConformalQuantile(t *testing.T) {
	tests := []struct {
		name   string
		scores []float64
		alpha  float64
		want   float64
	}{
		{"no data holds everything", nil, 0.1, 1},
		{"too few for the level", []float64{0.1, 0.2}, 0.1, 1},
		{"finite-sample corrected", []float64{0.05, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 0.95, 0.99, 0.2, 0.3, 0.1, 0.4, 0.5, 0.6, 0.7, 0.8}, 0.1, 0.95},
		{"alpha one takes the smallest score", []float64{0.3, 0.1, 0.2}, 1, 0.1},
		// k = ceil(10 * 0.9) = 9 = n: the largest score, not "hold everything".
		{"k equal to n takes the largest score", []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9}, 0.1, 0.9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := learn.ConformalQuantile(tt.scores, tt.alpha); got != tt.want {
				t.Errorf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestRiskControlledThreshold(t *testing.T) {
	safe := make([]learn.RiskPoint, 0, 60)
	for i := range 60 {
		safe = append(safe, learn.RiskPoint{PSafe: 0.5 + float64(i)/200, Safe: true})
	}
	// 2 confident mistakes out of 62 is 3.2%: within α=5% even after the CRC
	// correction, so τ may stay low. 6 out of 66 (9%) is not.
	fewWrong := append([]learn.RiskPoint{{PSafe: 0.95, Safe: false}, {PSafe: 0.9, Safe: false}}, safe...)
	manyWrong := append([]learn.RiskPoint{
		{PSafe: 0.95, Safe: false},
		{PSafe: 0.94, Safe: false},
		{PSafe: 0.93, Safe: false},
		{PSafe: 0.92, Safe: false},
		{PSafe: 0.91, Safe: false},
		{PSafe: 0.9, Safe: false},
	}, safe...)
	grid := model.Grid{Start: 0.5, Stop: 1, Step: 0.01} // the grid under test, not the shipped default
	never := grid.Stop + grid.Step
	tests := []struct {
		name   string
		points []learn.RiskPoint
		alpha  float64
		want   float64
	}{
		{"no data never downgrades", nil, 0.05, never},
		{"all safe gives the lowest grid value", safe, 0.05, 0.5},
		{"mistakes within alpha are tolerated", fewWrong, 0.05, 0.5},
		// At τ=0.93 three mistakes remain (bound 0.060 > α); at 0.94 two remain (0.045).
		{"mistakes beyond alpha push tau up to the first safe grid value", manyWrong, 0.05, 0.94},
		// The +1/(n+1) term alone exceeds α with two points: no τ qualifies.
		{"too few points never qualify", []learn.RiskPoint{{0.99, true}, {0.98, true}}, 0.05, never},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, learn.RiskControlledThreshold(tt.points, tt.alpha, grid), approx); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestAUC(t *testing.T) {
	tests := []struct {
		name  string
		pairs []learn.Pair
		want  float64 // math.NaN() when undefined
	}{
		{"perfect separation", []learn.Pair{{0.9, true}, {0.8, true}, {0.2, false}}, 1},
		{"inverted separation", []learn.Pair{{0.1, true}, {0.8, false}}, 0},
		{"ties count half", []learn.Pair{{0.5, true}, {0.5, false}}, 0.5},
		{"mixed", []learn.Pair{{0.9, true}, {0.4, true}, {0.5, false}, {0.4, false}}, 0.625},
		{"no negatives is undefined", []learn.Pair{{0.5, true}}, math.NaN()},
		{"no positives is undefined", []learn.Pair{{0.5, false}}, math.NaN()},
		{"no pairs is undefined", nil, math.NaN()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, learn.AUC(tt.pairs), cmpopts.EquateNaNs()); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestGranularity(t *testing.T) {
	tests := []struct {
		name     string
		scores   []float64
		decimals int
		want     int
	}{
		{"no scores", nil, 4, 0},
		{"values equal after rounding collapse", []float64{0.1, 0.1, 0.2, 0.30000001}, 4, 3},
		{"coarser rounding merges more", []float64{0.11, 0.12, 0.26}, 1, 2},
		{"values round to the nearest, not down", []float64{0.14, 0.16}, 1, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := learn.Granularity(tt.scores, tt.decimals); got != tt.want {
				t.Errorf("granularity = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCollapse(t *testing.T) {
	tests := []struct {
		name  string
		tiers []model.Tier
		want  float64
	}{
		{"no decisions", nil, 0},
		{"one tier dominates", []model.Tier{model.TierHaiku, model.TierHaiku, model.TierOpus, model.TierHaiku}, 0.75},
		{"a single tier has fully collapsed", []model.Tier{model.TierSonnet, model.TierSonnet}, 1},
		{"an even split", []model.Tier{model.TierHaiku, model.TierSonnet}, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := learn.Collapse(tt.tiers); got != tt.want {
				t.Errorf("collapse = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTriageGate(t *testing.T) {
	tests := []struct {
		name                string
		success, ratio, auc float64
		n                   int
		want                bool
	}{
		{"not enough evidence keeps the gate open", 0.1, 0.5, 0.4, 3, true},
		{"exactly min_n is enough evidence", 0.1, 0.5, 0.4, 8, false},
		{"an AUC exactly at the floor passes", 0.9, 0.2, 0.56, 20, true},
		{"cheap tier worse than its cost ratio closes", 0.15, 0.2, 0.9, 20, false},
		{"success equal to the cost ratio closes", 0.2, 0.2, 0.9, 20, false},
		{"weak separation closes", 0.9, 0.2, 0.5, 20, false},
		{"both conditions hold", 0.9, 0.2, 0.7, 20, true},
		{"unknown AUC keeps the gate open", 0.9, 0.2, math.NaN(), 20, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := learn.TriageGate(tt.success, tt.ratio, tt.auc, tt.n, 8, 0.56); got != tt.want {
				t.Errorf("got %v", got)
			}
		})
	}
}

func TestGridValues(t *testing.T) {
	tests := []struct {
		name    string
		grid    model.Grid
		wantLen int
		at      int
		wantAt  float64
	}{
		// 0.5 + 43*0.01 must be exactly 0.93 (no float drift).
		{"grid has no float drift", model.Grid{Start: 0.5, Stop: 1, Step: 0.01}, 51, 43, 0.93},
		{"a zero step is the start alone", model.Grid{Start: 0.7, Stop: 1, Step: 0}, 1, 0, 0.7},
		{"a reversed grid is the start alone", model.Grid{Start: 0.7, Stop: 0.5, Step: 0.1}, 1, 0, 0.7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := tt.grid.Values()
			if len(values) != tt.wantLen || values[tt.at] != tt.wantAt {
				t.Errorf("len=%d values[%d]=%v, want len=%d value=%v", len(values), tt.at, values[tt.at], tt.wantLen, tt.wantAt)
			}
		})
	}
}

func TestFolds(t *testing.T) {
	tests := []struct {
		name string
		ys   []int
		k    int
		want [][]int
	}{
		{
			name: "stratified round robin over sorted labels",
			ys:   []int{0, 0, 0, 1, 1, 1, 2, 2, 2, 2},
			k:    3,
			want: [][]int{{0, 3, 6, 9}, {1, 4, 7}, {2, 5, 8}},
		},
		{
			name: "abstain labels sort first and still fall into folds",
			ys:   []int{1, -1, 0, -1},
			k:    2,
			want: [][]int{{1, 2}, {3, 0}},
		},
		{
			name: "more folds than samples leaves some empty",
			ys:   []int{0},
			k:    3,
			want: [][]int{{0}, nil, nil},
		},
		{
			name: "no samples",
			k:    2,
			want: [][]int{nil, nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			folds := learn.Folds(tt.ys, tt.k)
			if diff := cmp.Diff(tt.want, folds); diff != "" {
				t.Error(diff)
			}
			// Disjoint and covering, whatever the table says.
			all := slices.Sorted(slices.Values(slices.Concat(folds...)))
			indices := make([]int, len(tt.ys))
			for i := range indices {
				indices[i] = i
			}
			if diff := cmp.Diff(indices, all, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("folds are not a partition: %s", diff)
			}
		})
	}
}

func TestTrainSoftmax(t *testing.T) {
	// Two classes, each marked by its own feature; feature 2 never occurs, so
	// its weights must stay exactly zero (the sparse-update skip).
	xs := []service.Vector{
		{Idx: []int32{0}, Val: []float32{1}},
		{Idx: []int32{0}, Val: []float32{1}},
		{Idx: []int32{1}, Val: []float32{1}},
		{Idx: []int32{1}, Val: []float32{1}},
	}
	ys := []int{0, 0, 1, 1}
	opt := model.TrainingSettings{Epochs: 100, LearningRate: 0.1, AdamBeta1: 0.9, AdamBeta2: 0.999, AdamEps: 1e-8}
	tests := []struct {
		name     string
		xs       []service.Vector
		ys       []int
		opt      model.TrainingSettings
		wantPred []int // argmax of Logits for each x in xs
		zeroW    bool  // every weight stays zero
	}{
		{"separable data is learned", xs, ys, opt, []int{0, 0, 1, 1}, false},
		{"zero epochs leave the zero model", xs, ys, model.TrainingSettings{}, []int{0, 0, 0, 0}, true},
		{"no data moves nothing", nil, nil, opt, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := learn.TrainSoftmax(tt.xs, tt.ys, 2, 3, tt.opt)
			if len(m.W) != 2 || len(m.W[0]) != 3 || len(m.B) != 2 {
				t.Fatalf("shape W=%dx%d B=%d", len(m.W), len(m.W[0]), len(m.B))
			}
			var pred []int
			for _, x := range tt.xs {
				pred = append(pred, service.Argmax(m.Logits(x)))
			}
			if diff := cmp.Diff(tt.wantPred, pred); diff != "" {
				t.Error(diff)
			}
			if m.W[0][2] != 0 || m.W[1][2] != 0 {
				t.Errorf("an unused feature moved: %v %v", m.W[0][2], m.W[1][2])
			}
			nonzero := slices.ContainsFunc(m.W, func(w []float64) bool { return slices.ContainsFunc(w, func(v float64) bool { return v != 0 }) })
			if nonzero == tt.zeroW {
				t.Errorf("weights nonzero=%v, want zero=%v", nonzero, tt.zeroW)
			}
		})
	}
}

func TestSoftmaxModelLogits(t *testing.T) {
	m := learn.SoftmaxModel{W: [][]float64{{1, 2}, {-1, 0}}, B: []float64{0.5, -0.5}}
	tests := []struct {
		name string
		x    service.Vector
		want []float64
	}{
		{"empty vector gives the bias", service.Vector{}, []float64{0.5, -0.5}},
		{"weighted sum plus bias", service.Vector{Idx: []int32{0, 1}, Val: []float32{1, 0.5}}, []float64{2.5, -1.5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, m.Logits(tt.x), approx); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestFitTemperature(t *testing.T) {
	grid := model.Grid{Start: 0.25, Stop: 5, Step: 0.05}
	// Logit gap 2·ln3 with 3 of 4 right: NLL is minimal where σ(gap/T)=0.75, i.e. T=2.
	gap := 2 * math.Log(3)
	overconfident := [][]float64{{gap, 0}, {gap, 0}, {gap, 0}, {gap, 0}}
	tests := []struct {
		name   string
		logits [][]float64
		ys     []int
		grid   model.Grid
		want   float64
	}{
		{"over-confident logits are softened", overconfident, []int{0, 0, 0, 1}, grid, 2},
		{"always right sharpens to the lowest grid value", [][]float64{{1, 0}, {0, 1}}, []int{0, 1}, grid, 0.25},
		{"no logits keep the first grid value", nil, nil, grid, 0.25},
		{"a single-value grid", overconfident, []int{0, 0, 0, 1}, model.Grid{Start: 1}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := learn.FitTemperature(tt.logits, tt.ys, tt.grid); got != tt.want {
				t.Errorf("T = %v, want %v", got, tt.want)
			}
		})
	}
}

// trainingSet is a small synthetic labelled set: three prompts per class and
// a few abstain prompts, short and medium length.
var trainingSet = []learn.Sample{
	{Text: "UserService がどこで定義されているか探して", Class: model.ClassLookup},
	{Text: "find all call sites of parseConfig in the repo", Class: model.ClassLookup},
	{Text: "環境変数 DATABASE_URL の参照箇所を教えて", Class: model.ClassLookup},
	{Text: "where is the retry policy defined?", Class: model.ClassLookup},
	{Text: "getUser を fetchUser にリネームして", Class: model.ClassMechanical},
	{Text: "fix the typo in CONTRIBUTING.md", Class: model.ClassMechanical},
	{Text: "bump the version to 2.3.1 and commit", Class: model.ClassMechanical},
	{Text: "このファイルのインデントを整形して", Class: model.ClassMechanical},
	{Text: "この関数の単体テストを書いて", Class: model.ClassStandard},
	{Text: "implement pagination for the orders list endpoint", Class: model.ClassStandard},
	{Text: "CSV エクスポートの機能を追加して", Class: model.ClassStandard},
	{Text: "write tests for the date parsing helper", Class: model.ClassStandard},
	{Text: "CI でたまに落ちるテストの原因を調べて", Class: model.ClassComplex},
	{Text: "find the root cause of the race condition in the job scheduler", Class: model.ClassComplex},
	{Text: "investigate the memory leak in the websocket handler", Class: model.ClassComplex},
	{Text: "並列実行するとデッドロックする問題を解析して", Class: model.ClassComplex},
	{Text: "通知基盤のアーキテクチャを設計して", Class: model.ClassArchitecture},
	{Text: "design the architecture for a multi-tenant billing system", Class: model.ClassArchitecture},
	{Text: "write a migration plan from REST to GraphQL", Class: model.ClassArchitecture},
	{Text: "ログ基盤の技術選定をして", Class: model.ClassArchitecture},
	{Text: "続けて", Class: model.ClassNone},
	{Text: "sounds good", Class: model.ClassNone},
	{Text: "thanks!", Class: model.ClassNone},
	{Text: "このリポジトリで fetchOrders を呼んでいる箇所を一覧にして、それぞれのファイルについて呼び出し元の関数名と引数の渡し方を表にまとめて、重複している呼び出しがあれば印をつけて", Class: model.ClassLookup},
}

func TestTrain(t *testing.T) {
	lex, table, defaults := fixtures(t)
	meta := learn.Meta{Now: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Source: "synthetic"}
	fast := func(mut func(*model.TrainingSettings)) model.Settings {
		st := defaults
		st.Training.Epochs, st.Training.Folds = 40, 3
		mut(&st.Training)
		return st
	}
	tests := []struct {
		name         string
		samples      []learn.Sample
		st           model.Settings
		wantReady    bool
		wantIsotonic bool
		wantGlobal   bool // every bucket uses the global τ
	}{
		{
			name:       "fewer labelled samples than classes leaves the artifact rules-only",
			samples:    []learn.Sample{trainingSet[0], trainingSet[4], trainingSet[20]},
			st:         defaults,
			wantGlobal: true,
		},
		{
			name:         "per-bucket thresholds and isotonic calibration",
			samples:      trainingSet,
			st:           fast(func(o *model.TrainingSettings) { o.MinBucketN, o.IsotonicMinN, o.Triage.MinN = 1, 1, 1 }),
			wantReady:    true,
			wantIsotonic: true,
		},
		{
			name:       "sparse buckets fall back to the global threshold without isotonic",
			samples:    trainingSet,
			st:         fast(func(o *model.TrainingSettings) { o.MinBucketN, o.IsotonicMinN = 1<<20, 1<<20 }),
			wantReady:  true,
			wantGlobal: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			art := learn.Train(tt.samples, lex, table, tt.st, meta)
			// Training is deterministic: the same inputs give the same artifact.
			if diff := cmp.Diff(art, learn.Train(tt.samples, lex, table, tt.st, meta), cmpopts.EquateNaNs()); diff != "" {
				t.Errorf("not deterministic: %s", diff)
			}
			header := model.Artifact{Version: "1", TrainedAt: meta.Now, Samples: len(tt.samples), Source: meta.Source, Features: tt.st.Features, Classes: model.ClassOrder, Alpha: tt.st.Training.Alpha}
			got := model.Artifact{Version: art.Version, TrainedAt: art.TrainedAt, Samples: art.Samples, Source: art.Source, Features: art.Features, Classes: art.Classes, Alpha: art.Alpha}
			if diff := cmp.Diff(header, got); diff != "" {
				t.Error(diff)
			}
			if art.Ready() != tt.wantReady || (art.SafeIsotonic != nil) != tt.wantIsotonic {
				t.Fatalf("ready=%v isotonic=%v", art.Ready(), art.SafeIsotonic != nil)
			}
			if !tt.wantReady {
				if art.Temperature != 1 || art.ConformalQ != 1 || len(art.Tau) != 0 || len(art.Metrics) != 0 || len(art.Gate) != 0 {
					t.Errorf("a rules-only artifact must keep neutral calibration: %+v", art)
				}
				return
			}
			checkTrained(t, art, tt.st, tt.samples, tt.wantGlobal)
		})
	}
}

func checkTrained(t *testing.T, art model.Artifact, st model.Settings, samples []learn.Sample, wantGlobal bool) {
	t.Helper()
	labelled := 0
	for _, s := range samples {
		if s.Class != model.ClassNone {
			labelled++
		}
	}
	dim := service.Dim(st.Features)
	if len(art.Weights) != len(model.ClassOrder) || len(art.Weights[0]) != dim || len(art.Bias) != len(model.ClassOrder) {
		t.Errorf("weights %dx%d bias %d", len(art.Weights), len(art.Weights[0]), len(art.Bias))
	}
	temps := st.Training.TemperatureGrid.Values()
	if art.Temperature < temps[0] || art.Temperature > temps[len(temps)-1] {
		t.Errorf("temperature %v outside its grid", art.Temperature)
	}
	if art.ConformalQ < 0 || art.ConformalQ > 1 {
		t.Errorf("conformal quantile %v", art.ConformalQ)
	}
	global := art.Metrics["tau_global"]
	for _, b := range model.Buckets {
		tau, ok := art.Tau[b]
		if !ok {
			t.Errorf("bucket %s has no tau", b)
		}
		if wantGlobal && tau != global {
			t.Errorf("bucket %s tau %v, want the global %v", b, tau, global)
		}
		if !wantGlobal && tau != global && tau < st.Training.TauFloor {
			t.Errorf("bucket %s tau %v below the floor", b, tau)
		}
	}
	for _, key := range []string{"oof_accuracy", "oof_ece", "psafe_granularity", "tau_global", "risk_points", "temperature", "conformal_q"} {
		if _, ok := art.Metrics[key]; !ok {
			t.Errorf("metric %s missing", key)
		}
	}
	for key, v := range art.Metrics {
		if math.IsNaN(v) {
			t.Errorf("metric %s is NaN", key)
		}
	}
	if art.Metrics["temperature"] != art.Temperature || art.Metrics["conformal_q"] != art.ConformalQ {
		t.Error("metrics disagree with the artifact")
	}
	if len(art.Gate) == 0 {
		t.Error("no class was gated")
	}
	if len(art.Seen) != st.Features.SeenBits/model.BitsPerWord || !slices.ContainsFunc(art.Seen, func(w uint64) bool { return w != 0 }) {
		t.Errorf("seen bitset: %d words", len(art.Seen))
	}
	if len(art.Neighbors) != labelled {
		t.Errorf("neighbors %d, want %d", len(art.Neighbors), labelled)
	}
	if art.MinNeighborSim <= 0 || art.MinNeighborSim > 1+1e-9 {
		t.Errorf("neighbor floor %v", art.MinNeighborSim)
	}
	if art.MaxUnseenRatio != st.Training.MaxUnseenRatio {
		t.Errorf("max unseen ratio %v", art.MaxUnseenRatio)
	}
}

func TestEvaluate(t *testing.T) {
	lex, table, defaults := fixtures(t)
	lookup := "UserService がどこで定義されているか探して"
	cases := []learn.Case{
		{Text: lookup, Expect: model.ClassLookup, Lang: "ja"},
		// Routed to sonnet while complex work needs opus: harmful.
		{Text: "この関数の単体テストを書いて", Expect: model.ClassComplex, Lang: "ja"},
		// Abstains, and abstain was expected: the session tier is kept.
		{Text: "いい感じにして", Expect: model.ClassNone, Lang: "en"},
		// The danger floor keeps the session tier: no leak.
		{Text: "[route: mechanical] 本番 DB の列名をリネームして", Expect: model.ClassMechanical, Danger: true, Lang: "ja"},
		// Labelled dangerous but downgraded: a leak.
		{Text: lookup, Expect: model.ClassLookup, Danger: true, Lang: "ja"},
	}
	cost := defaults.Eval.Cost
	haiku, sonnet, opus := cost(model.TierHaiku), cost(model.TierSonnet), cost(model.TierOpus)
	type result struct {
		Got              model.Class
		Sufficient, Harm bool
		Reason           string
	}
	type summary struct {
		Exact, Injected, Harmful, DangerLeaks int
		ByLang                                map[string][2]int
		Collapse                              float64
		Baselines                             learn.Baselines
		Results                               []result
	}
	tests := []struct {
		name   string
		cases  []learn.Case
		policy func(model.Settings) model.Settings
		want   summary
	}{
		{
			name: "no cases",
			want: summary{ByLang: map[string][2]int{}},
		},
		{
			name:  "enforce mode on mixed cases",
			cases: cases,
			want: summary{
				Exact: 4, Injected: 3, Harmful: 1, DangerLeaks: 1,
				ByLang:   map[string][2]int{"ja": {4, 3}, "en": {1, 1}},
				Collapse: 0.4,
				Baselines: learn.Baselines{
					Router: 0.8, AlwaysStrong: 1, AlwaysCheap: 0.6, Static: 0.8, Oracle: 1,
					RouterCost: (haiku + sonnet + opus + opus + haiku) / 5,
					StrongCost: opus,
					StaticCost: (haiku + sonnet + opus + haiku + haiku) / 5,
					OracleCost: (haiku + opus + opus + haiku + haiku) / 5,
				},
				Results: []result{
					{model.ClassLookup, true, false, "rule:lookup"},
					{model.ClassStandard, false, true, "rule:standard"},
					{model.ClassNone, true, false, "abstain"},
					{model.ClassMechanical, true, false, "danger-keep"},
					{model.ClassLookup, true, false, "rule:lookup"},
				},
			},
		},
		{
			name:  "shadow mode judges the tier it would have chosen but injects nothing",
			cases: cases[:1],
			policy: func(st model.Settings) model.Settings {
				st.Routing.Mode = model.ModeShadow
				return st
			},
			want: summary{
				Exact: 1, ByLang: map[string][2]int{"ja": {1, 1}}, Collapse: 1,
				Baselines: learn.Baselines{
					Router: 1, AlwaysStrong: 1, AlwaysCheap: 1, Static: 1, Oracle: 1,
					RouterCost: opus, StrongCost: opus, StaticCost: haiku, OracleCost: haiku,
				},
				Results: []result{{model.ClassLookup, true, false, "rule:lookup"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := defaults
			st.Routing.Mode = model.ModeEnforce
			if tt.policy != nil {
				st = tt.policy(st)
			}
			rep := learn.Evaluate(tt.cases, service.RouteInput{
				Session:  model.SessionModel{Model: st.Eval.SessionModel, Source: model.SourceExplicit},
				Settings: st, Table: table, Lexicon: lex,
			})
			got := summary{
				Exact: rep.Exact, Injected: rep.Injected, Harmful: rep.Harmful, DangerLeaks: rep.DangerLeaks,
				ByLang: rep.ByLang, Collapse: rep.Collapse, Baselines: rep.Baselines,
			}
			for _, r := range rep.Results {
				got.Results = append(got.Results, result{r.Got, r.Sufficient, r.Harmful, r.Decision.Reason})
			}
			if diff := cmp.Diff(tt.want, got, approx); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestLabelRetries(t *testing.T) {
	t0 := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	run := func(session, sha string, tier model.Tier, at time.Duration) learn.Outcome {
		return learn.Outcome{SessionID: session, PromptSHA: sha, Tier: tier, Success: true, At: t0.Add(at)}
	}
	tests := []struct {
		name     string
		outcomes []learn.Outcome
		want     []bool // Success, in time order
	}{
		{"no outcomes", nil, nil},
		{"a retry on a higher tier fails the first run", []learn.Outcome{run("s", "a", model.TierHaiku, 0), run("s", "a", model.TierOpus, time.Minute)}, []bool{false, true}},
		{"input order does not matter", []learn.Outcome{run("s", "a", model.TierOpus, time.Minute), run("s", "a", model.TierHaiku, 0)}, []bool{false, true}},
		{"a retry on the same tier is not a failure", []learn.Outcome{run("s", "a", model.TierHaiku, 0), run("s", "a", model.TierHaiku, time.Minute)}, []bool{true, true}},
		{"a lower tier later is not a failure", []learn.Outcome{run("s", "a", model.TierOpus, 0), run("s", "a", model.TierHaiku, time.Minute)}, []bool{true, true}},
		{"another session is not a retry", []learn.Outcome{run("s", "a", model.TierHaiku, 0), run("t", "a", model.TierOpus, time.Minute)}, []bool{true, true}},
		{"another brief is not a retry", []learn.Outcome{run("s", "a", model.TierHaiku, 0), run("s", "b", model.TierOpus, time.Minute)}, []bool{true, true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []bool
			for _, o := range learn.LabelRetries(tt.outcomes) {
				got = append(got, o.Success)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestLedgerStats(t *testing.T) {
	t0 := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	lookup := func(sha string, tier model.Tier, ok bool, tokens int, at time.Duration) learn.Outcome {
		return learn.Outcome{SessionID: "s", PromptSHA: sha, Class: model.ClassLookup, Bucket: model.BucketShort, Tier: tier, Success: ok, Tokens: tokens, At: t0.Add(at)}
	}
	tests := []struct {
		name      string
		outcomes  []learn.Outcome
		wantPosts map[string]model.Beta
		wantCosts map[string]float64
	}{
		{"no outcomes", nil, map[string]model.Beta{}, map[string]float64{}},
		{
			name: "a retried brief counts as a failure of the cheaper run",
			outcomes: []learn.Outcome{
				lookup("a", model.TierHaiku, true, 100, 0),
				lookup("a", model.TierOpus, true, 900, time.Minute),
				lookup("b", model.TierHaiku, true, 300, 0),
			},
			wantPosts: map[string]model.Beta{
				model.PosteriorKey(model.ClassLookup, model.BucketShort, model.TierHaiku): {Alpha: 2, Beta: 2},
				model.PosteriorKey(model.ClassLookup, model.BucketShort, model.TierOpus):  {Alpha: 2, Beta: 1},
			},
			wantCosts: map[string]float64{"lookup|haiku": 300, "lookup|opus": 900},
		},
		{
			name: "the median of an odd group and a reported failure",
			outcomes: []learn.Outcome{
				lookup("a", model.TierSonnet, false, 500, 0),
				lookup("b", model.TierSonnet, true, 100, 0),
				lookup("c", model.TierSonnet, true, 200, 0),
			},
			wantPosts: map[string]model.Beta{
				model.PosteriorKey(model.ClassLookup, model.BucketShort, model.TierSonnet): {Alpha: 3, Beta: 2},
			},
			wantCosts: map[string]float64{"lookup|sonnet": 200},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			posts, costs := learn.LedgerStats(tt.outcomes)
			if diff := cmp.Diff(tt.wantPosts, posts); diff != "" {
				t.Error(diff)
			}
			if diff := cmp.Diff(tt.wantCosts, costs); diff != "" {
				t.Error(diff)
			}
		})
	}
}
