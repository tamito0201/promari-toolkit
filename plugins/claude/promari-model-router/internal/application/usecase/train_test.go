package usecase_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/repository"
	"promari-model-router/internal/infrastructure/artifact"
	"promari-model-router/internal/infrastructure/cases"
	"promari-model-router/internal/infrastructure/clock"
	"promari-model-router/pkg/fp"
)

// Each case is a full training run (CPU-bound, about 5 minutes for the table
// under CI's race detector and atomic coverage when run one after another), so
// the cases run in parallel. The parent cannot: it isolates HOME once for all of
// them with t.Setenv, which t.Parallel forbids.
//
//nolint:tparallel // see above: t.Setenv in the parent rules out t.Parallel there
func TestTrain(t *testing.T) {
	env(t)
	good := `{"lang":"ja","expect":"lookup","text":"` + lookupPrompt + `"}` + "\n" +
		`{"lang":"ja","expect":"standard","text":"ログイン画面のバリデーションを実装してテストを書いて"}` + "\n"
	ledger := []model.Entry{
		{At: testNow, Event: model.EventSubagent, ToolUseID: "t1", SessionID: "s1", PromptSHA: "a", Class: model.ClassLookup, PromptChars: 20},
		{At: testNow, Event: model.EventSubagentResult, ToolUseID: "t1", Resolved: "claude-haiku-4-5", Status: "completed", TotalTokens: 100},
		{At: testNow, Event: model.EventSubagent, ToolUseID: "t2", SessionID: "s1", Class: model.ClassNone},
		{At: testNow, Event: model.EventSubagentResult, ToolUseID: "t2", Resolved: "claude-haiku-4-5"},
		{At: testNow, Event: model.EventSubagentResult, ToolUseID: "orphan"},
		{At: testNow, Event: model.EventSubagent, SessionID: "s1", Class: model.ClassLookup},
		{At: testNow, Event: model.EventPrompt, ToolUseID: "t1"},
	}
	tests := []struct {
		name       string
		in         func(t *testing.T) usecase.TrainInput
		ledger     []model.Entry
		sinceErr   error
		saveErr    error
		wantOrigin string
		wantPath   func(in usecase.TrainInput) string
		wantMetric fp.Option[float64] // ledger_outcomes
		wantErr    error              // matched with errors.Is
	}{
		{
			// It used to be written over the artifact routing uses, putting
			// routing back on the rules without a word.
			name: "embedded set only: recorded as embedded, not written over the store",
			in: func(t *testing.T) usecase.TrainInput {
				t.Helper()
				return usecase.TrainInput{}
			},
			wantOrigin: model.OriginEmbedded,
			wantPath:   func(usecase.TrainInput) string { return "" },
			wantErr:    usecase.ErrNotRouting,
		},
		{
			name: "own labels and the ledger: local, joined outcomes counted",
			in: func(t *testing.T) usecase.TrainInput {
				t.Helper()
				return usecase.TrainInput{Files: []string{writeFile(t, good), ""}, LedgerDays: 7}
			},
			ledger:     ledger,
			wantOrigin: model.OriginLocal,
			wantPath:   func(usecase.TrainInput) string { return "/store/artifact.json" },
			wantMetric: fp.Some(1.0),
		},
		{
			name: "explicit output file",
			in: func(t *testing.T) usecase.TrainInput {
				t.Helper()
				return usecase.TrainInput{Files: []string{writeFile(t, good), ""}, Output: filepath.Join(t.TempDir(), "out.json")}
			},
			wantOrigin: model.OriginLocal,
			wantPath:   func(in usecase.TrainInput) string { return in.Output },
		},
		{
			name: "unreadable label file",
			in: func(t *testing.T) usecase.TrainInput {
				t.Helper()
				return usecase.TrainInput{Files: []string{filepath.Join(t.TempDir(), "absent.jsonl")}}
			},
			wantErr: os.ErrNotExist,
		},
		{
			name: "ledger read error",
			in: func(t *testing.T) usecase.TrainInput {
				t.Helper()
				return usecase.TrainInput{LedgerDays: 1}
			},
			sinceErr:   errLedger,
			wantOrigin: model.OriginEmbedded,
			wantPath:   func(usecase.TrainInput) string { return "" },
			wantErr:    errLedger,
		},
		{
			// A negative window used to learn from an empty future window without a word.
			name: "a negative ledger window is refused",
			in: func(t *testing.T) usecase.TrainInput {
				t.Helper()
				return usecase.TrainInput{LedgerDays: -1}
			},
			wantOrigin: model.OriginEmbedded,
			wantPath:   func(usecase.TrainInput) string { return "" },
			wantErr:    usecase.ErrInvalidDays,
		},
		{
			name: "store save error",
			in: func(t *testing.T) usecase.TrainInput {
				t.Helper()
				return usecase.TrainInput{Files: []string{writeFile(t, good), ""}}
			},
			saveErr:    errArtifact,
			wantOrigin: model.OriginLocal,
			wantPath:   func(usecase.TrainInput) string { return "/store/artifact.json" },
			wantErr:    errArtifact,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixtureInEnv(t)
			f.ledger.entries, f.ledger.sinceErr = tt.ledger, tt.sinceErr
			f.artifacts.path, f.artifacts.saveErr = "/store/artifact.json", tt.saveErr
			in := tt.in(t)
			art, path, err := usecase.TrainUseCase{
				Config: f.config, Ledger: f.ledger, Cases: cases.New(), Artifacts: f.artifacts,
				OutputTo: func(p string) repository.ArtifactWriter { return artifact.File{Path: p} },
				Clock:    clock.Fixed{At: testNow},
			}.Execute(t.Context(), in)
			if (err != nil) != (tt.wantErr != nil) || !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantPath == nil {
				return
			}
			if diff := cmp.Diff([]string{tt.wantOrigin, tt.wantPath(in)}, []string{art.Origin, path}); diff != "" {
				t.Errorf("origin, path (-want +got):\n%s", diff)
			}
			if !art.Ready() || !art.TrainedAt.Equal(testNow) {
				t.Errorf("artifact not trained on the injected clock: ready=%v at=%v", art.Ready(), art.TrainedAt)
			}
			v, ok := art.Metrics["ledger_outcomes"]
			if diff := cmp.Diff(tt.wantMetric, fp.OptionFrom(v, ok), cmpopts.EquateComparable(fp.Option[float64]{})); diff != "" {
				t.Errorf("ledger_outcomes (-want +got):\n%s", diff)
			}
			if in.Output != "" {
				raw, err := os.ReadFile(in.Output)
				if err != nil {
					t.Fatal(err)
				}
				var back model.Artifact
				if err := json.Unmarshal(raw, &back); err != nil || back.Origin != art.Origin {
					t.Errorf("output file = %v, %+v", err, back.Origin)
				}
			}
		})
	}
}
