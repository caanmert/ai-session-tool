package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/caanmert/ai-session-tool/internal/launch"
	"github.com/caanmert/ai-session-tool/internal/model"
)

type notificationSettings struct {
	Hooks map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Type    string `json:"type"`
			Command string `json:"command"`
			Timeout int    `json:"timeout"`
		} `json:"hooks"`
	} `json:"hooks"`
}

func TestNotificationEvents(t *testing.T) {
	var settings notificationSettings
	if err := json.Unmarshal([]byte(claudeNotificationSettings), &settings); err != nil {
		t.Fatal(err)
	}
	groups := settings.Hooks["Notification"]
	if len(settings.Hooks) != 1 || len(groups) != 1 || len(groups[0].Hooks) != 1 {
		t.Fatalf("unexpected hooks: %+v", settings)
	}
	matcher, err := regexp.Compile(groups[0].Matcher)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"permission_prompt", "idle_prompt", "elicitation_dialog", "elicitation_url_dialog"} {
		if !matcher.MatchString(event) {
			t.Errorf("missing %s", event)
		}
	}
	for _, event := range []string{"auth_success", "quota_auto_resume", "something_idle_prompt"} {
		if matcher.MatchString(event) {
			t.Errorf("unexpected alert for %s", event)
		}
	}
	for _, tool := range []model.Tool{model.ToolClaude, model.ToolCodex} {
		if args := notificationArgs(tool, false); len(args) != 0 {
			t.Fatal("opt-out injected arguments", args)
		}
	}
	// Hook subprocesses without a tmux identity must not notify another pane,
	// print anything into the conversation, or fail the agent's hook.
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	out, err := exec.Command("sh", "-c", groups[0].Hooks[0].Command).CombinedOutput()
	if err != nil || len(out) != 0 {
		t.Fatalf("outside tmux: %q, %v", out, err)
	}
}

// TestNotificationAgentHelper emulates each tool's documented notification
// contract. It is invoked only inside an isolated tmux pane, never as an LLM.
func TestNotificationAgentHelper(t *testing.T) {
	if os.Getenv("AIS_RUNNER_NOTIFICATION_HELPER") != "1" {
		return
	}
	var args []string
	for i, arg := range os.Args {
		if arg == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	if len(args) < 3 {
		os.Exit(2)
	}
	tool, agentArgs := args[0], args[1:]
	data, _ := json.Marshal(agentArgs)
	if err := os.WriteFile("ready.json", data, 0o600); err != nil {
		os.Exit(3)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat("notify-now"); err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		errText := ""
		switch tool {
		case "claude":
			if agentArgs[0] != "--settings" {
				break
			}
			var settings notificationSettings
			if err := json.Unmarshal([]byte(agentArgs[1]), &settings); err != nil {
				os.Exit(4)
			}
			hook := settings.Hooks["Notification"][0].Hooks[0]
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			c := exec.CommandContext(ctx, "sh", "-c", hook.Command)
			c.Stdin = strings.NewReader(`{"hook_event_name":"Notification","notification_type":"permission_prompt"}`)
			// Simulate hooks that capture stdout and have no controlling tty.
			c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			out, err := c.CombinedOutput()
			cancel()
			if err != nil || len(out) != 0 {
				errText = fmt.Sprintf("hook output=%q, error=%v", out, err)
			}
		case "codex":
			if agentArgs[0] != "-c" {
				break
			}
			configs := map[string]bool{}
			for i := 0; i+1 < len(agentArgs) && agentArgs[i] == "-c"; i += 2 {
				configs[agentArgs[i+1]] = true
			}
			if configs[`tui.notifications=["agent-turn-complete","approval-requested"]`] &&
				configs[`tui.notification_method="bel"`] && configs[`tui.notification_condition="always"`] {
				fmt.Print("\a")
			} else {
				errText = "missing Codex event configuration"
			}
		}
		if err := os.WriteFile("notified", []byte(errText), 0o600); err != nil {
			os.Exit(5)
		}
		// Avoid test harness output generating unrelated terminal activity.
		os.Exit(0)
	}
	os.Exit(6)
}

func waitFile(t *testing.T, path string) []byte {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil {
			return data
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
	return nil
}

func TestTmuxEventNotifications(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	for _, tool := range []model.Tool{model.ToolCodex, model.ToolClaude} {
		for _, enabled := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/notify=%v", tool, enabled), func(t *testing.T) {
				repo, dir := repository(t), t.TempDir()
				bin, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				agent := filepath.Join(dir, "fake agent")
				script := "#!/bin/sh\nAIS_RUNNER_NOTIFICATION_HELPER=1 exec " + launch.Quote(bin) + " -test.run=^TestNotificationAgentHelper$ -- " + string(tool) + " \"$@\"\n"
				if err := os.WriteFile(agent, []byte(script), 0o700); err != nil {
					t.Fatal(err)
				}
				socket := fmt.Sprintf("ais-notify-%d-%d", os.Getpid(), time.Now().UnixNano())
				tmux := func(ctx context.Context, cwd, bin string, args ...string) (string, error) {
					if bin == "tmux" {
						args = append([]string{"-L", socket, "-f", "/dev/null"}, args...)
					}
					return command(ctx, cwd, bin, args...)
				}
				t.Cleanup(func() { _, _ = tmux(context.Background(), "", "tmux", "kill-server") })
				m := New(filepath.Join(dir, "runs"))
				m.Command, m.Notifications = tmux, enabled
				m.LookPath = func(name string) (string, error) {
					if name == string(tool) {
						return agent, nil
					}
					return exec.LookPath(name)
				}
				prompt := "event 'quoted' $(touch INJECTED)"
				r, err := m.Start(context.Background(), repo, tool, []string{"silent task", prompt}, 0)
				if err != nil {
					t.Fatal(err)
				}
				for _, task := range r.Tasks {
					waitFile(t, filepath.Join(task.Path, "ready.json"))
				}
				var gotArgs []string
				if err := json.Unmarshal(waitFile(t, filepath.Join(r.Tasks[1].Path, "ready.json")), &gotArgs); err != nil {
					t.Fatal(err)
				}
				if gotArgs[len(gotArgs)-2] != "--" || gotArgs[len(gotArgs)-1] != prompt {
					t.Fatal(gotArgs)
				}
				if _, err := os.Stat(filepath.Join(r.Tasks[1].Path, "INJECTED")); !os.IsNotExist(err) {
					t.Fatal("prompt executed as shell code")
				}
				target := "=" + r.ID + ":task-2"
				query := func(format string) string {
					out, err := tmux(context.Background(), "", "tmux", "display-message", "-p", "-t", target, format)
					if err != nil {
						t.Fatal(err)
					}
					return out
				}
				if got := query("#{monitor-silence}:#{window_bell_flag}"); got != "0:0" {
					t.Fatalf("silence produced alert: %s", got)
				}
				if err := os.WriteFile(filepath.Join(r.Tasks[1].Path, "notify-now"), nil, 0o600); err != nil {
					t.Fatal(err)
				}
				if data := waitFile(t, filepath.Join(r.Tasks[1].Path, "notified")); len(data) != 0 {
					t.Fatal(string(data))
				}
				want := "0"
				if enabled {
					want = "1"
				}
				deadline := time.Now().Add(2 * time.Second)
				got := query("#{window_bell_flag}")
				for got != want && time.Now().Before(deadline) {
					time.Sleep(20 * time.Millisecond)
					got = query("#{window_bell_flag}")
				}
				if got != want {
					t.Fatalf("event alert = %s, want %s", got, want)
				}
				if enabled {
					if _, err := tmux(context.Background(), "", "tmux", "select-window", "-t", target); err != nil {
						t.Fatal(err)
					}
					if got := query("#{window_bell_flag}"); got != "0" {
						t.Fatalf("viewed window retained alert: %s", got)
					}
				}
			})
		}
	}
}
