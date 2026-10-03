package claude_test

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/claude"
	"promari-statusline/internal/infrastructure/platform"
	"promari-statusline/internal/infrastructure/platform/platformtest"
)

var t0 = time.Date(2026, 10, 3, 4, 9, 5, 0, time.UTC)

func TestConfigDir(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	if got := claude.ConfigDir(sys); got != "/h/.claude" {
		t.Errorf("ConfigDir() = %q", got)
	}
	sys.Env["CLAUDE_CONFIG_DIR"] = "/elsewhere"
	if got := claude.ConfigDir(sys); got != "/elsewhere" {
		t.Errorf("ConfigDir() with CLAUDE_CONFIG_DIR = %q", got)
	}
}

func TestTranscriptTools(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		transcript string
		want       model.ToolStats
		err        error
	}{
		{
			"counts by tool, the top three by count and then by first use",
			`{"type":"tool_use","name":"Read"}
{"type":"tool_use","name":"Bash"}{"type":"tool_use","name":"Bash"}
{"type":"tool_use","name":"Edit"}
{"type":"tool_use","name":"Grep"}
{"type":"tool_result","is_error":true}
{"type":"tool_result","is_error":true,"again":{"is_error":true}}
{"type":"tool_result","is_error":false}
`,
			model.ToolStats{Total: 5, Errors: 2, Top: []model.ToolCount{{Name: "Bash", Count: 2}, {Name: "Read", Count: 1}, {Name: "Edit", Count: 1}}},
			nil,
		},
		{
			"the calls as Claude Code writes them, with the tools of an MCP server",
			`{"message":{"content":[{"type":"tool_use","id":"toolu_01","name":"Bash","input":{"command":"ls"}}]}}
{"message":{"content":[{"type":"tool_use","id":"toolu_02","name":"mcp__notion__API-post-page","input":{}}]}}
{"message":{"content":[{"type":"tool_use","id":"toolu_03","name":"mcp__docs.v2__read","input":{}}]}}
`,
			model.ToolStats{Total: 3, Top: []model.ToolCount{
				{Name: "Bash", Count: 1}, {Name: "mcp__notion__API-post-page", Count: 1}, {Name: "mcp__docs.v2__read", Count: 1},
			}},
			nil,
		},
		{
			// A transcript is full of other names; a git remote is the one that
			// showed up as a tool ("origin41").
			"a name that is not the name of a tool call is not a tool",
			`{"remotes":[{"name":"origin","host":"github.com"}],"model":{"name":"claude-opus"}}
{"type":"tool_use","id":"toolu_01","name":"Read","input":{"file_path":"/a"}}
`,
			model.ToolStats{Total: 1, Top: []model.ToolCount{{Name: "Read", Count: 1}}},
			nil,
		},
		{
			"a name inside the input of a call is not the name of the call",
			`{"type":"tool_use","id":"toolu_01","input":{"name":"notatool"}}
{"type":"tool_use","id":"toolu_02","name":"Edit","input":{"name":"x"}}
`,
			model.ToolStats{Total: 1, Top: []model.ToolCount{{Name: "Edit", Count: 1}}},
			nil,
		},
		{
			"text that quotes a tool call is not a call",
			`{"type":"text","text":"{\"type\":\"tool_use\",\"id\":\"toolu_01\",\"name\":\"Bash\"}"}` + "\n",
			model.ToolStats{},
			repository.ErrNone,
		},
		{
			"a call without a usable name is not counted",
			`{"type":"tool_use","id":"a","name":"a b"} {"type":"tool_use","id":"b","name":""} {"type":"tool_use","id":"c","name":"` + strings.Repeat("x", 129) + `"} {"type":"tool_use","id":"d","name":"cut off` + "\n" + `{"type":"tool_use","id":"e"} {"type":"tool_use"`,
			model.ToolStats{},
			repository.ErrNone,
		},
		{
			"a name of the longest length is a name",
			`{"type":"tool_use","id":"a","name":"` + strings.Repeat("x", 128) + `"}` + "\n",
			model.ToolStats{Total: 1, Top: []model.ToolCount{{Name: strings.Repeat("x", 128), Count: 1}}},
			nil,
		},
		{"a transcript without tool calls", `{"type":"user"}` + "\n", model.ToolStats{}, repository.ErrNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.Files["/t.jsonl"] = []byte(tt.transcript)
			read, err := claude.Transcript{Sys: sys}.Transcript(context.Background(), "/t.jsonl", model.Transcript{})
			got := read.Tools
			if got.Total != tt.want.Total || got.Errors != tt.want.Errors || !slices.Equal(got.Top, tt.want.Top) || !errors.Is(err, tt.err) {
				t.Errorf("Transcript().Tools = %+v, %v; want %+v, %v", got, err, tt.want, tt.err)
			}
		})
	}
	t.Run("a transcript that cannot be read", func(t *testing.T) {
		t.Parallel()
		if _, err := (claude.Transcript{Sys: platformtest.New(t0)}).Transcript(context.Background(), "/missing", model.Transcript{}); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("err = %v", err)
		}
	})
}

func TestTodos(t *testing.T) {
	t.Parallel()
	const dir = "/h/.claude/todos/"
	tests := []struct {
		name  string
		files map[string]string
		times map[string]time.Time
		want  model.Todos
		err   error
	}{
		{
			"the newest list of the session",
			map[string]string{
				dir + "s1-agent-a.json": `[{"status":"completed","content":"old"}]`,
				dir + "s1-agent-b.json": `[{"status":"completed","content":"a"},{"status":"in_progress","content":"b"},{"status":"in_progress","content":"c"},{"status":"pending","content":"d"}]`,
				dir + "s2-agent-a.json": `[{"status":"pending","content":"another session"}]`,
			},
			map[string]time.Time{dir + "s1-agent-a.json": t0.Add(-time.Hour), dir + "s1-agent-b.json": t0},
			model.Todos{Done: 1, Total: 4, Doing: "b"},
			nil,
		},
		{
			"items of an unexpected shape still count",
			map[string]string{dir + "s1.json": `[{"status":7},{"status":"completed"}]`},
			nil,
			model.Todos{Done: 1, Total: 2},
			nil,
		},
		{"an empty list", map[string]string{dir + "s1.json": `[]`}, nil, model.Todos{}, repository.ErrNone},
		{"a file that is not a list", map[string]string{dir + "s1.json": `{"todos":[]}`}, nil, model.Todos{}, repository.ErrNone},
		{"a file that is not JSON", map[string]string{dir + "s1.json": `[`}, nil, model.Todos{}, repository.ErrNone},
		{"no list for the session", map[string]string{dir + "s2.json": `[{"status":"pending"}]`}, nil, model.Todos{}, repository.ErrNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			for name, content := range tt.files {
				sys.Files[name] = []byte(content)
			}
			sys.Times = tt.times
			got, err := claude.Todos{Sys: sys}.Todos(context.Background(), "s1")
			if got != tt.want || !errors.Is(err, tt.err) {
				t.Errorf("Todos() = %+v, %v; want %+v, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestAccount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		config string
		want   string
		err    error
	}{
		{"signed in", `{"oauthAccount":{"emailAddress":"someone@example.com"},"other":[1,2]}`, "someone@example.com", nil},
		{"not signed in", `{"numStartups":3}`, "", repository.ErrNone},
		{"an account of an unexpected shape", `{"oauthAccount":"someone"}`, "", repository.ErrNone},
		{"a file that is not JSON", `{`, "", repository.ErrNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.Files["/h/.claude.json"] = []byte(tt.config)
			got, err := claude.Account{Sys: sys}.Account(context.Background())
			if got != tt.want || !errors.Is(err, tt.err) {
				t.Errorf("Account() = %q, %v; want %q, %v", got, err, tt.want, tt.err)
			}
		})
	}
	t.Run("no file", func(t *testing.T) {
		t.Parallel()
		if _, err := (claude.Account{Sys: platformtest.New(t0)}).Account(context.Background()); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("the login of the session's configuration directory", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		sys.Env["CLAUDE_CONFIG_DIR"] = "/work-account"
		sys.Files["/h/.claude.json"] = []byte(`{"oauthAccount":{"emailAddress":"home@example.com"}}`)
		sys.Files["/work-account/.claude.json"] = []byte(`{"oauthAccount":{"emailAddress":"work@example.com"}}`)
		a := claude.Account{Sys: sys}
		got, err := a.Account(context.Background())
		if got != "work@example.com" || err != nil || a.File() != "/work-account/.claude.json" {
			t.Errorf("Account() = %q, %v from %s", got, err, a.File())
		}
	})
}

func TestStatusPage(t *testing.T) {
	t.Parallel()
	const url = "https://status.anthropic.com/api/v2/status.json"
	tests := []struct {
		name string
		body string
		want model.Incident
		err  error
	}{
		{"operational", `{"status":{"indicator":"none","description":"All Systems Operational"}}`, model.Incident{}, repository.ErrNone},
		{"an incident", `{"page":{},"status":{"indicator":"major","description":"Partial Outage"}}`, model.Incident{Indicator: "major", Description: "Partial Outage"}, nil},
		{"a body of another shape", `{"status":"ok"}`, model.Incident{}, repository.ErrNone},
		{"a body that is not JSON", `<html>`, model.Incident{}, repository.ErrNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.URLs[url] = []byte(tt.body)
			got, err := claude.StatusPage{Sys: sys}.Incident(context.Background())
			if got != tt.want || !errors.Is(err, tt.err) {
				t.Errorf("Incident() = %+v, %v; want %+v, %v", got, err, tt.want, tt.err)
			}
		})
	}
	t.Run("the page cannot be reached", func(t *testing.T) {
		t.Parallel()
		_, err := claude.StatusPage{Sys: platformtest.New(t0)}.Incident(context.Background())
		if err == nil || errors.Is(err, repository.ErrNone) {
			t.Errorf("err = %v, want a failure", err)
		}
	})
}

func TestRegistry(t *testing.T) {
	t.Parallel()
	const url = "https://registry.npmjs.org/@anthropic-ai/claude-code/latest"
	tests := []struct {
		name string
		body string
		want string
		err  error
	}{
		{"a release", `{"name":"@anthropic-ai/claude-code","version":"2.1.287"}`, "2.1.287", nil},
		{"no version", `{"name":"@anthropic-ai/claude-code"}`, "", repository.ErrNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.URLs[url] = []byte(tt.body)
			got, err := claude.Registry{Sys: sys}.Latest(context.Background())
			if got != tt.want || !errors.Is(err, tt.err) {
				t.Errorf("Latest() = %q, %v; want %q, %v", got, err, tt.want, tt.err)
			}
		})
	}
	t.Run("the registry cannot be reached", func(t *testing.T) {
		t.Parallel()
		_, err := claude.Registry{Sys: platformtest.New(t0)}.Latest(context.Background())
		if err == nil || errors.Is(err, repository.ErrNone) {
			t.Errorf("err = %v, want a failure", err)
		}
	})
}

const settingsPath = "/h/.claude/settings.json"

func TestSettingsStatusLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		file    string
		want    model.StatusLineSetting
		err     error
		wantErr string
	}{
		{"no file", "", model.StatusLineSetting{}, repository.ErrNone, ""},
		{"no status line", `{"tui":"fullscreen"}`, model.StatusLineSetting{}, repository.ErrNone, ""},
		{
			"a status line",
			`{"statusLine":{"type":"command","command":"~/.claude/statusline.sh","padding":2,"extra":true}}`,
			model.StatusLineSetting{Type: "command", Command: "~/.claude/statusline.sh", Padding: 2},
			nil, "",
		},
		{"a status line of another shape", `{"statusLine":"off"}`, model.StatusLineSetting{}, nil, "statusLine"},
		{"a file that is not an object", `[1]`, model.StatusLineSetting{}, nil, "not a JSON object"},
		{"a file that is not JSON", `{"a":`, model.StatusLineSetting{}, nil, "not a JSON object"},
		{"a file that ends too early", `{"a":1`, model.StatusLineSetting{}, nil, "not a JSON object"},
		{"an empty file", ` `, model.StatusLineSetting{}, nil, "not a JSON object"},
		{"a member without a value", `{"a"}`, model.StatusLineSetting{}, nil, "not a JSON object"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			if tt.file != "" {
				sys.Files[settingsPath] = []byte(tt.file)
			}
			got, err := claude.Settings{Sys: sys}.StatusLine()
			switch {
			case tt.wantErr != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("err = %v, want %q", err, tt.wantErr)
				}
			case got != tt.want || !errors.Is(err, tt.err):
				t.Errorf("StatusLine() = %+v, %v; want %+v, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestSettingsEdit(t *testing.T) {
	t.Parallel()
	ours := model.StatusLineSetting{Type: model.CommandType, Command: "~/.claude/promari-statusline/psl render"}
	const backupPath = settingsPath + ".bak-20261003-040905"
	// The expected files are indented as psl writes them, not as Go is.
	// editorconfig-checker-disable
	tests := []struct {
		name   string
		before string
		edit   func(claude.Settings) (string, error)
		after  string
	}{
		{
			"a new status line goes to the end, and nothing else moves",
			`{"zeta": {"b":1,"a":[1,2]}, "alpha": true, "note": "é<>&"}`,
			func(s claude.Settings) (string, error) { return s.SetStatusLine(ours) },
			`{
  "zeta": {
    "b": 1,
    "a": [
      1,
      2
    ]
  },
  "alpha": true,
  "note": "é<>&",
  "statusLine": {
    "type": "command",
    "command": "~/.claude/promari-statusline/psl render",
    "padding": 0
  }
}`,
		},
		{
			"an existing status line is replaced in its place",
			`{"a":1,"statusLine":{"type":"command","command":"old"},"z":2}`,
			func(s claude.Settings) (string, error) { return s.SetStatusLine(ours) },
			`{
  "a": 1,
  "statusLine": {
    "type": "command",
    "command": "~/.claude/promari-statusline/psl render",
    "padding": 0
  },
  "z": 2
}`,
		},
		{
			"removing takes out only the status line",
			`{"a":1,"statusLine":{"type":"command","command":"old"},"z":2}`,
			func(s claude.Settings) (string, error) { return s.RemoveStatusLine() },
			`{
  "a": 1,
  "z": 2
}`,
		},
	}
	// editorconfig-checker-enable
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.Files[settingsPath] = []byte(tt.before)
			backup, err := tt.edit(claude.Settings{Sys: sys})
			if err != nil || backup != backupPath {
				t.Fatalf("edit = %q, %v; want the backup %q", backup, err, backupPath)
			}
			// The file ends with a newline, like any text file.
			if got, _ := sys.File(settingsPath); got != tt.after+"\n" {
				t.Errorf("settings =\n%s\nwant\n%s", got, tt.after)
			}
			if got, _ := sys.File(backupPath); got != tt.before {
				t.Errorf("backup = %q, want the file as it was", got)
			}
			if sys.Modes[settingsPath] != platform.Private || sys.Modes[backupPath] != platform.Private {
				t.Errorf("modes = %v", sys.Modes)
			}
		})
	}
	t.Run("without a file there is nothing to back up", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		backup, err := claude.Settings{Sys: sys}.SetStatusLine(ours)
		if err != nil || backup != "" {
			t.Fatalf("SetStatusLine = %q, %v", backup, err)
		}
		if got, err := (claude.Settings{Sys: sys}).StatusLine(); got != ours || err != nil {
			t.Errorf("read back %+v, %v", got, err)
		}
	})
	t.Run("a file that is not a JSON object is never overwritten", func(t *testing.T) {
		t.Parallel()
		for _, content := range []string{`[1]`, `{"a":1} trailing`, `{"a":1,"a":2}`, `// comment` + "\n" + `{}`} {
			sys := platformtest.New(t0)
			sys.Files[settingsPath] = []byte(content)
			if _, err := (claude.Settings{Sys: sys}).SetStatusLine(ours); err == nil {
				t.Errorf("%q was edited", content)
			}
			if got, _ := sys.File(settingsPath); got != content || len(sys.Files) != 1 {
				t.Errorf("%q became %q with files %v", content, got, sys.Glob("/h/.claude/*"))
			}
		}
	})
	t.Run("a file that cannot be written", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		sys.Files[settingsPath] = []byte(`{}`)
		sys.ReadOnly = true
		if _, err := (claude.Settings{Sys: sys}).SetStatusLine(ours); err == nil || !strings.Contains(err.Error(), "back up") {
			t.Errorf("err = %v, want the backup to fail first", err)
		}
		sys = platformtest.New(t0)
		sys.ReadOnly = true
		if _, err := (claude.Settings{Sys: sys}).RemoveStatusLine(); err == nil || !strings.Contains(err.Error(), "write the settings") {
			t.Errorf("err = %v, want the write to fail", err)
		}
	})
}

func TestBinary(t *testing.T) {
	t.Parallel()
	const installed = "/h/.claude/promari-statusline/psl"
	newSys := func() *platformtest.Fake {
		sys := platformtest.New(t0)
		sys.Self = "/plugins/cache/psl-1.0.0"
		sys.Files[sys.Self] = []byte("binary v1")
		return sys
	}
	t.Run("where the copy lives and how it is run", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			home    string
			config  string
			suffix  string
			path    string
			command string
		}{
			{"under the home directory: written with ~", "/h", "", "", installed, "~/.claude/promari-statusline/psl render"},
			{"windows", "/h", "", ".exe", installed + ".exe", "~/.claude/promari-statusline/psl.exe render"},
			{"a configuration directory elsewhere", "/h", "/etc/claude", "", "/etc/claude/promari-statusline/psl", "/etc/claude/promari-statusline/psl render"},
			{"a path with a space is quoted", "/h", "/my config", "", "/my config/promari-statusline/psl", `"/my config/promari-statusline/psl" render`},
			{"no home directory", "", "/c", "", "/c/promari-statusline/psl", "/c/promari-statusline/psl render"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				sys := platformtest.New(t0)
				sys.Home = tt.home
				sys.Env["CLAUDE_CONFIG_DIR"] = tt.config
				b := claude.Binary{Sys: sys, Suffix: tt.suffix}
				if b.Path() != tt.path || b.Command() != tt.command {
					t.Errorf("Path, Command = %q, %q; want %q, %q", b.Path(), b.Command(), tt.path, tt.command)
				}
			})
		}
	})
	t.Run("install, compare, replace, remove", func(t *testing.T) {
		t.Parallel()
		sys := newSys()
		b := claude.Binary{Sys: sys}
		if _, err := b.InSync(); !errors.Is(err, repository.ErrNone) {
			t.Fatalf("InSync() before any install: %v", err)
		}
		if err := b.Install(); err != nil {
			t.Fatal(err)
		}
		if got, _ := sys.File(installed); got != "binary v1" || sys.Modes[installed] != platform.Executable {
			t.Errorf("installed %q with mode %v", got, sys.Modes[installed])
		}
		if same, err := b.InSync(); !same || err != nil {
			t.Errorf("InSync() after an install = %v, %v", same, err)
		}
		sys.Files[sys.Self] = []byte("binary v2")
		if same, err := b.InSync(); same || err != nil {
			t.Errorf("InSync() after an update = %v, %v", same, err)
		}
		if err := b.Remove(); err != nil {
			t.Fatal(err)
		}
		if _, ok := sys.File(installed); ok {
			t.Error("the copy is still there after Remove")
		}
	})
	t.Run("a running binary that cannot be found or read", func(t *testing.T) {
		t.Parallel()
		sys := newSys()
		sys.Files[installed] = []byte("binary v1")
		sys.Self = ""
		b := claude.Binary{Sys: sys}
		if _, err := b.InSync(); err == nil {
			t.Error("InSync() without a running binary succeeded")
		}
		if err := b.Install(); err == nil {
			t.Error("Install() without a running binary succeeded")
		}
		sys.Self = "/gone"
		if err := b.Install(); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Install() of a binary that cannot be read: %v", err)
		}
	})
}

func TestLauncher(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data string
		file string
		log  string
		want string
		err  error
	}{
		{"no failure recorded", "", "", "", "", repository.ErrNone},
		{
			"a failure in the default data directory",
			"", "/h/.claude/plugins/data/promari-statusline/launcher_error", "2026-10-03T00:00:00Z\ndownload failed: https://example.com/psl\n",
			"2026-10-03T00:00:00Z download failed: https://example.com/psl", nil,
		},
		{"a failure in the directory Claude Code names", "/data", "/data/launcher_error", "t\nno binary", "t no binary", nil},
		{"an empty file", "", "/h/.claude/plugins/data/promari-statusline/launcher_error", " \n", "", repository.ErrNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.Env["CLAUDE_PLUGIN_DATA"] = tt.data
			if tt.file != "" {
				sys.Files[tt.file] = []byte(tt.log)
			}
			got, err := claude.Launcher{Sys: sys}.LastError()
			if got != tt.want || !errors.Is(err, tt.err) {
				t.Errorf("LastError() = %q, %v; want %q, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

// A transcript as Claude Code writes it, one entry per line. The JSON below
// is laid out as JSON is, not as Go is.
// editorconfig-checker-disable
const recorded = `{"type":"user","timestamp":"2026-10-03T04:00:00Z","origin":{"kind":"human"},"message":{"role":"user","content":"fix the bug"}}
{"type":"assistant","timestamp":"2026-10-03T04:00:05Z","thinkingDurationMs":1500,"message":{"id":"msg_1","model":"claude-opus-5-5","stop_reason":"tool_use","usage":{"input_tokens":10,"cache_creation_input_tokens":1000,"cache_read_input_tokens":5000,"output_tokens":200,"output_tokens_details":{"thinking_tokens":50},"server_tool_use":{"web_search_requests":1,"web_fetch_requests":2},"cache_creation":{"ephemeral_1h_input_tokens":1000}},"content":[{"type":"thinking","thinking":"..."}]}}
{"type":"assistant","timestamp":"2026-10-03T04:00:06Z","message":{"id":"msg_1","model":"claude-opus-5-5","usage":{"input_tokens":10,"cache_creation_input_tokens":1000,"cache_read_input_tokens":5000,"output_tokens":200},"content":[{"type":"tool_use","id":"toolu_1","name":"Edit","input":{"file_path":"/w/a.go","old_string":"x"}}]}}
{"type":"user","timestamp":"2026-10-03T04:00:07Z","origin":null,"message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","is_error":true}]}}
{"type":"user","timestamp":"2026-10-03T04:00:08Z","origin":{"kind":"task-notification"},"message":{"content":"a task ended"}}
{"type":"user","timestamp":"2026-10-03T04:00:09Z","toolDenialKind":"user-rejected","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_2"}]}}
{"type":"assistant","timestamp":"2026-10-03T04:00:10Z","isSidechain":true,"message":{"id":"msg_2","model":"claude-haiku-4-5","stop_reason":"max_tokens","usage":{"input_tokens":3,"output_tokens":7},"content":[{"type":"tool_use","id":"toolu_3","name":"Write","input":{"file_path":"/w/b.go"}},{"type":"tool_use","id":"toolu_4","name":"NotebookEdit","input":{"notebook_path":"/w/c.ipynb"}},{"type":"tool_use","id":"toolu_5","name":"Edit","input":{"file_path":"/w/a.go"}}]}}
{"type":"assistant","message":{"id":"msg_3","model":"<synthetic>","stop_reason":"refusal","usage":{}}}
{"type":"user","interruptedMessageId":"msg_3","message":{"content":"[Request interrupted by user]"}}
{"type":"system","subtype":"turn_duration","durationMs":64000}
{"type":"system","subtype":"turn_duration","durationMs":0}
{"type":"system","subtype":"compact_boundary","timestamp":"2026-10-03T04:30:00Z"}
{"type":"permission-mode","permissionMode":"plan"}
{"type":"permission-mode","permissionMode":"acceptEdits"}
{"type":"permission-mode"}
{"type":"attachment","attachment":{"type":"hook_success","hookEvent":"PreToolUse"}}
{"type":"attachment","attachment":{"type":"hook_success","hookEvent":"PostToolUse"}}
{"type":"attachment","attachment":{"type":"hook_non_blocking_error"}}
{"type":"attachment","attachment":{"type":"hook_cancelled"}}
{"type":"attachment","attachment":{"type":"queued_command"}}
{"type":"attachment","attachment":{"type":"diagnostics"}}
{"type":"attachment","attachment":{"type":"output_style"}}
not json
`

// editorconfig-checker-enable

func TestTranscriptRecords(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	sys.Files["/t.jsonl"] = []byte(recorded)
	got, err := claude.Transcript{Sys: sys}.Transcript(context.Background(), "/t.jsonl", model.Transcript{})
	if err != nil {
		t.Fatal(err)
	}
	want := model.TokenTotals{Input: 13, CacheWrite: 1000, CacheWrite1h: 1000, CacheRead: 5000, Output: 207, Thinking: 50}
	if got.Tokens != want {
		t.Errorf("Tokens = %+v; want %+v (a message written as two entries counts once)", got.Tokens, want)
	}
	for _, c := range []struct {
		name      string
		got, want int
	}{
		{"requests", got.Requests, 3},
		{"side requests", got.SideRequests, 1},
		{"prompts", got.Prompts, 1},
		{"interrupts", got.Interrupts, 1},
		{"denials", got.Denials, 1},
		{"refusals", got.Refusals, 1},
		{"truncated", got.Truncated, 1},
		{"web searches", got.WebSearches, 1},
		{"web fetches", got.WebFetches, 2},
		{"compactions", got.Compactions, 1},
		{"tool calls", got.Tools.Total, 4},
		{"tool errors", got.Tools.Errors, 1},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
	if !maps.Equal(got.Models, map[string]int{"claude-opus-5-5": 1, "claude-haiku-4-5": 1}) {
		t.Errorf("Models = %v; a synthetic response is no model", got.Models)
	}
	if want := []string{"/w/a.go", "/w/b.go", "/w/c.ipynb"}; !slices.Equal(got.Files, want) {
		t.Errorf("Files = %v, want %v", got.Files, want)
	}
	if got.PermissionMode != "acceptEdits" {
		t.Errorf("PermissionMode = %q; the last mode, an entry without one changes nothing", got.PermissionMode)
	}
	if !slices.Equal(got.Turns, []float64{64}) || got.ThinkingSeconds != 1.5 {
		t.Errorf("Turns = %v, ThinkingSeconds = %v", got.Turns, got.ThinkingSeconds)
	}
	if !got.Started.Equal(time.Date(2026, 10, 3, 4, 0, 0, 0, time.UTC)) || !got.LastCompaction.Equal(time.Date(2026, 10, 3, 4, 30, 0, 0, time.UTC)) {
		t.Errorf("Started = %v, LastCompaction = %v", got.Started, got.LastCompaction)
	}
	if got.Cursor.Offset != int64(len(recorded)) || !slices.Equal(got.Cursor.Recent, []string{"msg_1", "msg_2", "msg_3"}) {
		t.Errorf("Cursor = %+v", got.Cursor)
	}
}

func TestTranscriptContinues(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const first = `{"type":"assistant","message":{"id":"msg_1","usage":{"output_tokens":5},"content":[{"type":"tool_use","id":"a","name":"Bash","input":{}}]}}` + "\n"
	const repeat = `{"type":"assistant","message":{"id":"msg_1","usage":{"output_tokens":5},"content":[{"type":"tool_use","id":"b","name":"Read","input":{}}]}}` + "\n"
	const half = `{"type":"assistant","message":{"id":"msg_2","usage":{"output_tokens":7}`
	sys := platformtest.New(t0)
	reader := claude.Transcript{Sys: sys}

	sys.Files["/t.jsonl"] = []byte(first + half)
	got, err := reader.Transcript(ctx, "/t.jsonl", model.Transcript{})
	if err != nil || got.Tokens.Output != 5 || got.Cursor.Offset != int64(len(first)) {
		t.Fatalf("first read = %+v, %v; an unfinished line waits for the next read", got, err)
	}
	// The rest of the response arrives, and a block of the first message
	// that repeats its usage.
	sys.Files["/t.jsonl"] = []byte(first + half + `,"content":[]}}` + "\n" + repeat)
	got, err = reader.Transcript(ctx, "/t.jsonl", got)
	if err != nil || got.Requests != 2 || got.Tokens.Output != 12 || got.Tools.Total != 2 {
		t.Fatalf("second read = %+v, %v", got, err)
	}
	if want := []model.ToolCount{{Name: "Bash", Count: 1}, {Name: "Read", Count: 1}}; !slices.Equal(got.Tools.Top, want) {
		t.Errorf("Top = %v, want %v", got.Tools.Top, want)
	}
	nothingNew, err := reader.Transcript(ctx, "/t.jsonl", got)
	if err != nil || nothingNew.Requests != 2 || nothingNew.Cursor.Offset != got.Cursor.Offset {
		t.Errorf("a read without anything new = %+v, %v", nothingNew, err)
	}

	// A shorter file is a new transcript, read from the start.
	sys.Files["/t.jsonl"] = []byte(first)
	got, err = reader.Transcript(ctx, "/t.jsonl", got)
	if err != nil || got.Requests != 1 || got.Cursor.Offset != int64(len(first)) {
		t.Errorf("a replaced transcript = %+v, %v", got, err)
	}
}

func TestHumanPrompts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		entry string
		want  int
	}{
		{"named human", `{"type":"user","origin":{"kind":"human"},"message":{"content":"x"}}`, 1},
		{"named otherwise", `{"type":"user","origin":{"kind":"task-notification"},"message":{"content":"x"}}`, 0},
		{"unnamed text", `{"type":"user","message":{"content":"x"}}`, 1},
		{"unnamed blocks of text", `{"type":"user","message":{"content":[{"type":"text","text":"x"}]}}`, 1},
		{"unnamed tool result", `{"type":"user","message":{"content":[{"type":"tool_result"}]}}`, 0},
		{"unnamed empty", `{"type":"user","message":{"content":[]}}`, 0},
		{"a meta entry", `{"type":"user","isMeta":true,"message":{"content":"x"}}`, 0},
		{"a compaction summary", `{"type":"user","isCompactSummary":true,"message":{"content":"x"}}`, 0},
		{"a subagent's prompt", `{"type":"user","isSidechain":true,"message":{"content":"x"}}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.Files["/t.jsonl"] = []byte(tt.entry + "\n")
			got, _ := claude.Transcript{Sys: sys}.Transcript(context.Background(), "/t.jsonl", model.Transcript{})
			if got.Prompts != tt.want {
				t.Errorf("Prompts = %d, want %d", got.Prompts, tt.want)
			}
		})
	}
}

// quality is a transcript of checks and edits: a test run whose pipe hid its
// failure, a refused run, an interrupted run, a passing run, a build, an edit
// that failed and one that succeeded, and a call repeated as it was.
const quality = `{"type":"assistant","timestamp":"2026-10-03T04:00:00Z","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"go test ./... | tail"}}]}}
{"type":"user","timestamp":"2026-10-03T04:01:00Z","toolUseResult":{"stdout":"--- FAIL: TestX\nFAIL\n","stderr":"","interrupted":false},"message":{"content":[{"type":"tool_result","tool_use_id":"t1","is_error":false,"content":"--- FAIL"}]}}
{"type":"assistant","message":{"id":"m2","content":[{"type":"tool_use","id":"e1","name":"Edit","input":{"file_path":"/w/a.go"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"e1","is_error":true,"content":"<tool_use_error>String to replace not found"}]}}
{"type":"assistant","message":{"id":"m3","content":[{"type":"tool_use","id":"e2","name":"Edit","input":{"file_path":"/w/a.go"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"e2","content":"ok"}]}}
{"type":"assistant","message":{"id":"m4","content":[{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"go test ./..."}}]}}
{"type":"user","toolDenialKind":"rule","message":{"content":[{"type":"tool_result","tool_use_id":"t2","is_error":true,"content":"Permission denied"}]}}
{"type":"assistant","message":{"id":"m5","content":[{"type":"tool_use","id":"t3","name":"Bash","input":{"command":"go test ./..."}}]}}
{"type":"user","toolUseResult":{"stdout":"","interrupted":true},"message":{"content":[{"type":"tool_result","tool_use_id":"t3","is_error":true,"content":"Interrupted"}]}}
{"type":"assistant","message":{"id":"m6","content":[{"type":"tool_use","id":"b1","name":"Bash","input":{"command":"go vet ./..."}}]}}
{"type":"user","toolUseResult":"Error: Exit code 1","message":{"content":[{"type":"tool_result","tool_use_id":"b1","is_error":true,"content":[{"type":"text","text":"Exit code 1\na.go:1:1: bad"}]}]}}
{"type":"assistant","message":{"id":"m7","content":[{"type":"tool_use","id":"x1","name":"Read","input":{"file_path":"/w/b.go"}}]}}
{"type":"assistant","message":{"id":"m8","content":[{"type":"tool_use","id":"x2","name":"Read","input":{"file_path":"/w/b.go"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"unknown","content":"x"}]}}
`

func TestTranscriptQuality(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	sys.Files["/q.jsonl"] = []byte(quality)
	got, err := claude.Transcript{Sys: sys}.Transcript(context.Background(), "/q.jsonl", model.Transcript{})
	if err != nil {
		t.Fatal(err)
	}
	q := got.Quality
	if want := (model.CheckRuns{Runs: 1, Failed: 1, Masked: 1, Last: model.OutcomeFail}); q.Tests != want {
		t.Errorf("Tests = %+v, want %+v (a refused or interrupted run did not run)", q.Tests, want)
	}
	if !q.RedSince.Equal(time.Date(2026, 10, 3, 4, 1, 0, 0, time.UTC)) {
		t.Errorf("RedSince = %v; the time of the result", q.RedSince)
	}
	if want := (model.CheckRuns{Runs: 1, Failed: 1, Last: model.OutcomeFail}); q.Builds != want {
		t.Errorf("Builds = %+v, want %+v (the text of the result is read when there is no structured one)", q.Builds, want)
	}
	if q.Edits != 2 || q.EditFailures != 1 || q.ReEdits != 1 || q.EditFailStreak != 0 {
		t.Errorf("edits = %d, failures %d, re-edits %d, streak %d", q.Edits, q.EditFailures, q.ReEdits, q.EditFailStreak)
	}
	if tr := got.Trace; tr.Explores != 2 || tr.Rereads != 1 || tr.MaxEditRun != 2 || tr.ObservedBytes == 0 || q.RedCalls != 7 {
		t.Errorf("Trace = %+v, RedCalls = %d", tr, q.RedCalls)
	}
	// Repeated as they were: the edit after it failed, the run after it was refused, the read.
	if !slices.Equal(q.Unverified, []string{"/w/a.go"}) || q.Repeats != 3 || len(q.Pending) != 0 {
		t.Errorf("Unverified = %v, Repeats = %d, Pending = %v", q.Unverified, q.Repeats, q.Pending)
	}
}

func TestTranscriptFormat(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	sys.Files["/q.jsonl"] = []byte(quality)
	// A reading made by an earlier version reached the end, but never counted the quality.
	old := model.Transcript{Prompts: 99, Cursor: model.TranscriptCursor{Offset: int64(len(quality))}}
	got, err := claude.Transcript{Sys: sys}.Transcript(context.Background(), "/q.jsonl", old)
	if err != nil {
		t.Fatal(err)
	}
	if got.Prompts == 99 || got.Quality.Tests.Runs != 1 || got.Cursor.Format != model.TranscriptFormat {
		t.Errorf("got Prompts %d, Tests %+v, Format %d; a reading of another format is read again from the start", got.Prompts, got.Quality.Tests, got.Cursor.Format)
	}
}

// claims is a transcript in which the agent says the tests pass before any ran,
// two prompts with tokens between them, and a compaction.
const claims = `{"type":"user","message":{"role":"user","content":"fix it"}}
{"type":"assistant","message":{"id":"r1","usage":{"input_tokens":100,"output_tokens":50},"content":[{"type":"text","text":"Done. All tests pass now."}]}}
{"type":"assistant","isSidechain":true,"message":{"id":"r2","usage":{"input_tokens":10},"content":[{"type":"text","text":"tests passed"}]}}
{"type":"user","message":{"role":"user","content":"again"}}
{"type":"system","subtype":"compact_boundary","timestamp":"2026-10-03T05:00:00Z"}
{"type":"user","message":{"role":"user","content":"and again"}}
`

func TestTranscriptClaims(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	sys.Files["/c.jsonl"] = []byte(claims)
	got, err := claude.Transcript{Sys: sys}.Transcript(context.Background(), "/c.jsonl", model.Transcript{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Quality.Claims != 0 {
		t.Errorf("Claims = %d; before a test run is recognised, a claim cannot be told from the truth", got.Quality.Claims)
	}
	if tr := got.Trace; !slices.Equal(tr.TurnTokens, []float64{160}) || tr.PromptsSinceCompact != 1 {
		t.Errorf("TurnTokens = %v, PromptsSinceCompact = %d", tr.TurnTokens, tr.PromptsSinceCompact)
	}
}
