package usecase_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/application/usecase"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/service"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/clock"
)

func TestReportUseCase(t *testing.T) {
	tests := []struct {
		name string
		// run drives the hooks against the fixture before the report.
		run      func(t *testing.T, f *fixture)
		days     float64 // 0 = the default window (one hour here)
		sinceErr error
		want     func(r usecase.Report) []int
		wantVals []int
		wantErr  error
	}{
		{
			name: "hook flow: decision joined to its result, every entry on the injected clock",
			run: func(t *testing.T, f *fixture) {
				t.Helper()
				ctx, ev := t.Context(), usecase.Event{SessionID: "s1", ToolUseID: "t1"}
				f.sessionStart().Execute(ctx, usecase.SessionStartInput{Event: ev, Model: "claude-opus-5-5", Source: "startup"})
				f.subagentStart().Execute(ctx, ev, model.AgentCall{Prompt: lookupPrompt})
				f.subagentStart().Execute(ctx, ev, model.AgentCall{Prompt: lookupPrompt})
				f.subagentFinish().Execute(ctx, ev, model.SubagentOutcome{ToolUseID: "t1", Requested: "haiku", Resolved: "claude-haiku-4-5", Status: "completed", TotalTokens: 500}, "x")
				for i := range f.ledger.entries {
					if !f.ledger.entries[i].At.Equal(testNow) {
						t.Errorf("entry not stamped by the injected clock: %v", f.ledger.entries[i].At)
					}
				}
			},
			want: func(r usecase.Report) []int {
				return []int{r.Entries, r.Subagents.Calls, r.Results.Joined, r.Results.TokensByTier["haiku"], r.Results.TokensBelowSession}
			},
			wantVals: []int{4, 2, 1, 500, 500},
		},
		{
			name: "zero days is the default window; a given window replaces it",
			run: func(t *testing.T, f *fixture) {
				t.Helper()
				f.ledger.entries = []model.Entry{{At: testNow.Add(-2 * time.Hour), Event: model.EventPrompt}, {At: testNow, Event: model.EventPrompt}}
			},
			want:     func(r usecase.Report) []int { return []int{r.Entries} },
			wantVals: []int{1},
		},
		{
			name: "one day reaches the older entry",
			run: func(t *testing.T, f *fixture) {
				t.Helper()
				f.ledger.entries = []model.Entry{{At: testNow.Add(-2 * time.Hour), Event: model.EventPrompt}, {At: testNow, Event: model.EventPrompt}}
			},
			days: 1, want: func(r usecase.Report) []int { return []int{r.Entries} }, wantVals: []int{2},
		},
		// Every entry point used to decide this on its own (the MCP tool took a
		// negative window as the default, the CLI as an empty future window).
		{name: "a negative window is refused", run: func(*testing.T, *fixture) {}, days: -1, wantErr: usecase.ErrInvalidDays},
		{name: "NaN is refused", run: func(*testing.T, *fixture) {}, days: math.NaN(), wantErr: usecase.ErrInvalidDays},
		{name: "an infinite window is refused", run: func(*testing.T, *fixture) {}, days: math.Inf(1), wantErr: usecase.ErrInvalidDays},
		{
			name: "a ledger read error is returned",
			run: func(t *testing.T, _ *fixture) {
				t.Helper()
			},
			sinceErr: errLedger,
			wantErr:  errLedger,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			tt.run(t, f)
			f.ledger.sinceErr = tt.sinceErr
			rep, err := usecase.ReportUseCase{Ledger: f.ledger, Clock: clock.Fixed{At: testNow}, DefaultDays: 1.0 / 24}.Execute(t.Context(), tt.days)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.want == nil {
				return
			}
			if diff := cmp.Diff(tt.wantVals, tt.want(rep)); diff != "" {
				t.Errorf("report (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildReport(t *testing.T) {
	t0 := testNow.Add(-time.Hour)
	prices := service.PriceTable{AsOf: "2026-09-01", Tiers: map[model.Tier]service.Price{
		model.TierHaiku: {Input: 1, Output: 5, CacheRead: 0.1},
	}}
	tests := []struct {
		name    string
		entries []model.Entry
		want    usecase.Report
	}{
		{
			name:    "empty window: no dates, zero counts",
			entries: nil,
			want: usecase.Report{
				PricesAsOf:       "2026-09-01",
				Prompts:          usecase.PromptStats{ByClass: map[string]int{}, ByLang: map[string]usecase.Ratio{}},
				Subagents:        usecase.SubagentStats{ByAction: map[string]int{}, ByReason: map[string]int{}, Injected: map[string]int{}, Shadow: map[string]int{}},
				Results:          usecase.ResultStats{CallsByTier: map[string]int{}, TokensByTier: map[string]int{}},
				EstimatedCostUSD: map[string]float64{},
			},
		},
		{
			name: "every kind of entry",
			entries: []model.Entry{
				{At: t0, Event: model.EventPrompt, Class: model.ClassLookup, Lang: model.Language("ja"), Advised: true, Danger: true},
				{At: t0, Event: model.EventPrompt, Class: model.ClassNone, Lang: model.Language("ja")},
				{At: t0, Event: model.EventPrompt, Class: model.ClassStandard, Lang: model.Language("en")},
				{At: t0, Event: model.EventSubagent, ToolUseID: "t1", SessionModel: "claude-opus-5-5", Action: model.ActionInject, Reason: "rule:lookup", Target: model.TierHaiku},
				{At: t0, Event: model.EventSubagent, ToolUseID: "t2", SessionModel: "claude-haiku-4-5", Action: model.ActionShadow, Reason: "shadow", Target: model.TierHaiku},
				{At: t0, Event: model.EventSubagent, Action: model.ActionNone, Reason: "abstain"},
				{At: t0, Event: model.EventSubagentResult, ToolUseID: "t1", Resolved: "claude-haiku-4-5", TotalTokens: 1000, InputTokens: 1_000_000, OutputTokens: 200_000, CacheReadTokens: 1_000_000, Mismatch: model.MismatchYes},
				{At: t0, Event: model.EventSubagentResult, ToolUseID: "t2", Resolved: "claude-haiku-4-5", TotalTokens: 10, Mismatch: model.MismatchNo},
				{At: t0, Event: model.EventSubagentResult, ToolUseID: "orphan", Status: "async_launched"},
				{At: testNow, Event: model.EventError},
			},
			want: usecase.Report{
				From: t0, To: testNow, Entries: 10, Errors: 1, PricesAsOf: "2026-09-01",
				Prompts: usecase.PromptStats{
					Total: 3, Classified: 2, Advised: 1, Danger: 1,
					ByClass: map[string]int{"lookup": 1, "(abstain)": 1, "standard": 1},
					ByLang:  map[string]usecase.Ratio{"ja": {Part: 1, Whole: 2}, "en": {Part: 1, Whole: 1}},
				},
				Subagents: usecase.SubagentStats{
					Calls:    3,
					ByAction: map[string]int{string(model.ActionInject): 1, string(model.ActionShadow): 1, string(model.ActionNone): 1},
					ByReason: map[string]int{"rule:lookup": 1, "shadow": 1, "abstain": 1},
					Injected: map[string]int{"haiku": 1},
					Shadow:   map[string]int{"haiku": 1},
				},
				Results: usecase.ResultStats{
					Count: 3, Joined: 2,
					CallsByTier:        map[string]int{"haiku": 2, "unknown": 1},
					TokensByTier:       map[string]int{"haiku": 1010, "unknown": 0},
					TokensBelowSession: 1000, Mismatch: 1, AsyncNoUsage: 1,
				},
				EstimatedCostUSD: map[string]float64{"haiku": 2.1, "unknown": 0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, usecase.BuildReport(tt.entries, prices)); diff != "" {
				t.Errorf("report (-want +got):\n%s", diff)
			}
		})
	}
}
