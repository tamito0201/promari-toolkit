package settings

import (
	"slices"
	"sync"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/repository"
	"promari-model-router/internal/domain/service"
)

// Provider implements repository.ConfigProvider with a per-cwd cache. Every
// accessor returns a copy: the cache is shared, and a caller that adjusts its
// value (eval forces the mode) must not change what the next caller reads.
type Provider struct {
	mu      sync.Mutex
	byCwd   map[string]loaded
	process *loaded
	tiers   model.TierTable
	prices  service.PriceTable
}

type loaded struct {
	settings model.Settings
	sources  []string
	problems []string
	lexicon  *service.Lexicon
}

var _ repository.ConfigProvider = (*Provider)(nil)

// NewProvider builds a provider.
func NewProvider() *Provider {
	return &Provider{byCwd: map[string]loaded{}, tiers: Tiers(), prices: Prices()}
}

func (p *Provider) get(cwd string) loaded {
	p.mu.Lock()
	defer p.mu.Unlock()
	if l, ok := p.byCwd[cwd]; ok {
		return l
	}
	st, sources, problems := Load(cwd)
	l := loaded{settings: st, sources: sources, problems: problems, lexicon: Lexicon(st)}
	p.byCwd[cwd] = l
	return l
}

// Process returns the settings the process is wired with: the defaults and
// the user file, never a project file (see LoadUser).
func (p *Provider) Process() model.Settings {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.process == nil {
		st, sources, problems := LoadUser()
		p.process = &loaded{settings: st, sources: sources, problems: problems}
	}
	return p.process.settings.Clone()
}

// Settings returns the effective settings.
func (p *Provider) Settings(cwd string) model.Settings { return p.get(cwd).settings.Clone() }

// Lexicon returns the compiled lexicon (never mutated after Compile).
func (p *Provider) Lexicon(cwd string) *service.Lexicon { return p.get(cwd).lexicon }

// Tiers returns the tier table.
func (p *Provider) Tiers() model.TierTable { return p.tiers.Clone() }

// Prices returns the price table.
func (p *Provider) Prices() service.PriceTable { return p.prices.Clone() }

// Sources are the override files that were applied.
func (p *Provider) Sources(cwd string) []string { return slices.Clone(p.get(cwd).sources) }

// Problems are the override files that were rejected.
func (p *Provider) Problems(cwd string) []string { return slices.Clone(p.get(cwd).problems) }
