// Package cli is the Cobra command tree of `pmr`. Each command is handed the
// one use case it runs (see Scope) and only formats output; no business logic
// here.
// Each subcommand lives in its own file; this one holds the tree and the
// helpers they share.
package cli

import (
	"encoding/json"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"promari-model-router/internal/application/usecase"
)

// Version is set at build time with -ldflags "-X .../cli.Version=...".
var Version = "dev"

// New builds the root command around the factory of its scopes (injected so
// that tests can replace infrastructure). Every command opens a scope of its
// own and closes it when it ends.
func New(open Opener) *cobra.Command {
	// Flag defaults come from the TOML settings (no literals in this package).
	boot := open()
	st := boot.Settings()
	_ = boot.Close()
	var asJSON bool
	root := &cobra.Command{
		Use:           "pmr",
		Short:         "promari-model-router: route Claude Code subagents by task difficulty",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       Version,
	}
	root.PersistentFlags().BoolVar(&asJSON, "json", false, "print JSON")

	printer := func(w io.Writer) func(v any, text func()) error {
		return func(v any, text func()) error {
			if asJSON {
				enc := json.NewEncoder(w)
				enc.SetIndent("", "  ")
				return enc.Encode(v)
			}
			text()
			return nil
		}
	}

	root.AddCommand(
		hookCmd(open, st),
		explainCmd(open, printer, st),
		reportCmd(open, printer, st),
		evalCmd(open, printer, st),
		trainCmd(open, printer),
		doctorCmd(open, printer, st.Display.ErrorChars),
		lintCmd(open),
		queryCmd(open, printer, st),
		verifyCmd(open),
		costCmd(open, printer, st),
		serveCmd(open, st),
		mcpCmd(open),
		cloudCmd(open, printer, st.Cloud),
	)
	return root
}

type printerFn = func(w io.Writer) func(v any, text func()) error

func orDash(s, dash string) string {
	if s == "" {
		return dash
	}
	return s
}

// artifactLine says which learned artifact a command routed with.
func artifactLine(a usecase.ArtifactUsed) string {
	readiness := "ready"
	if !a.Ready {
		readiness = "rules only"
	}
	line := orDash(a.Origin, "-") + " (" + readiness + ")"
	if a.Problem != "" {
		line += "; the local artifact was not used: " + a.Problem
	}
	return line
}

func sortedKeys[V any](m map[string]V) []string { return slices.Sorted(maps.Keys(m)) }

// firstRunes shortens text for one line of output: newlines become spaces and
// anything past n runes is cut and marked with an ellipsis.
func firstRunes(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
