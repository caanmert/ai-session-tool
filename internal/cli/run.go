package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/caanmert/ai-session-tool/internal/launch"
	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/runner"
)

func (a *App) runner() (*runner.Manager, error) {
	if a.DataDir == nil {
		return nil, errors.New("parallel runs need ais's data directory")
	}
	dir, err := a.DataDir()
	if err != nil {
		return nil, err
	}
	return runner.New(filepath.Join(dir, "runs")), nil
}

func newRunCmd(app *App) *cobra.Command {
	var tool, project string
	var quiet time.Duration
	var notify bool
	cmd := &cobra.Command{
		Use:   "run [flags] -- <prompt> [prompt...]",
		Short: "Start parallel tasks in separate worktrees and tmux windows",
		Long: `Start one interactive agent per prompt, each on a new branch and worktree
at the project's current HEAD. Uncommitted changes are not copied. Runs start
detached; use ais run attach to join them. Each prompt must be one quoted argument.

Agent notifications highlight tmux windows when attention is needed. Codex
reports completed turns and approval requests; Claude reports idle prompts,
permission prompts and elicitation dialogs. Claude's notifications are delayed
by the agent and require hooks to be enabled. Use --notify=false to opt out.
Optional --quiet adds silence alerts; silence does not confirm agent state.
Stopping a run preserves its worktrees and branches for review.`,
		Example: `  ais run --tool claude -- "Fix login validation" "Add export tests"
  ais run --tool codex --project /path/to/repo -- "Improve search"
  ais run list --json
  ais run status ais-1234
  ais run diff ais-1234 --task task-1
  ais run attach ais-1234
  ais run stop ais-1234`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := app.runner()
			if err != nil {
				return err
			}
			m.Notifications = notify
			r, err := m.Start(cmd.Context(), project, model.Tool(tool), args, quiet)
			if err != nil {
				return err
			}
			fmt.Fprintf(app.Out, "Started %s: %d %s tasks\n", r.ID, len(r.Tasks), r.Tool)
			for _, task := range r.Tasks {
				fmt.Fprintf(app.Out, "  %s  %s\n", task.Branch, task.Path)
			}
			fmt.Fprintf(app.Out, "Attach: ais run attach %s\n", r.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&tool, "tool", "claude", "agent: claude or codex")
	cmd.Flags().StringVarP(&project, "project", "p", ".", "Git project directory")
	cmd.Flags().BoolVar(&notify, "notify", true, "enable agent event alerts in tmux")
	cmd.Flags().DurationVar(&quiet, "quiet", 0, "optional silence alerts after this interval (0s disables)")
	var asJSON bool
	list := &cobra.Command{
		Use: "list", Short: "List saved runs, including stopped runs", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			m, err := app.runner()
			if err != nil {
				return err
			}
			runs, err := m.List()
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(app.Out)
				enc.SetIndent("", "  ")
				return enc.Encode(runs)
			}
			if len(runs) == 0 {
				fmt.Fprintln(app.Out, "No saved runs.")
				return nil
			}
			w := tabwriter.NewWriter(app.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tTOOL\tTASKS\tCREATED\tPROJECT")
			for _, r := range runs {
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n", r.ID, r.Tool, len(r.Tasks), r.CreatedAt.Local().Format("2006-01-02 15:04"), r.Repository)
			}
			return w.Flush()
		},
	}
	list.Flags().BoolVar(&asJSON, "json", false, "output manifests as JSON, including branches, worktrees and prompts")
	var attachTask string
	attach := &cobra.Command{
		Use: "attach <run-id>", Short: "Attach to a run's tmux session", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := app.runner()
			if err != nil {
				return err
			}
			r, err := m.Find(args[0])
			if err != nil {
				return err
			}
			attachCmd, err := m.TaskAttachCmd(cmd.Context(), r, attachTask, os.Getenv("TMUX") != "")
			if err != nil {
				return err
			}
			return launch.Exec(attachCmd)
		},
	}
	attach.Flags().StringVar(&attachTask, "task", "", "jump to a task window (e.g. task-2)")
	stop := &cobra.Command{
		Use: "stop <run-id>", Short: "Stop a run's agents, keeping all branches and worktrees", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := app.runner()
			if err != nil {
				return err
			}
			r, err := m.Find(args[0])
			if err != nil {
				return err
			}
			if err := m.Stop(cmd.Context(), r); err != nil {
				return err
			}
			fmt.Fprintf(app.Out, "Stopped %s. Branches and worktrees are preserved.\n", r.ID)
			return nil
		},
	}
	cmd.AddCommand(list, attach, stop, newRunStatusCmd(app), newRunDiffCmd(app), newRunReviewCmd(app))
	return cmd
}
