package index

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/caanmert/ai-session-tool/internal/provider"
	"github.com/caanmert/ai-session-tool/internal/provider/claude"
	"github.com/caanmert/ai-session-tool/internal/provider/codex"
)

// fixtures copies testdata into a temp dir so tests can change files.
func fixtures(t *testing.T) (root string, providers []provider.Provider) {
	t.Helper()
	root = t.TempDir()
	src, err := filepath.Abs("../../testdata")
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		dst := filepath.Join(root, rel)
		if info.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	cp := codex.New(filepath.Join(root, "codex"))
	cp.OpenFiles = func(context.Context) ([]byte, error) { return nil, nil }
	return root, []provider.Provider{claude.New(filepath.Join(root, "claude")), cp}
}

func open(t *testing.T) *Index {
	t.Helper()
	ix, err := Open(filepath.Join(t.TempDir(), "cache", "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ix.Close() })
	return ix
}

func syncAll(t *testing.T, ix *Index, providers []provider.Provider) provider.ScanResult {
	t.Helper()
	res, err := ix.Sync(context.Background(), providers)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func asJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSyncMatchesDirectScan(t *testing.T) {
	_, providers := fixtures(t)
	ix := open(t)

	direct, err := provider.Scan(context.Background(), providers)
	if err != nil {
		t.Fatal(err)
	}
	indexed := syncAll(t, ix, providers)
	if got, want := asJSON(t, indexed.Sessions), asJSON(t, direct.Sessions); got != want {
		t.Errorf("indexed sessions differ from a direct scan:\n%s\n--- want ---\n%s", got, want)
	}
	if len(indexed.Warnings) != len(direct.Warnings) || indexed.Empty != direct.Empty || indexed.Hidden != direct.Hidden {
		t.Errorf("warnings %d/%d, empty %d/%d, hidden %d/%d",
			len(indexed.Warnings), len(direct.Warnings), indexed.Empty, direct.Empty, indexed.Hidden, direct.Hidden)
	}
	if parsed, _ := ix.LastSync(); parsed != 11 {
		t.Errorf("first sync parsed %d files, want all 11", parsed)
	}

	// Nothing changed: nothing is parsed, and Load alone returns the same.
	again := syncAll(t, ix, providers)
	if parsed, removed := ix.LastSync(); parsed != 0 || removed != 0 {
		t.Errorf("second sync parsed %d, removed %d; want 0, 0", parsed, removed)
	}
	loaded, err := ix.Load(context.Background(), providers)
	if err != nil {
		t.Fatal(err)
	}
	if asJSON(t, again.Sessions) != asJSON(t, direct.Sessions) || asJSON(t, loaded.Sessions) != asJSON(t, direct.Sessions) {
		t.Error("cached sessions changed between syncs")
	}
}

func TestSyncPicksUpChanges(t *testing.T) {
	root, providers := fixtures(t)
	ix := open(t)
	syncAll(t, ix, providers)

	// Append a prompt to one Claude session.
	path := filepath.Join(root, "claude/projects/-Users-dev-code-web/22222222-2222-4222-8222-222222222222.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, f, `{"type":"user","cwd":"/Users/dev/code/web","timestamp":"2026-10-03T09:00:00Z","message":{"role":"user","content":"Now add a high-contrast theme"}}`+"\n")

	// Delete another session.
	if err := os.Remove(filepath.Join(root, "claude/projects/-Users-dev-code-api/44444444-4444-4444-8444-444444444444.jsonl")); err != nil {
		t.Fatal(err)
	}

	res := syncAll(t, ix, providers)
	if parsed, removed := ix.LastSync(); parsed != 1 || removed != 1 {
		t.Errorf("parsed %d, removed %d; want 1, 1", parsed, removed)
	}
	if res.Sessions[0].ID != "22222222-2222-4222-8222-222222222222" || res.Sessions[0].LastPrompt != "Now add a high-contrast theme" {
		t.Errorf("updated session: %+v", res.Sessions[0])
	}
	for _, s := range res.Sessions {
		if strings.HasPrefix(s.ID, "44444444") {
			t.Error("deleted session is still listed")
		}
	}
	hits, err := ix.Search(context.Background(), "contrast", 0)
	if err != nil || len(hits) != 1 || !strings.HasPrefix(hits[0].ID, "22222222") {
		t.Errorf("search for new text: %+v, %v", hits, err)
	}
}

func TestCodexRenameInvalidatesTitles(t *testing.T) {
	root, providers := fixtures(t)
	ix := open(t)
	syncAll(t, ix, providers)

	names := filepath.Join(root, "codex/session_index.jsonl")
	f, err := os.OpenFile(names, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, f, `{"id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb","thread_name":"Date picker flake","updated_at":"2026-10-03T10:00:00Z"}`+"\n")
	// Make sure the mtime moves even on coarse filesystems.
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(names, later, later); err != nil {
		t.Fatal(err)
	}

	res := syncAll(t, ix, providers)
	if parsed, _ := ix.LastSync(); parsed != 7 {
		t.Errorf("rename should re-parse every Codex rollout (7), parsed %d", parsed)
	}
	for _, s := range res.Sessions {
		if strings.HasPrefix(s.ID, "bbbbbbbb") && s.Title != "Date picker flake" {
			t.Errorf("title = %q after rename", s.Title)
		}
	}
}

func TestSchemaChangeRebuilds(t *testing.T) {
	_, providers := fixtures(t)
	path := filepath.Join(t.TempDir(), "index.db")
	ix, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	syncAll(t, ix, providers)
	if _, err := ix.db.Exec(`PRAGMA user_version = 0`); err != nil { // as if written by an older ais
		t.Fatal(err)
	}
	ix.Close()

	ix, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	if loaded, _ := ix.Load(context.Background(), providers); len(loaded.Sessions) != 0 {
		t.Errorf("old index should be discarded, still has %d sessions", len(loaded.Sessions))
	}
	syncAll(t, ix, providers)
	if parsed, _ := ix.LastSync(); parsed != 11 {
		t.Errorf("rebuild parsed %d, want 11", parsed)
	}
}

func TestConcurrentSyncs(t *testing.T) {
	_, providers := fixtures(t)
	path := filepath.Join(t.TempDir(), "index.db")
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ix, err := Open(path)
			if err != nil {
				errs <- err
				return
			}
			defer ix.Close()
			res, err := ix.Sync(context.Background(), providers)
			if err == nil && len(res.Sessions) != 8 {
				err = os.ErrInvalid
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent sync: %v", err)
		}
	}
}

func TestSearch(t *testing.T) {
	_, providers := fixtures(t)
	ix := open(t)
	syncAll(t, ix, providers)
	ctx := context.Background()

	tests := []struct {
		q       string
		first   string // id prefix of the best hit ("" = no hits)
		snippet string // must appear in the first hit's snippet ("" = metadata-only match)
	}{
		{"webhook", "aaaaaaaa", "webhook"},
		{"retr", "aaaaaaaa", "retries"},                  // prefix
		{`"exponential backoff"`, "aaaaaaaa", "backoff"}, // phrase
		{"install guide", "99999999", "install"},         // reverted thread, base history
		{"infra", "dddddddd", ""},                        // project path only
		{"timezone UTC", "bbbbbbbb", "timezone"},         // compressed rollout
		{"résumé", "", ""},                               // no such text
		{`AND OR NOT "`, "", ""},                         // operators are literal, no syntax error
		{"Reverted-away", "", ""},                        // cut off by history_base
	}
	for _, tt := range tests {
		hits, err := ix.Search(ctx, tt.q, 5)
		if err != nil {
			t.Errorf("%q: %v", tt.q, err)
			continue
		}
		if tt.first == "" {
			if len(hits) != 0 {
				t.Errorf("%q: want no hits, got %+v", tt.q, hits)
			}
			continue
		}
		if len(hits) == 0 || !strings.HasPrefix(hits[0].ID, tt.first) {
			t.Errorf("%q: hits %+v, want %s first", tt.q, hits, tt.first)
			continue
		}
		plain := strings.NewReplacer(MarkStart, "", MarkEnd, "").Replace(hits[0].Snippet)
		if tt.snippet == "" && hits[0].Snippet != "" || !strings.Contains(strings.ToLower(plain), tt.snippet) {
			t.Errorf("%q: snippet %q, want it to contain %q", tt.q, hits[0].Snippet, tt.snippet)
		}
	}
}

func TestFTSQuery(t *testing.T) {
	for in, want := range map[string]string{
		"":                      "",
		"retry":                 `"retry"*`,
		"  retry   backoff ":    `"retry"* "backoff"*`,
		`"exponential backoff"`: `"exponential backoff"`,
		`say "hi there" now`:    `"say"* "hi there" "now"*`,
		`foo"bar`:               `"foo""bar"*`,
		`"unterminated phrase`:  `"unterminated phrase"`,
		`AND OR NOT`:            `"AND"* "OR"* "NOT"*`,
		`""`:                    ``,
	} {
		if got := ftsQuery(in); got != want {
			t.Errorf("ftsQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

func mustWrite(t *testing.T, f *os.File, s string) {
	t.Helper()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
