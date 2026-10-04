package cli

import (
	"strings"
	"testing"
)

func TestRunCommands(t *testing.T) {
	if out, err := run(t, "run", "list", "--json"); err != nil || strings.TrimSpace(out) != "[]" {
		t.Fatalf("list = %q, %v", out, err)
	}
	for _, args := range [][]string{
		{"run"},
		{"run", "--tool", "other", "--", "task"},
		{"run", "--quiet", "-1s", "--", "task"},
		{"run", "--notify=invalid", "--", "task"},
		{"run", "attach", "missing"},
		{"run", "stop", "missing"},
	} {
		if _, err := run(t, args...); err == nil {
			t.Fatalf("ais %v should fail", args)
		}
	}
}
