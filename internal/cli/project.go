package cli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/caanmert/ai-session-tool/internal/application"
	"github.com/caanmert/ai-session-tool/internal/launch"
	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
	"github.com/caanmert/ai-session-tool/internal/render"
)

func validateAgent(tool string) error              { return application.ValidateTool(tool) }
func projectDirectory(path string) (string, error) { return application.Directory(path) }
func (a *App) gitProjectRoot(ctx context.Context, dir string) string {
	return (application.Projects{LookPath: a.lookPath}).Root(ctx, dir)
}

func newNewCmd(app *App) *cobra.Command {
	var tool, project string
	var print bool
	cmd := &cobra.Command{
		Use: "new", Short: "Start an agent in the current project directory",
		Long: `Start a fresh agent conversation in the current directory, or --project.
Choose an installed agent interactively, or name one with --tool. If only one
agent is installed, it is selected automatically. Git and tmux are not required.
The agent takes over this terminal; exiting it returns to your shell.`,
		Example: "  ais new\n  ais new --tool codex\n  ais new --tool claude --project /path/to/project\n  ais new --tool codex --print",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateAgent(tool); err != nil {
				return err
			}
			dir, err := projectDirectory(project)
			if err != nil {
				return err
			}
			p, err := app.chooseAgent(cmd.Context(), tool, dir)
			if errors.Is(err, errChoiceCanceled) {
				return nil
			}
			if err != nil {
				return err
			}
			agentCmd, err := application.NewCommand([]provider.Provider{p}, string(p.Tool()), dir)
			if err != nil {
				return err
			}
			return app.startAgent(cmd.Context(), agentCmd, print)
		},
	}
	cmd.Flags().StringVar(&tool, "tool", "", "agent to launch (claude or codex)")
	cmd.Flags().StringVarP(&project, "project", "p", ".", "directory to start in (defaults to the current directory)")
	cmd.Flags().BoolVar(&print, "print", false, "print the shell command instead of starting the agent")
	return cmd
}

func (a *App) chooseAgent(ctx context.Context, tool, dir string) (provider.Provider, error) {
	providers := a.Providers()
	if tool != "" {
		p := provider.For(providers, model.Tool(tool))
		if p == nil {
			return nil, fmt.Errorf("no provider for %s", tool)
		}
		return p, nil
	}
	var choices []provider.Provider
	var labels []string
	for _, p := range providers {
		cmd := p.NewCmd(dir)
		if _, err := a.lookPath(cmd.Args[0]); err == nil {
			choices = append(choices, p)
			labels = append(labels, string(p.Tool()))
		}
	}
	switch len(choices) {
	case 0:
		return nil, errors.New("no agent found on PATH; run ais setup for installation instructions")
	case 1:
		return choices[0], nil
	}
	i, err := a.choose(ctx, "Start a new session in "+dir, labels)
	if err != nil {
		return nil, err
	}
	return choices[i], nil
}

func (a *App) startAgent(ctx context.Context, cmd *exec.Cmd, print bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if print {
		fmt.Fprintln(a.Out, launch.ShellLine(cmd))
		return nil
	}
	if a.Interactive == nil || !a.Interactive() {
		return errors.New("starting an interactive agent requires a terminal; use --print to preview the command")
	}
	if err := launch.Check(cmd); err != nil {
		return fmt.Errorf("%w (run ais setup to check installed tools)", err)
	}
	// Release database handles before replacing this process with the agent.
	if err := a.Close(); err != nil {
		return err
	}
	if a.Exec != nil {
		return a.Exec(cmd)
	}
	return launch.Exec(cmd)
}

func newContinueCmd(app *App) *cobra.Command {
	var tool, project string
	var latest, print bool
	cmd := &cobra.Command{
		Use: "continue [id]", Short: "Continue a session from the current project",
		Long: `Choose a saved session from the current Git worktree, including sessions
started in its subdirectories. Other worktrees and nested repositories are
excluded. Outside Git (or when Git is unavailable), match the directory exactly.
Archived sessions are hidden. Resume uses the selected session's recorded cwd.
Use --latest to resume the most recently updated match, or supply a unique ID
prefix. A single match is selected automatically. The agent takes over the terminal.`,
		Example: "  ais continue\n  ais continue --tool codex\n  ais continue --latest\n  ais continue 3f2a --print\n  ais continue --project /path/to/project",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if latest && len(args) > 0 {
				return errors.New("use either a session ID or --latest, not both")
			}
			if err := validateAgent(tool); err != nil {
				return err
			}
			dir, err := projectDirectory(project)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			sessions, root, err := app.projectSessions(ctx, dir, tool)
			if err != nil {
				return err
			}
			if len(sessions) == 0 {
				return fmt.Errorf("no matching sessions in %s; start one with ais new, or use ais resume <id> for another project", root)
			}
			selected := 0
			switch {
			case len(args) > 0:
				selected, err = projectSessionID(sessions, args[0])
			case !latest && len(sessions) > 1:
				labels := make([]string, len(sessions))
				for i, s := range sessions {
					labels[i] = fmt.Sprintf("%-6s  %-8.8s  %4s  %s  [%s]", s.Tool, s.ID, render.Age(app.Now(), s.UpdatedAt), s.Title, s.GitBranch)
				}
				selected, err = app.choose(ctx, "Continue a session in "+root, labels)
			}
			if errors.Is(err, errChoiceCanceled) {
				return nil
			}
			if err != nil {
				return err
			}
			s := sessions[selected]
			p := provider.For(app.Providers(), s.Tool)
			if p == nil {
				return fmt.Errorf("no provider for %s", s.Tool)
			}
			c, err := application.ResumeCommand([]provider.Provider{p}, s)
			if err != nil {
				return err
			}
			return app.startAgent(ctx, c, print)
		},
	}
	cmd.Flags().StringVar(&tool, "tool", "", "only sessions from this agent (claude or codex)")
	cmd.Flags().StringVarP(&project, "project", "p", ".", "project directory (defaults to the current directory)")
	cmd.Flags().BoolVar(&latest, "latest", false, "select the most recently updated matching session")
	cmd.Flags().BoolVar(&print, "print", false, "print the resume command instead of starting the agent")
	return cmd
}

func (a *App) projectSessions(ctx context.Context, dir, tool string) ([]model.Session, string, error) {
	root := a.gitProjectRoot(ctx, dir)
	res, err := a.scan(ctx)
	if err != nil {
		return nil, root, err
	}
	if len(res.Warnings) > 0 {
		fmt.Fprintf(a.Err, "ais: %d scan warnings (run ais doctor for details)\n", len(res.Warnings))
	}
	return (application.Projects{LookPath: a.lookPath}).Filter(ctx, dir, tool, res.Sessions)
}

func projectSessionID(sessions []model.Session, prefix string) (int, error) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return 0, errors.New("empty session ID")
	}
	var matches, exact []int
	for i, s := range sessions {
		id := strings.ToLower(s.ID)
		if strings.HasPrefix(id, prefix) {
			matches = append(matches, i)
		}
		if id == prefix {
			exact = append(exact, i)
		}
	}
	if len(exact) > 0 {
		matches = exact
	}
	if len(matches) != 1 {
		return 0, fmt.Errorf("session ID %q matches %d sessions in this project; use a longer ID or --tool", prefix, len(matches))
	}
	return matches[0], nil
}
