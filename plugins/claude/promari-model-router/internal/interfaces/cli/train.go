package cli

import (
	"fmt"
	"os"

	"github.com/samber/do/v2"
	"github.com/spf13/cobra"

	"promari-model-router/internal/application/usecase"
)

func trainCmd(with withFn, printer printerFn) *cobra.Command {
	var files []string
	var ledgerDays float64
	var output string
	cmd := &cobra.Command{
		Use:   "train",
		Short: "fit, calibrate and save the routing artifact (logistic regression + conformal risk control + ledger posteriors)",
		RunE: with(func(cmd *cobra.Command, _ []string, i do.Injector) error {
			cwd, _ := os.Getwd()
			art, path, err := do.MustInvoke[usecase.TrainUseCase](i).Execute(cmd.Context(), usecase.TrainInput{Files: files, LedgerDays: ledgerDays, Output: output, Cwd: cwd})
			if err != nil {
				return err
			}
			summary := map[string]any{"path": path, "samples": art.Samples, "source": art.Source, "metrics": art.Metrics, "tau": art.Tau, "gate": art.Gate}
			return printer(cmd.OutOrStdout())(summary, func() {
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "✅ train complete: %s\n  %s\n", path, art.Source)
				for _, k := range sortedKeys(art.Metrics) {
					fmt.Fprintf(w, "  %-20s %.4f\n", k, art.Metrics[k])
				}
				fmt.Fprintf(w, "  tau by length bucket: %v\n  gate by class: %v\n", art.Tau, art.Gate)
			})
		}),
	}
	cmd.Flags().StringSliceVar(&files, "file", nil, "labelled JSONL files (default: embedded evaluation set)")
	cmd.Flags().Float64Var(&ledgerDays, "ledger-days", 0, "also learn posteriors from this many days of the ledger")
	cmd.Flags().StringVar(&output, "output", "", "write here instead of the plugin data directory")
	return cmd
}
