package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"

	"github.com/caanmert/ai-session-tool/internal/runner"
	"github.com/caanmert/ai-session-tool/internal/runview"
)

func newRunReviewCmd(app *App) *cobra.Command {
	var task string
	var refresh time.Duration
	cmd := &cobra.Command{
		Use: "review <run-id>", Short: "Interactively review tasks, changed files and diffs", Args: cobra.ExactArgs(1),
		Long: `Review a saved run in a terminal. Tab switches between tasks, files and the
diff; arrow keys select or scroll. Press a to attach to the selected agent,
r to refresh, ? for keys, and q to quit. Review never stages or merges changes.
Outside tmux, detach from the agent to return here. Inside tmux, switch back to
the review window/session. Use ais run status or diff for non-interactive output.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if refresh < 0 || (refresh > 0 && refresh < time.Second) {
				return errors.New("refresh must be 0s (manual) or at least 1s")
			}
			m, r, err := findRunTasks(app, args[0], "")
			if err != nil {
				return err
			}
			if _, err := runner.SelectTasks(r, task); err != nil {
				return err
			}
			if app.Interactive == nil || !app.Interactive() {
				return errors.New("interactive review requires a terminal; use ais run status or ais run diff")
			}
			inside := os.Getenv("TMUX") != ""
			d := runview.Deps{
				Run: r, InitialTask: task, Refresh: refresh, Theme: app.theme(),
				Status: func(ctx context.Context) (runner.RunStatus, error) { return m.Status(ctx, r) },
				Diff: func(ctx context.Context, t runner.Task, file string) (string, error) {
					return m.DiffFile(ctx, r, t, file)
				},
				Attach: func(ctx context.Context, name string) (*exec.Cmd, error) {
					return m.TaskAttachCmd(ctx, r, name, inside)
				},
			}
			return runview.Run(cmd.Context(), d, app.In, app.Out)
		},
	}
	cmd.Flags().StringVar(&task, "task", "", "initially select a task (e.g. task-2)")
	cmd.Flags().DurationVar(&refresh, "refresh", 3*time.Second, "refresh interval (0s for manual refresh)")
	return cmd
}
