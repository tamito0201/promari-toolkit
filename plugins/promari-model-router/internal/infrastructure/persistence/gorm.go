// Package persistence implements the repositories with GORM on SQLite. The
// driver is ncruces/go-sqlite3 (SQLite compiled to WebAssembly, run by
// wazero): pure Go, so the plugin cross-compiles without cgo. Measured on an
// Apple M-series laptop: open + migrate + insert ≈ 1.6 ms per hook process.
package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/ncruces/go-sqlite3/gormlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/repository"
)

// DB owns the GORM handle. The handle is not exported: the repositories in
// this package are the only code that issues SQL against the ledger.
type DB struct {
	g   *gorm.DB
	opt Options
}

// Options tune the database connection (from [runtime] in the settings).
type Options struct {
	BusyTimeoutMS int
	Batch         int
}

// fileURI builds the `file:` URI the driver opens. The path is escaped (a
// directory named "a?mode=ro" or "b#c" must not turn into URI syntax); the
// query carries the driver parameters.
func fileURI(path, query string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: query}).String()
}

// Open opens (and creates, 0600) the SQLite database with WAL and a busy
// timeout, because several hooks may write at the same moment.
//
// Transactions begin IMMEDIATE (_txlock): Append reads the chain head and then
// inserts. A deferred transaction takes the write lock only at the insert, and
// two processes that both read first deadlock on the upgrade; SQLite answers
// SQLITE_BUSY at once (the busy timeout does not cover it), and the row was
// lost. An immediate transaction takes the write lock before the read, so the
// second writer waits for the busy timeout instead.
func Open(path string, opt Options) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600); err == nil {
		_ = f.Close()
	}
	dsn := fileURI(path, fmt.Sprintf("_pragma=busy_timeout(%d)&_pragma=journal_mode(wal)&_pragma=synchronous(normal)&_txlock=immediate", opt.BusyTimeoutMS))
	gdb, err := gorm.Open(gormlite.Open(dsn), &gorm.Config{Logger: logger.Discard, SkipDefaultTransaction: true})
	if err != nil {
		return nil, fmt.Errorf("open ledger database: %w", err)
	}
	if err := gdb.AutoMigrate(&SessionRow{}, &EntryRow{}); err != nil {
		return nil, fmt.Errorf("migrate ledger database: %w", err)
	}
	return &DB{g: gdb, opt: opt}, nil
}

// OpenReadOnly opens the database for ad-hoc queries (`pmr query`).
func OpenReadOnly(path string, opt Options) (*DB, error) {
	dsn := fileURI(path, fmt.Sprintf("mode=ro&_pragma=busy_timeout(%d)&_pragma=query_only(1)", opt.BusyTimeoutMS))
	gdb, err := gorm.Open(gormlite.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return nil, fmt.Errorf("open ledger read-only: %w", err)
	}
	return &DB{g: gdb, opt: opt}, nil
}

// Close closes the underlying connection.
func (db *DB) Close() error {
	sqlDB, err := db.g.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// SessionRow is the sessions table. UpdatedAt comes from the injected clock:
// autoUpdateTime is off because GORM would otherwise overwrite it with the wall
// clock on every upsert (OnConflict UpdateAll), ignoring Session.UpdatedAt.
type SessionRow struct {
	ID        string `gorm:"primaryKey"`
	Model     string
	Source    string
	UpdatedAt time.Time `gorm:"index;autoUpdateTime:false"`
}

// TableName pins the table name.
func (SessionRow) TableName() string { return "sessions" }

// EntryRow is the ledger table. Hash chains every row to the previous one so
// that editing or deleting a row is detectable (`pmr verify`). HashVersion
// names the form the hash was computed in (see RowHash); rows written before
// the column existed read as HashV1.
type EntryRow struct {
	ID              uint      `gorm:"primaryKey;autoIncrement"`
	At              time.Time `gorm:"index"`
	Event           string    `gorm:"index"`
	SessionID       string    `gorm:"index"`
	ToolUseID       string    `gorm:"index"`
	SessionModel    string
	SessionSource   string
	Class           string `gorm:"index"`
	Confidence      int
	Margin          int
	Danger          bool
	Codex           string
	Continuation    bool
	Lang            string
	PromptChars     int
	PromptSHA       string `gorm:"index"`
	Advised         bool
	PressureHigh    bool
	Action          string `gorm:"index"`
	Reason          string
	Target          string
	SubagentType    string
	Requested       string
	Resolved        string
	Mismatch        *bool
	Status          string
	TotalTokens     int
	InputTokens     int
	OutputTokens    int
	CacheReadTokens int
	DurationMS      int
	Nested          bool
	Detail          string
	PrevHash        string
	Hash            string `gorm:"index"`
	HashVersion     int    `gorm:"not null;default:1"`
}

// TableName pins the table name.
func (EntryRow) TableName() string { return "ledger" }

func toRow(e model.Entry) EntryRow {
	return EntryRow{
		At: e.At.UTC(), Event: string(e.Event), SessionID: e.SessionID, ToolUseID: e.ToolUseID,
		SessionModel: e.SessionModel, SessionSource: string(e.SessionSource), Class: string(e.Class),
		Confidence: e.Confidence, Margin: e.Margin, Danger: e.Danger, Codex: string(e.Codex),
		Continuation: e.Continuation, Lang: string(e.Lang), PromptChars: e.PromptChars, PromptSHA: e.PromptSHA,
		Advised: e.Advised, PressureHigh: e.PressureHigh, Action: string(e.Action), Reason: e.Reason,
		Target: string(e.Target), SubagentType: e.SubagentType, Requested: e.Requested, Resolved: e.Resolved,
		Mismatch: mismatchColumn(e.Mismatch), Status: string(e.Status), TotalTokens: e.TotalTokens, InputTokens: e.InputTokens,
		OutputTokens: e.OutputTokens, CacheReadTokens: e.CacheReadTokens, DurationMS: e.DurationMS,
		Nested: e.Nested, Detail: e.Detail,
	}
}

// mismatchColumn stores the tri-state as a nullable boolean (a fresh pointer
// per row, never shared with the entry).
func mismatchColumn(m model.Mismatch) *bool {
	if !m.Known() {
		return nil
	}
	return new(m.Yes())
}

func mismatchValue(b *bool) model.Mismatch {
	switch {
	case b == nil:
		return model.MismatchUnknown
	case *b:
		return model.MismatchYes
	default:
		return model.MismatchNo
	}
}

func fromRow(r EntryRow) model.Entry {
	return model.Entry{
		At: r.At, Event: model.EventKind(r.Event), SessionID: r.SessionID, ToolUseID: r.ToolUseID,
		SessionModel: r.SessionModel, SessionSource: model.SessionSource(r.SessionSource), Class: model.Class(r.Class),
		Confidence: r.Confidence, Margin: r.Margin, Danger: r.Danger, Codex: model.CodexKind(r.Codex),
		Continuation: r.Continuation, Lang: model.Language(r.Lang), PromptChars: r.PromptChars, PromptSHA: r.PromptSHA,
		Advised: r.Advised, PressureHigh: r.PressureHigh, Action: model.Action(r.Action), Reason: r.Reason,
		Target: model.Tier(r.Target), SubagentType: r.SubagentType, Requested: r.Requested, Resolved: r.Resolved,
		Mismatch: mismatchValue(r.Mismatch), Status: model.SubagentStatus(r.Status), TotalTokens: r.TotalTokens, InputTokens: r.InputTokens,
		OutputTokens: r.OutputTokens, CacheReadTokens: r.CacheReadTokens, DurationMS: r.DurationMS,
		Nested: r.Nested, Detail: r.Detail,
	}
}

// Hash forms. A row is verified in the form it was written in, so a ledger
// written by an older release stays verifiable after an upgrade.
const (
	// HashV1 covered only some columns joined by "|": the status, the token
	// counts other than the total, the flags and the session model could be
	// edited without breaking the chain, and a "|" inside a value could move
	// the boundary between two fields. Kept to verify old rows only.
	HashV1 = 1
	// HashV2 covers every column but the id and the hash itself, encoded as
	// JSON (quoted strings, so no value can move a field boundary).
	HashV2 = 2
	// hashCurrent is the form new rows are written in.
	hashCurrent = HashV2
)

// RowHash is the SHA-256 of the previous hash and the row's content in the
// row's hash form (HashVersion; 0 means the current form). An unknown form
// yields "", which no stored hash equals.
func RowHash(prev string, r EntryRow) string {
	switch r.HashVersion {
	case HashV1:
		return rowHashV1(prev, r)
	case 0, HashV2:
		return rowHashV2(prev, r)
	default:
		return ""
	}
}

func rowHashV1(prev string, r EntryRow) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%d|%d|%s|%s",
		prev, r.At.UTC().Format(time.RFC3339Nano), r.Event, r.SessionID, r.ToolUseID, r.Class, r.Action,
		r.Reason, r.Target, r.Requested, r.Resolved, r.TotalTokens, r.PromptChars, r.PromptSHA, r.Detail)
	return hex.EncodeToString(h.Sum(nil))
}

// hashedV2 is the canonical form of HashV2: every content column in a fixed
// order (encoding/json writes struct fields in declaration order).
type hashedV2 struct {
	Version         int    `json:"v"`
	Prev            string `json:"prev"`
	At              string `json:"at"`
	Event           string `json:"event"`
	SessionID       string `json:"session_id"`
	ToolUseID       string `json:"tool_use_id"`
	SessionModel    string `json:"session_model"`
	SessionSource   string `json:"session_source"`
	Class           string `json:"class"`
	Confidence      int    `json:"confidence"`
	Margin          int    `json:"margin"`
	Danger          bool   `json:"danger"`
	Codex           string `json:"codex"`
	Continuation    bool   `json:"continuation"`
	Lang            string `json:"lang"`
	PromptChars     int    `json:"prompt_chars"`
	PromptSHA       string `json:"prompt_sha"`
	Advised         bool   `json:"advised"`
	PressureHigh    bool   `json:"pressure_high"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
	Target          string `json:"target"`
	SubagentType    string `json:"subagent_type"`
	Requested       string `json:"requested"`
	Resolved        string `json:"resolved"`
	Mismatch        *bool  `json:"mismatch"`
	Status          string `json:"status"`
	TotalTokens     int    `json:"total_tokens"`
	InputTokens     int    `json:"input_tokens"`
	OutputTokens    int    `json:"output_tokens"`
	CacheReadTokens int    `json:"cache_read_tokens"`
	DurationMS      int    `json:"duration_ms"`
	Nested          bool   `json:"nested"`
	Detail          string `json:"detail"`
}

func rowHashV2(prev string, r EntryRow) string {
	raw, _ := json.Marshal(hashedV2{ //nolint:errchkjson // strings, ints, bools and a *bool always encode
		Version: HashV2, Prev: prev, At: r.At.UTC().Format(time.RFC3339Nano), Event: r.Event, SessionID: r.SessionID,
		ToolUseID: r.ToolUseID, SessionModel: r.SessionModel, SessionSource: r.SessionSource, Class: r.Class,
		Confidence: r.Confidence, Margin: r.Margin, Danger: r.Danger, Codex: r.Codex, Continuation: r.Continuation,
		Lang: r.Lang, PromptChars: r.PromptChars, PromptSHA: r.PromptSHA, Advised: r.Advised, PressureHigh: r.PressureHigh,
		Action: r.Action, Reason: r.Reason, Target: r.Target, SubagentType: r.SubagentType, Requested: r.Requested,
		Resolved: r.Resolved, Mismatch: r.Mismatch, Status: r.Status, TotalTokens: r.TotalTokens,
		InputTokens: r.InputTokens, OutputTokens: r.OutputTokens, CacheReadTokens: r.CacheReadTokens,
		DurationMS: r.DurationMS, Nested: r.Nested, Detail: r.Detail,
	})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// LedgerRepo implements repository.LedgerRepository.
type LedgerRepo struct{ db *DB }

var (
	_ repository.LedgerRepository  = (*LedgerRepo)(nil)
	_ repository.SessionRepository = (*SessionRepo)(nil)
)

// NewLedgerRepo builds the ledger repository.
func NewLedgerRepo(db *DB) *LedgerRepo { return &LedgerRepo{db: db} }

// Append inserts one row, chaining its hash to the latest row in an
// IMMEDIATE transaction (see Open), so concurrent writers from other
// processes queue instead of losing rows.
func (r *LedgerRepo) Append(ctx context.Context, e model.Entry) error {
	return r.db.g.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var last EntryRow
		prev := ""
		if err := tx.Order("id desc").Limit(1).Take(&last).Error; err == nil {
			prev = last.Hash
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		row := toRow(e)
		row.HashVersion = hashCurrent
		row.PrevHash, row.Hash = prev, RowHash(prev, row)
		return tx.Create(&row).Error
	})
}

// Since yields entries at or after from in insertion (id) order, in batches.
// The time bound compares the stored RFC 3339 text, which drops trailing
// zeros of the fraction, so it is exact only to the second; a caller that
// must neither repeat nor skip rows follows positions (After) instead.
func (r *LedgerRepo) Since(ctx context.Context, from time.Time) iter.Seq2[model.Entry, error] {
	return func(yield func(model.Entry, error) bool) {
		for p, err := range r.After(ctx, 0, from) {
			if !yield(p.Entry, err) || err != nil {
				return
			}
		}
	}
}

// Head is the position (id) of the newest row, 0 when the ledger is empty.
func (r *LedgerRepo) Head(ctx context.Context) (uint, error) {
	var head uint
	err := r.db.g.WithContext(ctx).Model(&EntryRow{}).Select("coalesce(max(id), 0)").Scan(&head).Error
	return head, err
}

// After yields the rows whose position is greater than pos and whose time is
// at or after from, in insertion order, each with its position.
func (r *LedgerRepo) After(ctx context.Context, pos uint, from time.Time) iter.Seq2[repository.Positioned, error] {
	return func(yield func(repository.Positioned, error) bool) {
		for {
			var rows []EntryRow
			err := r.db.g.WithContext(ctx).Where("at >= ? AND id > ?", from.UTC(), pos).Order("id").Limit(r.db.batch()).Find(&rows).Error
			if err != nil {
				yield(repository.Positioned{}, err)
				return
			}
			for i := range rows {
				row := &rows[i]
				if !yield(repository.Positioned{Pos: row.ID, Entry: fromRow(*row)}, nil) {
					return
				}
				pos = row.ID
			}
			if len(rows) < r.db.batch() {
				return
			}
		}
	}
}

// PruneOlderThan deletes rows before cutoff. The chain restarts at the oldest
// kept row, which `pmr verify` accepts as a chain start.
func (r *LedgerRepo) PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res := r.db.g.WithContext(ctx).Where("at < ?", cutoff.UTC()).Delete(&EntryRow{})
	return res.RowsAffected, res.Error
}

// SeenPrompt reports whether the session already delegated the same brief.
func (r *LedgerRepo) SeenPrompt(ctx context.Context, sessionID, promptSHA string) (bool, error) {
	if sessionID == "" || promptSHA == "" {
		return false, nil
	}
	var n int64
	err := r.db.g.WithContext(ctx).Model(&EntryRow{}).
		Where("event = ? AND session_id = ? AND prompt_sha = ?", string(model.EventSubagent), sessionID, promptSHA).
		Count(&n).Error
	return n > 0, err
}

// Verify walks the chain and returns the ID of the first broken row (0 =
// intact). A row breaks the chain when its hash does not match its content in
// its hash form, when it does not link to the row before it, or when its form
// is older than a row before it (a downgrade to HashV1 would hide an edit to
// a column V1 does not cover).
//
// What the chain cannot detect: rows removed from the end (nothing links to
// them), rows removed from the start (indistinguishable from the retention
// prune, which restarts the chain at the oldest kept row), and a rewrite by
// someone who recomputes every hash after the edit (there is no secret key;
// the chain is tamper evidence against edits, not a signature).
func (r *LedgerRepo) Verify(ctx context.Context) (checked int, brokenID uint, err error) {
	var rows []EntryRow
	if err := r.db.g.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return 0, 0, err
	}
	for i := range rows {
		row := &rows[i]
		prev := row.PrevHash
		switch {
		case i > 0 && prev != rows[i-1].Hash,
			i > 0 && row.HashVersion < rows[i-1].HashVersion,
			RowHash(prev, *row) != row.Hash:
			return i, row.ID, nil
		}
	}
	return len(rows), 0, nil
}

// SessionRepo implements repository.SessionRepository.
type SessionRepo struct{ db *DB }

// NewSessionRepo builds the session repository.
func NewSessionRepo(db *DB) *SessionRepo { return &SessionRepo{db: db} }

// Find returns the session, if recorded.
func (r *SessionRepo) Find(ctx context.Context, id string) (model.Session, bool, error) {
	var row SessionRow
	err := r.db.g.WithContext(ctx).Take(&row, "id = ?", id).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return model.Session{}, false, nil
	case err != nil:
		return model.Session{}, false, err
	}
	return model.Session{ID: row.ID, Model: row.Model, Source: model.SessionSource(row.Source), UpdatedAt: row.UpdatedAt}, true, nil
}

// Save upserts the session.
func (r *SessionRepo) Save(ctx context.Context, s model.Session) error {
	row := SessionRow{ID: s.ID, Model: s.Model, Source: string(s.Source), UpdatedAt: s.UpdatedAt.UTC()}
	return r.db.g.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}

// PruneOlderThan deletes sessions not updated since cutoff.
func (r *SessionRepo) PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res := r.db.g.WithContext(ctx).Where("updated_at < ?", cutoff.UTC()).Delete(&SessionRow{})
	return res.RowsAffected, res.Error
}

// Shutdown lets the DI container close the database (do.ShutdownerWithError).
func (db *DB) Shutdown() error { return db.Close() }

func (db *DB) batch() int { return max(db.opt.Batch, 1) }
