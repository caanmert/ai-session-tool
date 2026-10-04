//go:build unix

// Package proc has small process helpers.
package proc

import (
	"errors"
	"syscall"
)

// Alive reports whether a process with the given pid exists.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	// EPERM means the process exists but belongs to another user.
	return err == nil || errors.Is(err, syscall.EPERM)
}
