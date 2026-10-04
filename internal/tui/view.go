package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/render"
	"github.com/caanmert/ai-session-tool/internal/textutil"
	"github.com/caanmert/ai-session-tool/internal/ui"
)

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	listH, previewH := m.layout()
	parts := []string{m.header()}
	parts = append(parts, m.list(listH)...)
	if previewH > 0 {
		parts = append(parts, m.separator())
		if m.help {
			parts = append(parts, fit(m.helpView(), previewH)...)
		} else {
			m.preview.Width, m.preview.Height = m.width, previewH
			parts = append(parts, fit(m.preview.View(), previewH)...)
		}
	}
	parts = append(parts, m.footer())
	for i, p := range parts {
		parts[i] = ansi.Truncate(p, m.width, "…")
	}
	return strings.Join(parts, "\n")
}

// fit returns exactly n lines of s, padding with empty lines.
func fit(s string, n int) []string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	return lines
}

func (m Model) header() string {
	t := m.d.Theme
	title := t.Bold(" ais")
	var info string
	switch {
	case m.loading && !m.scanned:
		info = m.spinner.View() + t.Faint(" scanning sessions…")
	default:
		claude, codex := 0, 0
		for _, s := range m.sessions {
			if s.Tool == model.ToolCodex {
				codex++
			} else {
				claude++
			}
		}
		count := fmt.Sprintf("%d sessions", len(m.sessions))
		if !m.query.empty() {
			count = fmt.Sprintf("%d of %d sessions", len(m.view), len(m.sessions))
		}
		info = t.Faint(count+"  ·  ") + t.Tool(model.ToolClaude, fmt.Sprintf("claude %d", claude)) +
			t.Faint("  ") + t.Tool(model.ToolCodex, fmt.Sprintf("codex %d", codex))
		if m.loading {
			info += "  " + m.spinner.View()
		}
		if m.warnings > 0 {
			info += t.Faint("  ·  ") + t.Warn(fmt.Sprintf("%d warnings (ais doctor)", m.warnings))
		}
	}
	return title + "  " + info
}

// columns decides which list columns fit the terminal width.
type columns struct {
	project, branch, tokens bool
	titleW                  int
}

func (m Model) columns() columns {
	c := columns{project: m.width >= 70, branch: m.width >= 100, tokens: m.width >= 85}
	// cursor 2, live 2, tool 6, age 4, msgs 4, plus two-space gaps.
	used := 2 + 2 + 6 + 2 + 2 + 4 + 2 + 4
	if c.project {
		used += 14 + 2
	}
	if c.branch {
		used += 16 + 2
	}
	if c.tokens {
		used += 6 + 2
	}
	c.titleW = max(m.width-used, 10)
	return c
}

func (m Model) list(n int) []string {
	t := m.d.Theme
	if !m.scanned {
		return fit("", n)
	}
	if len(m.view) == 0 {
		msg := "  no sessions found — run `ais doctor` to see where ais looks"
		if !m.query.empty() {
			msg = "  no sessions match — esc clears the filter"
		}
		return fit(t.Faint(msg), n)
	}
	c := m.columns()
	now := m.d.Now()
	var rows []string
	for i := m.top; i < len(m.view) && i < m.top+n; i++ {
		s := m.sessions[m.view[i]]
		sel := i == m.cursor
		pad := func(text string, w int, right bool) string {
			text = textutil.Truncate(text, w)
			gap := strings.Repeat(" ", max(w-ui.Width(text), 0))
			if right {
				return gap + text
			}
			return text + gap
		}

		cursor := "  "
		if sel {
			cursor = t.Tool(s.Tool, "▌ ")
			if !t.Color {
				cursor = "> "
			}
		}
		live := "  "
		if s.Live != nil {
			live = t.Live("● ")
		}
		title := pad(s.Title, c.titleW, false)
		if sel {
			title = t.Bold(title)
		}
		row := cursor + live + t.Tool(s.Tool, pad(string(s.Tool), 6, false)) + "  " + title
		if c.project {
			row += "  " + t.Faint(pad(s.Project(), 14, false))
		}
		if c.branch {
			row += "  " + t.Branch(pad(s.GitBranch, 16, false))
		}
		row += "  " + t.Age(now, s.UpdatedAt, pad(render.Age(now, s.UpdatedAt), 4, true))
		row += "  " + t.Faint(pad(fmt.Sprint(s.MessageCount()), 4, true))
		if c.tokens {
			row += "  " + t.Faint(pad(render.Tokens(s.Usage.Total()), 6, true))
		}
		rows = append(rows, row)
	}
	return fit(strings.Join(rows, "\n"), n)
}

func (m Model) separator() string {
	t := m.d.Theme
	label := " preview "
	if m.help {
		label = " keys "
	}
	if m.focus == focusPreview && !m.help {
		label = " preview · scrolling (tab to go back) "
	}
	left := "──"
	rest := max(m.width-ui.Width(left+label), 0)
	if m.focus == focusPreview && !m.help {
		return t.Faint(left) + t.Bold(label) + t.Faint(strings.Repeat("─", rest))
	}
	return t.Faint(left + label + strings.Repeat("─", rest))
}

func (m Model) footer() string {
	t := m.d.Theme
	if m.filtering {
		return m.filter.View()
	}
	if m.status != "" {
		if m.statusError {
			return " " + t.Error(m.status)
		}
		return " " + t.OK(m.status)
	}
	type hint struct {
		text string
		prio int // lower stays longer on narrow screens
	}
	key := func(k, desc string, prio int) hint { return hint{t.Bold(k) + " " + t.Faint(desc), prio} }
	hints := []hint{
		key("enter", "resume", 0), key("f", "fork", 3), key("n", "new", 4), key("y", "copy", 5),
		key("/", "filter", 1), key("tab", "preview", 6), key("?", "keys", 2), key("q", "quit", 2),
	}
	if !m.query.empty() {
		hints = append([]hint{{t.Faint("filter: ") + m.filter.Value(), 0}, key("esc", "clear", 1)}, hints...)
	}
	sep := t.Faint("  ·  ")
	// Drop the least important hints until the line fits.
	for maxPrio := 6; maxPrio >= 0; maxPrio-- {
		var parts []string
		for _, h := range hints {
			if h.prio <= maxPrio {
				parts = append(parts, h.text)
			}
		}
		line := " " + strings.Join(parts, sep)
		if ui.Width(line) <= m.width || maxPrio == 0 {
			return line
		}
	}
	return ""
}

func (m Model) helpView() string {
	t := m.d.Theme
	section := func(name string) string { return "\n " + t.Bold(name) }
	k := func(keys, desc string) string {
		return fmt.Sprintf("   %s  %s", t.ID(fmt.Sprintf("%-14s", keys)), desc)
	}
	return strings.Join([]string{
		section("Sessions"),
		k("↑/↓  j/k", "move"),
		k("pgup/pgdn", "page"),
		k("g/G", "first / last"),
		k("enter", "resume in its project directory; you come back here when it exits"),
		k("f", "fork: continue in a new session, keep the original"),
		k("n", "start a new session in the same project with the same tool"),
		k("y", "copy the resume command"),
		k("r", "rescan"),
		section("Filter"),
		k("/", "type to filter; enter keeps it, esc clears it"),
		"   " + t.Faint("words match title, prompts, project, branch, id and model;"),
		"   " + t.Faint("t:codex  p:api  b:main  is:live narrow by tool, project, branch, running"),
		section("Preview"),
		k("tab", "focus the preview, then ↑/↓ pgup/pgdn g/G to scroll"),
		k("J/K", "scroll the preview from the list"),
		section("Other"),
		k("?", "toggle this help"),
		k("q  ctrl+c", "quit"),
	}, "\n")
}
