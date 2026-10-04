package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/textutil"
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
	const fixed = 2 + 8 + 2 + 6 + 2 + 14 + 2 + 14 + 2 + 10 + 2 + 5 + 2 + 7 + 2 // all columns but the title
	titleWidth := 80
	if w := width(app.Out); w > 0 {
		titleWidth = max(w-fixed, 20)
	}

	tw := tabwriter.NewWriter(app.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, " \tID\tTOOL\tPROJECT\tBRANCH\tUPDATED\tMSGS\tTOKENS\tTITLE")
	for _, s := range sessions {
		dot := " "
		if s.Live != nil {
			dot = "●"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			dot,
			shortID(s.ID),
			s.Tool,
			textutil.Truncate(s.Project(), 14),
			textutil.Truncate(s.GitBranch, 14),
			age(now, s.UpdatedAt),
			s.MessageCount(),
			tokens(s.Usage.Total()),
			textutil.Truncate(s.Title, titleWidth),
		)
	}
	tw.Flush()
}

// shortID is the id prefix shown in tables; any unique prefix resolves.
func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
