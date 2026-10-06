package model_test

import (
	"math"
	"slices"
	"testing"

	"promari-statusline/internal/domain/model"
)

func TestDescribeKnownDistributions(t *testing.T) {
	t.Parallel()
	values := []float64{4, 1, 3, 2, math.NaN(), math.Inf(1), -1}
	before := slices.Clone(values)
	d := model.Describe(values)
	if d.N != 4 || d.Mean != 2.5 || d.Min != 1 || d.Max != 4 || d.P50 != 2 || d.P95 != 4 || d.P99 != 4 || math.Abs(d.SD-math.Sqrt(1.25)) > 1e-12 || math.Abs(d.Top10-40) > 1e-12 {
		t.Fatalf("既知分布: %+v", d)
	}
	if !slices.EqualFunc(values, before, func(a, b float64) bool { return a == b || math.IsNaN(a) && math.IsNaN(b) }) {
		t.Fatal("入力を書き換えた")
	}
	for _, v := range [][]float64{nil, {-1, math.NaN()}} {
		if model.Describe(v).N != 0 {
			t.Fatal("欠測を観測に含めた")
		}
	}
	if d := model.Describe([]float64{0, 0}); d.N != 2 || d.Mean != 0 || d.CV != 0 {
		t.Fatalf("ゼロ: %+v", d)
	}
	if d := model.Describe([]float64{7}); d.P50 != 7 || d.SD != 0 || d.Top10 != 100 {
		t.Fatalf("単一標本: %+v", d)
	}
	samples := make([]float64, 100)
	for i := range samples {
		samples[i] = float64(i + 1)
	}
	d = model.Describe(samples)
	if d.P50 != 50 || d.P95 != 95 || d.P99 != 99 || math.Abs(d.Mean-50.5) > 1e-12 || math.Abs(d.Top10-955.0/5050*100) > 1e-12 {
		t.Fatalf("100件の順位: %+v", d)
	}
	for _, v := range [][]float64{{math.MaxFloat64, math.MaxFloat64}, {0, math.MaxFloat64}, {math.SmallestNonzeroFloat64}} {
		d = model.Describe(v)
		for _, n := range []float64{d.Mean, d.SD, d.CV, d.Top10} {
			if math.IsNaN(n) || math.IsInf(n, 0) {
				t.Fatalf("極端値を非有限へ変換: %+v", d)
			}
		}
	}
}

func TestWilsonIntervalsAtBoundaries(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		k, n   int
		lo, hi float64
	}{
		{0, 10, 0, 27.7532799862889}, {10, 10, 72.2467200137111, 100}, {5, 10, 23.659309051264, 76.340690948736},
	} {
		lo, hi, ok := model.Wilson95(c.k, c.n)
		if !ok || math.Abs(lo-c.lo) > 1e-9 || math.Abs(hi-c.hi) > 1e-9 {
			t.Errorf("Wilson(%d,%d)=%v,%v,%v", c.k, c.n, lo, hi, ok)
		}
	}
	for _, c := range [][2]int{{0, 0}, {-1, 10}, {11, 10}, {0, -1}} {
		if _, _, ok := model.Wilson95(c[0], c[1]); ok {
			t.Errorf("不正な分母/件数: %v", c)
		}
	}
}

func TestToolDiversityUniformAndConcentrated(t *testing.T) {
	t.Parallel()
	if got := model.ToolDiversity(nil); got.N != 0 {
		t.Fatal(got)
	}
	uniform := model.ToolDiversity(map[string]int{"a": 2, "b": 2, "c": 2, "d": 2, "missing": 0, "invalid": -1})
	if uniform.N != 8 || uniform.Kinds != 4 || uniform.Entropy != 2 || uniform.Effective != 4 || uniform.Dominance != 25 {
		t.Fatal(uniform)
	}
	single := model.ToolDiversity(map[string]int{"one": 10})
	if single.Entropy != 0 || single.Effective != 1 || single.Dominance != 100 {
		t.Fatal(single)
	}
	skew := model.ToolDiversity(map[string]int{"a": 9, "b": 1})
	if skew.Entropy >= 1 || skew.Effective <= 1 || skew.Effective >= 2 || skew.Dominance != 90 {
		t.Fatal(skew)
	}
}
