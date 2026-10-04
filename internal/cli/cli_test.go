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
	return runIn(t, indexPath, filepath.Join(t.TempDir(), "data"), args...)
}

// runIn also fixes the data directory, to share annotations between runs.
func runIn(t *testing.T, indexPath, dataDir string, args ...string) (stdout string, err error) {
	t.Helper()
	root, absErr := filepath.Abs("../../testdata")
	if absErr != nil {
		t.Fatal(absErr)
	}
	return (&env{root: root, index: indexPath, data: dataDir}).run(t, args...)
}

// env is a test environment: transcripts under root, an index and a data
// directory, and optional stdin.
type env struct {
	root, index, data, stdin, config string
}

// newEnv copies the fixtures so tests may change them (the trash moves files).
func newEnv(t *testing.T) *env {
	t.Helper()
	src, err := filepath.Abs("../../testdata")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	err = filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(root, rel), 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(root, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return &env{root: root, index: filepath.Join(t.TempDir(), "index.db"), data: filepath.Join(t.TempDir(), "data")}
}

func (e *env) run(t *testing.T, args ...string) (stdout string, err error) {
	t.Helper()
	t.Setenv("HOME", t.TempDir()) // keep ~ abbreviation out of golden output
	var out bytes.Buffer
	app := &App{
		In:  strings.NewReader(e.stdin),
		Out: &out,
		Err: &bytes.Buffer{},
		Now: func() time.Time { return time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC) },
		Providers: func() []provider.Provider {
			cp := codex.New(filepath.Join(e.root, "codex"))
			cp.OpenFiles = func(context.Context) ([]byte, error) { return nil, nil }
			return []provider.Provider{
				claude.New(filepath.Join(e.root, "claude")),
				cp,
			}
		},
		Version:   "test",
		IndexPath: func() (string, error) { return e.index, nil },
		DataDir:   func() (string, error) { return e.data, nil },
		ConfigPath: func() (string, error) {
			if e.config != "" {
				return e.config, nil
			}
			return filepath.Join(e.data, "no-config.toml"), nil
		},
	}
	t.Cleanup(func() { app.Close() })
	cmd := NewRootCmd(app)
	cmd.SetArgs(args)
	err = cmd.ExecuteContext(context.Background())
	stdout = strings.ReplaceAll(out.String(), e.root, "$ROOT")
	stdout = strings.ReplaceAll(stdout, e.data, "$DATA")
	if e.config != "" {
		stdout = strings.ReplaceAll(stdout, e.config, "$CONFIG")
	}
	return stdout, err
}

// must runs a command that has to succeed.
func (e *env) must(t *testing.T, args ...string) string {
	t.Helper()
	out, err := e.run(t, args...)
	if err != nil {
		t.Fatalf("ais %s: %v", strings.Join(args, " "), err)
	}
	return out
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

func TestAnnotations(t *testing.T) {
	e := newEnv(t)
	e.must(t, "rename", "1111", "Token", "refresh", "fix")
	e.must(t, "tag", "1111", "bug", "#Auth")
	e.must(t, "pin", "dddd")
	e.must(t, "archive", "4444")
	out := e.must(t, "tag", "2222", "ui")
	if out != "tagged 22222222  Dark mode  #ui\n" {
		t.Errorf("tag confirmation: %q", out)
	}

	golden(t, "ls_annotated.txt", e.must(t, "ls"))
	all := e.must(t, "ls", "--all")
	if !strings.Contains(all, "/review src/auth  (archived)") {
		t.Errorf("ls --all should show the archived session:\n%s", all)
	}
	if got := e.must(t, "ls", "--tag", "bug", "--json"); !strings.Contains(got, `"title": "Token refresh fix"`) ||
		!strings.Contains(got, `"originalTitle": "Fix auth token refresh"`) || !strings.Contains(got, `"tags": [`) ||
		strings.Count(got, `"id":`) != 1 {
		t.Errorf("ls --tag bug --json:\n%s", got)
	}

	show := e.must(t, "show", "1111", "--info")
	for _, want := range []string{"Token refresh fix", "renamed   from: Fix auth token refresh", "marks     #auth #bug"} {
		if !strings.Contains(show, want) {
			t.Errorf("show lacks %q:\n%s", want, show)
		}
	}
	if got := e.must(t, "tags"); got != "#auth  1\n#bug  1\n#ui  1\n" {
		t.Errorf("tags: %q", got)
	}
	// Your titles and tags are searchable.
	if got := e.must(t, "search", "token", "fix"); !strings.HasPrefix(got, "11111111") {
		t.Errorf("search by your title: %q", got)
	}

	e.must(t, "rename", "1111", "--reset")
	e.must(t, "unpin", "dddd")
	e.must(t, "unarchive", "4444")
	e.must(t, "tag", "1111", "--remove", "bug", "auth")
	e.must(t, "tag", "2222", "-r", "ui")
	if got, want := e.must(t, "ls"), mustRead(t, "testdata/ls.txt"); got != want {
		t.Errorf("undoing every annotation should restore the plain list:\n%s\n---\n%s", got, want)
	}

	if _, err := e.run(t, "tag", "1111", "two words"); err == nil {
		t.Error("invalid tag accepted")
	}
	if _, err := e.run(t, "rename", "1111"); err == nil {
		t.Error("rename without a title or --reset accepted")
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestTrashAndRestore(t *testing.T) {
	e := newEnv(t)
	transcript := filepath.Join(e.root, "claude/projects/-Users-dev-code-web/22222222-2222-4222-8222-222222222222.jsonl")

	out := e.must(t, "trash", "2222")
	if out != "trashed 22222222  Dark mode\n" {
		t.Errorf("trash: %q", out)
	}
	if _, err := os.Stat(transcript); !errors.Is(err, os.ErrNotExist) {
		t.Error("the transcript should have left the Claude project folder")
	}
	if strings.Contains(e.must(t, "ls"), "Dark mode") {
		t.Error("a trashed session must not be listed")
	}
	if got := e.must(t, "trash"); !strings.Contains(got, "22222222  claude  now   Dark mode") {
		t.Errorf("trash list: %q", got)
	}

	e.must(t, "restore", "2222")
	if _, err := os.Stat(transcript); err != nil {
		t.Errorf("restore did not bring the transcript back: %v", err)
	}
	if !strings.Contains(e.must(t, "ls"), "Dark mode") {
		t.Error("a restored session should be listed again")
	}

	// Emptying asks first, and "no" keeps everything.
	e.must(t, "trash", "2222")
	e.stdin = "n\n"
	if _, err := e.run(t, "trash", "--empty"); err == nil || err.Error() != "cancelled" {
		t.Errorf("declined --empty: %v", err)
	}
	e.stdin = "y\n"
	if got := e.must(t, "trash", "--empty"); got != "deleted 1 session\n" {
		t.Errorf("--empty: %q", got)
	}
	if got := e.must(t, "trash"); got != "" {
		t.Errorf("trash should be empty: %q", got)
	}
	if _, err := e.run(t, "restore", "2222"); err == nil {
		t.Error("a deleted session can't be restored")
	}
}

func TestTrashCodexUsesCodex(t *testing.T) {
	e := newEnv(t)
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	script := "#!/bin/sh\necho \"$*\" >> " + log + "\n"
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	e.must(t, "trash", "aaaa")
	e.must(t, "restore", "aaaa")
	e.must(t, "trash", "aaaa")
	e.must(t, "trash", "--empty", "--yes")
	calls, _ := os.ReadFile(log)
	want := "archive aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa\nunarchive aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa\n" +
		"archive aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa\ndelete aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa --force\n"
	if string(calls) != want {
		t.Errorf("codex calls:\n%s\nwant:\n%s", calls, want)
	}
}

func TestStats(t *testing.T) {
	e := newEnv(t)
	golden(t, "stats.txt", e.must(t, "stats"))
	golden(t, "stats_model.txt", e.must(t, "stats", "--by", "model"))

	if got := e.must(t, "stats", "--by", "tool", "--tool", "claude"); !regexp.MustCompile(`(?m)^claude\s+3\s`).MatchString(got) || strings.Contains(got, "codex") {
		t.Errorf("--tool claude:\n%s", got)
	}
	if got := e.must(t, "stats", "--since", "2d"); !strings.Contains(got, "Fri Oct 2") || strings.Contains(got, "Sep") {
		t.Errorf("--since 2d:\n%s", got)
	}

	var rep struct {
		By    string `json:"by"`
		Total struct {
			Sessions int     `json:"sessions"`
			Cost     float64 `json:"cost"`
			Complete bool    `json:"complete"`
		} `json:"total"`
		Unpriced []string `json:"unpricedModels"`
	}
	if err := json.Unmarshal([]byte(e.must(t, "stats", "--by", "project", "--json")), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.By != "project" || rep.Total.Sessions != 6 || rep.Total.Complete || len(rep.Unpriced) != 3 {
		t.Errorf("json report: %+v", rep)
	}

	// Pricing the Codex models in the config completes the estimate.
	e.config = filepath.Join(t.TempDir(), "config.toml")
	cfg := "[prices.\"gpt-5\"]\ninput = 1.25\noutput = 10\ncache_read = 0.125\n"
	if err := os.WriteFile(e.config, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	got := e.must(t, "stats", "--by", "model")
	if !strings.Contains(got, "excludes models without a price: (unknown) (add them under [prices] in $CONFIG)") ||
		!regexp.MustCompile(`(?m)^gpt-5\.5-codex .* \$0\.02$`).MatchString(got) {
		t.Errorf("with the gpt-5 family priced, only the model-less old rollout stays unpriced:\n%s", got)
	}

	if _, err := e.run(t, "stats", "--by", "hour"); err == nil {
		t.Error("unknown --by accepted")
	}
	if out, err := e.run(t, "stats", "--since", "1m"); err != nil || out != "" {
		t.Errorf("no usage in the window: %q, %v", out, err)
	}
}
