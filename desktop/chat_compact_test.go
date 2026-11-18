package main

import (
	"context"
	"strings"
	"testing"
)

func TestCompactWithEachAgent(t *testing.T) {
	for _, tool := range []string{"codex", "claude"} {
		t.Run(tool, func(t *testing.T) {
			m := fakeChats(t, "compact")
			fresh := createChat(t, m, t.TempDir(), tool, "")
			if err := m.compact(fresh.ID); err == nil || !strings.Contains(err.Error(), "nothing to compact") {
				t.Fatalf("a conversation without a session: %v", err)
			}
			// A Git project, so a normal turn would keep a checkpoint.
			state := createChat(t, m, newGitRepo(t).dir, tool, "native-thread")
			c, _ := m.get(state.ID)
			c.mu.Lock()
			c.state.Context = &ContextUsage{Used: 180000, Window: 200000}
			c.mu.Unlock()
			if err := m.compact(state.ID); err != nil {
				t.Fatal(err)
			}
			done := waitChat(t, m, state.ID, func(s ChatState) bool { return s.Status == "idle" && len(s.Messages) >= 2 })
			if done.Messages[0].Role != "user" || done.Messages[0].Text != "/compact" || done.Messages[0].Turn == nil {
				t.Fatalf("the compaction turn: %+v", done.Messages[0])
			}
			step := done.Messages[1]
			if step.Kind != "tool" || step.Name != "Conversation compacted" || step.Status != "completed" {
				t.Fatalf("the compaction step: %+v", step)
			}
			if tool == "claude" && !strings.Contains(step.Text, "184,000 tokens") {
				t.Fatalf("Claude reports the size before: %q", step.Text)
			}
			if done.Context == nil || done.Context.Used != 0 || done.Context.Window != 200000 {
				t.Fatalf("the context figure must wait for the next report: %+v", done.Context)
			}
			if len(done.Checkpoints) != 0 {
				t.Fatalf("compacting changes no files, so it keeps no checkpoint: %+v", done.Checkpoints)
			}
		})
	}
}

func TestContinueWithSameAgent(t *testing.T) {
	repo := newGitRepo(t)
	m := newChats(t.TempDir())
	t.Cleanup(m.close)
	state := createChat(t, m, repo.dir, "codex", "")
	c, _ := m.get(state.ID)
	c.mu.Lock()
	c.state.Messages = []ChatMessage{{ID: "u", Role: "user", Kind: "text", Text: "Fix a.txt"}, {ID: "a", Role: "assistant", Kind: "text", Text: "Done."}}
	c.mu.Unlock()
	out, err := m.handOff(context.Background(), state.ID, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if out.Chat.Tool != "codex" || out.Chat.ContinuedFrom != state.ID || out.Chat.HandedFrom != "" || !strings.Contains(out.Prompt, "1. Fix a.txt") {
		t.Fatalf("continued: %+v\n%s", out.Chat, out.Prompt)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.state.ContinuedIn) != 1 || c.state.ContinuedIn[0] != out.Chat.ID || len(c.state.HandedTo) != 0 {
		t.Fatalf("link back: %+v %+v", c.state.ContinuedIn, c.state.HandedTo)
	}
}

func TestGroupDigits(t *testing.T) {
	for n, want := range map[int64]string{0: "0", 999: "999", 1000: "1,000", 184000: "184,000", 1234567: "1,234,567"} {
		if got := groupDigits(n); got != want {
			t.Fatalf("%d: %q", n, got)
		}
	}
}
