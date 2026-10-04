package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/pricing"
	"github.com/caanmert/ai-session-tool/internal/render"
	"github.com/caanmert/ai-session-tool/internal/stats"
	"github.com/caanmert/ai-session-tool/internal/textutil"
	"github.com/caanmert/ai-session-tool/internal/ui"
)

func newStatsCmd(app *App) *cobra.Command {
	var (
		by, since, tool string
		asJSON          bool
	)
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Token usage and estimated cost by day, week, month, project, model or tool",
		Long: `Token usage and estimated cost, grouped by day (default), week, month,
project, model or tool.

Cost is an estimate at API list prices (≈). Current Claude models are
priced out of the box; add other models, such as the ones Codex uses, under
[prices] in ~/.config/ais/config.toml. On a subscription you pay a flat fee
instead, so read cost as "what this would cost on the API".`,
		Example: `  ais stats
  ais stats --by model --since 7d
  ais stats --by project --since all --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStats(cmd.Context(), app, by, since, tool, asJSON)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&by, "by", "b", "day", "group by "+strings.Join(stats.Dimensions, ", "))
	f.StringVar(&since, "since", "30d", `only usage since then (30m, 12h, 7d, 2w, 2006-01-02, or "all")`)
	f.StringVar(&tool, "tool", "", "only this tool (claude, codex)")
	f.BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

// prices loads the pricing table, warning about a broken config.
func (a *App) prices() (*pricing.Table, string) {
	if a.ConfigPath == nil {
		return pricing.Defaults(), ""
	}
	path, err := a.ConfigPath()
	if err != nil {
		return pricing.Defaults(), ""
	}
	tab, err := pricing.Load(path)
	if err != nil {
		fmt.Fprintf(a.Err, "%s %v (using built-in prices)\n", a.themeFor(a.Err).Warn("ais:"), err)
	}
	return tab, path
}

func runStats(ctx context.Context, app *App, by, since, tool string, asJSON bool) error {
	if !stats.Valid(by) {
		return fmt.Errorf("unknown --by %q: use %s", by, strings.Join(stats.Dimensions, ", "))
	}
	if tool != "" && tool != string(model.ToolClaude) && tool != string(model.ToolCodex) {
		return fmt.Errorf("unknown --tool %q: use claude or codex", tool)
	}
	now := app.Now()
	var from time.Time
	if since != "all" && since != "" {
		var err error
		if from, err = parseSince(since, now); err != nil {
			return err
		}
	}
	res, err := app.scan(ctx)
	if err != nil {
		return err
	}
	prices, configPath := app.prices()
	rep, err := stats.Build(res.Sessions, prices, stats.Options{By: by, Since: from, Tool: model.Tool(tool)})
	if err != nil {
		return err
	}

	if asJSON {
		if rep.Rows == nil {
			rep.Rows = []stats.Row{}
		}
		enc := json.NewEncoder(app.Out)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}

	t := app.theme()
	period := "all time"
	if !from.IsZero() {
		period = "since " + from.Local().Format("Jan 2, 2006")
	}
	if len(rep.Rows) == 0 {
		fmt.Fprintf(app.Err, "no token usage %s\n", period)
		return nil
	}
	fmt.Fprintf(app.Out, "%s %s\n\n", t.Bold("Usage "+period),
		t.Faint("· "+render.Plural(rep.Total.Sessions, "session", "sessions")+" · ≈ ")+t.Bold(costText(rep.Total)))

	header := map[string]string{"day": "DAY", "week": "WEEK OF", "month": "MONTH", "project": "PROJECT", "model": "MODEL", "tool": "TOOL"}[by]
	cols := []ui.Column{
		{Header: header}, {Header: "SESSIONS", Right: true}, {Header: "INPUT", Right: true}, {Header: "OUTPUT", Right: true},
		{Header: "CACHE READ", Right: true}, {Header: "CACHE WRITE", Right: true}, {Header: "TOTAL", Right: true}, {Header: "≈ COST", Right: true},
	}
	if t.Color {
		cols = append(cols, ui.Column{Header: ""})
	}
	var biggest int64
	for _, r := range rep.Rows {
		biggest = max(biggest, r.Usage.Total())
	}
	row := func(r stats.Row, label string, emphasize bool) []ui.Cell {
		style := t.Faint
		if emphasize {
			style = t.Bold
		}
		cells := []ui.Cell{
			{Text: label, Style: func(s string) string {
				if emphasize {
					return t.Bold(s)
				}
				return s
			}},
			{Text: fmt.Sprint(r.Sessions), Style: style},
			{Text: render.Tokens(r.Usage.Input), Style: style},
			{Text: render.Tokens(r.Usage.Output), Style: style},
			{Text: render.Tokens(r.Usage.CacheRead), Style: style},
			{Text: render.Tokens(r.Usage.CacheWrite), Style: style},
			{Text: render.Tokens(r.Usage.Total()), Style: t.Bold},
			{Text: costText(r), Style: func(s string) string { return t.OK(s) }},
		}
		if t.Color {
			bar := ""
			if !emphasize && biggest > 0 {
				bar = strings.Repeat("▇", max(int(20*r.Usage.Total()/biggest), 1))
			}
			cells = append(cells, ui.Cell{Text: bar, Style: t.ID})
		}
		return cells
	}
	var rows [][]ui.Cell
	for _, r := range rep.Rows {
		rows = append(rows, row(r, statsLabel(by, r.Key, now), false))
	}
	rows = append(rows, row(rep.Total, "total", true))
	if err := t.Table(app.Out, cols, rows); err != nil {
		return err
	}

	fmt.Fprintln(app.Out)
	if len(rep.Unpriced) > 0 {
		where := "~/.config/ais/config.toml"
		if configPath != "" {
			where = ui.Home(configPath)
		}
		fmt.Fprintf(app.Out, "%s\n", t.Faint(fmt.Sprintf("+ excludes models without a price: %s (add them under [prices] in %s)",
			strings.Join(rep.Unpriced, ", "), where)))
	}
	fmt.Fprintln(app.Out, t.Faint("≈ cost at API list prices; on a subscription you pay a flat fee instead"))
	return nil
}

// costText renders a row's estimated cost: "—" when nothing had a price,
// a trailing "+" when some usage had none.
func costText(r stats.Row) string {
	if !r.Priced {
		return "—"
	}
	s := fmt.Sprintf("$%.2f", r.Cost)
	if r.Cost > 0 && r.Cost < 0.005 {
		s = "<$0.01"
	}
	if !r.Complete {
		s += "+"
	}
	return s
}

func statsLabel(by, key string, now time.Time) string {
	switch by {
	case "project":
		if key == "" {
			return "(unknown)"
		}
		return textutil.TruncateMiddle(ui.Home(key), 40)
	case "week", "day":
		if t, err := time.ParseInLocation("2006-01-02", key, time.Local); err == nil {
			if t.Year() != now.Local().Year() {
				return t.Format("Mon Jan 2 2006")
			}
			return t.Format("Mon Jan 2")
		}
	}
	return key
}
