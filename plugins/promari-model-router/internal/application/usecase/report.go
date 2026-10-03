package usecase

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"math"
	"slices"
	"time"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/repository"
	"promari-model-router/internal/domain/service"
	"promari-model-router/pkg/fp"
)

// Report summarises the ledger. Every rate carries its denominator so that a
// zero-event window reads "n/a", never 0% (a rate with no denominator is not
// evidence that the router works).
type Report struct {
	From             time.Time          `json:"from"`
	To               time.Time          `json:"to"`
	Entries          int                `json:"entries"`
	Prompts          PromptStats        `json:"prompts"`
	Subagents        SubagentStats      `json:"subagents"`
	Results          ResultStats        `json:"results"`
	Errors           int                `json:"errors"`
	EstimatedCostUSD map[string]float64 `json:"estimated_cost_usd,omitempty"`
	PricesAsOf       string             `json:"prices_as_of,omitempty"`
}

// PromptStats covers user prompts.
type PromptStats struct {
	Total      int              `json:"total"`
	Classified int              `json:"classified"`
	Advised    int              `json:"advised"`
	Danger     int              `json:"danger"`
	ByClass    map[string]int   `json:"by_class"`
	ByLang     map[string]Ratio `json:"classified_by_lang"`
}

// SubagentStats covers routing decisions.
type SubagentStats struct {
	Calls    int            `json:"calls"`
	ByAction map[string]int `json:"by_action"`
	ByReason map[string]int `json:"by_reason"`
	Injected map[string]int `json:"injected_targets"`
	Shadow   map[string]int `json:"shadow_targets"`
}

// ResultStats covers what actually ran.
type ResultStats struct {
	Count              int            `json:"count"`
	Joined             int            `json:"joined_to_decision"`
	CallsByTier        map[string]int `json:"calls_by_resolved_tier"`
	TokensByTier       map[string]int `json:"tokens_by_resolved_tier"`
	TokensBelowSession int            `json:"tokens_below_session_tier"`
	Mismatch           int            `json:"requested_vs_resolved_mismatch"`
	AsyncNoUsage       int            `json:"background_without_usage"`
}

// Ratio is a count out of a denominator.
type Ratio struct {
	Part  int `json:"part"`
	Whole int `json:"whole"`
}

// ReportUseCase builds reports.
type ReportUseCase struct {
	Ledger repository.LedgerReader
	Clock  repository.Clock
	Prices service.PriceTable
	// DefaultDays is the window when the caller gives none ([report].default_days).
	DefaultDays float64
}

// ErrInvalidDays rejects a window that is negative or not a number.
var ErrInvalidDays = errors.New("days must be a positive number")

// window turns a number of days into a duration: 0 is the default, anything
// negative or not finite is refused. Every entry point (CLI, HTTP, MCP,
// training) goes through here, so they agree on what a window means.
func window(days, fallback float64) (time.Duration, error) {
	if days == 0 {
		days = fallback
	}
	if math.IsNaN(days) || days <= 0 || math.IsInf(days, 1) {
		return 0, fmt.Errorf("%w: %v", ErrInvalidDays, days)
	}
	return time.Duration(days * float64(24*time.Hour)), nil
}

// Execute summarises the last `days` of the ledger (0 = DefaultDays).
func (u ReportUseCase) Execute(ctx context.Context, days float64) (Report, error) {
	w, err := window(days, u.DefaultDays)
	if err != nil {
		return Report{}, err
	}
	entries, err := collect(u.Ledger.Since(ctx, u.Clock.Now().Add(-w)))
	if err != nil {
		return Report{}, err
	}
	return BuildReport(entries, u.Prices), nil
}

func collect(seq iter.Seq2[model.Entry, error]) ([]model.Entry, error) {
	var out []model.Entry
	for e, err := range seq {
		if err != nil {
			return out, err
		}
		out = append(out, e)
	}
	return out, nil
}

func ofEvent(k model.EventKind) func(model.Entry) bool {
	return func(e model.Entry) bool { return e.Event == k }
}

// BuildReport is the pure aggregation behind Execute.
func BuildReport(entries []model.Entry, prices service.PriceTable) Report {
	all := slices.Values(entries)
	prompts := slices.Collect(fp.Filter(all, ofEvent(model.EventPrompt)))
	calls := slices.Collect(fp.Filter(all, ofEvent(model.EventSubagent)))
	results := slices.Collect(fp.Filter(all, ofEvent(model.EventSubagentResult)))

	rep := Report{Entries: len(entries), Errors: fp.Count(all, ofEvent(model.EventError)), PricesAsOf: prices.AsOf}
	if len(entries) > 0 {
		rep.From, rep.To = entries[0].At, entries[len(entries)-1].At
	}

	classified := func(e model.Entry) bool { return e.Class != model.ClassNone }
	rep.Prompts = PromptStats{
		Total:      len(prompts),
		Classified: fp.Count(slices.Values(prompts), classified),
		Advised:    fp.Count(slices.Values(prompts), func(e model.Entry) bool { return e.Advised }),
		Danger:     fp.Count(slices.Values(prompts), func(e model.Entry) bool { return e.Danger }),
		ByClass: fp.CountBy(slices.Values(prompts), func(e model.Entry) string {
			return string(fp.OptionFrom(e.Class, e.Class != model.ClassNone).OrElse("(abstain)"))
		}),
		ByLang: map[string]Ratio{},
	}
	for lang, group := range fp.GroupBy(slices.Values(prompts), func(e model.Entry) string { return string(e.Lang) }) {
		rep.Prompts.ByLang[lang] = Ratio{Part: fp.Count(slices.Values(group), classified), Whole: len(group)}
	}

	byAction := func(a model.Action) func(model.Entry) bool { return func(e model.Entry) bool { return e.Action == a } }
	rep.Subagents = SubagentStats{
		Calls:    len(calls),
		ByAction: fp.CountBy(slices.Values(calls), func(e model.Entry) string { return string(e.Action) }),
		ByReason: fp.CountBy(slices.Values(calls), func(e model.Entry) string { return e.Reason }),
		Injected: fp.CountBy(fp.Filter(slices.Values(calls), byAction(model.ActionInject)), func(e model.Entry) string { return string(e.Target) }),
		Shadow:   fp.CountBy(fp.Filter(slices.Values(calls), byAction(model.ActionShadow)), func(e model.Entry) string { return string(e.Target) }),
	}

	rs := ResultStats{Count: len(results), CallsByTier: map[string]int{}, TokensByTier: map[string]int{}}
	costs := map[string]float64{}
	runs := service.JoinSubagentRuns(entries)
	for i := range runs {
		run := &runs[i]
		r := &run.Result
		tier := model.TierOf(r.Resolved)
		name := string(fp.OptionFrom(tier, tier.Known()).OrElse("unknown"))
		rs.CallsByTier[name]++
		rs.TokensByTier[name] += r.TotalTokens
		costs[name] += prices.Cost(tier, service.Usage{Input: r.InputTokens, Output: r.OutputTokens, CacheRead: r.CacheReadTokens})
		if run.Routed {
			rs.Joined++
			if model.TierOf(run.Decision.SessionModel).Above(tier) {
				rs.TokensBelowSession += r.TotalTokens
			}
		}
		if r.Mismatch.Yes() {
			rs.Mismatch++
		}
		if r.Status.Background() {
			rs.AsyncNoUsage++
		}
	}
	rep.Results = rs
	rep.EstimatedCostUSD = costs
	return rep
}
