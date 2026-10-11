package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/caanmert/ai-session-tool/internal/model"
)

type dependency struct {
	name, purpose, path, hint string
}

func (a *App) lookPath(name string) (string, error) {
	if a.LookPath != nil {
		return a.LookPath(name)
	}
	return exec.LookPath(name)
}

func (a *App) dependencies() []dependency {
	deps := []dependency{
		{name: "claude", purpose: "resume Claude sessions and launch Claude tasks"},
		{name: "codex", purpose: "resume Codex sessions and launch Codex tasks"},
		{name: "git", purpose: "parallel tasks and change review"},
		{name: "tmux", purpose: "parallel tasks only"},
		{name: "lsof", purpose: "Codex live-session detection and safe trash operations"},
	}
	_, brewErr := a.lookPath("brew")
	for i := range deps {
		d := &deps[i]
		if path, err := a.lookPath(d.name); err == nil {
			d.path = path
		}
		d.hint = dependencyHint(d.name, runtime.GOOS, brewErr == nil)
	}
	return deps
}

func dependencyHint(name, goos string, brew bool) string {
	if goos == "darwin" && brew {
		switch name {
		case "claude":
			return "brew install --cask claude-code"
		case "codex":
			return "brew install --cask codex"
		}
	}
	if name == "claude" {
		return "Install Claude Code: https://code.claude.com/docs/en/setup; add claude to PATH."
	}
	if name == "codex" {
		return "Install Codex CLI: https://github.com/openai/codex#installation; add codex to PATH."
	}
	if brew {
		return "brew install " + name
	}
	if goos == "darwin" && name == "git" {
		return "xcode-select --install"
	}
	if goos == "darwin" && name == "lsof" {
		return "Check that /usr/sbin is on PATH (macOS includes /usr/sbin/lsof)."
	}
	return "Install " + name + " with your system package manager and add it to PATH."
}

func (a *App) printDependencies(deps []dependency) {
	for _, d := range deps {
		if d.path != "" {
			fmt.Fprintf(a.Out, "  [ok] %s: %s\n", d.name, d.path)
		} else {
			fmt.Fprintf(a.Out, "  [missing] %s: %s\n    Fix: %s\n", d.name, d.purpose, d.hint)
		}
	}
}

func newSetupCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Check available features and show how to get started",
		Long: `Inspect agent locations, saved transcripts and optional dependencies.
Shows next steps without installing software, changing settings or starting agents.
Missing optional tools are reported without causing a nonzero exit status.
Use ais doctor for a full transcript scan and runtime diagnostics.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSetup(cmd.Context(), app)
		},
	}
}

func runSetup(ctx context.Context, app *App) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fmt.Fprintln(app.Out, "ais setup — your local session manager")
	fmt.Fprintln(app.Out, "\nTools (you only need the agent you use)")
	deps := app.dependencies()
	app.printDependencies(deps)
	available := map[string]bool{}
	for _, d := range deps {
		available[d.name] = d.path != ""
	}

	fmt.Fprintln(app.Out, "\nSession locations")
	files, problems := 0, 0
	var resumable []string
	for _, p := range app.Providers() {
		fmt.Fprintf(app.Out, "  %s: %s\n", p.Tool(), p.Root())
		override := "CLAUDE_CONFIG_DIR"
		if p.Tool() == model.ToolCodex {
			override = "CODEX_HOME"
		}
		fmt.Fprintf(app.Out, "    Custom location: set %s to the agent's data directory.\n", override)
		info, err := os.Stat(p.Root())
		switch {
		case os.IsNotExist(err):
			fmt.Fprintf(app.Out, "    Not created yet. Run %s in a project and start a conversation, or check %s.\n", p.Tool(), override)
			continue
		case err != nil:
			fmt.Fprintf(app.Out, "    Cannot read location: %v\n", err)
			problems++
			continue
		case !info.IsDir():
			fmt.Fprintf(app.Out, "    Expected a directory. Check %s.\n", override)
			problems++
			continue
		}
		refs, err := p.Discover(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			fmt.Fprintf(app.Out, "    Cannot discover transcripts: %v\n", err)
			problems++
			continue
		}
		fmt.Fprintf(app.Out, "    %d transcript files found.\n", len(refs))
		if len(refs) == 0 {
			fmt.Fprintf(app.Out, "    Start a conversation with %s, or check %s if sessions are stored elsewhere.\n", p.Tool(), override)
		}
		files += len(refs)
		if len(refs) > 0 && available[string(p.Tool())] {
			resumable = append(resumable, string(p.Tool()))
		}
	}

	fmt.Fprintln(app.Out, "\nAvailable features")
	switch {
	case files > 0:
		fmt.Fprintf(app.Out, "  Browse/search: %d transcript files available to index; run ais.\n", files)
	case problems > 0:
		fmt.Fprintln(app.Out, "  Browse/search: fix the session location problems above, then rerun ais setup.")
	default:
		fmt.Fprintln(app.Out, "  Browse/search: waiting for your first saved conversation; no Go, Git or tmux needed.")
	}
	if len(resumable) > 0 {
		fmt.Fprintf(app.Out, "  Resume: CLI and transcript files found for %s.\n", strings.Join(resumable, ", "))
	} else {
		fmt.Fprintln(app.Out, "  Resume: needs a saved conversation and its matching agent on PATH.")
	}
	var missing []string
	for _, name := range []string{"git", "tmux"} {
		if !available[name] {
			missing = append(missing, name)
		}
	}
	if !available["claude"] && !available["codex"] {
		missing = append(missing, "claude or codex")
	}
	if len(missing) == 0 {
		fmt.Fprintln(app.Out, "  Parallel tasks: tools found; use a Git repository with at least one commit.")
	} else {
		fmt.Fprintf(app.Out, "  Parallel tasks: install %s (optional for browsing).\n", strings.Join(missing, ", "))
	}
	fmt.Fprintln(app.Out, "  Agent sign-in and permissions are managed by each agent; setup does not verify them.")
	fmt.Fprintln(app.Out, "\nNext steps\n  ais new      Start an agent in the current project.\n  ais continue Resume a conversation from the current project.\n  ais          Open the browser; / searches, Enter resumes, ? shows shortcuts.\n  ais ls       List sessions without an interactive terminal.\n  ais doctor   Diagnose parsing, live detection, storage and configuration.\n  ais run --help   Learn about parallel tasks.")
	return ctx.Err()
}
