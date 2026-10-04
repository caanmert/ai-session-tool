// Package cli implements the `ais` command line.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/caanmert/ai-session-tool/internal/clipboard"
	"github.com/caanmert/ai-session-tool/internal/index"
	"github.com/caanmert/ai-session-tool/internal/meta"
	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
	"github.com/caanmert/ai-session-tool/internal/provider/claude"
	"github.com/caanmert/ai-session-tool/internal/provider/codex"
	"github.com/caanmert/ai-session-tool/internal/trash"
	"github.com/caanmert/ai-session-tool/internal/tui"
	"github.com/caanmert/ai-session-tool/internal/ui"
)

// App holds the dependencies commands use, so tests can swap them.
type App struct {
	In        io.Reader
	Out, Err  io.Writer
	Now       func() time.Time
	Providers func() []provider.Provider
	Version   string
	// Interactive reports whether `ais` without arguments may open the TUI
	// (stdin and stdout are terminals). Nil means never.
	Interactive func() bool
	// IndexPath locates the session index. Nil disables the index.
	IndexPath func() (string, error)
	// DataDir locates ais's own data: annotations and the trash. Nil
	// disables both.
	DataDir func() (string, error)

	color   ui.Mode
	themes  map[io.Writer]*ui.Theme
	noIndex bool
	ix      *index.Index
	ixTried bool
	st      *meta.Store
	stErr   error
	stTried bool
}

// store opens the annotation store once.
func (a *App) store() (*meta.Store, error) {
	if a.stTried {
		return a.st, a.stErr
	}
	a.stTried = true
	if a.DataDir == nil {
		a.stErr = errors.New("annotations are disabled")
		return nil, a.stErr
	}
	dir, err := a.DataDir()
	if err == nil {
		a.st, err = meta.Open(filepath.Join(dir, "ais.db"))
	}
	a.stErr = err
	return a.st, err
}

// trash returns the trash in ais's data directory.
func (a *App) trash() (*trash.Trash, error) {
	if a.DataDir == nil {
		return nil, errors.New("the trash is disabled")
	}
	dir, err := a.DataDir()
	if err != nil {
		return nil, err
	}
	return trash.New(filepath.Join(dir, "trash")), nil
}

// annotate applies your annotations to sessions (pinned first). Without a
// store it leaves them as they are.
func (a *App) annotate(ctx context.Context) func(provider.ScanResult, error) (provider.ScanResult, error) {
	return func(res provider.ScanResult, err error) (provider.ScanResult, error) {
		return a.applyAnnotations(ctx, res, err)
	}
}

func (a *App) applyAnnotations(ctx context.Context, res provider.ScanResult, err error) (provider.ScanResult, error) {
	if err != nil {
		return res, err
	}
	st, serr := a.store()
	if serr != nil {
		return res, nil
	}
	ann, aerr := st.All(ctx)
	if aerr != nil {
		fmt.Fprintf(a.Err, "%s annotations unavailable: %v\n", a.themeFor(a.Err).Warn("ais:"), aerr)
		return res, nil
	}
	meta.Apply(res.Sessions, ann)
	return res, nil
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

// Close releases the index and the annotation store.
func (a *App) Close() error {
	var errs []error
	if a.ix != nil {
		errs = append(errs, a.ix.Close())
		a.ix, a.ixTried = nil, false
	}
	if a.st != nil {
		errs = append(errs, a.st.Close())
		a.st, a.stTried = nil, false
	}
	return errors.Join(errs...)
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
		In:        os.Stdin,
		Out:       os.Stdout,
		Err:       os.Stderr,
		Now:       time.Now,
		Providers: DefaultProviders,
		Version:   version,
		Interactive: func() bool {
			return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
		},
		IndexPath: index.DefaultPath,
		DataDir:   meta.DefaultDir,
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
		newRenameCmd(app),
		newTagCmd(app),
		newTagsCmd(app),
		newFlagCmd(app, "pin", "Keep a session at the top of every list", func(st *meta.Store, ctx context.Context, s model.Session) error { return st.SetPinned(ctx, s, true) }),
		newFlagCmd(app, "unpin", "Stop keeping a session at the top", func(st *meta.Store, ctx context.Context, s model.Session) error { return st.SetPinned(ctx, s, false) }),
		newFlagCmd(app, "archive", "Hide a session from lists (ls --all and is:archived still show it)", func(st *meta.Store, ctx context.Context, s model.Session) error { return st.SetArchived(ctx, s, true) }),
		newFlagCmd(app, "unarchive", "Show an archived session in lists again", func(st *meta.Store, ctx context.Context, s model.Session) error { return st.SetArchived(ctx, s, false) }),
		newTrashCmd(app),
		newRestoreCmd(app),
	)
	return root
}

// scan loads every session from every provider, through the index when
// it is available, with your annotations applied.
func (a *App) scan(ctx context.Context) (provider.ScanResult, error) {
	if ix := a.index(); ix != nil {
		return a.annotate(ctx)(ix.Sync(ctx, a.Providers()))
	}
	return a.annotate(ctx)(provider.Scan(ctx, a.Providers()))
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
	d.Sync = func(ctx context.Context) (provider.ScanResult, error) {
		return a.annotate(ctx)(provider.Scan(ctx, providers))
	}
	if st, err := a.store(); err == nil {
		d.Actions = &actions{app: a, st: st}
	}
	if ix := a.index(); ix != nil {
		d.Load = func(ctx context.Context) (provider.ScanResult, error) {
			return a.annotate(ctx)(ix.Load(ctx, providers))
		}
		d.Sync = func(ctx context.Context) (provider.ScanResult, error) {
			return a.annotate(ctx)(ix.Sync(ctx, providers))
		}
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
