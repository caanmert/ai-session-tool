// Package cli implements the `ais` command line.
package cli

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/caanmert/ai-session-tool/internal/provider"
	"github.com/caanmert/ai-session-tool/internal/provider/claude"
	"github.com/caanmert/ai-session-tool/internal/provider/codex"
)

// App holds the dependencies commands use, so tests can swap them.
type App struct {
	Out, Err  io.Writer
	Now       func() time.Time
	Providers func() []provider.Provider
	Version   string
}

// DefaultApp wires the real filesystem, clock and terminal.
func DefaultApp(version string) *App {
	return &App{
		Out:       os.Stdout,
		Err:       os.Stderr,
		Now:       time.Now,
		Providers: DefaultProviders,
		Version:   version,
	}
}

// DefaultProviders returns one provider per supported tool, rooted at the
// tool's standard location (overridable with the tool's own env var).
func DefaultProviders() []provider.Provider {
	return []provider.Provider{
		claude.New(claude.DefaultRoot()),
		codex.New(codex.DefaultRoot()),
	}
}

// NewRootCmd builds the command tree.
func NewRootCmd(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:   "ais",
		Short: "Find, preview and resume your Claude Code and Codex sessions",
		Long: `ais indexes the sessions Claude Code and Codex CLI keep on disk so you can
find any of them from anywhere, see what it was about, and jump back in.

Run without arguments to list recent sessions (the interactive TUI is coming).`,
		Version:       app.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd.Context(), app, listOptions{limit: 30})
		},
	}
	root.SetOut(app.Out)
	root.SetErr(app.Err)
	root.AddCommand(
		newListCmd(app),
		newShowCmd(app),
		newResumeCmd(app),
		newDoctorCmd(app),
	)
	return root
}

// scan loads every session from every provider.
func (a *App) scan(ctx context.Context) (provider.ScanResult, error) {
	return provider.Scan(ctx, a.Providers())
}
