package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/domain/model"
)

func queryCmd(open Opener, printer printerFn, st model.Settings) *cobra.Command {
	var limit int
	var schema bool
	cmd := &cobra.Command{
		Use:   "query [SELECT ...]",
		Short: "run a read-only SQL query on the ledger (or --schema to print the tables)",
		RunE: use(open, Scope.Query, func(cmd *cobra.Command, args []string, q usecase.QueryUseCase) error {
			if schema {
				stmts, err := q.Schema(cmd.Context())
				if err != nil {
					return err
				}
				return printer(cmd.OutOrStdout())(stmts, func() { fmt.Fprintln(cmd.OutOrStdout(), strings.Join(stmts, ";\n\n")+";") })
			}
			res, err := q.Query(cmd.Context(), strings.Join(args, " "), limit)
			if err != nil {
				return err
			}
			cols, rows := res.Columns, res.Rows
			return printer(cmd.OutOrStdout())(rows, func() {
				tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, strings.Join(cols, "\t"))
				for _, r := range rows {
					vals := make([]string, len(cols))
					for k, c := range cols {
						vals[k] = fmt.Sprint(r[c])
					}
					fmt.Fprintln(tw, strings.Join(vals, "\t"))
				}
				_ = tw.Flush()
			})
		}),
	}
	cmd.Flags().IntVar(&limit, "limit", st.Query.DefaultLimit, "maximum rows")
	cmd.Flags().BoolVar(&schema, "schema", false, "print the table definitions")
	return cmd
}
