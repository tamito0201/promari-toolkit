package service

import (
	"slices"

	"promari-model-router/internal/domain/model"
)

// Price is USD per million tokens for one tier.
type Price struct {
	Model        string   `json:"model" toml:"model"`
	Input        float64  `json:"input" toml:"input"`
	Output       float64  `json:"output" toml:"output"`
	CacheRead    float64  `json:"cache_read" toml:"cache_read"`
	CacheWrite5m float64  `json:"cache_write_5m" toml:"cache_write_5m"`
	Derived      []string `json:"derived,omitempty" toml:"derived"`
}

// PriceTable is data/pricing.toml.
type PriceTable struct {
	AsOf  string               `json:"as_of" toml:"as_of"`
	Tiers map[model.Tier]Price `json:"tiers" toml:"tiers"`
}

// Usage is the token usage of one run.
type Usage struct {
	Input, Output, CacheRead, CacheWrite int
}

// PerMillion is the unit of the price table (USD per million tokens).
const PerMillion = 1_000_000

// Cost is the list-price cost of a usage on a tier (USD).
func (t PriceTable) Cost(tier model.Tier, u Usage) float64 {
	p, ok := t.Tiers[tier]
	if !ok {
		return 0
	}
	return (float64(u.Input)*p.Input + float64(u.Output)*p.Output +
		float64(u.CacheRead)*p.CacheRead + float64(u.CacheWrite)*p.CacheWrite5m) / PerMillion
}

// SubagentShape describes a subagent run for the Harness Tokenomics cost model
// (arXiv:2609.28919, appendix B): k tool calls, a system+brief prompt of P
// tokens, t tokens of tool result and o tokens of output per call, and a final
// report of r tokens.
type SubagentShape struct {
	ToolCalls, Prompt, ToolResult, OutputPerCall, Report int
}

// Usage returns the token usage implied by the shape:
//
//	cache reads  = kP + (t+o)·k(k−1)/2
//	cache writes = P + k(t+o)
//	output       = ko + r
func (s SubagentShape) Usage() Usage {
	k, p, t, o, r := s.ToolCalls, s.Prompt, s.ToolResult, s.OutputPerCall, s.Report
	return Usage{
		CacheRead:  k*p + (t+o)*k*(k-1)/2,
		CacheWrite: p + k*(t+o),
		Output:     k*o + r,
	}
}

// Compare prices the same shape on every tier, cheapest tier first.
func (t PriceTable) Compare(s SubagentShape) map[model.Tier]float64 {
	out := map[model.Tier]float64{}
	for _, tier := range model.TierOrder {
		if _, ok := t.Tiers[tier]; ok {
			out[tier] = t.Cost(tier, s.Usage())
		}
	}
	return out
}

// Clone returns a deep copy (the provider hands the same table to every caller).
func (t PriceTable) Clone() PriceTable {
	if t.Tiers != nil {
		tiers := make(map[model.Tier]Price, len(t.Tiers))
		for k, p := range t.Tiers {
			p.Derived = slices.Clone(p.Derived)
			tiers[k] = p
		}
		t.Tiers = tiers
	}
	return t
}
