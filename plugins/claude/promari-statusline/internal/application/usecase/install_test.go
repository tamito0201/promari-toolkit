package usecase_test

import (
	"slices"
	"strings"
	"testing"

	"promari-statusline/internal/application/usecase"
	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
)

const ourCommand = "~/.claude/promari-statusline/psl render"

// settings is a settings file in memory.
type settings struct {
	path     string
	line     *model.StatusLineSetting
	readErr  error
	writeErr error
	writes   int
}

func (s *settings) Path() string {
	if s.path != "" {
		return s.path
	}
	return "/h/.claude/settings.json"
}

func (s *settings) StatusLine() (model.StatusLineSetting, error) {
	switch {
	case s.readErr != nil:
		return model.StatusLineSetting{}, s.readErr
	case s.line == nil:
		return model.StatusLineSetting{}, repository.ErrNone
	}
	return *s.line, nil
}

func (s *settings) SetStatusLine(line model.StatusLineSetting) (string, error) {
	if s.writeErr != nil {
		return "", s.writeErr
	}
	s.line = &line
	s.writes++
	return "/h/.claude/settings.json.bak", nil
}

func (s *settings) RemoveStatusLine() (string, error) {
	if s.writeErr != nil {
		return "", s.writeErr
	}
	s.line = nil
	s.writes++
	return "/h/.claude/settings.json.bak", nil
}

// projects are the projects Claude Code has opened, in memory. A project's
// settings files are created empty when first asked for.
type projects struct {
	dirs       []string
	listErr    error
	shared     map[string]*settings
	local      map[string]*settings
	excludeErr error
	excluded   []string
}

func newProjects(dirs ...string) *projects {
	return &projects{dirs: dirs, shared: map[string]*settings{}, local: map[string]*settings{}}
}

func (p *projects) Projects() ([]string, error) { return p.dirs, p.listErr }

func (p *projects) Shared(dir string) repository.SettingsStore {
	return file(p.shared, dir, "settings.json")
}

func (p *projects) Local(dir string) repository.SettingsStore {
	return file(p.local, dir, "settings.local.json")
}

func (p *projects) KeepOutOfGit(dir string) (bool, error) {
	if p.excludeErr != nil {
		return false, p.excludeErr
	}
	p.excluded = append(p.excluded, dir)
	return true, nil
}

func file(files map[string]*settings, dir, name string) *settings {
	s, ok := files[dir]
	if !ok {
		s = &settings{}
		files[dir] = s
	}
	if s.path == "" {
		s.path = dir + "/.claude/" + name
	}
	return s
}

// binary is the installed copy in memory.
type binary struct {
	installed bool
	inSync    bool
	err       error
	installs  int
	removes   int
}

func (b *binary) Command() string { return ourCommand }
func (b *binary) Path() string    { return "/h/.claude/promari-statusline/psl" }

func (b *binary) InSync() (bool, error) {
	switch {
	case b.err != nil:
		return false, b.err
	case !b.installed:
		return false, repository.ErrNone
	}
	return b.inSync, nil
}

func (b *binary) Install() error {
	if b.err != nil {
		return b.err
	}
	b.installed, b.inSync = true, true
	b.installs++
	return nil
}

func (b *binary) Remove() error {
	if b.err != nil {
		return b.err
	}
	b.installed = false
	b.removes++
	return nil
}

func ours() *model.StatusLineSetting {
	return &model.StatusLineSetting{Type: model.CommandType, Command: ourCommand, RefreshInterval: model.RefreshSeconds}
}

func other() *model.StatusLineSetting {
	return &model.StatusLineSetting{Type: model.CommandType, Command: "~/.claude/statusline.sh"}
}

func TestInstall(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		settings *settings
		binary   *binary
		dryRun   bool
		changed  bool
		previous string
		backup   bool
		installs int
		writes   int
		wantErr  string
	}{
		{"into settings without a status line", &settings{}, &binary{}, false, true, "", true, 1, 1, ""},
		{"over another status line, whose command is reported", &settings{line: other()}, &binary{}, false, true, "~/.claude/statusline.sh", true, 1, 1, ""},
		{"already installed: the binary is refreshed, the settings are left", &settings{line: ours()}, &binary{installed: true}, false, false, ourCommand, false, 1, 0, ""},
		{
			"a different padding counts as a change",
			&settings{line: &model.StatusLineSetting{Type: model.CommandType, Command: ourCommand, Padding: 2}},
			&binary{}, false, true, ourCommand, true, 1, 1, "",
		},
		{
			"a status line without the refresh is set up again",
			&settings{line: &model.StatusLineSetting{Type: model.CommandType, Command: ourCommand}},
			&binary{}, false, true, ourCommand, true, 1, 1, "",
		},
		{"a dry run writes nothing", &settings{line: other()}, &binary{}, true, true, "~/.claude/statusline.sh", false, 0, 0, ""},
		{"settings that cannot be read", &settings{readErr: errBroken}, &binary{}, false, false, "", false, 0, 0, "read the settings"},
		{"a binary that cannot be copied leaves the settings alone", &settings{}, &binary{err: errBroken}, false, true, "", false, 0, 0, "install the binary"},
		{"settings that cannot be written", &settings{writeErr: errBroken}, &binary{}, false, true, "", false, 1, 0, "write the settings"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			u := usecase.NewInstall(usecase.InstallDeps{Settings: tt.settings, Binary: tt.binary, Projects: newProjects()})
			got, err := u.Execute(tt.dryRun)
			if (err == nil) != (tt.wantErr == "") || err != nil && !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			if got.Changed != tt.changed || got.Previous != tt.previous || (got.Backup != "") != tt.backup || got.DryRun != tt.dryRun {
				t.Errorf("report = %+v", got)
			}
			if got.Command != ourCommand || got.Binary != tt.binary.Path() || got.Settings != tt.settings.Path() {
				t.Errorf("report names %q, %q, %q", got.Command, got.Binary, got.Settings)
			}
			if tt.binary.installs != tt.installs || tt.settings.writes != tt.writes {
				t.Errorf("installs, writes = %d, %d; want %d, %d", tt.binary.installs, tt.settings.writes, tt.installs, tt.writes)
			}
		})
	}
}

func TestUninstall(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		settings *settings
		binary   *binary
		removed  bool
		other    string
		writes   int
		removes  int
		wantErr  string
	}{
		{"our status line is taken out", &settings{line: ours()}, &binary{installed: true}, true, "", 1, 1, ""},
		{"someone else's status line is left alone", &settings{line: other()}, &binary{installed: true}, false, "~/.claude/statusline.sh", 0, 1, ""},
		{"no status line", &settings{}, &binary{}, false, "", 0, 1, ""},
		{"settings that cannot be read", &settings{readErr: errBroken}, &binary{}, false, "", 0, 0, "read the settings"},
		{"settings that cannot be written", &settings{line: ours(), writeErr: errBroken}, &binary{}, false, "", 0, 0, "write the settings"},
		{"a binary that cannot be removed", &settings{}, &binary{err: errBroken}, false, "", 0, 0, "remove the binary"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			u := usecase.NewUninstall(usecase.InstallDeps{Settings: tt.settings, Binary: tt.binary, Projects: newProjects()})
			got, err := u.Execute()
			if (err == nil) != (tt.wantErr == "") || err != nil && !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			if got.Removed != tt.removed || got.Other != tt.other {
				t.Errorf("report = %+v", got)
			}
			if tt.settings.writes != tt.writes || tt.binary.removes != tt.removes {
				t.Errorf("writes, removes = %d, %d; want %d, %d", tt.settings.writes, tt.binary.removes, tt.writes, tt.removes)
			}
		})
	}
}

func TestRefresh(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		binary   *binary
		replaced bool
		installs int
		wantErr  bool
	}{
		{"never set up: the plugin does not install itself", &binary{}, false, 0, false},
		{"the copy is current", &binary{installed: true, inSync: true}, false, 0, false},
		{"the copy is from another version", &binary{installed: true}, true, 1, false},
		{"the copy cannot be compared", &binary{installed: true, err: errBroken}, false, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			replaced, err := usecase.NewRefresh(tt.binary).Execute()
			if replaced != tt.replaced || (err != nil) != tt.wantErr || tt.binary.installs != tt.installs {
				t.Errorf("Execute() = %v, %v with %d installs; want %v, error %v, %d installs",
					replaced, err, tt.binary.installs, tt.replaced, tt.wantErr, tt.installs)
			}
		})
	}
	t.Run("a copy that differs but cannot be replaced", func(t *testing.T) {
		t.Parallel()
		b := &failingInstall{installed: true}
		if replaced, err := usecase.NewRefresh(b).Execute(); replaced || err == nil {
			t.Errorf("Execute() = %v, %v; want an error", replaced, err)
		}
	})
}

// failingInstall is a copy that can be compared but not replaced.
type failingInstall struct{ binary }

func (*failingInstall) Install() error { return errBroken }

// tools finds the tools a test lists.
type tools []string

func (f tools) Find(name string) (string, bool) {
	return "/usr/bin/" + name, slices.Contains(f, name)
}

// launcher is the launcher's last failure.
type launcher string

func (l launcher) LastError() (string, error) {
	if l == "" {
		return "", repository.ErrNone
	}
	return string(l), nil
}

func TestDiagnose(t *testing.T) {
	t.Parallel()
	type line struct {
		level  model.CheckLevel
		detail string
	}
	allTools := tools{"git", "gh", "ccusage", "nowplaying-cli"}
	tests := []struct {
		name     string
		settings *settings
		binary   *binary
		tools    tools
		launcher launcher
		want     map[string]line
	}{
		{
			"a working installation",
			&settings{line: ours()}, &binary{installed: true, inSync: true}, allTools, "",
			map[string]line{
				"settings /h/.claude/settings.json":                  {model.CheckOK, "statusLine runs " + ourCommand},
				"installed binary /h/.claude/promari-statusline/psl": {model.CheckOK, "is the running binary"},
				"git":            {model.CheckOK, "/usr/bin/git"},
				"terminal width": {model.CheckOK, "120 cells (test), 118 used per line"},
			},
		},
		{
			"set up before the refresh existed",
			&settings{line: &model.StatusLineSetting{Type: model.CommandType, Command: ourCommand}}, &binary{installed: true, inSync: true}, allTools, "",
			map[string]line{
				"settings /h/.claude/settings.json": {model.CheckWarn, "statusLine runs " + ourCommand +
					" but has no refreshInterval: an idle session will not follow the other sessions; run `psl setup`"},
			},
		},
		{
			"never set up",
			&settings{}, &binary{}, nil, "",
			map[string]line{
				"settings /h/.claude/settings.json":                  {model.CheckFail, "no statusLine; run `psl setup`"},
				"installed binary /h/.claude/promari-statusline/psl": {model.CheckFail, "not installed; run `psl setup`"},
				"ccusage": {model.CheckWarn, "not found; Today, Blk, $/h and Est will not be shown (install: npm install -g ccusage)"},
			},
		},
		{
			"another status line, a stale copy and a launcher failure",
			&settings{line: other()}, &binary{installed: true}, allTools, "2026-10-03T00:00:00Z download failed",
			map[string]line{
				"settings /h/.claude/settings.json":                  {model.CheckWarn, "statusLine runs another command: ~/.claude/statusline.sh; run `psl setup` to use this plugin"},
				"installed binary /h/.claude/promari-statusline/psl": {model.CheckWarn, "differs from the running binary; the next session start refreshes it, or run `psl setup`"},
				"launcher": {model.CheckWarn, "2026-10-03T00:00:00Z download failed"},
			},
		},
		{
			"files that cannot be read",
			&settings{readErr: errBroken}, &binary{installed: true, err: errBroken}, allTools, "",
			map[string]line{
				"settings /h/.claude/settings.json":                  {model.CheckFail, "cannot be read: broken"},
				"installed binary /h/.claude/promari-statusline/psl": {model.CheckFail, "cannot be compared: broken"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := newMemory()
			m.cells = 120
			u := usecase.NewDiagnose(usecase.DiagnoseDeps{
				Settings: tt.settings, Binary: tt.binary, Tools: tt.tools, Terminal: m, Launcher: tt.launcher,
				Projects: newProjects(),
			})
			got := map[string]line{}
			for _, check := range u.Execute() {
				got[check.Name] = line{check.Level, check.Detail}
			}
			for name, want := range tt.want {
				if got[name] != want {
					t.Errorf("%s = %+v, want %+v", name, got[name], want)
				}
			}
			if _, has := got["launcher"]; has != (tt.launcher != "") {
				t.Errorf("launcher check present = %v with failure %q", has, tt.launcher)
			}
		})
	}
}
