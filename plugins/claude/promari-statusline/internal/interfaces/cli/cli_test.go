package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"promari-statusline/internal/application/usecase"
	"promari-statusline/internal/interfaces/cli"
)

var errBroken = errors.New("broken")

// fakes answers every use case the command line calls.
type fakes struct {
	err      error
	install  usecase.InstallReport
	global   usecase.GlobalReport
	remove   usecase.UninstallReport
	checks   []usecase.Check
	dryRun   bool
	refreshs int
	addr     string
}

func (f *fakes) Handle(_ context.Context, in io.Reader, out io.Writer) error {
	if f.err != nil {
		return f.err
	}
	_, err := io.Copy(out, in)
	return err
}

type installer struct{ *fakes }

func (i installer) Execute(_ context.Context, dryRun bool) (usecase.InstallReport, error) {
	i.dryRun = dryRun
	report := i.install
	report.DryRun = dryRun
	return report, i.err
}

type globalInstaller struct{ *fakes }

func (g globalInstaller) Execute(_ context.Context, dryRun bool) (usecase.GlobalReport, error) {
	g.dryRun = dryRun
	report := g.global
	report.User.DryRun = dryRun
	return report, g.err
}

type uninstaller struct{ *fakes }

func (u uninstaller) Execute(context.Context) (usecase.UninstallReport, error) {
	return u.remove, u.err
}

type refresher struct{ *fakes }

func (r refresher) Execute(context.Context) (bool, error) {
	r.refreshs++
	return false, r.err
}

type dashboarder struct{ *fakes }

func (d dashboarder) Serve(_ context.Context, addr string, out io.Writer) error {
	d.addr = addr
	_, _ = io.WriteString(out, "serving "+addr+"\n")
	return d.err
}

type diagnoser struct{ *fakes }

func (d diagnoser) Execute(context.Context) []usecase.Check { return d.checks }

func run(f *fakes, stdin string, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	app := &cli.App{
		Render: f, Install: installer{f}, Global: globalInstaller{f}, Uninstall: uninstaller{f}, Refresh: refresher{f}, Diagnose: diagnoser{f},
		Dashboard: dashboarder{f},
		In:        strings.NewReader(stdin), Out: &out, Err: &errOut,
	}
	code = app.Run(context.Background(), args)
	return code, out.String(), errOut.String()
}

const (
	settings = "/h/.claude/settings.json"
	command  = "~/.claude/promari-statusline/psl render"
	binary   = "/h/.claude/promari-statusline/psl"
)

func TestRun(t *testing.T) {
	t.Parallel()
	report := usecase.InstallReport{Binary: binary, Settings: settings, Command: command}
	tests := []struct {
		name   string
		fakes  fakes
		stdin  string
		args   []string
		code   int
		stdout []string
		stderr []string
	}{
		{"no command prints the usage", fakes{}, "", nil, 2, nil, []string{"Usage:", "psl render"}},
		{"an unknown command", fakes{}, "", []string{"frobnicate"}, 2, nil, []string{`unknown command "frobnicate"`, "Usage:"}},
		{"help", fakes{}, "", []string{"help"}, 0, []string{"Usage:", "psl doctor"}, nil},
		{"--help", fakes{}, "", []string{"--help"}, 0, []string{"Usage:"}, nil},
		{"version", fakes{}, "", []string{"version"}, 0, []string{"psl dev"}, nil},
		{"--version", fakes{}, "", []string{"--version"}, 0, []string{"psl dev"}, nil},

		{"render passes standard input to the use case and its lines to standard output", fakes{}, "the report", []string{"render"}, 0, []string{"the report"}, nil},
		{"render that cannot write", fakes{err: errBroken}, "", []string{"render"}, 1, nil, []string{"❌ broken"}},

		{
			"setup that changed the settings",
			fakes{install: usecase.InstallReport{Binary: binary, Settings: settings, Command: command, Changed: true, Previous: "~/.claude/statusline.sh", Backup: settings + ".bak"}},
			"",
			[]string{"setup"},
			0,
			[]string{"✅ installed: " + binary, "✅ statusLine in " + settings + " now runs: " + command, "previous command: ~/.claude/statusline.sh", "backup: " + settings + ".bak", "takes precedence"},
			nil,
		},
		{"setup with nothing to change", fakes{install: report}, "", []string{"setup"}, 0, []string{"✅ installed", "settings unchanged: " + settings + " already runs " + command}, nil},
		{
			"setup --dry-run says what it would do",
			fakes{install: usecase.InstallReport{Binary: binary, Settings: settings, Command: command, Changed: true}},
			"",
			[]string{"setup", "--dry-run"},
			0,
			[]string{"would install: " + binary, "would set statusLine in " + settings + " to: " + command},
			nil,
		},
		{"setup that fails", fakes{err: errBroken}, "", []string{"setup"}, 1, nil, []string{"❌ broken"}},
		{"setup with an unknown flag", fakes{}, "", []string{"setup", "--force"}, 2, nil, []string{"flag provided but not defined"}},

		{"uninstall that removed the status line", fakes{remove: usecase.UninstallReport{Settings: settings, Read: true, Removed: true, Backup: settings + ".bak"}}, "", []string{"uninstall"}, 0, []string{"✅ removed statusLine from " + settings, "backup: ", "✅ removed the installed binary"}, nil},
		{"uninstall leaves another status line alone", fakes{remove: usecase.UninstallReport{Settings: settings, Read: true, Other: "other.sh"}}, "", []string{"uninstall"}, 0, []string{"left alone", "other.sh"}, nil},
		{"uninstall without a status line", fakes{remove: usecase.UninstallReport{Settings: settings, Read: true}}, "", []string{"uninstall"}, 0, []string{"no statusLine in " + settings}, nil},
		{"uninstall that fails before changing anything", fakes{err: errBroken}, "", []string{"uninstall"}, 1, nil, []string{"❌ broken"}},
		{
			"uninstall that fails part way says what it did and keeps the binary",
			fakes{err: errBroken, remove: usecase.UninstallReport{Settings: settings, Read: true, Removed: true, Backup: settings + ".bak"}},
			"",
			[]string{"uninstall"},
			1,
			[]string{"✅ removed statusLine from " + settings, "backup: " + settings + ".bak", "the installed binary is kept"},
			[]string{"❌ broken"},
		},

		{
			"doctor with warnings succeeds",
			fakes{checks: []usecase.Check{
				{Level: usecase.CheckOK, Finding: usecase.SettingsRunsThis, Subject: settings, Command: command},
				{Level: usecase.CheckWarn, Finding: usecase.ToolMissing, Subject: "ccusage"},
			}},
			"",
			[]string{"doctor"},
			0,
			[]string{"psl dev", "✅ settings " + settings + ": statusLine runs " + command, "⚠️  ccusage: not found"},
			nil,
		},
		{
			"doctor with a failed check fails",
			fakes{checks: []usecase.Check{{Level: usecase.CheckFail, Finding: usecase.SettingsMissing, Subject: settings}}},
			"",
			[]string{"doctor"},
			1,
			[]string{"❌ settings " + settings + ": no statusLine"},
			nil,
		},

		{"a hook is silent", fakes{}, "{}", []string{"hook", "SessionStart"}, 0, nil, nil},
		{"a hook never fails, whatever went wrong", fakes{err: errBroken}, "", []string{"hook", "SessionStart"}, 0, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := tt.fakes
			code, stdout, stderr := run(&f, tt.stdin, tt.args...)
			if code != tt.code {
				t.Errorf("exit code = %d, want %d (stdout %q, stderr %q)", code, tt.code, stdout, stderr)
			}
			for _, want := range tt.stdout {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout lacks %q:\n%s", want, stdout)
				}
			}
			for _, want := range tt.stderr {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr lacks %q:\n%s", want, stderr)
				}
			}
			if tt.stdout == nil && stdout != "" {
				t.Errorf("unexpected stdout: %q", stdout)
			}
			if tt.stderr == nil && stderr != "" {
				t.Errorf("unexpected stderr: %q", stderr)
			}
		})
	}
}

func TestHookRefreshes(t *testing.T) {
	t.Parallel()
	f := &fakes{}
	run(f, "", "hook", "SessionStart")
	if f.refreshs != 1 {
		t.Errorf("the hook refreshed %d times, want once", f.refreshs)
	}
}

func TestSetupPassesDryRun(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		args []string
		want bool
	}{{[]string{"setup"}, false}, {[]string{"setup", "--dry-run"}, true}} {
		f := &fakes{}
		run(f, "", tt.args...)
		if f.dryRun != tt.want {
			t.Errorf("%v: dryRun = %v, want %v", tt.args, f.dryRun, tt.want)
		}
	}
}

func TestSetupGlobal(t *testing.T) {
	t.Parallel()
	report := usecase.GlobalReport{
		User:    usecase.InstallReport{Binary: binary, Settings: settings, Command: command, Changed: true},
		Checked: 4,
		Fixes: []usecase.ProjectFix{
			{Dir: "/w/a", Previous: "~/.claude/statusline.sh", Settings: "/w/a/.claude/settings.local.json", Excluded: true, Backup: "/w/a/.claude/settings.local.json.bak"},
			{Dir: "/w/b", Previous: "~/.claude/statusline.sh", Settings: "/w/b/.claude/settings.local.json"},
		},
	}
	t.Run("apply", func(t *testing.T) {
		t.Parallel()
		f := &fakes{global: report}
		code, out, errOut := run(f, "", "setup", "--global")
		for _, want := range []string{
			"✅ statusLine in /w/a/.claude/settings.local.json now runs: " + command + " (it showed: ~/.claude/statusline.sh)",
			"added to the repository's .git/info/exclude",
			"backup: /w/a/.claude/settings.local.json.bak",
			"projects: 4 checked, 2 showing another status line",
			"shared .claude/settings.json is never changed",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("missing %q in:\n%s", want, out)
			}
		}
		if code != 0 || errOut != "" || f.dryRun {
			t.Errorf("code %d, stderr %q, dryRun %v", code, errOut, f.dryRun)
		}
	})
	t.Run("dry run", func(t *testing.T) {
		t.Parallel()
		f := &fakes{global: report}
		code, out, _ := run(f, "", "setup", "--global", "--dry-run")
		if code != 0 || !f.dryRun || !strings.Contains(out, "would set statusLine in /w/b/.claude/settings.local.json (it shows: ~/.claude/statusline.sh)") {
			t.Errorf("code %d, dryRun %v:\n%s", code, f.dryRun, out)
		}
	})
	t.Run("a project that could not be set up fails the command", func(t *testing.T) {
		t.Parallel()
		broken := report
		broken.Fixes = []usecase.ProjectFix{{Dir: "/w/c", Err: "broken"}}
		code, _, errOut := run(&fakes{global: broken}, "", "setup", "--global")
		if code != 1 || !strings.Contains(errOut, "/w/c: broken") {
			t.Errorf("code %d, stderr %q", code, errOut)
		}
	})
	t.Run("the installation fails", func(t *testing.T) {
		t.Parallel()
		code, _, errOut := run(&fakes{err: errors.New("no settings")}, "", "setup", "--global")
		if code != 1 || !strings.Contains(errOut, "no settings") {
			t.Errorf("code %d, stderr %q", code, errOut)
		}
	})
}

func TestUninstallReportsTheProjects(t *testing.T) {
	t.Parallel()
	f := &fakes{remove: usecase.UninstallReport{Settings: settings, Read: true, Removed: true, Projects: []string{"/w/a/.claude/settings.local.json"}}}
	if _, out, _ := run(f, "", "uninstall"); !strings.Contains(out, "✅ removed statusLine from /w/a/.claude/settings.local.json") {
		t.Errorf("out:\n%s", out)
	}
}

// TestDoctorWords covers the wording of every finding: what the use case found
// is said here, with what to run about it.
func TestDoctorWords(t *testing.T) {
	t.Parallel()
	others := []string{"/w/a", "/w/b", "/w/c", "/w/d", "/w/e", "/w/f", "/w/g"}
	for _, c := range []struct {
		check usecase.Check
		want  string
	}{
		{usecase.Check{Finding: usecase.SettingsRunsThis, Subject: settings, Command: command}, "settings " + settings + ": statusLine runs " + command},
		{usecase.Check{Finding: usecase.SettingsMissing, Subject: settings}, "settings " + settings + ": no statusLine; run `psl setup`"},
		{usecase.Check{Finding: usecase.SettingsUnreadable, Subject: settings, Err: errBroken}, "settings " + settings + ": cannot be read: broken"},
		{usecase.Check{Finding: usecase.SettingsRunOther, Subject: settings, Command: "~/.claude/statusline.sh"}, "statusLine runs another command: ~/.claude/statusline.sh; run `psl setup` to use this plugin"},
		{usecase.Check{Finding: usecase.SettingsNoRefresh, Subject: settings, Command: command}, "statusLine runs " + command + " but has no refreshInterval: an idle session will not follow the other sessions; run `psl setup`"},
		{usecase.Check{Finding: usecase.ProjectsShowThis, Subject: "projects", Projects: 2}, "projects: 2 projects show this status line"},
		{usecase.Check{Finding: usecase.ProjectsShowOther, Subject: "projects", Projects: 7, Others: others}, "projects: 7 of 7 projects show another status line (/w/a, /w/b, /w/c, /w/d, /w/e and 2 more); run `psl setup --global`"},
		{usecase.Check{Finding: usecase.ProjectsUnlisted, Subject: "projects", Err: errBroken}, "projects: cannot be listed: broken"},
		{usecase.Check{Finding: usecase.BinaryInSync, Subject: binary}, "installed binary " + binary + ": is the running binary"},
		{usecase.Check{Finding: usecase.BinaryMissing, Subject: binary}, "installed binary " + binary + ": not installed; run `psl setup`"},
		{usecase.Check{Finding: usecase.BinaryUnreadable, Subject: binary, Err: errBroken}, "installed binary " + binary + ": cannot be compared: broken"},
		{usecase.Check{Finding: usecase.BinaryStale, Subject: binary}, "differs from the running binary; the next session start refreshes it, or run `psl setup`"},
		{usecase.Check{Finding: usecase.LauncherFailed, Subject: "launcher", Detail: "2026-10-03T00:00:00Z download failed"}, "launcher: 2026-10-03T00:00:00Z download failed"},
		{usecase.Check{Finding: usecase.LauncherUnreadable, Subject: "launcher", Err: errBroken}, "launcher: its record cannot be read: broken"},
		{usecase.Check{Finding: usecase.ToolFound, Subject: "git", Detail: "/usr/bin/git"}, "git: /usr/bin/git"},
		{usecase.Check{Finding: usecase.ToolMissing, Subject: "ccusage"}, "ccusage: not found; Today, Blk, $/h and Est will not be shown (install: npm install -g ccusage)"},
		{usecase.Check{Finding: usecase.TerminalMeasured, Subject: "terminal width", Cells: 120, Source: "test", Budget: 118}, "terminal width: 120 cells (test), 118 used per line"},
	} {
		_, out, _ := run(&fakes{checks: []usecase.Check{c.check}}, "", "doctor")
		if !strings.Contains(out, c.want) {
			t.Errorf("finding %d: lacks %q in:\n%s", c.check.Finding, c.want, out)
		}
	}
	// A tool or a finding the words do not know is still named.
	if _, out, _ := run(&fakes{checks: []usecase.Check{{Finding: usecase.ToolMissing, Subject: "jq"}}}, "", "doctor"); !strings.Contains(out, "jq: not found") {
		t.Errorf("an unknown tool:\n%s", out)
	}
	if _, out, _ := run(&fakes{checks: []usecase.Check{{Finding: usecase.Finding(255), Subject: "something"}}}, "", "doctor"); !strings.Contains(out, "something: ") {
		t.Errorf("an unknown finding:\n%s", out)
	}
	// Every optional tool the use case looks for has its words.
	for _, tool := range usecase.OptionalTools() {
		_, out, _ := run(&fakes{checks: []usecase.Check{{Finding: usecase.ToolMissing, Subject: tool}}}, "", "doctor")
		if !strings.Contains(out, "will not be shown (install: ") {
			t.Errorf("no words for the missing tool %s:\n%s", tool, out)
		}
	}
}

func TestDashboard(t *testing.T) {
	t.Parallel()
	t.Run("it listens on the loopback address by default", func(t *testing.T) {
		t.Parallel()
		f := &fakes{}
		code, out, _ := run(f, "", "dashboard")
		if code != 0 || f.addr != "127.0.0.1:4646" || out != "serving 127.0.0.1:4646\n" {
			t.Errorf("code %d, addr %q, out %q", code, f.addr, out)
		}
	})
	t.Run("--addr chooses another address", func(t *testing.T) {
		t.Parallel()
		f := &fakes{}
		if code, _, _ := run(f, "", "dashboard", "--addr", "127.0.0.1:5000"); code != 0 || f.addr != "127.0.0.1:5000" {
			t.Errorf("code %d, addr %q", code, f.addr)
		}
	})
	t.Run("an unknown flag is a usage error", func(t *testing.T) {
		t.Parallel()
		f := &fakes{}
		if code, _, stderr := run(f, "", "dashboard", "--port", "1"); code != 2 || f.addr != "" || !strings.Contains(stderr, "-port") {
			t.Errorf("code %d, addr %q, stderr %q", code, f.addr, stderr)
		}
	})
	t.Run("a server that fails is an error", func(t *testing.T) {
		t.Parallel()
		if code, _, stderr := run(&fakes{err: errBroken}, "", "dashboard"); code != 1 || !strings.Contains(stderr, "broken") {
			t.Errorf("code %d, stderr %q", code, stderr)
		}
	})
	t.Run("the help names the command", func(t *testing.T) {
		t.Parallel()
		if _, out, _ := run(&fakes{}, "", "help"); !strings.Contains(out, "psl dashboard") {
			t.Errorf("help lacks the dashboard:\n%s", out)
		}
	})
}
