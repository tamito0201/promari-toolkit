package model_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
)

func TestBeta(t *testing.T) {
	type result struct {
		Mean, Variance float64
	}
	tests := []struct {
		name string
		beta model.Beta
		want result
	}{
		{"uniform prior", model.Beta{Alpha: 1, Beta: 1}, result{Mean: 0.5, Variance: 1.0 / 12}},
		{"skewed", model.Beta{Alpha: 3, Beta: 1}, result{Mean: 0.75, Variance: 3.0 / (16 * 5)}},
		{"after one success", model.Beta{Alpha: 1, Beta: 1}.Observe(true), result{Mean: 2.0 / 3, Variance: 2.0 / (9 * 4)}},
		{"after one failure", model.Beta{Alpha: 1, Beta: 1}.Observe(false), result{Mean: 1.0 / 3, Variance: 2.0 / (9 * 4)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := result{tt.beta.Mean(), tt.beta.Variance()}
			if diff := cmp.Diff(tt.want, got, cmpopts.EquateApprox(0, 1e-12)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBetaObserveIsImmutable(t *testing.T) {
	tests := []struct {
		name    string
		success bool
		want    model.Beta
	}{
		{"success increments alpha", true, model.Beta{Alpha: 3, Beta: 5}},
		{"failure increments beta", false, model.Beta{Alpha: 2, Beta: 6}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prior := model.Beta{Alpha: 2, Beta: 5}
			got := prior.Observe(tt.success)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("posterior mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(model.Beta{Alpha: 2, Beta: 5}, prior); diff != "" {
				t.Errorf("Observe changed the prior (-want +got):\n%s", diff)
			}
		})
	}
}

func TestKeys(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"posterior key", model.PosteriorKey(model.ClassLookup, model.BucketShort, model.TierHaiku), "lookup|short|haiku"},
		{"posterior key with abstain", model.PosteriorKey(model.ClassNone, model.BucketLong, model.TierUnknown), "|long|"},
		{"cost key", model.CostKey(model.ClassComplex, model.TierOpus), "complex|opus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, tt.got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestArtifactReadyTrusted(t *testing.T) {
	spec := model.FeatureSpec{HashBuckets: 2, SeenBits: 100}
	row := func() []float64 { return make([]float64, spec.Dim()) }
	ready := model.Artifact{
		Classes:  []model.Class{model.ClassLookup, model.ClassComplex},
		Weights:  [][]float64{row(), row()},
		Bias:     []float64{0, 0},
		Features: spec,
	}
	with := func(f func(*model.Artifact)) model.Artifact {
		a := ready
		f(&a)
		return a
	}
	tests := []struct {
		name        string
		artifact    model.Artifact
		wantErr     string
		wantTrusted bool
	}{
		{"zero artifact", model.Artifact{}, "no classes", false},
		{"ready without origin", ready, "", false},
		{"ready and local", with(func(a *model.Artifact) { a.Origin = model.OriginLocal }), "", true},
		{"ready but embedded", with(func(a *model.Artifact) { a.Origin = model.OriginEmbedded }), "", false},
		{"weights and classes disagree", with(func(a *model.Artifact) { a.Origin = model.OriginLocal; a.Weights = a.Weights[:1] }), "1 weight rows for 2 classes", false},
		{"no classes", with(func(a *model.Artifact) { a.Origin = model.OriginLocal; a.Classes, a.Weights = nil, nil }), "no classes", false},
		{"no hash buckets", with(func(a *model.Artifact) { a.Origin = model.OriginLocal; a.Features.HashBuckets = 0 }), "no hash buckets", false},
		{"a bias missing", with(func(a *model.Artifact) { a.Bias = a.Bias[:1] }), "1 biases for 2 classes", false},
		{"a short weight row", with(func(a *model.Artifact) { a.Weights = [][]float64{row(), {1}} }), "weight row 1 has 1 values, want 12", false},
		{"a seen bitset of the right size", with(func(a *model.Artifact) { a.Seen = make([]uint64, 2) }), "", false},
		{"a seen bitset too short", with(func(a *model.Artifact) { a.Seen = make([]uint64, 1) }), "seen bitset of 1 words for seen_bits 100", false},
		{"a seen bitset without seen bits", with(func(a *model.Artifact) { a.Seen, a.Features.SeenBits = make([]uint64, 2), 0 }), "seen bitset of 2 words for seen_bits 0", false},
		{"an uneven isotonic map", with(func(a *model.Artifact) { a.SafeIsotonic = &model.Isotonic{X: []float64{0, 1}, Y: []float64{0}} }), "isotonic map with 2 x and 1 y", false},
		{"an even isotonic map", with(func(a *model.Artifact) { a.SafeIsotonic = &model.Isotonic{X: []float64{0, 1}, Y: []float64{0, 1}} }), "", false},
		{"a neighbour in range", with(func(a *model.Artifact) { a.Neighbors = []model.Neighbor{{Idx: []int32{0, 11}, Val: []float32{1, 1}}} }), "", false},
		{"a neighbour with uneven vectors", with(func(a *model.Artifact) { a.Neighbors = []model.Neighbor{{Idx: []int32{0}}} }), "neighbour 0 has 1 indices and 0 values", false},
		{"a neighbour index past the end", with(func(a *model.Artifact) { a.Neighbors = []model.Neighbor{{Idx: []int32{12}, Val: []float32{1}}} }), "neighbour 0 has index 12 outside [0, 12)", false},
		{"a negative neighbour index", with(func(a *model.Artifact) { a.Neighbors = []model.Neighbor{{Idx: []int32{-1}, Val: []float32{1}}} }), "neighbour 0 has index -1 outside [0, 12)", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type result struct {
				Err            string
				Ready, Trusted bool
			}
			gotErr := ""
			if err := tt.artifact.Validate(); err != nil {
				gotErr = err.Error()
			}
			want := result{tt.wantErr, tt.wantErr == "", tt.wantTrusted}
			if diff := cmp.Diff(want, result{gotErr, tt.artifact.Ready(), tt.artifact.Trusted()}); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFeatureSpecSizes(t *testing.T) {
	tests := []struct {
		name             string
		spec             model.FeatureSpec
		wantDim, wantWds int
	}{
		{"zero", model.FeatureSpec{}, model.DenseFeatures, 0},
		{"one bit is one word", model.FeatureSpec{HashBuckets: 4, SeenBits: 1}, 4 + model.DenseFeatures, 1},
		{"a whole word", model.FeatureSpec{SeenBits: 64}, model.DenseFeatures, 1},
		{"one bit over a word", model.FeatureSpec{SeenBits: 65}, model.DenseFeatures, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff([]int{tt.wantDim, tt.wantWds}, []int{tt.spec.Dim(), tt.spec.SeenWords()}); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}
