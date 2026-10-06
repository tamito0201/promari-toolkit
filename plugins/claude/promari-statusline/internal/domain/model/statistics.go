package model

import (
	"math"
	"slices"
)

const (
	// P95Samples はp95の上側に少なくとも1観測を置く表示条件。精度保証ではない。
	P95Samples = 20
	// P99Samples はp99の上側に少なくとも1観測を置く表示条件。精度保証ではない。
	P99Samples   = 100
	percentile95 = 0.95
	percentile99 = 0.99
	upperShare   = 0.1
	wilsonZ      = 1.959963984540054
)

// Distribution は有限・非負の実測標本の記述統計。母集団の保証値ではない。
type Distribution struct {
	N                                            int
	Mean, Min, Max, P50, P95, P99, SD, CV, Top10 float64
}

// Describe は入力を変更せず、nearest-rank 分位点と母分散型の標本内ばらつきを返す。
// Dean/Barroso (2013) の裾の監視を、直近の観測窓へ適用する。
func Describe(values []float64) Distribution {
	clean := make([]float64, 0, len(values))
	for _, v := range values {
		if v >= 0 && !math.IsNaN(v) && !math.IsInf(v, 0) {
			clean = append(clean, v)
		}
	}
	d := Distribution{N: len(clean)}
	if d.N == 0 {
		return d
	}
	slices.Sort(clean)
	d.Min, d.Max = clean[0], clean[d.N-1]
	// 先に最大値で正規化し、極端な値の二乗と総和によるオーバーフローを避ける。
	if d.Max > 0 {
		scaled := 0.0
		for _, v := range clean {
			scaled += v / d.Max
		}
		mean := scaled / float64(d.N)
		variance := 0.0
		for _, v := range clean {
			delta := v/d.Max - mean
			variance += delta * delta
		}
		sd := math.Sqrt(variance / float64(d.N))
		d.Mean, d.SD, d.CV = mean*d.Max, sd*d.Max, sd/mean
		top := int(math.Ceil(float64(d.N) * upperShare))
		for _, v := range clean[d.N-top:] {
			d.Top10 += (v / d.Max) / scaled * percent
		}
	}
	d.P50, d.P95, d.P99 = quantile(clean, median), quantile(clean, percentile95), quantile(clean, percentile99)
	return d
}

// Wilson95 は独立ベルヌーイ試行を仮定した名目95%区間。連続したツール呼び出しでは
// 独立性を保証できず、タスク成功確率や将来のエラー率の保証には使えない。
func Wilson95(events, trials int) (low, high float64, ok bool) {
	if trials <= 0 || events < 0 || events > trials {
		return 0, 0, false
	}
	n, p := float64(trials), float64(events)/float64(trials)
	z2 := wilsonZ * wilsonZ
	denominator := 1 + z2/n
	center := (p + z2/(2*n)) / denominator
	margin := wilsonZ * math.Sqrt(p*(1-p)/n+z2/(4*n*n)) / denominator
	return max(0, center-margin) * percent, min(1, center+margin) * percent, true
}

// Diversity はツールの種類と利用頻度の偏り。多様性の高さを品質とみなさない。
type Diversity struct {
	N, Kinds                      int
	Entropy, Effective, Dominance float64
}

// ToolDiversity は Shannon の H（bit）、Hill の 2^H、最大占有率を返す。
func ToolDiversity(counts map[string]int) Diversity {
	d := Diversity{}
	largest := 0
	for _, n := range counts {
		if n > 0 {
			d.N += n
			d.Kinds++
			largest = max(largest, n)
		}
	}
	if d.N == 0 {
		return d
	}
	for _, n := range counts {
		if n > 0 {
			p := float64(n) / float64(d.N)
			d.Entropy -= p * math.Log2(p)
		}
	}
	d.Effective, d.Dominance = math.Exp2(d.Entropy), float64(largest)/float64(d.N)*percent
	return d
}
