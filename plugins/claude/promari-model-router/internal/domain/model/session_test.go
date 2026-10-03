package model_test

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/domain/model"
)

func TestSessionModelTier(t *testing.T) {
	tests := []struct {
		name        string
		sm          model.SessionModel
		wantCertain bool
		wantTier    model.Tier
	}{
		{"transcript", model.SessionModel{Model: "claude-opus-5-5", Source: model.SourceTranscript}, true, model.TierOpus},
		{"session state", model.SessionModel{Model: "sonnet", Source: model.SourceSessionState}, true, model.TierSonnet},
		{"explicit", model.SessionModel{Model: "haiku", Source: model.SourceExplicit}, true, model.TierHaiku},
		{"certain but no family", model.SessionModel{Model: "default", Source: model.SourceExplicit}, true, model.TierUnknown},
		{"env may be stale", model.SessionModel{Model: "opus", Source: model.SourceEnv}, false, model.TierUnknown},
		{"settings may be stale", model.SessionModel{Model: "opus", Source: model.SourceSettings}, false, model.TierUnknown},
		{"unknown source", model.SessionModel{Model: "opus", Source: model.SourceUnknown}, false, model.TierUnknown},
		{"empty source", model.SessionModel{Model: "opus"}, false, model.TierUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type result struct {
				Certain bool
				Tier    model.Tier
			}
			want := result{tt.wantCertain, tt.wantTier}
			if diff := cmp.Diff(want, result{tt.sm.Source.Certain(), tt.sm.Tier()}); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSessionSwitchModel(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	base := model.Session{ID: "s1", Model: "sonnet", Source: model.SourceEnv, UpdatedAt: t0}
	tests := []struct {
		name   string
		model  string
		source model.SessionSource
		at     time.Time
		want   model.Session
	}{
		{"switch to opus", "opus", model.SourceSessionState, t1, model.Session{ID: "s1", Model: "opus", Source: model.SourceSessionState, UpdatedAt: t1}},
		{"same model, new source", "sonnet", model.SourceTranscript, t1, model.Session{ID: "s1", Model: "sonnet", Source: model.SourceTranscript, UpdatedAt: t1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := base
			if diff := cmp.Diff(tt.want, s.SwitchModel(tt.model, tt.source, tt.at)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(base, s); diff != "" {
				t.Errorf("the receiver changed (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNewSession(t *testing.T) {
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		got  model.Session
		want model.Session
	}{
		{name: "no model yet", got: model.NewSession("s"), want: model.Session{ID: "s"}},
		{
			name: "then a switch", got: model.NewSession("s").SwitchModel("opus", model.SourceSessionState, at),
			want: model.Session{ID: "s", Model: "opus", Source: model.SourceSessionState, UpdatedAt: at},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, tt.got); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}
