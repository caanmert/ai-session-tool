package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/caanmert/ai-session-tool/internal/launch"
)

func newResumeCmd(app *App) *cobra.Command {
	var fork, print bool
	cmd := &cobra.Command{
		Use:   "resume <id>",
		Short: "Resume a session in its own project directory",
		Long: `Resume a session in its own project directory, handing this terminal to
claude (or codex). <id> may be any unique prefix of the session id.`,
		Example: `  ais resume 3f2a
  ais resume 3f2a --fork     # continue in a new session, keep the original
  ais resume 3f2a --print    # print the shell command instead of running it`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, p, err := app.lookup(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			c, err := p.ResumeCmd(s, fork)
			if err != nil {
				return fmt.Errorf("%s session %s: %w", s.Tool, s.ID, err)
			}
			if print {
				fmt.Fprintln(app.Out, launch.ShellLine(c))
				return nil
			}
			if err := launch.Check(c); err != nil {
				return err
			}
			fmt.Fprintf(app.Err, "resuming %s in %s\n", s.Title, s.CWD)
			return launch.Exec(c)
		},
	}
	cmd.Flags().BoolVar(&fork, "fork", false, "fork into a new session instead of continuing the original")
	cmd.Flags().BoolVar(&print, "print", false, "print the resume command instead of running it")
	return cmd
}
