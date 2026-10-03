package model_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/domain/model"
)

func TestTierOf(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  model.Tier
	}{
		{"alias", "haiku", model.TierHaiku},
		{"full id with context suffix", "claude-opus-5-5[1m]", model.TierOpus},
		{"mixed case", "Claude-Sonnet-4-6", model.TierSonnet},
		{"fable", "claude-fable-5", model.TierFable},
		{"inherit has no family", "inherit", model.TierUnknown},
		{"empty", "", model.TierUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, model.TierOf(tt.model)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTierOrdering(t *testing.T) {
	type result struct {
		Rank   int
		Known  bool
		String string
	}
	tests := []struct {
		name string
		tier model.Tier
		want result
	}{
		{"haiku", model.TierHaiku, result{0, true, "haiku"}},
		{"sonnet", model.TierSonnet, result{1, true, "sonnet"}},
		{"opus", model.TierOpus, result{2, true, "opus"}},
		{"fable", model.TierFable, result{3, true, "fable"}},
		{"unknown", model.TierUnknown, result{-1, false, ""}},
		{"not a tier", model.Tier("gpt"), result{-1, false, "gpt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := result{tt.tier.Rank(), tt.tier.Known(), tt.tier.String()}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTierCompare(t *testing.T) {
	tests := []struct {
		name      string
		t, other  model.Tier
		wantAbove bool
		wantMin   model.Tier
	}{
		{"higher than other", model.TierOpus, model.TierHaiku, true, model.TierHaiku},
		{"lower than other", model.TierHaiku, model.TierOpus, false, model.TierHaiku},
		{"equal", model.TierSonnet, model.TierSonnet, false, model.TierSonnet},
		{"unknown receiver yields the other", model.TierUnknown, model.TierOpus, false, model.TierOpus},
		{"unknown other yields the receiver", model.TierFable, model.TierUnknown, false, model.TierFable},
		{"both unknown", model.TierUnknown, model.TierUnknown, false, model.TierUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type result struct {
				Above bool
				Min   model.Tier
			}
			want := result{tt.wantAbove, tt.wantMin}
			if diff := cmp.Diff(want, result{tt.t.Above(tt.other), tt.t.Min(tt.other)}); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
