package service_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/service"
)

func TestSubagentShapeUsage(t *testing.T) {
	tests := []struct {
		name  string
		shape service.SubagentShape
		want  service.Usage
	}{
		{name: "no work", shape: service.SubagentShape{}, want: service.Usage{}},
		{
			name:  "report only",
			shape: service.SubagentShape{Prompt: 1000, Report: 50},
			want:  service.Usage{CacheWrite: 1000, Output: 50},
		},
		{
			name:  "one tool call re-reads nothing it produced",
			shape: service.SubagentShape{ToolCalls: 1, Prompt: 1000, ToolResult: 100, OutputPerCall: 10, Report: 50},
			want:  service.Usage{CacheRead: 1000, CacheWrite: 1000 + 110, Output: 10 + 50},
		},
		{
			name:  "k calls re-read the growing history",
			shape: service.SubagentShape{ToolCalls: 3, Prompt: 1000, ToolResult: 100, OutputPerCall: 10, Report: 50},
			want:  service.Usage{CacheRead: 3*1000 + 110*3, CacheWrite: 1000 + 3*110, Output: 30 + 50},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, tt.shape.Usage()); diff != "" {
				t.Errorf("Usage() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

var testPrices = service.PriceTable{AsOf: "2026-09-30", Tiers: map[model.Tier]service.Price{
	model.TierHaiku: {Model: "haiku", Input: 1, Output: 5, CacheRead: 0.1, CacheWrite5m: 1.25},
	model.TierOpus:  {Model: "opus", Input: 5, Output: 20, CacheRead: 0.2, CacheWrite5m: 5},
}}

func TestPriceTableCost(t *testing.T) {
	tests := []struct {
		name  string
		tier  model.Tier
		usage service.Usage
		want  float64
	}{
		{name: "a tier without a price costs nothing", tier: model.TierSonnet, usage: service.Usage{Input: service.PerMillion}, want: 0},
		{name: "no usage", tier: model.TierHaiku, usage: service.Usage{}, want: 0},
		{name: "one million input tokens", tier: model.TierHaiku, usage: service.Usage{Input: service.PerMillion}, want: 1},
		{
			name: "every token kind is priced",
			tier: model.TierOpus,
			usage: service.Usage{
				Input: service.PerMillion, Output: service.PerMillion,
				CacheRead: service.PerMillion, CacheWrite: service.PerMillion,
			},
			want: 5 + 20 + 0.2 + 5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, testPrices.Cost(tt.tier, tt.usage), approx); diff != "" {
				t.Errorf("Cost() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestPriceTableCompare(t *testing.T) {
	shape := service.SubagentShape{ToolCalls: 3, Prompt: 1000, ToolResult: 100, OutputPerCall: 10, Report: 50}
	u := shape.Usage()
	tests := []struct {
		name   string
		prices service.PriceTable
		want   map[model.Tier]float64
	}{
		{name: "an empty table", prices: service.PriceTable{}, want: map[model.Tier]float64{}},
		{
			name:   "only priced tiers, haiku cheaper than opus",
			prices: testPrices,
			want: map[model.Tier]float64{
				model.TierHaiku: (float64(u.Output)*5 + float64(u.CacheRead)*0.1 + float64(u.CacheWrite)*1.25) / service.PerMillion,
				model.TierOpus:  (float64(u.Output)*20 + float64(u.CacheRead)*0.2 + float64(u.CacheWrite)*5) / service.PerMillion,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, tt.prices.Compare(shape), approx); diff != "" {
				t.Errorf("Compare() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
