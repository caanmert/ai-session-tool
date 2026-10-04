// Package model defines the tool-agnostic session types shared by every
// provider, the CLI and (later) the TUI.
package model

import (
	"path/filepath"
	"time"
)

// Tool identifies which coding agent produced a session.
type Tool string

const (
	ToolClaude Tool = "claude"
	ToolCodex  Tool = "codex"
)

// Usage is the token usage of a session, summed over every API response.
type Usage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
}

// Total returns all tokens processed, cache reads and writes included.
func (u Usage) Total() int64 {
	return u.Input + u.Output + u.CacheRead + u.CacheWrite
}

// Add returns the element-wise sum of u and o.
func (u Usage) Add(o Usage) Usage {
	return Usage{
		Input:      u.Input + o.Input,
		Output:     u.Output + o.Output,
		CacheRead:  u.CacheRead + o.CacheRead,
		CacheWrite: u.CacheWrite + o.CacheWrite,
	}
}

// Session is one resumable conversation, summarised from its transcript.
type Session struct {
	Tool      Tool   `json:"tool"`
	ID        string `json:"id"`
	Path      string `json:"path"`
	CWD       string `json:"cwd"`
	GitBranch string `json:"gitBranch,omitempty"`

	// Title is the best human label: the tool's own custom title, else its
	// generated summary, else the first prompt.
	Title       string `json:"title"`
	Summary     string `json:"summary,omitempty"`
	FirstPrompt string `json:"firstPrompt,omitempty"`
	LastPrompt  string `json:"lastPrompt,omitempty"`

	StartedAt time.Time `json:"startedAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	UserTurns      int `json:"userTurns"`
	AssistantTurns int `json:"assistantTurns"`

	Model   string `json:"model,omitempty"`
	Version string `json:"version,omitempty"`
	Usage   Usage  `json:"usage"`

	Live *LiveState `json:"live,omitempty"`
}

// Project is the short name shown in lists: the last element of the cwd.
func (s Session) Project() string {
	if s.CWD == "" {
		return ""
	}
	return filepath.Base(s.CWD)
}

// MessageCount is the number of user prompts plus assistant replies.
func (s Session) MessageCount() int {
	return s.UserTurns + s.AssistantTurns
}

// LiveState describes a session whose agent process is currently running.
type LiveState struct {
	PID       int       `json:"pid"`
	SessionID string    `json:"sessionId"`
	CWD       string    `json:"cwd,omitempty"`
	Status    string    `json:"status,omitempty"` // e.g. "busy", "idle"
	Name      string    `json:"name,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitzero"`
}

// Role is who produced a transcript message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Kind is what a transcript message contains.
type Kind string

const (
	KindText       Kind = "text"
	KindToolUse    Kind = "tool_use"
	KindToolResult Kind = "tool_result"
	KindThinking   Kind = "thinking"
)

// Message is one renderable block of a transcript.
type Message struct {
	Role     Role      `json:"role"`
	Kind     Kind      `json:"kind"`
	Text     string    `json:"text,omitempty"`
	ToolName string    `json:"toolName,omitempty"`
	Time     time.Time `json:"time,omitzero"`
}
