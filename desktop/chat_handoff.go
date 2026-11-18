package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/caanmert/ai-session-tool/internal/launch"
)

// A conversation can be continued with the other agent: a new conversation
// with it opens in the same project, with a first message drafted from the
// original (the user's messages, the agent's last reply and the files
// changed). Nothing is sent until the user sends it. The two link to each
// other through HandedFrom and HandedTo.

// HandOff is the conversation started for the other agent and its draft.
type HandOff struct {
	Chat   ChatState `json:"chat"`
	Prompt string    `json:"prompt"`
}

const (
	handOffAsks     = 12
	handOffAskLimit = 600
	handOffReply    = 2000
	handOffFiles    = 30
)

func otherTool(tool string) string {
	if tool == "claude" {
		return "codex"
	}
	return "claude"
}

func clip(text string, limit int) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return strings.TrimSpace(string(runes[:limit])) + "…"
}

// handOffPrompt drafts the first message for the other agent.
func handOffPrompt(state ChatState, changes ChangeSummary) string {
	asks := []string{}
	reply := ""
	for _, m := range state.Messages {
		if m.Kind != "text" || strings.TrimSpace(m.Text) == "" {
			continue
		}
		switch m.Role {
		case "user":
			asks = append(asks, m.Text)
		case "assistant":
			reply = m.Text
		}
	}
	var b strings.Builder
	title := strings.TrimSpace(state.Title)
	if title == "" || title == "New conversation" {
		fmt.Fprintf(&b, "I'm continuing work that started in a %s conversation in this project. Here is where it stands.\n", toolName(state.Tool))
	} else {
		fmt.Fprintf(&b, "I'm continuing work that started in a %s conversation, %q, in this project. Here is where it stands.\n", toolName(state.Tool), title)
	}
	if len(asks) > 0 {
		b.WriteString("\nWhat I asked:\n")
		if left := len(asks) - handOffAsks; left > 0 {
			fmt.Fprintf(&b, "(%d earlier messages left out)\n", left)
			asks = asks[left:]
		}
		for i, ask := range asks {
			fmt.Fprintf(&b, "%d. %s\n", i+1, strings.ReplaceAll(clip(ask, handOffAskLimit), "\n", "\n   "))
		}
	}
	if reply != "" {
		fmt.Fprintf(&b, "\n%s's last reply:\n", toolName(state.Tool))
		for _, line := range strings.Split(clip(reply, handOffReply), "\n") {
			b.WriteString(strings.TrimRight("> "+line, " ") + "\n")
		}
	}
	if changes.Available && len(changes.Files) > 0 {
		b.WriteString("\nFiles changed and not yet committed:\n")
		for i, f := range changes.Files {
			if i == handOffFiles {
				fmt.Fprintf(&b, "- and %d more\n", len(changes.Files)-handOffFiles)
				break
			}
			if f.Binary {
				fmt.Fprintf(&b, "- %s %s (binary)\n", f.Status, f.Path)
			} else {
				fmt.Fprintf(&b, "- %s %s (+%d −%d)\n", f.Status, f.Path, f.Added, f.Removed)
			}
		}
	}
	b.WriteString("\nRead the changed files first, then continue from here.")
	return b.String()
}

// handOff starts a conversation with tool, the other agent or the same one,
// and links the two. With the same agent it continues the work with a fresh
// context.
func (m *chatManager) handOff(ctx context.Context, id, tool string) (HandOff, error) {
	c, err := m.get(id)
	if err != nil {
		return HandOff{}, err
	}
	c.mu.Lock()
	state := cloneState(c.state)
	c.mu.Unlock()
	switch {
	case state.Review != nil:
		return HandOff{}, errors.New("a review is not continued with another agent")
	case busyChat(state.Status):
		return HandOff{}, errors.New("wait for the agent to finish, or stop it, before handing the work over")
	}
	asked := false
	for _, msg := range state.Messages {
		if msg.Role == "user" && msg.Kind == "text" {
			asked = true
			break
		}
	}
	if !asked {
		return HandOff{}, errors.New("there is nothing to hand over yet")
	}
	changes := summarizeChanges(ctx, state.Project)
	if tool == "" {
		tool = otherTool(state.Tool)
	}
	same := tool == state.Tool
	next, err := m.create(state.Project, tool, "", "", nil)
	if err != nil {
		return HandOff{}, err
	}
	n, err := m.get(next.ID)
	if err != nil {
		return HandOff{}, err
	}
	n.mu.Lock()
	if same {
		n.state.ContinuedFrom = id
	} else {
		n.state.HandedFrom = id
	}
	n.change()
	next = cloneState(n.state)
	n.mu.Unlock()
	m.saveOrWarn(n)
	c.mu.Lock()
	if same {
		c.state.ContinuedIn = append(c.state.ContinuedIn, next.ID)
	} else {
		c.state.HandedTo = append(c.state.HandedTo, next.ID)
	}
	c.change()
	c.mu.Unlock()
	m.saveOrWarn(c)
	return HandOff{Chat: next, Prompt: handOffPrompt(state, changes)}, nil
}

// HandOffChat continues a conversation with the other agent: it opens a new
// conversation in the same project and drafts its first message. Nothing is
// sent.
func (a *App) HandOffChat(id string) (HandOff, error) {
	return a.continueWith(id, true)
}

// ContinueChat continues a conversation in a new one with the same agent, so
// the work goes on with an empty context. Its first message is drafted, not
// sent.
func (a *App) ContinueChat(id string) (HandOff, error) {
	return a.continueWith(id, false)
}

func (a *App) continueWith(id string, other bool) (HandOff, error) {
	if err := a.chatReady(); err != nil {
		return HandOff{}, err
	}
	c, err := a.chats.get(id)
	if err != nil {
		return HandOff{}, err
	}
	c.mu.Lock()
	project, tool := c.state.Project, c.state.Tool
	c.mu.Unlock()
	if other {
		tool = otherTool(tool)
	}
	cmd, err := a.service.NewSession(tool, project)
	if err != nil {
		return HandOff{}, err
	}
	if err = launch.Check(cmd); err != nil {
		return HandOff{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.chats.handOff(ctx, id, tool)
}
