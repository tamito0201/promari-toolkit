// Command psl is the promari-statusline binary: the status line Claude Code
// draws, and the commands that install and check it.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"promari-statusline/internal/di"
	"promari-statusline/internal/infrastructure/platform"
)

func main() { os.Exit(run()) }

func run() int {
	// Claude Code stops a status line that takes too long with SIGTERM; the
	// context then ends every command and request the render is waiting for.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app := di.New(platform.New(), di.Streams{In: os.Stdin, Out: os.Stdout, Err: os.Stderr})
	return app.Run(ctx, os.Args[1:])
}
