package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func watchSession(t *testing.T) (*chatManager, *chatSession, string) {
	t.Helper()
	m := newChats("")
	t.Cleanup(m.close)
	state, err := m.create(t.TempDir(), "codex", "", "", []ChatMessage{{ID: "saved", Role: "user", Kind: "text", Text: "earlier"}})
	if err != nil {
		t.Fatal(err)
	}
	c, err := m.get(state.ID)
	if err != nil {
		t.Fatal(err)
	}
	return m, c, state.ID
}

func TestWatchSendsOnlyChangedMessages(t *testing.T) {
	m, c, id := watchSession(t)
	full, err := m.watch(context.Background(), id, -1, 0)
	if err != nil || full.State == nil || full.Partial || len(full.State.Messages) != 1 {
		t.Fatalf("first read should be complete: %+v %v", full, err)
	}
	c.message(ChatMessage{ID: "a", Role: "assistant", Kind: "text", Text: "Hel"}, false)
	c.message(ChatMessage{ID: "tool", Role: "assistant", Kind: "tool", Status: "running"}, false)
	update, _ := m.watch(context.Background(), id, full.Version, 0)
	if !update.Partial || update.Count != 3 || update.First != "saved" || len(update.State.Messages) != 2 {
		t.Fatalf("expected the two new messages: %+v", update)
	}
	c.message(ChatMessage{ID: "a", Text: "lo"}, true)
	next, _ := m.watch(context.Background(), id, update.Version, 0)
	if !next.Partial || len(next.State.Messages) != 1 || next.State.Messages[0].Text != "Hello" || next.Count != 3 {
		t.Fatalf("expected only the streamed message: %+v", next.State.Messages)
	}
	// A change that touches no message still reports the new state.
	c.mu.Lock()
	c.state.Status = "waiting"
	c.change()
	c.mu.Unlock()
	status, _ := m.watch(context.Background(), id, next.Version, 0)
	if !status.Partial || len(status.State.Messages) != 0 || status.State.Status != "waiting" || status.Count != 3 {
		t.Fatalf("expected a status-only update: %+v", status)
	}
	// Readers that are up to date, or whose version is unknown, are handled.
	if same, _ := m.watch(context.Background(), id, status.Version, 0); same.State != nil {
		t.Fatal("no state expected when nothing changed")
	}
	if ahead, _ := m.watch(context.Background(), id, status.Version+10, 0); ahead.Partial || len(ahead.State.Messages) != 3 {
		t.Fatalf("an unknown version should receive everything: %+v", ahead)
	}
}

func TestWatchReportsTrimmedHistory(t *testing.T) {
	m, c, id := watchSession(t)
	start, _ := m.watch(context.Background(), id, -1, 0)
	for i := 0; i < 500; i++ {
		c.message(ChatMessage{ID: fmt.Sprintf("m%d", i), Role: "assistant", Kind: "text", Text: "x"}, false)
	}
	update, _ := m.watch(context.Background(), id, start.Version, 0)
	if !update.Partial || update.Count != 500 || update.First == "saved" || !update.State.Shortened {
		t.Fatalf("expected the oldest message to be trimmed: count=%d first=%q", update.Count, update.First)
	}
}

func TestWatchWaitsForTheNextChange(t *testing.T) {
	m, c, id := watchSession(t)
	current, _ := m.watch(context.Background(), id, -1, 0)

	start := time.Now()
	if idle, _ := m.watch(context.Background(), id, current.Version, 30*time.Millisecond); idle.State != nil || time.Since(start) < 30*time.Millisecond {
		t.Fatalf("an idle watch should wait, then report no change: %+v", idle)
	}

	result := make(chan ChatUpdate, 2)
	for i := 0; i < 2; i++ { // several readers may wait at once
		go func() {
			update, _ := m.watch(context.Background(), id, current.Version, 5*time.Second)
			result <- update
		}()
	}
	time.Sleep(20 * time.Millisecond)
	c.message(ChatMessage{ID: "a", Role: "assistant", Kind: "text", Text: "done"}, false)
	for i := 0; i < 2; i++ {
		select {
		case update := <-result:
			if update.State == nil || len(update.State.Messages) != 1 {
				t.Fatalf("expected the change: %+v", update)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("watch did not wake on change")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_, _ = m.watch(ctx, id, current.Version+1, 5*time.Second)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watch did not stop when its context ended")
	}
}

// A long conversation while a reply streams: compare sending everything on
// each change with sending only what changed.
func BenchmarkWatchStreaming(b *testing.B) {
	for _, mode := range []string{"full", "partial"} {
		b.Run(mode, func(b *testing.B) {
			m := newChats("")
			defer m.close()
			state, _ := m.create(b.TempDir(), "codex", "", "", nil)
			c, _ := m.get(state.ID)
			text := strings.Repeat("A line of an agent reply with some code.\n", 100) // ~4 KiB
			for i := 0; i < 400; i++ {
				c.message(ChatMessage{ID: fmt.Sprintf("m%d", i), Role: "assistant", Kind: "text", Text: text}, false)
			}
			seen, _ := m.watch(context.Background(), state.ID, -1, 0)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c.message(ChatMessage{ID: "reply", Role: "assistant", Kind: "text", Text: "token "}, true)
				version := int64(-1)
				if mode == "partial" {
					version = seen.Version
				}
				seen, _ = m.watch(context.Background(), state.ID, version, 0)
				// What the bridge does with the result.
				if _, err := json.Marshal(seen); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
