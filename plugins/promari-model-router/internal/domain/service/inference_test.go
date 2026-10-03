package service_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/service"
)

var approx = cmpopts.EquateApprox(0, 1e-4)

func TestSoftmax(t *testing.T) {
	tests := []struct {
		name   string
		logits []float64
		temp   float64
		want   []float64
	}{
		{name: "unit temperature", logits: []float64{2, 1, 0}, temp: 1, want: []float64{0.6652, 0.2447, 0.0900}},
		{name: "a lower temperature sharpens", logits: []float64{2, 1, 0}, temp: 0.25, want: []float64{0.9817, 0.0180, 0.0003}},
		{name: "zero temperature means one", logits: []float64{2, 1, 0}, temp: 0, want: []float64{0.6652, 0.2447, 0.0900}},
		{name: "large logits stay finite", logits: []float64{1000, 1000}, temp: 1, want: []float64{0.5, 0.5}},
		{name: "a single class", logits: []float64{-3}, temp: 1, want: []float64{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, service.Softmax(tt.logits, tt.temp), approx); diff != "" {
				t.Errorf("Softmax() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLogitsAndProbabilities(t *testing.T) {
	a := model.Artifact{
		Classes:     []model.Class{model.ClassLookup, model.ClassComplex},
		Weights:     [][]float64{{1, 0, 2}, {0, 1, 0}},
		Bias:        []float64{0.5, -0.5},
		Temperature: 1,
	}
	tests := []struct {
		name       string
		v          service.Vector
		wantLogits []float64
		wantProbs  []float64
	}{
		{
			name:       "weights times features plus bias",
			v:          service.Vector{Idx: []int32{0, 2}, Val: []float32{1, 1}},
			wantLogits: []float64{3.5, -0.5},
			wantProbs:  []float64{0.9820, 0.0180},
		},
		{
			name:       "the zero vector leaves the bias",
			v:          service.Vector{},
			wantLogits: []float64{0.5, -0.5},
			wantProbs:  []float64{0.7311, 0.2689},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.wantLogits, service.Logits(tt.v, a), approx); diff != "" {
				t.Errorf("Logits() mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantProbs, service.Probabilities(tt.v, a), approx); diff != "" {
				t.Errorf("Probabilities() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestArgmax(t *testing.T) {
	tests := []struct {
		name string
		p    []float64
		want int
	}{
		{name: "empty", p: nil, want: 0},
		{name: "largest in the middle", p: []float64{0.1, 0.7, 0.2}, want: 1},
		{name: "largest last", p: []float64{0.1, 0.2, 0.7}, want: 2},
		{name: "a tie keeps the first", p: []float64{0.4, 0.4, 0.2}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, service.Argmax(tt.p)); diff != "" {
				t.Errorf("Argmax() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestPredictionSet(t *testing.T) {
	classes := []model.Class{model.ClassLookup, model.ClassStandard, model.ClassComplex}
	p := []float64{0.6652, 0.2447, 0.0900}
	tests := []struct {
		name string
		q    float64
		want []model.Class
	}{
		{name: "a confident set", q: 0.5, want: []model.Class{model.ClassLookup}},
		{name: "a wider set", q: 0.8, want: []model.Class{model.ClassLookup, model.ClassStandard}},
		{name: "everything", q: 1, want: classes},
		{name: "nothing clears a strict quantile", q: 0.1, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := model.Artifact{Classes: classes, ConformalQ: tt.q}
			if diff := cmp.Diff(tt.want, service.PredictionSet(p, a)); diff != "" {
				t.Errorf("PredictionSet() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTiersOf(t *testing.T) {
	_, table, _ := fixtures(t)
	tests := []struct {
		name    string
		classes []model.Class
		want    []model.Tier
	}{
		{name: "none", classes: nil, want: nil},
		{name: "one tier for two cheap classes", classes: []model.Class{model.ClassMechanical, model.ClassLookup}, want: []model.Tier{model.TierHaiku}},
		{
			name:    "sorted cheapest first and deduplicated",
			classes: []model.Class{model.ClassArchitecture, model.ClassLookup, model.ClassComplex, model.ClassStandard},
			want:    []model.Tier{model.TierHaiku, model.TierSonnet, model.TierOpus},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, service.TiersOf(tt.classes, table), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("TiersOf() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSafeProbability(t *testing.T) {
	_, table, _ := fixtures(t)
	uniform := []float64{0.2, 0.2, 0.2, 0.2, 0.2}
	a := model.Artifact{Classes: model.ClassOrder}
	tests := []struct {
		name      string
		artifact  model.Artifact
		table     model.TierTable
		candidate model.Tier
		want      float64
	}{
		{name: "haiku covers the two cheap classes", artifact: a, table: table, candidate: model.TierHaiku, want: 0.4},
		{name: "sonnet adds standard", artifact: a, table: table, candidate: model.TierSonnet, want: 0.6},
		{name: "opus covers everything", artifact: a, table: table, candidate: model.TierOpus, want: 1},
		{name: "an unknown candidate covers nothing", artifact: a, table: table, candidate: model.TierUnknown, want: 0},
		{
			name: "a class without a tier never counts", artifact: a,
			table: withoutClass(model.ClassLookup)(table), candidate: model.TierOpus, want: 0.8,
		},
		{
			name: "an isotonic map calibrates the mass",
			artifact: model.Artifact{
				Classes:      model.ClassOrder,
				SafeIsotonic: &model.Isotonic{X: []float64{0, 1}, Y: []float64{0, 0.5}},
			},
			table: table, candidate: model.TierHaiku, want: 0.2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, service.SafeProbability(uniform, tt.artifact, tt.table, tt.candidate), approx); diff != "" {
				t.Errorf("SafeProbability() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestApplyIsotonic(t *testing.T) {
	iso := model.Isotonic{X: []float64{0.2, 0.4, 0.8}, Y: []float64{0.1, 0.5, 0.9}}
	tests := []struct {
		name string
		iso  model.Isotonic
		x    float64
		want float64
	}{
		{name: "no map is the identity", iso: model.Isotonic{}, x: 0.37, want: 0.37},
		{name: "on a knot", iso: iso, x: 0.4, want: 0.5},
		{name: "below the first knot", iso: iso, x: 0.1, want: 0.1},
		{name: "above the last knot", iso: iso, x: 0.95, want: 0.9},
		{name: "between knots interpolates", iso: iso, x: 0.6, want: 0.7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, service.ApplyIsotonic(tt.iso, tt.x), approx); diff != "" {
				t.Errorf("ApplyIsotonic() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNearestNeighbors(t *testing.T) {
	near := model.Neighbor{Idx: []int32{1}, Val: []float32{1}, Class: model.ClassLookup}
	half := model.Neighbor{Idx: []int32{1, 2}, Val: []float32{0.5, 0.5}, Class: model.ClassStandard}
	far := model.Neighbor{Idx: []int32{3}, Val: []float32{1}, Class: model.ClassComplex}
	v := service.Vector{Idx: []int32{1}, Val: []float32{1}}
	type got struct {
		Class model.Class
		Score float64
	}
	tests := []struct {
		name      string
		neighbors []model.Neighbor
		k         int
		want      []got
	}{
		{name: "no neighbours", neighbors: nil, k: 3, want: nil},
		{name: "most similar first", neighbors: []model.Neighbor{far, half, near}, k: 2, want: []got{{model.ClassLookup, 1}, {model.ClassStandard, 0.5}}},
		{name: "k larger than the set", neighbors: []model.Neighbor{far, near}, k: 5, want: []got{{model.ClassLookup, 1}, {model.ClassComplex, 0}}},
		{name: "ties keep their stored order", neighbors: []model.Neighbor{far, {Idx: []int32{4}, Val: []float32{1}, Class: model.ClassMechanical}}, k: 2, want: []got{{model.ClassComplex, 0}, {model.ClassMechanical, 0}}},
		{name: "k zero", neighbors: []model.Neighbor{near}, k: 0, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out []got
			for _, s := range service.NearestNeighbors(v, tt.neighbors, tt.k) {
				out = append(out, got{s.Item.Class, s.Score})
			}
			if diff := cmp.Diff(tt.want, out, cmpopts.EquateEmpty(), approx); diff != "" {
				t.Errorf("NearestNeighbors() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
