package service_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/service"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/pkg/fp"
)

func some(v float64) fp.Option[float64] { return fp.Some(v) }

// TestAdvise runs real prompts through the shipped lexicon: the advice must
// contain (and must not contain) the notes a user would see.
func TestAdvise(t *testing.T) {
	lex, _, st := fixtures(t)
	opus := model.SessionModel{Model: st.Eval.SessionModel, Source: model.SourceTranscript}
	advise := func(text string, cx model.CodexQuota, p model.Pressure) string {
		sig := lex.ExtractSignals(text)
		return service.Advise(service.AdviceInput{Classification: service.RuleClassify(sig, st.Classifier), Signals: sig, Session: opus, Settings: st, Codex: cx, Pressure: p})
	}
	available := model.CodexQuota{Available: true}
	tests := []struct {
		name, text    string
		codex         model.CodexQuota
		pressure      model.Pressure
		contains, not []string
	}{
		{name: "continuation gets nothing", text: "続けて", codex: available, not: []string{"promari"}},
		{
			name: "continuation keeps the safety note", text: "続けて。本番のパスワードも変えて", codex: available,
			contains: []string{"Safety-sensitive"}, not: []string{"/codex"},
		},
		{name: "danger", text: "本番の決済処理でタイムアウトする原因を調べて", codex: available, contains: []string{"Safety-sensitive"}},
		{name: "codex review", text: "この差分をレビューして問題点を洗い出して", codex: available, contains: []string{"/codex:review"}},
		{
			name: "codex blocked", text: "この差分をレビューして問題点を洗い出して",
			codex: model.CodexQuota{Available: false, Used: some(97)}, contains: []string{"97%"}, not: []string{"/codex:review"},
		},
		{
			name: "usage pressure", text: "この関数の単体テストを書いてほしい", codex: available,
			pressure: model.Pressure{Known: true, High: true, FiveHour: some(91)}, contains: []string{"5h 91%"},
		},
		{name: "correction", text: "いや、それは違う。もう一度やり直して", codex: available, contains: []string{"correcting"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := advise(tt.text, tt.codex, tt.pressure)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("advice %q does not contain %q", got, want)
				}
			}
			for _, bad := range tt.not {
				if strings.Contains(got, bad) {
					t.Errorf("advice %q contains %q", got, bad)
				}
			}
			if n := len([]rune(got)); n > st.Advice.MaxChars {
				t.Errorf("advice is %d runes, limit %d", n, st.Advice.MaxChars)
			}
		})
	}
}

// TestAdviseBranches drives every note with a hand-built classification, so
// each branch is pinned to the exact text it produces.
func TestAdviseBranches(t *testing.T) {
	_, _, st := fixtures(t)
	m := st.Messages
	opus := model.SessionModel{Model: "opus", Source: model.SourceTranscript}
	sonnet := model.SessionModel{Model: "sonnet", Source: model.SourceTranscript}
	haiku := model.SessionModel{Model: "haiku", Source: model.SourceTranscript}
	uncertain := model.SessionModel{Model: "sonnet", Source: model.SourceEnv}
	available := model.CodexQuota{Available: true}
	codexTask := "Codex can take this batch: `/codex:rescue --background --model " + st.Codex.Model + " --effort " + st.Codex.Effort + " <task>`; review its diff afterwards."
	advice := func(notes ...string) string { return m.Prefix + " " + strings.Join(notes, " ") }
	cls := func(c model.Class) model.Classification { return model.Classification{Class: c, Chars: 40} }

	tests := []struct {
		name     string
		class    model.Classification
		signals  service.Signals
		session  model.SessionModel
		codex    model.CodexQuota
		pressure model.Pressure
		policy   func(model.Settings) model.Settings
		want     string
	}{
		{
			name:   "advice disabled",
			class:  model.Classification{Class: model.ClassLookup, Chars: 40, Danger: true},
			policy: func(s model.Settings) model.Settings { s.Advice.Enabled = false; return s },
			want:   "",
		},
		{
			name:  "prompt shorter than the minimum",
			class: model.Classification{Class: model.ClassLookup, Chars: st.Advice.MinPromptChars - 1, Danger: true},
			want:  "",
		},
		{
			name:    "a prompt exactly the minimum length gets advice",
			class:   model.Classification{Class: model.ClassLookup, Chars: st.Advice.MinPromptChars},
			session: opus,
			want:    advice(m.Lookup),
		},
		{
			name:    "mechanical work without a codex task gets no bulk note",
			class:   cls(model.ClassMechanical),
			session: opus,
			codex:   available,
			want:    "",
		},
		{
			name:  "continuation without danger",
			class: model.Classification{Chars: 40, Continuation: true},
			want:  "",
		},
		{
			name:  "continuation with danger keeps only the safety note",
			class: model.Classification{Chars: 40, Continuation: true, Danger: true, Codex: model.CodexReview},
			codex: available,
			want:  advice(m.Danger),
		},
		{
			name:    "nothing to say",
			class:   cls(model.ClassStandard),
			session: opus,
			want:    "",
		},
		{
			name:    "danger outranks the other work notes and withholds codex",
			class:   model.Classification{Class: model.ClassLookup, Chars: 40, Danger: true, Codex: model.CodexReview},
			session: opus,
			codex:   available,
			want:    advice(m.Danger),
		},
		{
			name:    "a correction",
			class:   cls(model.ClassLookup),
			signals: service.Signals{Correction: true},
			session: opus,
			want:    advice(m.Correction),
		},
		{
			name:    "deep work on a weak session",
			class:   cls(model.ClassComplex),
			session: sonnet,
			want:    advice(service.Render(m.DeepOnWeakSession, map[string]string{"Class": "complex", "Session": "sonnet"})),
		},
		{
			name:    "deep work on an uncertain session says nothing",
			class:   cls(model.ClassArchitecture),
			session: uncertain,
			want:    "",
		},
		{
			name:    "lookup on a strong session",
			class:   cls(model.ClassLookup),
			session: opus,
			want:    advice(m.Lookup),
		},
		{
			name:    "lookup on a haiku session",
			class:   cls(model.ClassLookup),
			session: haiku,
			want:    "",
		},
		{
			name:    "bulk mechanical work with codex",
			class:   model.Classification{Class: model.ClassMechanical, Chars: 40, Codex: model.CodexTask},
			session: opus,
			codex:   available,
			want:    advice(m.BulkMechanical, codexTask),
		},
		{
			name:    "codex review",
			class:   model.Classification{Class: model.ClassStandard, Chars: 40, Codex: model.CodexReview},
			session: opus,
			codex:   available,
			want:    advice(m.CodexReview),
		},
		{
			name:    "codex with a known quota that is still available",
			class:   model.Classification{Class: model.ClassStandard, Chars: 40, Codex: model.CodexReview},
			session: opus,
			codex:   model.CodexQuota{Available: true, Used: some(40)},
			want:    advice(m.CodexReview),
		},
		{
			name:    "codex blocked by its quota",
			class:   model.Classification{Class: model.ClassStandard, Chars: 40, Codex: model.CodexTask},
			session: opus,
			codex:   model.CodexQuota{Used: some(97)},
			want:    advice("Codex usage is at 97%; keep this in Claude."),
		},
		{
			name:    "codex disabled",
			class:   model.Classification{Class: model.ClassStandard, Chars: 40, Codex: model.CodexReview},
			session: opus,
			codex:   available,
			policy:  func(s model.Settings) model.Settings { s.Codex.Enabled = false; return s },
			want:    "",
		},
		{
			name:     "pressure on both windows",
			class:    cls(model.ClassStandard),
			session:  opus,
			pressure: model.Pressure{Known: true, High: true, FiveHour: some(91), SevenDay: some(80)},
			want:     advice(service.Render(m.Pressure, map[string]string{"Usage": "5h 91% 7d 80%"})),
		},
		{
			name:     "pressure known but not high",
			class:    cls(model.ClassStandard),
			session:  opus,
			pressure: model.Pressure{Known: true, FiveHour: some(50)},
			want:     "",
		},
		{
			name:    "an empty message is dropped",
			class:   cls(model.ClassLookup),
			session: opus,
			policy:  func(s model.Settings) model.Settings { s.Messages.Lookup = ""; return s },
			want:    "",
		},
		{
			name:    "long advice is truncated",
			class:   cls(model.ClassLookup),
			session: opus,
			policy:  func(s model.Settings) model.Settings { s.Advice.MaxChars = 30; return s },
			want:    service.Truncate(advice(m.Lookup), 30),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := st
			if tt.policy != nil {
				s = tt.policy(s)
			}
			got := service.Advise(service.AdviceInput{
				Classification: tt.class, Signals: tt.signals, Session: tt.session,
				Settings: s, Pressure: tt.pressure, Codex: tt.codex,
			})
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Advise() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRender(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		data any
		want string
	}{
		{name: "fills the fields", tmpl: "{{.A}}-{{.B}}", data: map[string]string{"A": "x", "B": "y"}, want: "x-y"},
		{name: "a missing key is empty", tmpl: "[{{.Missing}}]", data: map[string]string{}, want: "[]"},
		{name: "plain text", tmpl: "no fields", data: nil, want: "no fields"},
		{name: "a parse error keeps the raw text", tmpl: "{{.A", data: nil, want: "{{.A"},
		{name: "an execution error keeps the raw text", tmpl: `{{template "absent"}}`, data: nil, want: `{{template "absent"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, service.Render(tt.tmpl, tt.data)); diff != "" {
				t.Errorf("Render() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		limit int
		want  string
	}{
		{name: "no limit", text: "abcdef", limit: 0, want: "abcdef"},
		{name: "negative limit", text: "abcdef", limit: -1, want: "abcdef"},
		{name: "shorter than the limit", text: "abc", limit: 5, want: "abc"},
		{name: "exactly the limit", text: "abcde", limit: 5, want: "abcde"},
		{name: "cut with a marker", text: "abcdef", limit: 4, want: "abc…"},
		{name: "counts runes, not bytes", text: "あいうえお", limit: 3, want: "あい…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, service.Truncate(tt.text, tt.limit)); diff != "" {
				t.Errorf("Truncate() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSessionContext(t *testing.T) {
	tests := []struct {
		name string
		msgs model.Messages
		want string
	}{
		{name: "prefix and policy", msgs: model.Messages{Prefix: "[p]", SessionPolicy: "route well"}, want: "[p] route well"},
		{name: "empty messages", msgs: model.Messages{}, want: " "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, service.SessionContext(tt.msgs)); diff != "" {
				t.Errorf("SessionContext() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestResolveSession(t *testing.T) {
	obs := func(m string, s model.SessionSource) fp.Option[model.SessionModel] {
		return fp.Some(model.SessionModel{Model: m, Source: s})
	}
	none := fp.None[model.SessionModel]()
	tests := []struct {
		name         string
		observations []fp.Option[model.SessionModel]
		want         model.SessionModel
	}{
		{
			name:         "the transcript wins",
			observations: []fp.Option[model.SessionModel]{obs("opus", model.SourceTranscript), obs("sonnet", model.SourceEnv)},
			want:         model.SessionModel{Model: "opus", Source: model.SourceTranscript},
		},
		{
			name:         "missing observations are skipped",
			observations: []fp.Option[model.SessionModel]{none, none, obs("sonnet", model.SourceEnv)},
			want:         model.SessionModel{Model: "sonnet", Source: model.SourceEnv},
		},
		{
			name:         "an observation without a model is skipped",
			observations: []fp.Option[model.SessionModel]{obs("", model.SourceSessionState), obs("haiku", model.SourceSettings)},
			want:         model.SessionModel{Model: "haiku", Source: model.SourceSettings},
		},
		{
			name:         "nothing observed",
			observations: []fp.Option[model.SessionModel]{none, obs("", model.SourceTranscript)},
			want:         model.SessionModel{Source: model.SourceUnknown},
		},
		{
			name: "no observations at all",
			want: model.SessionModel{Source: model.SourceUnknown},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, service.ResolveSession(tt.observations...)); diff != "" {
				t.Errorf("ResolveSession() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
