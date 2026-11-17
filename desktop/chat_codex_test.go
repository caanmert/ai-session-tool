package main

import (
	"encoding/json"
	"testing"
)

func TestCodexErrorsReadPlainly(t *testing.T) {
	for raw, want := range map[string]string{
		`{"code":-32600,"message":"thread 01a1 already has an active writer"}`: errOpenElsewhere.Error(),
		`{"code":-32603,"message":"  model not available  "}`:                  "Codex: model not available",
		`{"code":-32603}`: "Codex: {\n  \"code\": -32603\n}",
		`"plain"`:         "Codex: \"plain\"",
	} {
		if got := codexError("Codex", json.RawMessage(raw)).Error(); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

// A conversation another Codex program has open fails with a message that
// says so, and the user's message stays in the conversation.
func TestCodexConversationOpenElsewhere(t *testing.T) {
	m := fakeChats(t, "writer")
	chat := createChat(t, m, t.TempDir(), "codex", "")
	if err := m.send(chat.ID, "hello"); err != nil {
		t.Fatal(err)
	}
	state := waitChat(t, m, chat.ID, func(s ChatState) bool { return s.Status == "error" })
	if state.Error != errOpenElsewhere.Error() {
		t.Fatalf("unexpected error %q", state.Error)
	}
	if len(state.Messages) == 0 || state.Messages[0].Text != "hello" {
		t.Fatalf("the message was not kept: %+v", state.Messages)
	}
}
