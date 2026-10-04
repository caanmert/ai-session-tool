package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/caanmert/ai-session-tool/internal/launch"
	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
	"github.com/caanmert/ai-session-tool/internal/textutil"
	"github.com/caanmert/ai-session-tool/internal/ui"
)

// contentWidth is how wide styled output may get: the terminal width,
// capped so prose stays readable on very wide screens.
func contentWidth(t *ui.Theme) int {
	if t.Width <= 0 {
		return 100
	}
	return min(t.Width, 110)
}

// prettyDetails draws the session card, with the resume command below it
// so it can be copied without box-drawing characters.
func prettyDetails(w io.Writer, t *ui.Theme, now time.Time, s model.Session, p provider.Provider) {
	badge := t.Tool(s.Tool, "● "+string(s.Tool))
	if s.Live != nil {
		badge += t.Faint("  ·  ") + t.Live("running") + t.Faint(fmt.Sprintf(" (pid %d, %s)", s.Live.PID, s.Live.Status))
	}

	type kv struct{ k, v string }
	valueWidth := contentWidth(t) - 4 - 10 // card border and padding, key column
	project := textutil.TruncateMiddle(ui.Home(s.CWD), valueWidth/2)
	if s.GitBranch != "" {
		project += t.Faint("  on ") + t.Branch(s.GitBranch)
	}
	rows := []kv{
		{"id", t.ID(s.ID)},
		{"project", project},
		{"updated", ago(now, s.UpdatedAt) + t.Faint("  ·  started "+s.StartedAt.Local().Format("Jan 2 15:04"))},
	}
	if s.Model != "" || s.Version != "" {
		rows = append(rows, kv{"model", strings.Join(nonEmpty(s.Model, versionLabel(s.Version)), t.Faint("  ·  "))})
	}
	rows = append(rows,
		kv{"messages", plural(s.UserTurns, "prompt", "prompts") + t.Faint("  ·  ") + plural(s.AssistantTurns, "reply", "replies")},
		kv{"tokens", tokenSummary(t, s.Usage)},
		kv{"file", t.Faint(textutil.TruncateMiddle(ui.Home(s.Path), min(valueWidth, 64)))},
	)

	lines := []string{t.Bold(textutil.Truncate(s.Title, valueWidth+10)), badge, ""}
	for _, r := range rows {
		lines = append(lines, t.Faint(fmt.Sprintf("%-9s", r.k))+" "+r.v)
	}
	body := strings.Join(lines, "\n")

	widest := 0
	for _, l := range lines {
		widest = max(widest, ui.Width(l))
	}
	fmt.Fprintln(w, t.Card(body, min(contentWidth(t), widest+4)))

	if cmd, err := p.ResumeCmd(s, false); err == nil {
		fmt.Fprintf(w, "  %s %s\n", t.Faint("resume"), t.OK(launch.ShellLine(cmd)))
	}
}

func tokenSummary(t *ui.Theme, u model.Usage) string {
	parts := []string{"in " + tokens(u.Input), "out " + tokens(u.Output)}
	if u.CacheRead > 0 {
		parts = append(parts, "cache read "+tokens(u.CacheRead))
	}
	if u.CacheWrite > 0 {
		parts = append(parts, "cache write "+tokens(u.CacheWrite))
	}
	return t.Bold(tokens(u.Total())) + t.Faint("  ("+strings.Join(parts, " · ")+")")
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

// prettyTranscript renders the conversation like a chat: a colored header
// per speaker, your prompts set off by a bar, replies rendered as markdown,
// tool calls as dim one-liners.
func prettyTranscript(w io.Writer, t *ui.Theme, tool model.Tool, msgs []model.Message, o showOptions) {
	width := contentWidth(t)
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
			fmt.Fprintf(w, "  %s %s  %s\n", t.Tool(tool, "▸"), t.Bold(m.ToolName), t.Faint(m.Text))
		case model.KindToolResult:
			fmt.Fprintln(w, t.Faint(indent(truncateLines(strings.TrimSpace(m.Text), 8), "    │ ")))
		}
	}
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
