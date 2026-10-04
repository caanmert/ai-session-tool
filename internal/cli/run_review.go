package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/caanmert/ai-session-tool/internal/runner"
)

func newRunStatusCmd(app *App) *cobra.Command {
	var task string
	var asJSON bool
	cmd := &cobra.Command{
		Use: "status <run-id>", Short: "Inspect task panes, alerts, branches and changed files", Args: cobra.ExactArgs(1),
		Long: `Show each task's tmux pane state and net file changes from the run's starting
commit, including untracked files. Event and silence alerts are shown separately.
Missing panes and unavailable inspection are reported explicitly. Pane state
does not describe a detached daemon's work or prove an agent is waiting.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, r, err := findRunTasks(app, args[0], task)
			if err != nil {
				return err
			}
			status, err := m.Status(cmd.Context(), r)
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(app.Out)
				enc.SetIndent("", "  ")
				return enc.Encode(status)
			}
			fmt.Fprintf(app.Out, "%s  changes since %s\n", r.ID, r.Base)
			w := tabwriter.NewWriter(app.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "TASK\tPANE\tALERT\tBRANCH\tFILES")
			for _, st := range status.Tasks {
				state := st.State
				if st.ExitCode != nil {
					state += fmt.Sprintf(" (%d)", *st.ExitCode)
				}
				if st.Signal != "" {
					state += " (signal " + st.Signal + ")"
				}
				alerts := []string{}
				if st.Attention {
					alerts = append(alerts, "event")
				}
				if st.Quiet {
					alerts = append(alerts, "silence")
				}
				alert := "-"
				if len(alerts) > 0 {
					alert = strings.Join(alerts, ",")
				}
				branch := st.CurrentBranch
				if branch == "" {
					branch = st.Task.Branch + " (saved)"
				}
				files := fmt.Sprint(len(st.Files))
				if st.Error != "" {
					files = "unavailable"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", st.Task.Window, state, alert, branch, files)
			}
			if err := w.Flush(); err != nil {
				return err
			}
			for _, st := range status.Tasks {
				fmt.Fprintf(app.Out, "\n%s: %s\n", st.Task.Window, st.Task.Path)
				for _, file := range st.Files {
					fmt.Fprintf(app.Out, "  %s %q\n", file.Status, file.Path)
				}
				if st.Error != "" {
					fmt.Fprintf(app.Out, "  Could not inspect worktree: %s\n", st.Error)
				}
				if st.Pane != "" {
					fmt.Fprintf(app.Out, "  Attach: ais run attach %s --task %s\n", r.ID, st.Task.Window)
				}
			}
			if status.Warning != "" {
				fmt.Fprintf(app.Err, "Pane inspection unavailable: %s\n", status.Warning)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&task, "task", "", "inspect only this task (e.g. task-2)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output task states, file paths and inspection errors as JSON")
	return cmd
}

func newRunDiffCmd(app *App) *cobra.Command {
	var task string
	var stat bool
	cmd := &cobra.Command{
		Use: "diff <run-id>", Short: "Review task changes from the run's starting commit", Args: cobra.ExactArgs(1),
		Long: `Show each task's committed and uncommitted changes from the saved starting
commit, plus untracked files. Ignored files are excluded. This reads worktrees
without staging, committing or merging. It also works after tmux has stopped.
Use --task to review one task, or --stat for a summary.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, r, err := findRunTasks(app, args[0], task)
			if err != nil {
				return err
			}
			var failures []error
			for _, t := range r.Tasks {
				if err := cmd.Context().Err(); err != nil {
					return err
				}
				fmt.Fprintf(app.Out, "=== %s / %s (base %s) ===\n", r.ID, t.Window, r.Base)
				diff, err := m.Diff(cmd.Context(), r, t, stat)
				if err != nil {
					fmt.Fprintf(app.Out, "Could not read diff: %s\n\n", err)
					failures = append(failures, fmt.Errorf("%s: %w", t.Window, err))
					continue
				}
				if diff == "" {
					fmt.Fprintln(app.Out, "No changes.")
				} else {
					fmt.Fprint(app.Out, diff)
				}
				fmt.Fprintln(app.Out)
			}
			return errors.Join(failures...)
		},
	}
	cmd.Flags().StringVar(&task, "task", "", "review only this task (e.g. task-2)")
	cmd.Flags().BoolVar(&stat, "stat", false, "show changed-file statistics instead of patches")
	return cmd
}

func findRunTasks(app *App, id, task string) (*runner.Manager, runner.Run, error) {
	m, err := app.runner()
	if err != nil {
		return nil, runner.Run{}, err
	}
	r, err := m.Find(id)
	if err != nil {
		return nil, runner.Run{}, err
	}
	r, err = runner.SelectTasks(r, task)
	return m, r, err
}
