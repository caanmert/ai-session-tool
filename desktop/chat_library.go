package main

import (
	"errors"
	"sort"
	"time"
)

// ChatSummary intentionally omits messages, approvals, findings and patches.
// Listing a library must not serialize every saved conversation's contents.
type ChatSummary struct {
	ID           string    `json:"id"`
	Tool         string    `json:"tool"`
	Project      string    `json:"project"`
	Title        string    `json:"title"`
	Status       string    `json:"status"`
	UpdatedAt    time.Time `json:"updatedAt"`
	Closed       bool      `json:"closed"`
	IsReview     bool      `json:"isReview"`
	Complete     bool      `json:"complete"`
	FindingCount int       `json:"findingCount"`
	SentCount    int       `json:"sentCount"`
	FileCount    int       `json:"fileCount"`
}

func (m *chatManager) library() []ChatSummary {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]ChatSummary, 0, len(m.items))
	for _, c := range m.items {
		c.mu.Lock()
		s := c.state
		item := ChatSummary{ID: s.ID, Tool: s.Tool, Project: s.Project, Title: s.Title,
			Status: s.Status, UpdatedAt: s.UpdatedAt, Closed: s.Closed, IsReview: s.Review != nil}
		if s.Review != nil {
			item.Complete = s.Review.Complete
			item.FindingCount = len(s.Review.Findings)
			item.FileCount = len(s.Review.Snapshot.Files)
			for _, f := range s.Review.Findings {
				if f.Sent {
					item.SentCount++
				}
			}
		}
		c.mu.Unlock()
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		}
		return items[i].ID < items[j].ID
	})
	return items
}

// reopen restores the same record without launching a provider or validating a
// project directory. Saved reviews remain readable when the project is offline.
func (m *chatManager) reopen(id string) (ChatState, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ChatState{}, errors.New("the app is closing")
	}
	c := m.items[id]
	if c == nil {
		m.mu.Unlock()
		return ChatState{}, errors.New("saved conversation not found")
	}
	open := 0
	for _, other := range m.items {
		other.mu.Lock()
		if !other.state.Closed {
			open++
		}
		other.mu.Unlock()
	}
	c.mu.Lock()
	wasClosed := c.state.Closed
	if wasClosed && open >= 100 {
		c.mu.Unlock()
		m.mu.Unlock()
		return ChatState{}, errors.New("close an older conversation before opening another (maximum 100)")
	}
	if wasClosed {
		c.state.Closed = false
		c.change()
	}
	state := cloneState(c.state)
	c.mu.Unlock()
	m.mu.Unlock()
	if wasClosed {
		if err := m.save(c); err != nil {
			c.mu.Lock()
			if c.state.Version == state.Version {
				c.state.Closed = true
				c.change()
			}
			c.mu.Unlock()
			return ChatState{}, err
		}
	}
	return state, nil
}
