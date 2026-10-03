package usecase

import (
	"encoding/json"
	"math"
	"strings"

	"promari-model-router/internal/domain/service"
)

// TraceView is the serialisable form of a routing trace (ledger detail and
// `pmr explain`). NaN values are dropped because JSON cannot carry them.
type TraceView struct {
	Path        []string           `json:"path"`
	Source      string             `json:"source,omitempty"`
	RuleClass   string             `json:"rule_class,omitempty"`
	ModelClass  string             `json:"model_class,omitempty"`
	Probs       map[string]float64 `json:"probs,omitempty"`
	Set         []string           `json:"set,omitempty"`
	PSafe       float64            `json:"p_safe,omitempty"`
	Tau         float64            `json:"tau,omitempty"`
	Unseen      float64            `json:"unseen,omitempty"`
	NeighborSim float64            `json:"neighbor_sim,omitempty"`
	Posterior   *float64           `json:"posterior,omitempty"`
	Escalated   bool               `json:"escalated,omitempty"`
	Reasons     []string           `json:"reasons,omitempty"`
}

// ViewOf converts a trace, rounding probabilities to `decimals` places.
func ViewOf(tr service.RouteTrace, decimals int) TraceView {
	round := func(f float64) float64 {
		scale := math.Pow(10, float64(decimals))
		return math.Round(f*scale) / scale
	}
	v := TraceView{
		Path: tr.Path, Source: tr.Source, RuleClass: string(tr.Rule.Class), ModelClass: string(tr.ModelClass),
		PSafe: round(tr.PSafe), Tau: tr.Tau, Unseen: round(tr.Unseen), NeighborSim: round(tr.NeighborSim),
		Escalated: tr.Escalated, Reasons: tr.Rule.Reasons,
	}
	if len(tr.Probs) > 0 {
		v.Probs = map[string]float64{}
		for c, p := range tr.Probs {
			v.Probs[string(c)] = round(p)
		}
	}
	for _, c := range tr.Set {
		v.Set = append(v.Set, string(c))
	}
	if tr.Posterior != 0 && !math.IsNaN(tr.Posterior) {
		v.Posterior = new(round(tr.Posterior))
	}
	return v
}

// ledgerTrace is the ledger detail of a routing decision: the trace, and what
// could not be read while deciding (the decision went on without it).
type ledgerTrace struct {
	TraceView
	Degraded []string `json:"degraded,omitempty"`
}

func traceDetail(tr service.RouteTrace, decimals int, degraded []string) string {
	raw, err := json.Marshal(ledgerTrace{TraceView: ViewOf(tr, decimals), Degraded: degraded})
	if err != nil {
		return strings.Join(tr.Path, ">")
	}
	return string(raw)
}
