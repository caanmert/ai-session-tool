package model

import (
	"testing"
	"time"
)

func TestAddEntryKeepsLongPromptUsageApart(t *testing.T) {
	slot := SlotOf(time.Date(2026, 11, 1, 10, 5, 0, 0, time.UTC))
	later := slot.Add(15 * time.Minute)
	var entries []UsageEntry
	entries = AddEntry(entries, UsageEntry{Slot: later, Model: "claude-haiku-5-5", Usage: Usage{Input: 1}})
	entries = AddEntry(entries, UsageEntry{Slot: slot, Model: "claude-haiku-5-5", LongPrompt: true, Usage: Usage{Input: 200_000}})
	entries = AddEntry(entries, UsageEntry{Slot: slot, Model: "claude-haiku-5-5", Usage: Usage{Input: 10}})
	entries = AddEntry(entries, UsageEntry{Slot: slot, Model: "claude-haiku-5-5", Usage: Usage{Input: 5}})
	entries = AddUsage(entries, slot, "claude-opus-5-5", Usage{Input: 7})
	entries = AddEntry(entries, UsageEntry{Slot: slot, Model: "claude-haiku-5-5", LongPrompt: true}) // no tokens

	want := []UsageEntry{
		{Slot: slot, Model: "claude-haiku-5-5", Usage: Usage{Input: 15}},
		{Slot: slot, Model: "claude-haiku-5-5", LongPrompt: true, Usage: Usage{Input: 200_000}},
		{Slot: slot, Model: "claude-opus-5-5", Usage: Usage{Input: 7}},
		{Slot: later, Model: "claude-haiku-5-5", Usage: Usage{Input: 1}},
	}
	if len(entries) != len(want) {
		t.Fatalf("entries = %+v", entries)
	}
	for i := range want {
		if !entries[i].Slot.Equal(want[i].Slot) || entries[i].Model != want[i].Model ||
			entries[i].LongPrompt != want[i].LongPrompt || entries[i].Usage != want[i].Usage {
			t.Errorf("entry %d = %+v, want %+v", i, entries[i], want[i])
		}
	}
}

func TestPromptCountsCachedTokens(t *testing.T) {
	u := Usage{Input: 1, Output: 1000, CacheRead: 20, CacheWrite: 300, CacheWrite1h: 100}
	if got := u.Prompt(); got != 321 {
		t.Errorf("Prompt() = %d, want 321", got)
	}
}
