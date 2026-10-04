// Command ais finds, previews and resumes Claude Code and Codex sessions.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/caanmert/ai-session-tool/internal/cli"
	"github.com/caanmert/ai-session-tool/internal/ui"
)

// version is set at build time by goreleaser (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := cli.DefaultApp(version)
	if err := cli.NewRootCmd(app).ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, ui.New(os.Stderr, ui.Auto).Error("ais:"), err)
		return 1
	}
	return 0
}
