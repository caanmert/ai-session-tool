package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/textutil"
	"github.com/caanmert/ai-session-tool/internal/ui"
)

type listOptions struct {
	tool    string
	project string
	since   string
	live    bool
	limit   int
	json    bool
}

func newListCmd(app *App) *cobra.Command {
	var o listOptions
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List sessions, newest first",
		Example: `  ais ls
  ais ls --tool claude --project api --since 7d
  ais ls --json | jq '.[0].id'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd.Context(), app, o)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.tool, "tool", "", "only sessions of this tool (claude, codex)")
	f.StringVarP(&o.project, "project", "p", "", "only sessions whose cwd contains this text")
	f.StringVar(&o.since, "since", "", "only sessions updated within this window (30m, 12h, 7d, 2w) or since a date (2006-01-02)")
	f.BoolVar(&o.live, "live", false, "only sessions whose agent is running now")
	f.IntVarP(&o.limit, "limit", "n", 50, "maximum sessions to show (0 = all)")
	f.BoolVar(&o.json, "json", false, "print JSON instead of a table")
	return cmd
}

func runList(ctx context.Context, app *App, o listOptions) error {
	now := app.Now()
	var since time.Time
	if o.since != "" {
		var err error
		if since, err = parseSince(o.since, now); err != nil {
			return err
		}
	}
	if o.tool != "" && o.tool != string(model.ToolClaude) && o.tool != string(model.ToolCodex) {
		return fmt.Errorf("unknown --tool %q: use claude or codex", o.tool)
	}

	res, err := app.scan(ctx)
	if err != nil {
		return err
	}
	sessions := make([]model.Session, 0, len(res.Sessions))
	for _, s := range res.Sessions {
		switch {
		case o.tool != "" && string(s.Tool) != o.tool:
		case o.project != "" && !strings.Contains(strings.ToLower(s.CWD), strings.ToLower(o.project)):
		case !since.IsZero() && s.UpdatedAt.Before(since):
		case o.live && s.Live == nil:
		default:
			sessions = append(sessions, s)
		}
	}
	total := len(sessions)
	if o.limit > 0 && len(sessions) > o.limit {
		sessions = sessions[:o.limit]
	}

	if o.json {
		enc := json.NewEncoder(app.Out)
		enc.SetIndent("", "  ")
		return enc.Encode(sessions)
	}
	if len(sessions) == 0 {
		fmt.Fprintln(app.Err, "no sessions found (run `ais doctor` to check where ais is looking)")
		return nil
	}
	printTable(app, now, sessions)
	if total > len(sessions) {
		fmt.Fprintf(app.Err, "showing %d of %d sessions (use -n 0 for all)\n", len(sessions), total)
	}
	return nil
}

func printTable(app *App, now time.Time, sessions []model.Session) {
	t := app.theme()
	cols := []ui.Column{
		{Header: " "}, {Header: "ID"}, {Header: "TOOL"}, {Header: "PROJECT"}, {Header: "BRANCH"},
		{Header: "UPDATED"}, {Header: "MSGS", Right: true}, {Header: "TOKENS", Right: true}, {Header: "TITLE"},
	}
	const fixed = 1 + 8 + 6 + 14 + 14 + 7 + 4 + 6 + 8*2 // every column but the title, plus gaps
	titleWidth := 80
	if t.Width > 0 {
		titleWidth = max(t.Width-fixed, 20)
	}

	rows := make([][]ui.Cell, 0, len(sessions))
	for _, s := range sessions {
		dot := " "
		if s.Live != nil {
			dot = "●"
		}
		updated := s.UpdatedAt
		rows = append(rows, []ui.Cell{
			{Text: dot, Style: t.Live},
			{Text: shortID(s.ID), Style: t.ID},
			{Text: string(s.Tool), Style: func(x string) string { return t.Tool(s.Tool, x) }},
			{Text: textutil.Truncate(s.Project(), 14), Style: t.Bold},
			{Text: textutil.Truncate(s.GitBranch, 14), Style: t.Branch},
			{Text: age(now, updated), Style: func(x string) string { return t.Age(now, updated, x) }},
			{Text: fmt.Sprint(s.MessageCount()), Style: t.Faint},
			{Text: tokens(s.Usage.Total()), Style: t.Faint},
			{Text: textutil.Truncate(s.Title, titleWidth)},
		})
	}
	_ = t.Table(app.Out, cols, rows)
}

// shortID is the id prefix shown in tables; any unique prefix resolves.
func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
