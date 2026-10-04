package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/caanmert/ai-session-tool/internal/meta"
	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/render"
	"github.com/caanmert/ai-session-tool/internal/trash"
)

// done prints a one-line confirmation: "<verb> <id> <title>".
func (a *App) done(verb string, s model.Session) {
	t := a.theme()
	fmt.Fprintf(a.Out, "%s %s  %s\n", t.OK(verb), t.ID(render.ShortID(s.ID)), render.Title(t, s, 200, false))
}

// annotateOne resolves id, applies change and reports it.
func (a *App) annotateOne(ctx context.Context, id, verb string, change func(*meta.Store, context.Context, model.Session) error) error {
	st, err := a.store()
	if err != nil {
		return err
	}
	s, _, err := a.lookup(ctx, id)
	if err != nil {
		return err
	}
	if err := change(st, ctx, s); err != nil {
		return err
	}
	res, err := a.scan(ctx) // re-read to report the result as it now is
	if err == nil {
		for _, ns := range res.Sessions {
			if ns.Key() == s.Key() {
				s = ns
			}
		}
	}
	a.done(verb, s)
	return nil
}

func newFlagCmd(app *App, name, short string, change func(*meta.Store, context.Context, model.Session) error) *cobra.Command {
	verb := name + "ned"
	switch name {
	case "archive", "unarchive":
		verb = name + "d"
	}
	return &cobra.Command{
		Use:   name + " <id>...",
		Short: short,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, id := range args {
				if err := app.annotateOne(cmd.Context(), id, verb, change); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRenameCmd(app *App) *cobra.Command {
	var reset bool
	cmd := &cobra.Command{
		Use:   "rename <id> <title...>",
		Short: "Give a session your own title",
		Long:  "Give a session your own title. It is stored by ais; the tool's files are not changed. --reset goes back to the tool's title.",
		Example: `  ais rename 3f2a Token refresh fix
  ais rename 3f2a --reset`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			title := strings.TrimSpace(strings.Join(args[1:], " "))
			if title == "" && !reset {
				return errors.New("give a title, or --reset to use the tool's title")
			}
			if reset {
				title = ""
			}
			return app.annotateOne(cmd.Context(), args[0], "renamed", func(st *meta.Store, ctx context.Context, s model.Session) error {
				return st.SetTitle(ctx, s, title)
			})
		},
	}
	cmd.Flags().BoolVar(&reset, "reset", false, "drop your title and use the tool's")
	return cmd
}

func newTagCmd(app *App) *cobra.Command {
	var remove bool
	cmd := &cobra.Command{
		Use:   "tag <id> <tag>...",
		Short: "Tag a session (filter with #tag in the browser, --tag in ls)",
		Example: `  ais tag 3f2a bug auth
  ais tag 3f2a --remove auth`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			tags := args[1:]
			for _, tag := range tags {
				if _, err := meta.NormalizeTag(tag); err != nil {
					return err
				}
			}
			verb := "tagged"
			if remove {
				verb = "untagged"
			}
			return app.annotateOne(cmd.Context(), args[0], verb, func(st *meta.Store, ctx context.Context, s model.Session) error {
				if remove {
					return st.Tag(ctx, s, nil, tags)
				}
				return st.Tag(ctx, s, tags, nil)
			})
		},
	}
	cmd.Flags().BoolVarP(&remove, "remove", "r", false, "remove these tags instead")
	return cmd
}

func newTagsCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "tags",
		Short: "List your tags and how many sessions carry each",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := app.store()
			if err != nil {
				return err
			}
			counts, err := st.TagCounts(cmd.Context())
			if err != nil {
				return err
			}
			if len(counts) == 0 {
				fmt.Fprintln(app.Err, "no tags yet: ais tag <id> <tag>")
				return nil
			}
			tags := make([]string, 0, len(counts))
			for tag := range counts {
				tags = append(tags, tag)
			}
			sort.Strings(tags)
			t := app.theme()
			for _, tag := range tags {
				fmt.Fprintf(app.Out, "%s  %s\n", t.Branch("#"+tag), t.Faint(fmt.Sprint(counts[tag])))
			}
			return nil
		},
	}
}

func newTrashCmd(app *App) *cobra.Command {
	var (
		empty     bool
		olderThan string
		yes       bool
	)
	cmd := &cobra.Command{
		Use:   "trash [<id>...]",
		Short: "Move sessions to the trash, or list it",
		Long: `Move sessions to the trash, so they no longer show up here or in the
tool's own resume picker. Nothing is lost: ais restore brings them back.

Claude Code sessions are moved into ais's trash folder. Codex sessions are
archived with Codex's own "codex archive".

Without arguments, list the trash. --empty deletes trashed sessions for good.`,
		Example: `  ais trash 3f2a
  ais trash
  ais trash --empty --older-than 30d`,
		RunE: func(cmd *cobra.Command, args []string) error {
			tr, err := app.trash()
			if err != nil {
				return err
			}
			switch {
			case empty:
				if len(args) > 0 {
					return errors.New("--empty takes no session ids")
				}
				return app.emptyTrash(cmd.Context(), tr, olderThan, yes)
			case len(args) == 0:
				return app.listTrash(tr)
			}
			for _, id := range args {
				s, _, err := app.lookup(cmd.Context(), id)
				if err != nil {
					return err
				}
				if _, err := tr.Put(cmd.Context(), s); err != nil {
					return fmt.Errorf("%s: %w", render.ShortID(s.ID), err)
				}
				app.done("trashed", s)
			}
			fmt.Fprintln(app.Err, app.theme().Faint("restore with: ais restore <id>"))
			return nil
		},
	}
	cmd.Flags().BoolVar(&empty, "empty", false, "permanently delete trashed sessions")
	cmd.Flags().StringVar(&olderThan, "older-than", "", "with --empty, only sessions trashed longer ago than this (e.g. 30d)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "with --empty, don't ask for confirmation")
	return cmd
}

func (a *App) listTrash(tr *trash.Trash) error {
	entries, err := tr.List()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(a.Err, "the trash is empty")
		return nil
	}
	t, now := a.theme(), a.Now()
	for _, e := range entries {
		title := e.Title
		if e.Pending {
			title += " (archive incomplete; restore to recover)"
		}
		fmt.Fprintf(a.Out, "%s  %s  %s  %s\n", t.ID(render.ShortID(e.ID)), t.Tool(e.Tool, fmt.Sprintf("%-6s", e.Tool)),
			t.Faint(fmt.Sprintf("%-4s", render.Age(now, e.TrashedAt))), title)
	}
	return nil
}

func (a *App) emptyTrash(ctx context.Context, tr *trash.Trash, olderThan string, yes bool) error {
	entries, err := tr.List()
	if err != nil {
		return err
	}
	if olderThan != "" {
		cutoff, err := parseSince(olderThan, a.Now())
		if err != nil {
			return err
		}
		var keep []trash.Entry
		for _, e := range entries {
			if e.TrashedAt.Before(cutoff) {
				keep = append(keep, e)
			}
		}
		entries = keep
	}
	if len(entries) == 0 {
		fmt.Fprintln(a.Err, "nothing to delete")
		return nil
	}
	if !yes {
		fmt.Fprintf(a.Err, "Permanently delete %s? This can't be undone. [y/N] ", render.Plural(len(entries), "trashed session", "trashed sessions"))
		answer, _ := bufio.NewReader(a.In).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			return errors.New("cancelled")
		}
	}
	deleted := 0
	for _, e := range entries {
		if err := tr.Delete(ctx, e); err != nil {
			return fmt.Errorf("deleted %d, then %s: %w", deleted, render.ShortID(e.ID), err)
		}
		deleted++
	}
	fmt.Fprintf(a.Out, "%s %s\n", a.theme().OK("deleted"), render.Plural(deleted, "session", "sessions"))
	return nil
}

func newRestoreCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "restore <id>...",
		Short: "Bring sessions back from the trash",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tr, err := app.trash()
			if err != nil {
				return err
			}
			for _, id := range args {
				e, err := tr.Find(id)
				if err != nil {
					return err
				}
				if err := tr.Restore(cmd.Context(), e); err != nil {
					return err
				}
				app.done("restored", model.Session{ID: e.ID, Title: e.Title})
			}
			return nil
		},
	}
}

// actions lets the TUI change annotations and trash sessions.
type actions struct {
	app *App
	st  *meta.Store
}

func (x *actions) Rename(ctx context.Context, s model.Session, title string) error {
	return x.st.SetTitle(ctx, s, title)
}

func (x *actions) Tag(ctx context.Context, s model.Session, add, remove []string) error {
	return x.st.Tag(ctx, s, add, remove)
}

func (x *actions) SetPinned(ctx context.Context, s model.Session, pinned bool) error {
	return x.st.SetPinned(ctx, s, pinned)
}

func (x *actions) SetArchived(ctx context.Context, s model.Session, archived bool) error {
	return x.st.SetArchived(ctx, s, archived)
}

func (x *actions) Trash(ctx context.Context, s model.Session) error {
	tr, err := x.app.trash()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	_, err = tr.Put(ctx, s)
	return err
}
