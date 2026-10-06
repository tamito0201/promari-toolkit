package di_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"promari-statusline/internal/di"
	"promari-statusline/internal/infrastructure/platform/platformtest"
)

var t0 = time.Date(2026, 10, 3, 4, 9, 0, 0, time.UTC)

// The container is wiring, so the test is that the wires reach: each command
// runs through its use case to the adapters and comes back with their answer.
func TestNew(t *testing.T) {
	t.Parallel()
	const settings = "/h/.claude/settings.json"
	tests := []struct {
		name  string
		stdin string
		args  []string
		code  int
		want  []string
		check func(*testing.T, *platformtest.Fake)
	}{
		{
			"render reaches the sources, the stores and the presenter",
			`{"session_id":"s1","model":{"display_name":"Opus"},"workspace":{"current_dir":"/work"},"context_window":{"context_window_size":200000,"used_percentage":42},"rate_limits":{"five_hour":{"used_percentage":29,"resets_at":1790985000}}}`,
			[]string{"render"},
			0,
			[]string{"🧠 Context", "42%", "Opus", "develop", "👤 someone", "\x1b[38;5;141m"},
			func(t *testing.T, sys *platformtest.Fake) {
				t.Helper()
				for _, file := range []string{"last-input.json", "width.txt", "sessions/s1.json"} {
					if _, ok := sys.File("/h/.cache/promari-statusline/" + file); !ok {
						t.Errorf("the render did not write %s", file)
					}
				}
				// The remembered answers are kept per key, a file each.
				for _, kind := range []string{"account", "pull-request"} {
					if len(sys.Glob("/h/.cache/promari-statusline/"+kind+"/*.json")) != 1 {
						t.Errorf("the render did not remember its %s", kind)
					}
				}
				const board = `{"ts":1791000540,"rl":{"five_hour":{"used_percentage":29,"resets_at":1790985000}}}`
				if got, _ := sys.File("/h/.cache/claude-rate-limits.json"); got != board {
					t.Errorf("the board for other tools = %s, want %s", got, board)
				}
			},
		},
		{
			"setup copies the binary and writes the settings",
			"",
			[]string{"setup"},
			0,
			[]string{"✅ installed: /h/.claude/promari-statusline/psl", "now runs: ~/.claude/promari-statusline/psl render"},
			func(t *testing.T, sys *platformtest.Fake) {
				t.Helper()
				if got, _ := sys.File("/h/.claude/promari-statusline/psl"); got != "the binary" {
					t.Errorf("installed %q", got)
				}
				if got, _ := sys.File(settings); !strings.Contains(got, `"command": "~/.claude/promari-statusline/psl render"`) || !strings.Contains(got, `"tui": "fullscreen"`) {
					t.Errorf("settings = %s", got)
				}
			},
		},
		{
			"doctor reads the settings, the copy, PATH, the terminal and the launcher's record",
			"",
			[]string{"doctor"},
			1,
			[]string{"❌ settings " + settings + ": no statusLine", "❌ installed binary", "✅ git: /usr/bin/git", "⚠️  gh: not found", "87 cells (COLUMNS)", "⚠️  launcher: t download failed"},
			nil,
		},
		{
			"the hook leaves a user alone who never ran setup",
			"{}",
			[]string{"hook", "SessionStart"},
			0, nil,
			func(t *testing.T, sys *platformtest.Fake) {
				t.Helper()
				if _, ok := sys.File("/h/.claude/promari-statusline/psl"); ok {
					t.Error("the hook installed the binary without a setup")
				}
			},
		},
		{"uninstall", "", []string{"uninstall"}, 0, []string{"no statusLine in " + settings, "✅ removed the installed binary"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.Self = "/plugin/bin/psl-1.0.0"
			sys.Files[sys.Self] = []byte("the binary")
			sys.Files[settings] = []byte(`{"tui":"fullscreen"}`)
			sys.Files["/h/.claude.json"] = []byte(`{"oauthAccount":{"emailAddress":"someone@example.com"}}`)
			sys.Files["/h/.claude/plugins/data/promari-statusline/launcher_error"] = []byte("t\ndownload failed\n")
			sys.Env["COLUMNS"] = "87"
			sys.Path["git"] = "/usr/bin/git"
			sys.Cmds["git --no-optional-locks -C /work branch --show-current"] = platformtest.Result{Out: "develop\n"}

			var out, errOut bytes.Buffer
			app := di.New(sys, di.Streams{In: strings.NewReader(tt.stdin), Out: &out, Err: &errOut})
			if code := app.Run(t.Context(), tt.args); code != tt.code {
				t.Fatalf("exit code = %d, want %d\nstdout: %s\nstderr: %s", code, tt.code, out.String(), errOut.String())
			}
			for _, want := range tt.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("stdout lacks %q:\n%s", want, out.String())
				}
			}
			if tt.check != nil {
				tt.check(t, sys)
			}
		})
	}
}

// The dashboard is wired to the real server: an address it cannot listen on
// comes back as the server's own error.
func TestNewWiresTheDashboard(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	var out, errOut bytes.Buffer
	app := di.New(sys, di.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut})
	if code := app.Run(t.Context(), []string{"dashboard", "--addr", "127.0.0.1:notaport"}); code != 1 || !strings.Contains(errOut.String(), "listen on 127.0.0.1:notaport") {
		t.Errorf("code %d, stderr %q", code, errOut.String())
	}
}
