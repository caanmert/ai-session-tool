package cli

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/caanmert/ai-session-tool/internal/provider"
	"github.com/caanmert/ai-session-tool/internal/provider/claude"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestMain(m *testing.M) {
	// Golden output renders local times; pin the zone so it matches anywhere.
	time.Local = time.UTC
	os.Exit(m.Run())
}

// run executes ais with args against the fixture transcripts.
func run(t *testing.T, args ...string) (stdout string, err error) {
	t.Helper()
	root, absErr := filepath.Abs("../../testdata/claude")
	if absErr != nil {
		t.Fatal(absErr)
	}
	var out bytes.Buffer
	app := &App{
		Out: &out,
		Err: &bytes.Buffer{},
		Now: func() time.Time { return time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC) },
		Providers: func() []provider.Provider {
			return []provider.Provider{claude.New(root)}
		},
		Version: "test",
	}
	cmd := NewRootCmd(app)
	cmd.SetArgs(args)
	err = cmd.ExecuteContext(context.Background())
	return strings.ReplaceAll(out.String(), root, "$ROOT"), err
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/cli -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("%s mismatch (run go test ./internal/cli -update to accept)\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestListJSON(t *testing.T) {
	out, err := run(t, "ls", "--json")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "ls.json", out)
}

func TestListTable(t *testing.T) {
	out, err := run(t, "ls")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "ls.txt", out)
}

func TestListFilters(t *testing.T) {
	tests := []struct {
		args []string
		want []string // id prefixes, in order
	}{
		{[]string{"ls", "-p", "web"}, []string{"22222222"}},
		{[]string{"ls", "--since", "12h"}, []string{"22222222"}},
		{[]string{"ls", "--since", "1d"}, []string{"22222222", "11111111"}},
		{[]string{"ls", "--since", "2026-09-29"}, []string{"22222222", "11111111"}},
		{[]string{"ls", "--tool", "codex"}, nil},
		{[]string{"ls", "--live"}, nil},
		{[]string{"ls", "-n", "1"}, []string{"22222222"}},
	}
	for _, tt := range tests {
		out, err := run(t, tt.args...)
		if err != nil {
			t.Fatalf("%v: %v", tt.args, err)
		}
		var ids []string
		for _, line := range strings.Split(strings.TrimSpace(out), "\n")[1:] {
			if f := strings.Fields(line); len(f) > 0 {
				ids = append(ids, f[0])
			}
		}
		if strings.Join(ids, ",") != strings.Join(tt.want, ",") {
			t.Errorf("%v: ids %v, want %v", tt.args, ids, tt.want)
		}
	}
}

func TestListRejectsBadFlags(t *testing.T) {
	if _, err := run(t, "ls", "--tool", "gpt"); err == nil {
		t.Error("expected error for unknown tool")
	}
	if _, err := run(t, "ls", "--since", "yesterday"); err == nil {
		t.Error("expected error for bad --since")
	}
}

func TestShow(t *testing.T) {
	out, err := run(t, "show", "1111")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "show.txt", out)
}

func TestResumePrint(t *testing.T) {
	out, err := run(t, "resume", "2222", "--fork", "--print")
	if err != nil {
		t.Fatal(err)
	}
	want := "cd /Users/dev/code/web && claude --resume 22222222-2222-4222-8222-222222222222 --fork-session\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestUnknownSession(t *testing.T) {
	for _, args := range [][]string{{"resume", "9999"}, {"show", "x"}} {
		if _, err := run(t, args...); err == nil || !strings.Contains(err.Error(), "no session matches") {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}

func TestFormatting(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for d, want := range map[time.Duration]string{
		10 * time.Second: "now", 5 * time.Minute: "5m", 3 * time.Hour: "3h",
		4 * 24 * time.Hour: "4d", 20 * 24 * time.Hour: "2w",
	} {
		if got := age(now, now.Add(-d)); got != want {
			t.Errorf("age(%v) = %s, want %s", d, got, want)
		}
	}
	for n, want := range map[int64]string{0: "0", 999: "999", 1000: "1k", 12345: "12.3k", 4_100_000: "4.1M", 1_200_000_000: "1.2B"} {
		if got := tokens(n); got != want {
			t.Errorf("tokens(%d) = %s, want %s", n, got, want)
		}
	}
	if got := truncate("héllo world", 5); got != "héll…" {
		t.Errorf("truncate = %q", got)
	}
}
