// Package repository declares the ports the domain needs from the outside
// world. Implementations live in the infrastructure layer; the application
// layer depends only on these interfaces (dependency inversion).
//
// The ports are small on purpose (interface segregation): a use case names
// the one or two capabilities it uses, so a fake for its test implements only
// those, and a change to one capability does not ripple into unrelated use
// cases. The wide interfaces (LedgerRepository, ConfigProvider, ArtifactStore)
// are compositions kept for the adapters and the composition root.
package repository

import (
	"context"
	"errors"
	"iter"
	"time"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/learn"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/service"
)

// ---------------------------------------------------------------- sessions

// SessionFinder reads the observed model of a session.
type SessionFinder interface {
	Find(ctx context.Context, id string) (model.Session, bool, error)
}

// SessionWriter records the observed model of a session.
type SessionWriter interface {
	Save(ctx context.Context, s model.Session) error
}

// RetentionPruner applies a retention policy: it deletes what is older than
// the cutoff and says how many rows went (the ledger and the sessions).
type RetentionPruner interface {
	PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

// SessionRepository stores the observed model of each session.
type SessionRepository interface {
	SessionFinder
	SessionWriter
	RetentionPruner
}

// ---------------------------------------------------------------- ledger

// LedgerWriter appends to the decision ledger.
type LedgerWriter interface {
	Append(ctx context.Context, e model.Entry) error
}

// LedgerReader reads the decision ledger.
type LedgerReader interface {
	// Since yields entries at or after the given time in insertion order
	// (the order they were appended, not the order of their timestamps).
	// The time bound is exact to the second; use LedgerFeed to follow the
	// ledger without repeating or skipping entries.
	Since(ctx context.Context, from time.Time) iter.Seq2[model.Entry, error]
}

// Positioned is a ledger entry with its position: positions grow with every
// append, so "after position p" never repeats nor skips an entry.
type Positioned struct {
	Pos   uint
	Entry model.Entry
}

// LedgerFeed follows the ledger by position (the live feed of `pmr serve`).
type LedgerFeed interface {
	// Head is the position of the newest entry (0 when the ledger is empty).
	Head(ctx context.Context) (uint, error)
	// After yields entries whose position is greater than pos and whose time
	// is at or after from, in insertion order.
	After(ctx context.Context, pos uint, from time.Time) iter.Seq2[Positioned, error]
}

// PromptHistory answers whether a brief was delegated before.
type PromptHistory interface {
	// SeenPrompt reports whether this session already delegated the same brief.
	SeenPrompt(ctx context.Context, sessionID, promptSHA string) (bool, error)
}

// LedgerVerifier checks the ledger's tamper evidence.
type LedgerVerifier interface {
	// Verify walks the hash chain; brokenID is 0 when the chain is intact.
	Verify(ctx context.Context) (checked int, brokenID uint, err error)
}

// LedgerRepository is the append-only decision ledger with every capability.
type LedgerRepository interface {
	LedgerWriter
	LedgerReader
	LedgerFeed
	PromptHistory
	RetentionPruner
	LedgerVerifier
}

// QueryResult is the outcome of a read-only ledger query.
type QueryResult struct {
	Columns []string
	Rows    []map[string]any
}

// LedgerQuery runs ad-hoc read-only SQL against the ledger (`pmr query`).
type LedgerQuery interface {
	// Query runs one read-only statement and returns at most limit rows.
	Query(ctx context.Context, sql string, limit int) (QueryResult, error)
	// Schema returns the CREATE statements of the ledger tables.
	Schema(ctx context.Context) ([]string, error)
}

// ---------------------------------------------------------------- configuration

// SettingsProvider supplies the layered TOML settings for a working directory.
type SettingsProvider interface {
	Settings(cwd string) model.Settings
}

// LexiconProvider supplies the compiled lexicon for a working directory.
type LexiconProvider interface {
	Lexicon(cwd string) *service.Lexicon
}

// TierProvider supplies the tier table that ships with the plugin.
type TierProvider interface {
	Tiers() model.TierTable
}

// PriceProvider supplies the price table that ships with the plugin.
type PriceProvider interface {
	Prices() service.PriceTable
}

// ConfigDiagnostics reports which configuration files were read.
type ConfigDiagnostics interface {
	// Sources lists the files that were applied; Problems lists files that
	// were rejected (unknown keys, syntax errors) so `pmr doctor` can say so.
	Sources(cwd string) []string
	Problems(cwd string) []string
}

// RoutingConfig is what running the routing workflow reads.
type RoutingConfig interface {
	SettingsProvider
	LexiconProvider
	TierProvider
}

// ConfigProvider supplies the layered TOML configuration for a working
// directory as domain values, plus the data tables that ship with the plugin.
type ConfigProvider interface {
	RoutingConfig
	PriceProvider
	ConfigDiagnostics
}

// EnvReader reads the process environment.
type EnvReader interface {
	Getenv(key string) string
}

// ---------------------------------------------------------------- observations

// TranscriptReader reads the model of the latest main-thread assistant turn.
type TranscriptReader interface {
	LatestModel(path string) (string, bool)
}

// UsageReader reads the plan-usage caches a status line writes.
type UsageReader interface {
	Claude() model.Pressure
	Codex() model.CodexQuota
}

// SettingsReader reads model settings Claude Code itself uses, as fallbacks.
type SettingsReader interface {
	EnvModel() (string, bool)
	SettingsModel(cwd string) (string, bool)
}

// ---------------------------------------------------------------- learning data

// ArtifactLoader loads the learned routing artifact.
type ArtifactLoader interface {
	Load() (model.Artifact, error)
}

// ArtifactWriter saves a learned routing artifact and returns where it went.
type ArtifactWriter interface {
	Save(a model.Artifact) (string, error)
}

// ArtifactStore loads and saves the learned routing artifact.
type ArtifactStore interface {
	ArtifactLoader
	ArtifactWriter
}

// CaseSource reads labelled prompts (JSONL). The path "" is the evaluation
// set that ships with the plugin.
type CaseSource interface {
	Cases(path string) ([]learn.Case, error)
}

// ErrNoFrontmatter is returned for an agent definition without a YAML
// frontmatter block.
var ErrNoFrontmatter = errors.New("no frontmatter")

// AgentSource reads the fixed-tier agent definitions that ship with the plugin.
type AgentSource interface {
	// AgentSpec returns the model and effort declared in the frontmatter of
	// agents/<name>.md: an error wrapping fs.ErrNotExist when the file is
	// missing, ErrNoFrontmatter when it has no frontmatter.
	AgentSpec(name string) (model.AgentSpec, error)
}

// ---------------------------------------------------------------- failures outside the ledger

// FailureRecorder keeps the last failure in a plain file, for when the ledger
// itself cannot be written (or opened).
type FailureRecorder interface {
	RecordFailure(f model.Failure) error
}

// FailureReader reads the last failure a component left (ok is false when
// there is none).
type FailureReader interface {
	LastFailure() (f model.Failure, ok bool, err error)
}

// ---------------------------------------------------------------- time

// Clock abstracts time for deterministic tests.
type Clock interface {
	Now() time.Time
}
