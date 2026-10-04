package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/caanmert/ai-session-tool/internal/index"
	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/render"
	"github.com/caanmert/ai-session-tool/internal/textutil"
	"github.com/caanmert/ai-session-tool/internal/ui"
)

var errNoIndex = errors.New("this needs the session index (drop --no-index)")

func newSearchCmd(app *App) *cobra.Command {
	var (
		tool   string
		limit  int
		asJSON bool
	)
	cmd := &cobra.Command{
		Use:   "search <words...>",
		Short: "Search everything said in every session",
		Long: `Search titles, projects, prompts, replies and tool calls of every session.
Every term must appear; words match as prefixes ("retr" finds "retries") and
"quoted text" matches as a phrase. Best matches come first.`,
		Example: `  ais search webhook retry
  ais search '"exponential backoff"' --tool codex
  ais search migration --json | jq -r '.[0].session.id'`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(cmd.Context(), app, strings.Join(args, " "), tool, limit, asJSON)
		},
	}
	cmd.Flags().StringVar(&tool, "tool", "", "only sessions of this tool (claude, codex)")
	cmd.Flags().IntVarP(&limit, "limit", "n", 20, "maximum results (0 = all)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func runSearch(ctx context.Context, app *App, q, tool string, limit int, asJSON bool) error {
	ix := app.index()
	if ix == nil {
		return errNoIndex
	}
	res, err := ix.Sync(ctx, app.Providers())
	if err != nil {
		return err
	}
	hits, err := ix.Search(ctx, q, 0)
	if err != nil {
		return err
	}
	sessions := map[string]model.Session{}
	for _, s := range res.Sessions {
		sessions[index.Key(s.Tool, s.ID)] = s
	}

	type result struct {
		Session model.Session `json:"session"`
		Snippet string        `json:"snippet,omitempty"`
	}
	var results []result
	seen := map[string]bool{}
	add := func(s model.Session, snippet string) {
		if seen[s.Key()] || tool != "" && string(s.Tool) != tool || limit > 0 && len(results) == limit {
			return
		}
		seen[s.Key()] = true
		results = append(results, result{s, snippet})
	}
	for _, h := range hits {
		if s, ok := sessions[index.Key(h.Tool, h.ID)]; ok {
			add(s, h.Snippet)
		}
	}
	// Your own titles and tags are not in the index; match them here.
	for _, s := range res.Sessions {
		if s.OriginalTitle != "" || len(s.Tags) > 0 {
			if annotationMatches(s, q) {
				add(s, "")
			}
		}
	}

	if asJSON {
		for i := range results {
			results[i].Snippet = stripMarks(results[i].Snippet)
			results[i].Session.Breakdown = nil
		}
		if results == nil {
			results = []result{}
		}
		enc := json.NewEncoder(app.Out)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}
	if len(results) == 0 {
		fmt.Fprintf(app.Err, "no sessions match %q\n", q)
		return nil
	}

	t, now := app.theme(), app.Now()
	width := t.Width
	if width <= 0 {
		width = 120
	}
	projectW := 0
	for _, r := range results {
		projectW = max(projectW, ui.Width(textutil.Truncate(r.Session.Project(), 14)))
	}
	pad := func(s string, w int) string { return s + strings.Repeat(" ", max(w-ui.Width(s), 0)) }
	for i, r := range results {
		s := r.Session
		if i > 0 {
			fmt.Fprintln(app.Out)
		}
		project := textutil.Truncate(s.Project(), 14)
		age := render.Age(now, s.UpdatedAt)
		head := t.ID(render.ShortID(s.ID)) + "  " + t.Tool(s.Tool, pad(string(s.Tool), 6)) + "  " +
			t.Bold(pad(project, projectW)) + "  " + t.Age(now, s.UpdatedAt, pad(age, 4)) + "  "
		fmt.Fprintln(app.Out, head+render.Title(t, s, max(width-ui.Width(head), 20), false))
		if r.Snippet != "" {
			fmt.Fprintln(app.Out, "    "+highlight(t, textutil.Truncate(r.Snippet, max(width-6, 20))))
		}
	}
	return nil
}

// highlight renders snippet matches; plain output just drops the markers.
func highlight(t *ui.Theme, snippet string) string {
	var b strings.Builder
	for {
		start := strings.Index(snippet, index.MarkStart)
		if start < 0 {
			b.WriteString(t.Faint(strings.ReplaceAll(snippet, index.MarkEnd, "")))
			return b.String()
		}
		b.WriteString(t.Faint(snippet[:start]))
		rest := snippet[start+len(index.MarkStart):]
		end := strings.Index(rest, index.MarkEnd)
		if end < 0 {
			end = len(rest)
		}
		b.WriteString(t.Highlight(rest[:end]))
		snippet = strings.TrimPrefix(rest[end:], index.MarkEnd)
	}
}

func stripMarks(s string) string {
	return strings.NewReplacer(index.MarkStart, "", index.MarkEnd, "").Replace(s)
}

func newReindexCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "reindex",
		Short: "Rebuild the session index from scratch",
		Long:  "Delete the session index and rebuild it by reading every transcript. The index is only a cache, so this is always safe.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if app.noIndex || app.IndexPath == nil {
				return errNoIndex
			}
			path, err := app.IndexPath()
			if err != nil {
				return err
			}
			if err := app.Close(); err != nil {
				return err
			}
			for _, suffix := range []string{"", "-wal", "-shm"} {
				if err := os.Remove(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
			ix := app.index()
			if ix == nil {
				return errNoIndex
			}
			start := time.Now()
			res, err := ix.Sync(cmd.Context(), app.Providers())
			if err != nil {
				return err
			}
			parsed, _ := ix.LastSync()
			t := app.theme()
			fmt.Fprintf(app.Out, "%s %d sessions from %d transcripts in %s\n  %s\n",
				t.OK("indexed"), len(res.Sessions), parsed, time.Since(start).Round(time.Millisecond), t.Faint(path))
			return nil
		},
	}
}

// annotationMatches reports whether every word of q appears in your title
// or tags for s.
func annotationMatches(s model.Session, q string) bool {
	hay := strings.ToLower(s.Title + " " + strings.Join(s.Tags, " "))
	words := strings.Fields(strings.ToLower(strings.ReplaceAll(q, `"`, " ")))
	for _, w := range words {
		if !strings.Contains(hay, strings.TrimPrefix(w, "#")) {
			return false
		}
	}
	return len(words) > 0
}
