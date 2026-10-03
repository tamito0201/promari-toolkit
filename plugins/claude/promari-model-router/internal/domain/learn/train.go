package learn

import (
	"cmp"
	"maps"
	"math"
	"slices"
	"time"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/service"
	"promari-model-router/pkg/fp"
)

// Sample is one labelled prompt. Class ClassNone means "no single class"
// (abstain): such samples never train the classifier but do count as unsafe
// to downgrade during risk calibration.
type Sample struct {
	Text  string
	Class model.Class
}

// Meta describes a training run for the artifact header.
type Meta struct {
	Now    time.Time
	Source string
}

type prepared struct {
	sample model.Class
	vec    service.Vector
	sig    service.Signals
	label  int // index in classes, -1 for abstain
}

// riskRow is one out-of-fold downgrade candidate for risk calibration.
type riskRow struct {
	bucket model.LengthBucket
	point  RiskPoint
	class  model.Class
}

// trainer carries the inputs of one training run; each method is one stage.
type trainer struct {
	st      model.Settings
	table   model.TierTable
	classes []model.Class
	data    []prepared
	dim     int
}

// Train fits the classifier, calibrates it on out-of-fold predictions, and
// returns the artifact with its evaluation metrics. The stages run in order:
// out-of-fold logits → temperature → conformal quantile → risk threshold per
// length bucket → Triage gate → final model → out-of-distribution references.
func Train(samples []Sample, lex *service.Lexicon, table model.TierTable, st model.Settings, meta Meta) model.Artifact {
	t := trainer{st: st, table: table, classes: model.ClassOrder, dim: service.Dim(st.Features)}
	t.data = slices.Collect(fp.Map(slices.Values(samples), func(s Sample) prepared {
		sig := lex.ExtractSignals(s.Text)
		return prepared{sample: s.Class, vec: service.Featurize(sig, st.Features), sig: sig, label: slices.Index(t.classes, s.Class)}
	}))
	art := model.Artifact{
		Version: "1", TrainedAt: meta.Now, Samples: len(samples), Source: meta.Source,
		Features: st.Features, Classes: t.classes, Alpha: st.Training.Alpha, Temperature: 1, ConformalQ: 1,
		Tau: map[model.LengthBucket]float64{}, Metrics: map[string]float64{}, Gate: map[model.Class]bool{},
	}
	if len(t.labelled()) < len(t.classes) {
		return art // not enough data: the artifact stays not Ready, the router uses rules only
	}
	oof := t.outOfFoldLogits()
	topPairs := t.calibrate(&art, oof)
	rows := t.riskRows(art, oof)
	all := slices.Collect(fp.Map(slices.Values(rows), func(r riskRow) RiskPoint { return r.point }))
	global := t.thresholds(&art, rows, all)
	t.gates(&art, rows)
	t.fitFinal(&art)
	t.outOfDistribution(&art)
	t.metrics(&art, topPairs, all, global)
	return art
}

func (t trainer) labelled() []prepared {
	return slices.Collect(fp.Filter(slices.Values(t.data), func(p prepared) bool { return p.label >= 0 }))
}

// outOfFoldLogits predicts every sample with a model that never saw it
// (abstain samples fall into a fold and are predicted by that fold's model).
func (t trainer) outOfFoldLogits() [][]float64 {
	oof := make([][]float64, len(t.data))
	labels := slices.Collect(fp.Map(slices.Values(t.data), func(p prepared) int { return p.label }))
	for _, fold := range Folds(labels, t.st.Training.Folds) {
		held := fp.IndexBy(slices.Values(fold), func(i int) int { return i })
		var xs []service.Vector
		var ys []int
		for i := range t.data {
			if _, out := held[i]; !out && t.data[i].label >= 0 {
				xs, ys = append(xs, t.data[i].vec), append(ys, t.data[i].label)
			}
		}
		m := TrainSoftmax(xs, ys, len(t.classes), t.dim, t.st.Training)
		for _, i := range fold {
			oof[i] = m.Logits(t.data[i].vec)
		}
	}
	return oof
}

// calibrate fits the temperature and the conformal quantile on labelled
// out-of-fold logits and returns the top-class calibration pairs.
func (t trainer) calibrate(art *model.Artifact, oof [][]float64) []Pair {
	var logits [][]float64
	var ys []int
	for i := range t.data {
		if t.data[i].label >= 0 {
			logits, ys = append(logits, oof[i]), append(ys, t.data[i].label)
		}
	}
	art.Temperature = FitTemperature(logits, ys, t.st.Training.TemperatureGrid)
	scores := make([]float64, 0, len(logits))
	pairs := make([]Pair, 0, len(logits))
	for i, z := range logits {
		p := service.Softmax(z, art.Temperature)
		scores = append(scores, 1-p[ys[i]])
		pairs = append(pairs, Pair{P: slices.Max(p), OK: service.Argmax(p) == ys[i]})
	}
	art.ConformalQ = ConformalQuantile(scores, t.st.Training.SetAlpha)
	return pairs
}

// riskRows asks, for each sample: would downgrading to the predicted class's
// tier have been safe? Only downgrades below opus carry a risk to calibrate.
func (t trainer) riskRows(art model.Artifact, oof [][]float64) []riskRow {
	var rows []riskRow
	for i := range t.data {
		p := &t.data[i]
		probs := service.Softmax(oof[i], art.Temperature)
		pred := t.classes[service.Argmax(probs)]
		candidate := t.table.Target(pred)
		if !candidate.Known() || !model.TierOpus.Above(candidate) {
			continue
		}
		needed := model.TierOpus // abstain samples: assume the strong tier is needed
		if p.label >= 0 {
			needed = t.table.Target(t.classes[p.label])
		}
		pSafe := service.SafeProbability(probs, art, t.table, candidate)
		rows = append(rows, riskRow{t.st.Buckets.Of(p.sig.Chars), RiskPoint{PSafe: pSafe, Safe: needed.Rank() <= candidate.Rank()}, pred})
	}
	return rows
}

// thresholds sets the risk-controlled τ per length bucket (Mondrian), falling
// back to the global τ for buckets with too few points; it returns the global τ.
func (t trainer) thresholds(art *model.Artifact, rows []riskRow, all []RiskPoint) float64 {
	opt := t.st.Training
	global := RiskControlledThreshold(all, opt.Alpha, opt.TauGrid)
	byBucket := fp.GroupBy(slices.Values(rows), func(r riskRow) model.LengthBucket { return r.bucket })
	for _, b := range model.Buckets {
		pts := slices.Collect(fp.Map(slices.Values(byBucket[b]), func(r riskRow) RiskPoint { return r.point }))
		art.Tau[b] = global
		if len(pts) >= opt.MinBucketN {
			art.Tau[b] = max(RiskControlledThreshold(pts, opt.Alpha, opt.TauGrid), opt.TauFloor)
		}
	}
	if pairs := riskPairs(all); len(pairs) >= opt.IsotonicMinN {
		iso := FitIsotonic(pairs)
		art.SafeIsotonic = &iso
	}
	return global
}

func riskPairs(points []RiskPoint) []Pair {
	return slices.Collect(fp.Map(slices.Values(points), func(p RiskPoint) Pair { return Pair{P: p.PSafe, OK: p.Safe} }))
}

// gates applies Triage's two conditions per class.
func (t trainer) gates(art *model.Artifact, rows []riskRow) {
	opt := t.st.Training.Triage
	for c, group := range fp.GroupBy(slices.Values(rows), func(r riskRow) model.Class { return r.class }) {
		pairs := riskPairs(slices.Collect(fp.Map(slices.Values(group), func(r riskRow) RiskPoint { return r.point })))
		success := float64(fp.Count(slices.Values(pairs), func(p Pair) bool { return p.OK })) / float64(len(pairs))
		ratio := t.st.Eval.Cost(t.table.Target(c)) / t.st.Eval.Cost(model.TierOpus)
		art.Gate[c] = TriageGate(success, ratio, AUC(pairs), len(pairs), opt.MinN, opt.MinAUC)
	}
}

// fitFinal trains the shipped model on all labelled data.
func (t trainer) fitFinal(art *model.Artifact) {
	labelled := t.labelled()
	final := TrainSoftmax(
		slices.Collect(fp.Map(slices.Values(labelled), func(p prepared) service.Vector { return p.vec })),
		slices.Collect(fp.Map(slices.Values(labelled), func(p prepared) int { return p.label })),
		len(t.classes), t.dim, t.st.Training,
	)
	art.Weights, art.Bias = final.W, final.B
}

// outOfDistribution stores the seen n-grams and the neighbour vectors.
func (t trainer) outOfDistribution(art *model.Artifact) {
	spec := t.st.Features
	labelled := t.labelled()
	art.Seen = make([]uint64, spec.SeenWords())
	for i := range labelled {
		p := &labelled[i]
		for _, g := range service.NGrams(p.sig.Normalised, spec.NGramMax, spec.NGramMax) {
			idx := service.SeenIndex(g, spec.SeenBits)
			art.Seen[idx/model.BitsPerWord] |= 1 << (uint(idx) % model.BitsPerWord)
		}
		art.Neighbors = append(art.Neighbors, model.Neighbor{Idx: p.vec.Idx, Val: p.vec.Val, Class: p.sample})
	}
	art.MinNeighborSim = neighborFloor(labelled, t.st.Training.NeighborFloorQuantile)
	art.MaxUnseenRatio = t.st.Training.MaxUnseenRatio
}

// metrics records the out-of-fold evaluation numbers (NaN values are dropped:
// JSON cannot carry them).
func (t trainer) metrics(art *model.Artifact, topPairs []Pair, all []RiskPoint, global float64) {
	opt := t.st.Training
	art.Metrics["oof_accuracy"] = float64(fp.Count(slices.Values(topPairs), func(p Pair) bool { return p.OK })) / float64(len(topPairs))
	art.Metrics["oof_ece"] = ECE(topPairs, opt.ECEBins)
	art.Metrics["psafe_auc"] = AUC(riskPairs(all))
	art.Metrics["psafe_granularity"] = float64(Granularity(slices.Collect(fp.Map(slices.Values(all), func(p RiskPoint) float64 { return p.PSafe })), opt.GranularityDecimals))
	art.Metrics["tau_global"] = global
	art.Metrics["risk_points"] = float64(len(all))
	art.Metrics["temperature"] = art.Temperature
	art.Metrics["conformal_q"] = art.ConformalQ
	maps.DeleteFunc(art.Metrics, func(_ string, v float64) bool { return math.IsNaN(v) })
}

// neighborFloor is the given quantile of each example's similarity to its
// nearest other example: prompts less similar than that to every training
// example are treated as out of distribution.
func neighborFloor(data []prepared, quantile float64) float64 {
	if len(data) < 2 {
		return 0
	}
	var best []float64
	for i := range data {
		top := 0.0
		for j := range data {
			if i != j {
				top = max(top, service.Cosine(data[i].vec, data[j].vec))
			}
		}
		best = append(best, top)
	}
	slices.Sort(best)
	return best[min(int(quantile*float64(len(best))), len(best)-1)]
}

// Outcome is one finished subagent run joined with its routing decision.
type Outcome struct {
	SessionID string
	PromptSHA string
	Class     model.Class
	Bucket    model.LengthBucket
	Tier      model.Tier
	Success   bool
	Tokens    int
	At        time.Time
}

// LabelRetries marks a run as failed when the same brief was delegated again
// later in the same session at a higher tier (ACQB, arXiv:2602.02061: a retry
// is an implicit failure signal). Credit is per task (TRACE-Router,
// arXiv:2607.22465): the decision and the result are joined by tool_use_id
// before this function is called.
func LabelRetries(outcomes []Outcome) []Outcome {
	out := slices.Clone(outcomes)
	slices.SortStableFunc(out, func(a, b Outcome) int { return a.At.Compare(b.At) })
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j].SessionID == out[i].SessionID && out[j].PromptSHA == out[i].PromptSHA && out[j].Tier.Above(out[i].Tier) {
				out[i].Success = false
				break
			}
		}
	}
	return out
}

// LedgerStats builds Beta(1,1)-prior posteriors per class|bucket|tier and the
// median tokens per class|tier (CADMAS-CTX arXiv:2604.17950; CARROT
// arXiv:2502.03261 predicts cost separately from quality).
func LedgerStats(outcomes []Outcome) (map[string]model.Beta, map[string]float64) {
	posts := map[string]model.Beta{}
	for _, o := range LabelRetries(outcomes) {
		key := model.PosteriorKey(o.Class, o.Bucket, o.Tier)
		post := cmp.Or(posts[key], model.NewBeta())
		posts[key] = post.Observe(o.Success)
	}
	costs := map[string]float64{}
	for key, group := range fp.GroupBy(slices.Values(outcomes), func(o Outcome) string { return model.CostKey(o.Class, o.Tier) }) {
		tokens := slices.Sorted(fp.Map(slices.Values(group), func(o Outcome) float64 { return float64(o.Tokens) }))
		costs[key] = tokens[len(tokens)/2]
	}
	return posts, costs
}
