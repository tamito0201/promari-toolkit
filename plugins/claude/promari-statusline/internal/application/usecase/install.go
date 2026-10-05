package usecase

import (
	"context"
	"errors"
	"fmt"

	"promari-statusline/internal/domain/repository"
)

// InstallDeps is what installing for the user depends on: the user's settings
// and the installed copy of the binary. The use cases that also reach the
// projects take the ProjectStore on top of these.
type InstallDeps struct {
	Settings repository.SettingsStore
	Binary   repository.BinaryStore
}

// InstallReport says what an installation did, or would do.
type InstallReport struct {
	// Binary is where the copy of the binary is kept.
	Binary string
	// Settings is the settings file that was edited.
	Settings string
	// Command is the statusLine command written to the settings.
	Command string
	// Previous is the command the settings held before, or "".
	Previous string
	// Backup is the copy made of the settings file before it was changed, or "".
	Backup string
	// Changed is false when the settings already held the command.
	Changed bool
	// DryRun is true when nothing was written.
	DryRun bool
}

// Install puts the status line into Claude Code's settings.
//
// A plugin cannot declare a status line, and the directory a plugin is
// installed in changes with every version. So the binary is copied to a path
// that stays the same, and the settings point at that copy.
type Install struct {
	deps InstallDeps
}

// NewInstall returns the use case.
func NewInstall(deps InstallDeps) *Install {
	mustBeWired("InstallDeps", deps)
	return &Install{deps: deps}
}

// Execute installs the status line. With dryRun it only reports what it would do.
func (u *Install) Execute(ctx context.Context, dryRun bool) (InstallReport, error) {
	d := u.deps
	want := wanted(d.Binary.Command())
	report := InstallReport{Binary: d.Binary.Path(), Settings: d.Settings.Path(), Command: want.Command, DryRun: dryRun}

	current, err := d.Settings.StatusLine()
	switch {
	case err == nil:
		report.Previous = current.Command
	case !errors.Is(err, repository.ErrNone):
		return report, fmt.Errorf("read the settings: %w", err)
	}
	report.Changed = err != nil || current != want
	if dryRun {
		return report, nil
	}
	if err := ctx.Err(); err != nil {
		return report, fmt.Errorf("stopped before installing: %w", err)
	}

	if err := d.Binary.Install(); err != nil {
		return report, fmt.Errorf("install the binary: %w", err)
	}
	if !report.Changed {
		return report, nil
	}
	if report.Backup, err = d.Settings.SetStatusLine(want); err != nil {
		return report, fmt.Errorf("write the settings: %w", err)
	}
	return report, nil
}

// UninstallReport says what an uninstallation did.
type UninstallReport struct {
	Settings string
	// Read is false when the user's settings could not be read; nothing was
	// changed then.
	Read bool
	// Removed is false when the settings held no status line of this plugin;
	// someone else's status line is left alone.
	Removed bool
	// Other is the command of a status line that is not this plugin's, or "".
	Other  string
	Backup string
	// Projects are the personal settings files of projects the status line was
	// taken out of (a global installation put it there).
	Projects []string
}

// Uninstall takes the status line out of Claude Code's settings and removes
// the installed copy of the binary.
type Uninstall struct {
	deps     InstallDeps
	projects repository.ProjectStore
}

// NewUninstall returns the use case.
func NewUninstall(deps InstallDeps, projects repository.ProjectStore) *Uninstall {
	mustBeWired("Uninstall", struct {
		InstallDeps
		Projects repository.ProjectStore
	}{deps, projects})
	return &Uninstall{deps: deps, projects: projects}
}

// Execute uninstalls the status line. The report says what was done even when
// an error stops it part way: the user's settings may be changed already.
func (u *Uninstall) Execute(ctx context.Context) (UninstallReport, error) {
	d := u.deps
	report := UninstallReport{Settings: d.Settings.Path()}
	current, err := d.Settings.StatusLine()
	switch {
	case errors.Is(err, repository.ErrNone):
	case err != nil:
		return report, fmt.Errorf("read the settings: %w", err)
	case current.Command != d.Binary.Command():
		report.Other = current.Command
	default:
		if report.Backup, err = d.Settings.RemoveStatusLine(); err != nil {
			return report, fmt.Errorf("write the settings: %w", err)
		}
		report.Removed = true
	}
	report.Read = true
	projects, err := u.unsetProjects(ctx)
	report.Projects = projects
	if err != nil {
		// The binary stays: a project still set to run it keeps working.
		return report, err
	}
	if err := d.Binary.Remove(); err != nil {
		return report, fmt.Errorf("remove the binary: %w", err)
	}
	return report, nil
}

// unsetProjects takes this plugin's status line out of the personal settings of
// every project, so that no project is left running a binary that is gone. A
// status line of another command is left alone. A personal settings file that
// cannot be read stops it: the file may run this binary, which must then stay.
func (u *Uninstall) unsetProjects(ctx context.Context) ([]string, error) {
	dirs, err := u.projects.Projects()
	if err != nil {
		return nil, fmt.Errorf("list the projects: %w", err)
	}
	var done []string
	for _, dir := range dirs {
		if err := ctx.Err(); err != nil {
			return done, fmt.Errorf("stopped before %s: %w", dir, err)
		}
		local := u.projects.Local(dir)
		s, err := local.StatusLine()
		switch {
		case errors.Is(err, repository.ErrNone):
			continue
		case err != nil:
			return done, fmt.Errorf("read %s: %w", local.Path(), err)
		case s.Command != u.deps.Binary.Command():
			continue
		}
		if _, err := local.RemoveStatusLine(); err != nil {
			return done, fmt.Errorf("write %s: %w", local.Path(), err)
		}
		done = append(done, local.Path())
	}
	return done, nil
}

// Refresh brings the installed copy of the binary up to the running one. It
// runs at the start of every session, so an updated plugin reaches the status
// line without another setup. It does nothing for a user who never ran setup:
// the plugin does not install itself.
type Refresh struct {
	binary repository.BinaryStore
}

// NewRefresh returns the use case.
func NewRefresh(binary repository.BinaryStore) *Refresh {
	mustBeWired("Refresh", struct{ Binary repository.BinaryStore }{binary})
	return &Refresh{binary: binary}
}

// Execute refreshes the installed copy and reports whether it was replaced.
// A session start that is stopped leaves the copy as it was.
func (u *Refresh) Execute(ctx context.Context) (replaced bool, err error) {
	same, err := u.binary.InSync()
	switch {
	case errors.Is(err, repository.ErrNone):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("compare the installed binary: %w", err)
	case same:
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("stopped before installing the binary: %w", err)
	}
	if err := u.binary.Install(); err != nil {
		return false, fmt.Errorf("install the binary: %w", err)
	}
	return true, nil
}
