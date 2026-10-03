package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/domain/model"
)

func evalCmd(open Opener, printer printerFn, st model.Settings) *cobra.Command {
	var file, session string
	var minAcc float64
	var maxHarm int
	var rulesOnly, verbose, allowEmbedded bool
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "run the labelled evaluation set through the full routing workflow (CI gate)",
		RunE: use(open, Scope.Eval, func(cmd *cobra.Command, _ []string, uc usecase.EvalUseCase) error {
			cwd, _ := os.Getwd()
			opt := usecase.EvalOptions{Path: file, SessionModel: session, Cwd: cwd}
			if cmd.Flags().Changed("min-accuracy") {
				opt.MinAccuracy = new(minAcc)
			}
			if cmd.Flags().Changed("max-harmful") {
				opt.MaxHarmful = new(maxHarm)
			}
			if rulesOnly {
				opt.UseModel = new(false)
			}
			opt.AllowEmbedded = allowEmbedded
			s, err := uc.Execute(opt)
			if err != nil {
				return err
			}
			err = printer(cmd.OutOrStdout())(s, func() {
				w := cmd.OutOrStdout()
				accuracy := "n/a"
				if s.Cases > 0 {
					accuracy = fmt.Sprintf("%.3f", s.Accuracy)
				}
				fmt.Fprintf(w, "eval: exact %d/%d = %s (gate %.2f)  injected %d  harmful downgrades %d (gate %d)  danger leaks %d  [session %s]\n",
					s.Exact, s.Cases, accuracy, s.Gate.MinAccuracy, s.Injected, s.Harmful, s.Gate.MaxHarmful, s.DangerLeaks, s.Gate.SessionModel)
				for _, lang := range sortedKeys(s.ByLang) {
					fmt.Fprintf(w, "  %s: %s\n", lang, ratio(s.ByLang[lang]))
				}
				b := s.Baselines
				fmt.Fprintf(w, "  sufficient tier: router %.2f  always-cheap %.2f  static %.2f  oracle %.2f\n", b.Router, b.AlwaysCheap, b.Static, b.Oracle)
				fmt.Fprintf(w, "  relative cost  : router %.2f  always-strong %.2f  static %.2f  oracle %.2f\n", b.RouterCost, b.StrongCost, b.StaticCost, b.OracleCost)
				fmt.Fprintf(w, "  collapse (share of the most common tier): %.2f\n", s.Collapse)
				fmt.Fprintf(w, "  artifact: %s\n", artifactLine(s.Artifact))
				if verbose {
					for _, m := range s.Misses {
						fmt.Fprintf(w, "  miss: want=%-12s got=%-12s %-7s %s\n", m.Want, m.Got,
							map[bool]string{true: "HARMFUL", false: ""}[m.Harmful], firstRunes(m.Text, st.Display.MissChars))
					}
				}
				if s.Passed {
					fmt.Fprintln(w, "✅ eval complete")
				} else {
					fmt.Fprintln(w, "❌ eval failed: "+strings.Join(s.FailedBecause, "; "))
				}
			})
			if err == nil && !s.Passed {
				return errors.New("eval gate failed")
			}
			return err
		}),
	}
	cmd.Flags().StringVar(&file, "file", "", "JSONL file (default: embedded evaluation set)")
	cmd.Flags().StringVar(&session, "session-model", st.Eval.SessionModel, "session model to evaluate against")
	cmd.Flags().Float64Var(&minAcc, "min-accuracy", st.Eval.MinAccuracy, "minimum exact-class accuracy")
	cmd.Flags().IntVar(&maxHarm, "max-harmful", st.Eval.MaxHarmful, "maximum routes below the needed tier")
	cmd.Flags().BoolVar(&rulesOnly, "rules-only", false, "disable the learned stages")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "list misses")
	cmd.Flags().BoolVar(&allowEmbedded, "allow-embedded", false, "also use the synthetic embedded artifact (never used on real traffic)")
	return cmd
}
