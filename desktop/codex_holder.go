package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/caanmert/ai-session-tool/internal/provider/codex"
)

// Codex lets one program write to a conversation at a time; the writer keeps
// the conversation's rollout file open for writing. Before a message is sent,
// and while a conversation is shown, ais checks read-only, with macOS's lsof,
// whether another program holds it. Any failure means "nobody": a check that
// cannot run never blocks sending.

// safeThread is a Codex conversation ID that can go into a file pattern.
var safeThread = regexp.MustCompile(`^[0-9A-Za-z-]{8,64}$`)

// codexRollout finds a Codex conversation's rollout file.
func codexRollout(thread string) string {
	if !safeThread.MatchString(thread) {
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(codex.DefaultRoot(), "sessions", "*", "*", "*", "rollout-*-"+thread+".jsonl"))
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1]
}

// listOpeners runs lsof for one file: field output with each process's ID,
// command and access mode. Replaced in tests.
var listOpeners = func(ctx context.Context, file string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "/usr/sbin/lsof", "-w", "-Fpca", "--", file).Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 && len(out) == 0 {
		return nil, nil // Nothing has the file open.
	}
	return out, err
}

// describeProcess returns a process's parent, terminal and command line.
// Replaced in tests.
var describeProcess = func(ctx context.Context, pid int) (parent int, tty, command string, err error) {
	out, err := exec.CommandContext(ctx, "/bin/ps", "-o", "ppid=,tty=,command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, "", "", err
	}
	fields := strings.Fields(string(out))
	if len(fields) < 3 {
		return 0, "", "", errors.New("unexpected ps output")
	}
	parent, err = strconv.Atoi(fields[0])
	return parent, fields[1], strings.Join(fields[2:], " "), err
}

// writers lists the processes, other than this one, that have file open for
// writing.
func writers(ctx context.Context, file string) []int {
	out, err := listOpeners(ctx, file)
	if err != nil {
		return nil
	}
	self := os.Getpid()
	found := []int{}
	pid := 0
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			pid, _ = strconv.Atoi(line[1:])
		case 'a':
			mode := strings.TrimSpace(line[1:])
			if pid > 0 && pid != self && (mode == "w" || mode == "u") && (len(found) == 0 || found[len(found)-1] != pid) {
				found = append(found, pid)
			}
		}
	}
	return found
}

// holderName says in words which program a process is.
func holderName(ctx context.Context, pid int) string {
	parent, tty, command, err := describeProcess(ctx, pid)
	switch {
	case err != nil:
		return "another Codex program"
	case parent == os.Getpid():
		return "a terminal tab in ais"
	case strings.Contains(command, ".app/") || strings.Contains(command, "app-server-daemon"):
		return "the Codex app"
	case tty != "" && tty != "??" && tty != "?":
		return "Codex in a terminal"
	}
	return "another Codex program"
}

// codexHolder names the program that has a Codex conversation open for
// writing, or returns "" when none does or it can't be told.
func codexHolder(ctx context.Context, thread string) string {
	file := codexRollout(thread)
	if file == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	found := writers(ctx, file)
	if len(found) == 0 {
		return ""
	}
	return holderName(ctx, found[0])
}

// openElsewhereIn is errOpenElsewhere naming the program that holds the
// conversation.
func openElsewhereIn(holder string) error {
	return fmt.Errorf("This Codex conversation is open in %s, and Codex lets only one program write to a conversation at a time. Close it there, then send your message again", holder)
}

// heldElsewhere checks an idle Codex conversation. A running turn holds the
// conversation itself, so it is not checked.
func (m *chatManager) heldElsewhere(ctx context.Context, id string) (string, error) {
	c, err := m.get(id)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	tool, thread, busy := c.state.Tool, c.state.SessionID, busyChat(c.state.Status)
	c.mu.Unlock()
	if tool != "codex" || thread == "" || busy {
		return "", nil
	}
	return codexHolder(ctx, thread), nil
}

// CodexHolder names the other program that has a Codex conversation open, so
// sending from here would fail; "" when none does. Read-only.
func (a *App) CodexHolder(chatID string) (string, error) {
	if err := a.chatReady(); err != nil {
		return "", err
	}
	return a.chats.heldElsewhere(a.ctx, chatID)
}
