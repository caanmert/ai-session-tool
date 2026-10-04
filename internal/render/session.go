package render

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/textutil"
	"github.com/caanmert/ai-session-tool/internal/ui"
)

// Options choose which transcript blocks are shown.
type Options struct {
	Tools    bool // tool output
	Thinking bool // reasoning blocks
}

// visible reports whether a transcript message is shown with options o.
func visible(m model.Message, o Options) bool {
	switch m.Kind {
	case model.KindThinking:
		return o.Thinking && strings.TrimSpace(m.Text) != ""
	case model.KindToolResult:
		return o.Tools
	}
	return true
}

// PlainDetails writes a session's details as plain text. resume is the
// shell command that resumes it, or "".
func PlainDetails(w io.Writer, now time.Time, s model.Session, resume string) {
	fmt.Fprintln(w, s.Title)
	row := func(k, v string) { fmt.Fprintf(w, "  %-9s %s\n", k, v) }
	row("id", fmt.Sprintf("%s (%s)", s.ID, s.Tool))
	if s.Live != nil {
		row("live", fmt.Sprintf("running, pid %d, %s", s.Live.PID, s.Live.Status))
	}
	cwd := s.CWD
	if s.GitBranch != "" {
		cwd += "  (branch " + s.GitBranch + ")"
	}
	row("project", cwd)
	row("started", s.StartedAt.Local().Format("2006-01-02 15:04"))
	row("updated", fmt.Sprintf("%s (%s)", s.UpdatedAt.Local().Format("2006-01-02 15:04"), Ago(now, s.UpdatedAt)))
	if s.Model != "" || s.Version != "" {
		row("model", strings.TrimSpace(s.Model+"  "+VersionLabel(s.Version)))
	}
	row("messages", Plural(s.UserTurns, "prompt", "prompts")+", "+Plural(s.AssistantTurns, "reply", "replies"))
	u := s.Usage
	row("tokens", fmt.Sprintf("%s total (input %s, output %s, cache read %s, cache write %s)",
		Tokens(u.Total()), Tokens(u.Input), Tokens(u.Output), Tokens(u.CacheRead), Tokens(u.CacheWrite)))
	row("file", s.Path)
	if resume != "" {
		row("resume", resume)
	}
}

// PlainTranscript writes a conversation as plain text.
func PlainTranscript(w io.Writer, tool model.Tool, msgs []model.Message, o Options) {
	var lastRole model.Role
	for _, m := range msgs {
		if !visible(m, o) {
			continue
		}
		if m.Role != lastRole && m.Kind != model.KindToolResult {
			who := "you"
			if m.Role == model.RoleAssistant {
				who = string(tool)
			}
			stamp := ""
			if !m.Time.IsZero() {
				stamp = " · " + m.Time.Local().Format("Jan 2 15:04")
			}
			fmt.Fprintf(w, "\n── %s%s ──\n", who, stamp)
			lastRole = m.Role
		}
		switch m.Kind {
		case model.KindText:
			fmt.Fprintln(w, strings.TrimSpace(m.Text))
		case model.KindThinking:
			fmt.Fprintln(w, indent("(thinking) "+strings.TrimSpace(m.Text), "  "))
		case model.KindToolUse:
			fmt.Fprintf(w, "  ▸ %s  %s\n", m.ToolName, m.Text)
		case model.KindToolResult:
			fmt.Fprintln(w, indent(truncateLines(strings.TrimSpace(m.Text), 8), "    │ "))
		}
	}
}

// Details draws the styled session card, with the resume command below it
// so it can be copied without box-drawing characters.
func Details(w io.Writer, t *ui.Theme, now time.Time, s model.Session, resume string, width int) {
	type kv struct{ k, v string }
	valueWidth := width - 4 - 10 // card border and padding, key column
	project := textutil.TruncateMiddle(ui.Home(s.CWD), valueWidth/2)
	if s.GitBranch != "" {
		project += t.Faint("  on ") + t.Branch(s.GitBranch)
	}
	rows := []kv{
		{"id", t.ID(s.ID)},
		{"project", project},
		{"updated", Ago(now, s.UpdatedAt) + t.Faint("  ·  started "+s.StartedAt.Local().Format("Jan 2 15:04"))},
	}
	if s.Model != "" || s.Version != "" {
		rows = append(rows, kv{"model", strings.Join(nonEmpty(s.Model, VersionLabel(s.Version)), t.Faint("  ·  "))})
	}
	rows = append(rows,
		kv{"messages", Plural(s.UserTurns, "prompt", "prompts") + t.Faint("  ·  ") + Plural(s.AssistantTurns, "reply", "replies")},
		kv{"tokens", tokenSummary(t, s.Usage)},
		kv{"file", t.Faint(textutil.TruncateMiddle(ui.Home(s.Path), min(valueWidth, 64)))},
	)

	lines := []string{t.Bold(textutil.Truncate(s.Title, valueWidth+10)), Badge(t, s), ""}
	for _, r := range rows {
		lines = append(lines, t.Faint(fmt.Sprintf("%-9s", r.k))+" "+r.v)
	}
	widest := 0
	for _, l := range lines {
		widest = max(widest, ui.Width(l))
	}
	fmt.Fprintln(w, t.Card(strings.Join(lines, "\n"), min(width, widest+4)))
	if resume != "" {
		fmt.Fprintf(w, "  %s %s\n", t.Faint("resume"), t.OK(resume))
	}
}

// Badge renders the tool name, plus the running state for live sessions.
func Badge(t *ui.Theme, s model.Session) string {
	badge := t.Tool(s.Tool, "● "+string(s.Tool))
	if s.Live != nil {
		badge += t.Faint("  ·  ") + t.Live("running") + t.Faint(fmt.Sprintf(" (pid %d, %s)", s.Live.PID, s.Live.Status))
	}
	return badge
}

// Meta renders a session's details as two compact lines, for previews.
func Meta(t *ui.Theme, now time.Time, s model.Session, width int) string {
	project := textutil.TruncateMiddle(ui.Home(s.CWD), max(width/2, 20))
	if s.GitBranch != "" {
		project += t.Faint(" on ") + t.Branch(s.GitBranch)
	}
	sep := t.Faint("  ·  ")
	first := Badge(t, s) + sep + project
	second := strings.Join(nonEmpty(
		t.ID(ShortID(s.ID)),
		Ago(now, s.UpdatedAt),
		Plural(s.UserTurns, "prompt", "prompts")+", "+Plural(s.AssistantTurns, "reply", "replies"),
		Tokens(s.Usage.Total())+" tokens",
		s.Model,
	), sep)
	return first + "\n" + second
}

func tokenSummary(t *ui.Theme, u model.Usage) string {
	parts := []string{"in " + Tokens(u.Input), "out " + Tokens(u.Output)}
	if u.CacheRead > 0 {
		parts = append(parts, "cache read "+Tokens(u.CacheRead))
	}
	if u.CacheWrite > 0 {
		parts = append(parts, "cache write "+Tokens(u.CacheWrite))
	}
	return t.Bold(Tokens(u.Total())) + t.Faint("  ("+strings.Join(parts, " · ")+")")
}

// Transcript renders the conversation like a chat: a colored header per
// speaker, your prompts set off by a bar, replies rendered as markdown,
// tool calls as dim one-liners. It wraps at width columns.
func Transcript(w io.Writer, t *ui.Theme, tool model.Tool, msgs []model.Message, o Options, width int) {
	md := t.Markdown(width - 2)
	var lastRole model.Role
	for _, m := range msgs {
		if !visible(m, o) {
			continue
		}
		if m.Role != lastRole && m.Kind != model.KindToolResult {
			name := t.Bold(t.User("you"))
			if m.Role == model.RoleAssistant {
				name = t.Bold(t.Tool(tool, string(tool)))
			}
			stamp := ""
			if !m.Time.IsZero() {
				stamp = t.Faint("  " + m.Time.Local().Format("Jan 2 15:04"))
			}
			fmt.Fprintf(w, "\n%s%s\n", name, stamp)
			lastRole = m.Role
		}
		switch m.Kind {
		case model.KindText:
			text := strings.TrimSpace(m.Text)
			if m.Role == model.RoleUser {
				fmt.Fprintln(w, t.Bar(ui.UserColor(), text, width))
			} else {
				fmt.Fprintln(w, indentText(md.Render(text), "  "))
			}
		case model.KindThinking:
			fmt.Fprintln(w, indent(t.Italic("∴ "+strings.TrimSpace(m.Text)), "  "))
		case model.KindToolUse:
			line := textutil.Truncate(m.ToolName+"  "+m.Text, max(width-4, 10))
			name, rest, _ := strings.Cut(line, "  ")
			fmt.Fprintf(w, "  %s %s  %s\n", t.Tool(tool, "▸"), t.Bold(name), t.Faint(rest))
		case model.KindToolResult:
			fmt.Fprintln(w, t.Faint(indent(truncateLines(strings.TrimSpace(m.Text), 8), "    │ ")))
		}
	}
}

func nonEmpty(vals ...string) []string {
	var out []string
	for _, v := range vals {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

// indentText indents every non-empty line, leaving blank lines empty.
func indentText(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			lines[i] = prefix + l
		} else {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

func truncateLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n") + fmt.Sprintf("\n… %d more lines", len(lines)-n)
}
