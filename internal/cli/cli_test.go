package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// run executes ais with args against the fixture transcripts, with a
// fresh index.
func run(t *testing.T, args ...string) (stdout string, err error) {
	t.Helper()
	return runWith(t, filepath.Join(t.TempDir(), "index.db"), args...)
}

// runWith is run with a given index path, to share an index between runs.
func runWith(t *testing.T, indexPath string, args ...string) (stdout string, err error) {
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
		Version:   "test",
		IndexPath: func() (string, error) { return indexPath, nil },
	}
	t.Cleanup(func() { app.Close() })
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

func TestSearch(t *testing.T) {
	out, err := run(t, "search", "webhook", "retr")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "search.txt", out)

	// Metadata-only match: no snippet line.
	out, err = run(t, "search", "infra")
	if err != nil || out != "dddddddd  codex   infra  2w    Bump terraform to 1.9\n" {
		t.Errorf("metadata match: %q, %v", out, err)
	}

	out, err = run(t, "search", "--json", "delay")
	if err != nil {
		t.Fatal(err)
	}
	var results []struct {
		Session struct{ ID string } `json:"session"`
		Snippet string              `json:"snippet"`
	}
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Session.ID != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" ||
		!strings.Contains(results[0].Snippet, "delay") || strings.ContainsAny(results[0].Snippet, "\x02\x03") {
		t.Errorf("json results: %+v", results)
	}

	if out, err := run(t, "search", "--tool", "claude", "webhook"); err != nil || out != "" {
		t.Errorf("filtered search: %q, %v", out, err)
	}
	if out, err := run(t, "search", "--json", "nothing-like-this"); err != nil || strings.TrimSpace(out) != "[]" {
		t.Errorf("empty json search: %q, %v", out, err)
	}
}

func TestSearchHighlightsInColor(t *testing.T) {
	out, err := run(t, "--color=always", "search", "delay")
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(out, "\x02\x03") || !escapes.MatchString(out) {
		t.Errorf("colored search output should use escapes, not raw markers: %q", out)
	}
	if plain := escapes.ReplaceAllString(out, ""); !strings.Contains(plain, "logs the attempt and delay") {
		t.Errorf("snippet missing: %q", plain)
	}
}

func TestNoIndex(t *testing.T) {
	indexed, err := run(t, "ls")
	if err != nil {
		t.Fatal(err)
	}
	direct, err := run(t, "--no-index", "ls")
	if err != nil || direct != indexed {
		t.Errorf("--no-index ls differs (err %v):\n%s\n---\n%s", err, direct, indexed)
	}
	for _, args := range [][]string{{"--no-index", "search", "x"}, {"--no-index", "reindex"}} {
		if _, err := run(t, args...); !errors.Is(err, errNoIndex) {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}

func TestIndexPersistsAndReindexes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	first, err := runWith(t, path, "ls")
	if err != nil {
		t.Fatal(err)
	}
	second, err := runWith(t, path, "ls") // served from the existing index
	if err != nil || second != first {
		t.Errorf("second run differs (err %v)", err)
	}
	out, err := runWith(t, path, "reindex")
	if err != nil || !strings.Contains(out, "indexed 8 sessions from 11 transcripts") {
		t.Errorf("reindex: %q, %v", out, err)
	}
	out, err = runWith(t, path, "doctor")
	if err != nil || !strings.Contains(out, "[index]") || !strings.Contains(out, "sessions     8 (11 transcripts tracked)") {
		t.Errorf("doctor index section: %q, %v", out, err)
	}
}
