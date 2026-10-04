// Package cli implements the `ais` command line.
package cli

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/caanmert/ai-session-tool/internal/clipboard"
	"github.com/caanmert/ai-session-tool/internal/provider"
	"github.com/caanmert/ai-session-tool/internal/provider/claude"
	"github.com/caanmert/ai-session-tool/internal/provider/codex"
	"github.com/caanmert/ai-session-tool/internal/tui"
	"github.com/caanmert/ai-session-tool/internal/ui"
)

// App holds the dependencies commands use, so tests can swap them.
type App struct {
	Out, Err  io.Writer
	Now       func() time.Time
	Providers func() []provider.Provider
	Version   string
	// Interactive reports whether `ais` without arguments may open the TUI
	// (stdin and stdout are terminals). Nil means never.
	Interactive func() bool

	color  ui.Mode
	themes map[io.Writer]*ui.Theme
}

// theme returns the output theme for stdout.
func (a *App) theme() *ui.Theme { return a.themeFor(a.Out) }

func (a *App) themeFor(w io.Writer) *ui.Theme {
	if a.themes == nil {
		a.themes = map[io.Writer]*ui.Theme{}
	}
	if t, ok := a.themes[w]; ok {
		return t
	}
	t := ui.New(w, a.color)
	a.themes[w] = t
	return t
}

// DefaultApp wires the real filesystem, clock and terminal.
func DefaultApp(version string) *App {
	return &App{
		Out:       os.Stdout,
		Err:       os.Stderr,
		Now:       time.Now,
		Providers: DefaultProviders,
		Version:   version,
		Interactive: func() bool {
			return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
		},
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

Run without arguments in a terminal to browse sessions interactively: filter,
preview, then resume or fork with one key. Piped, it prints the recent list.`,
		Version:       app.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if app.Interactive != nil && app.Interactive() {
				return tui.Run(cmd.Context(), tui.Deps{
					Providers: app.Providers(),
					Now:       app.Now,
					Theme:     app.theme(),
					Copy:      clipboard.Copy,
				})
			}
			return runList(cmd.Context(), app, listOptions{limit: 30})
		},
	}
	var color string
	root.PersistentFlags().StringVar(&color, "color", "auto", "colorize output: auto, always or never (NO_COLOR is honored)")
	root.PersistentPreRunE = func(*cobra.Command, []string) error {
		mode, err := ui.ParseMode(color)
		app.color = mode
		return err
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
