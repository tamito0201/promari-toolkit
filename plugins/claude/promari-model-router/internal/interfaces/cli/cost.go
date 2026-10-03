package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/domain/model"
)

func costCmd(open Opener, printer printerFn, st model.Settings) *cobra.Command {
	var shape usecase.SubagentShape
	cmd := &cobra.Command{
		Use:   "cost",
		Short: "price one subagent run on every tier (Harness Tokenomics cost model)",
		RunE: use(open, Scope.Cost, func(cmd *cobra.Command, _ []string, uc usecase.CostUseCase) error {
			est := uc.Execute(shape)
			return printer(cmd.OutOrStdout())(est, func() {
				u := est.Usage
				fmt.Fprintf(cmd.OutOrStdout(), "usage: cache read %d, cache write %d, output %d (list prices as of %s)\n", u.CacheRead, u.CacheWrite, u.Output, est.PricesAsOf)
				for _, t := range model.TierOrder {
					if v, ok := est.USDByTier[t]; ok {
						fmt.Fprintf(cmd.OutOrStdout(), "  %-7s $%.*f\n", t, st.Display.CostDecimals, v)
					}
				}
			})
		}),
	}
	cmd.Flags().IntVar(&shape.ToolCalls, "tool-calls", st.Cost.ToolCalls, "k: tool calls in the subagent")
	cmd.Flags().IntVar(&shape.Prompt, "prompt", st.Cost.Prompt, "P: system prompt + brief tokens")
	cmd.Flags().IntVar(&shape.ToolResult, "tool-result", st.Cost.ToolResult, "t: tokens per tool result")
	cmd.Flags().IntVar(&shape.OutputPerCall, "output-per-call", st.Cost.OutputPerCall, "o: output tokens per call")
	cmd.Flags().IntVar(&shape.Report, "report", st.Cost.Report, "r: final report tokens")
	return cmd
}
