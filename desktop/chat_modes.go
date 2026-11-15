package main

import (
	"errors"
	"fmt"
	"strings"
)

// A conversation's permission mode decides how much the agent may do without
// asking. ais only selects among modes the agent itself provides; it never
// grants more than the agent's own mode does.
const (
	modeDefault  = ""            // the agent asks, as it does in its own CLI
	modePlan     = "plan"        // look and plan only; nothing is changed
	modeAutoEdit = "acceptEdits" // file edits in the project are not asked about
)

// ModeOption describes one permission mode for the mode picker.
type ModeOption struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
}

// chatModes lists the modes a tool supports, in the order they are offered.
// Codex already edits inside the project without asking in its default
// sandbox, so it has no separate auto-edit mode.
func chatModes(tool string) []ModeOption {
	plan := ModeOption{ID: modePlan, Label: "Plan", Detail: "Reads and proposes a plan. Changes nothing."}
	if tool == "claude" {
		return []ModeOption{
			plan,
			{ID: modeDefault, Label: "Ask", Detail: "Asks before editing files or running commands."},
			{ID: modeAutoEdit, Label: "Auto-edit", Detail: "Edits files in the project without asking. Still asks before commands."},
		}
	}
	return []ModeOption{
		plan,
		{ID: modeDefault, Label: "Default", Detail: "Edits files in the project. Asks before commands that need more access."},
	}
}

func validMode(tool, mode string) bool {
	for _, option := range chatModes(tool) {
		if option.ID == mode {
			return true
		}
	}
	return false
}

// claudeMode is the --permission-mode value for a conversation mode.
func claudeMode(mode string) string {
	if mode == modePlan || mode == modeAutoEdit {
		return mode
	}
	return "default"
}

// codexSandbox is the sandbox a Codex turn runs in for a conversation mode.
func codexSandbox(mode string) string {
	if mode == modePlan {
		return "read-only"
	}
	return "workspace-write"
}

func (m *chatManager) setMode(id, mode string) error {
	c, err := m.get(id)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if c.state.Review != nil {
		c.mu.Unlock()
		return errors.New("a review only reads; it has no permission mode")
	}
	if !validMode(c.state.Tool, mode) {
		c.mu.Unlock()
		return fmt.Errorf("this agent has no %q mode", mode)
	}
	if busyChat(c.state.Status) {
		c.mu.Unlock()
		return errors.New("finish or stop the response before changing the mode")
	}
	previous := c.state.Mode
	c.state.Mode = mode
	c.change()
	c.mu.Unlock()
	if err := m.save(c); err != nil {
		c.mu.Lock()
		c.state.Mode = previous
		c.change()
		c.mu.Unlock()
		return err
	}
	return nil
}

// approveOption answers a request with one of the wider grants the agent
// itself offered for it, such as allowing a command for the rest of the
// session. An option the request did not offer is refused.
func (m *chatManager) approveOption(id, request, option string) error {
	c, err := m.get(id)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.pending[request]
	if p == nil || p.Answered {
		return errors.New("this request is no longer pending")
	}
	offered := false
	for _, choice := range p.View.Options {
		offered = offered || choice.ID == option
	}
	if !offered || strings.TrimSpace(option) == "" {
		return errors.New("the agent did not offer that choice for this request")
	}
	select {
	case p.Reply <- approvalReply{Allow: true, Option: option}:
		p.Answered = true
		return nil
	default:
		return errors.New("request already answered")
	}
}

// ChatModes lists the permission modes the conversation's agent supports.
func (a *App) ChatModes(id string) ([]ModeOption, error) {
	if err := a.chatReady(); err != nil {
		return nil, err
	}
	c, err := a.chats.get(id)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Review != nil {
		return []ModeOption{}, nil
	}
	return chatModes(c.state.Tool), nil
}

// SetChatMode changes a conversation's permission mode from its next message.
func (a *App) SetChatMode(id, mode string) error {
	if err := a.chatReady(); err != nil {
		return err
	}
	return a.chats.setMode(id, mode)
}

// AnswerChatOption allows a request using a wider grant the agent offered.
func (a *App) AnswerChatOption(id, request, option string) error {
	if err := a.chatReady(); err != nil {
		return err
	}
	return a.chats.approveOption(id, request, option)
}
