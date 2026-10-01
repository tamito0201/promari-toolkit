package learn

import (
	"slices"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/service"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/pkg/fp"
)

// Case is one labelled evaluation example.
type Case struct {
	Text   string
	Expect model.Class // ClassNone = abstain
	Danger bool
	Lang   string
}

// CaseResult is the router's behaviour on one case.
type CaseResult struct {
	Case       Case
	Decision   model.Decision
	Trace      service.RouteTrace
	Got        model.Class
	Sufficient bool // the chosen tier is at least what the expected class needs
	Harmful    bool // routed below what the expected class needs
}

// Report aggregates an evaluation run.
type Report struct {
	Results     []CaseResult
	Exact       int
	Injected    int
	Harmful     int
	DangerLeaks int
	ByLang      map[string][2]int // [n, exact]
	Collapse    float64
	Baselines   Baselines
}

// Evaluate runs the full routing workflow (not just the classifier) on every
// case as a general-purpose subagent brief, with the given session model.
// Abstain-labelled cases need the session tier: routing them anywhere lower
// counts as harmful. This is the gate CI enforces.
func Evaluate(cases []Case, base service.RouteInput) Report {
	rep := Report{ByLang: map[string][2]int{}}
	session := base.Session.Tier()
	for _, c := range cases {
		in := base
		in.Call = model.AgentCall{Prompt: c.Text, SubagentType: model.DefaultSubagentType}
		d, tr := service.Route(in)
		needed := session
		if c.Expect != model.ClassNone {
			needed = base.Table.Target(c.Expect).Min(session)
		}
		chosen := session
		if d.Rewrites() || d.Action == model.ActionShadow {
			chosen = d.Target
		}
		r := CaseResult{Case: c, Decision: d, Trace: tr, Got: gotClass(d)}
		r.Sufficient = !needed.Above(chosen)
		r.Harmful = !r.Sufficient
		rep.Results = append(rep.Results, r)
		if r.Got == c.Expect {
			rep.Exact++
		}
		if d.Rewrites() {
			rep.Injected++
		}
		if r.Harmful {
			rep.Harmful++
		}
		if c.Danger && (d.Rewrites() || !tr.Rule.Danger) {
			rep.DangerLeaks++
		}
		n := rep.ByLang[c.Lang]
		n[0]++
		if r.Got == c.Expect {
			n[1]++
		}
		rep.ByLang[c.Lang] = n
	}
	chosen := slices.Collect(fp.Map(slices.Values(rep.Results), func(r CaseResult) model.Tier {
		if r.Decision.Rewrites() {
			return r.Decision.Target
		}
		return session
	}))
	rep.Collapse = Collapse(chosen)
	rep.Baselines = baselines(rep.Results, base.Table, session, base.Settings.Eval)
	return rep
}

func gotClass(d model.Decision) model.Class {
	if d.Rewrites() || d.Action == model.ActionShadow || d.Reason == "same-tier" || d.Reason == "unknown-session" {
		return d.Class
	}
	switch d.Reason {
	case "danger-keep", "long-prompt-keep", "risk-hold", "context-keep", "gate-closed", "retry-keep":
		return d.Class
	}
	return model.ClassNone
}

func baselines(results []CaseResult, table model.TierTable, session model.Tier, ev model.EvalSettings) Baselines {
	b := Baselines{}
	n := float64(max(len(results), 1))
	for i := range results {
		r := &results[i]
		needed := session
		if r.Case.Expect != model.ClassNone {
			needed = table.Target(r.Case.Expect).Min(session)
		}
		chosen := session
		if r.Decision.Rewrites() {
			chosen = r.Decision.Target
		}
		static := session
		if r.Got != model.ClassNone {
			static = table.Target(r.Got).Min(session)
		}
		b.Router += bool01(!needed.Above(chosen)) / n
		b.AlwaysStrong += 1 / n
		b.AlwaysCheap += bool01(!needed.Above(model.TierHaiku)) / n
		b.Static += bool01(!needed.Above(static)) / n
		b.Oracle += 1 / n
		b.RouterCost += ev.Cost(chosen) / n
		b.StrongCost += ev.Cost(session) / n
		b.StaticCost += ev.Cost(static) / n
		b.OracleCost += ev.Cost(needed) / n
	}
	return b
}

func bool01(v bool) float64 {
	if v {
		return 1
	}
	return 0
}
