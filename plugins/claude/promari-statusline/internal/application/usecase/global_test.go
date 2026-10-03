package usecase_test

import (
	"slices"
	"strings"
	"testing"

	"promari-statusline/internal/application/usecase"
	"promari-statusline/internal/domain/model"
)

func ourLine() *model.StatusLineSetting {
	return &model.StatusLineSetting{Type: model.CommandType, Command: ourCommand, RefreshInterval: model.RefreshSeconds}
}

// world is a machine with projects in every state a global installation meets.
func globalWorld() *projects {
	p := newProjects("/w/other", "/w/stale", "/w/ours", "/w/plain", "/w/broken")
	p.shared["/w/other"] = &settings{line: other()}
	p.local["/w/stale"] = &settings{line: &model.StatusLineSetting{Type: model.CommandType, Command: ourCommand}}
	p.shared["/w/stale"] = &settings{line: other()}
	p.local["/w/ours"] = &settings{line: ourLine()}
	p.shared["/w/ours"] = &settings{line: other()}
	p.shared["/w/broken"] = &settings{readErr: errBroken}
	return p
}

func dirs(fixes []usecase.ProjectFix) []string {
	out := make([]string, 0, len(fixes))
	for _, f := range fixes {
		out = append(out, f.Dir)
	}
	return out
}

func TestInstallGlobal(t *testing.T) {
	t.Parallel()
	p := globalWorld()
	user := &settings{}
	r, err := usecase.NewInstallGlobal(usecase.InstallDeps{Settings: user, Binary: &binary{}, Projects: p}).Execute(false)
	if err != nil {
		t.Fatal(err)
	}
	if user.line == nil || *user.line != *ourLine() {
		t.Errorf("user settings = %+v", user.line)
	}
	if r.Checked != 5 {
		t.Errorf("checked %d projects, want 5", r.Checked)
	}
	if got, want := dirs(r.Fixes), []string{"/w/other", "/w/stale", "/w/broken"}; !slices.Equal(got, want) {
		t.Fatalf("fixes = %v, want %v", got, want)
	}
	other, stale, broken := r.Fixes[0], r.Fixes[1], r.Fixes[2]
	if other.Previous != "~/.claude/statusline.sh" || other.Settings != "/w/other/.claude/settings.local.json" || !other.Excluded || other.Err != "" {
		t.Errorf("other = %+v", other)
	}
	if stale.Previous != ourCommand || stale.Err != "" {
		t.Errorf("stale = %+v (our command without the refresh is set up again)", stale)
	}
	if !strings.Contains(broken.Err, "broken") {
		t.Errorf("broken = %+v", broken)
	}
	for _, dir := range []string{"/w/other", "/w/stale", "/w/ours"} {
		if got := p.local[dir].line; got == nil || *got != *ourLine() {
			t.Errorf("%s personal settings = %+v", dir, got)
		}
	}
	if p.shared["/w/other"].writes != 0 || p.shared["/w/stale"].writes != 0 {
		t.Error("a project's shared settings were written")
	}
	if p.local["/w/ours"].writes != 0 {
		t.Error("a project that already shows the status line was written again")
	}
	if _, touched := p.local["/w/plain"]; touched && p.local["/w/plain"].writes != 0 {
		t.Error("a project without a status line of its own was written")
	}
	if !slices.Equal(p.excluded, []string{"/w/other", "/w/stale"}) {
		t.Errorf("excluded = %v", p.excluded)
	}
}

func TestInstallGlobalDryRunWritesNothing(t *testing.T) {
	t.Parallel()
	p := globalWorld()
	user := &settings{}
	b := &binary{}
	r, err := usecase.NewInstallGlobal(usecase.InstallDeps{Settings: user, Binary: b, Projects: p}).Execute(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Fixes) != 3 || user.writes != 0 || b.installs != 0 || len(p.excluded) != 0 {
		t.Errorf("fixes %d, user writes %d, installs %d, excluded %v", len(r.Fixes), user.writes, b.installs, p.excluded)
	}
	for dir, s := range p.local {
		if s.writes != 0 {
			t.Errorf("%s was written in a dry run", dir)
		}
	}
}

func TestInstallGlobalFailures(t *testing.T) {
	t.Parallel()
	t.Run("the user's installation fails: no project is touched", func(t *testing.T) {
		t.Parallel()
		p := globalWorld()
		_, err := usecase.NewInstallGlobal(usecase.InstallDeps{Settings: &settings{readErr: errBroken}, Binary: &binary{}, Projects: p}).Execute(false)
		if err == nil || len(p.excluded) != 0 {
			t.Errorf("err = %v, excluded = %v", err, p.excluded)
		}
	})
	t.Run("the projects cannot be listed", func(t *testing.T) {
		t.Parallel()
		p := newProjects()
		p.listErr = errBroken
		_, err := usecase.NewInstallGlobal(usecase.InstallDeps{Settings: &settings{}, Binary: &binary{}, Projects: p}).Execute(false)
		if err == nil || !strings.Contains(err.Error(), "list the projects") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("git cannot be kept out: the settings are not written", func(t *testing.T) {
		t.Parallel()
		p := newProjects("/w/other")
		p.shared["/w/other"] = &settings{line: other()}
		p.excludeErr = errBroken
		r, err := usecase.NewInstallGlobal(usecase.InstallDeps{Settings: &settings{}, Binary: &binary{}, Projects: p}).Execute(false)
		if err != nil || len(r.Fixes) != 1 || r.Fixes[0].Err == "" {
			t.Fatalf("r = %+v, err = %v", r, err)
		}
		if p.local["/w/other"].writes != 0 {
			t.Error("the personal settings were written although git could pick them up")
		}
	})
	t.Run("the personal settings cannot be written", func(t *testing.T) {
		t.Parallel()
		p := newProjects("/w/other")
		p.shared["/w/other"] = &settings{line: other()}
		p.local["/w/other"] = &settings{writeErr: errBroken}
		r, _ := usecase.NewInstallGlobal(usecase.InstallDeps{Settings: &settings{}, Binary: &binary{}, Projects: p}).Execute(false)
		if len(r.Fixes) != 1 || !strings.Contains(r.Fixes[0].Err, "broken") {
			t.Errorf("fixes = %+v", r.Fixes)
		}
	})
}

func TestUninstallTakesTheStatusLineOutOfTheProjects(t *testing.T) {
	t.Parallel()
	p := newProjects("/w/ours", "/w/other", "/w/plain")
	p.local["/w/ours"] = &settings{line: ourLine()}
	p.local["/w/other"] = &settings{line: other()}
	r, err := usecase.NewUninstall(usecase.InstallDeps{Settings: &settings{line: ours()}, Binary: &binary{installed: true}, Projects: p}).Execute()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.Projects, []string{"/w/ours/.claude/settings.local.json"}) {
		t.Errorf("projects = %v", r.Projects)
	}
	if p.local["/w/ours"].line != nil || p.local["/w/other"].line == nil {
		t.Errorf("ours = %+v, other = %+v", p.local["/w/ours"].line, p.local["/w/other"].line)
	}

	t.Run("the projects cannot be listed", func(t *testing.T) {
		t.Parallel()
		p := newProjects()
		p.listErr = errBroken
		b := &binary{installed: true}
		if _, err := usecase.NewUninstall(usecase.InstallDeps{Settings: &settings{}, Binary: b, Projects: p}).Execute(); err == nil || b.removes != 0 {
			t.Errorf("err = %v, removes = %d", err, b.removes)
		}
	})
	t.Run("a project's settings cannot be written", func(t *testing.T) {
		t.Parallel()
		p := newProjects("/w/ours")
		p.local["/w/ours"] = &settings{line: ourLine(), writeErr: errBroken}
		if _, err := usecase.NewUninstall(usecase.InstallDeps{Settings: &settings{}, Binary: &binary{installed: true}, Projects: p}).Execute(); err == nil {
			t.Error("a failed write was not reported")
		}
	})
}

func TestDiagnoseProjects(t *testing.T) {
	t.Parallel()
	many := newProjects()
	for _, d := range []string{"/w/a", "/w/b", "/w/c", "/w/d", "/w/e", "/w/f", "/w/g"} {
		many.dirs = append(many.dirs, d)
		many.shared[d] = &settings{line: other()}
	}
	clean := newProjects("/w/ours", "/w/plain")
	clean.local["/w/ours"] = &settings{line: ourLine()}
	broken := newProjects()
	broken.listErr = errBroken
	for _, c := range []struct {
		name     string
		projects *projects
		level    model.CheckLevel
		detail   string
	}{
		{"every project shows this status line", clean, model.CheckOK, "2 projects show this status line"},
		{"projects show another", many, model.CheckWarn, "7 of 7 projects show another status line (/w/a, /w/b, /w/c, /w/d, /w/e and 2 more); run `psl setup --global`"},
		{"the projects cannot be listed", broken, model.CheckWarn, "cannot be listed: broken"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			m := newMemory()
			u := usecase.NewDiagnose(usecase.DiagnoseDeps{
				Settings: &settings{line: ourLine()}, Binary: &binary{installed: true, inSync: true}, Tools: tools{}, Terminal: m, Launcher: launcher(""), Projects: c.projects,
			})
			idx := slices.IndexFunc(u.Execute(), func(ch model.Check) bool { return ch.Name == "projects" })
			if idx < 0 {
				t.Fatal("no projects check")
			}
			got := u.Execute()[idx]
			if got.Level != c.level || got.Detail != c.detail {
				t.Errorf("projects = %v %q\nwant       %v %q", got.Level, got.Detail, c.level, c.detail)
			}
		})
	}
}
