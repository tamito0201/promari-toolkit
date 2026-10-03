package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"promari-model-router/internal/application/usecase"
)

func verifyCmd(open Opener) *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "verify the ledger's hash chain (tamper evidence)",
		RunE: use(open, Scope.Verify, func(cmd *cobra.Command, _ []string, uc usecase.VerifyUseCase) error {
			v, err := uc.Execute(cmd.Context())
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
