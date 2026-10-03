package learn

import (
	"maps"
	"math"
	"slices"

	"promari-model-router/internal/domain/model"
	"promari-model-router/pkg/fp"
)

// AUC is the area under the ROC curve by the Mann–Whitney statistic: the
// probability that a random positive scores above a random negative.
func AUC(pairs []Pair) float64 {
	pos := slices.Collect(fp.Filter(slices.Values(pairs), func(p Pair) bool { return p.OK }))
	neg := slices.Collect(fp.Filter(slices.Values(pairs), func(p Pair) bool { return !p.OK }))
	if len(pos) == 0 || len(neg) == 0 {
		return math.NaN()
	}
	wins := 0.0
	for _, a := range pos {
		for _, b := range neg {
			switch {
			case a.P > b.P:
				wins++
			case a.P == b.P:
				wins += 0.5
			}
		}
	}
	return wins / float64(len(pos)*len(neg))
}

// Granularity is the number of distinct scores: a router whose score takes
// only a few values cannot place a threshold finely (Score Granularity Gap,
// arXiv:2606.22179).
func Granularity(scores []float64, decimals int) int {
	scale := math.Pow(10, float64(decimals))
	rounded := slices.Collect(fp.Map(slices.Values(scores), func(v float64) float64 { return math.Round(v*scale) / scale }))
	return len(slices.Compact(slices.Sorted(slices.Values(rounded))))
}

// Collapse is the share of decisions that go to the single most frequent
// tier; a router that always picks one tier has collapsed (EquiRouter,
// arXiv:2602.03478).
func Collapse(tiers []model.Tier) float64 {
	if len(tiers) == 0 {
		return 0
	}
	counts := fp.CountBy(slices.Values(tiers), func(t model.Tier) model.Tier { return t })
	top := slices.Max(slices.Collect(maps.Values(counts)))
	return float64(top) / float64(len(tiers))
}

// TriageGate reports whether downgrading a class is justified by the two
// falsifiable conditions of Triage (arXiv:2604.07494): the cheap tier's
// success rate exceeds the cost ratio (cheap/strong), and the score separates
// successes from failures (AUC ≥ min_auc, 0.56 in the paper). Unknown AUC keeps the gate open only
// when the success condition holds with at least minN observations.
func TriageGate(successRate, costRatio, auc float64, n, minN int, minAUC float64) bool {
	if n < minN {
		return true // not enough evidence to close the gate
	}
	if successRate <= costRatio {
		return false
	}
	return math.IsNaN(auc) || auc >= minAUC
}

// Baselines compares a router's tier choices with simple references
// (LLMRouterBench, arXiv:2601.07206): always the strongest tier, always the
// cheapest, the static class table, and the oracle (cheapest sufficient tier).
type Baselines struct {
	Router, AlwaysStrong, AlwaysCheap, Static, Oracle float64 // share of cases whose tier is sufficient
	RouterCost, StrongCost, StaticCost, OracleCost    float64 // mean relative cost units
}
