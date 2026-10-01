package usecase

import (
	"context"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/repository"
)

// QueryUseCase answers ad-hoc read-only questions about the ledger
// (`pmr query`). The port refuses anything but a single SELECT and runs it on
// a read-only connection, so this use case cannot write.
type QueryUseCase struct {
	Ledger repository.LedgerQuery
}

// Query runs one read-only statement and returns at most limit rows.
func (u QueryUseCase) Query(ctx context.Context, sql string, limit int) (repository.QueryResult, error) {
	return u.Ledger.Query(ctx, sql, limit)
}

// Schema returns the ledger's table definitions (context for writing a query).
func (u QueryUseCase) Schema(ctx context.Context) ([]string, error) {
	return u.Ledger.Schema(ctx)
}
