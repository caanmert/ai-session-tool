package runview

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/caanmert/ai-session-tool/internal/ui"
)

type layout struct {
	tasks, files, diff, fileWidth, diffWidth int
	wide                                     bool
}

func (m Model) layout() layout {
	l := layout{tasks: min(max(len(m.status.Tasks), 1), max(1, min(6, m.height/5))), wide: m.width >= 100}
	remaining := max(m.height-l.tasks-5, 3)
	if l.wide {
		l.fileWidth = min(40, m.width/3)
		l.diffWidth = max(m.width-l.fileWidth-3, 1)
		l.files, l.diff = remaining-1, remaining-1
	} else {
		l.fileWidth, l.diffWidth = max(m.width, 1), max(m.width, 1)
		l.files = min(4, max((remaining-2)/4, 1))
		l.diff = max(remaining-l.files-2, 1)
	}
	return l
}

func scrollTop(cursor, top, rows, count int) int {
	if cursor < top {
		top = cursor
	}
	if cursor >= top+rows {
		top = cursor - rows + 1
	}
	return min(max(top, 0), max(count-rows, 0))
}

func (m *Model) resize() {
	l := m.layout()
	m.taskTop = scrollTop(m.task, m.taskTop, l.tasks, len(m.status.Tasks))
	if st, ok := m.current(); ok {
		m.fileTop = scrollTop(m.file, m.fileTop, l.files, len(st.Files)+1)
	}
	m.preview.Width, m.preview.Height = l.diffWidth, l.diff
	m.preview.SetYOffset(m.preview.YOffset)
}

const maxDiffBytes = 512 * 1024
const maxDiffLines = 4000

func limitDiff(text string) string {
	truncated := len(text) > maxDiffBytes
	if truncated {
		text = strings.ToValidUTF8(text[:maxDiffBytes], "�")
	}
	lines := strings.SplitN(text, "\n", maxDiffLines+1)
	if len(lines) > maxDiffLines {
		text = strings.Join(lines[:maxDiffLines], "\n")
		truncated = true
	}
	if truncated {
		text += "\n\n… Preview truncated. Use ais run diff for the complete patch.\n"
	}
	return text
}

// Escape file-provided controls before adding our own styling. Raw patch text
// must not be able to emit terminal commands, clipboard escapes, or fake rows.
func safeText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteString("    ")
		case r != '\n' && unicode.IsControl(r):
			quoted := strconv.QuoteRune(r)
			b.WriteString(quoted[1 : len(quoted)-1])
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func inline(s string) string { return strings.ReplaceAll(safeText(s), "\n", "\\n") }

func (m *Model) renderDiff() {
	t := m.d.Theme
	text := m.diffText
	switch {
	case m.diffError != "":
		text = t.Error("Could not read diff: " + inline(m.diffError))
	case m.diffLoading && text == "":
		text = t.Faint("Loading diff…")
	case text == "":
		text = t.Faint("No changes from the starting commit.")
	default:
		lines := strings.Split(safeText(text), "\n")
		for i, line := range lines {
			switch {
			case strings.HasPrefix(line, "diff --git"), strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
				lines[i] = t.Bold(line)
			case strings.HasPrefix(line, "@@"):
				lines[i] = t.ID(line)
			case strings.HasPrefix(line, "+"):
				lines[i] = t.OK(line)
			case strings.HasPrefix(line, "-"):
				lines[i] = t.Error(line)
			}
		}
		text = strings.Join(lines, "\n")
	}
	offset := m.preview.YOffset
	m.preview.SetContent(text)
	m.preview.SetYOffset(offset)
}

func fitLines(s string, n int) []string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	return lines
}

func cell(s string, width int) string {
	s = ansi.Truncate(s, max(width, 0), "…")
	return s + strings.Repeat(" ", max(width-ui.Width(s), 0))
}

func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	t := m.d.Theme
	header := t.Bold(" ais run review ") + t.ID(inline(m.d.Run.ID))
	if m.loading {
		header += t.Faint(" · refreshing…")
	}
	if m.width < 30 || m.height < 12 {
		lines := fitLines(header+"\nEnlarge the terminal to review.\nq quit · a attach", m.height)
		for i, line := range lines {
			lines[i] = ansi.Truncate(line, m.width, "…")
		}
		return strings.Join(lines, "\n")
	}
	if m.help {
		body := `Navigate
  tab / shift+tab   switch tasks, files and diff panels
  up/down or j/k    select a task/file or scroll the diff
  pgup/pgdn        move by a page
  g/G              first/last item or top/bottom of diff
  h/l or left/right scroll the diff horizontally

Review
  All changes      preview the complete task patch
  enter on a file  focus its diff
  a                attach to the selected agent from any panel
  enter            attach (except in the files panel)
  r / ctrl+r       refresh status and the selected diff
  ? / esc          close help
  q / ctrl+c       quit review; agents keep running

Outside tmux: detach with ctrl+b d to return to review.
Inside tmux: switch back to the review window/session.
Event and silence alerts are separate; pane state is not agent state.
Review reads worktrees without staging, committing or merging.`
		lines := append([]string{header}, fitLines(body, m.height-2)...)
		lines = append(lines, t.Faint(" ? / esc close help · q quit"))
		for i, line := range lines {
			lines[i] = ansi.Truncate(line, m.width, "…")
		}
		return strings.Join(lines, "\n")
	}
	l := m.layout()
	label := func(name string, f focus) string {
		if m.focus == f {
			return t.Bold("▸ " + name)
		}
		return t.Faint("  " + name)
	}
	lines := []string{header, label("Tasks · pane state / alerts / files / branch", tasksFocus)}
	for i := 0; i < l.tasks; i++ {
		idx := m.taskTop + i
		if idx >= len(m.status.Tasks) {
			lines = append(lines, "")
			continue
		}
		st := m.status.Tasks[idx]
		branch := st.CurrentBranch
		if branch == "" {
			branch = st.Task.Branch
		}
		count := fmt.Sprintf("%d files", len(st.Files))
		if st.Error != "" {
			count = "files unavailable"
		}
		row := fmt.Sprintf("  %-10s %-26s %-17s %s", inline(st.Task.Window), inline(paneLabel(st)), count, inline(branch))
		if idx == m.task {
			row = "> " + row[2:]
			row = t.Bold(row)
		}
		lines = append(lines, row)
	}
	meta := "No tasks in this run."
	if st, ok := m.current(); ok {
		meta = fmt.Sprintf(" %s · %s", inline(st.Task.Window), strconv.Quote(st.Task.Path))
	}
	lines = append(lines, t.Faint(meta))
	message := m.notice
	switch {
	case m.statusError != "":
		message = "Refresh failed (previous snapshot): " + inline(m.statusError)
	case m.status.Warning != "":
		message = "Pane inspection unavailable: " + inline(m.status.Warning)
	case message == "":
		message = " Changes since " + m.d.Run.Base + " · read-only"
	}
	lines = append(lines, t.Warn(inline(message)))
	fileRows := m.fileRows(l.files)
	diffRows := fitLines(m.preview.View(), l.diff)
	diffName := m.filename()
	if diffName == "" {
		diffName = "All changes"
	} else {
		diffName = strconv.Quote(diffName)
	}
	diffTitle := "Diff · " + diffName
	if m.diffLoading {
		diffTitle += " · loading…"
	}
	if l.wide {
		lines = append(lines, cell(label("Files", filesFocus), l.fileWidth)+" │ "+label(diffTitle, diffFocus))
		for i := 0; i < l.diff; i++ {
			lines = append(lines, cell(fileRows[i], l.fileWidth)+" │ "+diffRows[i])
		}
	} else {
		lines = append(lines, label("Files", filesFocus))
		lines = append(lines, fileRows...)
		lines = append(lines, label(diffTitle, diffFocus))
		lines = append(lines, diffRows...)
	}
	footer := " q quit · tab panels · ↑↓ select · a attach · r refresh · ? keys"
	if m.focus == diffFocus {
		footer = " q quit · tab panels · ↑↓ scroll · ←→ pan · a attach · r refresh · ? keys"
	}
	lines = append(lines, t.Faint(footer))
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, m.width, "…")
	}
	return strings.Join(lines, "\n")
}

func (m Model) fileRows(n int) []string {
	rows := []string{}
	st, ok := m.current()
	if !ok {
		return fitLines("No task selected.", n)
	}
	for idx := m.fileTop; idx <= len(st.Files) && len(rows) < n; idx++ {
		text := "All changes"
		if idx > 0 {
			f := st.Files[idx-1]
			text = f.Status + " " + strconv.Quote(f.Path)
		}
		if idx == m.file {
			text = m.d.Theme.Bold("> " + text)
		} else {
			text = "  " + text
		}
		rows = append(rows, text)
	}
	return fitLines(strings.Join(rows, "\n"), n)
}
