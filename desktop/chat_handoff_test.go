package main

import (
	"context"
	"strings"
	"testing"
)

func TestHandOffPrompt(t *testing.T) {
	state := ChatState{Tool: "codex", Title: "Make checkout retries safe", Messages: []ChatMessage{
		{Role: "user", Kind: "text", Text: "Add safe retries.\nKeep errors clear."},
		{Role: "assistant", Kind: "tool", Name: "Bash", Text: "npm test"},
		{Role: "assistant", Kind: "text", Text: "First reply"},
		{Role: "user", Kind: "text", Text: "Now add tests"},
		{Role: "assistant", Kind: "text", Text: "Added tests.\n\nAll pass."},
	}}
	changes := ChangeSummary{Available: true, Files: []ChangedFile{{Path: "src/retry.ts", Status: "A", Added: 24}, {Path: "logo.png", Status: "M", Binary: true}}}
	got := handOffPrompt(state, changes)
	want := `I'm continuing work that started in a Codex conversation, "Make checkout retries safe", in this project. Here is where it stands.

What I asked:
1. Add safe retries.
   Keep errors clear.
2. Now add tests

Codex's last reply:
> Added tests.
>
> All pass.

Files changed and not yet committed:
- A src/retry.ts (+24 −0)
- M logo.png (binary)

Read the changed files first, then continue from here.`
	if got != want {
		t.Fatalf("prompt:\n%s\n--- want ---\n%s", got, want)
	}
	// Long conversations keep the newest asks, each shortened.
	long := ChatState{Tool: "claude", Title: "New conversation"}
	for i := 0; i < handOffAsks+3; i++ {
		long.Messages = append(long.Messages, ChatMessage{Role: "user", Kind: "text", Text: strings.Repeat("x", handOffAskLimit+50)})
	}
	got = handOffPrompt(long, ChangeSummary{})
	if !strings.HasPrefix(got, "I'm continuing work that started in a Claude Code conversation in this project.") || !strings.Contains(got, "(3 earlier messages left out)") || strings.Contains(got, "13. ") || !strings.Contains(got, "x…") || strings.Contains(got, "Files changed") {
		t.Fatalf("long prompt:\n%s", got)
	}
}

func TestHandOff(t *testing.T) {
	repo := newGitRepo(t)
	m := newChats(t.TempDir())
	t.Cleanup(m.close)
	ctx := context.Background()
	state := createChat(t, m, repo.dir, "codex", "")
	if _, err := m.handOff(ctx, state.ID); err == nil || !strings.Contains(err.Error(), "nothing to hand over") {
		t.Fatalf("empty: %v", err)
	}
	c, _ := m.get(state.ID)
	c.mu.Lock()
	c.state.Messages = []ChatMessage{{ID: "u", Role: "user", Kind: "text", Text: "Fix a.txt"}, {ID: "a", Role: "assistant", Kind: "text", Text: "Done."}}
	c.mu.Unlock()
	repo.write("a.txt", "changed\n")
	out, err := m.handOff(ctx, state.ID)
	if err != nil {
		t.Fatal(err)
	}
	if out.Chat.Tool != "claude" || out.Chat.Project != state.Project || out.Chat.HandedFrom != state.ID || len(out.Chat.Messages) != 0 {
		t.Fatalf("new chat: %+v", out.Chat)
	}
	if !strings.Contains(out.Prompt, "1. Fix a.txt") || !strings.Contains(out.Prompt, "> Done.") || !strings.Contains(out.Prompt, "- M a.txt (+1 −1)") {
		t.Fatalf("prompt:\n%s", out.Prompt)
	}
	c.mu.Lock()
	linked := append([]string(nil), c.state.HandedTo...)
	c.mu.Unlock()
	if len(linked) != 1 || linked[0] != out.Chat.ID {
		t.Fatalf("link back: %v", linked)
	}
	// Nothing was sent to the new agent.
	n, _ := m.get(out.Chat.ID)
	n.mu.Lock()
	status := n.state.Status
	n.mu.Unlock()
	if status != "idle" {
		t.Fatalf("the new conversation must wait for the user: %s", status)
	}
	// The links survive a restart.
	again := newChats(m.dataDir)
	t.Cleanup(again.close)
	if err := again.load(); err != nil {
		t.Fatal(err)
	}
	if loaded, err := again.get(out.Chat.ID); err != nil || loaded.state.HandedFrom != state.ID {
		t.Fatalf("saved link: %v", err)
	}
	// Busy conversations and reviews are refused.
	c.mu.Lock()
	c.state.Status = "running"
	c.mu.Unlock()
	if _, err := m.handOff(ctx, state.ID); err == nil || !strings.Contains(err.Error(), "wait for the agent") {
		t.Fatalf("busy: %v", err)
	}
}
