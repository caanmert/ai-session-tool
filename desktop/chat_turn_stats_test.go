package main

import "testing"

// Each finished turn keeps its time on the message that started it, with the
// tokens and cost the agent reported.
func TestTurnStatsFromBothAgents(t *testing.T) {
	for _, tc := range []struct {
		tool   string
		tokens int64
		cost   float64
	}{
		// Claude: every model's tokens, cache included, and its cost estimate.
		{"claude", 45000 + 600, 0.42},
		// Codex: its running total at the end less the total before the turn.
		{"codex", 142000 - 38000, 0},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			m := fakeChats(t, "context")
			state := createChat(t, m, t.TempDir(), tc.tool, "")
			if err := m.send(state.ID, "Help me"); err != nil {
				t.Fatal(err)
			}
			done := waitChat(t, m, state.ID, func(s ChatState) bool {
				return !busyChat(s.Status) && len(s.Messages) > 0 && s.Messages[0].Turn != nil
			})
			turn := done.Messages[0].Turn
			if turn.Tokens != tc.tokens || turn.CostUSD != tc.cost || turn.Ms < 0 {
				t.Fatalf("turn %+v, want %d tokens and %v", *turn, tc.tokens, tc.cost)
			}
			for _, message := range done.Messages[1:] {
				if message.Turn != nil {
					t.Fatalf("figures on a reply: %+v", message)
				}
			}
		})
	}
	// An agent that reports nothing still has the turn's time.
	m := fakeChats(t, "normal")
	state := createChat(t, m, t.TempDir(), "codex", "")
	if err := m.send(state.ID, "Help me"); err != nil {
		t.Fatal(err)
	}
	done := waitChat(t, m, state.ID, func(s ChatState) bool {
		return !busyChat(s.Status) && len(s.Messages) > 0 && s.Messages[0].Turn != nil
	})
	if turn := done.Messages[0].Turn; turn.Tokens != 0 || turn.CostUSD != 0 {
		t.Fatalf("figures that were never reported: %+v", *turn)
	}
}

// A failed turn still records how long it ran.
func TestFailedTurnKeepsItsTime(t *testing.T) {
	m := fakeChats(t, "writer")
	state := createChat(t, m, t.TempDir(), "codex", "")
	if err := m.send(state.ID, "hello"); err != nil {
		t.Fatal(err)
	}
	done := waitChat(t, m, state.ID, func(s ChatState) bool { return s.Status == "error" })
	if done.Messages[0].Turn == nil {
		t.Fatal("a failed turn has no time")
	}
}
