package main

import (
	"strings"
	"testing"
)

func TestCodexWebSearchSetting(t *testing.T) {
	m := fakeChats(t, "websearch")
	state := createChat(t, m, t.TempDir(), "codex", "")
	if err := m.setWebSearch(state.ID, "everything"); err == nil {
		t.Fatal("an unknown setting must be refused")
	}
	if err := m.setWebSearch(state.ID, "live"); err != nil { // the fake requires it in the thread config
		t.Fatal(err)
	}
	if err := m.send(state.ID, "Look it up"); err != nil {
		t.Fatal(err)
	}
	done := waitChat(t, m, state.ID, func(s ChatState) bool { return !busyChat(s.Status) })
	if done.Status != "idle" || done.Error != "" || done.WebSearch != "live" {
		t.Fatalf("turn: %s %s %q", done.Status, done.Error, done.WebSearch)
	}
	// The choice is saved with the conversation.
	reloaded := newChats(m.dataDir)
	t.Cleanup(reloaded.close)
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	if update, err := reloaded.read(state.ID, -1); err != nil || update.State.WebSearch != "live" {
		t.Fatalf("saved: %+v %v", update, err)
	}

	claude := createChat(t, m, t.TempDir(), "claude", "")
	if err := m.setWebSearch(claude.ID, "live"); err == nil || !strings.Contains(err.Error(), "only Codex") {
		t.Fatalf("Claude Code has no setting: %v", err)
	}
	c, _ := m.get(state.ID)
	c.mu.Lock()
	c.state.Status = "running"
	c.mu.Unlock()
	if err := m.setWebSearch(state.ID, "disabled"); err == nil || !strings.Contains(err.Error(), "finish or stop") {
		t.Fatalf("busy: %v", err)
	}
	c.mu.Lock()
	c.state.Status = "idle"
	c.mu.Unlock()
}
