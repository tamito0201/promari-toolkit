package usecase

import (
	"context"
	"errors"

	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/domain/service"
)

// DiagnoseDeps is what Diagnose depends on. Diagnosing only looks: the
// settings and the binary are reached through their reading side.
type DiagnoseDeps struct {
	Settings repository.SettingsReader
	Binary   repository.BinaryInspector
	Tools    repository.ToolFinder
	Terminal repository.Terminal
	Launcher repository.LauncherLog
	Projects repository.ProjectStore
}

// Diagnose checks the installation: whether the settings point at the status
// line, whether the installed copy is current, which optional tools are there,
// and how wide the terminal is. It says what it found; how that is worded and
// what to run about it is the command line's to say.
type Diagnose struct {
	deps DiagnoseDeps
}

// NewDiagnose returns the use case.
func NewDiagnose(deps DiagnoseDeps) *Diagnose {
	mustBeWired("DiagnoseDeps", deps)
	return &Diagnose{deps: deps}
}

// CheckLevel says how a check went.
type CheckLevel uint8

// The levels of a check.
const (
	CheckOK CheckLevel = iota
	CheckWarn
	CheckFail
)

// Finding is what a check found.
type Finding uint8

// The findings, by check.
const (
	SettingsRunsThis   Finding = iota // the settings run this status line
	SettingsMissing                   // the settings have no statusLine
	SettingsUnreadable                // the settings cannot be read
	SettingsRunOther                  // the settings run another command
	SettingsNoRefresh                 // the settings run this one without a refresh interval
	ProjectsShowThis                  // every project shows this status line
	ProjectsShowOther                 // some projects show another one
	ProjectsUnlisted                  // the projects cannot be listed
	BinaryInSync                      // the installed copy is the running binary
	BinaryMissing                     // no copy is installed
	BinaryUnreadable                  // the copy cannot be compared
	BinaryStale                       // the copy differs from the running binary
	LauncherFailed                    // the launcher recorded a failure
	LauncherUnreadable                // the launcher's record cannot be read
	ToolFound                         // an optional tool is on PATH
	ToolMissing                       // an optional tool is not
	TerminalMeasured                  // the terminal's width
)

// Check is one finding of a diagnosis, with what it was found about.
type Check struct {
	Level   CheckLevel
	Finding Finding
	// Subject is what was checked: a file, a tool's name.
	Subject string
	// Command is the statusLine command the settings run.
	Command string
	// Detail is a path found, or the launcher's record.
	Detail string
	// Err is why the subject could not be read.
	Err error
	// Projects counts the projects, and Others are those that show another
	// status line.
	Projects int
	Others   []string
	// Cells, Source and Budget describe the terminal.
	Cells  int
	Source string
	Budget int
}

// OptionalTools returns the tools the status line uses when they are there;
// their absence only hides chips.
func OptionalTools() []string { return []string{"git", "gh", "ccusage", "nowplaying-cli"} }

// Execute returns the checks, in the order they should be read. A stop (ctx)
// leaves out the checks not made yet.
func (u *Diagnose) Execute(ctx context.Context) []Check {
	checks := []Check{u.settings(), u.projects(ctx), u.binary()}
	switch failure, err := u.deps.Launcher.LastError(); {
	case err == nil:
		checks = append(checks, Check{Level: CheckWarn, Finding: LauncherFailed, Subject: "launcher", Detail: failure})
	case !errors.Is(err, repository.ErrNone):
		checks = append(checks, Check{Level: CheckWarn, Finding: LauncherUnreadable, Subject: "launcher", Err: err})
	}
	for _, tool := range OptionalTools() {
		if path, ok := u.deps.Tools.Find(tool); ok {
			checks = append(checks, Check{Level: CheckOK, Finding: ToolFound, Subject: tool, Detail: path})
		} else {
			checks = append(checks, Check{Level: CheckWarn, Finding: ToolMissing, Subject: tool})
		}
	}
	cells, source := u.deps.Terminal.Width()
	return append(checks, Check{Level: CheckOK, Finding: TerminalMeasured, Subject: "terminal width", Cells: cells, Source: source, Budget: service.Budget(cells)})
}

// projects checks that no project hides this status line behind one of its
// own: a project's settings take precedence over the user's.
func (u *Diagnose) projects(ctx context.Context) Check {
	check := Check{Subject: "projects"}
	dirs, err := u.deps.Projects.Projects()
	if err != nil {
		check.Level, check.Finding, check.Err = CheckWarn, ProjectsUnlisted, err
		return check
	}
	want := wanted(u.deps.Binary.Command())
	for _, dir := range dirs {
		if ctx.Err() != nil {
			break
		}
		shown, err := shownIn(u.deps.Projects, dir)
		if s, ok := shown.Get(); err != nil || (ok && s != want) {
			check.Others = append(check.Others, dir)
		}
	}
	check.Projects = len(dirs)
	if len(check.Others) == 0 {
		check.Finding = ProjectsShowThis
		return check
	}
	check.Level, check.Finding = CheckWarn, ProjectsShowOther
	return check
}

func (u *Diagnose) settings() Check {
	d := u.deps
	check := Check{Subject: d.Settings.Path()}
	current, err := d.Settings.StatusLine()
	check.Command = current.Command
	switch {
	case errors.Is(err, repository.ErrNone):
		check.Level, check.Finding = CheckFail, SettingsMissing
	case err != nil:
		check.Level, check.Finding, check.Err = CheckFail, SettingsUnreadable, err
	case current.Command != d.Binary.Command():
		check.Level, check.Finding = CheckWarn, SettingsRunOther
	case current.RefreshInterval <= 0:
		check.Level, check.Finding = CheckWarn, SettingsNoRefresh
	default:
		check.Finding = SettingsRunsThis
	}
	return check
}

func (u *Diagnose) binary() Check {
	d := u.deps
	check := Check{Subject: d.Binary.Path()}
	same, err := d.Binary.InSync()
	switch {
	case errors.Is(err, repository.ErrNone):
		check.Level, check.Finding = CheckFail, BinaryMissing
	case err != nil:
		check.Level, check.Finding, check.Err = CheckFail, BinaryUnreadable, err
	case !same:
		check.Level, check.Finding = CheckWarn, BinaryStale
	default:
		check.Finding = BinaryInSync
	}
	return check
}
