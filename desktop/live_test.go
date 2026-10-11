package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func terminalFixture(t *testing.T, script string, args ...string) (*terminals, LiveSession) {
	t.Helper()
	m := newTerminals()
	t.Cleanup(m.close)
	cmd := exec.Command("/bin/sh", append([]string{"-c", script, "fixture"}, args...)...)
	cmd.Dir = t.TempDir()
	session, err := m.start(cmd, "codex", "codex/test-session")
	if err != nil {
		t.Fatal(err)
	}
	return m, session
}

func readUntil(t *testing.T, m *terminals, id string, cursor *int64, match func(TerminalChunk, string) bool) (TerminalChunk, string) {
	t.Helper()
	var output strings.Builder
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		chunk, err := m.poll(id, *cursor)
		if err != nil {
			t.Fatal(err)
		}
		*cursor = chunk.Cursor
		output.Write(chunk.Data)
		if match(chunk, output.String()) {
			return chunk, output.String()
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out reading terminal: %q", output.String())
	return TerminalChunk{}, ""
}

func TestLiveTerminalInteractiveResizeAndExit(t *testing.T) {
	m, s := terminalFixture(t, `test -t 0 && test -t 1 || exit 99; stty -echo; printf 'READY\n'; IFS= read -r text; printf 'INPUT:%s\n' "$text"; stty size; printf 'UTF8:λ🙂\n'; exit 7`)
	var cursor int64
	readUntil(t, m, s.ID, &cursor, func(_ TerminalChunk, out string) bool { return strings.Contains(out, "READY") })
	if err := m.resize(s.ID, 123, 42); err != nil {
		t.Fatal(err)
	}
	if err := m.input(s.ID, "hello ' $(literal)\r"); err != nil {
		t.Fatal(err)
	}
	chunk, out := readUntil(t, m, s.ID, &cursor, func(c TerminalChunk, _ string) bool { return c.Exited })
	for _, want := range []string{"INPUT:hello ' $(literal)", "42 123", "UTF8:λ🙂"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
	if chunk.ExitCode != 7 {
		t.Fatalf("exit code: %d", chunk.ExitCode)
	}
	if err := m.input(s.ID, "x"); err == nil {
		t.Fatal("accepted input after exit")
	}
	if err := m.remove(s.ID); err != nil {
		t.Fatal(err)
	}
	if len(m.list()) != 0 {
		t.Fatal("tab retained after close")
	}
}

func TestTerminalStopAndDuplicateResume(t *testing.T) {
	m, s := terminalFixture(t, `printf 'READY\n'; sleep 60`)
	var cursor int64
	readUntil(t, m, s.ID, &cursor, func(_ TerminalChunk, out string) bool { return strings.Contains(out, "READY") })
	duplicate, err := m.start(exec.Command("/no/such/agent"), "codex", s.Key)
	if err != nil || duplicate.ID != s.ID {
		t.Fatalf("duplicate resume: %+v %v", duplicate, err)
	}
	if err := m.remove(s.ID); err == nil {
		t.Fatal("removed running session")
	}
	terminal, _ := m.get(s.ID)
	pgid, err := unix.Getpgid(terminal.cmd.Process.Pid)
	if err != nil || pgid == unix.Getpgrp() {
		t.Fatalf("agent is not isolated: pgid=%d err=%v", pgid, err)
	}
	if err := m.stop(s.ID); err != nil {
		t.Fatal(err)
	}
	chunk, _ := readUntil(t, m, s.ID, &cursor, func(c TerminalChunk, _ string) bool { return c.Exited })
	if chunk.ExitCode == 0 {
		t.Fatal("stopped agent reported successful exit")
	}
}

func TestTerminalShutdownAndInputValidation(t *testing.T) {
	m, s := terminalFixture(t, `printf 'READY\n'; sleep 60`)
	if err := m.resize(s.ID, 0, 30); err == nil {
		t.Fatal("accepted invalid size")
	}
	if err := m.input(s.ID, strings.Repeat("x", 65537)); err == nil {
		t.Fatal("accepted oversized input")
	}
	if _, err := m.poll(s.ID, -1); err == nil {
		t.Fatal("accepted invalid cursor")
	}
	if err := m.input("unknown", "x"); err == nil {
		t.Fatal("accepted unknown ID")
	}
	m.close()
	if _, err := m.start(exec.Command("/bin/echo"), "codex", ""); err == nil {
		t.Fatal("started after shutdown")
	}
	terminal, _ := m.get(s.ID)
	select {
	case <-terminal.done:
	default:
		t.Fatal("reader survived shutdown")
	}
}

func TestTerminalBoundedReplayAndFinalDrain(t *testing.T) {
	m := newTerminals()
	terminal := &liveTerminal{info: LiveSession{ID: "test", Exited: true, ExitCode: 3}}
	data := bytes.Repeat([]byte("0123456789"), terminalBufferSize/10+100)
	terminal.append(data)
	m.items["test"] = terminal
	cursor := int64(0)
	var output []byte
	for {
		chunk, err := m.poll("test", cursor)
		if err != nil {
			t.Fatal(err)
		}
		if cursor == 0 && !chunk.Lost {
			t.Fatal("did not report dropped history")
		}
		if len(chunk.Data) > 65536 {
			t.Fatal("unbounded read")
		}
		output = append(output, chunk.Data...)
		cursor = chunk.Cursor
		if chunk.Exited {
			if chunk.ExitCode != 3 {
				t.Fatal(chunk.ExitCode)
			}
			break
		}
	}
	if !bytes.Equal(output, data[len(data)-terminalBufferSize:]) {
		t.Fatal("replay corrupted or final output lost")
	}
}

func TestTerminalPreservesArgumentsAndLaunchFailure(t *testing.T) {
	argument := "text'; echo injected; #"
	m, s := terminalFixture(t, `printf '%s\n' "$1"`, argument)
	var cursor int64
	_, out := readUntil(t, m, s.ID, &cursor, func(c TerminalChunk, _ string) bool { return c.Exited })
	if strings.TrimSpace(out) != argument {
		t.Fatalf("argument changed: %q", out)
	}
	if _, err := m.start(exec.Command("/no/such/agent"), "codex", ""); err == nil {
		t.Fatal("missing binary accepted")
	}
}

func TestStoppingOneTabLeavesAnotherRunning(t *testing.T) {
	m, first := terminalFixture(t, `printf 'READY\n'; sleep 60`)
	cmd := exec.Command("/bin/sh", "-c", `printf 'READY\n'; IFS= read -r text; printf 'SECOND:%s\n' "$text"`)
	cmd.Dir = t.TempDir()
	second, err := m.start(cmd, "claude", "")
	if err != nil {
		t.Fatal(err)
	}
	var cursor int64
	readUntil(t, m, second.ID, &cursor, func(_ TerminalChunk, out string) bool { return strings.Contains(out, "READY") })
	if err := m.stop(first.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.input(second.ID, "still alive\r"); err != nil {
		t.Fatal(err)
	}
	_, out := readUntil(t, m, second.ID, &cursor, func(c TerminalChunk, _ string) bool { return c.Exited })
	if !strings.Contains(out, "SECOND:still alive") {
		t.Fatal(out)
	}
}
