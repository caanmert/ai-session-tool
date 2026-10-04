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
// The buckets are disjoint: Input excludes cached tokens.
type Usage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
	// CacheWrite1h is the part of CacheWrite written with a one-hour TTL,
	// which is priced higher than the default five minutes.
	CacheWrite1h int64 `json:"cacheWrite1h,omitempty"`
}

// Total returns all tokens processed, cache reads and writes included.
func (u Usage) Total() int64 {
	return u.Input + u.Output + u.CacheRead + u.CacheWrite
}

// Add returns the element-wise sum of u and o.
func (u Usage) Add(o Usage) Usage {
	return Usage{
		Input:        u.Input + o.Input,
		Output:       u.Output + o.Output,
		CacheRead:    u.CacheRead + o.CacheRead,
		CacheWrite:   u.CacheWrite + o.CacheWrite,
		CacheWrite1h: u.CacheWrite1h + o.CacheWrite1h,
	}
}

// IsZero reports whether no tokens were used.
func (u Usage) IsZero() bool { return u == Usage{} }

// UsageEntry is the usage of one model in one 15-minute slot (UTC), the
// unit stats are built from: slots bucket cleanly into local days, weeks
// and months in every time zone.
type UsageEntry struct {
	Slot  time.Time `json:"slot"`
	Model string    `json:"model,omitempty"`
	Usage Usage     `json:"usage"`
}

// SlotOf truncates t to its 15-minute usage slot.
func SlotOf(t time.Time) time.Time { return t.UTC().Truncate(15 * time.Minute) }

// AddUsage adds u to the entry for (slot, model) in entries, keeping them
// ordered by slot then model.
func AddUsage(entries []UsageEntry, slot time.Time, modelName string, u Usage) []UsageEntry {
	if u.IsZero() {
		return entries
	}
	for i := range entries {
		if entries[i].Slot.Equal(slot) && entries[i].Model == modelName {
			entries[i].Usage = entries[i].Usage.Add(u)
			return entries
		}
	}
	entries = append(entries, UsageEntry{Slot: slot, Model: modelName, Usage: u})
	for i := len(entries) - 1; i > 0; i-- {
		a, b := entries[i-1], entries[i]
		if a.Slot.Before(b.Slot) || a.Slot.Equal(b.Slot) && a.Model <= b.Model {
			break
		}
		entries[i-1], entries[i] = b, a
	}
	return entries
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
	// Breakdown splits Usage by time slot and model, for stats.
	Breakdown []UsageEntry `json:"breakdown,omitempty"`

	Live *LiveState `json:"live,omitempty"`

	// Your own annotations, kept by ais (never written to the tools' files).
	// OriginalTitle holds the tool's title when you renamed the session.
	OriginalTitle string   `json:"originalTitle,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Pinned        bool     `json:"pinned,omitempty"`
	Archived      bool     `json:"archived,omitempty"`
}

// Key identifies a session across tools: "tool/id".
func (s Session) Key() string { return string(s.Tool) + "/" + s.ID }

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
