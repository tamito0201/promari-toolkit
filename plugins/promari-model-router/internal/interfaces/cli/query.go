package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/samber/do/v2"
	"github.com/spf13/cobra"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/application/usecase"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
)

func queryCmd(with withFn, printer printerFn, st model.Settings) *cobra.Command {
	var limit int
	var schema bool
	cmd := &cobra.Command{
		Use:   "query [SELECT ...]",
		Short: "run a read-only SQL query on the ledger (or --schema to print the tables)",
		RunE: with(func(cmd *cobra.Command, args []string, i do.Injector) error {
			q := do.MustInvoke[usecase.QueryUseCase](i)
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
