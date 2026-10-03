package usecase

import (
	"errors"
	"fmt"
	"io/fs"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/repository"
	"promari-model-router/pkg/fp"
)

// LintUseCase checks the shipped agent frontmatter against data/tiers.toml.
type LintUseCase struct {
	Tiers  repository.TierProvider
	Agents repository.AgentSource
}

// LintResult lists the disagreements among the agents checked.
type LintResult struct {
	Agents   int
	Problems []string
}

// Execute checks every agent in the tier table.
func (u LintUseCase) Execute() LintResult {
	table := u.Tiers.Tiers()
	return LintResult{Agents: len(table.Agents), Problems: lintAgents(u.Agents, table)}
}

// lintAgents checks each agent named in the table against its definition.
func lintAgents(agents repository.AgentSource, table model.TierTable) []string {
	var problems []string
	for _, name := range fp.SortedKeys(table.Agents) {
		want := table.Agents[name]
		got, err := agents.AgentSpec(name)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			problems = append(problems, fmt.Sprintf("agents/%s.md is missing", name))
			continue
		case errors.Is(err, repository.ErrNoFrontmatter):
			problems = append(problems, fmt.Sprintf("agents/%s.md has no frontmatter", name))
			continue
		case err != nil:
			problems = append(problems, fmt.Sprintf("agents/%s.md cannot be read: %v", name, err))
			continue
		}
		if got.Model != want.Model {
			problems = append(problems, fmt.Sprintf("agents/%s.md model=%s but tiers.toml says %s", name, got.Model, want.Model))
		}
		if got.Effort != want.Effort {
			problems = append(problems, fmt.Sprintf("agents/%s.md effort=%q but tiers.toml says %q", name, got.Effort, want.Effort))
		}
	}
	return problems
}
