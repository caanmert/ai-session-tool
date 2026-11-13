//go:build darwin

package main

import "golang.org/x/sys/unix"

// Process state and flag from <sys/proc.h>.
const (
	procZombie  = 5      // SZOMB: exited, waiting to be reaped
	procExiting = 0x2000 // P_WEXIT: working on exiting
)

// groupHasLiveProcess reports whether any member of a process group is still
// running. If the group cannot be inspected it assumes so, so errors surface.
func groupHasLiveProcess(pgid int) bool {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.pgrp", pgid)
	if err != nil {
		return true
	}
	for _, p := range procs {
		if p.Proc.P_stat != procZombie && p.Proc.P_flag&procExiting == 0 {
			return true
		}
	}
	return false
}
