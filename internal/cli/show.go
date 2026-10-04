package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/caanmert/ai-session-tool/internal/launch"
	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
)

type showOptions struct {
	json     bool
	tools    bool
	thinking bool
	info     bool
}

func newShowCmd(app *App) *cobra.Command {
	var o showOptions
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show a session's details and transcript",
		Long:  "Show a session's details and transcript. <id> may be any unique prefix of the session id.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShow(cmd.Context(), app, args[0], o)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&o.json, "json", false, "print the session and transcript as JSON")
	f.BoolVar(&o.tools, "tools", false, "include tool output in the transcript")
	f.BoolVar(&o.thinking, "thinking", false, "include thinking blocks in the transcript")
	f.BoolVar(&o.info, "info", false, "print only the details, no transcript")
	return cmd
}

// lookup scans all providers and resolves an id prefix to one session.
func (a *App) lookup(ctx context.Context, id string) (model.Session, provider.Provider, error) {
	res, err := a.scan(ctx)
	if err != nil {
		return model.Session{}, nil, err
	}
	s, err := provider.Find(res.Sessions, id)
	if err != nil {
		return model.Session{}, nil, err
	}
	p := provider.For(a.Providers(), s.Tool)
	if p == nil {
		return model.Session{}, nil, fmt.Errorf("no provider for %s", s.Tool)
	}
	return s, p, nil
}

func runShow(ctx context.Context, app *App, id string, o showOptions) error {
	s, p, err := app.lookup(ctx, id)
	if err != nil {
		return err
	}
	var msgs []model.Message
	if !o.info {
		if msgs, err = p.Transcript(ctx, s); err != nil {
			return err
		}
	}

	if o.json {
		enc := json.NewEncoder(app.Out)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Session  model.Session   `json:"session"`
			Messages []model.Message `json:"messages,omitempty"`
		}{s, msgs})
	}

	printDetails(app.Out, app.Now(), s, p)
	if o.info {
		return nil
	}
	fmt.Fprintln(app.Out)
	printTranscript(app.Out, s.Tool, msgs, o)
	return nil
}

func printDetails(w io.Writer, now time.Time, s model.Session, p provider.Provider) {
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
	ago := age(now, s.UpdatedAt)
	if ago == "now" {
		ago = "just now"
	} else {
		ago += " ago"
	}
	row("updated", fmt.Sprintf("%s (%s)", s.UpdatedAt.Local().Format("2006-01-02 15:04"), ago))
	if s.Model != "" || s.Version != "" {
		row("model", strings.TrimSpace(s.Model+"  "+versionLabel(s.Version)))
	}
	row("messages", fmt.Sprintf("%d prompts, %d replies", s.UserTurns, s.AssistantTurns))
	u := s.Usage
	row("tokens", fmt.Sprintf("%s total (input %s, output %s, cache read %s, cache write %s)",
		tokens(u.Total()), tokens(u.Input), tokens(u.Output), tokens(u.CacheRead), tokens(u.CacheWrite)))
	row("file", s.Path)
	if cmd, err := p.ResumeCmd(s, false); err == nil {
		row("resume", launch.ShellLine(cmd))
	}
}

func versionLabel(v string) string {
	if v == "" {
		return ""
	}
	return "v" + v
}

func printTranscript(w io.Writer, tool model.Tool, msgs []model.Message, o showOptions) {
	var lastRole model.Role
	for _, m := range msgs {
		switch m.Kind {
		case model.KindThinking:
			if !o.thinking || strings.TrimSpace(m.Text) == "" {
				continue
			}
		case model.KindToolResult:
			if !o.tools {
				continue
			}
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

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
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
