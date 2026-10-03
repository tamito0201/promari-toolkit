package model_test

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/domain/model"
)

func TestPromptDigest(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		// sha256("") = e3b0c44298fc1c14...
		{"empty", "", "e3b0c44298fc"},
		// sha256("abc") = ba7816bf8f01cfea...
		{"abc", "abc", "ba7816bf8f01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, model.PromptDigest(tt.text)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNewEntry(t *testing.T) {
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	cls := model.Classification{
		Class: model.ClassLookup, Confidence: 9, Margin: 4, Danger: true,
		Codex: model.CodexReview, Continuation: true, Lang: model.LangJA, Chars: 120,
	}
	dec := model.Decision{
		Action: model.ActionInject, Reason: "cheap", Target: model.TierHaiku,
		Class: model.ClassStandard, SubagentType: "scout", Requested: "opus",
	}
	tests := []struct {
		name  string
		event model.EventKind
		opts  []model.EntryOption
		want  model.Entry
	}{
		{
			name:  "no options",
			event: model.EventSessionStart,
			want:  model.Entry{At: at, Event: model.EventSessionStart},
		},
		{
			name:  "session",
			event: model.EventModelSwitch,
			opts:  []model.EntryOption{model.WithSession("s1", model.SessionModel{Model: "opus", Source: model.SourceTranscript})},
			want: model.Entry{
				At: at, Event: model.EventModelSwitch, SessionID: "s1",
				SessionModel: "opus", SessionSource: model.SourceTranscript,
			},
		},
		{
			name:  "classification",
			event: model.EventPrompt,
			opts:  []model.EntryOption{model.WithClassification(cls)},
			want: model.Entry{
				At: at, Event: model.EventPrompt, Class: model.ClassLookup, Confidence: 9, Margin: 4,
				Danger: true, Codex: model.CodexReview, Continuation: true, Lang: model.LangJA, PromptChars: 120,
			},
		},
		{
			name:  "decision fills the class when none was classified",
			event: model.EventSubagent,
			opts:  []model.EntryOption{model.WithDecision(dec)},
			want: model.Entry{
				At: at, Event: model.EventSubagent, Action: model.ActionInject, Reason: "cheap",
				Target: model.TierHaiku, SubagentType: "scout", Requested: "opus", Class: model.ClassStandard,
			},
		},
		{
			name:  "decision keeps an existing class",
			event: model.EventSubagent,
			opts:  []model.EntryOption{model.WithClassification(cls), model.WithDecision(dec)},
			want: model.Entry{
				At: at, Event: model.EventSubagent, Class: model.ClassLookup, Confidence: 9, Margin: 4,
				Danger: true, Codex: model.CodexReview, Continuation: true, Lang: model.LangJA, PromptChars: 120,
				Action: model.ActionInject, Reason: "cheap", Target: model.TierHaiku, SubagentType: "scout", Requested: "opus",
			},
		},
		{
			name:  "prompt digest only",
			event: model.EventPrompt,
			opts:  []model.EntryOption{model.WithPrompt("abc")},
			want:  model.Entry{At: at, Event: model.EventPrompt, PromptSHA: "ba7816bf8f01"},
		},
		{
			name:  "detail",
			event: model.EventModelSwitch,
			opts:  []model.EntryOption{model.WithDetail("from=opus")},
			want:  model.Entry{At: at, Event: model.EventModelSwitch, Detail: "from=opus"},
		},
		{
			name:  "tool use of a nested call",
			event: model.EventSubagent,
			opts:  []model.EntryOption{model.WithToolUse("t-1", true)},
			want:  model.Entry{At: at, Event: model.EventSubagent, ToolUseID: "t-1", Nested: true},
		},
		{
			name:  "advice under high usage",
			event: model.EventPrompt,
			opts:  []model.EntryOption{model.WithAdvice(true, true)},
			want:  model.Entry{At: at, Event: model.EventPrompt, Advised: true, PressureHigh: true},
		},
		{
			name:  "outcome",
			event: model.EventSubagentResult,
			opts: []model.EntryOption{model.WithOutcome(model.SubagentOutcome{
				ToolUseID: "t-1", SubagentType: "Explore", Requested: "haiku", Resolved: "claude-sonnet-4-6", Status: model.StatusCompleted,
				TotalTokens: 10, InputTokens: 1, OutputTokens: 2, CacheReadTokens: 3, DurationMS: 4,
			})},
			want: model.Entry{
				At: at, Event: model.EventSubagentResult, ToolUseID: "t-1", SubagentType: "Explore", Requested: "haiku",
				Resolved: "claude-sonnet-4-6", Mismatch: model.MismatchYes, Status: model.StatusCompleted,
				TotalTokens: 10, InputTokens: 1, OutputTokens: 2, CacheReadTokens: 3, DurationMS: 4,
			},
		},
		{
			name:  "failure",
			event: model.EventError,
			opts:  []model.EntryOption{model.WithFailure("PreToolUse", "boom")},
			want:  model.Entry{At: at, Event: model.EventError, Reason: "PreToolUse", Detail: "boom"},
		},
		{
			name:  "later options win",
			event: model.EventError,
			opts:  []model.EntryOption{model.WithDetail("first"), model.WithDetail("second")},
			want:  model.Entry{At: at, Event: model.EventError, Detail: "second"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, model.NewEntry(at, tt.event, tt.opts...)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSubagentOutcomeMismatch(t *testing.T) {
	tests := []struct {
		name    string
		outcome model.SubagentOutcome
		want    model.Mismatch
		known   bool
		yes     bool
	}{
		{"nothing requested", model.SubagentOutcome{Resolved: "claude-opus-5-5"}, model.MismatchUnknown, false, false},
		{"nothing resolved", model.SubagentOutcome{Requested: "opus"}, model.MismatchUnknown, false, false},
		{"both missing", model.SubagentOutcome{}, model.MismatchUnknown, false, false},
		{"same tier by alias and id", model.SubagentOutcome{Requested: "haiku", Resolved: "claude-haiku-4-5-20251001"}, model.MismatchNo, true, false},
		{"different tier", model.SubagentOutcome{Requested: "haiku", Resolved: "claude-sonnet-4-6"}, model.MismatchYes, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.outcome.Mismatch()
			if diff := cmp.Diff([]any{tt.want, tt.known, tt.yes}, []any{m, m.Known(), m.Yes()}); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSubagentStatus(t *testing.T) {
	tests := []struct {
		status                model.SubagentStatus
		succeeded, background bool
	}{
		{model.StatusCompleted, true, false},
		{model.StatusAsyncLaunched, false, true},
		{"failed", false, false},
		{"", false, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if diff := cmp.Diff([]bool{tt.succeeded, tt.background}, []bool{tt.status.Succeeded(), tt.status.Background()}); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}
