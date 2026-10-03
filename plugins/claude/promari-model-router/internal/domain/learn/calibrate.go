package learn

import (
	"cmp"
	"math"
	"slices"

	"promari-model-router/internal/domain/model"
)

// Pair is a predicted probability and whether the event happened.
type Pair struct {
	P  float64
	OK bool
}

// FitIsotonic fits a monotone non-decreasing map by pool-adjacent-violators
// (UCCI arXiv:2605.18796 / AutoRelAnnotator arXiv:2606.25871 use isotonic
// calibration once enough labels exist).
func FitIsotonic(pairs []Pair) model.Isotonic {
	sorted := slices.SortedFunc(slices.Values(pairs), func(a, b Pair) int { return cmp.Compare(a.P, b.P) })
	type block struct{ sumX, sumY, n float64 }
	var blocks []block
	for _, p := range sorted {
		y := 0.0
		if p.OK {
			y = 1
		}
		blocks = append(blocks, block{p.P, y, 1})
		for len(blocks) > 1 {
			a, b := blocks[len(blocks)-2], blocks[len(blocks)-1]
			if a.sumY/a.n <= b.sumY/b.n {
				break
			}
			blocks = append(blocks[:len(blocks)-2], block{a.sumX + b.sumX, a.sumY + b.sumY, a.n + b.n})
		}
	}
	iso := model.Isotonic{}
	for _, b := range blocks {
		iso.X = append(iso.X, b.sumX/b.n)
		iso.Y = append(iso.Y, b.sumY/b.n)
	}
	return iso
}

// ECE is the expected calibration error over equal-width bins.
func ECE(pairs []Pair, bins int) float64 {
	if len(pairs) == 0 {
		return 0
	}
	type acc struct{ conf, hit, n float64 }
	b := make([]acc, bins)
	for _, p := range pairs {
		i := min(int(p.P*float64(bins)), bins-1)
		b[i].conf += p.P
		b[i].n++
		if p.OK {
			b[i].hit++
		}
	}
	total := 0.0
	for _, x := range b {
		if x.n > 0 {
			total += x.n * math.Abs(x.hit/x.n-x.conf/x.n)
		}
	}
	return total / float64(len(pairs))
}

// ConformalQuantile is the split-conformal quantile of non-conformity scores
// with the finite-sample correction ⌈(n+1)(1−α)⌉/n (Conformal Cascade,
// arXiv:2607.25018). With too few scores it returns 1 (every set is full,
// so the router holds).
func ConformalQuantile(scores []float64, alpha float64) float64 {
	n := len(scores)
	if n == 0 {
		return 1
	}
	k := int(math.Ceil(float64(n+1) * (1 - alpha)))
	if k > n {
		return 1
	}
	sorted := slices.Sorted(slices.Values(scores))
	return sorted[max(k-1, 0)]
}

// RiskPoint is one calibration example for the downgrade decision: the
// probability that the candidate tier is enough, and whether it really was.
type RiskPoint struct {
	PSafe float64
	Safe  bool
}

// RiskControlledThreshold picks the smallest τ on a grid such that the
// conformal-risk-control bound on the wrong-downgrade rate stays ≤ α:
//
//	(n/(n+1))·R̂(τ) + B/(n+1) ≤ α,  R̂(τ) = mean(1[PSafe ≥ τ and not Safe])
//
// with the loss bounded by B = 1 (Conformal Risk Control, arXiv:2208.02814;
// CR², arXiv:2605.12001). Returns a value above the grid (never downgrade)
// when no τ qualifies.
func RiskControlledThreshold(points []RiskPoint, alpha float64, grid model.Grid) float64 {
	n := float64(len(points))
	never := grid.Stop + grid.Step // above the grid: never downgrade
	if n == 0 {
		return never
	}
	// The grid is built from integer indices (model.Grid.Values): accumulating
	// the step in floating point drifts (0.93 becomes 0.9300000000000004) and
	// silently excludes points that sit exactly on a grid value.
	for _, tau := range grid.Values() {
		wrong := 0.0
		for _, p := range points {
			if p.PSafe >= tau && !p.Safe {
				wrong++
			}
		}
		if n/(n+1)*(wrong/n)+1/(n+1) <= alpha {
			return tau
		}
	}
	return never
}
