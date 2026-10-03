package model_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/domain/model"
)

func TestDecisionWith(t *testing.T) {
	base := model.Decision{
		Action: model.ActionNone, Reason: "start", Target: model.TierSonnet,
		Class: model.ClassStandard, SubagentType: "scout", Requested: "opus",
	}
	tests := []struct {
		name         string
		apply        func(model.Decision) model.Decision
		want         model.Decision
		wantRewrites bool
	}{
		{
			name:  "unchanged",
			apply: func(d model.Decision) model.Decision { return d },
			want:  base,
		},
		{
			name:  "With sets action and reason",
			apply: func(d model.Decision) model.Decision { return d.With(model.ActionInject, "cheap") },
			want: model.Decision{
				Action: model.ActionInject, Reason: "cheap", Target: model.TierSonnet,
				Class: model.ClassStandard, SubagentType: "scout", Requested: "opus",
			},
			wantRewrites: true,
		},
		{
			name:  "With shadow does not rewrite",
			apply: func(d model.Decision) model.Decision { return d.With(model.ActionShadow, "record") },
			want: model.Decision{
				Action: model.ActionShadow, Reason: "record", Target: model.TierSonnet,
				Class: model.ClassStandard, SubagentType: "scout", Requested: "opus",
			},
		},
		{
			name:  "WithClass",
			apply: func(d model.Decision) model.Decision { return d.WithClass(model.ClassLookup) },
			want: model.Decision{
				Action: model.ActionNone, Reason: "start", Target: model.TierSonnet,
				Class: model.ClassLookup, SubagentType: "scout", Requested: "opus",
			},
		},
		{
			name:  "WithTarget",
			apply: func(d model.Decision) model.Decision { return d.WithTarget(model.TierHaiku) },
			want: model.Decision{
				Action: model.ActionNone, Reason: "start", Target: model.TierHaiku,
				Class: model.ClassStandard, SubagentType: "scout", Requested: "opus",
			},
		},
		{
			name: "chained",
			apply: func(d model.Decision) model.Decision {
				return d.WithClass(model.ClassLookup).WithTarget(model.TierHaiku).With(model.ActionInject, "lookup")
			},
			want: model.Decision{
				Action: model.ActionInject, Reason: "lookup", Target: model.TierHaiku,
				Class: model.ClassLookup, SubagentType: "scout", Requested: "opus",
			},
			wantRewrites: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := base
			got := tt.apply(d)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
			if got.Rewrites() != tt.wantRewrites {
				t.Errorf("Rewrites = %v, want %v", got.Rewrites(), tt.wantRewrites)
			}
			if diff := cmp.Diff(base, d); diff != "" {
				t.Errorf("the receiver changed (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAgentCallSubagentTypeOrDefault(t *testing.T) {
	tests := []struct {
		name string
		call model.AgentCall
		want string
	}{
		{"empty defaults to general-purpose", model.AgentCall{}, "general-purpose"},
		{"explicit type", model.AgentCall{SubagentType: "Explore"}, "Explore"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, tt.call.SubagentTypeOrDefault()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSubagentModelForced(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"1", true},
		{"true", true},
		{" TRUE ", true},
		{"yes", true},
		{"On", true},
		{"", false},
		{"0", false},
		{"false", false},
		{" No ", false},
		{"off", false},
		// A model name is not a switch: the three callers used to disagree on it.
		{"sonnet", false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got := model.SubagentModelForced(func(k string) string {
				if k != model.ForceEnv {
					t.Errorf("read %q", k)
				}
				return tt.value
			})
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}
