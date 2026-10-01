package usecase_test

import (
	"context"
	"errors"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/application/usecase"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/repository"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/service"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/agents"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/clock"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/settings"
)

// In-memory fakes for the domain ports: the use cases are exercised without
// SQLite, which is what the dependency inversion is for. Every fake can also
// fail, so the error branches of the use cases are reachable.

var (
	errLedger   = errors.New("ledger down")
	errSessions = errors.New("sessions down")
	errArtifact = errors.New("no artifact")
)

var testNow = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

type memLedger struct {
	entries []model.Entry
	// sinceErr is yielded after the entries.
	sinceErr  error
	appendErr error
	seenErr   error
	headErr   error
	pruneErr  error
	checked   int
	broken    uint
	verifyErr error
	pruned    []time.Time
}

func (m *memLedger) Append(_ context.Context, e model.Entry) error {
	if m.appendErr != nil {
		return m.appendErr
	}
	m.entries = append(m.entries, e)
	return nil
}

func (m *memLedger) Head(context.Context) (uint, error) { return uint(len(m.entries)), m.headErr }

// After treats the position of entries[i] as i+1.
func (m *memLedger) After(_ context.Context, pos uint, from time.Time) iter.Seq2[repository.Positioned, error] {
	return func(yield func(repository.Positioned, error) bool) {
		for i := int(pos); i < len(m.entries); i++ {
			if !m.entries[i].At.Before(from) && !yield(repository.Positioned{Pos: uint(i + 1), Entry: m.entries[i]}, nil) {
				return
			}
		}
		if m.sinceErr != nil {
			yield(repository.Positioned{}, m.sinceErr)
		}
	}
}

func (m *memLedger) Since(_ context.Context, from time.Time) iter.Seq2[model.Entry, error] {
	return func(yield func(model.Entry, error) bool) {
		for i := range m.entries {
			if !m.entries[i].At.Before(from) && !yield(m.entries[i], nil) {
				return
			}
		}
		if m.sinceErr != nil {
			yield(model.Entry{}, m.sinceErr)
		}
	}
}

func (m *memLedger) PruneOlderThan(_ context.Context, cutoff time.Time) (int64, error) {
	m.pruned = append(m.pruned, cutoff)
	return 0, m.pruneErr
}

func (m *memLedger) SeenPrompt(_ context.Context, sid, sha string) (bool, error) {
	if m.seenErr != nil {
		return false, m.seenErr
	}
	for i := range m.entries {
		if e := &m.entries[i]; e.Event == model.EventSubagent && e.SessionID == sid && e.PromptSHA == sha {
			return true, nil
		}
	}
	return false, nil
}

func (m *memLedger) Verify(context.Context) (int, uint, error) {
	return m.checked, m.broken, m.verifyErr
}

type memSessions struct {
	m        map[string]model.Session
	findErr  error
	saveErr  error
	pruneErr error
}

func (s *memSessions) Find(_ context.Context, id string) (model.Session, bool, error) {
	if s.findErr != nil {
		return model.Session{}, false, s.findErr
	}
	v, ok := s.m[id]
	return v, ok, nil
}

func (s *memSessions) Save(_ context.Context, v model.Session) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.m[v.ID] = v
	return nil
}

func (s *memSessions) PruneOlderThan(context.Context, time.Time) (int64, error) { return 0, s.pruneErr }

type fakeTranscript struct{ model string }

func (f fakeTranscript) LatestModel(string) (string, bool) { return f.model, f.model != "" }

type fakeUsage struct {
	claude model.Pressure
	codex  model.CodexQuota
}

func (f fakeUsage) Claude() model.Pressure  { return f.claude }
func (f fakeUsage) Codex() model.CodexQuota { return f.codex }

type fakeSettings struct{ env, file string }

func (f fakeSettings) EnvModel() (string, bool)            { return f.env, f.env != "" }
func (f fakeSettings) SettingsModel(string) (string, bool) { return f.file, f.file != "" }

// fakeEnv is the process environment as a map.
type fakeEnv map[string]string

func (e fakeEnv) Getenv(k string) string { return e[k] }

// mapAgents serves agent specs from memory; a missing name is missing, a
// nil spec pointer has no frontmatter, a spec with Class "unreadable" fails.
type mapAgents map[string]*model.AgentSpec

func (m mapAgents) AgentSpec(name string) (model.AgentSpec, error) {
	v, ok := m[name]
	switch {
	case !ok:
		return model.AgentSpec{}, fs.ErrNotExist
	case v == nil:
		return model.AgentSpec{}, repository.ErrNoFrontmatter
	case v.Class == "unreadable":
		return model.AgentSpec{}, errors.New("permission denied")
	}
	return *v, nil
}

type fakeArtifacts struct {
	art     model.Artifact
	loadErr error
	saved   []model.Artifact
	path    string
	saveErr error
}

func (f *fakeArtifacts) Load() (model.Artifact, error) { return f.art, f.loadErr }
func (f *fakeArtifacts) Save(a model.Artifact) (string, error) {
	f.saved = append(f.saved, a)
	return f.path, f.saveErr
}

// fakeConfig is the real layered provider with test overrides on top.
type fakeConfig struct {
	*settings.Provider
	mutate   func(*model.Settings)
	tiers    *model.TierTable
	sources  []string
	problems []string
}

func (c fakeConfig) Settings(cwd string) model.Settings {
	st := c.Provider.Settings(cwd)
	if c.mutate != nil {
		c.mutate(&st)
	}
	return st
}

func (c fakeConfig) Lexicon(cwd string) *service.Lexicon { return c.Provider.Lexicon(cwd) }

func (c fakeConfig) Tiers() model.TierTable {
	if c.tiers != nil {
		return *c.tiers
	}
	return c.Provider.Tiers()
}

func (c fakeConfig) Sources(string) []string  { return c.sources }
func (c fakeConfig) Problems(string) []string { return c.problems }

// env isolates the configuration from the developer's machine.
func env(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_PROJECT_DIR", t.TempDir())
}

// deps are the fakes a fixture hands to the use cases (each use case takes
// only the ones it needs).
type deps struct {
	Sessions  *memSessions
	Resolver  usecase.SessionResolver
	Ledger    *memLedger
	Usage     repository.UsageReader
	Artifacts *fakeArtifacts
	Config    fakeConfig
	Clock     repository.Clock
	Env       repository.EnvReader
}

// fixture is a set of fakes and the use cases wired to them.
type fixture struct {
	ledger    *memLedger
	sessions  *memSessions
	artifacts *fakeArtifacts
	config    fakeConfig
	failures  *memFailures // last_error
	launcher  *memFailures // launcher_error
	deps      deps
}

// fixtureOpt adjusts a fixture before the use cases are assembled.
type fixtureOpt func(*fixture)

func newFixture(t *testing.T, opts ...fixtureOpt) *fixture {
	t.Helper()
	env(t)
	f := &fixture{
		ledger:    &memLedger{},
		sessions:  &memSessions{m: map[string]model.Session{}},
		artifacts: &fakeArtifacts{},
		config:    fakeConfig{Provider: settings.NewProvider()},
		failures:  &memFailures{},
		launcher:  &memFailures{},
	}
	f.deps = deps{
		Resolver: usecase.SessionResolver{Transcript: fakeTranscript{}, Settings: fakeSettings{}},
		Usage:    fakeUsage{codex: model.CodexQuota{Available: true}},
		Clock:    clock.Fixed{At: testNow}, Env: fakeEnv{},
	}
	for _, o := range opts {
		o(f)
	}
	f.deps.Sessions, f.deps.Ledger, f.deps.Artifacts, f.deps.Config = f.sessions, f.ledger, f.artifacts, f.config
	f.deps.Resolver.Sessions = f.sessions
	return f
}

func (f *fixture) recordError() usecase.RecordErrorUseCase {
	return usecase.RecordErrorUseCase{
		Ledger: f.ledger, Failures: f.failures, Clock: f.deps.Clock,
		DetailRunes: f.config.Settings("").Runtime.ErrorDetailRunes, Timeout: time.Second,
	}
}

func (f *fixture) sessionStart() usecase.SessionStart {
	return usecase.SessionStart{
		Sessions: f.sessions, SessionRetention: f.sessions, LedgerRetention: f.ledger, Ledger: f.ledger,
		Config: f.config, Env: f.deps.Env, Clock: f.deps.Clock, Errors: f.recordError(),
	}
}

func (f *fixture) modelSwitch() usecase.ModelSwitch {
	return usecase.ModelSwitch{Sessions: f.sessions, Ledger: f.ledger, Clock: f.deps.Clock, Errors: f.recordError()}
}

func (f *fixture) promptSubmit() usecase.PromptSubmit {
	return usecase.PromptSubmit{
		Config: f.config, Resolver: f.deps.Resolver, Usage: f.deps.Usage, Ledger: f.ledger, Clock: f.deps.Clock, Errors: f.recordError(),
	}
}

func (f *fixture) subagentStart() usecase.SubagentStart {
	return usecase.SubagentStart{
		Config: f.config, Resolver: f.deps.Resolver, Artifacts: f.artifacts, History: f.ledger, Ledger: f.ledger,
		Env: f.deps.Env, Clock: f.deps.Clock, Errors: f.recordError(),
	}
}

func (f *fixture) subagentFinish() usecase.SubagentFinish {
	return usecase.SubagentFinish{Ledger: f.ledger, Clock: f.deps.Clock, Errors: f.recordError()}
}

// doctor wires the doctor to the fixture's fakes and the shipped agents.
func (f *fixture) doctor() usecase.DoctorUseCase {
	return usecase.DoctorUseCase{
		Config: f.config, Env: f.deps.Env, Resolver: f.deps.Resolver, Usage: f.deps.Usage,
		Artifacts: f.artifacts, Agents: agents.New(), Ledger: f.ledger,
		HookFailures: f.failures, LauncherFailures: f.launcher, Clock: f.deps.Clock,
		DataDir: "/data", LedgerPath: "/data/ledger.db",
	}
}

// memFailures is the failure file in memory.
type memFailures struct {
	last    *model.Failure
	readErr error
}

func (m *memFailures) RecordFailure(f model.Failure) error { m.last = &f; return nil }

func (m *memFailures) LastFailure() (model.Failure, bool, error) {
	if m.readErr != nil || m.last == nil {
		return model.Failure{}, false, m.readErr
	}
	return *m.last, true, nil
}

// writeFile writes content to a fresh file and returns its path.
func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "cases.jsonl")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func withEnv(kv map[string]string) fixtureOpt {
	return func(f *fixture) { f.deps.Env = fakeEnv(kv) }
}

func withSettings(mut func(*model.Settings)) fixtureOpt {
	return func(f *fixture) { f.config.mutate = mut }
}
