package main

import (
	"strings"
	"testing"
	"time"
)

func TestSnippetAround(t *testing.T) {
	text := strings.Repeat("lead ", 30) + "The Retry key   is reused for every retry attempt." + strings.Repeat(" tail", 40)
	got := snippetAround(text, []string{"retry", "key"})
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("cut: %q", got)
	}
	if !strings.Contains(got, "\x02Retry\x03 \x02key\x03 is reused for every \x02retry\x03 attempt") {
		t.Fatalf("marks: %q", got)
	}
	// Multi-byte text is cut on rune boundaries.
	wide := strings.Repeat("é", 100) + " retry " + strings.Repeat("ü", 200)
	if got := snippetAround(wide, []string{"retry"}); !strings.Contains(got, "\x02retry\x03") || !utf8Valid(got) {
		t.Fatalf("wide: %q", got)
	}
}

func utf8Valid(s string) bool { return strings.ToValidUTF8(s, "?") == s }

func TestSearchChats(t *testing.T) {
	m := newChats(t.TempDir())
	t.Cleanup(m.close)
	older := createChat(t, m, t.TempDir(), "codex", "")
	newer := createChat(t, m, t.TempDir(), "claude", "")
	put := func(id string, updated time.Time, messages ...ChatMessage) {
		c, _ := m.get(id)
		c.mu.Lock()
		c.state.Messages = messages
		c.state.UpdatedAt = updated
		c.mu.Unlock()
	}
	put(older.ID, time.Now().Add(-time.Hour),
		ChatMessage{ID: "o1", Role: "user", Kind: "text", Text: "Fix the checkout"},
		ChatMessage{ID: "o2", Role: "assistant", Kind: "text", Text: "The idempotency key now covers retries."})
	put(newer.ID, time.Now(),
		ChatMessage{ID: "n1", Role: "assistant", Kind: "tool", Name: "npm test -- retries", Text: "ok"},
		ChatMessage{ID: "n2", Role: "assistant", Kind: "text", Text: "Retries pass."})
	hits := m.searchChats("RETRIES", 10)
	if len(hits) != 2 || hits[0].ChatID != newer.ID || hits[0].MessageID != "n1" || hits[1].MessageID != "o2" {
		t.Fatalf("hits: %+v", hits)
	}
	// Every term must be in one message.
	if hits := m.searchChats("checkout retries", 10); len(hits) != 0 {
		t.Fatalf("terms across messages must not match: %+v", hits)
	}
	if hits := m.searchChats("idempotency key", 10); len(hits) != 1 || !strings.Contains(hits[0].Snippet, "\x02idempotency\x03 \x02key\x03") {
		t.Fatalf("snippet: %+v", hits)
	}
	if hits := m.searchChats("retries", 1); len(hits) != 1 {
		t.Fatalf("limit: %+v", hits)
	}
	if hits := m.searchChats("   ", 10); len(hits) != 0 {
		t.Fatal("an empty search finds nothing")
	}
}
