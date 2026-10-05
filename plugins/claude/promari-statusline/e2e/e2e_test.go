// Package e2e_test runs the built binary the way Claude Code and a user do:
// as a process, with a home directory of its own. Nothing is faked except the
// places it would otherwise reach out of the test: the cache holds fresh
// answers for the two HTTP endpoints, and the working directory is no git
// repository, so no test depends on the network.
package e2e_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"promari-statusline/internal/domain/service"
)

// binary is the psl built for this test run.
var binary string

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "psl-e2e")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()
	binary = filepath.Join(dir, "psl")
	args := []string{"build", "-o", binary}
	// Under `task coverage` the binary counts its own statements into the same
	// directory as the unit tests (see Taskfile.yml).
	if os.Getenv("PSL_E2E_COVERDIR") != "" {
		args = append(args, "-cover", "-coverpkg=promari-statusline/...")
	}
	build := exec.CommandContext(context.Background(), "go", append(args, "../cmd/psl")...)
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build psl: %v\n%s", err, out)
		return 1
	}
	return m.Run()
}

// home is a user's home directory for one test.
type home struct {
	t   *testing.T
	dir string
	// columns is the terminal width psl is told; zero means the default.
	columns int
}

const defaultColumns = 100

func newHome(t *testing.T) home {
	t.Helper()
	h := home{t: t, dir: t.TempDir()}
	// Fresh answers for the endpoints, so that a render does not ask the network.
	now := time.Now().Format(time.RFC3339)
	h.write(".cache/promari-statusline/incident.json", `{"at":"`+now+`","found":false,"value":{"indicator":""}}`)
	h.write(".cache/promari-statusline/latest.json", `{"at":"`+now+`","found":true,"value":"99.0.0"}`)
	return h
}

func (h home) path(name string) string { return filepath.Join(h.dir, name) }

func (h home) write(name, content string) {
	h.t.Helper()
	if err := os.MkdirAll(filepath.Dir(h.path(name)), 0o700); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(h.path(name), []byte(content), 0o600); err != nil {
		h.t.Fatal(err)
	}
}

func (h home) read(name string) string {
	h.t.Helper()
	data, err := os.ReadFile(h.path(name))
	if err != nil {
		h.t.Fatal(err)
	}
	return string(data)
}

// psl runs the binary in this home and returns its exit code and output.
func (h home) psl(stdin string, args ...string) (code int, stdout, stderr string) {
	h.t.Helper()
	cmd := exec.CommandContext(h.t.Context(), binary, args...)
	cmd.Dir = h.dir
	cmd.Stdin = strings.NewReader(stdin)
	columns := h.columns
	if columns == 0 {
		columns = defaultColumns
	}
	cmd.Env = []string{
		"HOME=" + h.dir,
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
		fmt.Sprintf("COLUMNS=%d", columns),
	}
	if dir := os.Getenv("PSL_E2E_COVERDIR"); dir != "" {
		cmd.Env = append(cmd.Env, "GOCOVERDIR="+dir)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return exit.ExitCode(), out.String(), errOut.String()
	}
	if err != nil {
		h.t.Fatalf("run psl %v: %v", args, err)
	}
	return 0, out.String(), errOut.String()
}

var sgr = regexp.MustCompile(`\x1b\[[0-9;]*m`)

const report = `{"session_id":"e2e","prompt_id":"p1","version":"2.1.34","model":{"display_name":"Opus 5.5"},
"context_window":{"context_window_size":200000,"used_percentage":42,"current_usage":{"input_tokens":84000}},
"cost":{"total_cost_usd":1.23,"total_duration_ms":600000,"total_api_duration_ms":300000,"total_lines_added":10,"total_lines_removed":2},
"rate_limits":{"five_hour":{"used_percentage":1},"seven_day":{"used_percentage":73}}}`

func TestRender(t *testing.T) {
	t.Parallel()
	h := newHome(t)
	code, stdout, stderr := h.psl(report, "render")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	plain := sgr.ReplaceAllString(stdout, "")
	for _, want := range []string{
		"🧠 Context │ ████░░░░░░ 42% 84k/200k 残 116k", "⚡ Claude 5h ░░░░░ 1% 7d ████░ 73%", "💰 Cost │ Sess $1.23",
		"⏰ 10m (API 5m)", "Lines +10-2", "Parallel ×0.50", "🧭 Env │ Opus 5.5", "🕐 ", "v2.1.34", "🆙 Update v99.0.0",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("the status line lacks %q:\n%s", want, plain)
		}
	}
	if !strings.Contains(stdout, "\x1b[38;5;") {
		t.Error("the status line has no colours")
	}
	if got := h.read(".cache/promari-statusline/last-input.json"); got != report {
		t.Errorf("the recorded input differs from what was sent: %q", got)
	}
	if got := h.read(".cache/promari-statusline/width.txt"); got != "COLUMNS 97\n" {
		t.Errorf("recorded width %q", got)
	}
	// What another tool reads (see internal/infrastructure/usage/board.go).
	board := regexp.MustCompile(`^\{"ts":[0-9.]+,"rl":\{"five_hour":\{"used_percentage":1\},"seven_day":\{"used_percentage":73\}\}\}$`)
	if got := h.read(".cache/claude-rate-limits.json"); !board.MatchString(got) {
		t.Errorf("the board for other tools = %s", got)
	}

	// The next render of the session remembers the first.
	_, stdout, _ = h.psl(strings.Replace(report, `"p1"`, `"p2"`, 1), "render")
	if plain := sgr.ReplaceAllString(stdout, ""); !strings.Contains(plain, "Turns ×2") {
		t.Errorf("two prompts were not counted:\n%s", plain)
	}
	// A render without rate limits shows the remembered ones.
	_, stdout, _ = h.psl(`{"model":{"display_name":"Opus 5.5"}}`, "render")
	if plain := sgr.ReplaceAllString(stdout, ""); !strings.Contains(plain, "⚡ Claude 5h") || !strings.Contains(plain, "初回応答待ち") {
		t.Errorf("the first render of a new session:\n%s", plain)
	}
}

func TestRenderNeverFails(t *testing.T) {
	t.Parallel()
	for name, stdin := range map[string]string{"no input": "", "an empty object": "{}", "not JSON": "<html>", "a list": "[1]"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			code, stdout, stderr := newHome(t).psl(stdin, "render")
			if code != 0 || stderr != "" || !strings.Contains(stdout, "💻 System") {
				t.Errorf("exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
			}
		})
	}
}

func TestSetupDoctorUninstall(t *testing.T) {
	t.Parallel()
	h := newHome(t)
	const settings = ".claude/settings.json"
	h.write(settings, `{"tui":"fullscreen","statusLine":{"type":"command","command":"~/.claude/statusline.sh"},"z":1}`)
	before := h.read(settings)

	if code, stdout, _ := h.psl("", "doctor"); code != 1 || !strings.Contains(stdout, "statusLine runs another command") || !strings.Contains(stdout, "not installed") {
		t.Errorf("doctor before setup: exit %d\n%s", code, stdout)
	}

	code, stdout, _ := h.psl("", "setup", "--dry-run")
	if code != 0 || !strings.Contains(stdout, "would set statusLine") || h.read(settings) != before {
		t.Fatalf("a dry run: exit %d, settings changed: %v\n%s", code, h.read(settings) != before, stdout)
	}
	if _, err := os.Stat(h.path(".claude/promari-statusline/psl")); err == nil {
		t.Fatal("a dry run installed the binary")
	}

	code, stdout, stderr := h.psl("", "setup")
	if code != 0 || !strings.Contains(stdout, "previous command: ~/.claude/statusline.sh") {
		t.Fatalf("setup: exit %d, stderr %q\n%s", code, stderr, stdout)
	}
	// The expected file is indented as psl writes it, not as Go is.
	// editorconfig-checker-disable
	want := `{
  "tui": "fullscreen",
  "statusLine": {
    "type": "command",
    "command": "~/.claude/promari-statusline/psl render",
    "padding": 0,
    "refreshInterval": 5
  },
  "z": 1
}
`
	// editorconfig-checker-enable
	if got := h.read(settings); got != want {
		t.Errorf("settings after setup =\n%s\nwant\n%s", got, want)
	}
	backups, _ := filepath.Glob(h.path(settings + ".bak-*"))
	if len(backups) != 1 {
		t.Fatalf("backups: %v", backups)
	}
	if data, _ := os.ReadFile(backups[0]); string(data) != before {
		t.Errorf("the backup is not the file as it was: %q", data)
	}
	info, err := os.Stat(h.path(".claude/promari-statusline/psl"))
	if err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("the installed binary: %v, %v", info, err)
	}

	// The installed copy is what Claude Code runs.
	installed := exec.CommandContext(t.Context(), h.path(".claude/promari-statusline/psl"), "version")
	if out, err := installed.Output(); err != nil || !strings.HasPrefix(string(out), "psl ") {
		t.Errorf("the installed copy: %q, %v", out, err)
	}

	if code, stdout, _ := h.psl("", "doctor"); code != 0 || !strings.Contains(stdout, "is the running binary") {
		t.Errorf("doctor after setup: exit %d\n%s", code, stdout)
	}
	if code, stdout, _ := h.psl("", "setup"); code != 0 || !strings.Contains(stdout, "settings unchanged") {
		t.Errorf("a second setup: exit %d\n%s", code, stdout)
	}
	if code, stdout, stderr := h.psl("{}", "hook", "SessionStart"); code != 0 || stdout != "" || stderr != "" {
		t.Errorf("the hook: exit %d, %q, %q", code, stdout, stderr)
	}

	if code, stdout, _ := h.psl("", "uninstall"); code != 0 || !strings.Contains(stdout, "removed statusLine") {
		t.Errorf("uninstall: exit %d\n%s", code, stdout)
	}
	if got := h.read(settings); strings.Contains(got, "statusLine") || !strings.Contains(got, `"tui": "fullscreen"`) {
		t.Errorf("settings after uninstall = %s", got)
	}
	if _, err := os.Stat(h.path(".claude/promari-statusline/psl")); err == nil {
		t.Error("the binary is still installed after uninstall")
	}
}

func TestSetupLeavesBrokenSettingsAlone(t *testing.T) {
	t.Parallel()
	h := newHome(t)
	const settings = ".claude/settings.json"
	h.write(settings, `{"tui": "fullscreen",}`)
	code, _, stderr := h.psl("", "setup")
	if code != 1 || !strings.Contains(stderr, "not a JSON object, left untouched") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
	if got := h.read(settings); got != `{"tui": "fullscreen",}` {
		t.Errorf("settings were changed: %q", got)
	}
}

func TestCommandLine(t *testing.T) {
	t.Parallel()
	h := newHome(t)
	tests := []struct {
		args   []string
		code   int
		stdout string
		stderr string
	}{
		{[]string{"version"}, 0, "psl ", ""},
		{[]string{"help"}, 0, "Usage:", ""},
		{nil, 2, "", "Usage:"},
		{[]string{"frobnicate"}, 2, "", "unknown command"},
	}
	for _, tt := range tests {
		code, stdout, stderr := h.psl("", tt.args...)
		if code != tt.code || !strings.Contains(stdout, tt.stdout) || !strings.Contains(stderr, tt.stderr) {
			t.Errorf("psl %v: exit %d, stdout %q, stderr %q", tt.args, code, stdout, stderr)
		}
	}
}

// The screen of 2026-10-05: at 66 columns Claude Code draws the status line
// two cells indented and cuts a line that would touch the last column, so it
// showed 63 cells and cut a 64-cell packed line to "Est $2…". A report shaped
// like that session must render with every line inside what the host shows.
func TestRenderFitsTheHostAtNarrowWidth(t *testing.T) {
	t.Parallel()
	h := newHome(t)
	h.columns = 66
	at := func(d time.Duration) float64 { return float64(time.Now().Add(d).Unix()) }
	report := fmt.Sprintf(`{"session_id":"replay","prompt_id":"p1","version":"2.1.289",
"model":{"display_name":"Fable 5"},"effort":{"level":"high"},"thinking":{"enabled":true},
"context_window":{"context_window_size":1000000,"used_percentage":18.7,"total_output_tokens":201,
"current_usage":{"input_tokens":2,"cache_creation_input_tokens":2000,"cache_read_input_tokens":185000}},
"cost":{"total_cost_usd":4.41,"total_duration_ms":300000,"total_api_duration_ms":90000},
"prompt_cache":{"hit_ratio":0.85,"ttl":"1h","expires_at":%.0f,"recache_tokens_if_cold":187000,"cache_write_tokens":162000,"warm":true,"caching_observed":true},
"rate_limits":{"five_hour":{"used_percentage":8,"resets_at":%.0f},"seven_day":{"used_percentage":88,"resets_at":%.0f}}}`,
		at(59*time.Minute+30*time.Second), at(4*time.Hour+23*time.Minute+30*time.Second), at(68*time.Hour+20*time.Minute))

	code, stdout, stderr := h.psl(report, "render")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	plain := sgr.ReplaceAllString(stdout, "")
	for _, want := range []string{
		"██░░░░░░░░ 19% 187k/1.00M 残 813k",
		"⚡ Claude 5h ░░░░░ 8%", "7d ████░ 88%", "Pace ×1.5",
		"Sess $4.41", "Parallel ×0.30",
		"📦 Cache│Hit 85% TTL 1h 残 59m│Save 76%│🧊 Cold 187k│Write 162k",
		"Last new 2 wr 2k rd 185k → out 201",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("the status line lacks %q:\n%s", want, plain)
		}
	}
	const display = 66 - 3 // what the host shows before cutting, measured 2026-10-05
	for line := range strings.Lines(plain) {
		if cells := service.Cells(strings.TrimSuffix(line, "\n")); cells > display {
			t.Errorf("a line of %d cells would be cut by the host: %q", cells, line)
		}
	}
	if got := h.read(".cache/promari-statusline/width.txt"); got != "COLUMNS 63\n" {
		t.Errorf("recorded width %q", got)
	}
}
