// Package di is the composition root: it wires the infrastructure
// implementations into the application use cases with go.uber.org/dig, and
// hands the command line a Scope that builds them. Scope satisfies cli.Scope
// without importing the command line (main and the tests check it), so the
// entry points can be tested over a real scope without an import cycle.
//
// dig builds a value the first time something asks for it, so a hook process
// only opens the ledger when the use case it runs needs it. A constructor that
// fails is tried again on the next request (dig keeps only what was built),
// which is what lets a hook fall back to the failure file when the ledger
// cannot be opened. dig has no lifecycle of its own: what must be released
// (the ledger) registers itself with the scope's closers when it is built.
package di

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"time"

	"go.uber.org/dig"

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

// ---------------------------------------------------------------- infrastructure

// infrastructure are the adapters behind the domain ports.
var infrastructure = []any{
	settings.NewProvider,
	func(p *settings.Provider) repository.ConfigProvider { return p },
	// Settings for process-level wiring (paths, limits): the defaults and the
	// user file only, never a project file. Project-specific settings are
	// resolved per event through ConfigProvider.
	func(p *settings.Provider) model.Settings { return p.Process() },
	newPaths,
	func(st model.Settings) persistence.Options {
		return persistence.Options{BusyTimeoutMS: st.Runtime.SQLiteBusyTimeoutMS, Batch: st.Runtime.LedgerBatch}
	},
	openLedger,
	func(db *persistence.DB) repository.LedgerRepository { return persistence.NewLedgerRepo(db) },
	func(p Paths, o persistence.Options) repository.LedgerQuery {
		return persistence.ReadOnlyLedger{Path: p.Ledger, Options: o}
	},
	func(db *persistence.DB) repository.SessionRepository { return persistence.NewSessionRepo(db) },
	func() repository.Clock { return clock.System{} },
	func(st model.Settings) repository.TranscriptReader {
		return transcript.Reader{TailBytes: st.Runtime.TranscriptTailBytes}
	},
	func() repository.SettingsReader { return settings.EnvSettings{} },
	func() repository.EnvReader { return settings.ProcessEnv{} },
	func() repository.CaseSource { return cases.New() },
	func() repository.AgentSource { return agents.New() },
	func(p Paths) repository.ArtifactStore { return artifact.Store{LocalPath: p.Artifact} },
	func(st model.Settings, c repository.Clock) repository.UsageReader { return usage.New(st.Pressure, c) },
	func(c repository.ConfigProvider) service.PriceTable { return c.Prices() },
	func(st model.Settings) repository.CloudMessenger {
		return cloudrelay.Messenger{Bin: st.Cloud.ClaudeBin, Timeout: time.Duration(st.Cloud.SendTimeoutMS) * time.Millisecond}
	},
	func(p Paths, st model.Settings) repository.CloudLinkStore {
		return cloudrelay.Store{Path: filepath.Join(p.DataDir, st.Cloud.LinkFile)}
	},
	// The failure file needs no database, so it is there when the ledger is not.
	func(p Paths) repository.FailureRecorder { return failurelog.File{Path: p.LastError} },
}

func newPaths(st model.Settings) Paths {
	dir := settings.DataDir(st)
	return Paths{
		DataDir: dir, Ledger: filepath.Join(dir, st.Runtime.LedgerFile), Artifact: filepath.Join(dir, st.Runtime.ArtifactFile),
		LastError:     filepath.Join(dir, st.Runtime.LastErrorFile),
		LauncherError: filepath.Join(settings.PluginDataDir(), settings.LauncherErrorFile),
	}
}

// openLedger opens the ledger and has the scope close it.
func openLedger(p Paths, o persistence.Options, c *closers) (*persistence.DB, error) {
	db, err := persistence.Open(p.Ledger, o)
	if err != nil {
		return nil, err
	}
	c.add(db.Close)
	return db, nil
}

// ---------------------------------------------------------------- application

// application are the use cases, each built with only the ports it uses and
// checked to have all of them (complete).
var application = []any{
	func(t repository.TranscriptReader, s repository.SessionRepository, r repository.SettingsReader) (usecase.SessionResolver, error) {
		return complete(usecase.SessionResolver{Transcript: t, Sessions: s, Settings: r})
	},
	func(st model.Settings, l repository.LedgerRepository, f repository.FailureRecorder, c repository.Clock) (usecase.RecordErrorUseCase, error) {
		return complete(usecase.RecordErrorUseCase{
			Ledger: l, Failures: f, Clock: c, DetailRunes: st.Runtime.ErrorDetailRunes,
			Timeout: time.Duration(st.Runtime.ErrorWriteTimeoutMS) * time.Millisecond,
		})
	},
	newHandlers,
	func(l repository.LedgerRepository, c repository.Clock, p service.PriceTable, st model.Settings) (usecase.ReportUseCase, error) {
		return complete(usecase.ReportUseCase{Ledger: l, Clock: c, Prices: p, DefaultDays: st.Report.DefaultDays})
	},
	func(l repository.LedgerRepository, c repository.Clock) (usecase.FeedUseCase, error) {
		return complete(usecase.FeedUseCase{Ledger: l, Clock: c})
	},
	func(c repository.ConfigProvider) (usecase.PolicyUseCase, error) {
		return complete(usecase.PolicyUseCase{Config: c})
	},
	func(l repository.LedgerRepository) (usecase.VerifyUseCase, error) {
		return complete(usecase.VerifyUseCase{Ledger: l})
	},
	func(c repository.ConfigProvider) (usecase.CostUseCase, error) {
		return complete(usecase.CostUseCase{Prices: c})
	},
	func(c repository.ConfigProvider, a repository.ArtifactStore, e repository.EnvReader) (usecase.ExplainUseCase, error) {
		return complete(usecase.ExplainUseCase{Config: c, Artifacts: a, Env: e})
	},
	func(c repository.ConfigProvider, a repository.ArtifactStore, s repository.CaseSource) (usecase.EvalUseCase, error) {
		return complete(usecase.EvalUseCase{Config: c, Artifacts: a, Cases: s})
	},
	func(c repository.ConfigProvider, l repository.LedgerRepository, s repository.CaseSource, a repository.ArtifactStore, clk repository.Clock) (usecase.TrainUseCase, error) {
		return complete(usecase.TrainUseCase{
			Config: c, Ledger: l, Cases: s, Artifacts: a, Clock: clk,
			OutputTo: func(path string) repository.ArtifactWriter { return artifact.File{Path: path} },
		})
	},
	func(c repository.ConfigProvider, a repository.AgentSource) (usecase.LintUseCase, error) {
		return complete(usecase.LintUseCase{Tiers: c, Agents: a})
	},
	func(m repository.CloudMessenger, l repository.CloudLinkStore, c repository.Clock) (usecase.CloudUseCase, error) {
		return complete(usecase.CloudUseCase{Messenger: m, Links: l, Clock: c})
	},
	func(q repository.LedgerQuery) (usecase.QueryUseCase, error) {
		return complete(usecase.QueryUseCase{Ledger: q})
	},
	newDoctor,
}

// handlerDeps are what the hook handlers are built from (a dig parameter
// object: the list is long, and named fields read better than positions).
type handlerDeps struct {
	dig.In

	Settings model.Settings
	Ledger   repository.LedgerRepository
	Sessions repository.SessionRepository
	Config   repository.ConfigProvider
	Resolver usecase.SessionResolver
	Clock    repository.Clock
	Env      repository.EnvReader
	Errors   usecase.RecordErrorUseCase
	Usage    repository.UsageReader
	Artifact repository.ArtifactStore
}

func newHandlers(d handlerDeps) (hook.Handlers, error) {
	h := hook.Handlers{
		Config: hook.Config{StdinLimitBytes: d.Settings.Runtime.StdinLimitBytes},
		SessionStart: usecase.SessionStart{
			Sessions: d.Sessions, SessionRetention: d.Sessions, LedgerRetention: d.Ledger, Ledger: d.Ledger,
			Config: d.Config, Env: d.Env, Clock: d.Clock, Errors: d.Errors,
		},
		ModelSwitch: usecase.ModelSwitch{Sessions: d.Sessions, Ledger: d.Ledger, Clock: d.Clock, Errors: d.Errors},
		PromptSubmit: usecase.PromptSubmit{
			Config: d.Config, Resolver: d.Resolver, Usage: d.Usage, Ledger: d.Ledger, Clock: d.Clock, Errors: d.Errors,
		},
		SubagentStart: usecase.SubagentStart{
			Config: d.Config, Resolver: d.Resolver, Artifacts: d.Artifact,
			History: d.Ledger, Ledger: d.Ledger, Env: d.Env, Clock: d.Clock, Errors: d.Errors,
		},
		SubagentFinish: usecase.SubagentFinish{Ledger: d.Ledger, Clock: d.Clock, Errors: d.Errors},
	}
	for _, uc := range []any{h.SessionStart, h.ModelSwitch, h.PromptSubmit, h.SubagentStart, h.SubagentFinish} {
		if _, err := complete(uc); err != nil {
			return hook.Handlers{}, err
		}
	}
	return h, nil
}

// doctorDeps are what the doctor is built from.
type doctorDeps struct {
	dig.In

	Paths     Paths
	Config    repository.ConfigProvider
	Env       repository.EnvReader
	Resolver  usecase.SessionResolver
	Usage     repository.UsageReader
	Artifacts repository.ArtifactStore
	Agents    repository.AgentSource
	Ledger    repository.LedgerRepository
	Clock     repository.Clock
}

func newDoctor(d doctorDeps) (usecase.DoctorUseCase, error) {
	return complete(usecase.DoctorUseCase{
		Config: d.Config, Env: d.Env, Resolver: d.Resolver, Usage: d.Usage, Artifacts: d.Artifacts,
		Agents: d.Agents, Ledger: d.Ledger, Clock: d.Clock,
		HookFailures:     failurelog.File{Path: d.Paths.LastError},
		LauncherFailures: failurelog.File{Path: d.Paths.LauncherError},
		DataDir:          d.Paths.DataDir,
		LedgerPath:       d.Paths.Ledger,
	})
}

// complete returns the use case unless one of its ports (an interface or func
// field) was left nil: a port added to a use case and forgotten here is an
// error when the use case is built, naming the field, instead of a nil
// dereference in the middle of a hook.
func complete[T any](uc T) (T, error) {
	v := reflect.ValueOf(uc)
	for i := range v.NumField() {
		f := v.Field(i)
		if (f.Kind() == reflect.Interface || f.Kind() == reflect.Func) && f.IsNil() {
			var zero T
			return zero, fmt.Errorf("di: %s.%s is not wired", v.Type().Name(), v.Type().Field(i).Name)
		}
	}
	return uc, nil
}

// ---------------------------------------------------------------- scope

// closers are what a scope releases when it closes: what was built, last
// first.
type closers struct {
	mu  sync.Mutex
	fns []func() error
}

func (c *closers) add(fn func() error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fns = append(c.fns, fn)
}

func (c *closers) close() error {
	c.mu.Lock()
	fns := c.fns
	c.fns = nil
	c.mu.Unlock()
	var errs []error
	for _, fn := range slices.Backward(fns) {
		errs = append(errs, fn())
	}
	return errors.Join(errs...)
}

// Option changes what a scope is built from, before anything is built.
type Option func(*Scope) error

var errorType = reflect.TypeFor[error]()

// Replace makes the scope use what ctor returns instead of what it would
// build. ctor is a function returning the type it replaces (and optionally an
// error); when it takes no arguments, the replaced value is never built (a
// fake ledger never opens the database).
//
// dig accepts a replacement for a type nothing provides and then never uses
// it, so a test replacing a mistyped port would run against the real one and
// pass. Replace refuses it instead.
func Replace(ctor any) Option {
	return func(s *Scope) error {
		t := reflect.TypeOf(ctor)
		if t == nil || t.Kind() != reflect.Func || t.NumOut() == 0 || t.NumOut() > 2 || (t.NumOut() == 2 && t.Out(1) != errorType) {
			return fmt.Errorf("Replace(%T): want a function returning the replaced type (and an error)", ctor)
		}
		if !s.provides[t.Out(0)] {
			return fmt.Errorf("Replace(%T): nothing provides %v", ctor, t.Out(0))
		}
		return s.c.Decorate(ctor)
	}
}

// Scope builds what one command runs (cli.Scope).
type Scope struct {
	c        *dig.Container
	closers  *closers
	provides map[reflect.Type]bool
}

// New returns a scope. Nothing is built until it is asked for. A constructor
// that dig rejects is a mistake in this package, so New panics on it.
func New(opts ...Option) *Scope {
	s := &Scope{c: dig.New(), closers: &closers{}, provides: map[reflect.Type]bool{}}
	must := func(err error) {
		if err != nil {
			panic(fmt.Sprintf("di: %v", err))
		}
	}
	must(s.c.Provide(func() *closers { return s.closers }))
	for _, ctor := range slices.Concat(infrastructure, application) {
		must(s.c.Provide(ctor))
		s.provides[reflect.TypeOf(ctor).Out(0)] = true
	}
	for _, opt := range opts {
		must(opt(s))
	}
	return s
}

// Resolve builds a T, or says why it could not. It is for tests and for this
// package: the command line goes through cli.Scope, and depguard keeps
// entry points from importing this package.
//
// The error is the one the failing constructor returned. dig's own wrapping
// names its functions by source path and line, and the error is written to
// the ledger and the failure file, where a path on this machine is noise.
func Resolve[T any](s *Scope) (T, error) {
	var v T
	if err := s.c.Invoke(func(built T) { v = built }); err != nil {
		return v, dig.RootCause(err)
	}
	return v, nil
}

// Close is part of cli.Scope.
func (s *Scope) Close() error { return s.closers.close() }

// Settings is part of cli.Scope. The process settings fall back to the
// defaults and cannot fail; a failure here is a wiring mistake.
func (s *Scope) Settings() model.Settings {
	st, err := Resolve[model.Settings](s)
	if err != nil {
		panic(fmt.Sprintf("di: process settings: %v", err))
	}
	return st
}

// Hook is part of cli.Scope.
func (s *Scope) Hook() (hook.Handlers, error) { return Resolve[hook.Handlers](s) }

// RecordError is part of cli.Scope.
func (s *Scope) RecordError() (usecase.RecordErrorUseCase, error) {
	return Resolve[usecase.RecordErrorUseCase](s)
}

// FailureRecorder is part of cli.Scope.
func (s *Scope) FailureRecorder() (repository.FailureRecorder, error) {
	return Resolve[repository.FailureRecorder](s)
}

// Clock is part of cli.Scope.
func (s *Scope) Clock() (repository.Clock, error) { return Resolve[repository.Clock](s) }

// Cloud is part of cli.Scope.
func (s *Scope) Cloud() (usecase.CloudUseCase, error) { return Resolve[usecase.CloudUseCase](s) }

// Cost is part of cli.Scope.
func (s *Scope) Cost() (usecase.CostUseCase, error) { return Resolve[usecase.CostUseCase](s) }

// Doctor is part of cli.Scope.
func (s *Scope) Doctor() (usecase.DoctorUseCase, error) { return Resolve[usecase.DoctorUseCase](s) }

// Eval is part of cli.Scope.
func (s *Scope) Eval() (usecase.EvalUseCase, error) { return Resolve[usecase.EvalUseCase](s) }

// Explain is part of cli.Scope.
func (s *Scope) Explain() (usecase.ExplainUseCase, error) { return Resolve[usecase.ExplainUseCase](s) }

// Feed is part of cli.Scope.
func (s *Scope) Feed() (usecase.FeedUseCase, error) { return Resolve[usecase.FeedUseCase](s) }

// Lint is part of cli.Scope.
func (s *Scope) Lint() (usecase.LintUseCase, error) { return Resolve[usecase.LintUseCase](s) }

// Policy is part of cli.Scope.
func (s *Scope) Policy() (usecase.PolicyUseCase, error) { return Resolve[usecase.PolicyUseCase](s) }

// Query is part of cli.Scope.
func (s *Scope) Query() (usecase.QueryUseCase, error) { return Resolve[usecase.QueryUseCase](s) }

// Report is part of cli.Scope.
func (s *Scope) Report() (usecase.ReportUseCase, error) { return Resolve[usecase.ReportUseCase](s) }

// Train is part of cli.Scope.
func (s *Scope) Train() (usecase.TrainUseCase, error) { return Resolve[usecase.TrainUseCase](s) }

// Verify is part of cli.Scope.
func (s *Scope) Verify() (usecase.VerifyUseCase, error) { return Resolve[usecase.VerifyUseCase](s) }
