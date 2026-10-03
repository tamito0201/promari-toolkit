package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"promari-statusline/internal/application/usecase"
	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/interfaces/cli"
)

var errBroken = errors.New("broken")

// fakes answers every use case the command line calls.
type fakes struct {
	err      error
	install  usecase.InstallReport
	global   usecase.GlobalReport
	remove   usecase.UninstallReport
	checks   []model.Check
	dryRun   bool
	refreshs int
}

func (f *fakes) Handle(_ context.Context, in io.Reader, out io.Writer) error {
	if f.err != nil {
		return f.err
	}
	_, err := io.Copy(out, in)
	return err
}

type installer struct{ *fakes }

func (i installer) Execute(dryRun bool) (usecase.InstallReport, error) {
	i.dryRun = dryRun
	report := i.install
	report.DryRun = dryRun
	return report, i.err
}

type globalInstaller struct{ *fakes }

func (g globalInstaller) Execute(dryRun bool) (usecase.GlobalReport, error) {
	g.dryRun = dryRun
	report := g.global
	report.User.DryRun = dryRun
	return report, g.err
}

type uninstaller struct{ *fakes }

func (u uninstaller) Execute() (usecase.UninstallReport, error) { return u.remove, u.err }

type refresher struct{ *fakes }

func (r refresher) Execute() (bool, error) {
	r.refreshs++
	return false, r.err
}

type diagnoser struct{ *fakes }

func (d diagnoser) Execute() []model.Check { return d.checks }

func run(f *fakes, stdin string, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	app := &cli.App{
		Render: f, Install: installer{f}, Global: globalInstaller{f}, Uninstall: uninstaller{f}, Refresh: refresher{f}, Diagnose: diagnoser{f},
		In: strings.NewReader(stdin), Out: &out, Err: &errOut,
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

		{"uninstall that removed the status line", fakes{remove: usecase.UninstallReport{Settings: settings, Removed: true, Backup: settings + ".bak"}}, "", []string{"uninstall"}, 0, []string{"✅ removed statusLine from " + settings, "backup: ", "✅ removed the installed binary"}, nil},
		{"uninstall leaves another status line alone", fakes{remove: usecase.UninstallReport{Settings: settings, Other: "other.sh"}}, "", []string{"uninstall"}, 0, []string{"left alone", "other.sh"}, nil},
		{"uninstall without a status line", fakes{remove: usecase.UninstallReport{Settings: settings}}, "", []string{"uninstall"}, 0, []string{"no statusLine in " + settings}, nil},
		{"uninstall that fails", fakes{err: errBroken}, "", []string{"uninstall"}, 1, nil, []string{"❌ broken"}},

		{
			"doctor with warnings succeeds",
			fakes{checks: []model.Check{{Level: model.CheckOK, Name: "settings", Detail: "fine"}, {Level: model.CheckWarn, Name: "ccusage", Detail: "not found"}}},
			"",
			[]string{"doctor"},
			0,
			[]string{"psl dev", "✅ settings: fine", "⚠️  ccusage: not found"},
			nil,
		},
		{
			"doctor with a failed check fails",
			fakes{checks: []model.Check{{Level: model.CheckFail, Name: "settings", Detail: "no statusLine"}}},
			"",
			[]string{"doctor"},
			1,
			[]string{"❌ settings: no statusLine"},
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
	f := &fakes{remove: usecase.UninstallReport{Settings: settings, Removed: true, Projects: []string{"/w/a/.claude/settings.local.json"}}}
	if _, out, _ := run(f, "", "uninstall"); !strings.Contains(out, "✅ removed statusLine from /w/a/.claude/settings.local.json") {
		t.Errorf("out:\n%s", out)
	}
}
