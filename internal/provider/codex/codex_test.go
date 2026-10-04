package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
)

const (
	idLegacy    = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" // user_message events, every item kind
	idPaginated = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" // item_completed, token_usage_record, .zst
	idSibling   = "cccccccc-cccc-4ccc-8ccc-cccccccccccc" // plain + .zst sibling
	idOld       = "dddddddd-dddd-4ddd-8ddd-dddddddddddd" // response_item fallback, flat token_count
	idEmpty     = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	idSubagent  = "ffffffff-ffff-4fff-8fff-ffffffffffff"
	idReverted  = "99999999-9999-4999-8999-999999999999"
)

func fixture(t *testing.T) *Provider {
	t.Helper()
	root, err := filepath.Abs("../../../testdata/codex")
	if err != nil {
		t.Fatal(err)
	}
	return New(root)
}

// parseID discovers all rollouts and parses the active one of thread id.
func parseID(t *testing.T, p *Provider, id string) (model.Session, []provider.Warning, error) {
	t.Helper()
	refs, err := p.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range refs {
		if strings.Contains(filepath.Base(r.Path), id) {
			return p.Parse(context.Background(), r)
		}
	}
	t.Fatalf("no rollout for %s", id)
	return model.Session{}, nil, nil
}

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

type check struct {
	name      string
	got, want any
}

func run(t *testing.T, checks []check) {
	t.Helper()
	for _, c := range checks {
		if fmt.Sprint(c.got) != fmt.Sprint(c.want) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestParseRolloutName(t *testing.T) {
	tests := []struct {
		in, thread, rollout string
		ok                  bool
	}{
		{"rollout-2026-10-02T14-30-00-" + idLegacy + ".jsonl", idLegacy, idLegacy, true},
		{"rollout-2026-10-02T14-30-00-" + idLegacy + ".jsonl.zst", idLegacy, idLegacy, true},
		{"rollout-2026-10-01T11-00-00-" + idReverted + "_7777.jsonl", idReverted, "7777", true},
		{"rollout-2026-10-02.jsonl", "", "", false},
		{"notes.txt", "", "", false},
	}
	for _, tt := range tests {
		n, ok := parseRolloutName(tt.in)
		if ok != tt.ok || n.thread != tt.thread || n.rollout != tt.rollout {
			t.Errorf("parseRolloutName(%q) = %+v, %v", tt.in, n, ok)
		}
	}
}

func TestDiscover(t *testing.T) {
	refs, err := fixture(t).Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range refs {
		names = append(names, strings.TrimPrefix(filepath.Base(r.Path), "rollout-"))
	}
	sort.Strings(names)
	got := strings.Join(names, "\n")
	want := strings.Join([]string{
		"2026-09-15T08-00-00-" + idOld + ".jsonl",
		"2026-09-16T08-00-00-" + idEmpty + ".jsonl",
		"2026-09-20T09-00-00-" + idPaginated + ".jsonl.zst",                                 // only a compressed copy exists
		"2026-09-21T10-00-00-" + idSibling + ".jsonl",                                       // plain hides its .zst sibling
		"2026-10-01T11-00-00-" + idReverted + "_77777777-7777-4777-8777-777777777777.jsonl", // newest rollout of the thread
		"2026-10-02T14-30-00-" + idLegacy + ".jsonl",
		"2026-10-02T14-31-00-" + idSubagent + ".jsonl",
	}, "\n")
	if got != want {
		t.Errorf("Discover:\n%s\n\nwant (no archived, no notes.txt):\n%s", got, want)
	}
}

func TestDiscoverMissingRoot(t *testing.T) {
	refs, err := New(filepath.Join(t.TempDir(), "nope")).Discover(context.Background())
	if err != nil || len(refs) != 0 {
		t.Fatalf("Discover on missing root = %v, %v", refs, err)
	}
}

func TestParseLegacyEvents(t *testing.T) {
	s, warns, err := parseID(t, fixture(t), idLegacy)
	if err != nil {
		t.Fatal(err)
	}
	run(t, []check{
		{"ID", s.ID, idLegacy},
		{"Tool", s.Tool, model.ToolCodex},
		{"CWD", s.CWD, "/Users/dev/code/api"},
		{"GitBranch", s.GitBranch, "feat/webhooks"},
		{"Version", s.Version, "0.150.0"},
		{"Model", s.Model, "gpt-5.5-codex"},
		{"Title (session_index name, newest entry)", s.Title, "Webhook retries"},
		{"FirstPrompt (event, not injected context)", s.FirstPrompt, "Add retries to the webhook sender. Use exponential backoff."},
		{"LastPrompt", s.LastPrompt, "Also log each retry."},
		{"UserTurns (events only, response copies not double counted)", s.UserTurns, 2},
		{"AssistantTurns (response items, agent_message copies ignored)", s.AssistantTurns, 2},
		// Last cumulative total: input 20000 includes 15000 cached.
		{"Usage", s.Usage, model.Usage{Input: 5000, Output: 1200, CacheRead: 15000}},
		{"StartedAt", s.StartedAt, mustTime("2026-10-02T14:30:00Z")},
		{"UpdatedAt", s.UpdatedAt, mustTime("2026-10-02T14:35:31Z")},
	})
	if len(warns) != 1 || warns[0].Line != 16 || !strings.Contains(warns[0].Msg, "malformed") {
		t.Errorf("warnings = %v, want one malformed line at 16 (and none for tool_search_call or unknown kinds)", warns)
	}
}

func TestParsePaginatedCompressed(t *testing.T) {
	s, warns, err := parseID(t, fixture(t), idPaginated)
	if err != nil {
		t.Fatal(err)
	}
	run(t, []check{
		{"Title (item_completed UserMessage)", s.Title, "Fix the flaky date picker test"},
		{"UserTurns", s.UserTurns, 1},
		{"AssistantTurns", s.AssistantTurns, 1},
		{"Model", s.Model, "gpt-5.4"},
		{"GitBranch", s.GitBranch, "main"},
		// token_usage_record wins over token_count events.
		{"Usage", s.Usage, model.Usage{Input: 1500, Output: 200, CacheRead: 1000, CacheWrite: 500}},
	})
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
}

func TestParseSiblingPrefersPlain(t *testing.T) {
	s, _, err := parseID(t, fixture(t), idSibling)
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Plain copy wins" || strings.HasSuffix(s.Path, ".zst") {
		t.Errorf("got %q from %s", s.Title, s.Path)
	}
}

func TestParseOldFallbacks(t *testing.T) {
	s, _, err := parseID(t, fixture(t), idOld)
	if err != nil {
		t.Fatal(err)
	}
	run(t, []check{
		{"Title (response item, injected context skipped, prefix stripped)", s.Title, "Bump terraform to 1.9"},
		{"UserTurns", s.UserTurns, 1},
		{"AssistantTurns", s.AssistantTurns, 1},
		{"GitBranch", s.GitBranch, ""},
		{"Usage (flat token_count)", s.Usage, model.Usage{Input: 800, Output: 50, CacheRead: 100}},
	})
}

func TestParseSkips(t *testing.T) {
	p := fixture(t)
	if _, _, err := parseID(t, p, idEmpty); !errors.Is(err, provider.ErrEmpty) {
		t.Errorf("empty rollout: err = %v, want ErrEmpty", err)
	}
	if _, _, err := parseID(t, p, idSubagent); !errors.Is(err, provider.ErrHidden) {
		t.Errorf("subagent rollout: err = %v, want ErrHidden", err)
	}
}

func TestParseRevertedThreadFollowsHistoryBase(t *testing.T) {
	p := fixture(t)
	// Parse without Discover first: the rollout index must load lazily.
	path := filepath.Join(p.Root(), "sessions/2026/10/01",
		"rollout-2026-10-01T11-00-00-"+idReverted+"_77777777-7777-4777-8777-777777777777.jsonl")
	s, warns, err := p.Parse(context.Background(), provider.FileRef{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	run(t, []check{
		{"ID", s.ID, idReverted},
		{"Title (from the base prefix)", s.Title, "Write the install guide"},
		{"LastPrompt", s.LastPrompt, "Add a macOS section"},
		{"UserTurns (reverted-away turn excluded)", s.UserTurns, 2},
		{"AssistantTurns", s.AssistantTurns, 2},
		{"StartedAt", s.StartedAt, mustTime("2026-10-01T10:00:00Z")},
		{"UpdatedAt", s.UpdatedAt, mustTime("2026-10-01T11:00:20Z")},
	})
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
}

func TestParseMissingHistoryBaseWarns(t *testing.T) {
	dir := t.TempDir()
	day := filepath.Join(dir, "sessions", "2026", "10", "03")
	if err := os.MkdirAll(day, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(day, "rollout-2026-10-03T08-00-00-abc_def.jsonl")
	writeLines(t, path,
		map[string]any{"timestamp": "2026-10-03T08:00:00Z", "type": "session_meta", "payload": map[string]any{
			"id": "abc", "cwd": "/p", "history_base": map[string]any{"thread_id": "gone", "end_ordinal_exclusive": 1, "end_byte_offset": 10},
		}},
		map[string]any{"timestamp": "2026-10-03T08:00:01Z", "type": "event_msg", "payload": map[string]any{"type": "user_message", "message": "hi"}},
	)
	s, warns, err := New(dir).Parse(context.Background(), provider.FileRef{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if s.UserTurns != 1 || len(warns) != 1 || !strings.Contains(warns[0].Msg, "not found") {
		t.Errorf("UserTurns %d, warnings %v", s.UserTurns, warns)
	}
}

func TestParseHugeLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-2026-10-03T08-00-00-big.jsonl")
	writeLines(t, path,
		map[string]any{"timestamp": "2026-10-03T08:00:00Z", "type": "session_meta", "payload": map[string]any{"id": "big", "cwd": "/p"}},
		map[string]any{"timestamp": "2026-10-03T08:00:01Z", "type": "event_msg", "payload": map[string]any{"type": "user_message", "message": "dump the log"}},
		map[string]any{"timestamp": "2026-10-03T08:00:02Z", "type": "response_item", "payload": map[string]any{
			"type": "function_call_output", "call_id": "c", "output": strings.Repeat("x", 5<<20),
		}},
	)
	s, warns, err := New(dir).Parse(context.Background(), provider.FileRef{Path: path})
	if err != nil || len(warns) != 0 || s.Title != "dump the log" {
		t.Fatalf("got %q, warnings %v, err %v", s.Title, warns, err)
	}
}

func TestTranscript(t *testing.T) {
	p := fixture(t)
	s, _, err := parseID(t, p, idLegacy)
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := p.Transcript(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range msgs {
		name := ""
		if m.ToolName != "" {
			name = m.ToolName + " "
		}
		got = append(got, fmt.Sprintf("%s/%s:%s%s", m.Role, m.Kind, name, m.Text))
	}
	want := []string{
		"user/text:Add retries to the webhook sender.\nUse exponential backoff.",
		"assistant/thinking:Find the sender first.",
		`assistant/tool_use:shell rg -n "func Send" internal/webhook`,
		"user/tool_result:internal/webhook/send.go:12:func Send(ctx context.Context) error {",
		"assistant/tool_use:apply_patch internal/webhook/send.go, internal/webhook/retry.go",
		"user/tool_result:Success. Updated the following files:\nM internal/webhook/send.go",
		"assistant/tool_use:shell go test ./internal/webhook/...",
		"assistant/text:Added retries with exponential backoff (max 5 attempts).",
		"user/text:Also log each retry.",
		"assistant/text:Done: each retry now logs the attempt and delay.",
	}
	if strings.Join(got, "\n---\n") != strings.Join(want, "\n---\n") {
		t.Errorf("transcript:\n%s\n\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestResumeCmd(t *testing.T) {
	p := New(t.TempDir())
	s := model.Session{ID: "abc", CWD: "/work/api"}
	for fork, want := range map[bool]string{false: "codex resume abc", true: "codex fork abc"} {
		cmd, err := p.ResumeCmd(s, fork)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(cmd.Args, " "); got != want || cmd.Dir != "/work/api" {
			t.Errorf("fork=%v: %q in %q, want %q", fork, got, cmd.Dir, want)
		}
	}
	if _, err := p.ResumeCmd(model.Session{ID: "abc"}, false); err == nil {
		t.Error("expected error when cwd is unknown")
	}
}

func writeLines(t *testing.T, path string, lines ...map[string]any) {
	t.Helper()
	var b strings.Builder
	for _, l := range lines {
		data, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestUsageBreakdownFromRunningTotals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-2026-10-03T08-00-00-tot.jsonl")
	total := func(ts string, in, cached, out int64) map[string]any {
		return map[string]any{"timestamp": ts, "type": "event_msg", "payload": map[string]any{"type": "token_count",
			"info": map[string]any{"total_token_usage": map[string]any{"input_tokens": in, "cached_input_tokens": cached, "output_tokens": out}}}}
	}
	ctxModel := func(ts, m string) map[string]any {
		return map[string]any{"timestamp": ts, "type": "turn_context", "payload": map[string]any{"cwd": "/p", "model": m}}
	}
	writeLines(t, path,
		map[string]any{"timestamp": "2026-10-03T08:00:00Z", "type": "session_meta", "payload": map[string]any{"id": "tot", "cwd": "/p"}},
		map[string]any{"timestamp": "2026-10-03T08:00:01Z", "type": "event_msg", "payload": map[string]any{"type": "user_message", "message": "go"}},
		ctxModel("2026-10-03T08:00:02Z", "gpt-a"),
		total("2026-10-03T08:01:00Z", 1000, 200, 100),
		total("2026-10-03T08:01:00Z", 1000, 200, 100), // repeated total (rate-limit update): no new usage
		ctxModel("2026-10-03T08:30:00Z", "gpt-b"),
		total("2026-10-03T08:31:00Z", 3000, 1200, 300),
		total("2026-10-03T08:40:00Z", 10, 0, 1), // totals restarted: new baseline, nothing counted
		total("2026-10-03T08:41:00Z", 110, 0, 11),
	)
	s, _, err := New(dir).Parse(context.Background(), provider.FileRef{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	var sum model.Usage
	for _, e := range s.Breakdown {
		got = append(got, fmt.Sprintf("%s %s %v", e.Slot.Format("15:04"), e.Model, e.Usage))
		sum = sum.Add(e.Usage)
	}
	want := []string{
		"08:00 gpt-a {800 100 200 0 0}",
		"08:30 gpt-b {1100 210 1000 0 0}", // 1000 in + 200 out from the second step, then 100 in + 10 out after the restart
	}
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Errorf("breakdown:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if sum != s.Usage {
		t.Errorf("breakdown sums to %v, session usage is %v", sum, s.Usage)
	}
}
