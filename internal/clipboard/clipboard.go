// Package clipboard copies text to the system clipboard.
package clipboard

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/muesli/termenv"
)

// Copy puts text on the clipboard using the first available tool (pbcopy
// on macOS; wl-copy, xclip or xsel on Linux). Without one it falls back to
// the OSC 52 terminal escape, which many terminals (iTerm2, kitty,
// WezTerm, tmux with set-clipboard) honor, also over SSH.
func Copy(text string) error {
	for _, c := range [][]string{
		{"pbcopy"},
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
	} {
		path, err := exec.LookPath(c[0])
		if err != nil {
			continue
		}
		cmd := exec.Command(path, c[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	if os.Getenv("TERM") == "" || os.Getenv("TERM") == "dumb" {
		return errors.New("no clipboard tool found")
	}
	termenv.NewOutput(os.Stdout).Copy(text)
	return nil
}
