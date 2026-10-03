package model_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/domain/model"
)

func TestParseClass(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want model.Class
	}{
		{"lookup", "lookup", model.ClassLookup},
		{"mechanical", "mechanical", model.ClassMechanical},
		{"standard", "standard", model.ClassStandard},
		{"complex", "complex", model.ClassComplex},
		{"architecture", "architecture", model.ClassArchitecture},
		{"unknown name abstains", "architectur", model.ClassNone},
		{"case sensitive", "Lookup", model.ClassNone},
		{"empty", "", model.ClassNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, model.ParseClass(tt.in)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClassPredicates(t *testing.T) {
	type result struct {
		Rank               int
		Abstained          bool
		Cheap              bool
		Deep               bool
		DowngradeSensitive bool
		String             string
	}
	tests := []struct {
		name  string
		class model.Class
		want  result
	}{
		{"none", model.ClassNone, result{Rank: -1, Abstained: true}},
		{"lookup", model.ClassLookup, result{Rank: 0, Cheap: true, DowngradeSensitive: true, String: "lookup"}},
		{"mechanical", model.ClassMechanical, result{Rank: 1, Cheap: true, DowngradeSensitive: true, String: "mechanical"}},
		{"standard", model.ClassStandard, result{Rank: 2, DowngradeSensitive: true, String: "standard"}},
		{"complex", model.ClassComplex, result{Rank: 3, Deep: true, String: "complex"}},
		{"architecture", model.ClassArchitecture, result{Rank: 4, Deep: true, String: "architecture"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.class
			got := result{c.Rank(), c.Abstained(), c.Cheap(), c.Deep(), c.DowngradeSensitive(), c.String()}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTierTableTarget(t *testing.T) {
	table := model.TierTable{
		Ceiling: model.TierOpus,
		Classes: map[model.Class]model.TierSpec{
			model.ClassLookup:       {Model: model.TierHaiku},
			model.ClassStandard:     {Model: model.TierSonnet},
			model.ClassComplex:      {Model: model.TierOpus},
			model.ClassArchitecture: {Model: model.TierFable},
			model.ClassMechanical:   {Model: model.Tier("inherit")},
		},
	}
	tests := []struct {
		name  string
		table model.TierTable
		class model.Class
		want  model.Tier
	}{
		{"below the ceiling", table, model.ClassLookup, model.TierHaiku},
		{"at the ceiling", table, model.ClassComplex, model.TierOpus},
		{"capped at the ceiling", table, model.ClassArchitecture, model.TierOpus},
		{"unknown model in the table", table, model.ClassMechanical, model.TierUnknown},
		{"class missing from the table", table, model.ClassNone, model.TierUnknown},
		{
			"no ceiling keeps the spec",
			model.TierTable{Classes: map[model.Class]model.TierSpec{model.ClassArchitecture: {Model: model.TierFable}}},
			model.ClassArchitecture, model.TierFable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, tt.table.Target(tt.class)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTierTableIsBuiltinAgent(t *testing.T) {
	table := model.TierTable{BuiltinAgents: []string{"general-purpose", "Explore"}}
	tests := []struct {
		name  string
		table model.TierTable
		agent string
		want  bool
	}{
		{"listed", table, "Explore", true},
		{"not listed", table, "scout", false},
		{"case sensitive", table, "explore", false},
		{"empty table", model.TierTable{}, "Explore", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, tt.table.IsBuiltinAgent(tt.agent)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClassificationScore(t *testing.T) {
	c := model.Classification{Scores: map[model.Class]int{model.ClassLookup: 5, model.ClassComplex: 2}}
	tests := []struct {
		name  string
		c     model.Classification
		class model.Class
		want  int
	}{
		{"present", c, model.ClassLookup, 5},
		{"another present", c, model.ClassComplex, 2},
		{"absent is zero", c, model.ClassStandard, 0},
		{"nil scores", model.Classification{}, model.ClassLookup, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, tt.c.Score(tt.class)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
