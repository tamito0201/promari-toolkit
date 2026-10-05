package claude_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/infrastructure/claude"
	"promari-statusline/internal/infrastructure/platform/platformtest"
)

func TestProjectsList(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	for _, dir := range []string{"/w/a", "/w/b", "/h"} {
		sys.Files[dir] = nil // the fake knows a directory by an entry of its own
	}
	sys.Files["/h/.claude.json"] = []byte(`{"projects":{
		"/w/b":{}, "/w/a":{}, "/w/a/":{}, "/h":{}, "relative/dir":{}, "/w/gone":{}
	},"numStartups":3}`)
	got, err := claude.Projects{Sys: sys}.Projects()
	if want := []string{"/w/a", "/w/b"}; err != nil || !slices.Equal(got, want) {
		t.Errorf("Projects() = %v, %v; want %v (sorted, once each, no home, no relative, no missing)", got, err, want)
	}

	t.Run("a configuration directory of its own", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		sys.Env["CLAUDE_CONFIG_DIR"] = "/work-account"
		sys.Files["/w/x"] = nil
		sys.Files["/work-account/.claude.json"] = []byte(`{"projects":{"/w/x":{}}}`)
		if got, err := (claude.Projects{Sys: sys}).Projects(); err != nil || !slices.Equal(got, []string{"/w/x"}) {
			t.Errorf("Projects() = %v, %v", got, err)
		}
	})
	t.Run("no state file", func(t *testing.T) {
		t.Parallel()
		if got, err := (claude.Projects{Sys: platformtest.New(t0)}).Projects(); err != nil || got != nil {
			t.Errorf("Projects() = %v, %v", got, err)
		}
	})
	t.Run("a state file that cannot be read", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		sys.Files["/h/.claude.json"] = []byte("{}")
		sys.ReadErr = map[string]error{"/h/.claude.json": errors.New("denied")}
		if _, err := (claude.Projects{Sys: sys}).Projects(); err == nil {
			t.Error("an unreadable state file was taken for no projects")
		}
	})
}

func TestProjectSettingsFiles(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	p := claude.Projects{Sys: sys}
	if got := p.Shared("/w/a").Path(); got != "/w/a/.claude/settings.json" {
		t.Errorf("Shared = %s", got)
	}
	local := p.Local("/w/a")
	if got := local.Path(); got != "/w/a/.claude/settings.local.json" {
		t.Errorf("Local = %s", got)
	}
	sys.Files["/w/a/.claude/settings.local.json"] = []byte(`{"permissions":{"allow":["Bash(ls)"]}}`)
	want := model.StatusLineSetting{Type: model.CommandType, Command: "psl render", RefreshInterval: model.RefreshSeconds}
	if _, err := local.SetStatusLine(want); err != nil {
		t.Fatal(err)
	}
	data, _ := sys.File("/w/a/.claude/settings.local.json")
	if !strings.Contains(data, `"allow"`) || !strings.Contains(data, `"refreshInterval": 5`) {
		t.Errorf("personal settings = %s", data)
	}
	if got, err := local.StatusLine(); err != nil || got != want {
		t.Errorf("StatusLine() = %+v, %v", got, err)
	}
}

const gitPrefix = "git --no-optional-locks -C /w/repo/app "

func TestKeepOutOfGit(t *testing.T) {
	t.Parallel()
	inRepo := func(sys *platformtest.Fake) {
		sys.Cmds[gitPrefix+"rev-parse --git-dir"] = platformtest.Result{Out: "/w/repo/.git\n"}
		sys.Cmds[gitPrefix+"check-ignore -q .claude/settings.local.json"] = platformtest.Result{Err: errors.New("exit 1")}
		sys.Cmds[gitPrefix+"rev-parse --show-prefix"] = platformtest.Result{Out: "app/\n"}
		sys.Cmds[gitPrefix+"rev-parse --path-format=absolute --git-path info/exclude"] = platformtest.Result{Out: "/w/repo/.git/info/exclude\n"}
	}
	const line = "/app/.claude/settings.local.json\n"

	t.Run("added to the repository's own exclude list", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		inRepo(sys)
		sys.Files["/w/repo/.git/info/exclude"] = []byte("# git ls-files --others --exclude-from=.git/info/exclude\n*.swp")
		added, err := claude.Projects{Sys: sys}.KeepOutOfGit(t.Context(), "/w/repo/app")
		data, _ := sys.File("/w/repo/.git/info/exclude")
		if !added || err != nil || !strings.HasPrefix(data, "# git ls-files") || !strings.Contains(data, "*.swp\n") || !strings.HasSuffix(data, line) {
			t.Errorf("added = %v, %v; exclude =\n%s", added, err, data)
		}
		if sys.Modes["/w/repo/.git/info/exclude"] != 0o644 {
			t.Errorf("mode = %v", sys.Modes["/w/repo/.git/info/exclude"])
		}
	})
	t.Run("a repository without an exclude file", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		inRepo(sys)
		if added, err := (claude.Projects{Sys: sys}).KeepOutOfGit(t.Context(), "/w/repo/app"); !added || err != nil {
			t.Fatalf("added = %v, %v", added, err)
		}
		if data, _ := sys.File("/w/repo/.git/info/exclude"); !strings.HasSuffix(data, line) || strings.HasPrefix(data, "\n") {
			t.Errorf("exclude =\n%s", data)
		}
	})
	t.Run("already ignored", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		inRepo(sys)
		sys.Cmds[gitPrefix+"check-ignore -q .claude/settings.local.json"] = platformtest.Result{}
		if added, err := (claude.Projects{Sys: sys}).KeepOutOfGit(t.Context(), "/w/repo/app"); added || err != nil {
			t.Errorf("added = %v, %v", added, err)
		}
		if _, ok := sys.File("/w/repo/.git/info/exclude"); ok {
			t.Error("the exclude file was written")
		}
	})
	t.Run("not a repository", func(t *testing.T) {
		t.Parallel()
		if added, err := (claude.Projects{Sys: platformtest.New(t0)}).KeepOutOfGit(t.Context(), "/w/repo/app"); added || err != nil {
			t.Errorf("added = %v, %v", added, err)
		}
	})
	for _, c := range []struct {
		name    string
		breakIt func(*platformtest.Fake)
	}{
		{"the prefix cannot be read", func(sys *platformtest.Fake) {
			sys.Cmds[gitPrefix+"rev-parse --show-prefix"] = platformtest.Result{Err: errors.New("exit 128")}
		}},
		{"the exclude file cannot be found", func(sys *platformtest.Fake) {
			sys.Cmds[gitPrefix+"rev-parse --path-format=absolute --git-path info/exclude"] = platformtest.Result{}
		}},
		{"the exclude file cannot be read", func(sys *platformtest.Fake) {
			sys.ReadErr = map[string]error{"/w/repo/.git/info/exclude": errors.New("denied")}
		}},
		{"the exclude file cannot be written", func(sys *platformtest.Fake) { sys.ReadOnly = true }},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			inRepo(sys)
			c.breakIt(sys)
			if _, err := (claude.Projects{Sys: sys}).KeepOutOfGit(t.Context(), "/w/repo/app"); err == nil {
				t.Error("no error")
			}
		})
	}
}
