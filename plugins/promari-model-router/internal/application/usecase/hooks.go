// Package usecase holds the application services. Each use case depends only
// on the domain ports (repository interfaces) it uses, receives them by
// constructor injection, and has one reason to change.
package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/repository"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/service"
)

// Event is the part of a hook event the use cases read.
type Event struct {
	SessionID      string
	Cwd            string
	TranscriptPath string
	ToolUseID      string
	AgentID        string
}

// ErrorRecorder leaves a trace of a failure without failing the hook
// (implemented by RecordErrorUseCase).
type ErrorRecorder interface {
	RecordError(ctx context.Context, where, detail string)
}

// ---------------------------------------------------------------- errors

// RecordErrorUseCase records a failed hook for `pmr doctor`: in the ledger
// when it can be written, else in the failure file beside it (last_error).
type RecordErrorUseCase struct {
	Ledger   repository.LedgerWriter
	Failures repository.FailureRecorder
	Clock    repository.Clock
	// DetailRunes is how much of the detail (its tail) is kept.
	DetailRunes int
	// Timeout bounds the write. The hook's own context may already be
	// cancelled or out of time when it fails, so the write runs on a context
	// that keeps its values but not its cancellation.
	Timeout time.Duration
}

var _ ErrorRecorder = RecordErrorUseCase{}

// RecordError implements ErrorRecorder.
func (u RecordErrorUseCase) RecordError(ctx context.Context, where, detail string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), u.Timeout)
	defer cancel()
	now := u.Clock.Now()
	detail = truncate(detail, u.DetailRunes)
	if err := u.Ledger.Append(ctx, model.NewEntry(now, model.EventError, model.WithFailure(where, detail))); err != nil {
		// Nothing is left to report a failure of this write to.
		_ = u.Failures.RecordFailure(model.Failure{At: now, Message: where + ": " + detail + "\n(ledger unavailable: " + err.Error() + ")"})
	}
}

// truncate keeps the last n runes (the failing frame ends a stack trace).
func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[len(r)-n:])
	}
	return s
}

// recorder appends to the ledger and reports a failed append (the ledger must
// never block a prompt, and must never fail in silence either).
type recorder struct {
	ledger repository.LedgerWriter
	errors ErrorRecorder
}

func (r recorder) record(ctx context.Context, where string, e model.Entry) {
	if err := r.ledger.Append(ctx, e); err != nil {
		r.errors.RecordError(ctx, where, "append "+string(e.Event)+": "+err.Error())
	}
}

// ---------------------------------------------------------------- SessionStart

// SessionStartConfig is the configuration SessionStart reads: the settings
// and which override files were rejected (to warn about them).
type SessionStartConfig interface {
	repository.SettingsProvider
	repository.ConfigDiagnostics
}

// SessionStart records the session model, prunes old data, and returns the
// context to inject (the routing policy, and warnings).
type SessionStart struct {
	Sessions         repository.SessionWriter
	SessionRetention repository.RetentionPruner
	LedgerRetention  repository.RetentionPruner
	Ledger           repository.LedgerWriter
	Config           SessionStartConfig
	Env              repository.EnvReader
	Clock            repository.Clock
	Errors           ErrorRecorder
}

// SessionStartInput is the SessionStart event.
type SessionStartInput struct {
	Event
	Model  string
	Source string
}

const whereSessionStart = "SessionStart"

// Execute runs the use case.
func (u SessionStart) Execute(ctx context.Context, in SessionStartInput) string {
	now := u.Clock.Now()
	rec := recorder{ledger: u.Ledger, errors: u.Errors}
	if in.Model != "" {
		if err := u.Sessions.Save(ctx, model.NewSession(in.SessionID).SwitchModel(in.Model, model.SourceSessionState, now)); err != nil {
			u.Errors.RecordError(ctx, whereSessionStart, "save session: "+err.Error())
		}
	}
	st := u.Config.Settings(in.Cwd)
	if _, err := u.LedgerRetention.PruneOlderThan(ctx, now.AddDate(0, 0, -st.Runtime.LedgerRetentionDays)); err != nil {
		u.Errors.RecordError(ctx, whereSessionStart, "prune ledger: "+err.Error())
	}
	if _, err := u.SessionRetention.PruneOlderThan(ctx, now.AddDate(0, 0, -st.Runtime.SessionRetentionDays)); err != nil {
		u.Errors.RecordError(ctx, whereSessionStart, "prune sessions: "+err.Error())
	}
	rec.record(ctx, whereSessionStart, model.NewEntry(now, model.EventSessionStart,
		model.WithSession(in.SessionID, model.SessionModel{Model: in.Model, Source: model.SourceSessionState}),
		model.WithDetail("source="+in.Source+" mode="+string(st.Routing.Mode))))

	var notes []string
	if st.Runtime.InjectSessionPolicy && st.Routing.Mode != model.ModeOff {
		notes = append(notes, service.SessionContext(st.Messages))
	}
	if model.SubagentModelForced(u.Env.Getenv) {
		notes = append(notes, st.Messages.Prefix+" "+st.Messages.ForceDisabled)
	}
	// A rejected file is otherwise invisible until someone runs `pmr doctor`:
	// a mistyped `mode = "off"` would leave routing on without a word.
	for _, problem := range u.Config.Problems(in.Cwd) {
		rec.record(ctx, whereSessionStart, model.NewEntry(now, model.EventError,
			model.WithSession(in.SessionID, model.SessionModel{}), model.WithFailure(whereSessionStart+": config rejected", problem)))
		notes = append(notes, st.Messages.Prefix+" "+service.Render(st.Messages.ConfigRejected, map[string]string{"Problem": problem}))
	}
	return strings.Join(notes, "\n")
}

// ---------------------------------------------------------------- PostModelSwitch

// ModelSwitch follows /model changes.
type ModelSwitch struct {
	Sessions repository.SessionWriter
	Ledger   repository.LedgerWriter
	Clock    repository.Clock
	Errors   ErrorRecorder
}

const whereModelSwitch = "PostModelSwitch"

// Execute records the new session model.
func (u ModelSwitch) Execute(ctx context.Context, ev Event, from, to string) {
	now := u.Clock.Now()
	if to != "" {
		if err := u.Sessions.Save(ctx, model.NewSession(ev.SessionID).SwitchModel(to, model.SourceSessionState, now)); err != nil {
			u.Errors.RecordError(ctx, whereModelSwitch, "save session: "+err.Error())
		}
	}
	recorder{ledger: u.Ledger, errors: u.Errors}.record(ctx, whereModelSwitch, model.NewEntry(now, model.EventModelSwitch,
		model.WithSession(ev.SessionID, model.SessionModel{Model: to, Source: model.SourceSessionState}),
		model.WithDetail("from="+from)))
}

// ---------------------------------------------------------------- UserPromptSubmit

// PromptSubmit classifies the user's prompt and returns advice (never blocks).
type PromptSubmit struct {
	Config   repository.RoutingConfig
	Resolver SessionResolver
	Usage    repository.UsageReader
	Ledger   repository.LedgerWriter
	Clock    repository.Clock
	Errors   ErrorRecorder
}

// Execute runs the use case.
func (u PromptSubmit) Execute(ctx context.Context, ev Event, prompt string) string {
	lex := u.Config.Lexicon(ev.Cwd)
	sig := lex.ExtractSignals(prompt)
	st := u.Config.Settings(ev.Cwd)
	cls := service.RuleClassify(sig, st.Classifier)
	session := u.Resolver.Resolve(ctx, ev)
	pressure := u.Usage.Claude()
	advice := service.Advise(service.AdviceInput{
		Classification: cls, Signals: sig, Session: session,
		Settings: st, Pressure: pressure, Codex: u.Usage.Codex(),
	})
	recorder{ledger: u.Ledger, errors: u.Errors}.record(ctx, "UserPromptSubmit", model.NewEntry(u.Clock.Now(), model.EventPrompt,
		model.WithSession(ev.SessionID, session), model.WithClassification(cls), model.WithPrompt(prompt),
		model.WithAdvice(advice != "", pressure.High)))
	return advice
}

// ---------------------------------------------------------------- PreToolUse(Agent)

// SubagentStart routes one Agent call.
type SubagentStart struct {
	Config    repository.RoutingConfig
	Resolver  SessionResolver
	Artifacts repository.ArtifactLoader
	History   repository.PromptHistory
	Ledger    repository.LedgerWriter
	Env       repository.EnvReader
	Clock     repository.Clock
	Errors    ErrorRecorder
}

// Execute returns the decision and its trace. A failed lookup never stops
// routing (fail-open): the decision goes on with what is known, and the
// ledger row lists what was missing under "degraded".
func (u SubagentStart) Execute(ctx context.Context, ev Event, call model.AgentCall) (model.Decision, service.RouteTrace) {
	session := u.Resolver.Resolve(ctx, ev)
	st := u.Config.Settings(ev.Cwd)
	var degraded []string
	art, err := u.Artifacts.Load()
	if err != nil {
		degraded = append(degraded, "artifact: "+err.Error())
	}
	sha := model.PromptDigest(call.Prompt)
	retried, err := u.History.SeenPrompt(ctx, ev.SessionID, sha)
	if err != nil {
		degraded = append(degraded, "seen_prompt: "+err.Error())
	}
	d, tr := service.Route(service.RouteInput{
		Call: call, Session: session, Settings: st, Table: u.Config.Tiers(),
		Lexicon: u.Config.Lexicon(ev.Cwd), Artifact: art, Forced: model.SubagentModelForced(u.Env.Getenv), Retried: retried,
	})
	recorder{ledger: u.Ledger, errors: u.Errors}.record(ctx, "PreToolUse", model.NewEntry(u.Clock.Now(), model.EventSubagent,
		model.WithSession(ev.SessionID, session), model.WithClassification(tr.Rule), model.WithDecision(d),
		model.WithPrompt(call.Prompt), model.WithToolUse(ev.ToolUseID, ev.AgentID != ""),
		model.WithDetail(traceDetail(tr, st.Display.TraceDecimals, degraded))))
	return d, tr
}

// AskReason is the text shown when the user is asked to approve a subagent
// that runs above the session model ([messages].ask_upgrade).
func (u SubagentStart) AskReason(cwd string, target model.Tier) string {
	return service.Render(u.Config.Settings(cwd).Messages.AskUpgrade, map[string]string{"Target": string(target)})
}

// ---------------------------------------------------------------- PostToolUse(Agent)

// SubagentFinish records what actually ran.
type SubagentFinish struct {
	Ledger repository.LedgerWriter
	Clock  repository.Clock
	Errors ErrorRecorder
}

// Execute records the outcome.
func (u SubagentFinish) Execute(ctx context.Context, ev Event, o model.SubagentOutcome, prompt string) {
	recorder{ledger: u.Ledger, errors: u.Errors}.record(ctx, "PostToolUse", model.NewEntry(u.Clock.Now(), model.EventSubagentResult,
		model.WithSession(ev.SessionID, model.SessionModel{}), model.WithPrompt(prompt), model.WithOutcome(o)))
}
