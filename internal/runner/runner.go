// Package runner starts independent coding tasks in Git worktrees and tmux.
package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/caanmert/ai-session-tool/internal/launch"
	"github.com/caanmert/ai-session-tool/internal/model"
)

// Task keeps the location of a task's work even after its agent exits.
type Task struct {
	Prompt string `json:"prompt"`
	Branch string `json:"branch"`
	Path   string `json:"path"`
	Window string `json:"window"`
}

// Run is persisted before any agents are started.
type Run struct {
	ID            string     `json:"id"`
	Tool          model.Tool `json:"tool"`
	Repository    string     `json:"repository"`
	Base          string     `json:"base"`
	CreatedAt     time.Time  `json:"createdAt"`
	Notifications bool       `json:"notifications,omitempty"`
	Tasks         []Task     `json:"tasks"`
}

// Manager owns run manifests and worktrees under Dir. Command is injectable
// so tests never need to launch a real coding agent.
type Manager struct {
	Dir      string
	Command  func(context.Context, string, string, ...string) (string, error)
	LookPath func(string) (string, error)
	// Notifications configures agent event alerts for this run only.
	Notifications bool
}

func New(dir string) *Manager {
	return &Manager{Dir: dir, Command: command, LookPath: exec.LookPath, Notifications: true}
}

func command(ctx context.Context, cwd, bin string, args ...string) (string, error) {
	c := exec.CommandContext(ctx, bin, args...)
	c.Dir = cwd
	out, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", bin, args[0], err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// Start creates one branch and worktree per prompt at the same committed
// revision. Existing uncommitted changes are not copied. All worktrees are
// prepared before the first agent starts; failures preserve paths in a manifest.
func (m *Manager) Start(ctx context.Context, cwd string, tool model.Tool, prompts []string, quiet time.Duration) (Run, error) {
	if tool != model.ToolClaude && tool != model.ToolCodex {
		return Run{}, fmt.Errorf("unknown tool %q: use claude or codex", tool)
	}
	if len(prompts) == 0 || len(prompts) > 32 {
		return Run{}, errors.New("provide between 1 and 32 task prompts")
	}
	for _, p := range prompts {
		if strings.TrimSpace(p) == "" {
			return Run{}, errors.New("task prompts must not be empty")
		}
	}
	if quiet < 0 || quiet > 24*time.Hour || quiet%time.Second != 0 {
		return Run{}, errors.New("quiet interval must be whole seconds between 0s and 24h")
	}
	var agent string
	for _, bin := range []string{"git", "tmux", string(tool)} {
		path, err := m.LookPath(bin)
		if err != nil {
			return Run{}, fmt.Errorf("%s is required on PATH: %w", bin, err)
		}
		if bin == string(tool) {
			agent = path
		}
	}
	repo, err := m.Command(ctx, cwd, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return Run{}, err
	}
	base, err := m.Command(ctx, repo, "git", "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return Run{}, err
	}
	var nonce [6]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return Run{}, err
	}
	r := Run{ID: "ais-" + hex.EncodeToString(nonce[:]), Tool: tool, Repository: repo, Base: base, CreatedAt: time.Now().UTC(), Notifications: m.Notifications}
	dir, err := filepath.Abs(filepath.Join(m.Dir, r.ID))
	if err != nil {
		return Run{}, err
	}
	if err := os.MkdirAll(m.Dir, 0o700); err != nil {
		return Run{}, err
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		return Run{}, err
	}
	for i, prompt := range prompts {
		name := fmt.Sprintf("task-%d", i+1)
		r.Tasks = append(r.Tasks, Task{Prompt: prompt, Branch: r.ID + "/" + name, Path: filepath.Join(dir, name), Window: name})
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return Run{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "run.json"), data, 0o600); err != nil {
		_ = os.Remove(filepath.Join(dir, "run.json"))
		_ = os.Remove(dir)
		return Run{}, err
	}
	fail := func(err error) (Run, error) {
		return r, fmt.Errorf("run %s setup incomplete; work is preserved in %s; agents already started may still be running (use ais run attach/stop %s): %w", r.ID, dir, r.ID, err)
	}
	for _, task := range r.Tasks {
		if _, err := m.Command(ctx, repo, "git", "worktree", "add", "-b", task.Branch, task.Path, base); err != nil {
			return fail(err)
		}
	}
	// A holding process prevents fast-exiting agents from closing their window
	// before remain-on-exit is configured. Only our own panes are respawned.
	for i, task := range r.Tasks {
		args := []string{"new-window", "-d", "-t", "=" + r.ID + ":", "-n", task.Window, "-c", task.Path, "-P", "-F", "#{pane_id}"}
		if i == 0 {
			args = []string{"new-session", "-d", "-s", r.ID, "-n", task.Window, "-c", task.Path, "-P", "-F", "#{pane_id}"}
		}
		args = append(args, "sleep", "2147483647")
		pane, err := m.Command(ctx, "", "tmux", args...)
		if err != nil {
			return fail(err)
		}
		for _, option := range [][]string{
			{"set-option", "-p", "-t", pane, "@ais_task", task.Window},
			{"set-option", "-w", "-t", pane, "remain-on-exit", "on"},
			{"set-option", "-w", "-t", pane, "automatic-rename", "off"},
			{"set-option", "-w", "-t", pane, "monitor-bell", onOff(m.Notifications)},
			{"set-option", "-w", "-t", pane, "monitor-silence", strconv.FormatInt(int64(quiet/time.Second), 10)},
		} {
			if _, err := m.Command(ctx, "", "tmux", option...); err != nil {
				return fail(err)
			}
		}
		if i == 0 {
			for _, option := range [][]string{
				{"set-option", "-t", r.ID, "visual-silence", "on"},
				{"set-option", "-t", r.ID, "silence-action", "other"},
				{"set-option", "-t", r.ID, "visual-bell", "on"},
				{"set-option", "-t", r.ID, "bell-action", "other"},
			} {
				if _, err := m.Command(ctx, "", "tmux", option...); err != nil {
					return fail(err)
				}
			}
		}
		// Quote every argument for the shell. In particular, prompts are never
		// interpreted as commands or agent flags.
		line := agentShellLine(agent, task.Prompt, notificationArgs(tool, m.Notifications))
		if _, err := m.Command(ctx, "", "tmux", "respawn-pane", "-k", "-t", pane, "-c", task.Path, "sh", "-c", line); err != nil {
			return fail(err)
		}
	}
	return r, nil
}

// tmux inherits its server's environment, which may belong to an older
// invocation. Set (or explicitly unset) the caller's tool configuration paths
// in each agent process, without changing other tmux sessions.
func agentShellLine(agent, prompt string, args []string) string {
	var line strings.Builder
	for _, name := range []string{"PATH", "CODEX_HOME", "CODEX_SQLITE_HOME", "CLAUDE_CONFIG_DIR"} {
		if value, ok := os.LookupEnv(name); ok {
			line.WriteString("export " + name + "=" + launch.Quote(value) + "; ")
		} else {
			line.WriteString("unset " + name + "; ")
		}
	}
	line.WriteString("exec " + launch.Quote(agent))
	for _, arg := range args {
		line.WriteString(" " + launch.Quote(arg))
	}
	line.WriteString(" -- " + launch.Quote(prompt))
	return line.String()
}

// List reads only manifests; it works without tmux or the agent installed.
func (m *Manager) List() ([]Run, error) {
	entries, err := os.ReadDir(m.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Run{}, nil
	}
	if err != nil {
		return nil, err
	}
	runs := []Run{}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "ais-") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.Dir, entry.Name(), "run.json"))
		if err != nil {
			return nil, err
		}
		var r Run
		if err := json.Unmarshal(data, &r); err != nil {
			return nil, fmt.Errorf("run %s: %w", entry.Name(), err)
		}
		if r.ID != entry.Name() {
			return nil, fmt.Errorf("run %s: manifest id mismatch", entry.Name())
		}
		runs = append(runs, r)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt.After(runs[j].CreatedAt) })
	return runs, nil
}

// Find accepts a full id or a unique prefix.
func (m *Manager) Find(prefix string) (Run, error) {
	if prefix == "" {
		return Run{}, errors.New("empty run id")
	}
	runs, err := m.List()
	if err != nil {
		return Run{}, err
	}
	var matches []Run
	for _, r := range runs {
		if r.ID == prefix {
			return r, nil
		}
		if strings.HasPrefix(r.ID, prefix) {
			matches = append(matches, r)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return Run{}, fmt.Errorf("%q matches %d runs; use a unique run id", prefix, len(matches))
}

// Stop closes only this run's tmux session, retaining all worktrees and branches.
func (m *Manager) Stop(ctx context.Context, r Run) error {
	_, err := m.Command(ctx, "", "tmux", "kill-session", "-t", "="+r.ID)
	return err
}

// AttachCmd switches clients already inside tmux, or attaches from outside.
func AttachCmd(r Run, inside bool) *exec.Cmd {
	sub := "attach-session"
	if inside {
		sub = "switch-client"
	}
	return exec.Command("tmux", sub, "-t", "="+r.ID)
}
