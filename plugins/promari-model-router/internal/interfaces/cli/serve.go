package cli

import (
	"github.com/samber/do/v2"
	"github.com/spf13/cobra"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/application/usecase"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/interfaces/mcpserver"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/interfaces/web"
)

func serveCmd(with withFn, st model.Settings) *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "serve the read-only HTTP API (OpenAPI: openapi/openapi.yaml) on localhost",
		RunE: with(func(cmd *cobra.Command, _ []string, i do.Injector) error {
			return web.Serve(cmd.Context(), addr, web.Deps{
				Report: do.MustInvoke[usecase.ReportUseCase](i), Explain: do.MustInvoke[usecase.ExplainUseCase](i),
				Feed: do.MustInvoke[usecase.FeedUseCase](i), Serve: st.Serve,
			}, cmd.ErrOrStderr())
		}),
	}
	cmd.Flags().StringVar(&addr, "addr", st.Serve.Addr, "listen address (loopback only)")
	return cmd
}

func mcpCmd(with withFn) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "run the MCP server on stdio (tools: explain_route, routing_report, route_policy)",
		RunE: with(func(cmd *cobra.Command, _ []string, i do.Injector) error {
			return mcpserver.Run(cmd.Context(), mcpserver.Deps{
				Report: do.MustInvoke[usecase.ReportUseCase](i), Explain: do.MustInvoke[usecase.ExplainUseCase](i),
				Policy: do.MustInvoke[usecase.PolicyUseCase](i), Version: Version,
			})
		}),
	}
}
