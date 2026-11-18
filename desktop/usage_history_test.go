package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUsageHistory(t *testing.T) {
	zone := time.FixedZone("UTC+3", 3*3600)
	now := time.Date(2026, 11, 18, 10, 0, 0, 0, zone)
	m := newChats(t.TempDir())
	t.Cleanup(m.close)
	add := func(project, tool string, closed bool, messages ...ChatMessage) {
		state := createChat(t, m, project, tool, "")
		c, _ := m.get(state.ID)
		c.mu.Lock()
		c.state.Messages = messages
		c.state.Closed = closed
		c.mu.Unlock()
	}
	turn := func(at string, ms, tokens int64, cost float64) ChatMessage {
		return ChatMessage{Role: "user", Kind: "text", At: at, Turn: &TurnStats{Ms: ms, Tokens: tokens, CostUSD: cost}}
	}
	alpha, beta := t.TempDir(), t.TempDir()
	add(alpha, "claude", false,
		turn("2026-11-18T06:00:00Z", 60000, 1000, 0.5),
		ChatMessage{Role: "assistant", Kind: "text", Text: "Done"},
		// 22:30 UTC on the 16th is the 17th in the user's zone.
		turn("2026-11-16T22:30:00Z", 30000, 500, 0.25),
		// Before the window, and a turn saved before figures were kept.
		turn("2026-11-10T12:00:00Z", 99000, 9, 9),
		ChatMessage{Role: "user", Kind: "text", At: "2026-11-18T07:00:00Z"},
	)
	// A closed conversation still counts.
	add(beta, "codex", true, turn("2026-11-18T05:00:00Z", 120000, 4000, 0))
	got, err := m.usageHistory(3, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Days) != 3 || got.Days[0].Day != "2026-11-16" || got.Days[2].Day != "2026-11-18" {
		t.Fatalf("days: %+v", got.Days)
	}
	if got.Days[0].Turns != 0 || got.Days[1].Ms != 30000 || got.Days[2].Turns != 2 || got.Days[2].Ms != 180000 {
		t.Fatalf("per day: %+v", got.Days)
	}
	if got.Total != (UsageTotals{Turns: 3, Ms: 210000, Tokens: 5500, CostUSD: 0.75}) {
		t.Fatalf("total: %+v", got.Total)
	}
	if len(got.Projects) != 2 || filepath.Base(got.Projects[0].Key) != filepath.Base(beta) || got.Projects[0].Ms != 120000 || got.Projects[1].Turns != 2 {
		t.Fatalf("projects, most time first: %+v", got.Projects)
	}
	if len(got.Agents) != 2 || got.Agents[0].Key != "codex" || got.Agents[1].Key != "claude" || got.Agents[1].CostUSD != 0.75 {
		t.Fatalf("agents: %+v", got.Agents)
	}
	data, _ := json.Marshal(got.Days[2])
	if !strings.Contains(string(data), `"day":"2026-11-18","turns":2,"ms":180000`) {
		t.Fatalf("totals sit beside the key in JSON: %s", data)
	}
	for _, days := range []int{0, 367} {
		if _, err := m.usageHistory(days, now); err == nil {
			t.Fatalf("%d days must be refused", days)
		}
	}
	empty, _ := newChats(t.TempDir()).usageHistory(7, now)
	if len(empty.Days) != 7 || empty.Total.Turns != 0 || empty.Projects == nil || empty.Agents == nil {
		t.Fatalf("empty: %+v", empty)
	}
}
