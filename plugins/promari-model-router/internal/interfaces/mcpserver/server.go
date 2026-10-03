// Package mcpserver exposes the router over the Model Context Protocol (stdio,
// official Go SDK). Tool input and output schemas are generated from the Go
// types (JSON Schema), so Claude can ask "which tier fits this brief?" and
// "how has routing gone this week?" through typed tool calls.
//
// Trust boundary: every tool is read-only. Nothing here writes the ledger,
// changes settings, or reaches the network (least privilege).
package mcpserver

import (
	"context"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/domain/model"
)

// Deps are the use cases the server reads from.
type Deps struct {
	Report  usecase.ReportUseCase
	Explain usecase.ExplainUseCase
	Policy  usecase.PolicyUseCase
	Version string
}

// ExplainArgs is the input of explain_route.
type ExplainArgs struct {
	Prompt       string `json:"prompt" jsonschema:"the subagent brief or user request to classify"`
	SubagentType string `json:"subagent_type,omitempty" jsonschema:"subagent type to simulate (default general-purpose)"`
	SessionModel string `json:"session_model,omitempty" jsonschema:"model of the main conversation (default: [eval].session_model)"`
}

// ReportArgs is the input of routing_report.
type ReportArgs struct {
	Days float64 `json:"days,omitempty" jsonschema:"window in days (default: [report].default_days)"`
}

// PolicyOut is the output of route_policy.
type PolicyOut = usecase.Policy

// NewServer builds the MCP server with its tools.
func NewServer(d Deps) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "promari-model-router", Version: d.Version}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "explain_route",
		Description: "Classify a brief and show which model tier the router would pick for a subagent, stage by stage (rules, calibrated model, conformal set, risk threshold, safety floors).",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in ExplainArgs) (*mcp.CallToolResult, usecase.Explanation, error) {
		cwd, _ := os.Getwd()
		out := d.Explain.Execute(usecase.ExplainInput{
			Prompt: in.Prompt, SubagentType: model.AgentCall{SubagentType: in.SubagentType}.SubagentTypeOrDefault(),
			SessionModel: in.SessionModel, Cwd: cwd,
		})
		return nil, out, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "routing_report",
		Description: "Summarise the routing ledger: classification rate per language, routed subagents, tokens by the model that actually ran, and list-price estimates.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ReportArgs) (*mcp.CallToolResult, usecase.Report, error) {
		rep, err := d.Report.Execute(ctx, in.Days)
		return nil, rep, err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "route_policy",
		Description: "Return the routing policy and the [route: <class>] tags to put at the start of a subagent brief.",
	}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, PolicyOut, error) {
		cwd, _ := os.Getwd()
		return nil, d.Policy.Execute(cwd), nil
	})
	return s
}

// Run serves MCP on stdin/stdout until the client disconnects.
func Run(ctx context.Context, d Deps) error {
	return NewServer(d).Run(ctx, &mcp.StdioTransport{})
}
