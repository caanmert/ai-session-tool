package main

import (
	"errors"
	"fmt"
)

// Codex's web search tool is off unless its web_search setting says
// otherwise. A conversation can choose: "" keeps Codex's own setting,
// "disabled" turns it off, "cached" searches an index of pages, and "live"
// fetches pages as they are now. Claude Code's search tools need no setting;
// they ask for approval like any other tool.
var webSearchModes = map[string]bool{"": true, "disabled": true, "cached": true, "live": true}

func (m *chatManager) setWebSearch(id, mode string) error {
	c, err := m.get(id)
	if err != nil {
		return err
	}
	c.mu.Lock()
	switch {
	case c.state.Tool != "codex":
		c.mu.Unlock()
		return errors.New("only Codex has a web search setting; Claude Code asks before it searches")
	case c.state.Review != nil:
		c.mu.Unlock()
		return errors.New("a review only reads the snapshot; it does not search the web")
	case !webSearchModes[mode]:
		c.mu.Unlock()
		return fmt.Errorf("unknown web search setting %q", mode)
	case busyChat(c.state.Status):
		c.mu.Unlock()
		return errors.New("finish or stop the response before changing web search")
	}
	previous := c.state.WebSearch
	c.state.WebSearch = mode
	c.change()
	c.mu.Unlock()
	if err := m.save(c); err != nil {
		c.mu.Lock()
		c.state.WebSearch = previous
		c.change()
		c.mu.Unlock()
		return err
	}
	return nil
}

// SetChatWebSearch chooses Codex's web search for a conversation, from its
// next message.
func (a *App) SetChatWebSearch(id, mode string) error {
	if err := a.chatReady(); err != nil {
		return err
	}
	return a.chats.setWebSearch(id, mode)
}
