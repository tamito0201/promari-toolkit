package usecase_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/repository"
	"promari-model-router/internal/infrastructure/agents"
)

func TestLint(t *testing.T) {
	shipped := newFixture(t).config
	tests := []struct {
		name   string
		tiers  repository.TierProvider
		agents repository.AgentSource
		want   usecase.LintResult
	}{
		{
			name: "shipped agents match the shipped table", tiers: shipped, agents: agents.New(),
			want: usecase.LintResult{Agents: len(shipped.Tiers().Agents)},
		},
		{
			name: "missing file, no frontmatter, drifted model and effort",
			tiers: fakeTiers{Agents: map[string]model.AgentSpec{
				"ok":      {Model: model.TierHaiku, Effort: "low"},
				"absent":  {Model: model.TierHaiku},
				"bare":    {Model: model.TierHaiku},
				"drifted": {Model: model.TierHaiku, Effort: "medium"},
			}},
			agents: mapAgents{
				"ok":      {Model: model.TierHaiku, Effort: "low"},
				"bare":    nil,
				"drifted": {Model: model.TierSonnet, Effort: "high"},
			},
			want: usecase.LintResult{Agents: 4, Problems: []string{
				"agents/absent.md is missing",
				"agents/bare.md has no frontmatter",
				"agents/drifted.md model=sonnet but tiers.toml says haiku",
				`agents/drifted.md effort="high" but tiers.toml says "medium"`,
			}},
		},
		{
			name:   "an unreadable definition",
			tiers:  fakeTiers{Agents: map[string]model.AgentSpec{"locked": {Model: model.TierHaiku}}},
			agents: mapAgents{"locked": {Class: "unreadable"}},
			want:   usecase.LintResult{Agents: 1, Problems: []string{"agents/locked.md cannot be read: permission denied"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := usecase.LintUseCase{Tiers: tt.tiers, Agents: tt.agents}.Execute()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("lint (-want +got):\n%s", diff)
			}
		})
	}
}

// fakeTiers is a fixed tier table.
type fakeTiers model.TierTable

func (f fakeTiers) Tiers() model.TierTable { return model.TierTable(f) }
