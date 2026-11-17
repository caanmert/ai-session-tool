package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeCodexHome makes a rollout file for thread under a temporary CODEX_HOME.
func fakeCodexHome(t *testing.T, thread string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	dir := filepath.Join(home, "sessions", "2026", "11", "17")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "rollout-2026-11-17T09-00-00-"+thread+".jsonl")
	if err := os.WriteFile(file, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return file
}

func fakeOpeners(t *testing.T, out string, processes map[int][3]string) {
	t.Helper()
	list, describe := listOpeners, describeProcess
	t.Cleanup(func() { listOpeners, describeProcess = list, describe })
	listOpeners = func(context.Context, string) ([]byte, error) { return []byte(out), nil }
	describeProcess = func(_ context.Context, pid int) (int, string, string, error) {
		p, ok := processes[pid]
		if !ok {
			return 0, "", "", errors.New("no such process")
		}
		parent := 1
		if p[0] == "self" {
			parent = os.Getpid()
		}
		return parent, p[1], p[2], nil
	}
}

func TestCodexHolderNamesTheWriter(t *testing.T) {
	const thread = "01a1e102-9d7f-7391-a7c8-93b58cbbd29d"
	fakeCodexHome(t, thread)
	ctx := context.Background()
	for _, tc := range []struct {
		name, lsof string
		process    [3]string
		want       string
	}{
		{"Codex app", "p48140\nccodex\nf45\nau\n", [3]string{"", "??", "/Applications/ChatGPT.app/Contents/Resources/codex-cli/CodexCLI.app/Contents/MacOS/codex app-server"}, "the Codex app"},
		{"terminal", "p82366\nccodex\nf12\naw\n", [3]string{"", "ttys004", "codex"}, "Codex in a terminal"},
		{"ais tab", "p900\nccodex\nf9\nau\n", [3]string{"self", "ttys009", "codex resume"}, "a terminal tab in ais"},
		{"other", "p77\nccodex\nf3\nau\n", [3]string{"", "??", "codex exec"}, "another Codex program"},
		{"reader only", "p55\ncais\nf7\nar\n", [3]string{"", "??", "reader"}, ""},
		{"nobody", "", [3]string{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pid := 0
			for _, line := range strings.Split(tc.lsof, "\n") {
				if strings.HasPrefix(line, "p") {
					pid, _ = strconv.Atoi(line[1:])
				}
			}
			fakeOpeners(t, tc.lsof, map[int][3]string{pid: tc.process})
			if got := codexHolder(ctx, thread); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	// This process is never the holder, and a failed lsof means nobody.
	fakeOpeners(t, "p"+strconv.Itoa(os.Getpid())+"\ncais\nf3\nau\n", nil)
	if got := codexHolder(ctx, thread); got != "" {
		t.Fatalf("this process counted: %q", got)
	}
	listOpeners = func(context.Context, string) ([]byte, error) { return nil, errors.New("no lsof") }
	if got := codexHolder(ctx, thread); got != "" {
		t.Fatalf("a failed check named %q", got)
	}
	// IDs that could widen the file pattern are not looked up.
	if codexRollout("*") != "" || codexRollout("../x") != "" {
		t.Fatal("an unsafe ID was looked up")
	}
}

// A message to a conversation another program holds stops before Codex
// starts, and says which program it is.
func TestCodexSendStopsWhenHeldElsewhere(t *testing.T) {
	fakeCodexHome(t, "native-thread")
	fakeOpeners(t, "p48140\nccodex\nf45\nau\n", map[int][3]string{48140: {"", "??", "/Applications/Codex.app/Contents/MacOS/codex app-server"}})
	m := fakeChats(t, "resume")
	started := false
	command := m.command
	m.command = func(tool string, args ...string) *exec.Cmd {
		started = true
		return command(tool, args...)
	}
	chat := createChat(t, m, t.TempDir(), "codex", "native-thread")
	if holder, _ := m.heldElsewhere(context.Background(), chat.ID); holder != "the Codex app" {
		t.Fatalf("holder %q", holder)
	}
	if err := m.send(chat.ID, "hello"); err != nil {
		t.Fatal(err)
	}
	state := waitChat(t, m, chat.ID, func(s ChatState) bool { return s.Status == "error" })
	if !strings.HasPrefix(state.Error, "This Codex conversation is open in the Codex app") || started {
		t.Fatalf("error %q, Codex started: %v", state.Error, started)
	}
}

// With the real lsof, a process that has the file open for writing is found.
func TestCodexHolderWithLsof(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("lsof's location and output are checked on macOS")
	}
	file := fakeCodexHome(t, "real-thread-0001")
	cmd := exec.Command("/bin/sh", "-c", `exec 3>>"$0"; exec sleep 30`, file)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if found := writers(context.Background(), file); len(found) == 1 && found[0] == cmd.Process.Pid {
			if got := codexHolder(context.Background(), "real-thread-0001"); got != "a terminal tab in ais" {
				t.Fatalf("a child of this process is %q", got)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("lsof did not find the writer")
}
