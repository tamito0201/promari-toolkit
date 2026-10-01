package service

import (
	"cmp"
	"math"
	"slices"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/pkg/fp"
)

// Softmax with temperature T > 0 (arXiv:2603.18174 replaces independent
// per-class thresholds with one temperature softmax so classes cannot both fire).
func Softmax(logits []float64, t float64) []float64 {
	t = cmp.Or(t, 1)
	peak := slices.Max(logits)
	exps := slices.Collect(fp.Map(slices.Values(logits), func(z float64) float64 { return math.Exp((z - peak) / t) }))
	sum := fp.SumBy(slices.Values(exps), func(v float64) float64 { return v })
	return slices.Collect(fp.Map(slices.Values(exps), func(v float64) float64 { return v / sum }))
}

// Logits are W·x + b for every class.
func Logits(v Vector, a model.Artifact) []float64 {
	out := make([]float64, len(a.Classes))
	for c := range a.Classes {
		out[c] = Dot(v, a.Weights[c]) + a.Bias[c]
	}
	return out
}

// Probabilities are the calibrated class probabilities, indexed like a.Classes.
func Probabilities(v Vector, a model.Artifact) []float64 {
	return Softmax(Logits(v, a), a.Temperature)
}

// Argmax returns the index of the largest value.
func Argmax(p []float64) int {
	best := 0
	for i := range p {
		if p[i] > p[best] {
			best = i
		}
	}
	return best
}

// PredictionSet is the split-conformal set {c : 1 − p(c) ≤ q} (LAC score).
// A set spanning more than one tier means the router cannot tell and should
// hold (Conformal Cascade, arXiv:2607.25018).
func PredictionSet(p []float64, a model.Artifact) []model.Class {
	var set []model.Class
	for i, c := range a.Classes {
		if 1-p[i] <= a.ConformalQ {
			set = append(set, c)
		}
	}
	return set
}

// TiersOf maps classes to their distinct target tiers.
func TiersOf(classes []model.Class, table model.TierTable) []model.Tier {
	return slices.Compact(slices.SortedFunc(fp.Map(slices.Values(classes), table.Target),
		func(a, b model.Tier) int { return cmp.Compare(a.Rank(), b.Rank()) }))
}

// SafeProbability is P(needed tier ≤ candidate): the probability mass of the
// classes whose target tier is at or below the candidate (Harness Tokenomics,
// arXiv:2609.28919). An isotonic map calibrates it when one was learned.
func SafeProbability(p []float64, a model.Artifact, table model.TierTable, candidate model.Tier) float64 {
	mass := 0.0
	for i, c := range a.Classes {
		if t := table.Target(c); t.Known() && t.Rank() <= candidate.Rank() {
			mass += p[i]
		}
	}
	if a.SafeIsotonic != nil {
		return ApplyIsotonic(*a.SafeIsotonic, mass)
	}
	return mass
}

// ApplyIsotonic evaluates a monotone step function with linear interpolation.
func ApplyIsotonic(iso model.Isotonic, x float64) float64 {
	if len(iso.X) == 0 {
		return x
	}
	i, found := slices.BinarySearch(iso.X, x)
	switch {
	case found:
		return iso.Y[i]
	case i == 0:
		return iso.Y[0]
	case i >= len(iso.X):
		return iso.Y[len(iso.Y)-1]
	}
	x0, x1, y0, y1 := iso.X[i-1], iso.X[i], iso.Y[i-1], iso.Y[i]
	return y0 + (y1-y0)*(x-x0)/(x1-x0)
}

// NearestNeighbors returns the k most similar stored examples with similarities.
func NearestNeighbors(v Vector, neighbors []model.Neighbor, k int) []Scored[model.Neighbor] {
	scored := slices.Collect(fp.Map(slices.Values(neighbors), func(n model.Neighbor) Scored[model.Neighbor] {
		return Scored[model.Neighbor]{Item: n, Score: Cosine(v, Vector{Idx: n.Idx, Val: n.Val})}
	}))
	slices.SortStableFunc(scored, func(a, b Scored[model.Neighbor]) int { return cmp.Compare(b.Score, a.Score) })
	return scored[:min(k, len(scored))]
}

// Scored pairs an item with a similarity score.
type Scored[T any] struct {
	Item  T
	Score float64
}
