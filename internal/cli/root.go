// Package cli implements the `ais` command line.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/caanmert/ai-session-tool/internal/clipboard"
	"github.com/caanmert/ai-session-tool/internal/index"
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
	// IndexPath locates the session index. Nil disables the index.
	IndexPath func() (string, error)

	color   ui.Mode
	themes  map[io.Writer]*ui.Theme
	noIndex bool
	ix      *index.Index
	ixTried bool
}

// index opens the session index once. It returns nil when the index is
// disabled or can't be opened; callers then scan transcripts directly.
func (a *App) index() *index.Index {
	if a.noIndex || a.IndexPath == nil {
		return nil
	}
	if a.ixTried {
		return a.ix
	}
	a.ixTried = true
	path, err := a.IndexPath()
	if err == nil {
		a.ix, err = index.Open(path)
	}
	if err != nil {
		fmt.Fprintf(a.Err, "%s index unavailable, reading transcripts directly: %v\n", a.themeFor(a.Err).Warn("ais:"), err)
	}
	return a.ix
}

// Close releases the index.
func (a *App) Close() error {
	if a.ix == nil {
		return nil
	}
	err := a.ix.Close()
	a.ix, a.ixTried = nil, false
	return err
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
		IndexPath: index.DefaultPath,
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
				return tui.Run(cmd.Context(), app.tuiDeps())
			}
			return runList(cmd.Context(), app, listOptions{limit: 30})
		},
	}
	var color string
	root.PersistentFlags().StringVar(&color, "color", "auto", "colorize output: auto, always or never (NO_COLOR is honored)")
	root.PersistentFlags().BoolVar(&app.noIndex, "no-index", false, "read every transcript instead of using the session index")
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
		newSearchCmd(app),
		newReindexCmd(app),
	)
	return root
}

// scan loads every session from every provider, through the index when
// it is available.
func (a *App) scan(ctx context.Context) (provider.ScanResult, error) {
	if ix := a.index(); ix != nil {
		return ix.Sync(ctx, a.Providers())
	}
	return provider.Scan(ctx, a.Providers())
}

// tuiDeps connects the TUI to the index (or a direct scan without one).
func (a *App) tuiDeps() tui.Deps {
	providers := a.Providers()
	d := tui.Deps{
		Providers: providers,
		Now:       a.Now,
		Theme:     a.theme(),
		Copy:      clipboard.Copy,
	}
	if ix := a.index(); ix != nil {
		d.Load = func(ctx context.Context) (provider.ScanResult, error) { return ix.Load(ctx, providers) }
		d.Sync = func(ctx context.Context) (provider.ScanResult, error) { return ix.Sync(ctx, providers) }
		d.Search = func(ctx context.Context, q string) (map[string]bool, error) {
			hits, err := ix.Search(ctx, q, 0)
			keys := map[string]bool{}
			for _, h := range hits {
				keys[index.Key(h.Tool, h.ID)] = true
			}
			return keys, err
		}
	}
	return d
}
