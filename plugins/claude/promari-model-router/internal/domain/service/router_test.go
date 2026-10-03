package service_test

import (
	"maps"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/service"
	"promari-model-router/internal/infrastructure/settings"
	"promari-model-router/pkg/graph"
)

// fixtures load the shipped TOML (defaults, lexicon, tiers) through the real
// settings loader, with HOME pointed at an empty directory so no user or
// project override applies.
func fixtures(t *testing.T) (*service.Lexicon, model.TierTable, model.Settings) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_PROJECT_DIR", t.TempDir())
	st, _, problems := settings.Load("")
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	return settings.Lexicon(st), settings.Tiers(), st
}

// withoutClass returns a copy of the table in which class c has no tier.
func withoutClass(c model.Class) func(model.TierTable) model.TierTable {
	return func(t model.TierTable) model.TierTable {
		t.Classes = maps.Clone(t.Classes)
		delete(t.Classes, c)
		return t
	}
}

func TestRoute(t *testing.T) {
	lex, table, defaults := fixtures(t)
	opus := model.SessionModel{Model: defaults.Eval.SessionModel, Source: model.SourceTranscript}
	fable := model.SessionModel{Model: "claude-fable-5", Source: model.SourceTranscript}
	longBrief := "あなたはレビュー担当です。次のファイルを読んで一覧にして、不具合があれば修正して。" + strings.Repeat("内容の整合を見てください。", 40)

	tests := []struct {
		name    string
		call    model.AgentCall
		session model.SessionModel
		policy  func(model.Settings) model.Settings
		table   func(model.TierTable) model.TierTable
		forced  bool
		retried bool
		want    model.Decision
	}{
		{
			name: "japanese lookup goes to haiku",
			call: model.AgentCall{Prompt: "UserService がどこで定義されているか探して"},
			want: model.Decision{Action: model.ActionInject, Reason: "rule:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
		},
		{
			name: "standard work goes to sonnet",
			call: model.AgentCall{Prompt: "この関数の単体テストを書いて"},
			want: model.Decision{Action: model.ActionInject, Reason: "rule:standard", Target: model.TierSonnet, Class: model.ClassStandard},
		},
		{
			name: "route tag decides even for a long brief",
			call: model.AgentCall{Prompt: "[route: lookup]\n" + longBrief},
			want: model.Decision{Action: model.ActionInject, Reason: "tag:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
		},
		{
			name: "tag further down the brief is ignored",
			call: model.AgentCall{Prompt: "いい感じにして\n[route: lookup]"},
			want: model.Decision{Action: model.ActionNone, Reason: "abstain"},
		},
		{
			name:   "route tag is ignored when the policy disables it",
			call:   model.AgentCall{Prompt: "[route: lookup] いい感じにして"},
			policy: func(p model.Settings) model.Settings { p.Routing.HonorRouteTag = false; return p },
			want:   model.Decision{Action: model.ActionNone, Reason: "abstain"},
		},
		{
			name: "danger floor beats a cheap tag",
			call: model.AgentCall{Prompt: "[route: mechanical] 本番 DB の列名をリネームして"},
			want: model.Decision{Action: model.ActionNone, Reason: "danger-keep", Class: model.ClassMechanical},
		},
		{
			name: "long brief on a thin margin is kept",
			call: model.AgentCall{Prompt: longBrief},
			want: model.Decision{Action: model.ActionNone, Reason: "long-prompt-keep", Class: model.ClassLookup},
		},
		{
			name: "context-dependent brief is kept",
			call: model.AgentCall{Prompt: "上記の議論を踏まえて、該当ファイルを探して一覧にして"},
			want: model.Decision{Action: model.ActionNone, Reason: "context-keep", Class: model.ClassLookup},
		},
		{
			name:    "a retried brief is not downgraded again",
			call:    model.AgentCall{Prompt: "UserService がどこで定義されているか探して"},
			retried: true,
			want:    model.Decision{Action: model.ActionNone, Reason: "retry-keep", Class: model.ClassLookup},
		},
		{
			name:    "never above the session model",
			call:    model.AgentCall{Prompt: "この関数の単体テストを書いて"},
			session: model.SessionModel{Model: "haiku", Source: model.SourceTranscript},
			want:    model.Decision{Action: model.ActionNone, Reason: "same-tier", Target: model.TierHaiku, Class: model.ClassStandard},
		},
		{
			name:    "complex work on a fable session is capped at opus",
			call:    model.AgentCall{Prompt: "競合状態の原因を調べて"},
			session: model.SessionModel{Model: "claude-fable-5-1", Source: model.SourceTranscript},
			want:    model.Decision{Action: model.ActionInject, Reason: "rule:complex", Target: model.TierOpus, Class: model.ClassComplex},
		},
		{
			name:    "an uncertain session source only allows haiku",
			call:    model.AgentCall{Prompt: "この関数の単体テストを書いて"},
			session: model.SessionModel{Model: "claude-fable-5", Source: model.SourceEnv},
			want:    model.Decision{Action: model.ActionNone, Reason: "unknown-session", Target: model.TierSonnet, Class: model.ClassStandard},
		},
		{
			name:    "an uncertain session source still allows haiku",
			call:    model.AgentCall{Prompt: "UserService がどこで定義されているか探して"},
			session: model.SessionModel{Model: "claude-fable-5", Source: model.SourceEnv},
			want:    model.Decision{Action: model.ActionInject, Reason: "rule:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
		},
		{
			name:  "a class without a tier has no target",
			call:  model.AgentCall{Prompt: "[route: architecture] 全体を設計して"},
			table: withoutClass(model.ClassArchitecture),
			want:  model.Decision{Action: model.ActionNone, Reason: "no-target", Class: model.ClassArchitecture},
		},
		{
			name: "explicit model is respected",
			call: model.AgentCall{Prompt: "この関数の単体テストを書いて", Model: "opus"},
			want: model.Decision{Action: model.ActionNone, Reason: "explicit-model", Requested: "opus"},
		},
		{
			name:   "explicit fable is respected when asking is off",
			call:   model.AgentCall{Prompt: "x", Model: "fable"},
			policy: func(p model.Settings) model.Settings { p.Routing.AskOnUpgrade = false; return p },
			want:   model.Decision{Action: model.ActionNone, Reason: "explicit-model", Requested: "fable"},
		},
		{
			name:    "explicit upgrade asks",
			call:    model.AgentCall{Prompt: "x", Model: "opus"},
			session: model.SessionModel{Model: "sonnet", Source: model.SourceTranscript},
			want:    model.Decision{Action: model.ActionAsk, Reason: "explicit-upgrade", Target: model.TierOpus, Requested: "opus"},
		},
		{
			name: "explicit fable always asks",
			call: model.AgentCall{Prompt: "x", Model: "fable"},
			want: model.Decision{Action: model.ActionAsk, Reason: "explicit-upgrade", Target: model.TierFable, Requested: "fable"},
		},
		{
			// Not an upgrade over a fable session, but fable is the most
			// expensive tier: asking does not depend on the session.
			name:    "explicit fable on a fable session still asks",
			call:    model.AgentCall{Prompt: "x", Model: "fable"},
			session: fable,
			want:    model.Decision{Action: model.ActionAsk, Reason: "explicit-upgrade", Target: model.TierFable, Requested: "fable"},
		},
		{
			// "本番" is a danger cue, but complex work is not downgrade
			// sensitive: on a fable session it is still capped at opus.
			name:    "a danger cue does not hold work that is not downgrade sensitive",
			call:    model.AgentCall{Prompt: "本番の決済処理で競合状態の原因を調べて"},
			session: fable,
			want:    model.Decision{Action: model.ActionInject, Reason: "rule:complex", Target: model.TierOpus, Class: model.ClassComplex},
		},
		{
			name: "a context cue does not hold work that is not cheap",
			call: model.AgentCall{Prompt: "上記の議論を踏まえて、この関数の単体テストを書いて"},
			want: model.Decision{Action: model.ActionInject, Reason: "rule:standard", Target: model.TierSonnet, Class: model.ClassStandard},
		},
		{
			name: "a route tag is not held by a context cue",
			call: model.AgentCall{Prompt: "[route: lookup] 上記の議論を踏まえて探して"},
			want: model.Decision{Action: model.ActionInject, Reason: "tag:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
		},
		{
			// The lookup brief is 27 characters with a margin of 6. At
			// exactly long_prompt.chars it is long; a margin equal to
			// long_prompt.min_margin (6, the default) is enough.
			name: "a brief exactly long_prompt.chars long is held on a thin margin",
			call: model.AgentCall{Prompt: "UserService がどこで定義されているか探して"},
			policy: func(p model.Settings) model.Settings {
				p.Routing.LongPrompt.Chars, p.Routing.LongPrompt.MinMargin = 27, 7
				return p
			},
			want: model.Decision{Action: model.ActionNone, Reason: "long-prompt-keep", Class: model.ClassLookup},
		},
		{
			name: "a brief one character shorter than long_prompt.chars is not long",
			call: model.AgentCall{Prompt: "UserService がどこで定義されているか探して"},
			policy: func(p model.Settings) model.Settings {
				p.Routing.LongPrompt.Chars, p.Routing.LongPrompt.MinMargin = 28, 7
				return p
			},
			want: model.Decision{Action: model.ActionInject, Reason: "rule:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
		},
		{
			name:   "a long brief with a margin exactly long_prompt.min_margin goes through",
			call:   model.AgentCall{Prompt: "UserService がどこで定義されているか探して"},
			policy: func(p model.Settings) model.Settings { p.Routing.LongPrompt.Chars = 27; return p },
			want:   model.Decision{Action: model.ActionInject, Reason: "rule:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
		},
		{
			name: "custom agents keep their frontmatter",
			call: model.AgentCall{Prompt: "探して", SubagentType: "promari-model-router:scout"},
			want: model.Decision{Action: model.ActionNone, Reason: "custom-agent", SubagentType: "promari-model-router:scout"},
		},
		{
			name: "Plan is kept",
			call: model.AgentCall{Prompt: "設計して", SubagentType: "Plan"},
			want: model.Decision{Action: model.ActionNone, Reason: "policy-keep", SubagentType: "Plan"},
		},
		{
			name: "a built-in without a policy is kept",
			call: model.AgentCall{Prompt: "探して", SubagentType: "claude"},
			policy: func(p model.Settings) model.Settings {
				p.Routing.Subagents = maps.Clone(p.Routing.Subagents)
				delete(p.Routing.Subagents, "claude")
				return p
			},
			want: model.Decision{Action: model.ActionNone, Reason: "policy-keep", SubagentType: "claude"},
		},
		{
			name: "Explore defaults to haiku",
			call: model.AgentCall{Prompt: "認証まわりのファイルを探して", SubagentType: "Explore"},
			want: model.Decision{Action: model.ActionInject, Reason: "fixed:lookup", Target: model.TierHaiku, Class: model.ClassLookup, SubagentType: "Explore"},
		},
		{
			name: "Explore asked for deep work is kept",
			call: model.AgentCall{Prompt: "間欠的に落ちるテストの原因を特定して", SubagentType: "Explore"},
			want: model.Decision{Action: model.ActionNone, Reason: "deep-work-keep", Class: model.ClassComplex, SubagentType: "Explore"},
		},
		{
			name:   "FORCE env disables routing",
			call:   model.AgentCall{Prompt: "探して"},
			forced: true,
			want:   model.Decision{Action: model.ActionSkip, Reason: "subagent-model-forced"},
		},
		{
			name:   "shadow mode only records",
			call:   model.AgentCall{Prompt: "UserService がどこで定義されているか探して"},
			policy: func(p model.Settings) model.Settings { p.Routing.Mode = model.ModeShadow; return p },
			want:   model.Decision{Action: model.ActionShadow, Reason: "rule:lookup", Target: model.TierHaiku, Class: model.ClassLookup},
		},
		{
			name:   "mode off skips",
			call:   model.AgentCall{Prompt: "探して"},
			policy: func(p model.Settings) model.Settings { p.Routing.Mode = model.ModeOff; return p },
			want:   model.Decision{Action: model.ActionSkip, Reason: "mode-off"},
		},
		{
			name:   "a step limit below the path length is a workflow error",
			call:   model.AgentCall{Prompt: "UserService がどこで定義されているか探して"},
			policy: func(p model.Settings) model.Settings { p.Runtime.WorkflowStepLimit = 1; return p },
			want:   model.Decision{Action: model.ActionNone, Reason: "workflow-error"},
		},
		{
			name: "a zero step limit still runs the guard",
			call: model.AgentCall{Prompt: "探して"},
			policy: func(p model.Settings) model.Settings {
				p.Runtime.WorkflowStepLimit = 0
				p.Routing.Mode = model.ModeOff
				return p
			},
			want: model.Decision{Action: model.ActionSkip, Reason: "mode-off"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := defaults
			if tt.policy != nil {
				st = tt.policy(st)
			}
			tb := table
			if tt.table != nil {
				tb = tt.table(tb)
			}
			session := tt.session
			if session.Model == "" {
				session = opus
			}
			in := service.RouteInput{
				Call: tt.call, Session: session, Settings: st, Table: tb, Lexicon: lex,
				Forced: tt.forced, Retried: tt.retried,
			}
			got, trace := service.Route(in)
			want := tt.want
			if want.SubagentType == "" {
				want.SubagentType = "general-purpose"
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("Route() mismatch (-want +got):\n%s\npath: %v reasons: %v", diff, trace.Path, trace.Rule.Reasons)
			}
		})
	}
}

// Each stop leaves a different path through the graph; the tag skips
// classification but not the floors.
func TestRoutePath(t *testing.T) {
	lex, table, defaults := fixtures(t)
	opus := model.SessionModel{Model: defaults.Eval.SessionModel, Source: model.SourceTranscript}
	full := []string{
		service.StageGuard, service.StageSignals, service.StageTag, service.StageCascade, service.StageOOD,
		service.StageFloor, service.StageRisk, service.StageLedger, service.StageGate, service.StageFinish,
	}
	tests := []struct {
		name   string
		prompt string
		policy func(model.Settings) model.Settings
		want   []string
	}{
		{
			name:   "guard stops first",
			prompt: "探して",
			policy: func(p model.Settings) model.Settings { p.Routing.Mode = model.ModeOff; return p },
			want:   []string{service.StageGuard},
		},
		{
			name:   "a tag skips the cascade and the ood check",
			prompt: "[route: lookup] 探して",
			want: []string{
				service.StageGuard, service.StageSignals, service.StageTag,
				service.StageFloor, service.StageRisk, service.StageLedger, service.StageGate, service.StageFinish,
			},
		},
		{
			name:   "abstaining stops at the floor",
			prompt: "いい感じにして",
			want:   full[:6],
		},
		{
			name:   "a routed brief runs every stage",
			prompt: "UserService がどこで定義されているか探して",
			want:   full,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := defaults
			if tt.policy != nil {
				st = tt.policy(st)
			}
			_, trace := service.Route(service.RouteInput{
				Call: model.AgentCall{Prompt: tt.prompt}, Session: opus, Settings: st, Table: table, Lexicon: lex,
			})
			if diff := cmp.Diff(tt.want, trace.Path); diff != "" {
				t.Errorf("path mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRouteTag(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   model.Class
	}{
		{name: "leading tag", prompt: "[route: lookup] x", want: model.ClassLookup},
		{name: "case and spacing are ignored", prompt: "  [ROUTE:Architecture]\nplan", want: model.ClassArchitecture},
		{name: "unknown class", prompt: "[route: cheapest] x", want: model.ClassNone},
		{name: "tag not at the start", prompt: "x [route: lookup]", want: model.ClassNone},
		{name: "empty prompt", prompt: "", want: model.ClassNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, service.RouteTag(tt.prompt)); diff != "" {
				t.Errorf("RouteTag(%q) mismatch (-want +got):\n%s", tt.prompt, diff)
			}
		})
	}
}

// The routing graph must be complete: every stage has an outgoing edge and the
// entry exists. graph.Run would otherwise fail only on the path that reaches
// the broken stage. The dead-end row is the control that shows the check bites.
func TestRoutingWorkflowIsValid(t *testing.T) {
	tests := []struct {
		name     string
		validate func() error
		wantErr  error
	}{
		{name: "routing workflow", validate: service.RoutingWorkflow.Validate},
		{name: "routing workflow with a step limit", validate: service.RoutingWorkflow.WithStepLimit(1).Validate},
		{
			name:     "control: a stage without an outgoing edge",
			validate: service.RoutingWorkflow.WithNode("dead-end", nil).Validate,
			wantErr:  graph.ErrNoEdge,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.wantErr, tt.validate(), cmpopts.EquateErrors()); diff != "" {
				t.Errorf("Validate() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRoutingStagesIsTheWorkflowsSize(t *testing.T) {
	_, _, st := fixtures(t)
	tests := []struct {
		name string
		ok   bool
	}{
		{name: "the settings' minimum step limit counts every stage", ok: service.RoutingWorkflow.Len() == model.RoutingStages},
		{name: "the default step limit runs the whole workflow", ok: st.Runtime.WorkflowStepLimit >= service.RoutingWorkflow.Len()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.ok {
				t.Errorf("RoutingWorkflow has %d stages, model.RoutingStages = %d, default limit %d",
					service.RoutingWorkflow.Len(), model.RoutingStages, st.Runtime.WorkflowStepLimit)
			}
		})
	}
}
