package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// A conversation can run one of the project's commands beside it: a package
// script, a Makefile target, the language's tests, or a command the user
// types. Only what the user chooses runs, one at a time per conversation, in
// the project folder, and it can be stopped.

// ProjectCommand is a command the project offers.
type ProjectCommand struct {
	Label   string `json:"label"`
	Command string `json:"command"`
	// Source says where it was found, such as package.json or Makefile.
	Source string `json:"source"`
}

// ProjectRunState is the latest run in a conversation.
type ProjectRunState struct {
	Command   string `json:"command"`
	Running   bool   `json:"running"`
	ExitCode  int    `json:"exitCode"`
	Output    string `json:"output"`
	Shortened bool   `json:"shortened,omitempty"`
	StartedAt string `json:"startedAt"`
	EndedAt   string `json:"endedAt,omitempty"`
	// Stopped is set when the user stopped it.
	Stopped bool `json:"stopped,omitempty"`
}

const (
	runOutputLimit  = 256 << 10
	commandLimit    = 40
	commandLenLimit = 2000
)

type projectRun struct {
	mu    sync.Mutex
	state ProjectRunState
	cmd   *exec.Cmd
	done  chan struct{}
}

var makeTarget = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9_.-]*)\s*:([^=]|$)`)
var ansiCodes = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]|\x1b\\][^\x07]*\x07")

// projectCommands lists the commands found in a project folder.
func projectCommands(dir string) []ProjectCommand {
	commands := []ProjectCommand{}
	if data, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		if json.Unmarshal(data, &pkg) == nil {
			runner := "npm run"
			switch {
			case fileExists(filepath.Join(dir, "pnpm-lock.yaml")):
				runner = "pnpm run"
			case fileExists(filepath.Join(dir, "yarn.lock")):
				runner = "yarn run"
			case fileExists(filepath.Join(dir, "bun.lockb")), fileExists(filepath.Join(dir, "bun.lock")):
				runner = "bun run"
			}
			names := make([]string, 0, len(pkg.Scripts))
			for name := range pkg.Scripts {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				if strings.ContainsAny(name, " \t\n'\"`$;&|<>") {
					continue
				}
				commands = append(commands, ProjectCommand{Label: name, Command: runner + " " + name, Source: "package.json"})
			}
		}
	}
	if file, err := os.Open(filepath.Join(dir, "Makefile")); err == nil {
		scanner := bufio.NewScanner(file)
		seen := map[string]bool{}
		for scanner.Scan() {
			if match := makeTarget.FindStringSubmatch(scanner.Text()); match != nil && !strings.HasPrefix(match[1], ".") && !seen[match[1]] {
				seen[match[1]] = true
				commands = append(commands, ProjectCommand{Label: match[1], Command: "make " + match[1], Source: "Makefile"})
			}
		}
		file.Close()
	}
	if fileExists(filepath.Join(dir, "go.mod")) {
		commands = append(commands, ProjectCommand{Label: "go test", Command: "go test ./...", Source: "go.mod"})
	}
	if fileExists(filepath.Join(dir, "Cargo.toml")) {
		commands = append(commands, ProjectCommand{Label: "cargo test", Command: "cargo test", Source: "Cargo.toml"})
	}
	if len(commands) > commandLimit {
		commands = commands[:commandLimit]
	}
	return commands
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// runWriter keeps the newest output of a run.
type runWriter struct{ run *projectRun }

func (w runWriter) Write(p []byte) (int, error) {
	w.run.mu.Lock()
	defer w.run.mu.Unlock()
	out := w.run.state.Output + ansiCodes.ReplaceAllString(string(p), "")
	if len(out) > runOutputLimit {
		out = out[len(out)-runOutputLimit:]
		w.run.state.Shortened = true
	}
	w.run.state.Output = out
	return len(p), nil
}

func (m *chatManager) chatProject(chatID string) (string, error) {
	c, err := m.get(chatID)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	project, review := c.state.Project, c.state.Review
	c.mu.Unlock()
	if review != nil {
		return "", errors.New("a review does not run commands")
	}
	return projectDirectory(project)
}

func (m *chatManager) startProjectRun(chatID, command string) (ProjectRunState, error) {
	command = strings.TrimSpace(command)
	if command == "" || len(command) > commandLenLimit || strings.ContainsRune(command, 0) {
		return ProjectRunState{}, errors.New("enter a command of up to 2,000 characters")
	}
	dir, err := m.chatProject(chatID)
	if err != nil {
		return ProjectRunState{}, err
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ProjectRunState{}, errors.New("the app is closing")
	}
	if m.projectRuns == nil {
		m.projectRuns = map[string]*projectRun{}
	}
	if previous := m.projectRuns[chatID]; previous != nil {
		previous.mu.Lock()
		running := previous.state.Running
		previous.mu.Unlock()
		if running {
			m.mu.Unlock()
			return ProjectRunState{}, errors.New("a command is already running here; stop it first")
		}
	}
	run := &projectRun{done: make(chan struct{}), state: ProjectRunState{Command: command, Running: true, StartedAt: time.Now().UTC().Format(time.RFC3339)}}
	m.projectRuns[chatID] = run
	m.mu.Unlock()
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	// A login shell, so the project's tools are on the PATH as in a terminal.
	cmd := exec.Command(shell, "-l", "-c", command)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CI=", "FORCE_COLOR=0", "NO_COLOR=1", "TERM=dumb")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout, cmd.Stderr = runWriter{run}, runWriter{run}
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		run.mu.Lock()
		run.state.Running, run.state.ExitCode = false, -1
		run.state.Output = "Could not start: " + err.Error()
		run.state.EndedAt = time.Now().UTC().Format(time.RFC3339)
		run.mu.Unlock()
		close(run.done)
		return run.snapshot(), nil
	}
	run.cmd = cmd
	go func() {
		err := cmd.Wait()
		run.mu.Lock()
		run.state.Running = false
		run.state.ExitCode = cmd.ProcessState.ExitCode()
		if err != nil && run.state.ExitCode == 0 {
			run.state.ExitCode = -1
		}
		run.state.EndedAt = time.Now().UTC().Format(time.RFC3339)
		run.mu.Unlock()
		close(run.done)
	}()
	return run.snapshot(), nil
}

func (r *projectRun) snapshot() ProjectRunState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

func (m *chatManager) readProjectRun(chatID string) (ProjectRunState, bool) {
	m.mu.Lock()
	run := m.projectRuns[chatID]
	m.mu.Unlock()
	if run == nil {
		return ProjectRunState{}, false
	}
	return run.snapshot(), true
}

// stopProjectRun ends the run and everything it started: TERM first, then
// KILL if it is still there after two seconds.
func (m *chatManager) stopProjectRun(chatID string) error {
	m.mu.Lock()
	run := m.projectRuns[chatID]
	m.mu.Unlock()
	if run == nil || run.cmd == nil {
		return errors.New("nothing is running here")
	}
	run.mu.Lock()
	if !run.state.Running {
		run.mu.Unlock()
		return nil
	}
	run.state.Stopped = true
	pid := run.cmd.Process.Pid
	run.mu.Unlock()
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	select {
	case <-run.done:
	case <-time.After(2 * time.Second):
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		<-run.done
	}
	return nil
}

// stopAllProjectRuns is used when the app closes.
func (m *chatManager) stopAllProjectRuns() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.projectRuns))
	for id := range m.projectRuns {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		_ = m.stopProjectRun(id)
	}
}

// ProjectCommands lists the commands the conversation's project offers.
func (a *App) ProjectCommands(chatID string) ([]ProjectCommand, error) {
	if err := a.chatReady(); err != nil {
		return nil, err
	}
	dir, err := a.chats.chatProject(chatID)
	if err != nil {
		return nil, err
	}
	return projectCommands(dir), nil
}

// StartProjectCommand runs a command the user chose in the project folder.
func (a *App) StartProjectCommand(chatID, command string) (ProjectRunState, error) {
	if err := a.chatReady(); err != nil {
		return ProjectRunState{}, err
	}
	return a.chats.startProjectRun(chatID, command)
}

// ProjectRun reads the conversation's latest run.
func (a *App) ProjectRun(chatID string) (ProjectRunState, error) {
	if err := a.chatReady(); err != nil {
		return ProjectRunState{}, err
	}
	state, ok := a.chats.readProjectRun(chatID)
	if !ok {
		return ProjectRunState{}, fmt.Errorf("nothing has run here yet")
	}
	return state, nil
}

// StopProjectCommand stops the conversation's running command.
func (a *App) StopProjectCommand(chatID string) error {
	if err := a.chatReady(); err != nil {
		return err
	}
	return a.chats.stopProjectRun(chatID)
}
