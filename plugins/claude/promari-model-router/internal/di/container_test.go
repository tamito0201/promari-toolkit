package di_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/di"
	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/repository"
	"promari-model-router/internal/domain/service"
	"promari-model-router/internal/infrastructure/artifact"
	"promari-model-router/internal/infrastructure/clock"
	"promari-model-router/internal/infrastructure/failurelog"
	"promari-model-router/internal/infrastructure/persistence"
	"promari-model-router/internal/infrastructure/settings"
	"promari-model-router/internal/infrastructure/transcript"
	"promari-model-router/internal/interfaces/cli"
	"promari-model-router/internal/interfaces/hook"
)

func invoke[T any](s *di.Scope) (any, error) { return di.Resolve[T](s) }

// isolate points every path the container reads at fresh directories and
// returns the plugin data directory.
func isolate(t *testing.T) string {
	t.Helper()
	data := filepath.Join(t.TempDir(), "data")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_PROJECT_DIR", t.TempDir())
	t.Setenv("CLAUDE_PLUGIN_DATA", data)
	return data
}

func TestContainerResolvesEveryService(t *testing.T) {
	fixed := clock.Fixed{At: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)}
	tests := []struct {
		name      string
		overrides []di.Option
		invoke    func(*di.Scope) (any, error)
		// want returns the expected value from the data directory and the defaults.
		want func(data string, st model.Settings) any
		// wantType is compared instead of the value when want is nil.
		wantType string
	}{
		{
			name: "paths live in CLAUDE_PLUGIN_DATA", invoke: invoke[di.Paths],
			want: func(data string, st model.Settings) any {
				return di.Paths{
					DataDir: data, Ledger: filepath.Join(data, st.Runtime.LedgerFile), Artifact: filepath.Join(data, st.Runtime.ArtifactFile),
					LastError: filepath.Join(data, st.Runtime.LastErrorFile), LauncherError: filepath.Join(data, settings.LauncherErrorFile),
				}
			},
		},
		{
			name: "database options come from [runtime]", invoke: invoke[persistence.Options],
			want: func(_ string, st model.Settings) any {
				return persistence.Options{BusyTimeoutMS: st.Runtime.SQLiteBusyTimeoutMS, Batch: st.Runtime.LedgerBatch}
			},
		},
		{
			name: "process settings are the defaults", invoke: invoke[model.Settings],
			want: func(_ string, st model.Settings) any { return st.Runtime },
		},
		{
			name: "the transcript reader takes the tail size from [runtime]", invoke: invoke[repository.TranscriptReader],
			want: func(_ string, st model.Settings) any {
				return transcript.Reader{TailBytes: st.Runtime.TranscriptTailBytes}
			},
		},
		{
			name: "the artifact store writes into the data directory", invoke: invoke[repository.ArtifactStore],
			want: func(data string, st model.Settings) any {
				return artifact.Store{LocalPath: filepath.Join(data, st.Runtime.ArtifactFile)}
			},
		},
		{name: "the clock is the wall clock", invoke: invoke[repository.Clock], want: func(string, model.Settings) any { return clock.System{} }},
		{
			name: "the clock can be overridden", invoke: invoke[repository.Clock],
			overrides: []di.Option{di.Replace(func() repository.Clock { return fixed })},
			want:      func(string, model.Settings) any { return fixed },
		},
		{name: "settings reader", invoke: invoke[repository.SettingsReader], want: func(string, model.Settings) any { return settings.EnvSettings{} }},
		{name: "process environment", invoke: invoke[repository.EnvReader], want: func(string, model.Settings) any { return settings.ProcessEnv{} }},
		{name: "labelled cases", invoke: invoke[repository.CaseSource], wantType: "cases.Source"},
		{name: "agent definitions", invoke: invoke[repository.AgentSource], wantType: "agents.Source"},
		{
			name: "read-only ledger queries use the ledger path", invoke: invoke[repository.LedgerQuery],
			want: func(data string, st model.Settings) any {
				return persistence.ReadOnlyLedger{
					Path:    filepath.Join(data, st.Runtime.LedgerFile),
					Options: persistence.Options{BusyTimeoutMS: st.Runtime.SQLiteBusyTimeoutMS, Batch: st.Runtime.LedgerBatch},
				}
			},
		},
		{name: "config provider", invoke: invoke[repository.ConfigProvider], wantType: "*settings.Provider"},
		{name: "ledger database", invoke: invoke[*persistence.DB], wantType: "*persistence.DB"},
		{name: "ledger repository", invoke: invoke[repository.LedgerRepository], wantType: "*persistence.LedgerRepo"},
		{name: "session repository", invoke: invoke[repository.SessionRepository], wantType: "*persistence.SessionRepo"},
		{name: "usage reader", invoke: invoke[repository.UsageReader], wantType: "*usage.Reader"},
		{name: "price table", invoke: invoke[service.PriceTable], wantType: "service.PriceTable"},
		{name: "session resolver", invoke: invoke[usecase.SessionResolver], wantType: "usecase.SessionResolver"},
		{name: "error recorder", invoke: invoke[usecase.RecordErrorUseCase], wantType: "usecase.RecordErrorUseCase"},
		{
			name: "the failure file lives in the data directory", invoke: invoke[repository.FailureRecorder],
			want: func(data string, st model.Settings) any {
				return failurelog.File{Path: filepath.Join(data, st.Runtime.LastErrorFile)}
			},
		},
		{name: "feed", invoke: invoke[usecase.FeedUseCase], wantType: "usecase.FeedUseCase"},
		{name: "verify", invoke: invoke[usecase.VerifyUseCase], wantType: "usecase.VerifyUseCase"},
		{name: "cost", invoke: invoke[usecase.CostUseCase], wantType: "usecase.CostUseCase"},
		{name: "hook handlers", invoke: invoke[hook.Handlers], wantType: "hook.Handlers"},
		{name: "report", invoke: invoke[usecase.ReportUseCase], wantType: "usecase.ReportUseCase"},
		{name: "explain", invoke: invoke[usecase.ExplainUseCase], wantType: "usecase.ExplainUseCase"},
		{name: "eval", invoke: invoke[usecase.EvalUseCase], wantType: "usecase.EvalUseCase"},
		{name: "train", invoke: invoke[usecase.TrainUseCase], wantType: "usecase.TrainUseCase"},
		{name: "lint", invoke: invoke[usecase.LintUseCase], wantType: "usecase.LintUseCase"},
		{name: "query", invoke: invoke[usecase.QueryUseCase], wantType: "usecase.QueryUseCase"},
		{name: "doctor", invoke: invoke[usecase.DoctorUseCase], wantType: "usecase.DoctorUseCase"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := isolate(t)
			st, _, _ := settings.Load("")
			c := di.New(tt.overrides...)
			got, err := tt.invoke(c)
			if err != nil {
				t.Fatal(err)
			}
			if tt.want != nil {
				if s, ok := got.(model.Settings); ok {
					got = s.Runtime
				}
				if diff := cmp.Diff(tt.want(data, st), got); diff != "" {
					t.Errorf("service mismatch (-want +got):\n%s", diff)
				}
			} else if diff := cmp.Diff(tt.wantType, fmt.Sprintf("%T", got)); diff != "" {
				t.Errorf("service type mismatch (-want +got):\n%s", diff)
			}
			if err := c.Close(); err != nil {
				t.Errorf("Close() = %v", err)
			}
		})
	}
}

func TestContainerLedgerFailure(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*di.Scope) (any, error)
	}{
		{name: "the ledger cannot be opened", invoke: invoke[*persistence.DB]},
		{name: "the failure surfaces through the use cases", invoke: invoke[hook.Handlers]},
		{name: "and through the doctor", invoke: invoke[usecase.DoctorUseCase]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)
			// The data directory's parent is a file, so the ledger directory cannot be created.
			blocker := filepath.Join(t.TempDir(), "file")
			if err := os.WriteFile(blocker, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("CLAUDE_PLUGIN_DATA", filepath.Join(blocker, "data"))
			c := di.New()
			t.Cleanup(func() { _ = c.Close() })
			if _, err := tt.invoke(c); err == nil {
				t.Error("invoke succeeded, want the ledger error")
			}
		})
	}
}

// blockData makes the data directory impossible to create, so building the
// ledger fails.
func blockData(t *testing.T) {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_PLUGIN_DATA", filepath.Join(blocker, "data"))
}

func TestScope(t *testing.T) {
	fake := struct{ repository.LedgerRepository }{}
	tests := []struct {
		name  string
		setup func(t *testing.T)
		opts  []di.Option
		check func(t *testing.T, s *di.Scope)
	}{
		{
			name: "Close closes what the scope built, once",
			check: func(t *testing.T, s *di.Scope) {
				t.Helper()
				h, err := s.Hook() // builds the ledger
				if err != nil {
					t.Fatal(err)
				}
				if err := s.Close(); err != nil {
					t.Fatalf("Close() = %v", err)
				}
				if err := h.SubagentFinish.Ledger.Append(t.Context(), model.NewEntry(time.Now(), model.EventPrompt)); err == nil {
					t.Error("the ledger still takes writes after the scope closed")
				}
				if err := s.Close(); err != nil {
					t.Errorf("a second Close() = %v, want nothing left to close", err)
				}
			},
		},
		{
			name: "Close of a scope that built nothing",
			check: func(t *testing.T, s *di.Scope) {
				t.Helper()
				assertNoErr(t, s.Close())
			},
		},
		{
			name: "a replacement with no arguments never builds what it replaces",
			// Building the real ledger would fail.
			setup: blockData,
			opts:  []di.Option{di.Replace(func() repository.LedgerRepository { return fake })},
			check: func(t *testing.T, s *di.Scope) {
				t.Helper()
				got, err := s.Verify()
				if err != nil {
					t.Fatalf("Verify() = %v, want the fake ledger without opening the database", err)
				}
				if got.Ledger != repository.LedgerRepository(fake) {
					t.Errorf("Verify().Ledger = %T, want the replacement", got.Ledger)
				}
			},
		},
		{
			name: "a port left nil is named when the use case is built",
			opts: []di.Option{di.Replace(func() repository.LedgerQuery { return nil })},
			check: func(t *testing.T, s *di.Scope) {
				t.Helper()
				if _, err := s.Query(); err == nil || !strings.Contains(err.Error(), "QueryUseCase.Ledger is not wired") {
					t.Errorf("Query() = %v, want the missing port named", err)
				}
			},
		},
		{
			name: "a hook handler with a port left nil is named",
			opts: []di.Option{di.Replace(func() repository.UsageReader { return nil })},
			check: func(t *testing.T, s *di.Scope) {
				t.Helper()
				if _, err := s.Hook(); err == nil || !strings.Contains(err.Error(), "PromptSubmit.Usage is not wired") {
					t.Errorf("Hook() = %v, want the missing port named", err)
				}
			},
		},
		{
			name:  "a failure is the constructor's own error, without dig's source paths",
			setup: blockData,
			check: func(t *testing.T, s *di.Scope) {
				t.Helper()
				_, err := s.Report()
				if err == nil || strings.Contains(err.Error(), "container.go") || strings.Contains(err.Error(), "could not build") {
					t.Errorf("Report() = %v, want the ledger's own error", err)
				}
			},
		},
		{
			name: "process settings that cannot be built stop the program",
			opts: []di.Option{di.Replace(func() (model.Settings, error) { return model.Settings{}, errors.New("no settings") })},
			check: func(t *testing.T, s *di.Scope) {
				t.Helper()
				defer func() {
					if r := fmt.Sprint(recover()); !strings.Contains(r, "process settings: no settings") {
						t.Errorf("recover() = %q, want the settings failure", r)
					}
				}()
				s.Settings()
			},
		},
		{
			name: "every method of the command line's scope builds",
			check: func(t *testing.T, s *di.Scope) {
				t.Helper()
				var scope cli.Scope = s // the command line's factory, without a cycle
				if got, want := scope.Settings().Runtime, di.New().Settings().Runtime; got != want {
					t.Errorf("Settings() = %+v, want the process settings %+v", got, want)
				}
				for name, build := range map[string]func() error{
					"FailureRecorder": func() error { _, err := scope.FailureRecorder(); return err },
					"Clock":           func() error { _, err := scope.Clock(); return err },
					"Cloud":           func() error { _, err := scope.Cloud(); return err },
					"Eval":            func() error { _, err := scope.Eval(); return err },
					"Explain":         func() error { _, err := scope.Explain(); return err },
					"Feed":            func() error { _, err := scope.Feed(); return err },
					"Lint":            func() error { _, err := scope.Lint(); return err },
					"Policy":          func() error { _, err := scope.Policy(); return err },
					"Cost":            func() error { _, err := scope.Cost(); return err },
					"Doctor":          func() error { _, err := scope.Doctor(); return err },
					"Report":          func() error { _, err := scope.Report(); return err },
					"Train":           func() error { _, err := scope.Train(); return err },
					"Verify":          func() error { _, err := scope.Verify(); return err },
					"Query":           func() error { _, err := scope.Query(); return err },
					"Hook":            func() error { _, err := scope.Hook(); return err },
					"RecordError":     func() error { _, err := scope.RecordError(); return err },
				} {
					if err := build(); err != nil {
						t.Errorf("%s() = %v", name, err)
					}
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)
			if tt.setup != nil {
				tt.setup(t)
			}
			s := di.New(tt.opts...)
			t.Cleanup(func() { _ = s.Close() })
			tt.check(t, s)
		})
	}
}

func assertNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Error(err)
	}
}

func TestAWiringMistakeStopsTheProgram(t *testing.T) {
	tests := []struct {
		name string
		ctor any
		want string
	}{
		// A replacement for a type nothing provides would replace nothing.
		{name: "a type nothing provides", ctor: func() *time.Location { return time.UTC }, want: "nothing provides *time.Location"},
		{name: "not a function", ctor: "clock", want: "want a function"},
		{name: "a function returning nothing", ctor: func() {}, want: "want a function"},
		{name: "a second result that is not an error", ctor: func() (repository.Clock, int) { return nil, 0 }, want: "want a function"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := fmt.Sprint(recover()); !strings.Contains(r, tt.want) {
					t.Errorf("recover() = %q, want %q", r, tt.want)
				}
			}()
			di.New(di.Replace(tt.ctor))
		})
	}
}
