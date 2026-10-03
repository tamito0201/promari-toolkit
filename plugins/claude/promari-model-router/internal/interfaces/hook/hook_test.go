package hook_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/di"
	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/repository"
	"promari-model-router/internal/domain/service"
	"promari-model-router/internal/interfaces/hook"
)

// isolate points every piece of state (ledger, artifact, user and project
// config) at a fresh temporary directory and writes the project config.
func isolate(t *testing.T, projectTOML string) {
	t.Helper()
	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	for k, v := range map[string]string{
		"HOME": filepath.Join(tmp, "home"), "CLAUDE_PLUGIN_DATA": filepath.Join(tmp, "data"), "CLAUDE_PROJECT_DIR": project,
		"ANTHROPIC_MODEL": "", "CLAUDE_CODE_SUBAGENT_MODEL_FORCE": "",
	} {
		t.Setenv(k, v)
	}
	if projectTOML == "" {
		return
	}
	if err := os.MkdirAll(filepath.Join(project, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".claude", "promari-model-router.toml"), []byte(projectTOML), 0o600); err != nil {
		t.Fatal(err)
	}
}

// failWriter fails every write.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

type step struct {
	event string
	stdin string
}

func TestRun(t *testing.T) {
	agent := func(id, input string) string {
		return `{"session_id":"s","tool_name":"Agent","tool_use_id":"` + id + `","tool_input":` + input + `}`
	}
	lookup := `{"prompt":"UserService がどこで定義されているか探して","description":"find","subagent_type":"general-purpose","run_in_background":false}`
	opusSession := step{"SessionStart", `{"session_id":"s","source":"startup","model":{"id":"claude-opus-5-5"}}`}
	tests := []struct {
		name     string
		toml     string            // project configuration
		userTOML string            // ~/.claude configuration
		env      map[string]string // extra environment
		setup    []step            // events handled before the one under test
		event    string
		stdin    io.Reader
		stdout   io.Writer // nil = a buffer that is checked against want
		want     func(st model.Settings) map[string]any
		wantErrs []string             // "where: detail prefix" passed to onError
		wantLog  []string             // ledger event kinds, oldest first
		handlers func() hook.Handlers // nil = the container's handlers
	}{
		{
			name:  "SessionStart injects the routing policy (model as a string)",
			event: "SessionStart",
			stdin: strings.NewReader(`{"session_id":"s","source":"startup","model":"claude-opus-5-5"}`),
			want: func(st model.Settings) map[string]any {
				return map[string]any{"hookEventName": "SessionStart", "additionalContext": service.SessionContext(st.Messages)}
			},
			wantLog: []string{"session_start"},
		},
		{
			name:  "SessionStart accepts the model as an object",
			event: "SessionStart",
			stdin: strings.NewReader(opusSession.stdin),
			want: func(st model.Settings) map[string]any {
				return map[string]any{"hookEventName": "SessionStart", "additionalContext": service.SessionContext(st.Messages)}
			},
			wantLog: []string{"session_start"},
		},
		{
			name:  "SessionStart warns when CLAUDE_CODE_SUBAGENT_MODEL_FORCE disables routing",
			env:   map[string]string{model.ForceEnv: "1"},
			event: "SessionStart",
			stdin: strings.NewReader(`{"session_id":"s"}`),
			want: func(st model.Settings) map[string]any {
				return map[string]any{
					"hookEventName":     "SessionStart",
					"additionalContext": service.SessionContext(st.Messages) + "\n" + st.Messages.Prefix + " " + st.Messages.ForceDisabled,
				}
			},
			wantLog: []string{"session_start"},
		},
		{
			// A rejected project file was invisible during a session.
			name:  "SessionStart warns about a rejected configuration file",
			toml:  "[routing]\nmdoe = \"off\"\n",
			event: "SessionStart",
			stdin: strings.NewReader(`{"session_id":"s"}`),
			want: func(st model.Settings) map[string]any {
				problem := filepath.Join(os.Getenv("CLAUDE_PROJECT_DIR"), ".claude", "promari-model-router.toml") +
					": key(s) a project file may not set (misspelt, or reserved for ~/.claude/promari-model-router.toml): routing.mdoe"
				return map[string]any{
					"hookEventName": "SessionStart",
					"additionalContext": service.SessionContext(st.Messages) + "\n" + st.Messages.Prefix + " " +
						service.Render(st.Messages.ConfigRejected, map[string]string{"Problem": problem}),
				}
			},
			wantLog: []string{"session_start", "error"},
		},
		{
			name:    "SessionStart prints nothing when routing is off",
			toml:    "[routing]\nmode = \"off\"\n",
			event:   "SessionStart",
			stdin:   strings.NewReader(`{"session_id":"s","source":"startup"}`),
			wantLog: []string{"session_start"},
		},
		{
			name:    "PostModelSwitch records the switch and prints nothing",
			event:   "PostModelSwitch",
			stdin:   strings.NewReader(`{"session_id":"s","from_model":"claude-opus-5-5","to_model":"claude-sonnet-4-6"}`),
			wantLog: []string{"model_switch"},
		},
		{
			name:  "UserPromptSubmit advises on safety-sensitive requests",
			event: "UserPromptSubmit",
			stdin: strings.NewReader(`{"session_id":"s","prompt":"本番の決済処理でタイムアウトする原因を調べて"}`),
			want: func(st model.Settings) map[string]any {
				return map[string]any{"hookEventName": "UserPromptSubmit", "additionalContext": st.Messages.Prefix + " " + st.Messages.Danger}
			},
			wantLog: []string{"prompt"},
		},
		{
			name:    "UserPromptSubmit prints nothing for an ordinary request",
			event:   "UserPromptSubmit",
			stdin:   strings.NewReader(`{"session_id":"s","prompt":"hello"}`),
			wantLog: []string{"prompt"},
		},
		{
			name:  "PreToolUse rewrites the model and keeps every other field",
			event: "PreToolUse",
			stdin: strings.NewReader(agent("t-1", lookup)),
			want: func(model.Settings) map[string]any {
				return map[string]any{"hookEventName": "PreToolUse", "updatedInput": map[string]any{
					"prompt": "UserService がどこで定義されているか探して", "description": "find",
					"subagent_type": "general-purpose", "run_in_background": false, "model": "haiku",
				}}
			},
			wantLog: []string{"subagent"},
		},
		{
			// The ledger holds an inject the hook cannot apply: it used to say
			// nothing, and the ledger read as a rewrite that never happened.
			name:     "PreToolUse (Task) with an input that only decodes as a struct prints nothing and says why",
			event:    "PreToolUse",
			stdin:    strings.NewReader(`{"session_id":"s","tool_name":"Task","tool_use_id":"t-1","tool_input":{"prompt":"UserService がどこで定義されているか探して","big":1e999}}`),
			wantLog:  []string{"subagent", "error"},
			wantErrs: []string{"PreToolUse: rewrite tool_input: not an object"},
		},
		{
			name:  "PreToolUse asks before running above the session model",
			setup: []step{opusSession},
			event: "PreToolUse",
			stdin: strings.NewReader(agent("t-4", `{"prompt":"x","subagent_type":"general-purpose","model":"fable"}`)),
			want: func(st model.Settings) map[string]any {
				return map[string]any{
					"hookEventName": "PreToolUse", "permissionDecision": "ask",
					"permissionDecisionReason": service.Render(st.Messages.AskUpgrade, map[string]string{"Target": "fable"}),
				}
			},
			wantLog: []string{"session_start", "subagent"},
		},
		{
			name:    "PreToolUse keeps safety-sensitive work (no output)",
			event:   "PreToolUse",
			stdin:   strings.NewReader(agent("t-3", `{"prompt":"[route: mechanical] 本番 DB の users テーブルの列名をリネームして","subagent_type":"general-purpose"}`)),
			wantLog: []string{"subagent"},
		},
		{
			// An undecodable tool_input used to be routed as an empty brief.
			name:     "PreToolUse with a tool_input it cannot read records the error and changes nothing",
			event:    "PreToolUse",
			stdin:    strings.NewReader(`{"session_id":"s","tool_name":"Agent","tool_use_id":"t-9","tool_input":{"prompt":42}}`),
			wantErrs: []string{"PreToolUse: decode tool_input:"},
			wantLog:  []string{"error"},
		},
		{
			name:     "PostToolUse with a tool_input it cannot read records the error",
			event:    "PostToolUse",
			stdin:    strings.NewReader(`{"session_id":"s","tool_name":"Agent","tool_use_id":"t-9","tool_input":{"model":1}}`),
			wantErrs: []string{"PostToolUse: decode tool_input:"},
			wantLog:  []string{"error"},
		},
		{
			name:     "PostToolUse with a tool_response it cannot read records the error",
			event:    "PostToolUse",
			stdin:    strings.NewReader(`{"session_id":"s","tool_name":"Task","tool_use_id":"t-9","tool_input":{},"tool_response":{"totalTokens":"many"}}`),
			wantErrs: []string{"PostToolUse: decode tool_response:"},
			wantLog:  []string{"error"},
		},
		{
			name:    "PostToolUse without tool_input or tool_response records what it has",
			event:   "PostToolUse",
			stdin:   strings.NewReader(`{"session_id":"s","tool_name":"Agent","tool_use_id":"t-9"}`),
			wantLog: []string{"subagent_result"},
		},
		{
			name:  "PreToolUse ignores other tools",
			event: "PreToolUse",
			stdin: strings.NewReader(`{"session_id":"s","tool_name":"Bash","tool_input":{"command":"ls"}}`),
		},
		{
			name:  "PostToolUse records the outcome",
			event: "PostToolUse",
			stdin: strings.NewReader(agent("t-1", lookup)[:len(agent("t-1", lookup))-1] +
				`,"tool_response":{"resolvedModel":"claude-haiku-4-5-20251001","status":"completed","totalTokens":1200,"usage":{"input_tokens":10}}}`),
			wantLog: []string{"subagent_result"},
		},
		{
			name:  "PostToolUse ignores other tools",
			event: "PostToolUse",
			stdin: strings.NewReader(`{"session_id":"s","tool_name":"Read"}`),
		},
		{
			name:  "empty stdin and an unknown event do nothing",
			event: "Notification",
			stdin: strings.NewReader(""),
		},
		{
			name:     "malformed JSON fails open",
			event:    "PreToolUse",
			stdin:    strings.NewReader("{not json"),
			wantErrs: []string{"PreToolUse: decode event:"},
			wantLog:  []string{"error"},
		},
		{
			name:     "stdin beyond the limit is cut and fails open",
			userTOML: "[runtime]\nstdin_limit_bytes = 16\n",
			event:    "UserPromptSubmit",
			stdin:    strings.NewReader(`{"session_id":"s","prompt":"` + strings.Repeat("a", 64) + `"}`),
			wantErrs: []string{"UserPromptSubmit: decode event:"},
			wantLog:  []string{"error"},
		},
		{
			name:     "a failing stdin fails open",
			event:    "SessionStart",
			stdin:    iotest.ErrReader(errors.New("broken pipe")),
			wantErrs: []string{"SessionStart: broken pipe"},
			wantLog:  []string{"error"},
		},
		{
			name:     "a failing stdout is recorded",
			event:    "SessionStart",
			stdin:    strings.NewReader(`{"session_id":"s"}`),
			stdout:   failWriter{},
			wantErrs: []string{"SessionStart: encode output: disk full"},
			wantLog:  []string{"session_start", "error"},
		},
		{
			name:     "a panic is recovered",
			event:    "SessionStart",
			stdin:    strings.NewReader(`{}`),
			handlers: func() hook.Handlers { return hook.Handlers{} }, // nil ports
			wantErrs: []string{"SessionStart: panic:"},
			wantLog:  []string{"error"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t, tt.toml)
			if tt.userTOML != "" {
				home := os.Getenv("HOME")
				if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(home, ".claude", "promari-model-router.toml"), []byte(tt.userTOML), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			c := di.New()
			t.Cleanup(func() { _ = c.Close() })
			h := built(c.Hook())(t)
			rec := built(c.RecordError())(t)
			var errs []string
			onError := func(where, detail string) {
				errs = append(errs, where+": "+detail)
				rec.RecordError(t.Context(), where, detail)
			}
			for _, s := range tt.setup {
				h.Run(t.Context(), s.event, strings.NewReader(s.stdin), io.Discard, onError)
			}
			if tt.handlers != nil {
				h = tt.handlers()
			}
			var out bytes.Buffer
			w := tt.stdout
			if w == nil {
				w = &out
			}
			h.Run(t.Context(), tt.event, tt.stdin, w, onError)

			var want any
			if tt.want != nil {
				want = map[string]any{"hookSpecificOutput": tt.want(c.Settings())}
			}
			var got any
			if out.Len() > 0 {
				if err := json.Unmarshal(out.Bytes(), &got); err != nil {
					t.Fatalf("stdout is not JSON: %v\n%s", err, out.String())
				}
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("stdout (-want +got):\n%s", diff)
			}
			if len(errs) != len(tt.wantErrs) {
				t.Fatalf("errors = %q, want prefixes %q", errs, tt.wantErrs)
			}
			for i, p := range tt.wantErrs {
				if !strings.HasPrefix(errs[i], p) {
					t.Errorf("error %d = %q, want prefix %q", i, errs[i], p)
				}
			}
			var kinds []string
			for e, err := range built(di.Resolve[repository.LedgerRepository](c))(t).Since(t.Context(), time.Time{}) {
				if err != nil {
					t.Fatal(err)
				}
				kinds = append(kinds, string(e.Event))
			}
			if diff := cmp.Diff(tt.wantLog, kinds); diff != "" {
				t.Errorf("ledger (-want +got):\n%s", diff)
			}
		})
	}
}

// built returns what a scope built, failing the test when it could not:
// built(scope.Hook())(t).
func built[T any](v T, err error) func(t *testing.T) T {
	return func(t *testing.T) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}
