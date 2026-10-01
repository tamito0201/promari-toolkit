package di_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/samber/do/v2"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/application/usecase"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/di"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/repository"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/service"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/artifact"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/clock"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/failurelog"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/persistence"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/settings"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/transcript"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/interfaces/hook"
)

func invoke[T any](i do.Injector) (any, error) { return do.Invoke[T](i) }

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
		overrides []func(do.Injector)
		invoke    func(do.Injector) (any, error)
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
			overrides: []func(do.Injector){func(i do.Injector) { do.OverrideValue[repository.Clock](i, fixed) }},
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
			if report := c.Shutdown(); report != nil && !report.Succeed {
				t.Errorf("Shutdown() = %v", report)
			}
		})
	}
}

func TestContainerLedgerFailure(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(do.Injector) (any, error)
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
			t.Cleanup(func() { _ = c.Shutdown() })
			if _, err := tt.invoke(c); err == nil {
				t.Error("invoke succeeded, want the ledger error")
			}
		})
	}
}
