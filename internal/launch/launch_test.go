package launch

import (
	"os/exec"
	"testing"
)

func TestShellLine(t *testing.T) {
	cmd := &exec.Cmd{Args: []string{"claude", "--resume", "abc-123"}, Dir: "/Users/me/my project/it's"}
	got := ShellLine(cmd)
	want := `cd '/Users/me/my project/it'"'"'s' && claude --resume abc-123`
	if got != want {
		t.Fatalf("ShellLine = %s\nwant        %s", got, want)
	}
}

func TestCheckMissingDir(t *testing.T) {
	cmd := exec.Command("sh")
	cmd.Dir = "/definitely/not/here"
	if err := Check(cmd); err == nil {
		t.Fatal("expected error for missing directory")
	}
}
