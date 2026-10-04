//go:build unix

package launch

import (
	"os"
	"os/exec"
	"syscall"
)

// Exec replaces the current process with cmd, so the agent owns the
// terminal (signals, job control, raw mode) exactly as if run directly.
// It only returns on error.
func Exec(cmd *exec.Cmd) error {
	if err := Check(cmd); err != nil {
		return err
	}
	if cmd.Dir != "" {
		if err := os.Chdir(cmd.Dir); err != nil {
			return err
		}
	}
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	return syscall.Exec(cmd.Path, cmd.Args, env)
}
