package main

import (
	"testing"
	"time"
)

func TestMessagesKeepWhenTheyArrived(t *testing.T) {
	m := newChats(t.TempDir())
	t.Cleanup(m.close)
	state := createChat(t, m, t.TempDir(), "codex", "")
	c, err := m.get(state.ID)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 11, 16, 9, 5, 30, 0, time.FixedZone("EET", 2*3600))
	original := chatNow
	chatNow = func() time.Time { return clock }
	t.Cleanup(func() { chatNow = original })

	c.message(ChatMessage{ID: "reply", Role: "assistant", Kind: "text", Text: "Hel"}, true)
	clock = clock.Add(time.Minute)
	// Streaming more text into the same message keeps its first time.
	c.message(ChatMessage{ID: "reply", Role: "assistant", Kind: "text", Text: "lo"}, true)
	c.message(ChatMessage{ID: "saved", Role: "user", Kind: "text", Text: "Earlier", At: "2026-11-01T08:00:00Z"}, false)
	update, _ := m.read(state.ID, -1)
	got := map[string]string{}
	for _, message := range update.State.Messages {
		got[message.ID] = message.At
	}
	if got["reply"] != "2026-11-16T07:05:30Z" {
		t.Fatalf("a new message is stamped once, in UTC: %q", got["reply"])
	}
	if got["saved"] != "2026-11-01T08:00:00Z" {
		t.Fatalf("a known time is kept: %q", got["saved"])
	}
}
