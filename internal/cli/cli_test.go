package cli

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/caanmert/ai-session-tool/internal/provider"
	"github.com/caanmert/ai-session-tool/internal/provider/claude"
	"github.com/caanmert/ai-session-tool/internal/provider/codex"
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
	t.Setenv("HOME", t.TempDir()) // keep ~ abbreviation out of golden output
	root, absErr := filepath.Abs("../../testdata")
	if absErr != nil {
		t.Fatal(absErr)
	}
	var out bytes.Buffer
	app := &App{
		Out: &out,
		Err: &bytes.Buffer{},
		Now: func() time.Time { return time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC) },
		Providers: func() []provider.Provider {
			return []provider.Provider{
				claude.New(filepath.Join(root, "claude")),
				codex.New(filepath.Join(root, "codex")),
			}
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
		{[]string{"ls"}, []string{"aaaaaaaa", "99999999", "22222222", "11111111", "44444444", "cccccccc", "bbbbbbbb", "dddddddd"}},
		{[]string{"ls", "--tool", "codex"}, []string{"aaaaaaaa", "99999999", "cccccccc", "bbbbbbbb", "dddddddd"}},
		{[]string{"ls", "--tool", "claude"}, []string{"22222222", "11111111", "44444444"}},
		{[]string{"ls", "-p", "web"}, []string{"22222222", "bbbbbbbb"}},
		{[]string{"ls", "--since", "1d"}, []string{"aaaaaaaa"}},
		{[]string{"ls", "--since", "3d"}, []string{"aaaaaaaa", "99999999", "22222222"}},
		{[]string{"ls", "--since", "2026-09-29"}, []string{"aaaaaaaa", "99999999", "22222222", "11111111"}},
		{[]string{"ls", "--live"}, nil},
		{[]string{"ls", "-n", "1"}, []string{"aaaaaaaa"}},
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

var escapes = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func TestColorListMatchesPlainLayout(t *testing.T) {
	plain, err := run(t, "ls")
	if err != nil {
		t.Fatal(err)
	}
	colored, err := run(t, "--color=always", "ls")
	if err != nil {
		t.Fatal(err)
	}
	if !escapes.MatchString(colored) {
		t.Fatal("--color=always produced no escape sequences")
	}
	if got := escapes.ReplaceAllString(colored, ""); got != plain {
		t.Errorf("colored ls differs from plain ls once escapes are removed:\n%s\n---\n%s", got, plain)
	}
	never, err := run(t, "--color=never", "ls")
	if err != nil || never != plain {
		t.Errorf("--color=never should equal plain output (err %v)", err)
	}
}

func TestColorFlagValidation(t *testing.T) {
	if _, err := run(t, "--color=sometimes", "ls"); err == nil || !strings.Contains(err.Error(), "invalid --color") {
		t.Errorf("err = %v", err)
	}
}

func TestShowPretty(t *testing.T) {
	out, err := run(t, "--color=always", "show", "aaaa", "--tools", "--thinking")
	if err != nil {
		t.Fatal(err)
	}
	text := escapes.ReplaceAllString(out, "")
	// The file row holds a machine-specific absolute path; mask it.
	text = regexp.MustCompile(`(?m)^(│ file\s+).*?(\s*│)$`).ReplaceAllString(text, "${1}<path>${2}")
	golden(t, "show_pretty.txt", text)
}

func TestShowCodex(t *testing.T) {
	out, err := run(t, "show", "aaaa", "--tools")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "show_codex.txt", out)
}

func TestResumePrint(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"resume", "2222", "--fork", "--print"}, "cd /Users/dev/code/web && claude --resume 22222222-2222-4222-8222-222222222222 --fork-session\n"},
		{[]string{"resume", "aaaa", "--print"}, "cd /Users/dev/code/api && codex resume aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa\n"},
		{[]string{"resume", "9999", "--fork", "--print"}, "cd /Users/dev/code/docs && codex fork 99999999-9999-4999-8999-999999999999\n"},
	}
	for _, tt := range tests {
		out, err := run(t, tt.args...)
		if err != nil {
			t.Fatalf("%v: %v", tt.args, err)
		}
		if out != tt.want {
			t.Errorf("%v: got %q, want %q", tt.args, out, tt.want)
		}
	}
}

func TestUnknownSession(t *testing.T) {
	for _, args := range [][]string{{"resume", "0000"}, {"show", "x"}} {
		if _, err := run(t, args...); err == nil || !strings.Contains(err.Error(), "no session matches") {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}
