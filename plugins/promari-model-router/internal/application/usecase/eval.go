package usecase

import (
	"cmp"
	"fmt"

	"promari-model-router/internal/domain/learn"
	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/repository"
	"promari-model-router/internal/domain/service"
)

// EvalSummary is the serialisable evaluation result.
type EvalSummary struct {
	Cases       int              `json:"cases"`
	Exact       int              `json:"exact"`
	Accuracy    float64          `json:"accuracy"`
	Injected    int              `json:"injected"`
	Harmful     int              `json:"harmful_downgrades"`
	HarmfulRate float64          `json:"harmful_rate_of_injected"`
	DangerLeaks int              `json:"danger_leaks"`
	Collapse    float64          `json:"collapse"`
	ByLang      map[string]Ratio `json:"exact_by_lang"`
	Baselines   learn.Baselines  `json:"baselines"`
	Misses      []Miss           `json:"misses,omitempty"`
	Gate        EvalGate         `json:"gate"`
	Passed      bool             `json:"passed"`
	// FailedBecause lists why the gate failed (empty when it passed).
	FailedBecause []string `json:"failed_because,omitempty"`
}

// Miss is one case the router got wrong (Got differs from Want, or the
// route went below the tier the case needs).
type Miss struct {
	Want    string `json:"want"`
	Got     string `json:"got"`
	Harmful bool   `json:"harmful"`
	Text    string `json:"text"`
}

// EvalGate is the threshold set an evaluation was judged against.
type EvalGate struct {
	MinAccuracy  float64 `json:"min_accuracy"`
	MaxHarmful   int     `json:"max_harmful"`
	SessionModel string  `json:"session_model"`
}

func derefOr[T any](p *T, fallback T) T {
	if p != nil {
		return *p
	}
	return fallback
}

// EvalUseCase runs the evaluation gate.
type EvalUseCase struct {
	Config    repository.RoutingConfig
	Artifacts repository.ArtifactLoader
	Cases     repository.CaseSource
}

// EvalOptions are the gate thresholds.
type EvalOptions struct {
	Path         string
	SessionModel string
	MinAccuracy  *float64 // nil = [eval].min_accuracy
	MaxHarmful   *int     // nil = [eval].max_harmful
	UseModel     *bool
	// AllowEmbedded evaluates with the synthetic embedded artifact.
	AllowEmbedded bool
	Cwd           string
}

// Execute evaluates the full routing workflow.
func (u EvalUseCase) Execute(opt EvalOptions) (EvalSummary, error) {
	st := u.Config.Settings(opt.Cwd)
	cases, err := u.Cases.Cases(opt.Path)
	if err != nil {
		return EvalSummary{}, err
	}
	art, _ := u.Artifacts.Load()
	if opt.UseModel != nil {
		st.Model.Enabled = *opt.UseModel
	}
	st.Model.AllowEmbedded = st.Model.AllowEmbedded || opt.AllowEmbedded
	// Evaluate the routing policy itself: a rollout mode set in an override
	// file (shadow/off) would otherwise turn every decision into "none".
	st.Routing.Mode = model.ModeEnforce
	session := cmp.Or(opt.SessionModel, st.Eval.SessionModel)
	minAccuracy := derefOr(opt.MinAccuracy, st.Eval.MinAccuracy)
	maxHarmful := derefOr(opt.MaxHarmful, st.Eval.MaxHarmful)
	rep := learn.Evaluate(cases, service.RouteInput{
		Session:  model.SessionModel{Model: session, Source: model.SourceExplicit},
		Settings: st, Table: u.Config.Tiers(), Lexicon: u.Config.Lexicon(opt.Cwd), Artifact: art,
	})
	s := EvalSummary{
		Cases: len(cases), Exact: rep.Exact, Injected: rep.Injected, Harmful: rep.Harmful,
		DangerLeaks: rep.DangerLeaks, Collapse: rep.Collapse, Baselines: rep.Baselines, ByLang: map[string]Ratio{},
	}
	if len(cases) > 0 {
		s.Accuracy = float64(rep.Exact) / float64(len(cases))
	}
	if rep.Injected > 0 {
		s.HarmfulRate = float64(rep.Harmful) / float64(rep.Injected)
	}
	for lang, n := range rep.ByLang {
		s.ByLang[lang] = Ratio{Part: n[1], Whole: n[0]}
	}
	for i := range rep.Results {
		r := &rep.Results[i]
		if r.Harmful || r.Got != r.Case.Expect {
			s.Misses = append(s.Misses, Miss{Want: orAbstain(r.Case.Expect), Got: orAbstain(r.Got), Harmful: r.Harmful, Text: r.Case.Text})
		}
	}
	s.Gate = EvalGate{MinAccuracy: minAccuracy, MaxHarmful: maxHarmful, SessionModel: session}
	// A gate over no cases, or against a session model whose tier is unknown
	// (every route would stop at "unknown-session"), measures nothing: it
	// fails whatever the thresholds say.
	fail := func(ok bool, why string) {
		if !ok {
			s.FailedBecause = append(s.FailedBecause, why)
		}
	}
	fail(len(cases) > 0, "no cases")
	fail(model.TierOf(session).Known(), fmt.Sprintf("session model %q has no known tier", session))
	fail(s.Accuracy >= minAccuracy, fmt.Sprintf("accuracy %.3f < %.2f", s.Accuracy, minAccuracy))
	fail(s.Harmful <= maxHarmful, fmt.Sprintf("harmful downgrades %d > %d", s.Harmful, maxHarmful))
	fail(s.DangerLeaks == 0, fmt.Sprintf("danger leaks %d", s.DangerLeaks))
	s.Passed = len(s.FailedBecause) == 0
	return s, nil
}

func orAbstain(c model.Class) string {
	if c == model.ClassNone {
		return "abstain"
	}
	return string(c)
}
