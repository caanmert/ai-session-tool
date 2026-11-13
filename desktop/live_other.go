//go:build !darwin

package main

// Only Darwin reports EPERM for a process group whose members have all exited,
// so elsewhere an EPERM is always a real permission failure.
func groupHasLiveProcess(int) bool { return true }
