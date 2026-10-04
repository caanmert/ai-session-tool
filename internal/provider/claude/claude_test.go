package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
)

const fixtureRoot = "../../../testdata/claude"

func fixture(t *testing.T) *Provider {
	t.Helper()
	root, err := filepath.Abs(fixtureRoot)
	if err != nil {
		t.Fatal(err)
	}
	return New(root)
}

func parseFile(t *testing.T, p *Provider, rel string) (model.Session, []provider.Warning, error) {
	t.Helper()
	path := filepath.Join(p.Root(), "projects", rel)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return p.Parse(context.Background(), provider.FileRef{Path: path, Size: info.Size(), ModTime: info.ModTime()})
}

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestDiscoverSkipsSubagentTranscripts(t *testing.T) {
	p := fixture(t)
	refs, err := p.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range refs {
		names = append(names, filepath.Base(r.Path))
	}
	got := strings.Join(names, ",")
	if strings.Contains(got, "agent-") {
		t.Fatalf("Discover returned a subagent transcript: %s", got)
	}
	if len(refs) != 4 {
		t.Fatalf("Discover found %d files (%s), want 4", len(refs), got)
	}
}

func TestDiscoverMissingRoot(t *testing.T) {
	refs, err := New(filepath.Join(t.TempDir(), "nope")).Discover(context.Background())
	if err != nil || len(refs) != 0 {
		t.Fatalf("Discover on missing root = %v, %v; want empty, nil", refs, err)
	}
}

func TestParseFullSession(t *testing.T) {
	p := fixture(t)
	s, warns, err := parseFile(t, p, "-Users-dev-code-api/11111111-1111-4111-8111-111111111111.jsonl")
	if err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		name      string
		got, want any
	}{
		{"ID", s.ID, "11111111-1111-4111-8111-111111111111"},
		{"Tool", s.Tool, model.ToolClaude},
		{"CWD", s.CWD, "/Users/dev/code/api"},
		{"GitBranch (latest)", s.GitBranch, "feat/auth"},
		{"Title (summary wins)", s.Title, "Fix auth token refresh"},
		{"FirstPrompt", s.FirstPrompt, "The refresh token expires after 1h but we never renew it. Can you fix internal/auth/refresh.go?"},
		{"LastPrompt", s.LastPrompt, "Thanks! Now add a test."},
		{"UserTurns (no meta/command/interrupt/tool result/sidechain)", s.UserTurns, 2},
		{"AssistantTurns (deduped by message id, no sidechain)", s.AssistantTurns, 3},
		{"Model (synthetic skipped)", s.Model, "claude-opus-5-5"},
		{"Version", s.Version, "2.1.200"},
		{"StartedAt", s.StartedAt, mustTime("2026-09-30T10:00:00Z")},
		{"UpdatedAt", s.UpdatedAt, mustTime("2026-09-30T10:10:05Z")},
		// msg_A1 counted once + sidechain msg_S1 + msg_A2 + msg_A3.
		{"Usage", s.Usage, model.Usage{Input: 35, Output: 550, CacheRead: 3000, CacheWrite: 600}},
	}
	for _, c := range checks {
		if fmt.Sprint(c.got) != fmt.Sprint(c.want) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}

	if len(warns) != 1 || warns[0].Line != 12 || !strings.Contains(warns[0].Msg, "malformed") {
		t.Errorf("warnings = %v, want one malformed-JSON warning on line 12", warns)
	}
}

func TestParseTitleSources(t *testing.T) {
	p := fixture(t)
	tests := []struct {
		file, title string
		users       int
	}{
		{"-Users-dev-code-web/22222222-2222-4222-8222-222222222222.jsonl", "Dark mode", 1},        // custom-title record
		{"-Users-dev-code-api/44444444-4444-4444-8444-444444444444.jsonl", "/review src/auth", 0}, // only a slash command
	}
	for _, tt := range tests {
		s, _, err := parseFile(t, p, tt.file)
		if err != nil {
			t.Fatalf("%s: %v", tt.file, err)
		}
		if s.Title != tt.title || s.UserTurns != tt.users {
			t.Errorf("%s: title %q users %d, want %q %d", tt.file, s.Title, s.UserTurns, tt.title, tt.users)
		}
	}
}

func TestParseEmptySession(t *testing.T) {
	_, _, err := parseFile(t, fixture(t), "-Users-dev-code-api/33333333-3333-4333-8333-333333333333.jsonl")
	if !errors.Is(err, provider.ErrEmpty) {
		t.Fatalf("err = %v, want ErrEmpty", err)
	}
}

func TestParseHugeLineAndUnknownTypes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "55555555-5555-4555-8555-555555555555.jsonl")
	huge := strings.Repeat("x", 5<<20)
	lines := []map[string]any{
		{"type": "some-future-record", "payload": map[string]any{"nested": true}},
		{"type": "user", "cwd": "/tmp/p", "timestamp": "2026-01-01T00:00:00Z", "message": map[string]any{"role": "user", "content": "hello"}},
		{"type": "user", "cwd": "/tmp/p", "timestamp": "2026-01-01T00:00:01Z", "message": map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "tool_result", "tool_use_id": "t", "content": huge},
		}}},
		{"type": "assistant", "timestamp": "2026-01-01T00:00:02Z", "message": map[string]any{"id": "m1", "model": "m", "content": []any{
			map[string]any{"type": "text", "text": "hi"},
		}}},
	}
	var b strings.Builder
	for _, l := range lines {
		data, _ := json.Marshal(l)
		b.Write(data)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	s, warns, err := New(dir).Parse(context.Background(), provider.FileRef{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if s.UserTurns != 1 || s.AssistantTurns != 1 || s.Title != "hello" {
		t.Errorf("got users %d assistants %d title %q", s.UserTurns, s.AssistantTurns, s.Title)
	}
}

func TestPromptText(t *testing.T) {
	tests := []struct {
		name   string
		blocks []block
		text   string
		kind   promptKind
	}{
		{"plain", []block{{Type: "text", Text: " hi "}}, "hi", promptReal},
		{"image only", []block{{Type: "image"}}, "[image]", promptReal},
		{"tool result", []block{{Type: "tool_result"}}, "", promptNone},
		{"interrupt", []block{{Type: "text", Text: "[Request interrupted by user for tool use]"}}, "", promptNone},
		{"command no args", []block{{Type: "text", Text: "<command-name>/clear</command-name>\n<command-args></command-args>"}}, "/clear", promptCommand},
		{"bash mode", []block{{Type: "text", Text: "<bash-input>ls</bash-input>"}}, "!ls", promptCommand},
		{"local stdout", []block{{Type: "text", Text: "<local-command-stdout>ok</local-command-stdout>"}}, "", promptNone},
	}
	for _, tt := range tests {
		text, kind := promptText(tt.blocks)
		if text != tt.text || kind != tt.kind {
			t.Errorf("%s: got (%q, %d), want (%q, %d)", tt.name, text, kind, tt.text, tt.kind)
		}
	}
}

func TestTranscript(t *testing.T) {
	p := fixture(t)
	s, _, err := parseFile(t, p, "-Users-dev-code-api/11111111-1111-4111-8111-111111111111.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := p.Transcript(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range msgs {
		got = append(got, fmt.Sprintf("%s/%s:%s%s", m.Role, m.Kind, prefix(m.ToolName), m.Text))
	}
	want := []string{
		"user/text:/model",
		"user/text:The refresh token expires after 1h but we never renew it.\nCan you fix internal/auth/refresh.go?",
		"assistant/thinking:Look at refresh.go first.",
		"assistant/tool_use:Bash go test ./internal/auth/...",
		"user/tool_result:--- FAIL: TestRefresh\nexpected renewal",
		"assistant/text:Fixed: we now renew the token 5 minutes before expiry.",
		"user/text:Thanks! Now add a test.",
		"assistant/text:API Error: overloaded",
	}
	if strings.Join(got, "\n---\n") != strings.Join(want, "\n---\n") {
		t.Errorf("transcript:\n%s\n\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func prefix(name string) string {
	if name == "" {
		return ""
	}
	return name + " "
}

func TestLive(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(pid int, id string) {
		data := fmt.Sprintf(`{"pid":%d,"sessionId":%q,"cwd":"/p","status":"busy","updatedAt":1790000000000}`, pid, id)
		if err := os.WriteFile(filepath.Join(root, "sessions", fmt.Sprintf("%d.json", pid)), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(os.Getpid(), "alive-session")
	write(2147483646, "dead-session")
	if err := os.WriteFile(filepath.Join(root, "sessions", "garbage.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	states, err := New(root).Live(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].SessionID != "alive-session" || states[0].Status != "busy" {
		t.Fatalf("Live = %+v, want only alive-session", states)
	}
}

func TestResumeCmd(t *testing.T) {
	p := New(t.TempDir())
	s := model.Session{ID: "abc", CWD: "/work/api"}
	cmd, err := p.ResumeCmd(s, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cmd.Args, " "); got != "claude --resume abc --fork-session" || cmd.Dir != "/work/api" {
		t.Fatalf("ResumeCmd = %q in %q", got, cmd.Dir)
	}
	if _, err := p.ResumeCmd(model.Session{ID: "abc"}, false); err == nil {
		t.Fatal("expected error when cwd is unknown")
	}
}
