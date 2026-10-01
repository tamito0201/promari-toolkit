package usecase_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/application/usecase"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/cases"
)

func TestEval(t *testing.T) {
	long := strings.Repeat("設計", 60) + "\nを見直して"
	tests := []struct {
		name  string
		path  func(t *testing.T) string
		opt   usecase.EvalOptions
		check func(t *testing.T, s usecase.EvalSummary, st model.Settings)
		err   bool
	}{
		{
			name: "embedded set against the configured gate",
			path: func(t *testing.T) string {
				t.Helper()
				return ""
			},
			check: func(t *testing.T, s usecase.EvalSummary, st model.Settings) {
				t.Helper()
				want := usecase.EvalGate{MinAccuracy: st.Eval.MinAccuracy, MaxHarmful: st.Eval.MaxHarmful, SessionModel: st.Eval.SessionModel}
				if diff := cmp.Diff(want, s.Gate); diff != "" {
					t.Errorf("gate (-want +got):\n%s", diff)
				}
				if s.Cases == 0 || s.Injected == 0 || s.Accuracy <= 0 || len(s.ByLang) == 0 {
					t.Errorf("summary = %+v", s)
				}
				// The shipped labelled set must pass the shipped gate (the
				// same check as `task eval`), not merely agree with itself.
				if diff := cmp.Diff([]any{true, []string(nil)}, []any{s.Passed, s.FailedBecause}); diff != "" {
					t.Errorf("gate verdict (-want +got):\n%s", diff)
				}
			},
		},
		{
			name: "explicit thresholds, misses listed",
			path: func(t *testing.T) string {
				t.Helper()
				return writeFile(t, strings.Join([]string{
					`{"lang":"ja","expect":"lookup","text":"` + lookupPrompt + `"}`,
					`{"lang":"ja","expect":"architecture","text":"` + lookupPrompt + `"}`,
					`{"lang":"ja","expect":"","text":"` + lookupPrompt + `"}`,
					fmt.Sprintf(`{"lang":"ja","expect":"lookup","text":%q}`, long),
				}, "\n"))
			},
			opt: usecase.EvalOptions{
				SessionModel: "claude-opus-5-5", MinAccuracy: new(0.1), MaxHarmful: new(100),
				UseModel: new(false), AllowEmbedded: true,
			},
			check: func(t *testing.T, s usecase.EvalSummary, _ model.Settings) {
				t.Helper()
				if diff := cmp.Diff(usecase.EvalGate{MinAccuracy: 0.1, MaxHarmful: 100, SessionModel: "claude-opus-5-5"}, s.Gate); diff != "" {
					t.Errorf("gate (-want +got):\n%s", diff)
				}
				// By hand: the lookup brief goes to haiku three times (one
				// exact, two harmful). The long brief repeats one weak cue:
				// a phrase scores once however often it occurs, so "設計" and
				// "見直して" give architecture 2 < min_confidence 3 and the
				// rule abstains — a miss that injects nothing.
				type counts struct {
					Cases, Exact, Injected, Harmful int
					Accuracy, HarmfulRate           float64
					Passed                          bool
				}
				want := counts{Cases: 4, Exact: 1, Injected: 3, Harmful: 2, Accuracy: 0.25, HarmfulRate: 2.0 / 3, Passed: true}
				if diff := cmp.Diff(want, counts{s.Cases, s.Exact, s.Injected, s.Harmful, s.Accuracy, s.HarmfulRate, s.Passed}); diff != "" {
					t.Errorf("summary (-want +got):\n%s", diff)
				}
				wantMisses := []usecase.Miss{
					{Want: "architecture", Got: "lookup", Harmful: true, Text: lookupPrompt},
					{Want: "abstain", Got: "lookup", Harmful: true, Text: lookupPrompt},
					{Want: "lookup", Got: "abstain", Text: long},
				}
				if diff := cmp.Diff(wantMisses, s.Misses); diff != "" {
					t.Errorf("misses (-want +got):\n%s", diff)
				}
			},
		},
		{
			// It used to pass: a gate of 0 over no cases measured nothing.
			name: "empty file fails whatever the thresholds",
			path: func(t *testing.T) string {
				t.Helper()
				return writeFile(t, "")
			},
			opt: usecase.EvalOptions{MinAccuracy: new(0.0), MaxHarmful: new(0)},
			check: func(t *testing.T, s usecase.EvalSummary, _ model.Settings) {
				t.Helper()
				got := []any{s.Cases, s.Accuracy, s.HarmfulRate, s.Misses, s.Passed, s.FailedBecause}
				if diff := cmp.Diff([]any{0, 0.0, 0.0, []usecase.Miss(nil), false, []string{"no cases"}}, got); diff != "" {
					t.Errorf("summary (-want +got):\n%s", diff)
				}
			},
		},
		{
			// It used to pass: every route stopped at "unknown-session" and the
			// gate counted no harmful downgrade.
			name: "a session model without a known tier fails",
			path: func(t *testing.T) string {
				t.Helper()
				return writeFile(t, `{"lang":"ja","expect":"","text":"ok"}`)
			},
			opt: usecase.EvalOptions{SessionModel: "inherit", MinAccuracy: new(0.0), MaxHarmful: new(0)},
			check: func(t *testing.T, s usecase.EvalSummary, _ model.Settings) {
				t.Helper()
				if diff := cmp.Diff([]any{false, []string{`session model "inherit" has no known tier`}}, []any{s.Passed, s.FailedBecause}); diff != "" {
					t.Errorf("(-want +got):\n%s", diff)
				}
			},
		},
		{
			name: "an accuracy exactly at the minimum passes",
			path: func(t *testing.T) string {
				t.Helper()
				return writeFile(t, `{"lang":"ja","expect":"lookup","text":"`+lookupPrompt+`"}`)
			},
			opt: usecase.EvalOptions{SessionModel: "claude-opus-5-5", MinAccuracy: new(1.0), MaxHarmful: new(5), UseModel: new(false)},
			check: func(t *testing.T, s usecase.EvalSummary, _ model.Settings) {
				t.Helper()
				if diff := cmp.Diff([]any{1.0, true, []string(nil)}, []any{s.Accuracy, s.Passed, s.FailedBecause}); diff != "" {
					t.Errorf("(-want +got):\n%s", diff)
				}
			},
		},
		{
			name: "harmful downgrades exactly at the maximum pass",
			path: func(t *testing.T) string {
				t.Helper()
				return writeFile(t, `{"lang":"ja","expect":"architecture","text":"`+lookupPrompt+`"}`)
			},
			opt: usecase.EvalOptions{SessionModel: "claude-opus-5-5", MinAccuracy: new(0.0), MaxHarmful: new(1), UseModel: new(false)},
			check: func(t *testing.T, s usecase.EvalSummary, _ model.Settings) {
				t.Helper()
				if diff := cmp.Diff([]any{1, true, []string(nil)}, []any{s.Harmful, s.Passed, s.FailedBecause}); diff != "" {
					t.Errorf("(-want +got):\n%s", diff)
				}
			},
		},
		{
			name: "every failed threshold is named",
			path: func(t *testing.T) string {
				t.Helper()
				return writeFile(t, `{"lang":"ja","expect":"architecture","text":"`+lookupPrompt+`"}`)
			},
			opt: usecase.EvalOptions{SessionModel: "claude-opus-5-5", MinAccuracy: new(0.5), MaxHarmful: new(0), UseModel: new(false)},
			check: func(t *testing.T, s usecase.EvalSummary, _ model.Settings) {
				t.Helper()
				if diff := cmp.Diff([]string{"accuracy 0.000 < 0.50", "harmful downgrades 1 > 0"}, s.FailedBecause); diff != "" {
					t.Errorf("(-want +got):\n%s", diff)
				}
			},
		},
		{
			name: "missing file",
			path: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "absent.jsonl")
			},
			err: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.artifacts.loadErr = errArtifact
			// A shadow rollout in the config must not blank out the evaluation.
			f.config.mutate = func(s *model.Settings) { s.Routing.Mode = model.ModeShadow }
			opt := tt.opt
			opt.Path = tt.path(t)
			s, err := usecase.EvalUseCase{Config: f.config, Artifacts: f.artifacts, Cases: cases.New()}.Execute(opt)
			if (err != nil) != tt.err {
				t.Fatalf("err = %v, want error %v", err, tt.err)
			}
			if tt.check != nil {
				tt.check(t, s, f.config.Settings(""))
			}
		})
	}
}
