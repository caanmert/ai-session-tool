//go:build darwin

package main

import (
	"os/exec"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestStopToleratesExitedProcessGroup(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "sleep 60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	if !groupHasLiveProcess(pid) {
		_ = unix.Kill(-pid, unix.SIGKILL)
		_ = cmd.Wait()
		t.Fatal("running group reported as exited")
	}
	if err := unix.Kill(-pid, unix.SIGKILL); err != nil {
		t.Fatal(err)
	}
	// Leave the leader unreaped so the group holds only an exited process, as it
	// does between an agent exiting and the terminal's Wait returning.
	deadline := time.Now().Add(5 * time.Second)
	for groupHasLiveProcess(pid) {
		if time.Now().After(deadline) {
			_ = cmd.Wait()
			t.Fatal("killed group still reported as running")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := unix.Kill(-pid, 0); err != unix.EPERM {
		t.Logf("Darwin no longer reports EPERM for an exited group (got %v); the stop below is not exercising that path", err)
	}
	terminal := &liveTerminal{cmd: cmd, done: make(chan struct{})}
	close(terminal.done)
	if err := terminal.stop(); err != nil {
		t.Fatalf("stopping an exited group: %v", err)
	}
	_ = cmd.Wait()
	if err := killGroup(pid); err != nil {
		t.Fatalf("stopping a reaped group: %v", err)
	}
}
