package main

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// When a conversation's context fills up, the agent can compact it: it
// summarizes its own history and carries on from the summary. Claude Code
// does this for the /compact command and Codex for thread/compact/start. The
// conversation in ais keeps every message; only the agent's context shrinks.
// It runs only when the user asks.

// compactCommand is what a compacting turn shows as the user's message, and
// what Claude Code is sent.
const compactCommand = "/compact"

// compactable says why a conversation cannot be compacted, or nil.
func compactable(state ChatState) error {
	switch {
	case state.Review != nil:
		return errors.New("a review is not compacted")
	case state.SessionID == "":
		return errors.New("there is nothing to compact yet; send a message first")
	}
	return nil
}

// compacted records that the agent summarized its history: a step in the
// conversation, and a context figure that is unknown until the agent reports
// again. before is the context size before, when the agent said.
func (c *chatSession) compacted(id string, before int64) {
	if id == "" {
		id = uuid.NewString()
	}
	text := "The agent summarized the conversation so far to free its context. Earlier messages stay here."
	if before > 0 {
		text = fmt.Sprintf("The agent summarized the conversation so far to free its context, which held %s tokens. Earlier messages stay here.", groupDigits(before))
	}
	c.message(ChatMessage{ID: "compact/" + id, Role: "assistant", Kind: "tool", Name: "Conversation compacted", Text: text, Status: "completed"}, false)
	c.mu.Lock()
	if c.state.Context != nil && c.state.Context.Used > 0 {
		c.state.Context.Used = 0
		c.change()
	}
	c.mu.Unlock()
}

// groupDigits writes 184000 as "184,000".
func groupDigits(n int64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0 && s[i-1] != '-'; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// compact starts a turn in which the agent compacts its context.
func (m *chatManager) compact(id string) error {
	return m.start(id, compactCommand, nil, true)
}

// CompactChat asks the conversation's agent to summarize its history so the
// conversation can go on with a smaller context.
func (a *App) CompactChat(id string) error {
	if err := a.chatReady(); err != nil {
		return err
	}
	return a.chats.compact(id)
}
