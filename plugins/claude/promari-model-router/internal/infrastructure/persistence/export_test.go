package persistence

import "gorm.io/gorm"

// Test-only hooks into unexported helpers (compiled only with the tests).

// RowSource exposes the cursor interface collect reads.
type RowSource = rowSource

// Collect exposes collect.
func Collect(rows RowSource, limit int) ([]string, []map[string]any, error) {
	return collect(rows, limit)
}

// Gorm exposes the handle to tests that break or inspect the database.
func (db *DB) Gorm() *gorm.DB { return db.g }

// WrapGorm builds a DB around a handle (a handle without a connection pool).
func WrapGorm(g *gorm.DB) *DB { return &DB{g: g} }

// LegacyRowHash is the HashV1 form, to build ledgers an older release wrote.
func LegacyRowHash(prev string, r EntryRow) string { return rowHashV1(prev, r) }
