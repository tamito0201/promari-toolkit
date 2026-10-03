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
	"promari-statusline/internal/domain/model"
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
	Execute(dryRun bool) (usecase.InstallReport, error)
}

// Uninstaller is the uninstall use case.
type Uninstaller interface {
	Execute() (usecase.UninstallReport, error)
}

// Refresher is the use case behind the session-start hook.
type Refresher interface {
	Execute() (replaced bool, err error)
}

// Diagnoser is the doctor use case.
type Diagnoser interface {
	Execute() []model.Check
}

// App is the command line with the use cases it calls.
type App struct {
	Render    Renderer
	Install   Installer
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
		return a.setup(rest)
	case "uninstall":
		return a.uninstall()
	case "doctor":
		return a.doctor()
	case "hook":
		return a.hook()
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

func (a *App) setup(args []string) int {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	flags.SetOutput(a.Err)
	dryRun := flags.Bool("dry-run", false, "show what would change without writing anything")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	r, err := a.Install.Execute(*dryRun)
	if err != nil {
		return a.report(err)
	}
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
	if !r.DryRun {
		fmt.Fprintln(a.Out, "The status line appears on Claude Code's next redraw. A statusLine in a project's .claude/settings.json takes precedence over this one.")
	}
	return exitOK
}

func (a *App) uninstall() int {
	r, err := a.Uninstall.Execute()
	if err != nil {
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
	fmt.Fprintln(a.Out, "✅ removed the installed binary")
	return exitOK
}

// doctor prints the checks. It fails when a check failed, so a script can
// tell a working installation from a broken one.
func (a *App) doctor() int {
	fmt.Fprintln(a.Out, "psl "+Version)
	code := exitOK
	for _, check := range a.Diagnose.Execute() {
		mark := "✅"
		switch check.Level {
		case model.CheckWarn:
			mark = "⚠️ "
		case model.CheckFail:
			mark, code = "❌", exitError
		case model.CheckOK:
		}
		fmt.Fprintf(a.Out, "%s %s: %s\n", mark, check.Name, check.Detail)
	}
	return code
}

// hook handles a Claude Code hook. Every event does the same thing: bring the
// installed copy up to this binary. A hook must never stand in Claude Code's
// way, so it prints nothing and always succeeds; what went wrong shows in
// `psl doctor` as a copy that differs from the running binary.
func (a *App) hook() int {
	_, _ = a.Refresh.Execute()
	return exitOK
}
