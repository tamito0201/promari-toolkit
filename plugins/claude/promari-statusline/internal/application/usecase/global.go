package usecase

import (
	"context"
	"errors"
	"fmt"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
)

// ProjectFix is what a global installation did, or would do, in one project.
type ProjectFix struct {
	Dir string
	// Previous is the command the project showed instead of this status line.
	Previous string
	// Settings is the personal settings file written.
	Settings string
	Backup   string
	// Excluded is true when the personal settings file was added to the
	// repository's own exclude list, so that git does not pick it up.
	Excluded bool
	// Err says why the project was left as it is, or "".
	Err string
}

// GlobalReport says what a global installation did, or would do.
type GlobalReport struct {
	User InstallReport
	// Checked is the number of projects looked at.
	Checked int
	// Fixes are the projects that showed another status line.
	Fixes []ProjectFix
}

// InstallGlobal puts the status line on every terminal: into the user's
// settings, and into the personal settings of every project whose own settings
// show another status line. A project's settings take precedence over the
// user's, so without this a project that once set a status line keeps it.
// The shared settings of a project are never written: they belong to the
// repository and its other users.
type InstallGlobal struct {
	install  *Install
	projects repository.ProjectStore
	command  func() string
}

// NewInstallGlobal returns the use case.
func NewInstallGlobal(deps InstallDeps, projects repository.ProjectStore) *InstallGlobal {
	mustBeWired("InstallGlobal", struct{ Projects repository.ProjectStore }{projects})
	return &InstallGlobal{install: NewInstall(deps), projects: projects, command: deps.Binary.Command}
}

// Execute installs the status line everywhere. With dryRun it only reports
// what it would do. A stop (ctx) between two projects returns what was done.
func (u *InstallGlobal) Execute(ctx context.Context, dryRun bool) (GlobalReport, error) {
	user, err := u.install.Execute(ctx, dryRun)
	report := GlobalReport{User: user}
	if err != nil {
		return report, err
	}
	dirs, err := u.projects.Projects()
	if err != nil {
		return report, fmt.Errorf("list the projects: %w", err)
	}
	want := wanted(u.command())
	report.Checked = len(dirs)
	for _, dir := range dirs {
		if err := ctx.Err(); err != nil {
			return report, fmt.Errorf("stopped before %s: %w", dir, err)
		}
		shown, err := shownIn(u.projects, dir)
		if err != nil {
			report.Fixes = append(report.Fixes, ProjectFix{Dir: dir, Err: err.Error()})
			continue
		}
		// No status line of its own: the user's setting applies. The same as ours: nothing to do.
		s, ok := shown.Get()
		if !ok || s == want {
			continue
		}
		fix := ProjectFix{Dir: dir, Previous: s.Command, Settings: u.projects.Local(dir).Path()}
		if !dryRun {
			u.apply(ctx, dir, want, &fix)
		}
		report.Fixes = append(report.Fixes, fix)
	}
	return report, nil
}

// projectSettings is what shownIn reads of the projects.
type projectSettings interface {
	Local(dir string) repository.SettingsStore
	Shared(dir string) repository.SettingsReader
}

// shownIn returns the status line a project's own settings set: the personal
// file's, or else the shared file's; absent when neither sets one.
func shownIn(projects projectSettings, dir string) (model.Optional[model.StatusLineSetting], error) {
	for _, store := range []repository.SettingsReader{projects.Local(dir), projects.Shared(dir)} {
		s, err := store.StatusLine()
		switch {
		case err == nil:
			return model.Some(s), nil
		case !errors.Is(err, repository.ErrNone):
			return model.Optional[model.StatusLineSetting]{}, err
		}
	}
	return model.Optional[model.StatusLineSetting]{}, nil
}

// apply writes the status line into a project's personal settings, after
// making sure git will not pick that file up.
func (u *InstallGlobal) apply(ctx context.Context, dir string, want model.StatusLineSetting, fix *ProjectFix) {
	excluded, err := u.projects.KeepOutOfGit(ctx, dir)
	if err != nil {
		fix.Err = err.Error()
		return
	}
	fix.Excluded = excluded
	if fix.Backup, err = u.projects.Local(dir).SetStatusLine(want); err != nil {
		fix.Err = err.Error()
	}
}

// wanted is the statusLine entry the plugin installs.
func wanted(command string) model.StatusLineSetting {
	return model.StatusLineSetting{Type: model.CommandType, Command: command, RefreshInterval: model.RefreshSeconds}
}
