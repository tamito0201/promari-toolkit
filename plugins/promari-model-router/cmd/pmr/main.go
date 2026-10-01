// Command pmr is the promari-model-router binary: Claude Code hook handler,
// CLI, MCP server and local HTTP API in one static executable.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/samber/do/v2"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/di"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/interfaces/cli"
)

func main() { os.Exit(run()) }

func run() int {
	// SIGTERM too: `pmr serve` and `pmr mcp` are stopped that way by
	// supervisors, and must shut down (close the ledger) instead of dying.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	root := cli.New(func() *do.RootScope { return di.New() })
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		return 1
	}
	return 0
}
