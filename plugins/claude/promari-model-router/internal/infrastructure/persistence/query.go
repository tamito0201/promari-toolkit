package persistence

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"promari-model-router/internal/domain/repository"
)

// ErrNotReadOnly rejects anything but a single SELECT / WITH statement.
var ErrNotReadOnly = errors.New("only a single read-only SELECT (or WITH ... SELECT) statement is allowed")

var (
	selectRE    = regexp.MustCompile(`(?is)^\s*(select|with)\b`)
	forbiddenRE = regexp.MustCompile(`(?i)\b(insert|update|delete|drop|alter|create|replace|attach|detach|pragma|vacuum|reindex)\b`)
)

// ValidateQuery is the first of two guards for `pmr query`: it accepts only a
// single SELECT/WITH statement. The second guard is the read-only connection
// (mode=ro, query_only), so a statement that slips past this check still
// cannot write.
func ValidateQuery(sql string) error {
	trimmed := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(sql), ";"))
	if !selectRE.MatchString(trimmed) || strings.Contains(trimmed, ";") || forbiddenRE.MatchString(trimmed) {
		return ErrNotReadOnly
	}
	return nil
}

// Query runs a validated read-only statement and returns rows as maps.
func (db *DB) Query(ctx context.Context, sql string, limit int) ([]string, []map[string]any, error) {
	if err := ValidateQuery(sql); err != nil {
		return nil, nil, err
	}
	rows, err := db.g.WithContext(ctx).Raw(sql).Rows()
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	return collect(rows, limit)
}

// rowSource is the part of *sql.Rows that collect reads (an interface so that
// tests can make Columns and Scan fail, which a real SQLite cursor never does
// after a successful query).
type rowSource interface {
	Columns() ([]string, error)
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// collect reads up to limit rows as maps; []byte values become strings.
func collect(rows rowSource, limit int) ([]string, []map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	var out []map[string]any
	for rows.Next() && len(out) < limit {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		row := map[string]any{}
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				row[c] = string(b)
			} else {
				row[c] = vals[i]
			}
		}
		out = append(out, row)
	}
	return cols, out, rows.Err()
}

// Schema returns the CREATE statements of the ledger tables (schema context
// for anyone, human or model, who writes `pmr query` SQL).
func (db *DB) Schema(ctx context.Context) ([]string, error) {
	var stmts []string
	err := db.g.WithContext(ctx).Raw("SELECT sql FROM sqlite_master WHERE type='table' AND name IN ('ledger','sessions') ORDER BY name").
		Scan(&stmts).Error
	return stmts, err
}

// ReadOnlyLedger implements repository.LedgerQuery. Each call opens its own
// read-only connection (mode=ro, query_only) and closes it, so answering a
// query never creates, migrates or locks the ledger for writing.
type ReadOnlyLedger struct {
	Path    string
	Options Options
}

var _ repository.LedgerQuery = ReadOnlyLedger{}

func (l ReadOnlyLedger) with(fn func(db *DB) error) error {
	db, err := OpenReadOnly(l.Path, l.Options)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return fn(db)
}

// Query runs a validated read-only statement.
func (l ReadOnlyLedger) Query(ctx context.Context, sql string, limit int) (repository.QueryResult, error) {
	var res repository.QueryResult
	err := l.with(func(db *DB) error {
		var err error
		res.Columns, res.Rows, err = db.Query(ctx, sql, limit)
		return err
	})
	return res, err
}

// Schema returns the CREATE statements of the ledger tables.
func (l ReadOnlyLedger) Schema(ctx context.Context) ([]string, error) {
	var stmts []string
	err := l.with(func(db *DB) error {
		var err error
		stmts, err = db.Schema(ctx)
		return err
	})
	return stmts, err
}
