package repository_test

import (
	"context"
	"iter"
	"testing"
	"time"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/learn"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/repository"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/service"
)

// The package declares only interfaces (no statements to cover). These
// minimal fakes pin the method sets: a changed signature breaks this file,
// which is the contract the infrastructure adapters and test doubles rely on.

type fakeSessions struct{}

func (fakeSessions) Find(context.Context, string) (model.Session, bool, error) {
	return model.Session{}, false, nil
}
func (fakeSessions) Save(context.Context, model.Session) error                { return nil }
func (fakeSessions) PruneOlderThan(context.Context, time.Time) (int64, error) { return 0, nil }

type fakeLedger struct{}

func (fakeLedger) Append(context.Context, model.Entry) error { return nil }
func (fakeLedger) Since(context.Context, time.Time) iter.Seq2[model.Entry, error] {
	return func(func(model.Entry, error) bool) {}
}
func (fakeLedger) PruneOlderThan(context.Context, time.Time) (int64, error) { return 0, nil }
func (fakeLedger) SeenPrompt(context.Context, string, string) (bool, error) { return false, nil }
func (fakeLedger) Verify(context.Context) (int, uint, error)                { return 0, 0, nil }
func (fakeLedger) Head(context.Context) (uint, error)                       { return 0, nil }
func (fakeLedger) After(context.Context, uint, time.Time) iter.Seq2[repository.Positioned, error] {
	return func(func(repository.Positioned, error) bool) {}
}

type fakeConfig struct{}

func (fakeConfig) Settings(string) model.Settings  { return model.Settings{} }
func (fakeConfig) Lexicon(string) *service.Lexicon { return nil }
func (fakeConfig) Tiers() model.TierTable          { return model.TierTable{} }
func (fakeConfig) Prices() service.PriceTable      { return service.PriceTable{} }
func (fakeConfig) Sources(string) []string         { return nil }
func (fakeConfig) Problems(string) []string        { return nil }

type fakeTranscript struct{}

func (fakeTranscript) LatestModel(string) (string, bool) { return "", false }

type fakeUsage struct{}

func (fakeUsage) Claude() model.Pressure  { return model.Pressure{} }
func (fakeUsage) Codex() model.CodexQuota { return model.CodexQuota{} }

type fakeSettings struct{}

func (fakeSettings) EnvModel() (string, bool)            { return "", false }
func (fakeSettings) SettingsModel(string) (string, bool) { return "", false }

type fakeArtifacts struct{}

func (fakeArtifacts) Load() (model.Artifact, error)       { return model.Artifact{}, nil }
func (fakeArtifacts) Save(model.Artifact) (string, error) { return "", nil }

type fakeQuery struct{}

func (fakeQuery) Query(context.Context, string, int) (repository.QueryResult, error) {
	return repository.QueryResult{}, nil
}
func (fakeQuery) Schema(context.Context) ([]string, error) { return nil, nil }

type fakeEnv struct{}

func (fakeEnv) Getenv(string) string { return "" }

type fakeCases struct{}

func (fakeCases) Cases(string) ([]learn.Case, error) { return nil, nil }

type fakeAgents struct{}

func (fakeAgents) AgentSpec(string) (model.AgentSpec, error) { return model.AgentSpec{}, nil }

// writeOnly saves but cannot load: the export `pmr train --output` writes.
type writeOnly struct{}

func (writeOnly) Save(model.Artifact) (string, error) { return "", nil }

type fakeFailures struct{}

func (fakeFailures) RecordFailure(model.Failure) error         { return nil }
func (fakeFailures) LastFailure() (model.Failure, bool, error) { return model.Failure{}, false, nil }

type fakeClock struct{}

func (fakeClock) Now() time.Time { return time.Time{} }

type notAPort struct{}

func TestPortMethodSets(t *testing.T) {
	tests := []struct {
		name string
		impl any
		port func(any) bool
		want bool
	}{
		{"SessionRepository", fakeSessions{}, func(v any) bool { _, ok := v.(repository.SessionRepository); return ok }, true},
		{"LedgerRepository", fakeLedger{}, func(v any) bool { _, ok := v.(repository.LedgerRepository); return ok }, true},
		{"ConfigProvider", fakeConfig{}, func(v any) bool { _, ok := v.(repository.ConfigProvider); return ok }, true},
		{"TranscriptReader", fakeTranscript{}, func(v any) bool { _, ok := v.(repository.TranscriptReader); return ok }, true},
		{"UsageReader", fakeUsage{}, func(v any) bool { _, ok := v.(repository.UsageReader); return ok }, true},
		{"SettingsReader", fakeSettings{}, func(v any) bool { _, ok := v.(repository.SettingsReader); return ok }, true},
		{"ArtifactStore", fakeArtifacts{}, func(v any) bool { _, ok := v.(repository.ArtifactStore); return ok }, true},
		{"Clock", fakeClock{}, func(v any) bool { _, ok := v.(repository.Clock); return ok }, true},
		{"LedgerQuery", fakeQuery{}, func(v any) bool { _, ok := v.(repository.LedgerQuery); return ok }, true},
		{"EnvReader", fakeEnv{}, func(v any) bool { _, ok := v.(repository.EnvReader); return ok }, true},
		{"CaseSource", fakeCases{}, func(v any) bool { _, ok := v.(repository.CaseSource); return ok }, true},
		{"AgentSource", fakeAgents{}, func(v any) bool { _, ok := v.(repository.AgentSource); return ok }, true},
		{"ArtifactWriter", writeOnly{}, func(v any) bool { _, ok := v.(repository.ArtifactWriter); return ok }, true},
		{"a writer is not a loader", writeOnly{}, func(v any) bool { _, ok := v.(repository.ArtifactLoader); return ok }, false},
		{"FailureRecorder", fakeFailures{}, func(v any) bool { _, ok := v.(repository.FailureRecorder); return ok }, true},
		{"FailureReader", fakeFailures{}, func(v any) bool { _, ok := v.(repository.FailureReader); return ok }, true},
		{"a session repository writes and prunes", fakeSessions{}, func(v any) bool {
			_, w := v.(repository.SessionWriter)
			_, m := v.(repository.RetentionPruner)
			return w && m
		}, true},
		{"a ledger is a feed", fakeLedger{}, func(v any) bool { _, ok := v.(repository.LedgerFeed); return ok }, true},
		{"a ledger is a reader", fakeLedger{}, func(v any) bool { _, ok := v.(repository.LedgerReader); return ok }, true},
		{"a session repository finds sessions", fakeSessions{}, func(v any) bool { _, ok := v.(repository.SessionFinder); return ok }, true},
		{"the config provider is a routing config", fakeConfig{}, func(v any) bool { _, ok := v.(repository.RoutingConfig); return ok }, true},
		{"a session repository is not a ledger", fakeSessions{}, func(v any) bool { _, ok := v.(repository.LedgerRepository); return ok }, false},
		{"an empty type is not a clock", notAPort{}, func(v any) bool { _, ok := v.(repository.Clock); return ok }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.port(tt.impl); got != tt.want {
				t.Errorf("satisfies = %v, want %v", got, tt.want)
			}
		})
	}
}
