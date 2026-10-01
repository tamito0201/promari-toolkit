// Package cli is the Cobra command tree of `pmr`. Commands resolve their use
// cases from the DI container and only format output; no business logic here.
// Each subcommand lives in its own file; this one holds the tree and the
// helpers they share.
package cli

import (
	"encoding/json"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/samber/do/v2"
	"github.com/spf13/cobra"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
)

// Version is set at build time with -ldflags "-X .../cli.Version=...".
var Version = "dev"

// New builds the root command around a container factory (injected so that
// tests can replace infrastructure).
func New(container func() *do.RootScope) *cobra.Command {
	// Flag defaults come from the TOML settings (no literals in this package).
	boot := container()
	st := do.MustInvoke[model.Settings](boot)
	_ = boot.Shutdown()
	var asJSON bool
	root := &cobra.Command{
		Use:           "pmr",
		Short:         "promari-model-router: route Claude Code subagents by task difficulty",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       Version,
	}
	root.PersistentFlags().BoolVar(&asJSON, "json", false, "print JSON")

	with := func(run func(cmd *cobra.Command, args []string, i do.Injector) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			c := container()
			defer func() { _ = c.Shutdown() }()
			return run(cmd, args, c)
		}
	}
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
		hookCmd(container, st),
		explainCmd(with, printer, st),
		reportCmd(with, printer, st),
		evalCmd(with, printer, st),
		trainCmd(with, printer),
		doctorCmd(with, printer, st.Display.ErrorChars),
		lintCmd(with),
		queryCmd(with, printer, st),
		verifyCmd(with),
		costCmd(with, printer, st),
		serveCmd(with, st),
		mcpCmd(with),
	)
	return root
}

type (
	withFn    = func(run func(cmd *cobra.Command, args []string, i do.Injector) error) func(*cobra.Command, []string) error
	printerFn = func(w io.Writer) func(v any, text func()) error
)

func orDash(s, dash string) string {
	if s == "" {
		return dash
	}
	return s
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
