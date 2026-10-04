//go:build !unix

// Package proc has small process helpers.
package proc

// Alive reports whether a process with the given pid exists. Live detection
// is only implemented on Unix; elsewhere sessions are never reported live.
func Alive(pid int) bool { return false }
