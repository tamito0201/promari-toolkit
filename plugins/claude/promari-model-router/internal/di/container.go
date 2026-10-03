// Package di is the composition root: it wires the infrastructure
// implementations into the application use cases with samber/do. Services
// are lazy, so a hook process only opens the ledger when a use case needs it.
package di

import (
	"path/filepath"
	"time"

	"github.com/samber/do/v2"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/repository"
	"promari-model-router/internal/domain/service"
	"promari-model-router/internal/infrastructure/agents"
	"promari-model-router/internal/infrastructure/artifact"
	"promari-model-router/internal/infrastructure/cases"
	"promari-model-router/internal/infrastructure/clock"
	"promari-model-router/internal/infrastructure/cloudrelay"
	"promari-model-router/internal/infrastructure/failurelog"
	"promari-model-router/internal/infrastructure/persistence"
	"promari-model-router/internal/infrastructure/settings"
	"promari-model-router/internal/infrastructure/transcript"
	"promari-model-router/internal/infrastructure/usage"
	"promari-model-router/internal/interfaces/hook"
)

// Paths locate the plugin's state.
type Paths struct {
	DataDir  string
	Ledger   string
	Artifact string
	// LastError is where a hook leaves a failure the ledger could not take.
	LastError string
	// LauncherError is where bin/pmr leaves a failure to start the binary
	// (under ${CLAUDE_PLUGIN_DATA}, whatever runtime.data_dir says).
	LauncherError string
}

// Infrastructure registers the adapters behind the domain ports.
var Infrastructure = do.Package(
	do.Lazy(func(do.Injector) (*settings.Provider, error) { return settings.NewProvider(), nil }),
	do.Lazy(func(i do.Injector) (repository.ConfigProvider, error) {
		return do.MustInvoke[*settings.Provider](i), nil
	}),
	// Settings for process-level wiring (paths, limits): the defaults and the
	// user file only, never a project file. Project-specific settings are
	// resolved per event through ConfigProvider.
	do.Lazy(func(i do.Injector) (model.Settings, error) {
		return do.MustInvoke[*settings.Provider](i).Process(), nil
	}),
	do.Lazy(func(i do.Injector) (Paths, error) {
		st := do.MustInvoke[model.Settings](i)
		dir := settings.DataDir(st)
		return Paths{
			DataDir: dir, Ledger: filepath.Join(dir, st.Runtime.LedgerFile), Artifact: filepath.Join(dir, st.Runtime.ArtifactFile),
			LastError:     filepath.Join(dir, st.Runtime.LastErrorFile),
			LauncherError: filepath.Join(settings.PluginDataDir(), settings.LauncherErrorFile),
		}, nil
	}),
	do.Lazy(func(i do.Injector) (persistence.Options, error) {
		st := do.MustInvoke[model.Settings](i)
		return persistence.Options{BusyTimeoutMS: st.Runtime.SQLiteBusyTimeoutMS, Batch: st.Runtime.LedgerBatch}, nil
	}),
	do.Lazy(func(i do.Injector) (*persistence.DB, error) {
		return persistence.Open(do.MustInvoke[Paths](i).Ledger, do.MustInvoke[persistence.Options](i))
	}),
	do.Lazy(func(i do.Injector) (repository.LedgerRepository, error) {
		return persistence.NewLedgerRepo(do.MustInvoke[*persistence.DB](i)), nil
	}),
	do.Lazy(func(i do.Injector) (repository.LedgerQuery, error) {
		return persistence.ReadOnlyLedger{Path: do.MustInvoke[Paths](i).Ledger, Options: do.MustInvoke[persistence.Options](i)}, nil
	}),
	do.Lazy(func(i do.Injector) (repository.SessionRepository, error) {
		return persistence.NewSessionRepo(do.MustInvoke[*persistence.DB](i)), nil
	}),
	do.Lazy(func(do.Injector) (repository.Clock, error) { return clock.System{}, nil }),
	do.Lazy(func(i do.Injector) (repository.TranscriptReader, error) {
		return transcript.Reader{TailBytes: do.MustInvoke[model.Settings](i).Runtime.TranscriptTailBytes}, nil
	}),
	do.Lazy(func(do.Injector) (repository.SettingsReader, error) { return settings.EnvSettings{}, nil }),
	do.Lazy(func(do.Injector) (repository.EnvReader, error) { return settings.ProcessEnv{}, nil }),
	do.Lazy(func(do.Injector) (repository.CaseSource, error) { return cases.New(), nil }),
	do.Lazy(func(do.Injector) (repository.AgentSource, error) { return agents.New(), nil }),
	do.Lazy(func(i do.Injector) (repository.ArtifactStore, error) {
		return artifact.Store{LocalPath: do.MustInvoke[Paths](i).Artifact}, nil
	}),
	do.Lazy(func(i do.Injector) (repository.UsageReader, error) {
		return usage.New(do.MustInvoke[model.Settings](i).Pressure, do.MustInvoke[repository.Clock](i)), nil
	}),
	do.Lazy(func(i do.Injector) (service.PriceTable, error) {
		return do.MustInvoke[repository.ConfigProvider](i).Prices(), nil
	}),
	do.Lazy(func(i do.Injector) (repository.CloudMessenger, error) {
		c := do.MustInvoke[model.Settings](i).Cloud
		return cloudrelay.Messenger{Bin: c.ClaudeBin, Timeout: time.Duration(c.SendTimeoutMS) * time.Millisecond}, nil
	}),
	do.Lazy(func(i do.Injector) (repository.CloudLinkStore, error) {
		p := do.MustInvoke[Paths](i)
		return cloudrelay.Store{Path: filepath.Join(p.DataDir, do.MustInvoke[model.Settings](i).Cloud.LinkFile)}, nil
	}),
	// The failure file needs no database, so it is there when the ledger is not.
	do.Lazy(func(i do.Injector) (repository.FailureRecorder, error) {
		return failurelog.File{Path: do.MustInvoke[Paths](i).LastError}, nil
	}),
)

// Application registers the use cases, each with only the ports it uses.
var Application = do.Package(
	do.Lazy(func(i do.Injector) (usecase.SessionResolver, error) {
		return usecase.SessionResolver{
			Transcript: do.MustInvoke[repository.TranscriptReader](i),
			Sessions:   do.MustInvoke[repository.SessionRepository](i),
			Settings:   do.MustInvoke[repository.SettingsReader](i),
		}, nil
	}),
	do.Lazy(func(i do.Injector) (usecase.RecordErrorUseCase, error) {
		st := do.MustInvoke[model.Settings](i)
		return usecase.RecordErrorUseCase{
			Ledger: do.MustInvoke[repository.LedgerRepository](i), Failures: do.MustInvoke[repository.FailureRecorder](i),
			Clock: do.MustInvoke[repository.Clock](i), DetailRunes: st.Runtime.ErrorDetailRunes,
			Timeout: time.Duration(st.Runtime.ErrorWriteTimeoutMS) * time.Millisecond,
		}, nil
	}),
	do.Lazy(func(i do.Injector) (hook.Handlers, error) {
		st := do.MustInvoke[model.Settings](i)
		ledger := do.MustInvoke[repository.LedgerRepository](i)
		sessions := do.MustInvoke[repository.SessionRepository](i)
		config := do.MustInvoke[repository.ConfigProvider](i)
		resolver := do.MustInvoke[usecase.SessionResolver](i)
		clk := do.MustInvoke[repository.Clock](i)
		env := do.MustInvoke[repository.EnvReader](i)
		errs := do.MustInvoke[usecase.RecordErrorUseCase](i)
		return hook.Handlers{
			Config: hook.Config{StdinLimitBytes: st.Runtime.StdinLimitBytes},
			SessionStart: usecase.SessionStart{
				Sessions: sessions, SessionRetention: sessions, LedgerRetention: ledger, Ledger: ledger,
				Config: config, Env: env, Clock: clk, Errors: errs,
			},
			ModelSwitch: usecase.ModelSwitch{Sessions: sessions, Ledger: ledger, Clock: clk, Errors: errs},
			PromptSubmit: usecase.PromptSubmit{
				Config: config, Resolver: resolver, Usage: do.MustInvoke[repository.UsageReader](i),
				Ledger: ledger, Clock: clk, Errors: errs,
			},
			SubagentStart: usecase.SubagentStart{
				Config: config, Resolver: resolver, Artifacts: do.MustInvoke[repository.ArtifactStore](i),
				History: ledger, Ledger: ledger, Env: env, Clock: clk, Errors: errs,
			},
			SubagentFinish: usecase.SubagentFinish{Ledger: ledger, Clock: clk, Errors: errs},
		}, nil
	}),
	do.Lazy(func(i do.Injector) (usecase.ReportUseCase, error) {
		return usecase.ReportUseCase{
			Ledger: do.MustInvoke[repository.LedgerRepository](i), Clock: do.MustInvoke[repository.Clock](i),
			Prices: do.MustInvoke[service.PriceTable](i), DefaultDays: do.MustInvoke[model.Settings](i).Report.DefaultDays,
		}, nil
	}),
	do.Lazy(func(i do.Injector) (usecase.FeedUseCase, error) {
		return usecase.FeedUseCase{Ledger: do.MustInvoke[repository.LedgerRepository](i), Clock: do.MustInvoke[repository.Clock](i)}, nil
	}),
	do.Lazy(func(i do.Injector) (usecase.PolicyUseCase, error) {
		return usecase.PolicyUseCase{Config: do.MustInvoke[repository.ConfigProvider](i)}, nil
	}),
	do.Lazy(func(i do.Injector) (usecase.VerifyUseCase, error) {
		return usecase.VerifyUseCase{Ledger: do.MustInvoke[repository.LedgerRepository](i)}, nil
	}),
	do.Lazy(func(i do.Injector) (usecase.CostUseCase, error) {
		return usecase.CostUseCase{Prices: do.MustInvoke[repository.ConfigProvider](i)}, nil
	}),
	do.Lazy(func(i do.Injector) (usecase.ExplainUseCase, error) {
		return usecase.ExplainUseCase{
			Config: do.MustInvoke[repository.ConfigProvider](i), Artifacts: do.MustInvoke[repository.ArtifactStore](i),
			Env: do.MustInvoke[repository.EnvReader](i),
		}, nil
	}),
	do.Lazy(func(i do.Injector) (usecase.EvalUseCase, error) {
		return usecase.EvalUseCase{
			Config: do.MustInvoke[repository.ConfigProvider](i), Artifacts: do.MustInvoke[repository.ArtifactStore](i),
			Cases: do.MustInvoke[repository.CaseSource](i),
		}, nil
	}),
	do.Lazy(func(i do.Injector) (usecase.TrainUseCase, error) {
		return usecase.TrainUseCase{
			Config: do.MustInvoke[repository.ConfigProvider](i), Ledger: do.MustInvoke[repository.LedgerRepository](i),
			Cases: do.MustInvoke[repository.CaseSource](i), Artifacts: do.MustInvoke[repository.ArtifactStore](i),
			OutputTo: func(path string) repository.ArtifactWriter { return artifact.File{Path: path} },
			Clock:    do.MustInvoke[repository.Clock](i),
		}, nil
	}),
	do.Lazy(func(i do.Injector) (usecase.LintUseCase, error) {
		return usecase.LintUseCase{Tiers: do.MustInvoke[repository.ConfigProvider](i), Agents: do.MustInvoke[repository.AgentSource](i)}, nil
	}),
	do.Lazy(func(i do.Injector) (usecase.CloudUseCase, error) {
		return usecase.CloudUseCase{
			Messenger: do.MustInvoke[repository.CloudMessenger](i), Links: do.MustInvoke[repository.CloudLinkStore](i),
			Clock: do.MustInvoke[repository.Clock](i),
		}, nil
	}),
	do.Lazy(func(i do.Injector) (usecase.QueryUseCase, error) {
		return usecase.QueryUseCase{Ledger: do.MustInvoke[repository.LedgerQuery](i)}, nil
	}),
	do.Lazy(func(i do.Injector) (usecase.DoctorUseCase, error) {
		p := do.MustInvoke[Paths](i)
		return usecase.DoctorUseCase{
			Config:           do.MustInvoke[repository.ConfigProvider](i),
			Env:              do.MustInvoke[repository.EnvReader](i),
			Resolver:         do.MustInvoke[usecase.SessionResolver](i),
			Usage:            do.MustInvoke[repository.UsageReader](i),
			Artifacts:        do.MustInvoke[repository.ArtifactStore](i),
			Agents:           do.MustInvoke[repository.AgentSource](i),
			Ledger:           do.MustInvoke[repository.LedgerRepository](i),
			HookFailures:     failurelog.File{Path: p.LastError},
			LauncherFailures: failurelog.File{Path: p.LauncherError},
			Clock:            do.MustInvoke[repository.Clock](i),
			DataDir:          p.DataDir,
			LedgerPath:       p.Ledger,
		}, nil
	}),
)

// New builds the root container. Call Shutdown on it to close the ledger.
func New(overrides ...func(do.Injector)) *do.RootScope {
	return do.New(append([]func(do.Injector){Infrastructure, Application}, overrides...)...)
}
