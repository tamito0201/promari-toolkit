package claude

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform"
	"promari-statusline/pkg/jsonx"
)

const (
	localSettingsFile  = "settings.local.json"
	sharedSettingsFile = "settings.json"
	// excludeMode is the usual mode of .git/info/exclude: readable by all, as
	// git writes it.
	excludeMode fs.FileMode = 0o644
)

// Projects reaches the projects listed in Claude Code's state file.
type Projects struct {
	Sys platform.System
}

var _ repository.ProjectStore = Projects{}

// Projects implements repository.ProjectStore. The list is the keys of
// "projects" in .claude.json: every directory Claude Code was started in.
func (p Projects) Projects() ([]string, error) {
	data, err := p.Sys.ReadFile(StateFile(p.Sys))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the projects: %w", err)
	}
	state, _ := jsonx.Parse(data)
	user := filepath.Clean(ConfigDir(p.Sys))
	var dirs []string
	for dir := range jsonx.Child(state, "projects") {
		// A relative key is not a place; the home's .claude is the user's own settings.
		if !filepath.IsAbs(dir) || filepath.Join(filepath.Clean(dir), ".claude") == user {
			continue
		}
		if _, err := p.Sys.ModTime(dir); err != nil {
			continue // a project that was moved or deleted
		}
		dirs = append(dirs, filepath.Clean(dir))
	}
	slices.Sort(dirs)
	return slices.Compact(dirs), nil
}

// Shared implements repository.ProjectStore.
func (p Projects) Shared(dir string) repository.SettingsReader {
	return Settings{Sys: p.Sys, File: filepath.Join(dir, ".claude", sharedSettingsFile)}
}

// Local implements repository.ProjectStore.
func (p Projects) Local(dir string) repository.SettingsStore {
	return Settings{Sys: p.Sys, File: filepath.Join(dir, ".claude", localSettingsFile)}
}

// KeepOutOfGit implements repository.ProjectStore. git runs without optional
// locks: the status line must never leave an index.lock behind.
func (p Projects) KeepOutOfGit(ctx context.Context, dir string) (bool, error) {
	git := func(args ...string) (string, error) {
		out, err := p.Sys.Run(ctx, platform.Cmd{Name: "git", Args: append([]string{"--no-optional-locks", "-C", dir}, args...)})
		return strings.TrimSpace(out), err
	}
	succeeds := func(args ...string) bool {
		_, err := git(args...)
		return err == nil
	}
	// A directory outside a repository has nothing that could pick the file up,
	// and a file git ignores already needs nothing more.
	file := filepath.Join(".claude", localSettingsFile)
	if !succeeds("rev-parse", "--git-dir") || succeeds("check-ignore", "-q", file) {
		return false, nil
	}
	prefix, err := git("rev-parse", "--show-prefix")
	if err != nil {
		return false, fmt.Errorf("find %s in its repository: %w", dir, err)
	}
	exclude, err := git("rev-parse", "--path-format=absolute", "--git-path", "info/exclude")
	if err != nil || exclude == "" {
		return false, fmt.Errorf("find the exclude file of %s: %w", dir, err)
	}
	current, err := p.Sys.ReadFile(exclude)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("read %s: %w", exclude, err)
	}
	text := string(current)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += "# promari-statusline: the personal settings it writes stay out of git\n/" + filepath.ToSlash(filepath.Join(prefix, file)) + "\n"
	if err := p.Sys.WriteFile(exclude, []byte(text), excludeMode); err != nil {
		return false, fmt.Errorf("write %s: %w", exclude, err)
	}
	return true, nil
}
