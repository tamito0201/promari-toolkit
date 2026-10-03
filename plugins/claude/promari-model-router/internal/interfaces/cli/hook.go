package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"promari-model-router/internal/domain/model"
)

// hookCmd is what hooks/hooks.json runs. It never fails the process and never
// prints hook output after a failure; what went wrong is recorded in the
// ledger, else in the failure file beside it, and printed to stderr.
func hookCmd(open Opener, st model.Settings) *cobra.Command {
	return &cobra.Command{
		Use:   "hook <EventName>",
		Short: "handle one Claude Code hook event (stdin JSON -> stdout JSON)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(st.Runtime.HookTimeoutMS)*time.Millisecond)
			defer cancel()
			s := open()
			defer func() { _ = s.Close() }()
			onError := func(where, detail string) {
				fmt.Fprintf(cmd.ErrOrStderr(), "pmr hook %s: %s\n", where, detail)
				if rec, err := s.RecordError(); err == nil {
					rec.RecordError(ctx, where, detail)
					return
				}
				// Not even the ledger can be wired (the data directory cannot be
				// created, the database cannot be opened): the failure file needs
				// neither.
				f, err := s.FailureRecorder()
				clk, clkErr := s.Clock()
				if err == nil && clkErr == nil {
					_ = f.RecordFailure(model.Failure{At: clk.Now(), Message: where + ": " + detail})
				}
			}
			h, err := s.Hook()
			if err != nil {
				onError(args[0], "wire the hook: "+err.Error())
				return
			}
			h.Run(ctx, args[0], cmd.InOrStdin(), cmd.OutOrStdout(), onError)
		},
	}
}
