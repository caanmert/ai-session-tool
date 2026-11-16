package main

import "testing"

// One short turn per agent reports how full the context is.
func TestLiveContextUsage(t *testing.T) {
	requireLiveAgents(t)
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			m := newChats(t.TempDir())
			t.Cleanup(m.close)
			s := createChat(t, m, t.TempDir(), provider, "")
			if err := m.send(s.ID, "Reply with the single word ok. Use no tools."); err != nil {
				t.Fatal(err)
			}
			done := finishLiveChat(t, m, s.ID, nil)
			if done.Context == nil {
				t.Fatal("no context was reported")
			}
			t.Logf("%s (%s): %d of %d tokens", provider, done.ActiveModel, done.Context.Used, done.Context.Window)
			if done.Context.Used <= 0 || done.Context.Window <= 0 || done.Context.Used > done.Context.Window {
				t.Fatalf("unexpected context: %+v", *done.Context)
			}
		})
	}
}
