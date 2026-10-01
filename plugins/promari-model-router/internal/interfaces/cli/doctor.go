package cli

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"text/tabwriter"

	"github.com/samber/do/v2"
	"github.com/spf13/cobra"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/application/usecase"
)

// doctorHints are the next steps the CLI suggests per check and verdict (the
// use case reports facts; what to type is the CLI's business).
var doctorHints = map[usecase.CheckStatus]map[string]string{
	usecase.CheckWarn: {
		usecase.CheckArtifact: "run `pmr train --file <your labelled prompts>` to route with a learned artifact",
	},
	usecase.CheckFail: {
		usecase.CheckArtifact: "fix or delete the file, then run `pmr train`",
		usecase.CheckForce:    "unset it (or set it to 0) to let the router choose models",
		"wiring":              "check that the data directory can be created and written",
	},
}

// doctorLine is a check as the CLI prints it.
type doctorLine struct {
	usecase.Check
	Hint string `json:"hint,omitempty"`
}

func doctorCmd(with withFn, printer printerFn, errorChars int) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "check conflicting settings, usage caches, the artifact, agent tiers, the ledger chain and recorded failures",
		RunE: with(func(cmd *cobra.Command, _ []string, i do.Injector) error {
			cwd, _ := os.Getwd()
			var checks []usecase.Check
			if u, err := do.Invoke[usecase.DoctorUseCase](i); err != nil {
				// The container could not build the doctor (the data directory
				// or the ledger cannot be opened): that is the finding.
				checks = []usecase.Check{{Status: usecase.CheckFail, Name: "wiring", Detail: err.Error()}}
			} else {
				checks = u.Execute(cmd.Context(), cwd)
			}
			lines := make([]doctorLine, len(checks))
			for k, c := range checks {
				lines[k] = doctorLine{Check: c, Hint: doctorHints[c.Status][c.Name]}
			}
			err := printer(cmd.OutOrStdout())(lines, func() {
				tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
				icon := map[usecase.CheckStatus]string{usecase.CheckOK: "✅", usecase.CheckWarn: "⚠️ ", usecase.CheckFail: "❌"}
				for _, l := range lines {
					detail := firstRunes(l.Detail, errorChars)
					if l.Hint != "" {
						detail += " (" + l.Hint + ")"
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\n", icon[l.Status], l.Name, detail)
				}
				_ = tw.Flush()
			})
			if slices.ContainsFunc(checks, func(c usecase.Check) bool { return c.Status == usecase.CheckFail }) {
				return errors.New("doctor found failures")
			}
			return err
		}),
	}
}
