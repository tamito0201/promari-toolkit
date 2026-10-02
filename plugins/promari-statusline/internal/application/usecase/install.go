package usecase

import (
	"errors"
	"fmt"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
)

// InstallDeps is what the installation use cases depend on.
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
func NewInstall(deps InstallDeps) *Install { return &Install{deps: deps} }

// Execute installs the status line. With dryRun it only reports what it would do.
func (u *Install) Execute(dryRun bool) (InstallReport, error) {
	d := u.deps
	want := model.StatusLineSetting{Type: model.CommandType, Command: d.Binary.Command()}
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
	// Removed is false when the settings held no status line of this plugin;
	// someone else's status line is left alone.
	Removed bool
	// Other is the command of a status line that is not this plugin's, or "".
	Other  string
	Backup string
}

// Uninstall takes the status line out of Claude Code's settings and removes
// the installed copy of the binary.
type Uninstall struct {
	deps InstallDeps
}

// NewUninstall returns the use case.
func NewUninstall(deps InstallDeps) *Uninstall { return &Uninstall{deps: deps} }

// Execute uninstalls the status line.
func (u *Uninstall) Execute() (UninstallReport, error) {
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
	if err := d.Binary.Remove(); err != nil {
		return report, fmt.Errorf("remove the binary: %w", err)
	}
	return report, nil
}

// Refresh brings the installed copy of the binary up to the running one. It
// runs at the start of every session, so an updated plugin reaches the status
// line without another setup. It does nothing for a user who never ran setup:
// the plugin does not install itself.
type Refresh struct {
	binary repository.BinaryStore
}

// NewRefresh returns the use case.
func NewRefresh(binary repository.BinaryStore) *Refresh { return &Refresh{binary: binary} }

// Execute refreshes the installed copy and reports whether it was replaced.
func (u *Refresh) Execute() (replaced bool, err error) {
	same, err := u.binary.InSync()
	switch {
	case errors.Is(err, repository.ErrNone):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("compare the installed binary: %w", err)
	case same:
		return false, nil
	}
	if err := u.binary.Install(); err != nil {
		return false, fmt.Errorf("install the binary: %w", err)
	}
	return true, nil
}
