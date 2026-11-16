package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProjectCommands(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"scripts":{"test":"vitest","build":"vite build","bad name":"x","x;rm":"y"}}`)
	writeFile(t, filepath.Join(dir, "pnpm-lock.yaml"), "")
	writeFile(t, filepath.Join(dir, "Makefile"), ".PHONY: test\ntest: build\n\tgo test\nbuild:\n\tgo build\nVAR := value\n%.o: %.c\n")
	writeFile(t, filepath.Join(dir, "go.mod"), "module x\n")
	got := []string{}
	for _, c := range projectCommands(dir) {
		got = append(got, c.Source+": "+c.Command)
	}
	want := "package.json: pnpm run build, package.json: pnpm run test, Makefile: make test, Makefile: make build, go.mod: go test ./..."
	if strings.Join(got, ", ") != want {
		t.Fatalf("commands:\n got %s\nwant %s", strings.Join(got, ", "), want)
	}
	if len(projectCommands(t.TempDir())) != 0 {
		t.Fatal("an empty folder offers nothing")
	}
}

func waitRun(t *testing.T, m *chatManager, id string) ProjectRunState {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if state, ok := m.readProjectRun(id); ok && !state.Running {
			return state
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the run did not end")
	return ProjectRunState{}
}

func TestProjectRun(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	dir := t.TempDir()
	m := newChats(t.TempDir())
	t.Cleanup(m.close)
	state := createChat(t, m, dir, "codex", "")
	started, err := m.startProjectRun(state.ID, `printf 'hello\033[31m red\033[0m\n'; pwd; echo oops >&2; exit 3`)
	if err != nil || !started.Running {
		t.Fatalf("start: %+v %v", started, err)
	}
	done := waitRun(t, m, state.ID)
	real, _ := filepath.EvalSymlinks(dir)
	if done.ExitCode != 3 || !strings.Contains(done.Output, "hello red\n") || !strings.Contains(done.Output, real) || !strings.Contains(done.Output, "oops") || done.EndedAt == "" {
		t.Fatalf("run: %+v", done)
	}
	// One run at a time, and it can be stopped with what it started.
	if _, err := m.startProjectRun(state.ID, "sleep 30 & sleep 30"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.startProjectRun(state.ID, "echo again"); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("second run: %v", err)
	}
	began := time.Now()
	if err := m.stopProjectRun(state.ID); err != nil {
		t.Fatal(err)
	}
	stopped := waitRun(t, m, state.ID)
	if !stopped.Stopped || time.Since(began) > 5*time.Second {
		t.Fatalf("stop: %+v after %s", stopped, time.Since(began))
	}
	// Refusals.
	for _, command := range []string{"", "   ", strings.Repeat("x", commandLenLimit+1)} {
		if _, err := m.startProjectRun(state.ID, command); err == nil {
			t.Fatalf("%q must be refused", command)
		}
	}
	if _, err := m.startProjectRun("missing", "echo"); err == nil {
		t.Fatal("an unknown conversation must be refused")
	}
	// Output is bounded to the newest part.
	if _, err := m.startProjectRun(state.ID, "yes line | head -c 400000"); err != nil {
		t.Fatal(err)
	}
	big := waitRun(t, m, state.ID)
	if len(big.Output) > runOutputLimit || !big.Shortened {
		t.Fatalf("bounded: %d %v", len(big.Output), big.Shortened)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
}
