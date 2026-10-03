package cli

import (
	"github.com/spf13/cobra"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/interfaces/mcpserver"
	"promari-model-router/internal/interfaces/web"
)

func serveCmd(open Opener, st model.Settings) *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "serve the read-only HTTP API (OpenAPI: openapi/openapi.yaml) on localhost",
		RunE: use(open, func(s Scope) (web.Deps, error) {
			d := web.Deps{Serve: st.Serve}
			var err error
			if d.Report, err = s.Report(); err != nil {
				return d, err
			}
			if d.Explain, err = s.Explain(); err != nil {
				return d, err
			}
			d.Feed, err = s.Feed()
			return d, err
		}, func(cmd *cobra.Command, _ []string, d web.Deps) error {
			return web.Serve(cmd.Context(), addr, d, cmd.ErrOrStderr())
		}),
	}
	cmd.Flags().StringVar(&addr, "addr", st.Serve.Addr, "listen address (loopback only)")
	return cmd
}

func mcpCmd(open Opener) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "run the MCP server on stdio (tools: explain_route, routing_report, route_policy)",
		RunE: use(open, func(s Scope) (mcpserver.Deps, error) {
			d := mcpserver.Deps{Version: Version}
			var err error
			if d.Report, err = s.Report(); err != nil {
				return d, err
			}
			if d.Explain, err = s.Explain(); err != nil {
				return d, err
			}
			d.Policy, err = s.Policy()
			return d, err
		}, func(cmd *cobra.Command, _ []string, d mcpserver.Deps) error {
			return mcpserver.Run(cmd.Context(), d)
		}),
	}
}
