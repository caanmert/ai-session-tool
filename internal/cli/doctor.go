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
	w := app.Out
	fmt.Fprintf(w, "ais %s\n", app.Version)
	for _, p := range app.Providers() {
		fmt.Fprintf(w, "\n[%s]\n", p.Tool())
		row := func(k, v string) { fmt.Fprintf(w, "  %-12s %s\n", k, v) }

		root := p.Root()
		if _, err := os.Stat(root); err != nil {
			row("root", root+"  (not found)")
		} else {
			row("root", root)
		}
		row("binary", binaryInfo(ctx, string(p.Tool())))

		files, err := p.Discover(ctx)
		if err != nil {
			row("transcripts", "error: "+err.Error())
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
		row("sessions", fmt.Sprintf("%d (%d empty skipped)", len(res.Sessions), res.Empty))
		row("live", fmt.Sprint(live))
		row("warnings", fmt.Sprint(len(res.Warnings)))
		shown := res.Warnings
		if !verbose && len(shown) > 10 {
			shown = shown[:10]
		}
		for _, warn := range shown {
			fmt.Fprintf(w, "    %s\n", warn)
		}
		if len(shown) < len(res.Warnings) {
			fmt.Fprintf(w, "    … %d more (use -v)\n", len(res.Warnings)-len(shown))
		}
	}
	return nil
}

// binaryInfo reports where a tool's executable is and its version.
func binaryInfo(ctx context.Context, name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return "not on PATH (resume will fail)"
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return path
	}
	return fmt.Sprintf("%s  (%s)", path, strings.TrimSpace(string(out)))
}
