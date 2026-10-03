package cli

import (
	"fmt"

	"github.com/samber/do/v2"
	"github.com/spf13/cobra"

	"promari-model-router/internal/application/usecase"
)

func lintCmd(with withFn) *cobra.Command {
	return &cobra.Command{
		Use:   "lint",
		Short: "check agent frontmatter against data/tiers.toml",
		RunE: with(func(cmd *cobra.Command, _ []string, i do.Injector) error {
			res := do.MustInvoke[usecase.LintUseCase](i).Execute()
			if len(res.Problems) > 0 {
				for _, p := range res.Problems {
					fmt.Fprintln(cmd.OutOrStdout(), "❌", p)
				}
				return fmt.Errorf("%d agent(s) disagree with data/tiers.toml", len(res.Problems))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✅ lint complete: %d agents match data/tiers.toml\n", res.Agents)
			return nil
		}),
	}
}
