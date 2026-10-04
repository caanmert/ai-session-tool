package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/caanmert/ai-session-tool/internal/model"
)

func repository(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-qm", "Initial"},
	} {
		if _, err := command(context.Background(), dir, "git", args...); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestPrepareWorktreesAndPreserveOnLaunchFailure(t *testing.T) {
	repo := repository(t)
	m := New(filepath.Join(t.TempDir(), "runs"))
	m.LookPath = func(bin string) (string, error) { return "/test/" + bin, nil }
	var calls [][]string
	m.Command = func(ctx context.Context, cwd, bin string, args ...string) (string, error) {
		if bin == "git" {
			return command(ctx, cwd, bin, args...)
		}
		calls = append(calls, args)
		if args[0] == "respawn-pane" {
			return "", errors.New("simulated failure")
		}
		return "%1", nil
	}
	r, err := m.Start(context.Background(), repo, model.ToolClaude, []string{"first", "second"}, 30*time.Second)
	if err == nil || !strings.Contains(err.Error(), "work is preserved") {
		t.Fatalf("error = %v", err)
	}
	if len(calls) == 0 {
		t.Fatal("tmux was not reached")
	}
	for _, task := range r.Tasks {
		branch, err := command(context.Background(), task.Path, "git", "branch", "--show-current")
		if err != nil || branch != task.Branch {
			t.Fatalf("worktree branch = %q, %v", branch, err)
		}
		base, err := command(context.Background(), task.Path, "git", "rev-parse", "HEAD")
		if err != nil || base != r.Base {
			t.Fatalf("base = %q, %v", base, err)
		}
	}
	saved, err := m.Find(r.ID)
	if err != nil || !reflect.DeepEqual(saved, r) {
		t.Fatalf("saved = %#v, %v", saved, err)
	}
}

func TestRejectsInvalidInputBeforeSideEffects(t *testing.T) {
	for _, tc := range []struct {
		tool    model.Tool
		prompts []string
		quiet   time.Duration
	}{
		{"other", []string{"task"}, 0},
		{model.ToolClaude, nil, 0},
		{model.ToolCodex, []string{" "}, 0},
		{model.ToolClaude, []string{"task"}, -time.Second},
		{model.ToolClaude, []string{"task"}, time.Millisecond},
	} {
		m := New(filepath.Join(t.TempDir(), "absent"))
		m.Command = func(context.Context, string, string, ...string) (string, error) {
			t.Fatal("unexpected command")
			return "", nil
		}
		if _, err := m.Start(context.Background(), ".", tc.tool, tc.prompts, tc.quiet); err == nil {
			t.Fatal("expected validation error")
		}
		if _, err := os.Stat(m.Dir); !os.IsNotExist(err) {
			t.Fatal("created data directory")
		}
	}
}

func TestMissingDependencyBeforeWorktrees(t *testing.T) {
	m := New(t.TempDir())
	m.LookPath = func(bin string) (string, error) { return "", exec.ErrNotFound }
	m.Command = func(context.Context, string, string, ...string) (string, error) {
		t.Fatal("unexpected command")
		return "", nil
	}
	if _, err := m.Start(context.Background(), ".", model.ToolCodex, []string{"task"}, 0); err == nil {
		t.Fatal("expected missing dependency")
	}
}

func TestAttachTargetsExactRun(t *testing.T) {
	for _, inside := range []bool{false, true} {
		cmd := AttachCmd(Run{ID: "ais-123"}, inside)
		sub := "attach-session"
		if inside {
			sub = "switch-client"
		}
		if !reflect.DeepEqual(cmd.Args, []string{"tmux", sub, "-t", "=ais-123"}) {
			t.Fatal(cmd.Args)
		}
	}
}

// This integration test uses real Git and an isolated tmux server, but a fake
// agent. It checks argument safety and retained output without spending tokens.
func TestTmuxIntegration(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	repo := repository(t)
	dir := t.TempDir()
	agent := filepath.Join(dir, "fake agent")
	if err := os.WriteFile(agent, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > received.txt\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	socket := fmt.Sprintf("ais-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	tmux := func(ctx context.Context, cwd, bin string, args ...string) (string, error) {
		if bin == "tmux" {
			args = append([]string{"-L", socket, "-f", "/dev/null"}, args...)
		}
		out, err := command(ctx, cwd, bin, args...)
		if err != nil {
			t.Logf("%s %q: %v", bin, args, err)
		}
		return out, err
	}
	t.Cleanup(func() { _, _ = tmux(context.Background(), "", "tmux", "kill-server") })
	m := New(filepath.Join(dir, "runs"))
	m.Command = tmux
	m.Notifications = false // this test checks raw prompt forwarding
	m.LookPath = func(bin string) (string, error) {
		if bin == "claude" {
			return agent, nil
		}
		return exec.LookPath(bin)
	}
	prompts := []string{"--flag 'quoted' $(touch INJECTED); `touch INJECTED`\nsecond line", "another task"}
	r, err := m.Start(context.Background(), repo, model.ToolClaude, prompts, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for i, task := range r.Tasks {
		deadline := time.Now().Add(5 * time.Second)
		var data []byte
		for time.Now().Before(deadline) {
			data, err = os.ReadFile(filepath.Join(task.Path, "received.txt"))
			if err == nil && string(data) == "--\n"+prompts[i]+"\n" {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if string(data) != "--\n"+prompts[i]+"\n" {
			t.Fatalf("agent arguments = %q, %v", data, err)
		}
		if _, err := os.Stat(filepath.Join(task.Path, "INJECTED")); !os.IsNotExist(err) {
			t.Fatal("prompt executed as shell code")
		}
	}
	panes, err := tmux(context.Background(), "", "tmux", "list-panes", "-s", "-t", "="+r.ID, "-F", "#{pane_dead}")
	if err != nil || len(strings.Fields(panes)) != 2 {
		t.Fatalf("panes = %q, %v", panes, err)
	}
	if err := m.Stop(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	for _, task := range r.Tasks {
		if _, err := os.Stat(filepath.Join(task.Path, "received.txt")); err != nil {
			t.Fatal("stop lost work:", err)
		}
	}
}

func TestTmuxUsesCallerConfiguration(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	repo := repository(t)
	dir := t.TempDir()
	agent := filepath.Join(dir, "fake codex")
	script := "#!/bin/sh\nprintf '%s\\n' \"${CODEX_HOME-<unset>}\" \"${CODEX_SQLITE_HOME-<unset>}\" \"${CLAUDE_CONFIG_DIR-<unset>}\" > observed.tmp\nmv observed.tmp observed.txt\n"
	if err := os.WriteFile(agent, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	socket := fmt.Sprintf("ais-env-%d-%d", os.Getpid(), time.Now().UnixNano())
	tmux := func(ctx context.Context, cwd, bin string, args ...string) (string, error) {
		if bin == "tmux" {
			args = append([]string{"-L", socket, "-f", "/dev/null"}, args...)
		}
		return command(ctx, cwd, bin, args...)
	}
	keys := []string{"CODEX_HOME", "CODEX_SQLITE_HOME", "CLAUDE_CONFIG_DIR"}
	for _, key := range keys {
		t.Setenv(key, "/old-home")
	}
	if _, err := tmux(context.Background(), "", "tmux", "new-session", "-d", "-s", "seed", "sleep 60"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = tmux(context.Background(), "", "tmux", "kill-server") })
	m := New(filepath.Join(dir, "runs"))
	m.Command = tmux
	m.LookPath = func(bin string) (string, error) {
		if bin == "codex" {
			return agent, nil
		}
		return exec.LookPath(bin)
	}
	for _, unset := range []bool{false, true} {
		want := ""
		for _, key := range keys {
			value := filepath.Join(dir, "new ' home", key)
			if unset {
				if err := os.Unsetenv(key); err != nil {
					t.Fatal(err)
				}
				want += "<unset>\n"
			} else {
				t.Setenv(key, value)
				want += value + "\n"
			}
		}
		r, err := m.Start(context.Background(), repo, model.ToolCodex, []string{"check environment"}, 0)
		if err != nil {
			t.Fatal(err)
		}
		var data []byte
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			data, err = os.ReadFile(filepath.Join(r.Tasks[0].Path, "observed.txt"))
			if err == nil {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if string(data) != want {
			t.Fatalf("unset=%v: got %q, want %q, err=%v", unset, data, want, err)
		}
		if err := m.Stop(context.Background(), r); err != nil {
			t.Fatal(err)
		}
	}
}
