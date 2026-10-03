package usecase

import (
	"errors"
	"strconv"
	"strings"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/domain/service"
)

// DiagnoseDeps is what Diagnose depends on.
type DiagnoseDeps struct {
	Settings repository.SettingsStore
	Binary   repository.BinaryStore
	Tools    repository.ToolFinder
	Terminal repository.Terminal
	Launcher repository.LauncherLog
	Projects repository.ProjectStore
}

// Diagnose checks the installation: whether the settings point at the status
// line, whether the installed copy is current, which optional tools are there,
// and how wide the terminal is.
type Diagnose struct {
	deps DiagnoseDeps
}

// NewDiagnose returns the use case.
func NewDiagnose(deps DiagnoseDeps) *Diagnose { return &Diagnose{deps: deps} }

// optionalTool is a tool whose absence only hides chips.
type optionalTool struct {
	name  string
	chips string
	// install says how to get the tool: a command, or where to find one.
	install string
}

// optionalTools lists the tools the status line uses when they are there.
func optionalTools() []optionalTool {
	return []optionalTool{
		{"git", "🌿 Git", "https://git-scm.com/downloads"},
		{"gh", "🔀 PR", "https://cli.github.com"},
		{"ccusage", "Today, Blk, $/h and Est", "npm install -g ccusage"},
		{"nowplaying-cli", "🎵 Music", "brew install nowplaying-cli (macOS)"},
	}
}

// Execute returns the checks, in the order they should be read.
func (u *Diagnose) Execute() []model.Check {
	checks := []model.Check{u.settings(), u.projects(), u.binary()}
	if failure, err := u.deps.Launcher.LastError(); err == nil {
		checks = append(checks, model.Check{Level: model.CheckWarn, Name: "launcher", Detail: failure})
	}
	for _, tool := range optionalTools() {
		path, ok := u.deps.Tools.Find(tool.name)
		if ok {
			checks = append(checks, model.Check{Level: model.CheckOK, Name: tool.name, Detail: path})
			continue
		}
		checks = append(checks, model.Check{Level: model.CheckWarn, Name: tool.name, Detail: "not found; " + tool.chips + " will not be shown (install: " + tool.install + ")"})
	}
	cells, source := u.deps.Terminal.Width()
	return append(checks, model.Check{
		Level:  model.CheckOK,
		Name:   "terminal width",
		Detail: strconv.Itoa(cells) + " cells (" + source + "), " + strconv.Itoa(service.Budget(cells)) + " used per line",
	})
}

// projectsShown is how many projects that show another status line are named
// in the check; the rest are counted.
const projectsShown = 5

// projects checks that no project hides this status line behind one of its
// own: a project's settings take precedence over the user's.
func (u *Diagnose) projects() model.Check {
	check := model.Check{Name: "projects"}
	dirs, err := u.deps.Projects.Projects()
	if err != nil {
		check.Level, check.Detail = model.CheckWarn, "cannot be listed: "+err.Error()
		return check
	}
	want := wanted(u.deps.Binary.Command())
	var others []string
	for _, dir := range dirs {
		shown, err := shownIn(u.deps.Projects, dir)
		if s, ok := shown.Get(); err != nil || (ok && s != want) {
			others = append(others, dir)
		}
	}
	if len(others) == 0 {
		check.Detail = strconv.Itoa(len(dirs)) + " projects show this status line"
		return check
	}
	named := others[:min(len(others), projectsShown)]
	more := ""
	if len(others) > len(named) {
		more = " and " + strconv.Itoa(len(others)-len(named)) + " more"
	}
	check.Level = model.CheckWarn
	check.Detail = strconv.Itoa(len(others)) + " of " + strconv.Itoa(len(dirs)) + " projects show another status line (" +
		strings.Join(named, ", ") + more + "); run `psl setup --global`"
	return check
}

func (u *Diagnose) settings() model.Check {
	d := u.deps
	check := model.Check{Name: "settings " + d.Settings.Path()}
	current, err := d.Settings.StatusLine()
	switch {
	case errors.Is(err, repository.ErrNone):
		check.Level, check.Detail = model.CheckFail, "no statusLine; run `psl setup`"
	case err != nil:
		check.Level, check.Detail = model.CheckFail, "cannot be read: "+err.Error()
	case current.Command != d.Binary.Command():
		check.Level, check.Detail = model.CheckWarn, "statusLine runs another command: "+current.Command+"; run `psl setup` to use this plugin"
	case current.RefreshInterval <= 0:
		check.Level, check.Detail = model.CheckWarn, "statusLine runs "+current.Command+
			" but has no refreshInterval: an idle session will not follow the other sessions; run `psl setup`"
	default:
		check.Detail = "statusLine runs " + current.Command
	}
	return check
}

func (u *Diagnose) binary() model.Check {
	d := u.deps
	check := model.Check{Name: "installed binary " + d.Binary.Path()}
	same, err := d.Binary.InSync()
	switch {
	case errors.Is(err, repository.ErrNone):
		check.Level, check.Detail = model.CheckFail, "not installed; run `psl setup`"
	case err != nil:
		check.Level, check.Detail = model.CheckFail, "cannot be compared: "+err.Error()
	case !same:
		check.Level, check.Detail = model.CheckWarn, "differs from the running binary; the next session start refreshes it, or run `psl setup`"
	default:
		check.Detail = "is the running binary"
	}
	return check
}
