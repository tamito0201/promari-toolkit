// Package cli is the command line of psl: render (what Claude Code runs),
// setup and uninstall, doctor, the session-start hook, and version.
package cli

import (
	"context"
	_ "embed" // for the usage text
	"flag"
	"fmt"
	"io"

	"promari-statusline/internal/application/usecase"
)

// Version is the plugin's version, set at build time by the release build.
var Version = "dev"

// Exit codes.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

// usage is what `psl help` prints.
//
//go:embed usage.txt
var usage string

// Renderer draws the status line from in to out.
type Renderer interface {
	Handle(ctx context.Context, in io.Reader, out io.Writer) error
}

// Installer is the setup use case.
type Installer interface {
	Execute(ctx context.Context, dryRun bool) (usecase.InstallReport, error)
}

// GlobalInstaller is the setup use case for every project.
type GlobalInstaller interface {
	Execute(ctx context.Context, dryRun bool) (usecase.GlobalReport, error)
}

// Uninstaller is the uninstall use case.
type Uninstaller interface {
	Execute(ctx context.Context) (usecase.UninstallReport, error)
}

// Refresher is the use case behind the session-start hook.
type Refresher interface {
	Execute(ctx context.Context) (replaced bool, err error)
}

// Diagnoser is the doctor use case.
type Diagnoser interface {
	Execute(ctx context.Context) []usecase.Check
}

// App is the command line with the use cases it calls.
type App struct {
	Render    Renderer
	Install   Installer
	Global    GlobalInstaller
	Uninstall Uninstaller
	Refresh   Refresher
	Diagnose  Diagnoser

	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// Run executes one command line and returns the exit code.
func (a *App) Run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprint(a.Err, usage)
		return exitUsage
	}
	switch command, rest := args[0], args[1:]; command {
	case "render":
		return a.report(a.Render.Handle(ctx, a.In, a.Out))
	case "setup":
		return a.setup(ctx, rest)
	case "uninstall":
		return a.uninstall(ctx)
	case "doctor":
		return a.doctor(ctx)
	case "hook":
		return a.hook(ctx)
	case "version", "--version", "-v":
		fmt.Fprintln(a.Out, "psl "+Version)
		return exitOK
	case "help", "--help", "-h":
		fmt.Fprint(a.Out, usage)
		return exitOK
	default:
		fmt.Fprintf(a.Err, "psl: unknown command %q\n\n%s", command, usage)
		return exitUsage
	}
}

// report prints an error and turns it into an exit code.
func (a *App) report(err error) int {
	if err != nil {
		fmt.Fprintln(a.Err, "❌", err)
		return exitError
	}
	return exitOK
}

func (a *App) setup(ctx context.Context, args []string) int {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	flags.SetOutput(a.Err)
	dryRun := flags.Bool("dry-run", false, "show what would change without writing anything")
	global := flags.Bool("global", false, "also put the status line into every project that shows another one")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if *global {
		return a.setupGlobal(ctx, *dryRun)
	}
	r, err := a.Install.Execute(ctx, *dryRun)
	if err != nil {
		return a.report(err)
	}
	a.printInstall(r)
	if !r.DryRun {
		fmt.Fprintln(a.Out, "The status line appears on Claude Code's next redraw. A statusLine in a project's .claude/settings.json takes precedence over this one; `psl setup --global` covers those projects as well.")
	}
	return exitOK
}

// setupGlobal installs the status line for the user and for every project.
func (a *App) setupGlobal(ctx context.Context, dryRun bool) int {
	r, err := a.Global.Execute(ctx, dryRun)
	a.printInstall(r.User)
	if err != nil {
		return a.report(err)
	}
	failed := 0
	for _, f := range r.Fixes {
		switch {
		case f.Err != "":
			failed++
			fmt.Fprintf(a.Err, "❌ %s: %s\n", f.Dir, f.Err)
		case dryRun:
			fmt.Fprintf(a.Out, "would set statusLine in %s (it shows: %s)\n", f.Settings, f.Previous)
		default:
			fmt.Fprintf(a.Out, "✅ statusLine in %s now runs: %s (it showed: %s)\n", f.Settings, r.User.Command, f.Previous)
			if f.Excluded {
				fmt.Fprintf(a.Out, "   added to the repository's .git/info/exclude, so git does not pick it up\n")
			}
			if f.Backup != "" {
				fmt.Fprintf(a.Out, "   backup: %s\n", f.Backup)
			}
		}
	}
	fmt.Fprintf(a.Out, "projects: %d checked, %d showing another status line\n", r.Checked, len(r.Fixes))
	if !dryRun {
		fmt.Fprintln(a.Out, "Every terminal shows the status line on Claude Code's next redraw. A project's shared .claude/settings.json is never changed.")
	}
	if failed > 0 {
		return exitError
	}
	return exitOK
}

// printInstall prints what the installation for the user did.
func (a *App) printInstall(r usecase.InstallReport) {
	verb := "✅ installed"
	if r.DryRun {
		verb = "would install"
	}
	fmt.Fprintf(a.Out, "%s: %s\n", verb, r.Binary)
	switch {
	case !r.Changed:
		fmt.Fprintf(a.Out, "settings unchanged: %s already runs %s\n", r.Settings, r.Command)
	case r.DryRun:
		fmt.Fprintf(a.Out, "would set statusLine in %s to: %s\n", r.Settings, r.Command)
	default:
		fmt.Fprintf(a.Out, "✅ statusLine in %s now runs: %s\n", r.Settings, r.Command)
	}
	if r.Changed && r.Previous != "" {
		fmt.Fprintf(a.Out, "previous command: %s\n", r.Previous)
	}
	if r.Backup != "" {
		fmt.Fprintf(a.Out, "backup: %s\n", r.Backup)
	}
}

// uninstall prints what was done before the error, if any: the user's settings
// may be changed already, and their backup is worth knowing about.
func (a *App) uninstall(ctx context.Context) int {
	r, err := a.Uninstall.Execute(ctx)
	if !r.Read {
		return a.report(err)
	}
	switch {
	case r.Removed:
		fmt.Fprintf(a.Out, "✅ removed statusLine from %s\n", r.Settings)
	case r.Other != "":
		fmt.Fprintf(a.Out, "left alone: statusLine in %s runs another command: %s\n", r.Settings, r.Other)
	default:
		fmt.Fprintf(a.Out, "no statusLine in %s\n", r.Settings)
	}
	if r.Backup != "" {
		fmt.Fprintf(a.Out, "backup: %s\n", r.Backup)
	}
	for _, path := range r.Projects {
		fmt.Fprintf(a.Out, "✅ removed statusLine from %s\n", path)
	}
	if err != nil {
		fmt.Fprintln(a.Out, "the installed binary is kept")
		return a.report(err)
	}
	fmt.Fprintln(a.Out, "✅ removed the installed binary")
	return exitOK
}

// doctor prints the checks. It fails when a check failed, so a script can
// tell a working installation from a broken one.
func (a *App) doctor(ctx context.Context) int {
	fmt.Fprintln(a.Out, "psl "+Version)
	code := exitOK
	checks := a.Diagnose.Execute(ctx)
	for i := range checks {
		check := &checks[i]
		mark := "✅"
		switch check.Level {
		case usecase.CheckWarn:
			mark = "⚠️ "
		case usecase.CheckFail:
			mark, code = "❌", exitError
		case usecase.CheckOK:
		}
		name, detail := describe(check)
		fmt.Fprintf(a.Out, "%s %s: %s\n", mark, name, detail)
	}
	return code
}

// hook handles a Claude Code hook. Every event does the same thing: bring the
// installed copy up to this binary. A hook must never stand in Claude Code's
// way, so it prints nothing and always succeeds; what went wrong shows in
// `psl doctor` as a copy that differs from the running binary.
func (a *App) hook(ctx context.Context) int {
	_, _ = a.Refresh.Execute(ctx)
	return exitOK
}
