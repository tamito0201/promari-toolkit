package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/domain/model"
)

func explainCmd(open Opener, printer printerFn, st model.Settings) *cobra.Command {
	var session, subType string
	cmd := &cobra.Command{
		Use:     "explain [prompt]",
		Aliases: []string{"classify"},
		Short:   "show how a prompt is classified and routed, stage by stage",
		RunE: use(open, Scope.Explain, func(cmd *cobra.Command, args []string, uc usecase.ExplainUseCase) error {
			prompt := strings.Join(args, " ")
			if prompt == "" {
				raw, _ := io.ReadAll(cmd.InOrStdin())
				prompt = string(raw)
			}
			cwd, _ := os.Getwd()
			ex := uc.Execute(usecase.ExplainInput{Prompt: prompt, SubagentType: subType, SessionModel: session, Cwd: cwd})
			return printer(cmd.OutOrStdout())(ex, func() {
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "class      : %s  confidence=%d margin=%d\n", orDash(ex.Class, "(abstain)"), ex.Confidence, ex.Margin)
				fmt.Fprintf(w, "flags      : danger=%v codex=%s continuation=%v lang=%s chars=%d\n", ex.Danger, orDash(ex.Codex, "-"), ex.Continuation, ex.Lang, ex.Chars)
				fmt.Fprintf(w, "scores     : %v\n", ex.Scores)
				fmt.Fprintf(w, "reasons    : %s\n", strings.Join(ex.Trace.Reasons, ", "))
				fmt.Fprintf(w, "stages     : %s\n", strings.Join(ex.Trace.Path, " > "))
				if len(ex.Trace.Probs) > 0 {
					fmt.Fprintf(w, "model      : %s probs=%v set=%v p_safe=%.3f tau=%.2f unseen=%.2f neighbor=%.3f\n",
						ex.Trace.ModelClass, ex.Trace.Probs, ex.Trace.Set, ex.Trace.PSafe, ex.Trace.Tau, ex.Trace.Unseen, ex.Trace.NeighborSim)
				}
				fmt.Fprintf(w, "subagent   : %s (%s) -> %s  [type=%s, session=%s]\n", ex.Decision.Action, ex.Decision.Reason,
					orDash(ex.Decision.Target, "-"), ex.Decision.SubagentType, session)
				fmt.Fprintf(w, "advice     : %s\n", orDash(ex.Advice, "-"))
				fmt.Fprintf(w, "artifact   : %s\n", artifactLine(ex.Artifact))
			})
		}),
	}
	cmd.Flags().StringVar(&session, "session-model", st.Eval.SessionModel, "model of the main conversation")
	cmd.Flags().StringVar(&subType, "subagent-type", model.DefaultSubagentType, "subagent type to simulate")
	return cmd
}
