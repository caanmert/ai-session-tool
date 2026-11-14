package main

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// fakeTool answers like an agent's command-line tool, using a shell script.
func fakeTool(t *testing.T, script string) *chatManager {
	t.Helper()
	m := newChats("")
	t.Cleanup(m.close)
	m.command = func(_ string, args ...string) *exec.Cmd {
		return exec.Command("sh", append([]string{"-c", script, "tool"}, args...)...)
	}
	return m
}

func TestAgentHealth(t *testing.T) {
	ctx := context.Background()

	ok := fakeTool(t, `echo "2.1.325 (Claude Code)"`).health(ctx, "claude")
	if ok.Version != "2.1.325" || ok.Tested != "2.1.296" || ok.Problem != "" {
		t.Fatalf("a newer version that still works is simply reported: %+v", ok)
	}

	same := fakeTool(t, `echo "codex-cli 0.162.1"`).health(ctx, "codex")
	if same.Version != "0.162.1" || same.Problem != "" {
		t.Fatalf("the tested version: %+v", same)
	}

	// The version prints, but the options ais sends are no longer accepted.
	rejected := fakeTool(t, `if [ "$1" = "--version" ]; then echo "3.0.0"; else echo "error: unknown option '--permission-prompt-tool'" >&2; exit 1; fi`).health(ctx, "claude")
	if rejected.Version != "3.0.0" || !strings.Contains(rejected.Problem, "unknown option '--permission-prompt-tool'") {
		t.Fatalf("a rejected option should be reported: %+v", rejected)
	}

	broken := fakeTool(t, `echo "cannot start" >&2; exit 3`).health(ctx, "codex")
	if broken.Version != "" || !strings.Contains(broken.Problem, "version could not be read") {
		t.Fatalf("an unreadable version: %+v", broken)
	}

	// The probe passes exactly the options a conversation starts with.
	var seen []string
	m := newChats("")
	t.Cleanup(m.close)
	m.command = func(_ string, args ...string) *exec.Cmd {
		seen = args
		return exec.Command("sh", "-c", "echo 2.1.296")
	}
	m.health(ctx, "claude")
	joined := strings.Join(seen, " ")
	for _, option := range []string{"--permission-prompt-tool stdio", "--permission-mode default", "--input-format stream-json", "--version"} {
		if !strings.Contains(joined, option) {
			t.Errorf("probe is missing %q: %s", option, joined)
		}
	}
}
