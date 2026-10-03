package usecase

import (
	"context"
	"time"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/repository"
	"promari-model-router/internal/domain/service"
)

// VerifyUseCase checks the ledger's hash chain (`pmr verify`).
type VerifyUseCase struct {
	Ledger repository.LedgerVerifier
}

// Verification is the outcome of walking the chain.
type Verification struct {
	Checked int  `json:"checked"`
	Broken  uint `json:"broken_at,omitempty"` // 0 when the chain is intact
}

// Execute walks the chain.
func (u VerifyUseCase) Execute(ctx context.Context) (Verification, error) {
	checked, broken, err := u.Ledger.Verify(ctx)
	return Verification{Checked: checked, Broken: broken}, err
}

// CostUseCase prices one subagent run on every tier (`pmr cost`).
type CostUseCase struct {
	Prices repository.PriceProvider
}

// CostEstimate is the usage of a subagent shape and its list price per tier.
type CostEstimate struct {
	PricesAsOf string                 `json:"prices_as_of"`
	Usage      service.Usage          `json:"usage"`
	USDByTier  map[model.Tier]float64 `json:"usd_by_tier"`
}

// SubagentShape is the subagent run CostUseCase prices.
type SubagentShape = service.SubagentShape

// Execute prices the shape.
func (u CostUseCase) Execute(shape SubagentShape) CostEstimate {
	prices := u.Prices.Prices()
	return CostEstimate{PricesAsOf: prices.AsOf, Usage: shape.Usage(), USDByTier: prices.Compare(shape)}
}

// FeedUseCase follows the ledger for the live feed of `pmr serve` (SSE). It
// follows positions, not timestamps: resuming from "the time of the last
// entry" repeated rows (the stored text drops trailing zeros and compares
// above the bound) and would skip rows written with an earlier clock.
type FeedUseCase struct {
	Ledger repository.LedgerFeed
	Clock  repository.Clock
}

// FeedCursor is where a feed stands. The zero cursor has not started.
type FeedCursor struct {
	started bool
	from    time.Time
	pos     uint
}

// Start returns a cursor whose first Next yields the entries of the last
// `back` (0 = none) and whose later calls yield only new entries.
func (u FeedUseCase) Start(ctx context.Context, back time.Duration) (FeedCursor, error) {
	head, err := u.Ledger.Head(ctx)
	if err != nil {
		return FeedCursor{}, err
	}
	if back <= 0 {
		return FeedCursor{started: true, pos: head}, nil
	}
	return FeedCursor{from: u.Clock.Now().Add(-back), pos: head}, nil
}

// Next calls emit for every entry after the cursor, in insertion order, and
// returns the advanced cursor. The first call of a cursor with a backlog
// yields the backlog (entries at or after its start time, up to and past the
// head it saw); an emit error stops the feed and is returned.
func (u FeedUseCase) Next(ctx context.Context, c FeedCursor, emit func(model.Entry) error) (FeedCursor, error) {
	pos, from := c.pos, time.Time{}
	if !c.started {
		pos, from = 0, c.from
	}
	next := FeedCursor{started: true, pos: c.pos}
	for p, err := range u.Ledger.After(ctx, pos, from) {
		if err != nil {
			return next, err
		}
		if err := emit(p.Entry); err != nil {
			return next, err
		}
		next.pos = max(next.pos, p.Pos)
	}
	return next, nil
}

// PolicyUseCase is the routing policy as Claude sees it (the MCP route_policy tool).
type PolicyUseCase struct {
	Config repository.SettingsProvider
}

// Policy is the policy text and the route tags a brief may start with.
type Policy struct {
	Policy string   `json:"policy"`
	Tags   []string `json:"route_tags"`
}

// Execute renders the policy for a working directory.
func (u PolicyUseCase) Execute(cwd string) Policy {
	m := u.Config.Settings(cwd).Messages
	return Policy{Policy: service.SessionContext(m), Tags: m.RouteTags}
}
