package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/caanmert/ai-session-tool/internal/provider"
)

func newDoctorCmd(app *App) *cobra.Command {
	var verbose bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Show where ais looks for sessions and any parse problems",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDoctor(cmd.Context(), app, verbose)
		},
	}
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "list every warning")
	return cmd
}

func runDoctor(ctx context.Context, app *App, verbose bool) error {
	w, t := app.Out, app.theme()
	fmt.Fprintf(w, "%s %s\n", t.Bold("ais"), t.Faint(app.Version))
	for _, p := range app.Providers() {
		if t.Color {
			fmt.Fprintf(w, "\n%s\n", t.Bold(t.Tool(p.Tool(), "● "+string(p.Tool()))))
		} else {
			fmt.Fprintf(w, "\n[%s]\n", p.Tool())
		}
		row := func(k, v string) { fmt.Fprintf(w, "  %s %s\n", t.Faint(fmt.Sprintf("%-12s", k)), v) }

		root := p.Root()
		if _, err := os.Stat(root); err != nil {
			row("root", root+t.Warn("  (not found)"))
		} else {
			row("root", root)
		}
		if path, version, err := binaryInfo(ctx, string(p.Tool())); err != nil {
			row("binary", t.Warn("not on PATH (resume will fail)"))
		} else {
			row("binary", path+t.Faint(version))
		}

		files, err := p.Discover(ctx)
		if err != nil {
			row("transcripts", t.Error("error: "+err.Error()))
			continue
		}
		res, err := provider.Scan(ctx, []provider.Provider{p})
		if err != nil {
			return err
		}
		live := 0
		for _, s := range res.Sessions {
			if s.Live != nil {
				live++
			}
		}
		row("transcripts", fmt.Sprint(len(files)))
		skipped := fmt.Sprintf("%d empty", res.Empty)
		if res.Hidden > 0 {
			skipped += fmt.Sprintf(", %d subagent/internal", res.Hidden)
		}
		row("sessions", t.Bold(fmt.Sprint(len(res.Sessions)))+t.Faint(" ("+skipped+" skipped)"))
		liveText := fmt.Sprint(live)
		if live > 0 {
			liveText = t.Live(liveText)
		}
		row("live", liveText)
		if len(res.Warnings) == 0 {
			row("warnings", t.OK("0"))
		} else {
			row("warnings", t.Warn(fmt.Sprint(len(res.Warnings))))
		}
		shown := res.Warnings
		if !verbose && len(shown) > 10 {
			shown = shown[:10]
		}
		for _, warn := range shown {
			fmt.Fprintf(w, "    %s\n", t.Faint(warn.String()))
		}
		if len(shown) < len(res.Warnings) {
			fmt.Fprintf(w, "    … %d more (use -v)\n", len(res.Warnings)-len(shown))
		}
	}
	return nil
}

// binaryInfo finds a tool's executable and its version, formatted as
// "  (version)" or "" when it can't be read.
func binaryInfo(ctx context.Context, name string) (path, version string, err error) {
	path, err = exec.LookPath(name)
	if err != nil {
		return "", "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, verr := exec.CommandContext(ctx, path, "--version").Output()
	if verr != nil {
		return path, "", nil
	}
	return path, "  (" + strings.TrimSpace(string(out)) + ")", nil
}
