package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/caanmert/ai-session-tool/internal/launch"
	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
	"github.com/caanmert/ai-session-tool/internal/render"
	"github.com/caanmert/ai-session-tool/internal/ui"
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
		s.Breakdown = nil // ais stats reports usage over time
		enc := json.NewEncoder(app.Out)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Session  model.Session   `json:"session"`
			Messages []model.Message `json:"messages,omitempty"`
		}{s, msgs})
	}

	resume := ""
	if cmd, err := p.ResumeCmd(s, false); err == nil {
		resume = launch.ShellLine(cmd)
	}
	opts := render.Options{Tools: o.tools, Thinking: o.thinking}
	if t := app.theme(); t.Color {
		width := contentWidth(t)
		render.Details(app.Out, t, app.Now(), s, resume, width)
		if !o.info {
			render.Transcript(app.Out, t, s.Tool, msgs, opts, width)
		}
		return nil
	}
	render.PlainDetails(app.Out, app.Now(), s, resume)
	if !o.info {
		fmt.Fprintln(app.Out)
		render.PlainTranscript(app.Out, s.Tool, msgs, opts)
	}
	return nil
}

// contentWidth is how wide styled output may get: the terminal width,
// capped so prose stays readable on very wide screens.
func contentWidth(t *ui.Theme) int {
	if t.Width <= 0 {
		return 100
	}
	return min(t.Width, 110)
}
