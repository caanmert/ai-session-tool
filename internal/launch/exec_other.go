//go:build !unix

package launch

import (
	"os"
	"os/exec"
)

// Exec runs cmd attached to the current terminal and exits with its status.
func Exec(cmd *exec.Cmd) error {
	if err := Check(cmd); err != nil {
		return err
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok {
		os.Exit(exitErr.ExitCode())
	}
	if err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
