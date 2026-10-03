package service

import (
	"math"
	"regexp"
	"slices"
	"strings"

	"promari-model-router/internal/domain/model"
	"promari-model-router/pkg/graph"
)

// RouteInput is everything one routing decision depends on. It is plain data,
// so the decision is reproducible from a ledger row and the artifact version.
type RouteInput struct {
	Call     model.AgentCall
	Session  model.SessionModel
	Settings model.Settings
	Table    model.TierTable
	Lexicon  *Lexicon
	Artifact model.Artifact
	// Forced is true when CLAUDE_CODE_SUBAGENT_MODEL_FORCE disables routing
	// (model.SubagentModelForced; resolved by the caller, so the input stays
	// plain data that a ledger row can reproduce).
	Forced bool
	// Retried is true when the same brief was already delegated in this session.
	Retried bool
}

// RouteTrace records what every stage saw, for the ledger and `pmr explain`.
type RouteTrace struct {
	Path        []string
	Rule        model.Classification
	Source      string // tag | rule | model | fixed
	ModelClass  model.Class
	Probs       map[model.Class]float64
	Set         []model.Class
	PSafe       float64
	Tau         float64
	Unseen      float64
	NeighborSim float64
	Posterior   float64
	Escalated   bool
}

type routeState struct {
	in       RouteInput
	sig      Signals
	class    model.Class
	decision model.Decision
	done     bool
	trace    RouteTrace
	probs    []float64
}

// learned reports whether the artifact may drive this decision: the learned
// stages are enabled and the artifact was trained locally (or the embedded one
// is explicitly allowed). Every stage that reads the artifact — cascade, gate
// and ledger — asks this, so `--rules-only` and an untrusted artifact switch
// all of them off together.
func (s routeState) learned() bool {
	a, m := s.in.Artifact, s.in.Settings.Model
	return m.Enabled && (a.Trusted() || m.AllowEmbedded && a.Ready())
}

func (s routeState) stop(action model.Action, reason string) routeState {
	s.decision = s.decision.With(action, reason)
	if s.decision.Class == model.ClassNone {
		s.decision = s.decision.WithClass(s.class)
	}
	s.done = true
	return s
}

var routeTagRE = regexp.MustCompile(`(?i)^\s*\[route:\s*([a-z]+)\s*\]`)

// RouteTag returns the class of a leading `[route: <class>]` tag. The main
// model writes it (it understands the task in any language); the hook only
// enforces it (Planner-as-Router, arXiv:2609.32917). Only the very start of
// the brief counts, so a tag quoted from a file further down cannot steer it.
func RouteTag(prompt string) model.Class {
	if m := routeTagRE.FindStringSubmatch(prompt); m != nil {
		return model.ParseClass(strings.ToLower(m[1]))
	}
	return model.ClassNone
}

// Stage names of the routing workflow.
const (
	StageGuard   = "guard"
	StageSignals = "signals"
	StageTag     = "tag"
	StageCascade = "cascade"
	StageOOD     = "ood"
	StageFloor   = "floor"
	StageRisk    = "risk"
	StageLedger  = "ledger"
	StageGate    = "gate"
	StageFinish  = "finish"
)

// RoutingWorkflow is the decision as a state graph. Every stage either stops
// with a decision (edge to End) or hands over to the next stage.
var RoutingWorkflow = graph.New[routeState](StageGuard).
	WithNode(StageGuard, guardStage).
	WithNode(StageSignals, signalsStage).
	WithNode(StageTag, tagStage).
	WithNode(StageCascade, cascadeStage).
	WithNode(StageOOD, oodStage).
	WithNode(StageFloor, floorStage).
	WithNode(StageRisk, riskStage).
	WithNode(StageLedger, ledgerStage).
	WithNode(StageGate, gateStage).
	WithNode(StageFinish, finishStage).
	WithConditionalEdge(StageGuard, next(StageSignals), StageSignals, graph.End).
	WithConditionalEdge(StageSignals, next(StageTag), StageTag, graph.End).
	WithConditionalEdge(StageTag, afterTag, StageFloor, StageCascade, graph.End).
	WithConditionalEdge(StageCascade, next(StageOOD), StageOOD, graph.End).
	WithConditionalEdge(StageOOD, next(StageFloor), StageFloor, graph.End).
	WithConditionalEdge(StageFloor, next(StageRisk), StageRisk, graph.End).
	WithConditionalEdge(StageRisk, next(StageLedger), StageLedger, graph.End).
	WithConditionalEdge(StageLedger, next(StageGate), StageGate, graph.End).
	WithConditionalEdge(StageGate, next(StageFinish), StageFinish, graph.End).
	WithEdge(StageFinish, graph.End)

// afterTag routes past the tag stage: a tag skips classification but not the
// safety floors. The tag stage never stops today; the done case keeps the edge
// correct if it ever does, like every other edge.
func afterTag(s routeState) string {
	switch {
	case s.done:
		return graph.End
	case s.trace.Source == "tag":
		return StageFloor
	default:
		return StageCascade
	}
}

func next(stage string) graph.Router[routeState] {
	return func(s routeState) string {
		if s.done {
			return graph.End
		}
		return stage
	}
}

// Route runs the routing workflow for one Agent call.
func Route(in RouteInput) (model.Decision, RouteTrace) {
	init := routeState{in: in, decision: model.Decision{
		SubagentType: in.Call.SubagentTypeOrDefault(),
		Requested:    in.Call.Model,
	}}
	final, path, err := RoutingWorkflow.WithStepLimit(max(in.Settings.Runtime.WorkflowStepLimit, 1)).Invoke(init)
	final.trace.Path = path
	if err != nil {
		return init.decision.With(model.ActionNone, "workflow-error"), final.trace
	}
	return final.decision, final.trace
}

// guard handles everything that must not be routed at all.
func guardStage(s routeState) routeState {
	in := s.in
	session := in.Session.Tier()
	switch {
	case in.Settings.Routing.Mode == model.ModeOff:
		return s.stop(model.ActionSkip, "mode-off")
	case in.Forced:
		return s.stop(model.ActionSkip, "subagent-model-forced")
	case in.Call.Model != "":
		req := model.TierOf(in.Call.Model)
		if in.Settings.Routing.AskOnUpgrade && (req == model.TierFable || req.Above(session)) {
			s.decision = s.decision.WithTarget(req)
			return s.stop(model.ActionAsk, "explicit-upgrade")
		}
		return s.stop(model.ActionNone, "explicit-model")
	case !in.Table.IsBuiltinAgent(s.decision.SubagentType):
		return s.stop(model.ActionNone, "custom-agent")
	}
	if rule := in.Settings.Routing.Subagents[s.decision.SubagentType]; rule == "" || rule == model.RuleKeep {
		return s.stop(model.ActionNone, "policy-keep")
	}
	return s
}

func signalsStage(s routeState) routeState {
	s.sig = s.in.Lexicon.ExtractSignals(s.in.Call.Prompt)
	s.trace.Rule = RuleClassify(s.sig, s.in.Settings.Classifier)
	return s
}

func tagStage(s routeState) routeState {
	if !s.in.Settings.Routing.HonorRouteTag {
		return s
	}
	if tag := RouteTag(s.in.Call.Prompt); tag != model.ClassNone {
		s.class, s.trace.Source = tag, "tag"
	}
	return s
}

// cascade: a confident rule decides; otherwise a confident model decides;
// otherwise abstain (GuardChain, arXiv:2512.19011: only a confident stage decides).
func cascadeStage(s routeState) routeState {
	a := s.in.Artifact
	if s.learned() {
		s.probs = Probabilities(Featurize(s.sig, a.Features), a)
		s.trace.Probs = map[model.Class]float64{}
		for i, c := range a.Classes {
			s.trace.Probs[c] = s.probs[i]
		}
		s.trace.ModelClass = a.Classes[Argmax(s.probs)]
		s.trace.Set = PredictionSet(s.probs, a)
	}

	rule := s.in.Settings.Routing.Subagents[s.decision.SubagentType]
	if rule != model.RuleClassify {
		// A fixed rule (Explore -> lookup) still yields to evidence of deep work.
		// A tagged brief never reaches this stage (afterTag), so the model's
		// evidence is never weighed against a tag here.
		if s.trace.Rule.Class.Deep() || (s.trace.ModelClass.Deep() && len(s.trace.Set) == 1) {
			s.class = s.trace.Rule.Class
			return s.stop(model.ActionNone, "deep-work-keep")
		}
		s.class, s.trace.Source = model.ParseClass(string(rule)), "fixed"
		return s
	}
	switch {
	case s.trace.Rule.Class != model.ClassNone:
		s.class, s.trace.Source = s.trace.Rule.Class, "rule"
	// The argmax class is in every non-empty prediction set (it has the
	// largest probability), so one tier in the set is the whole condition.
	case s.probs != nil && len(TiersOf(s.trace.Set, s.in.Table)) == 1:
		s.class, s.trace.Source = s.trace.ModelClass, "model"
	}
	return s
}

// ood holds model-sourced decisions for prompts unlike anything in training.
func oodStage(s routeState) routeState {
	a := s.in.Artifact
	if !a.Ready() {
		return s
	}
	s.trace.Unseen = UnseenRatio(s.sig, a)
	if nn := NearestNeighbors(Featurize(s.sig, a.Features), a.Neighbors, max(s.in.Settings.Model.NeighborsK, 1)); len(nn) > 0 {
		s.trace.NeighborSim = nn[0].Score
	}
	if s.trace.Source != "model" {
		return s
	}
	maxUnseen := firstPositive(s.in.Settings.Model.MaxUnseenRatio, a.MaxUnseenRatio)
	minSim := firstPositive(s.in.Settings.Model.MinNeighborSim, a.MinNeighborSim)
	if (maxUnseen > 0 && s.trace.Unseen > maxUnseen) || (minSim > 0 && len(a.Neighbors) > 0 && s.trace.NeighborSim < minSim) {
		s.class = model.ClassNone
		return s.stop(model.ActionNone, "out-of-distribution")
	}
	return s
}

// floor applies the rules no score can override (the governance floor).
func floorStage(s routeState) routeState {
	p := s.in.Settings.Routing
	switch {
	case s.class == model.ClassNone:
		return s.stop(model.ActionNone, "abstain")
	case len(s.sig.Danger) > 0 && s.class.DowngradeSensitive() && s.trace.Source != "fixed":
		// A fixed rule routes a read-only built-in (Explore): reading auth code
		// on haiku cannot break it. The floor protects work that changes things.
		return s.stop(model.ActionNone, model.ReasonDangerKeep)
	case p.RetryEscalation && s.in.Retried:
		return s.stop(model.ActionNone, model.ReasonRetryKeep)
	case p.ContextHold && s.class.Cheap() && len(s.sig.ContextCues) > 0 && s.trace.Source != "tag":
		return s.stop(model.ActionNone, model.ReasonContextKeep)
	}
	return s
}

// risk: downgrade only when P(needed tier <= target) clears the risk-controlled
// threshold of the prompt's length bucket (CR², arXiv:2605.12001; Mondrian per
// bucket, arXiv:2608.14617). Without a learned artifact the fixed long-prompt
// guard stands in for it.
func riskStage(s routeState) routeState {
	a, p := s.in.Artifact, s.in.Settings.Routing.LongPrompt
	target := s.in.Table.Target(s.class)
	if s.trace.Source == "tag" {
		return s
	}
	if s.probs != nil {
		s.trace.PSafe = SafeProbability(s.probs, a, s.in.Table, target)
		s.trace.Tau = a.Tau[s.in.Settings.Buckets.Of(s.sig.Chars)]
		// A bucket without a learned τ reads as 0, which no probability is below.
		// Written as "not at least τ" so that a probability that is not a
		// number (NaN) holds instead of passing: every comparison with NaN is
		// false.
		if !(s.trace.PSafe >= s.trace.Tau) {
			return s.stop(model.ActionNone, model.ReasonRiskHold)
		}
		return s
	}
	if s.trace.Source == "rule" && s.class.Cheap() && s.sig.Chars >= max(p.Chars, 1) && s.trace.Rule.Margin < p.MinMargin {
		return s.stop(model.ActionNone, model.ReasonLongPromptKeep)
	}
	return s
}

// ledger: escalate when the observed success of the target tier on similar
// work is poor (CADMAS-CTX Beta posteriors with an uncertainty penalty,
// arXiv:2604.17950), then prefer the tier with the best μ − λ·ĉ (CARROT,
// arXiv:2502.03261) among tiers the posterior does not veto.
//
// No evidence and poor evidence are different: without observations of the
// target the static table's choice stands, but once a tier is seen failing,
// the call goes only to a tier the evidence clears. When none does (the tier
// above was never observed, or every tier up to the ceiling fails), the call
// is held on the session's tier rather than sent where it was seen to fail.
func ledgerStage(s routeState) routeState {
	a, p := s.in.Artifact, s.in.Settings.Model
	if !s.learned() || len(a.Posteriors) == 0 {
		return s
	}
	bucket := s.in.Settings.Buckets.Of(s.sig.Chars)
	target := s.in.Table.Target(s.class)
	for _, tier := range model.TierOrder[target.Rank():] {
		post, ok := a.Posteriors[model.PosteriorKey(s.class, bucket, tier)]
		switch {
		case tier == target && (tier.Above(s.in.Table.Ceiling) || !ok || post.Observations() < p.MinObservations):
			if !tier.Above(s.in.Table.Ceiling) {
				s.trace.Posterior = math.NaN()
			}
			return s // no evidence about the target: keep the static table's choice
		case tier.Above(s.in.Table.Ceiling):
			return s.stop(model.ActionNone, model.ReasonPosteriorHold) // seen failing up to the ceiling
		case !ok || post.Observations() < p.MinObservations:
			s.trace.Posterior = math.NaN()
			return s.stop(model.ActionNone, model.ReasonPosteriorHold) // nothing clears the tier above
		}
		score := post.Mean() - p.PosteriorGamma*math.Sqrt(post.Variance())
		s.trace.Posterior = score
		cost := a.CostMedian[model.CostKey(s.class, tier)] / max(p.CostScaleTokens, 1)
		if score-p.CostLambda*cost >= p.TargetSuccess {
			if tier != target {
				s.class = classForTier(s.in.Table, tier, s.class)
				s.trace.Escalated = true
			}
			return s
		}
	}
	// Every tier there is was seen failing.
	return s.stop(model.ActionNone, model.ReasonPosteriorHold)
}

func classForTier(table model.TierTable, tier model.Tier, fallback model.Class) model.Class {
	for _, c := range model.ClassOrder {
		if c.Rank() >= fallback.Rank() && table.Target(c) == tier {
			return c
		}
	}
	return fallback
}

// gate: Triage's two falsifiable conditions (arXiv:2604.07494) are checked at
// training time per class; a closed gate disables downgrades for that class.
func gateStage(s routeState) routeState {
	if !s.learned() {
		return s
	}
	if open, ok := s.in.Artifact.Gate[s.class]; ok && !open && s.trace.Source != "tag" {
		return s.stop(model.ActionNone, model.ReasonGateClosed)
	}
	return s
}

// finish caps the target at the session model and picks inject or shadow.
func finishStage(s routeState) routeState {
	target := s.in.Table.Target(s.class)
	s.decision = s.decision.WithClass(s.class)
	if !target.Known() {
		return s.stop(model.ActionNone, "no-target")
	}
	session := s.in.Session.Tier()
	if !session.Known() {
		if !slices.Contains(s.in.Settings.Routing.UnknownSessionAllows, target) {
			s.decision = s.decision.WithTarget(target)
			return s.stop(model.ActionNone, model.ReasonUnknownSession)
		}
	} else if target = target.Min(session); target == session {
		s.decision = s.decision.WithTarget(target)
		return s.stop(model.ActionNone, model.ReasonSameTier)
	}
	s.decision = s.decision.WithTarget(target)
	action := model.ActionInject
	if s.in.Settings.Routing.Mode == model.ModeShadow {
		action = model.ActionShadow
	}
	reason := s.trace.Source + ":" + string(s.class)
	if s.trace.Escalated {
		reason += "+posterior"
	}
	return s.stop(action, reason)
}

func firstPositive(values ...float64) float64 {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}
