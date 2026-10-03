package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"iter"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/di"
	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/repository"
	"promari-model-router/internal/infrastructure/settings"
	"promari-model-router/internal/interfaces/cli"
	"promari-model-router/internal/interfaces/hook"
)

// fakeLedger replaces the ledger where a failure has to be forced.
type fakeLedger struct {
	repository.LedgerRepository
	sinceErr  error
	checked   int
	broken    uint
	verifyErr error
}

func (l fakeLedger) Since(context.Context, time.Time) iter.Seq2[model.Entry, error] {
	return func(yield func(model.Entry, error) bool) { yield(model.Entry{}, l.sinceErr) }
}

func (l fakeLedger) Verify(context.Context) (int, uint, error) {
	return l.checked, l.broken, l.verifyErr
}

// extraAgent adds an agent that has no agents/<name>.md.
type extraAgent struct{ repository.ConfigProvider }

func (c extraAgent) Tiers() model.TierTable {
	t := c.ConfigProvider.Tiers()
	agents := map[string]model.AgentSpec{"nope": {Model: "haiku"}}
	maps.Copy(agents, t.Agents)
	t.Agents = agents
	return t
}

// isolate points all state at a fresh temporary directory; it returns that directory.
func isolate(t *testing.T, projectTOML string) string {
	t.Helper()
	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	for k, v := range map[string]string{
		"HOME": filepath.Join(tmp, "home"), "CLAUDE_PLUGIN_DATA": filepath.Join(tmp, "data"), "CLAUDE_PROJECT_DIR": project,
		"ANTHROPIC_MODEL": "", "CLAUDE_CODE_SUBAGENT_MODEL_FORCE": "", "CLAUDE_CODE_SUBAGENT_MODEL": "", "CLAUDE_CODE_EFFORT_LEVEL": "",
	} {
		t.Setenv(k, v)
	}
	if projectTOML != "" {
		write(t, filepath.Join(project, ".claude", "promari-model-router.toml"), projectTOML)
	}
	return tmp
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// newRoot builds the command tree over scopes with the given replacements.
func newRoot(inject []di.Option) *cobra.Command {
	return cli.New(func() cli.Scope { return di.New(inject...) })
}

// execute runs one pmr command line and returns stdout. With cancelled set,
// the command runs under an already cancelled context.
func execute(t *testing.T, inject []di.Option, cancelled bool, stdin string, args ...string) (string, error) {
	t.Helper()
	root := newRoot(inject)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if cancelled {
		cancel()
	}
	var out, errOut bytes.Buffer
	root.SetArgs(args)
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&out)
	root.SetErr(&errOut)
	err := root.ExecuteContext(ctx)
	return out.String(), err
}

// ledgerKinds lists the ledger's event kinds, oldest first.
func ledgerKinds(t *testing.T) []string {
	t.Helper()
	c := di.New()
	defer func() { _ = c.Close() }()
	ledger, err := di.Resolve[repository.LedgerRepository](c)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for e, err := range ledger.Since(t.Context(), time.Time{}) {
		if err != nil {
			t.Fatal(err)
		}
		kinds = append(kinds, string(e.Event)+":"+e.Reason)
	}
	return kinds
}

// pick follows a dotted path through decoded JSON (numbers index arrays).
func pick(v any, path string) any {
	for key := range strings.SplitSeq(path, ".") {
		switch x := v.(type) {
		case map[string]any:
			v = x[key]
		case []any:
			i, err := strconv.Atoi(key)
			if err != nil || i >= len(x) {
				return nil
			}
			v = x[i]
		default:
			return nil
		}
	}
	return v
}

type step struct {
	args  []string
	stdin string
}

func TestCommands(t *testing.T) {
	const lookup = `{"session_id":"s","tool_name":"Agent","tool_use_id":"t-1","tool_input":{"prompt":"UserService がどこで定義されているか探して","subagent_type":"general-purpose"}}`
	seed := []step{
		{args: []string{"hook", "SessionStart"}, stdin: `{"session_id":"s","source":"startup","model":"claude-opus-5-5"}`},
		{args: []string{"hook", "UserPromptSubmit"}, stdin: `{"session_id":"s","prompt":"UserService がどこで定義されているか探して"}`},
		{args: []string{"hook", "PreToolUse"}, stdin: lookup},
		{args: []string{"hook", "PostToolUse"}, stdin: `{"session_id":"s","tool_name":"Agent","tool_use_id":"t-1","tool_input":{"prompt":"UserService がどこで定義されているか探して","model":"haiku"},"tool_response":{"resolvedModel":"claude-haiku-4-5-20251001","status":"completed","totalTokens":1200,"usage":{"input_tokens":10,"output_tokens":90,"cache_read_input_tokens":1100}}}`},
	}
	failWiring := di.Replace(func() (hook.Handlers, error) { return hook.Handlers{}, errors.New("wiring failed") })
	tests := []struct {
		name     string
		toml     string            // project configuration
		env      map[string]string // extra environment
		files    map[string]string // written under $TMP
		setup    []step            // commands run first (seed the ledger)
		inject   []di.Option       // replacements in the scopes
		cancel   bool              // run with a cancelled context
		stdio    bool              // os.Stdin is a closed pipe, os.Stdout is discarded
		args     []string          // "$TMP" is replaced by the temporary directory
		stdin    string
		wantOut  []string       // substrings of stdout ("$TMP" replaced)
		golden   string         // the whole stdout, written by hand ("$TMP" replaced)
		empty    bool           // stdout must be empty
		wantJSON map[string]any // dotted path -> value in the JSON stdout
		wantErr  string         // substring of the error; "" = success
		check    func(t *testing.T, tmp string)
	}{
		// ------------------------------------------------------------ root
		{name: "version", args: []string{"--version"}, wantOut: []string{"pmr version " + cli.Version}},
		{name: "unknown command", args: []string{"nope"}, wantErr: `unknown command "nope"`},

		// ------------------------------------------------------------ hook
		{
			name: "hook prints the hook output", args: []string{"hook", "SessionStart"}, stdin: `{"session_id":"s"}`,
			wantJSON: map[string]any{"hookSpecificOutput.hookEventName": "SessionStart"},
		},
		{name: "hook needs exactly one event name", args: []string{"hook"}, wantErr: "accepts 1 arg(s), received 0"},
		{
			name: "hook records a wiring failure and fails open", inject: []di.Option{failWiring},
			args: []string{"hook", "SessionStart"}, stdin: `{}`, empty: true,
			check: func(t *testing.T, _ string) {
				t.Helper()
				if diff := cmp.Diff([]string{"error:SessionStart"}, ledgerKinds(t)); diff != "" {
					t.Errorf("ledger (-want +got):\n%s", diff)
				}
			},
		},
		{
			// It used to leave no trace at all.
			name: "hook leaves the failure file when not even the ledger can be wired",
			inject: []di.Option{
				failWiring,
				di.Replace(func() (usecase.RecordErrorUseCase, error) {
					return usecase.RecordErrorUseCase{}, errors.New("no ledger")
				}),
			},
			args: []string{"hook", "SessionStart"}, stdin: `{}`, empty: true,
			check: func(t *testing.T, tmp string) {
				t.Helper()
				if kinds := ledgerKinds(t); len(kinds) != 0 {
					t.Errorf("ledger = %v, want empty", kinds)
				}
				raw, err := os.ReadFile(filepath.Join(tmp, "data", "last_error"))
				if err != nil || !strings.Contains(string(raw), "\nSessionStart: wire the hook: wiring failed\n") {
					t.Errorf("last_error = %q, %v", raw, err)
				}
			},
		},
		{
			name: "hook stays silent when not even the failure file can be written",
			inject: []di.Option{
				failWiring,
				di.Replace(func() (usecase.RecordErrorUseCase, error) {
					return usecase.RecordErrorUseCase{}, errors.New("no ledger")
				}),
				di.Replace(func() (repository.FailureRecorder, error) { return nil, errors.New("no data dir") }),
			},
			args: []string{"hook", "SessionStart"}, stdin: `{}`, empty: true,
			check: func(t *testing.T, tmp string) {
				t.Helper()
				if _, err := os.Stat(filepath.Join(tmp, "data", "last_error")); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("last_error exists: %v", err)
				}
			},
		},

		// ------------------------------------------------------------ explain
		{
			// 14 characters; the strong cue "テストを書" gives standard 3 and
			// nothing else scores, so the margin is 3 too.
			name: "explain prints every stage", args: []string{"explain", "この関数の単体テストを書いて"},
			golden: "class      : standard  confidence=3 margin=3\n" +
				"flags      : danger=false codex=- continuation=false lang=ja chars=14\n" +
				"scores     : map[standard:3]\n" +
				"reasons    : standard:テストを書\n" +
				"stages     : guard > signals > tag > cascade > ood > floor > risk > ledger > gate > finish\n" +
				"subagent   : inject (rule:standard) -> sonnet  [type=general-purpose, session=claude-opus-5-5]\n" +
				"advice     : -\n" +
				"artifact   : embedded (ready)\n",
		},
		{
			// A broken local artifact used to fall back without a word.
			name: "explain says when it did not route with the local artifact",
			inject: []di.Option{di.Replace(func() repository.ArtifactStore {
				return brokenArtifact{err: errors.New("local artifact is broken: artifact.json: no classes")}
			})},
			args:    []string{"explain", "この関数の単体テストを書いて"},
			wantOut: []string{"artifact   : - (rules only); the local artifact was not used: local artifact is broken: artifact.json: no classes\n"},
		},
		{
			name: "explain reads the prompt from stdin", args: []string{"explain", "--json"}, stdin: "UserService がどこで定義されているか探して",
			wantJSON: map[string]any{"class": "lookup", "lang": "mixed", "subagent_decision.target": "haiku"},
		},
		{
			name: "explain shows the learned stage when it runs", files: map[string]string{"home/.claude/promari-model-router.toml": "[model]\nallow_embedded = true\n"},
			args: []string{"explain", "この関数の単体テストを書いて"}, wantOut: []string{"model      : standard probs="},
		},
		{
			name: "classify is an alias; abstention and flags are shown", args: []string{"classify", "--subagent-type", "promari-model-router:scout", "--session-model", "claude-sonnet-4-6", "hello"},
			wantOut: []string{"class      : (abstain)", "codex=-", "type=promari-model-router:scout, session=claude-sonnet-4-6"},
		},

		// ------------------------------------------------------------ report
		// A use case that cannot be built is the command's error, not a panic
		// (commands used to resolve their use case with a call that panics).
		{
			name:   "report says why its use case cannot be built",
			inject: []di.Option{di.Replace(func() (usecase.ReportUseCase, error) { return usecase.ReportUseCase{}, errors.New("no ledger") })},
			args:   []string{"report"}, wantErr: "no ledger",
		},
		{
			name:   "serve says why its report cannot be built",
			inject: []di.Option{di.Replace(func() (usecase.ReportUseCase, error) { return usecase.ReportUseCase{}, errors.New("no ledger") })},
			args:   []string{"serve"}, wantErr: "no ledger",
		},
		{
			name:   "serve says why its explanations cannot be built",
			inject: []di.Option{di.Replace(func() (usecase.ExplainUseCase, error) { return usecase.ExplainUseCase{}, errors.New("no artifact") })},
			args:   []string{"serve"}, wantErr: "no artifact",
		},
		{
			name:   "mcp says why its report cannot be built",
			inject: []di.Option{di.Replace(func() (usecase.ReportUseCase, error) { return usecase.ReportUseCase{}, errors.New("no ledger") })},
			args:   []string{"mcp"}, wantErr: "no ledger",
		},
		{
			name:   "mcp says why its explanations cannot be built",
			inject: []di.Option{di.Replace(func() (usecase.ExplainUseCase, error) { return usecase.ExplainUseCase{}, errors.New("no artifact") })},
			args:   []string{"mcp"}, wantErr: "no artifact",
		},
		{
			name: "report on an empty ledger", args: []string{"report"},
			wantOut: []string{"last 7 day(s), 0 ledger entries", "classified n/a (0 events)", "list-price estimate", "): none", "No data. Is the plugin enabled?"},
		},
		{
			name: "report with data", setup: seed, args: []string{"report", "--days", "1"},
			wantOut: []string{"last 1 day(s), 4 ledger entries", "classified [mixed]: 100.0% (1/1)", "Subagent calls: 1", "haiku $"},
		},
		{
			// By hand: 10 input, 90 output and 1,100 cache-read tokens at the
			// haiku list price ($1, $5 and $0.10 per million) is $0.00057.
			name: "report text in full", setup: seed, args: []string{"report", "--days", "1"},
			golden: "promari-model-router report — last 1 day(s), 4 ledger entries\n\n" +
				"Prompts: 1  classified 100.0% (1/1)  advised 1  danger 0\n" +
				"  classified [mixed]: 100.0% (1/1)\n\n" +
				"Subagent calls: 1\n" +
				"  actions: map[inject:1]\n" +
				"  reasons: map[rule:lookup:1]\n" +
				"  injected: map[haiku:1]  shadow: map[]\n\n" +
				"Subagent results: 1 (joined to a decision: 1)\n" +
				"  calls by resolved tier : map[haiku:1]\n" +
				"  tokens by resolved tier: map[haiku:1200]\n" +
				"  tokens run below the session tier: 1200\n" +
				"  requested != resolved: 0   background (no usage data): 0\n" +
				"  list-price estimate (USD, prices as of 2026-09-25): haiku $0.0006\n\n" +
				"Hook errors: 0\n\n" +
				"Note: token totals are what subagents used, not a saving. Compare against a baseline period (mode: shadow) before claiming one.\n",
		},
		{
			name: "report as JSON", setup: seed, args: []string{"report", "--json"},
			wantJSON: map[string]any{"entries": 4.0, "subagents.calls": 1.0, "results.joined_to_decision": 1.0},
		},
		{name: "report refuses a negative window", args: []string{"report", "--days", "-1"}, wantErr: "days must be a positive number"},
		{
			name: "report fails when the ledger fails",
			inject: []di.Option{
				di.Replace(func() repository.LedgerRepository { return fakeLedger{sinceErr: errors.New("ledger unreadable")} }),
			},
			args:    []string{"report"},
			wantErr: "ledger unreadable",
		},

		// ------------------------------------------------------------ eval
		{name: "eval passes on the embedded set", args: []string{"eval"}, wantOut: []string{"eval: exact ", "  sufficient tier: router", "  relative cost  :", "collapse", "✅ eval complete"}},
		{name: "eval as JSON", args: []string{"eval", "--json"}, wantJSON: map[string]any{"passed": true, "danger_leaks": 0.0, "harmful_downgrades": 0.0}},
		{
			name: "eval with explicit gates and stages", args: []string{"eval", "--rules-only", "--allow-embedded", "--min-accuracy", "0", "--max-harmful", "5", "--session-model", "claude-opus-5-5"},
			wantOut: []string{"(gate 0.00)", "(gate 5)", "✅ eval complete"},
		},
		{
			name:    "eval lists misses and fails the gate",
			files:   map[string]string{"cases.jsonl": `{"lang":"en","expect":"architecture","text":"list the files in this directory"}` + "\n"},
			args:    []string{"eval", "-v", "--file", "$TMP/cases.jsonl"},
			wantOut: []string{"eval: exact 0/1", "  en: ", "  miss: want=architecture", "❌ eval failed"},
			wantErr: "eval gate failed",
		},
		{
			// By hand: the lookup brief goes to haiku (exact); the English
			// brief abstains, so the opus session keeps it (a miss, not a
			// harmful one). Relative cost: (1 + 5) / 2 = 3 for the router.
			name: "eval text in full",
			files: map[string]string{"cases.jsonl": `{"lang":"ja","expect":"lookup","text":"UserService がどこで定義されているか探して"}` + "\n" +
				`{"lang":"en","expect":"architecture","text":"list the files in this directory"}` + "\n"},
			args: []string{"eval", "-v", "--file", "$TMP/cases.jsonl", "--min-accuracy", "0", "--max-harmful", "5"},
			golden: "eval: exact 1/2 = 0.500 (gate 0.00)  injected 1  harmful downgrades 0 (gate 5)  danger leaks 0  [session claude-opus-5-5]\n" +
				"  en: 0.0% (0/1)\n" +
				"  ja: 100.0% (1/1)\n" +
				"  sufficient tier: router 1.00  always-cheap 0.50  static 1.00  oracle 1.00\n" +
				"  relative cost  : router 3.00  always-strong 5.00  static 3.00  oracle 3.00\n" +
				"  collapse (share of the most common tier): 0.50\n" +
				"  artifact: embedded (ready)\n" +
				"  miss: want=architecture got=abstain              list the files in this directory\n" +
				"✅ eval complete\n",
		},
		{name: "eval reports a missing file", args: []string{"eval", "--file", "$TMP/missing.jsonl"}, wantErr: "no such file"},
		{
			name: "eval of no cases is n/a and fails", files: map[string]string{"empty.jsonl": ""},
			args:    []string{"eval", "--file", "$TMP/empty.jsonl", "--min-accuracy", "0"},
			wantOut: []string{"eval: exact 0/0 = n/a", "❌ eval failed: no cases"}, wantErr: "eval gate failed",
		},

		// ------------------------------------------------------------ train
		{
			name: "train writes the artifact", args: []string{"train", "--output", "$TMP/artifact.json"},
			wantOut: []string{"✅ train complete: $TMP/artifact.json", "tau by length bucket:", "gate by class:"},
			check: func(t *testing.T, tmp string) {
				t.Helper()
				if _, err := os.Stat(filepath.Join(tmp, "artifact.json")); err != nil {
					t.Error(err)
				}
			},
		},
		{
			name:     "train as JSON from a file and the ledger",
			files:    map[string]string{"cases.jsonl": `{"lang":"en","expect":"lookup","text":"list the files"}` + "\n"},
			setup:    seed,
			args:     []string{"train", "--json", "--file", "$TMP/cases.jsonl", "--ledger-days", "1", "--output", "$TMP/a.json"},
			wantJSON: map[string]any{"samples": 1.0},
		},
		{
			// One labelled prompt is fewer than the classes: the artifact
			// keeps the neutral calibration and no thresholds or gates.
			name:  "train on too few prompts writes a rules-only artifact",
			files: map[string]string{"cases.jsonl": `{"lang":"en","expect":"lookup","text":"list the files"}` + "\n"},
			args:  []string{"train", "--file", "$TMP/cases.jsonl", "--output", "$TMP/a.json"},
			golden: "✅ train complete: $TMP/a.json\n" +
				"  1 labelled prompts from 1 file(s)\n" +
				"  tau by length bucket: map[]\n" +
				"  gate by class: map[]\n",
			check: func(t *testing.T, tmp string) {
				t.Helper()
				raw, err := os.ReadFile(filepath.Join(tmp, "a.json"))
				if err != nil {
					t.Fatal(err)
				}
				var a map[string]any
				if err := json.Unmarshal(raw, &a); err != nil {
					t.Fatal(err)
				}
				want := map[string]any{"samples": 1.0, "origin": "local", "temperature": 1.0, "conformal_q": 1.0, "bias": nil, "tau": map[string]any{}}
				got := map[string]any{}
				for k := range want {
					got[k] = a[k]
				}
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("artifact (-want +got):\n%s", diff)
				}
			},
		},
		{name: "train reports a missing file", args: []string{"train", "--file", "$TMP/missing.jsonl"}, wantErr: "no such file"},

		// ------------------------------------------------------------ doctor
		{name: "doctor passes on a clean setup", args: []string{"doctor"}, wantOut: []string{"✅", "mode", "ledger chain", "⚠️"}},
		{
			name: "doctor fails when routing is forced off", env: map[string]string{"CLAUDE_CODE_SUBAGENT_MODEL_FORCE": "1"},
			args: []string{"doctor", "--json"}, wantOut: []string{`"name": "CLAUDE_CODE_SUBAGENT_MODEL_FORCE"`, `"status": "fail"`, `"hint": "unset it (or set it to 0)`},
			wantErr: "doctor found failures",
		},
		{
			name: "doctor cuts long details to [display].error_chars", files: map[string]string{"home/.claude/promari-model-router.toml": "[display]\nerror_chars = 4\n"},
			args: []string{"doctor"}, wantOut: []string{"mode                    enfo…\n"},
		},
		{
			name: "doctor prints the hint after the detail", args: []string{"doctor"},
			wantOut: []string{"rules only (run `pmr train --file <your labelled prompts>` to route with a learned artifact)"},
		},
		{
			// The doctor used to panic when the container could not be built.
			name:  "doctor reports a container it cannot build instead of panicking",
			files: map[string]string{"blocker": ""}, env: map[string]string{"CLAUDE_PLUGIN_DATA": "$TMP/blocker/data"},
			args: []string{"doctor"}, wantOut: []string{"❌  wiring", "(check that the data directory can be created and written)"},
			wantErr: "doctor found failures",
		},

		// ------------------------------------------------------------ lint
		{name: "lint passes", args: []string{"lint"}, wantOut: []string{"✅ lint complete: 4 agents match data/tiers.toml"}},
		{
			name: "lint lists disagreeing agents",
			inject: []di.Option{
				di.Replace(func() repository.ConfigProvider { return extraAgent{settings.NewProvider()} }),
			},
			args: []string{"lint"}, wantOut: []string{"❌ agents/nope.md is missing"}, wantErr: "1 agent(s) disagree with data/tiers.toml",
		},

		// ------------------------------------------------------------ query
		{name: "query without a ledger", args: []string{"query", "SELECT 1"}, wantErr: "open ledger read-only"},
		{name: "query prints the schema", setup: seed, args: []string{"query", "--schema"}, wantOut: []string{"CREATE TABLE", "ledger", "sessions"}},
		{name: "query prints the schema as JSON", setup: seed, args: []string{"query", "--schema", "--json"}, wantOut: []string{`"CREATE TABLE`}},
		{
			name: "query prints a table", setup: seed, args: []string{"query", "SELECT", "action,", "target", "FROM", "ledger", "WHERE", "event", "=", "'subagent'"},
			wantOut: []string{"action  target\n", "inject  haiku\n"},
		},
		{
			name: "query as JSON", setup: seed, args: []string{"query", "--json", "--limit", "1", "SELECT event FROM ledger ORDER BY id"},
			wantJSON: map[string]any{"0.event": "session_start", "1": nil},
		},
		{name: "query refuses writes", setup: seed, args: []string{"query", "DELETE FROM ledger"}, wantErr: "read-only"},
		{name: "query schema fails with the context", setup: seed, cancel: true, args: []string{"query", "--schema"}, wantErr: "context canceled"},
		{name: "query fails with the context", setup: seed, cancel: true, args: []string{"query", "SELECT 1"}, wantErr: "context canceled"},

		// ------------------------------------------------------------ verify
		{name: "verify an intact chain", setup: seed, args: []string{"verify"}, wantOut: []string{"✅ verify complete: 4 rows, chain intact"}},
		{
			name: "verify reports a broken chain",
			inject: []di.Option{
				di.Replace(func() repository.LedgerRepository { return fakeLedger{checked: 2, broken: 3} }),
			},
			args:    []string{"verify"},
			wantErr: "ledger chain broken at row 3 (after 2 intact rows)",
		},
		{
			name: "verify reports a read failure",
			inject: []di.Option{
				di.Replace(func() repository.LedgerRepository { return fakeLedger{verifyErr: errors.New("ledger unreadable")} }),
			},
			args:    []string{"verify"},
			wantErr: "ledger unreadable",
		},

		// ------------------------------------------------------------ cost
		{name: "cost prices every tier", args: []string{"cost"}, wantOut: []string{"usage: cache read ", "  haiku   $", "  sonnet  $", "  opus    $"}},
		{name: "cost as JSON", args: []string{"cost", "--json", "--tool-calls", "10"}, wantOut: []string{`"usd_by_tier"`, `"prices_as_of"`}},

		// ------------------------------------------------------------ serve / mcp
		{name: "serve refuses a public address", args: []string{"serve", "--addr", "0.0.0.0:7457"}, wantErr: "loopback"},
		{name: "serve stops with the context", cancel: true, args: []string{"serve", "--addr", "127.0.0.1:0"}},
		{name: "mcp ends when stdin closes", stdio: true, args: []string{"mcp"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := isolate(t, tt.toml)
			for k, v := range tt.env {
				t.Setenv(k, strings.ReplaceAll(v, "$TMP", tmp))
			}
			for name, body := range tt.files {
				write(t, filepath.Join(tmp, name), body)
			}
			for _, s := range tt.setup {
				if _, err := execute(t, nil, false, s.stdin, s.args...); err != nil {
					t.Fatalf("setup %v: %v", s.args, err)
				}
			}
			if tt.stdio {
				r, w, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				_ = w.Close()
				devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				oldIn, oldOut := os.Stdin, os.Stdout
				os.Stdin, os.Stdout = r, devnull
				t.Cleanup(func() {
					os.Stdin, os.Stdout = oldIn, oldOut
					_ = r.Close()
					_ = devnull.Close()
				})
			}
			args := make([]string, len(tt.args))
			for i, a := range tt.args {
				args[i] = strings.ReplaceAll(a, "$TMP", tmp)
			}
			out, err := execute(t, tt.inject, tt.cancel, tt.stdin, args...)

			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("error = %v\nstdout:\n%s", err, out)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("error = %v, want one containing %q", err, tt.wantErr)
			}
			var missing []string
			for _, w := range tt.wantOut {
				if w = strings.ReplaceAll(w, "$TMP", tmp); !strings.Contains(out, w) {
					missing = append(missing, w)
				}
			}
			if diff := cmp.Diff([]string(nil), missing); diff != "" {
				t.Errorf("stdout lacks (-want +got):\n%s\nstdout:\n%s", diff, out)
			}
			if tt.golden != "" {
				if diff := cmp.Diff(strings.ReplaceAll(tt.golden, "$TMP", tmp), out); diff != "" {
					t.Errorf("stdout (-want +got):\n%s", diff)
				}
			}
			if tt.empty && out != "" {
				t.Errorf("stdout = %q, want empty", out)
			}
			if tt.wantJSON != nil {
				var v any
				if err := json.Unmarshal([]byte(out), &v); err != nil {
					t.Fatalf("stdout is not JSON: %v\n%s", err, out)
				}
				got := map[string]any{}
				for path := range tt.wantJSON {
					got[path] = pick(v, path)
				}
				if diff := cmp.Diff(tt.wantJSON, got); diff != "" {
					t.Errorf("JSON (-want +got):\n%s", diff)
				}
			}
			if tt.check != nil {
				tt.check(t, tmp)
			}
		})
	}
}

// brokenArtifact is an artifact store whose local artifact cannot be used.
type brokenArtifact struct{ err error }

func (b brokenArtifact) Load() (model.Artifact, error)     { return model.Artifact{}, b.err }
func (brokenArtifact) Save(model.Artifact) (string, error) { return "", nil }
