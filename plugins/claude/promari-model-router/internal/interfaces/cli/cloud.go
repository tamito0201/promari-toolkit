package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/domain/model"
)

// cloudView is what `pmr cloud` prints. The pacing values are there for the
// /cloud command, which reads the reply and cannot read the TOML itself.
type cloudView struct {
	Session             string    `json:"session"`
	URL                 string    `json:"url"`
	SentAt              time.Time `json:"sent_at,omitzero"`
	PollIntervalSeconds int       `json:"poll_interval_seconds"`
	MaxPolls            int       `json:"max_polls"`
}

func cloudCmd(open Opener, printer printerFn, st model.CloudSettings) *cobra.Command {
	view := func(id model.CloudSessionID, page string, sentAt time.Time) cloudView {
		return cloudView{
			Session: id.String(), URL: orDash(page, id.URL()), SentAt: sentAt,
			PollIntervalSeconds: st.PollIntervalSeconds, MaxPolls: st.MaxPolls,
		}
	}
	show := func(cmd *cobra.Command, v cloudView) error {
		return printer(cmd.OutOrStdout())(v, func() {
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "session : %s\n", v.Session)
			fmt.Fprintf(w, "page    : %s\n", v.URL)
			if !v.SentAt.IsZero() {
				fmt.Fprintf(w, "sent at : %s\n", v.SentAt.Format(time.RFC3339))
			}
		})
	}
	cmd := &cobra.Command{
		Use:   "cloud",
		Short: "relay messages to a Claude Code cloud session (the work runs, and is billed, in the cloud)",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "use <session-id|claude.ai/code URL>",
			Short: "set the cloud session messages go to",
			Args:  cobra.ExactArgs(1),
			RunE: use(open, Scope.Cloud, func(cmd *cobra.Command, args []string, uc usecase.CloudUseCase) error {
				link, err := uc.Use(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				return show(cmd, view(link.Session, "", time.Time{}))
			}),
		},
		&cobra.Command{
			Use:   "send [message]",
			Short: "queue a message into the cloud session (read from stdin when no argument is given)",
			RunE: use(open, Scope.Cloud, func(cmd *cobra.Command, args []string, uc usecase.CloudUseCase) error {
				text := strings.Join(args, " ")
				if text == "" {
					raw, err := io.ReadAll(cmd.InOrStdin())
					if err != nil {
						return err
					}
					text = string(raw)
				}
				r, err := uc.Send(cmd.Context(), text)
				if err != nil {
					return err
				}
				return show(cmd, view(r.Session, r.URL, r.SentAt))
			}),
		},
		&cobra.Command{
			Use:   "status",
			Short: "show the cloud session in use and when the last message went",
			Args:  cobra.NoArgs,
			RunE: use(open, Scope.Cloud, func(cmd *cobra.Command, _ []string, uc usecase.CloudUseCase) error {
				link, err := uc.Status(cmd.Context())
				if err != nil {
					return err
				}
				return show(cmd, view(link.Session, "", link.SentAt))
			}),
		},
		&cobra.Command{
			Use:   "wait",
			Short: "wait one poll interval before /cloud reads the reply again",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				// A cancelled wait ends at once, even with a zero interval
				// (select would pick between two ready cases at random).
				if err := cmd.Context().Err(); err != nil {
					return err
				}
				select {
				case <-time.After(time.Duration(st.PollIntervalSeconds) * time.Second):
					return nil
				case <-cmd.Context().Done():
					return cmd.Context().Err()
				}
			},
		},
	)
	return cmd
}
