package model_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/domain/model"
)

func TestBucketSettingsOf(t *testing.T) {
	b := model.BucketSettings{ShortMaxChars: 200, MediumMaxChars: 1000}
	tests := []struct {
		name  string
		chars int
		want  model.LengthBucket
	}{
		{"zero", 0, model.BucketShort},
		{"just below short max", 199, model.BucketShort},
		{"at short max", 200, model.BucketMedium},
		{"just below medium max", 999, model.BucketMedium},
		{"at medium max", 1000, model.BucketLong},
		{"far above", 100000, model.BucketLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, b.Of(tt.chars)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGridValues(t *testing.T) {
	tests := []struct {
		name string
		grid model.Grid
		want []float64
	}{
		{"integer steps", model.Grid{Start: 1, Stop: 3, Step: 1}, []float64{1, 2, 3}},
		{"single point", model.Grid{Start: 2, Stop: 2, Step: 1}, []float64{2}},
		{"zero step yields the start", model.Grid{Start: 0.5, Stop: 1, Step: 0}, []float64{0.5}},
		{"negative step yields the start", model.Grid{Start: 0.5, Stop: 1, Step: -0.1}, []float64{0.5}},
		{"stop before start yields the start", model.Grid{Start: 1, Stop: 0.5, Step: 0.1}, []float64{1}},
		{"rounds to the literal (no drift)", model.Grid{Start: 0.9, Stop: 0.94, Step: 0.01}, []float64{0.9, 0.91, 0.92, 0.93, 0.94}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Exact comparison on purpose: grid values must equal the literals.
			if diff := cmp.Diff(tt.want, tt.grid.Values()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGridValuesLandOnLiterals(t *testing.T) {
	tests := []struct {
		name  string
		grid  model.Grid
		index int
		want  float64
	}{
		// 0.5 + 44*0.01 accumulates to 0.9400000000000001 without rounding.
		{"0.94 from 0.5 by 0.01", model.Grid{Start: 0.5, Stop: 1, Step: 0.01}, 44, 0.94},
		{"last value is the stop", model.Grid{Start: 0.5, Stop: 1, Step: 0.01}, 50, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, tt.grid.Values()[tt.index]); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestEvalSettingsCost(t *testing.T) {
	e := model.EvalSettings{RelativeCost: map[model.Tier]float64{
		model.TierHaiku: 1, model.TierOpus: 5, "unknown": 3,
	}}
	tests := []struct {
		name string
		eval model.EvalSettings
		tier model.Tier
		want float64
	}{
		{"listed tier", e, model.TierHaiku, 1},
		{"another listed tier", e, model.TierOpus, 5},
		{"unlisted tier falls back to unknown", e, model.TierSonnet, 3},
		{"empty tier falls back to unknown", e, model.TierUnknown, 3},
		{"no table is zero", model.EvalSettings{}, model.TierOpus, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, tt.eval.Cost(tt.tier)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValidateLexiconExtra(t *testing.T) {
	tests := []struct {
		name    string
		extra   map[string]model.LexiconExtra
		wantErr string
	}{
		{name: "nil", extra: nil},
		{name: "empty", extra: map[string]model.LexiconExtra{}},
		{
			name: "classes take strong and weak, lists take items",
			extra: map[string]model.LexiconExtra{
				"lookup":       {Strong: []string{"調べて"}, Weak: []string{"探す"}},
				"architecture": {Strong: []string{"設計"}},
				"danger":       {Items: []string{"rm -rf"}},
				"continuation": {Items: []string{"続けて"}},
				"context":      {Items: []string{"さっきの"}},
				"correction":   {Items: []string{"違う"}},
			},
		},
		{
			name:    "flag list with strong",
			extra:   map[string]model.LexiconExtra{"danger": {Strong: []string{"drop"}}},
			wantErr: "lexicon_extra.danger takes `items`, not strong/weak",
		},
		{
			name:    "flag list with weak",
			extra:   map[string]model.LexiconExtra{"context": {Weak: []string{"that"}}},
			wantErr: "lexicon_extra.context takes `items`, not strong/weak",
		},
		{
			name:    "class with items",
			extra:   map[string]model.LexiconExtra{"standard": {Items: []string{"x"}}},
			wantErr: "lexicon_extra.standard takes strong/weak, not `items`",
		},
		{
			name:    "misspelt class",
			extra:   map[string]model.LexiconExtra{"architectur": {Strong: []string{"設計"}}},
			wantErr: "lexicon_extra.architectur: unknown name (classes: [lookup mechanical standard complex architecture]; lists: [danger continuation context correction])",
		},
		{
			name: "reports the first bad name in sorted order",
			extra: map[string]model.LexiconExtra{
				"zzz":    {},
				"danger": {Weak: []string{"x"}},
			},
			wantErr: "lexicon_extra.danger takes `items`, not strong/weak",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := model.Settings{LexiconExtra: tt.extra}.ValidateLexiconExtra()
			got := ""
			if err != nil {
				got = err.Error()
			}
			if diff := cmp.Diff(tt.wantErr, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
