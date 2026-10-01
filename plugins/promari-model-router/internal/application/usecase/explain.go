package usecase

import (
	"cmp"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/repository"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/service"
)

// Explanation is what `pmr explain` / `pmr classify` / the MCP tool return.
type Explanation struct {
	Class        string         `json:"class,omitempty"`
	Confidence   int            `json:"confidence"`
	Margin       int            `json:"margin"`
	Scores       map[string]int `json:"scores"`
	Danger       bool           `json:"danger"`
	Codex        string         `json:"codex,omitempty"`
	Continuation bool           `json:"continuation"`
	Lang         string         `json:"lang"`
	Chars        int            `json:"chars"`
	Decision     DecisionView   `json:"subagent_decision"`
	Trace        TraceView      `json:"trace"`
	Advice       string         `json:"advice,omitempty"`
}

// DecisionView is the serialisable decision.
type DecisionView struct {
	Action       string `json:"action"`
	Reason       string `json:"reason"`
	Target       string `json:"target,omitempty"`
	Class        string `json:"class,omitempty"`
	SubagentType string `json:"subagent_type"`
}

// ExplainUseCase shows how a prompt would be classified and routed. It reads
// the same environment the hook does, so a forced subagent model shows up as
// the skip it causes.
type ExplainUseCase struct {
	Config    repository.RoutingConfig
	Artifacts repository.ArtifactLoader
	Env       repository.EnvReader
}

// ExplainInput selects the scenario ("" = the default subagent type and
// the [eval].session_model).
type ExplainInput struct {
	Prompt       string
	SubagentType string
	SessionModel string
	Cwd          string
}

// Execute runs the routing workflow without touching the ledger.
func (u ExplainUseCase) Execute(in ExplainInput) Explanation {
	art, _ := u.Artifacts.Load() // a broken local artifact falls back to the embedded one; `pmr doctor` reports it
	lex := u.Config.Lexicon(in.Cwd)
	st := u.Config.Settings(in.Cwd)
	session := model.SessionModel{Model: cmp.Or(in.SessionModel, st.Eval.SessionModel), Source: model.SourceExplicit}
	d, tr := service.Route(service.RouteInput{
		Call:    model.AgentCall{Prompt: in.Prompt, SubagentType: in.SubagentType},
		Session: session, Settings: st, Table: u.Config.Tiers(), Lexicon: lex, Artifact: art,
		Forced: model.SubagentModelForced(u.Env.Getenv),
	})
	sig := lex.ExtractSignals(in.Prompt)
	cls := tr.Rule
	advice := service.Advise(service.AdviceInput{
		Classification: cls, Signals: sig, Session: session,
		Settings: st, Codex: model.CodexQuota{Available: true},
	})
	scores := map[string]int{}
	for c, v := range cls.Scores {
		scores[string(c)] = v
	}
	return Explanation{
		Class: string(cls.Class), Confidence: cls.Confidence, Margin: cls.Margin, Scores: scores,
		Danger: cls.Danger, Codex: string(cls.Codex), Continuation: cls.Continuation, Lang: string(cls.Lang), Chars: cls.Chars,
		Decision: DecisionView{Action: string(d.Action), Reason: d.Reason, Target: string(d.Target), Class: string(d.Class), SubagentType: d.SubagentType},
		Trace:    ViewOf(tr, st.Display.TraceDecimals), Advice: advice,
	}
}
