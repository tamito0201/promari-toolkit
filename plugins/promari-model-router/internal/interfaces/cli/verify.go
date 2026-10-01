package cli

import (
	"fmt"

	"github.com/samber/do/v2"
	"github.com/spf13/cobra"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/application/usecase"
)

func verifyCmd(with withFn) *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "verify the ledger's hash chain (tamper evidence)",
		RunE: with(func(cmd *cobra.Command, _ []string, i do.Injector) error {
			v, err := do.MustInvoke[usecase.VerifyUseCase](i).Execute(cmd.Context())
			switch {
			case err != nil:
				return err
			case v.Broken != 0:
				return fmt.Errorf("ledger chain broken at row %d (after %d intact rows)", v.Broken, v.Checked)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✅ verify complete: %d rows, chain intact\n", v.Checked)
			return nil
		}),
	}
}
