package main

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/caanmert/ai-session-tool/internal/index"
)

// Searching inside conversations: the app's own conversations are scanned
// here and point at the matching message; saved agent sessions come from the
// CLI's full-text index with the matching passage. Everything stays local.

// MessageHit is one conversation that matches a search.
type MessageHit struct {
	// Kind is "chat" for a conversation in the app, "session" for a saved
	// agent session.
	Kind      string `json:"kind"`
	ChatID    string `json:"chatId,omitempty"`
	MessageID string `json:"messageId,omitempty"`
	Key       string `json:"key,omitempty"`
	Title     string `json:"title,omitempty"`
	Tool      string `json:"tool,omitempty"`
	// Snippet is the matching passage, with matches between "\x02" and "\x03".
	Snippet string `json:"snippet"`
}

const (
	chatHitLimit    = 30
	sessionHitLimit = 40
	snippetBefore   = 60
	snippetAfter    = 120
)

// snippetAround cuts text around the first term and marks every term.
func snippetAround(text string, terms []string) string {
	text = strings.Join(strings.Fields(text), " ")
	lower := strings.ToLower(text)
	at := strings.Index(lower, terms[0])
	start, end := max(0, at-snippetBefore), min(len(text), at+len(terms[0])+snippetAfter)
	// Keep the cut on rune boundaries.
	for start > 0 && start < len(text) && text[start]&0xC0 == 0x80 {
		start--
	}
	for end < len(text) && text[end]&0xC0 == 0x80 {
		end++
	}
	part, lowerPart := text[start:end], lower[start:end]
	var b strings.Builder
	if start > 0 {
		b.WriteString("…")
	}
	for i := 0; i < len(part); {
		matched := ""
		for _, term := range terms {
			if term != "" && strings.HasPrefix(lowerPart[i:], term) && len(term) > len(matched) {
				matched = term
			}
		}
		if matched == "" {
			b.WriteByte(part[i])
			i++
			continue
		}
		b.WriteString(index.MarkStart + part[i:i+len(matched)] + index.MarkEnd)
		i += len(matched)
	}
	if end < len(text) {
		b.WriteString("…")
	}
	return b.String()
}

// searchChats finds the app's conversations with a message (or a tool
// step's name) containing every term, newest conversation first.
func (m *chatManager) searchChats(query string, limit int) []MessageHit {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return []MessageHit{}
	}
	type found struct {
		hit     MessageHit
		updated time.Time
	}
	results := []found{}
	m.mu.Lock()
	sessions := make([]*chatSession, 0, len(m.items))
	for _, c := range m.items {
		sessions = append(sessions, c)
	}
	m.mu.Unlock()
	for _, c := range sessions {
		c.mu.Lock()
		state := c.state
		for _, message := range state.Messages {
			text := message.Text
			if message.Kind == "tool" {
				text = message.Name + " " + message.Text
			}
			lower := strings.ToLower(text)
			all := true
			for _, term := range terms {
				all = all && strings.Contains(lower, term)
			}
			if !all {
				continue
			}
			results = append(results, found{MessageHit{Kind: "chat", ChatID: state.ID, MessageID: message.ID, Title: state.Title, Tool: state.Tool, Key: state.SourceKey, Snippet: snippetAround(text, terms)}, state.UpdatedAt})
			break
		}
		c.mu.Unlock()
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].updated.After(results[j].updated) })
	hits := []MessageHit{}
	for i, r := range results {
		if i == limit {
			break
		}
		hits = append(hits, r.hit)
	}
	return hits
}

// SearchMessages finds conversations whose text matches: the app's own
// conversations first, pointing at the matching message, then saved agent
// sessions from the index. A session already shown as an app conversation
// is not listed twice.
func (a *App) SearchMessages(query string) ([]MessageHit, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	hits := []MessageHit{}
	covered := map[string]bool{}
	if a.chats != nil {
		for _, hit := range a.chats.searchChats(query, chatHitLimit) {
			hits = append(hits, hit)
			if hit.Key != "" {
				covered[hit.Key] = true
			}
		}
		for _, chat := range a.chats.list() {
			if chat.SessionID != "" {
				covered[chat.Tool+"/"+chat.SessionID] = true
			}
		}
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	sessions, err := a.service.SearchText(ctx, query, sessionHitLimit)
	if err != nil {
		return hits, nil
	}
	for _, hit := range sessions {
		if !covered[hit.Key] {
			hits = append(hits, MessageHit{Kind: "session", Key: hit.Key, Snippet: hit.Snippet})
		}
	}
	return hits, nil
}
