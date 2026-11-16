package main

import (
	"strings"
	"testing"
)

// With web search set to live, Codex searches and says what it found.
func TestLiveCodexWebSearch(t *testing.T) {
	requireLiveAgents(t)
	m := newChats(t.TempDir())
	t.Cleanup(m.close)
	s := createChat(t, m, t.TempDir(), "codex", "")
	if err := m.setWebSearch(s.ID, "live"); err != nil {
		t.Fatal(err)
	}
	if err := m.send(s.ID, "Search the web for the page https://example.com and reply with only its title."); err != nil {
		t.Fatal(err)
	}
	done := finishLiveChat(t, m, s.ID, func(ChatApproval) bool { return false })
	searched, reply := false, ""
	for _, message := range done.Messages {
		if message.Kind == "tool" && message.Name == "webSearch" {
			searched = true
		}
		if message.Role == "assistant" && message.Kind == "text" {
			reply = message.Text
		}
	}
	t.Logf("searched: %v; reply: %q", searched, shorten(reply, 200))
	if !searched || !strings.Contains(strings.ToLower(reply), "example domain") {
		t.Fatal("Codex did not search the web")
	}
}
