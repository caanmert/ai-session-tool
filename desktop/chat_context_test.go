package main

import (
	"encoding/json"
	"testing"
)

func TestContextUsageFromBothAgents(t *testing.T) {
	for _, tc := range []struct {
		tool string
		want ContextUsage
	}{
		// Claude: input + cache write + cache read + output of the latest
		// response, against the answering model's window.
		{"claude", ContextUsage{Used: 45000, Window: 200000}},
		// Codex: the last request's total less its reasoning.
		{"codex", ContextUsage{Used: 50000, Window: 272000}},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			m := fakeChats(t, "context")
			state := createChat(t, m, t.TempDir(), tc.tool, "")
			if err := m.send(state.ID, "Help me"); err != nil {
				t.Fatal(err)
			}
			done := waitChat(t, m, state.ID, func(s ChatState) bool {
				return !busyChat(s.Status) && s.Context != nil && s.Context.Window > 0
			})
			if *done.Context != tc.want {
				t.Fatalf("context: %+v, want %+v", *done.Context, tc.want)
			}
		})
	}
	// An agent that reports nothing leaves no figure, so none is shown.
	m := fakeChats(t, "normal")
	state := createChat(t, m, t.TempDir(), "claude", "")
	if err := m.send(state.ID, "Help me"); err != nil {
		t.Fatal(err)
	}
	if done := waitChat(t, m, state.ID, func(s ChatState) bool { return s.Status == "idle" }); done.Context != nil {
		t.Fatalf("no report must mean no figure: %+v", *done.Context)
	}
}

func TestContextUsageHelpers(t *testing.T) {
	usage := object(json.RawMessage(`{"claude-a":{"contextWindow":200000},"claude-b":{"contextWindow":1000000}}`))
	if got := claudeContextWindow(usage, "claude-a"); got != 200000 {
		t.Fatalf("answering model: %d", got)
	}
	if got := claudeContextWindow(usage, "unlisted"); got != 1000000 {
		t.Fatalf("largest when the model isn't listed: %d", got)
	}
	if got := claudeContextWindow(object(json.RawMessage(`null`)), "x"); got != 0 {
		t.Fatalf("no usage: %d", got)
	}
	used, window := codexContextUsage(object(json.RawMessage(`{"tokenUsage":{"last":{"totalTokens":1000,"reasoningOutputTokens":100},"modelContextWindow":null}}`)))
	if used != 900 || window != 0 {
		t.Fatalf("codex: %d %d", used, window)
	}

	m := newChats(t.TempDir())
	t.Cleanup(m.close)
	state := createChat(t, m, t.TempDir(), "codex", "")
	c, _ := m.get(state.ID)
	// The two halves can arrive separately; each keeps the other.
	c.reportContext(42000, 0)
	c.reportContext(0, 128000)
	c.reportContext(0, 0)
	c.reportContext(50000, 0)
	update, _ := m.read(state.ID, -1)
	if got := update.State.Context; got == nil || *got != (ContextUsage{Used: 50000, Window: 128000}) {
		t.Fatalf("context: %+v", got)
	}
}
