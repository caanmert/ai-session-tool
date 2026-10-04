package tui

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/pricing"
	"github.com/caanmert/ai-session-tool/internal/render"
	"github.com/caanmert/ai-session-tool/internal/stats"
	"github.com/caanmert/ai-session-tool/internal/textutil"
	"github.com/caanmert/ai-session-tool/internal/ui"
)

// statsModes cycle with the s key; "" means the preview is shown.
var statsModes = []string{"", "day", "model", "project"}

func nextStatsMode(cur string) string {
	for i, m := range statsModes {
		if m == cur {
			return statsModes[(i+1)%len(statsModes)]
		}
	}
	return ""
}

// statsView renders usage of the sessions the filter shows, last 30 days.
func (m Model) statsView(height int) string {
	t := m.d.Theme
	prices := m.d.Prices
	if prices == nil {
		prices = pricing.Defaults()
	}
	now := m.d.Now()
	sessions := make([]model.Session, 0, len(m.view))
	for _, i := range m.view {
		sessions = append(sessions, m.sessions[i])
	}
	rep, err := stats.Build(sessions, prices, stats.Options{By: m.statsBy, Since: now.AddDate(0, 0, -30)})
	if err != nil {
		return " " + t.Error(err.Error())
	}
	scope := "all sessions"
	if !m.query.empty() {
		scope = "the filtered sessions"
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, " %s %s\n\n", t.Bold("Last 30 days of "+scope),
		t.Faint("· "+render.Plural(rep.Total.Sessions, "session", "sessions")+" · ≈ ")+t.Bold(costLabel(rep.Total)))
	if len(rep.Rows) == 0 {
		b.WriteString(" " + t.Faint("no token usage in this period"))
		return b.String()
	}

	rows := rep.Rows
	room := max(height-6, 1) // title, blank, table header, total, blank, footnote
	if len(rows) > room {
		if m.statsBy == "day" {
			rows = rows[len(rows)-room:] // most recent days
		} else {
			rows = rows[:room] // biggest groups
		}
	}
	var biggest int64
	for _, r := range rows {
		biggest = max(biggest, r.Usage.Total())
	}
	header := map[string]string{"day": "DAY", "model": "MODEL", "project": "PROJECT"}[m.statsBy]
	cols := []ui.Column{{Header: " " + header}, {Header: "SESSIONS", Right: true}, {Header: "TOKENS", Right: true}, {Header: "≈ COST", Right: true}, {Header: ""}}
	cell := func(r stats.Row, label string, total bool) []ui.Cell {
		bar := ""
		if !total && biggest > 0 {
			bar = strings.Repeat("▇", max(int(int64(max(m.width-70, 10))*r.Usage.Total()/biggest), 1))
		}
		labelStyle := func(s string) string { return s }
		if total {
			labelStyle = t.Bold
		}
		return []ui.Cell{
			{Text: " " + label, Style: labelStyle},
			{Text: fmt.Sprint(r.Sessions), Style: t.Faint},
			{Text: render.Tokens(r.Usage.Total()), Style: t.Bold},
			{Text: costLabel(r), Style: t.OK},
			{Text: bar, Style: t.ID},
		}
	}
	var cells [][]ui.Cell
	for _, r := range rows {
		cells = append(cells, cell(r, statsKey(m.statsBy, r.Key, now), false))
	}
	cells = append(cells, cell(rep.Total, "total", true))
	_ = t.Table(&b, cols, cells)
	note := "s: next grouping"
	if len(rep.Unpriced) > 0 {
		note = "+ excludes models without a price (see ais stats)  ·  " + note
	}
	b.WriteString("\n " + t.Faint(note))
	return b.String()
}

func costLabel(r stats.Row) string {
	switch {
	case !r.Priced:
		return "—"
	case r.Cost > 0 && r.Cost < 0.005:
		return "<$0.01"
	case !r.Complete:
		return fmt.Sprintf("$%.2f+", r.Cost)
	}
	return fmt.Sprintf("$%.2f", r.Cost)
}

func statsKey(by, key string, now time.Time) string {
	switch by {
	case "day":
		if d, err := time.ParseInLocation("2006-01-02", key, time.Local); err == nil {
			if d.Year() != now.Year() {
				return d.Format("Mon Jan 2 2006")
			}
			return d.Format("Mon Jan 2")
		}
	case "project":
		if key == "" {
			return "(unknown)"
		}
		return textutil.TruncateMiddle(ui.Home(key), 36)
	}
	return key
}
