// Package launch hands the terminal over to a coding agent.
package launch

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Check verifies that cmd can run: the binary is on PATH and its working
// directory still exists.
func Check(cmd *exec.Cmd) error {
	if cmd.Err != nil {
		return fmt.Errorf("%s: %w", cmd.Args[0], cmd.Err)
	}
	if cmd.Dir != "" {
		info, err := os.Stat(cmd.Dir)
		if err != nil {
			return fmt.Errorf("project directory is gone: %s", cmd.Dir)
		}
		if !info.IsDir() {
			return fmt.Errorf("project path is not a directory: %s", cmd.Dir)
		}
	}
	return nil
}

// ShellLine renders cmd as a POSIX shell line, e.g. for copying:
// cd '/path/to/project' && claude --resume 1234
func ShellLine(cmd *exec.Cmd) string {
	parts := make([]string, len(cmd.Args))
	for i, a := range cmd.Args {
		parts[i] = Quote(a)
	}
	line := strings.Join(parts, " ")
	if cmd.Dir != "" {
		line = "cd " + Quote(cmd.Dir) + " && " + line
	}
	return line
}

// Quote single-quotes s for a POSIX shell when it contains anything beyond
// a conservative set of safe characters.
func Quote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, unsafeShellRune) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func unsafeShellRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return false
	}
	return !strings.ContainsRune("-_./:=@%+,", r)
}
