package cli

import (
	"cmp"
	"fmt"
	"io"
	"strings"

	"github.com/samber/do/v2"
	"github.com/spf13/cobra"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/domain/model"
)

func reportCmd(with withFn, printer printerFn, st model.Settings) *cobra.Command {
	var days float64
	cmd := &cobra.Command{
		Use:   "report",
		Short: "summarise the decision ledger",
		RunE: with(func(cmd *cobra.Command, _ []string, i do.Injector) error {
			rep, err := do.MustInvoke[usecase.ReportUseCase](i).Execute(cmd.Context(), days)
			if err != nil {
				return err
			}
			return printer(cmd.OutOrStdout())(rep, func() { printReport(cmd.OutOrStdout(), rep, days, st.Display.CostDecimals) })
		}),
	}
	cmd.Flags().Float64Var(&days, "days", st.Report.DefaultDays, "window in days")
	return cmd
}

func ratio(r usecase.Ratio) string {
	if r.Whole == 0 {
		return "n/a (0 events)"
	}
	return fmt.Sprintf("%.1f%% (%d/%d)", 100*float64(r.Part)/float64(r.Whole), r.Part, r.Whole)
}

func printReport(w io.Writer, rep usecase.Report, days float64, costDecimals int) {
	p, s, r := rep.Prompts, rep.Subagents, rep.Results
	fmt.Fprintf(w, "promari-model-router report — last %g day(s), %d ledger entries\n\n", days, rep.Entries)
	fmt.Fprintf(w, "Prompts: %d  classified %s  advised %d  danger %d\n", p.Total, ratio(usecase.Ratio{Part: p.Classified, Whole: p.Total}), p.Advised, p.Danger)
	for _, lang := range sortedKeys(p.ByLang) {
		fmt.Fprintf(w, "  classified [%s]: %s\n", lang, ratio(p.ByLang[lang]))
	}
	fmt.Fprintf(w, "\nSubagent calls: %d\n  actions: %v\n  reasons: %v\n  injected: %v  shadow: %v\n", s.Calls, s.ByAction, s.ByReason, s.Injected, s.Shadow)
	fmt.Fprintf(w, "\nSubagent results: %d (joined to a decision: %d)\n", r.Count, r.Joined)
	fmt.Fprintf(w, "  calls by resolved tier : %v\n  tokens by resolved tier: %v\n", r.CallsByTier, r.TokensByTier)
	fmt.Fprintf(w, "  tokens run below the session tier: %d\n  requested != resolved: %d   background (no usage data): %d\n", r.TokensBelowSession, r.Mismatch, r.AsyncNoUsage)
	costs := make([]string, 0, len(rep.EstimatedCostUSD))
	for _, tier := range sortedKeys(rep.EstimatedCostUSD) {
		costs = append(costs, fmt.Sprintf("%s $%.*f", tier, costDecimals, rep.EstimatedCostUSD[tier]))
	}
	fmt.Fprintf(w, "  list-price estimate (USD, prices as of %s): %s\n", rep.PricesAsOf, cmp.Or(strings.Join(costs, ", "), "none"))
	fmt.Fprintf(w, "\nHook errors: %d\n", rep.Errors)
	if rep.Entries == 0 {
		fmt.Fprintln(w, "\nNo data. Is the plugin enabled? Run `pmr doctor`.")
	}
	fmt.Fprintln(w, "\nNote: token totals are what subagents used, not a saving. Compare against a baseline period (mode: shadow) before claiming one.")
}
