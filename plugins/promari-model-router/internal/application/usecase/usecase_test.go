package usecase_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/application/usecase"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/service"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/pkg/fp"
)

const lookupPrompt = "UserService がどこで定義されているか探して"

func TestSessionStart(t *testing.T) {
	tests := []struct {
		name       string
		opts       []fixtureOpt
		in         usecase.SessionStartInput
		wantPolicy bool
		wantForce  bool
		wantSaved  bool
		wantDetail string
	}{
		{
			name:       "model recorded and policy injected",
			in:         usecase.SessionStartInput{Event: usecase.Event{SessionID: "s1"}, Model: "claude-opus-5-5", Source: "startup"},
			wantPolicy: true, wantSaved: true, wantDetail: "source=startup mode=enforce",
		},
		{
			name:       "no model: session not saved",
			in:         usecase.SessionStartInput{Event: usecase.Event{SessionID: "s1"}, Source: "resume"},
			wantPolicy: true, wantDetail: "source=resume mode=enforce",
		},
		{
			name:       "mode off: no policy",
			opts:       []fixtureOpt{withSettings(func(s *model.Settings) { s.Routing.Mode = model.ModeOff })},
			in:         usecase.SessionStartInput{Event: usecase.Event{SessionID: "s1"}, Model: "claude-opus-5-5", Source: "startup"},
			wantSaved:  true,
			wantDetail: "source=startup mode=off",
		},
		{
			name:       "policy injection disabled",
			opts:       []fixtureOpt{withSettings(func(s *model.Settings) { s.Runtime.InjectSessionPolicy = false })},
			in:         usecase.SessionStartInput{Event: usecase.Event{SessionID: "s1"}, Source: "startup"},
			wantDetail: "source=startup mode=enforce",
		},
		{
			name:       "force variable set in the environment",
			opts:       []fixtureOpt{withEnv(map[string]string{model.ForceEnv: "1"})},
			in:         usecase.SessionStartInput{Event: usecase.Event{SessionID: "s1"}, Source: "startup"},
			wantPolicy: true, wantForce: true, wantDetail: "source=startup mode=enforce",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, tt.opts...)
			st := f.config.Settings("")
			got := f.sessionStart().Execute(t.Context(), tt.in)

			if has := strings.Contains(got, "route tag"); has != tt.wantPolicy {
				t.Errorf("policy injected = %v, want %v: %q", has, tt.wantPolicy, got)
			}
			if has := strings.Contains(got, st.Messages.ForceDisabled); has != tt.wantForce {
				t.Errorf("force note = %v, want %v: %q", has, tt.wantForce, got)
			}
			if !tt.wantPolicy && !tt.wantForce && got != "" {
				t.Errorf("want no context, got %q", got)
			}
			_, saved := f.sessions.m["s1"]
			if saved != tt.wantSaved {
				t.Errorf("session saved = %v, want %v", saved, tt.wantSaved)
			}
			wantPruned := []time.Time{testNow.AddDate(0, 0, -st.Runtime.LedgerRetentionDays)}
			if diff := cmp.Diff(wantPruned, f.ledger.pruned); diff != "" {
				t.Errorf("ledger prune cutoff (-want +got):\n%s", diff)
			}
			if len(f.ledger.entries) != 1 {
				t.Fatalf("entries = %d, want 1", len(f.ledger.entries))
			}
			e := f.ledger.entries[0]
			if diff := cmp.Diff([]string{string(model.EventSessionStart), tt.wantDetail, tt.in.Model},
				[]string{string(e.Event), e.Detail, e.SessionModel}); diff != "" {
				t.Errorf("entry (-want +got):\n%s", diff)
			}
		})
	}
}

func TestModelSwitch(t *testing.T) {
	tests := []struct {
		name      string
		from, to  string
		appendErr error
		saveErr   error
		wantSaved bool
		wantKinds []model.EventKind // ledger rows
		wantFail  string            // prefix of the failure file ("" = none)
	}{
		{name: "switch saves the new model", from: "claude-opus-5-5", to: "claude-sonnet-4-6", wantSaved: true, wantKinds: []model.EventKind{model.EventModelSwitch}},
		{name: "empty target is not saved", from: "claude-opus-5-5", to: "", wantKinds: []model.EventKind{model.EventModelSwitch}},
		{
			// The failed save used to be dropped without a trace.
			name: "a failed save is recorded as an error", from: "a", to: "claude-sonnet-4-6", saveErr: errSessions,
			wantKinds: []model.EventKind{model.EventError, model.EventModelSwitch},
		},
		{
			// A failing ledger never blocks the hook, and used to fail in silence:
			// the failure now lands in the failure file beside the ledger.
			name: "a failing ledger leaves the failure file", from: "claude-opus-5-5", to: "claude-sonnet-4-6", appendErr: errLedger,
			wantSaved: true, wantFail: "PostModelSwitch: append model_switch: ledger down",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.ledger.appendErr, f.sessions.saveErr = tt.appendErr, tt.saveErr
			f.modelSwitch().Execute(t.Context(), usecase.Event{SessionID: "s1"}, tt.from, tt.to)
			s, saved := f.sessions.m["s1"]
			if saved != tt.wantSaved || (saved && s.Model != tt.to) {
				t.Errorf("session = %+v (saved %v), want model %q saved %v", s, saved, tt.to, tt.wantSaved)
			}
			kinds := make([]model.EventKind, 0, len(f.ledger.entries))
			for _, e := range f.ledger.entries {
				kinds = append(kinds, e.Event)
			}
			if diff := cmp.Diff(tt.wantKinds, kinds, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("ledger (-want +got):\n%s", diff)
			}
			fail := ""
			if f.failures.last != nil {
				fail = f.failures.last.Message
			}
			if !strings.HasPrefix(fail, tt.wantFail) || (tt.wantFail == "") != (fail == "") {
				t.Errorf("failure file = %q, want prefix %q", fail, tt.wantFail)
			}
			for _, e := range f.ledger.entries {
				if e.Event == model.EventModelSwitch {
					if diff := cmp.Diff([]string{"from=" + tt.from, tt.to}, []string{e.Detail, e.SessionModel}); diff != "" {
						t.Errorf("entry (-want +got):\n%s", diff)
					}
				}
			}
		})
	}
}

func TestSessionStartFailuresAndRejectedConfig(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(f *fixture)
		wantErrors  []string // Reason: Detail of the error rows
		wantContext string   // a substring of the injected context
	}{
		{
			name: "save and prune failures are recorded, the session still starts",
			setup: func(f *fixture) {
				f.sessions.saveErr, f.sessions.pruneErr, f.ledger.pruneErr = errSessions, errSessions, errLedger
			},
			wantErrors: []string{
				"SessionStart: save session: sessions down", "SessionStart: prune ledger: ledger down", "SessionStart: prune sessions: sessions down",
			},
			wantContext: "route tag",
		},
		{
			// A rejected file (a typo in `mode = "off"`) used to be visible only
			// to someone who ran `pmr doctor`.
			name:        "a rejected configuration file is recorded and announced",
			setup:       func(f *fixture) { f.config.problems = []string{"/p/.claude/promari-model-router.toml: routing.mdoe"} },
			wantErrors:  []string{"SessionStart: config rejected: /p/.claude/promari-model-router.toml: routing.mdoe"},
			wantContext: "was rejected, so it has no effect (the defaults and the other files apply): /p/.claude/promari-model-router.toml: routing.mdoe",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, tt.setup)
			got := f.sessionStart().Execute(t.Context(), usecase.SessionStartInput{SessionID: "s1", Model: "claude-opus-5-5"})
			var errs []string
			for _, e := range f.ledger.entries {
				if e.Event == model.EventError {
					errs = append(errs, e.Reason+": "+e.Detail)
				}
			}
			if diff := cmp.Diff(tt.wantErrors, errs); diff != "" {
				t.Errorf("error rows (-want +got):\n%s", diff)
			}
			if !strings.Contains(got, tt.wantContext) {
				t.Errorf("context %q lacks %q", got, tt.wantContext)
			}
		})
	}
}

func TestPromptSubmit(t *testing.T) {
	tests := []struct {
		name        string
		transcript  string
		stored      *model.Session
		findErr     error
		env, file   string
		pressure    model.Pressure
		prompt      string
		wantModel   string
		wantSource  model.SessionSource
		wantClass   model.Class
		wantAdvised bool
	}{
		{
			name:       "transcript wins over every other source",
			transcript: "claude-opus-5-5", stored: &model.Session{ID: "s1", Model: "claude-sonnet-4-6"},
			env: "claude-haiku-4-5", file: "claude-haiku-4-5",
			prompt: lookupPrompt, wantModel: "claude-opus-5-5", wantSource: model.SourceTranscript, wantClass: model.ClassLookup,
		},
		{
			name:   "stored session model",
			stored: &model.Session{ID: "s1", Model: "claude-sonnet-4-6"}, env: "claude-haiku-4-5",
			prompt: lookupPrompt, wantModel: "claude-sonnet-4-6", wantSource: model.SourceSessionState, wantClass: model.ClassLookup,
		},
		{
			name:    "session lookup error falls through to the environment",
			findErr: errSessions, env: "claude-opus-5-5",
			pressure: model.Pressure{Known: true, High: true, FiveHour: fp.Some(95.0)},
			prompt:   lookupPrompt, wantModel: "claude-opus-5-5", wantSource: model.SourceEnv, wantClass: model.ClassLookup,
		},
		{
			name: "settings file model", file: "claude-opus-5-5",
			prompt: lookupPrompt, wantModel: "claude-opus-5-5", wantSource: model.SourceSettings, wantClass: model.ClassLookup,
		},
		{
			name:   "nothing known: unknown session, unclassified prompt",
			prompt: "ok", wantSource: model.SourceUnknown, wantClass: model.ClassNone,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, func(f *fixture) {
				f.deps.Resolver.Transcript = fakeTranscript{model: tt.transcript}
				f.deps.Resolver.Settings = fakeSettings{env: tt.env, file: tt.file}
				f.deps.Usage = fakeUsage{claude: tt.pressure, codex: model.CodexQuota{Available: true}}
				f.sessions.findErr = tt.findErr
				if tt.stored != nil {
					f.sessions.m[tt.stored.ID] = *tt.stored
				}
			})
			advice := f.promptSubmit().Execute(t.Context(), usecase.Event{SessionID: "s1"}, tt.prompt)
			if len(f.ledger.entries) != 1 {
				t.Fatalf("entries = %d, want 1", len(f.ledger.entries))
			}
			e := f.ledger.entries[0]
			type view struct {
				Event        model.EventKind
				Model        string
				Source       model.SessionSource
				Class        model.Class
				Advised      bool
				PressureHigh bool
				At           time.Time
			}
			want := view{model.EventPrompt, tt.wantModel, tt.wantSource, tt.wantClass, advice != "", tt.pressure.High, testNow}
			got := view{e.Event, e.SessionModel, e.SessionSource, e.Class, e.Advised, e.PressureHigh, e.At}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("entry (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSubagentStart(t *testing.T) {
	lookup := model.Decision{Action: model.ActionInject, Reason: "rule:lookup", Target: model.TierHaiku, Class: model.ClassLookup, SubagentType: "general-purpose"}
	tests := []struct {
		name       string
		ev         usecase.Event
		seen       bool
		seenErr    error
		loadErr    error
		want       model.Decision
		wantNested bool
		wantDegr   []string
	}{
		{name: "lookup brief is downgraded", ev: usecase.Event{SessionID: "s1", ToolUseID: "t1"}, want: lookup},
		{
			name: "the same brief again is a retry: not downgraded again",
			ev:   usecase.Event{SessionID: "s1", ToolUseID: "t2"}, seen: true,
			want: model.Decision{Action: model.ActionNone, Reason: "retry-keep", Class: model.ClassLookup, SubagentType: "general-purpose"},
		},
		{
			name: "retry lookup failure counts as a first attempt, marked degraded",
			ev:   usecase.Event{SessionID: "s1", ToolUseID: "t3"}, seen: true, seenErr: errLedger, want: lookup,
			wantDegr: []string{"seen_prompt: ledger down"},
		},
		{
			name: "nested call with an unreadable artifact, marked degraded",
			ev:   usecase.Event{SessionID: "s1", ToolUseID: "t4", AgentID: "a1"}, loadErr: errArtifact,
			want: lookup, wantNested: true, wantDegr: []string{"artifact: no artifact"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, func(f *fixture) {
				f.deps.Resolver.Transcript = fakeTranscript{model: "claude-opus-5-5"}
				f.ledger.seenErr = tt.seenErr
				f.artifacts.loadErr = tt.loadErr
				if tt.seen {
					f.ledger.entries = append(f.ledger.entries, model.Entry{
						At: testNow, Event: model.EventSubagent, SessionID: "s1", PromptSHA: model.PromptDigest(lookupPrompt),
					})
				}
			})
			before := len(f.ledger.entries)
			got, tr := f.subagentStart().Execute(t.Context(), tt.ev, model.AgentCall{Prompt: lookupPrompt})
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("decision (-want +got):\n%s", diff)
			}
			if len(f.ledger.entries) != before+1 {
				t.Fatalf("entries = %d, want %d", len(f.ledger.entries), before+1)
			}
			e := f.ledger.entries[before]
			var detail struct {
				usecase.TraceView
				Degraded []string `json:"degraded"`
			}
			if err := json.Unmarshal([]byte(e.Detail), &detail); err != nil {
				t.Fatalf("detail is not a trace: %v (%q)", err, e.Detail)
			}
			if diff := cmp.Diff([]any{tr.Path, tt.wantDegr}, []any{detail.Path, detail.Degraded}); diff != "" {
				t.Errorf("trace path and degraded (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff([]any{model.EventSubagent, tt.ev.ToolUseID, tt.wantNested, got.Action, testNow},
				[]any{e.Event, e.ToolUseID, e.Nested, e.Action, e.At}); diff != "" {
				t.Errorf("entry (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSubagentFinish(t *testing.T) {
	tests := []struct {
		name         string
		outcome      model.SubagentOutcome
		wantMismatch model.Mismatch
	}{
		{
			name:         "resolved as requested",
			outcome:      model.SubagentOutcome{ToolUseID: "t1", SubagentType: "scout", Requested: "haiku", Resolved: "claude-haiku-4-5", Status: "completed", TotalTokens: 500, InputTokens: 300, OutputTokens: 150, CacheReadTokens: 50, DurationMS: 1200},
			wantMismatch: model.MismatchNo,
		},
		{
			name:         "resolved differently",
			outcome:      model.SubagentOutcome{ToolUseID: "t2", Requested: "haiku", Resolved: "claude-sonnet-4-6", Status: "completed"},
			wantMismatch: model.MismatchYes,
		},
		{
			name:    "background launch without a resolved model",
			outcome: model.SubagentOutcome{ToolUseID: "t3", Requested: "haiku", Status: "async_launched"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			o := tt.outcome
			f.subagentFinish().Execute(t.Context(), usecase.Event{SessionID: "s1", ToolUseID: o.ToolUseID}, o, "brief")
			if len(f.ledger.entries) != 1 {
				t.Fatalf("entries = %d, want 1", len(f.ledger.entries))
			}
			e := f.ledger.entries[0]
			want := model.SubagentOutcome{
				ToolUseID: e.ToolUseID, SubagentType: e.SubagentType, Requested: e.Requested, Resolved: e.Resolved,
				Status: e.Status, TotalTokens: e.TotalTokens, InputTokens: e.InputTokens, OutputTokens: e.OutputTokens,
				CacheReadTokens: e.CacheReadTokens, DurationMS: e.DurationMS,
			}
			if diff := cmp.Diff(o, want); diff != "" {
				t.Errorf("recorded outcome (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantMismatch, e.Mismatch); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
			if e.Event != model.EventSubagentResult || e.PromptSHA != model.PromptDigest("brief") {
				t.Errorf("entry = %+v", e)
			}
		})
	}
}

func TestRecordError(t *testing.T) {
	tests := []struct {
		name       string
		detail     string
		appendErr  error
		cancelled  bool
		wantDetail string
		wantFile   string
	}{
		{name: "short detail kept whole", detail: "boom", wantDetail: "boom"},
		{name: "exactly the limit", detail: "abcde", wantDetail: "abcde"},
		{name: "long detail keeps the tail (the failing frame)", detail: "前置き…panic: 失敗", wantDetail: "c: 失敗"},
		{
			// The hook's own context is often what ran out: the write must not inherit it.
			name: "an already cancelled hook context still records", detail: "boom", cancelled: true, wantDetail: "boom",
		},
		{
			name: "a ledger that cannot be written leaves the failure file", detail: "boom", appendErr: errLedger,
			wantFile: "pre-tool-use: boom\n(ledger unavailable: ledger down)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, withSettings(func(s *model.Settings) { s.Runtime.ErrorDetailRunes = 5 }))
			f.ledger.appendErr = tt.appendErr
			ctx, cancel := context.WithCancel(t.Context())
			if tt.cancelled {
				cancel()
			}
			defer cancel()
			rec := f.recordError()
			rec.Ledger = ctxLedger{f.ledger}
			rec.RecordError(ctx, "pre-tool-use", tt.detail)
			rows := make([]string, 0, len(f.ledger.entries))
			for _, e := range f.ledger.entries {
				rows = append(rows, string(e.Event)+"|"+e.Reason+"|"+e.Detail)
			}
			var wantRows []string
			if tt.wantDetail != "" {
				wantRows = []string{string(model.EventError) + "|pre-tool-use|" + tt.wantDetail}
			}
			file := ""
			if f.failures.last != nil {
				file = f.failures.last.Message
				if !f.failures.last.At.Equal(testNow) {
					t.Errorf("failure at %v", f.failures.last.At)
				}
			}
			if diff := cmp.Diff([]any{wantRows, tt.wantFile}, []any{rows, file}, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

// ctxLedger refuses a write under a cancelled context, as SQLite does.
type ctxLedger struct{ *memLedger }

func (l ctxLedger) Append(ctx context.Context, e model.Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return l.memLedger.Append(ctx, e)
}

func TestViewOfAndTraceDetail(t *testing.T) {
	nan := func() float64 { var z float64; return z / z }()
	tests := []struct {
		name       string
		tr         service.RouteTrace
		want       usecase.TraceView
		wantDetail string // "" = the JSON of want
	}{
		{name: "empty trace", tr: service.RouteTrace{Path: []string{"tag"}}, want: usecase.TraceView{Path: []string{"tag"}}},
		{
			name: "every field, rounded",
			tr: service.RouteTrace{
				Path: []string{"rule", "model"}, Source: "model", ModelClass: model.ClassLookup,
				Rule:  model.Classification{Class: model.ClassLookup, Reasons: []string{"verb:find"}},
				Probs: map[model.Class]float64{model.ClassLookup: 0.12345}, Set: []model.Class{model.ClassLookup, model.ClassStandard},
				PSafe: 0.98765, Tau: 0.9, Unseen: 0.11111, NeighborSim: 0.55555, Posterior: 0.66666, Escalated: true,
			},
			want: usecase.TraceView{
				Path: []string{"rule", "model"}, Source: "model", RuleClass: "lookup", ModelClass: "lookup",
				Probs: map[string]float64{"lookup": 0.123}, Set: []string{"lookup", "standard"},
				PSafe: 0.988, Tau: 0.9, Unseen: 0.111, NeighborSim: 0.556, Posterior: new(0.667), Escalated: true,
				Reasons: []string{"verb:find"},
			},
		},
		{
			name: "NaN posterior is dropped",
			tr:   service.RouteTrace{Path: []string{"rule"}, Posterior: nan},
			want: usecase.TraceView{Path: []string{"rule"}},
		},
		{
			name:       "NaN elsewhere cannot be JSON: the path stands in",
			tr:         service.RouteTrace{Path: []string{"rule", "model"}, PSafe: nan},
			wantDetail: "rule>model",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantDetail == "" {
				if diff := cmp.Diff(tt.want, usecase.ViewOf(tt.tr, 3)); diff != "" {
					t.Errorf("view (-want +got):\n%s", diff)
				}
				raw, err := json.Marshal(tt.want)
				if err != nil {
					t.Fatal(err)
				}
				tt.wantDetail = string(raw)
			}
			if diff := cmp.Diff(tt.wantDetail, usecase.TraceDetail(tt.tr, 3, nil)); diff != "" {
				t.Errorf("detail (-want +got):\n%s", diff)
			}
		})
	}
}
