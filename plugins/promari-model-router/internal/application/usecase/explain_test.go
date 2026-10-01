package usecase_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/application/usecase"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
)

func TestExplain(t *testing.T) {
	tests := []struct {
		name      string
		in        usecase.ExplainInput
		loadErr   error
		env       fakeEnv
		wantClass string
		want      usecase.DecisionView
	}{
		{
			name:      "lookup brief under opus is injected to haiku",
			in:        usecase.ExplainInput{Prompt: lookupPrompt, SessionModel: "claude-opus-5-5"},
			wantClass: "lookup",
			want:      usecase.DecisionView{Action: string(model.ActionInject), Reason: "rule:lookup", Target: "haiku", Class: "lookup", SubagentType: "general-purpose"},
		},
		{
			name:      "built-in Explore agent with an unreadable artifact",
			in:        usecase.ExplainInput{Prompt: lookupPrompt, SessionModel: "claude-opus-5-5", SubagentType: "Explore"},
			loadErr:   errArtifact,
			wantClass: "lookup",
			want:      usecase.DecisionView{Action: string(model.ActionInject), Reason: "fixed:lookup", Target: "haiku", Class: "lookup", SubagentType: "Explore"},
		},
		{
			name:      "no session model means the [eval] one (opus)",
			in:        usecase.ExplainInput{Prompt: lookupPrompt},
			wantClass: "lookup",
			want:      usecase.DecisionView{Action: string(model.ActionInject), Reason: "rule:lookup", Target: "haiku", Class: "lookup", SubagentType: "general-purpose"},
		},
		{
			// `pmr explain` used to show an injection the hook would never make.
			name: "a forced subagent model shows the skip the hook makes",
			in:   usecase.ExplainInput{Prompt: lookupPrompt, SessionModel: "claude-opus-5-5"},
			env:  fakeEnv{model.ForceEnv: "1"},
			want: usecase.DecisionView{Action: string(model.ActionSkip), Reason: "subagent-model-forced", SubagentType: "general-purpose"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.artifacts.loadErr = tt.loadErr
			got := usecase.ExplainUseCase{Config: f.config, Artifacts: f.artifacts, Env: tt.env}.Execute(tt.in)
			if diff := cmp.Diff(tt.want, got.Decision); diff != "" {
				t.Errorf("decision (-want +got):\n%s", diff)
			}
			if tt.wantClass == "" { // the guard stopped before classification
				if diff := cmp.Diff([]string{"guard"}, got.Trace.Path); diff != "" {
					t.Errorf("path (-want +got):\n%s", diff)
				}
				return
			}
			if got.Class != tt.wantClass || got.Scores[tt.wantClass] == 0 || got.Chars == 0 || len(got.Trace.Path) == 0 {
				t.Errorf("explanation = %+v", got)
			}
		})
	}
}
