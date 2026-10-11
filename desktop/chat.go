package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/caanmert/ai-session-tool/internal/application"
	"github.com/google/uuid"
)

type ChatMessage struct {
	ID     string `json:"id"`
	Role   string `json:"role"`
	Kind   string `json:"kind"`
	Text   string `json:"text"`
	Name   string `json:"name,omitempty"`
	Status string `json:"status,omitempty"`
	Model  string `json:"model,omitempty"`
}
type ChatQuestion struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	Options []string `json:"options"`
	Secret  bool     `json:"secret,omitempty"`
}
type ChatApproval struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Detail    string         `json:"detail"`
	Questions []ChatQuestion `json:"questions"`
}
type ChatState struct {
	Settings         ModelSettings  `json:"settings"`
	ReviewerSettings ModelSettings  `json:"reviewerSettings"`
	ActiveModel      string         `json:"activeModel"`
	ID               string         `json:"id"`
	Tool             string         `json:"tool"`
	Project          string         `json:"project"`
	SessionID        string         `json:"sessionId"`
	SourceKey        string         `json:"sourceKey"`
	Title            string         `json:"title"`
	Status           string         `json:"status"`
	Error            string         `json:"error"`
	Messages         []ChatMessage  `json:"messages"`
	Approvals        []ChatApproval `json:"approvals"`
	Review           *ReviewState   `json:"review,omitempty"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	Shortened        bool           `json:"shortened"`
	Version          int64          `json:"version"`
	Closed           bool           `json:"closed"`
}
type ChatUpdate struct {
	Version int64      `json:"version"`
	State   *ChatState `json:"state"`
}
type approvalReply struct {
	Allow   bool
	Answers map[string]string
}
type pendingApproval struct {
	View     ChatApproval
	Reply    chan approvalReply
	Cancel   context.CancelFunc
	Answered bool
}
type chatSession struct {
	mu         sync.Mutex
	state      ChatState
	cancel     context.CancelFunc
	done       chan struct{}
	pending    map[string]*pendingApproval
	dismissed  map[string]bool
	saveMu     sync.Mutex
	reviewMu   sync.Mutex
	settingsMu sync.Mutex
}
type chatManager struct {
	mu              sync.Mutex
	items           map[string]*chatSession
	drafts          map[string]ReviewDraft
	dataDir         string
	closed          bool
	command         func(string, ...string) *exec.Cmd
	catalogs        map[string]*modelCache
	discoveryCtx    context.Context
	cancelDiscovery context.CancelFunc
}

func newChats(dir string) *chatManager {
	m := &chatManager{items: map[string]*chatSession{}, drafts: map[string]ReviewDraft{}, dataDir: dir, command: exec.Command}
	m.discoveryCtx, m.cancelDiscovery = context.WithCancel(context.Background())
	return m
}
func (m *chatManager) load() error {
	entries, err := os.ReadDir(m.dataDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		// Only app-created UUID filenames may identify records. Bound disk reads.
		if _, err := uuid.Parse(strings.TrimSuffix(entry.Name(), ".json")); err != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Size() > 32<<20 {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.dataDir, entry.Name()))
		if err != nil {
			return err
		}
		var state ChatState
		if json.Unmarshal(data, &state) != nil || state.ID+".json" != entry.Name() {
			continue
		}
		if state.Messages == nil {
			state.Messages = []ChatMessage{}
		}
		state.Approvals = []ChatApproval{}
		if busyChat(state.Status) {
			state.Status = "stopped"
			state.Error = "The previous run was interrupted. Send a message to continue."
		}
		state.Version++
		m.items[state.ID] = &chatSession{state: state, pending: map[string]*pendingApproval{}}
	}
	return nil
}
func busyChat(status string) bool {
	return status == "running" || status == "waiting" || status == "stopping"
}
func cloneState(state ChatState) ChatState {
	data, _ := json.Marshal(state)
	var copy ChatState
	_ = json.Unmarshal(data, &copy)
	return copy
}
func (c *chatSession) change() { c.state.Version++; c.state.UpdatedAt = time.Now() }
func (m *chatManager) get(id string) (*chatSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.items[id]
	if c == nil {
		return nil, errors.New("conversation not found")
	}
	return c, nil
}
func (m *chatManager) list() []ChatState {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := []ChatState{}
	for _, c := range m.items {
		c.mu.Lock()
		if c.state.Closed {
			c.mu.Unlock()
			continue
		}
		state := cloneState(c.state)
		state.Messages = []ChatMessage{}
		state.Approvals = []ChatApproval{}
		if state.Review != nil {
			state.Review.Snapshot.Diff = ""
			for i := range state.Review.Snapshot.Files {
				state.Review.Snapshot.Files[i].Patch = ""
			}
		}
		c.mu.Unlock()
		result = append(result, state)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.Before(result[j].UpdatedAt) })
	return result
}
func (m *chatManager) read(id string, version int64) (ChatUpdate, error) {
	c, err := m.get(id)
	if err != nil {
		return ChatUpdate{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	result := ChatUpdate{Version: c.state.Version}
	if version != c.state.Version {
		s := cloneState(c.state)
		result.State = &s
	}
	return result, nil
}
func (m *chatManager) save(c *chatSession) error {
	if m.dataDir == "" {
		return nil
	}
	c.saveMu.Lock()
	defer c.saveMu.Unlock()
	c.mu.Lock()
	data, err := json.Marshal(c.state)
	id := c.state.ID
	c.mu.Unlock()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(m.dataDir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(m.dataDir, ".chat-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(m.dataDir, id+".json"))
}
func (m *chatManager) saveOrWarn(c *chatSession) {
	if err := m.save(c); err != nil {
		c.mu.Lock()
		c.state.Error = "Could not save desktop conversation: " + err.Error()
		c.change()
		c.mu.Unlock()
	}
}
func (m *chatManager) create(project, tool, sessionID, key string, messages []ChatMessage) (ChatState, error) {
	if tool != "codex" && tool != "claude" {
		return ChatState{}, errors.New("choose Codex or Claude Code")
	}
	if project == "" {
		return ChatState{}, errors.New("choose a project folder")
	}
	dir, err := application.Directory(project)
	if err != nil {
		return ChatState{}, err
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ChatState{}, errors.New("the app is closing")
	}
	open := 0
	for _, c := range m.items {
		c.mu.Lock()
		match := key != "" && (c.state.SourceKey == key || (c.state.Tool == tool && c.state.SessionID == sessionID))
		if !c.state.Closed {
			open++
		}
		if match {
			c.state.Closed = false
			c.change()
		}
		state := cloneState(c.state)
		c.mu.Unlock()
		if match {
			m.mu.Unlock()
			m.saveOrWarn(c)
			return state, nil
		}
	}
	if open >= 100 {
		m.mu.Unlock()
		return ChatState{}, errors.New("close an older conversation before opening another (maximum 100)")
	}
	if messages == nil {
		messages = []ChatMessage{}
	}
	state := ChatState{ID: uuid.NewString(), Tool: tool, Project: dir, SessionID: sessionID, SourceKey: key, Title: "New conversation", Status: "idle", Messages: messages, Approvals: []ChatApproval{}, Version: 1, UpdatedAt: time.Now()}
	c := &chatSession{state: state, pending: map[string]*pendingApproval{}}
	m.items[state.ID] = c
	m.mu.Unlock()
	if err := m.save(c); err != nil {
		m.mu.Lock()
		delete(m.items, state.ID)
		m.mu.Unlock()
		return ChatState{}, err
	}
	return state, nil
}
func shorten(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	text = text[:limit]
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return text + "\n[Display shortened]"
}
func (c *chatSession) message(item ChatMessage, appendText bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if item.Role == "assistant" && item.Kind == "text" && item.Model == "" {
		item.Model = c.state.ActiveModel
	}
	for i := range c.state.Messages {
		if c.state.Messages[i].ID == item.ID {
			old := &c.state.Messages[i]
			if appendText {
				old.Text = shorten(old.Text+item.Text, 100000)
			} else {
				old.Text = shorten(item.Text, 100000)
			}
			if item.Model != "" {
				old.Model = item.Model
			}
			if item.Status != "" {
				old.Status = item.Status
			}
			if item.Name != "" {
				old.Name = item.Name
			}
			c.boundMessages()
			c.change()
			return
		}
	}
	item.Text = shorten(item.Text, 100000)
	c.state.Messages = append(c.state.Messages, item)
	c.boundMessages()
	c.change()
}
func (c *chatSession) boundMessages() {
	size := 0
	for _, msg := range c.state.Messages {
		size += len(msg.Text)
	}
	for len(c.state.Messages) > 500 || size > 2<<20 {
		size -= len(c.state.Messages[0].Text)
		c.state.Messages = c.state.Messages[1:]
		c.state.Shortened = true
	}
}
func (c *chatSession) sessionID(id string) {
	if id == "" {
		return
	}
	c.mu.Lock()
	c.state.SessionID = id
	c.change()
	c.mu.Unlock()
}
func (m *chatManager) send(id, text string) error {
	if strings.TrimSpace(text) == "" || len(text) > reviewLimit+8192 {
		return errors.New("enter a message of up to 200,000 bytes")
	}
	c, err := m.get(id)
	if err != nil {
		return err
	}
	c.settingsMu.Lock()
	defer c.settingsMu.Unlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("the app is closing")
	}
	running := 0
	for _, other := range m.items {
		other.mu.Lock()
		if busyChat(other.state.Status) {
			running++
		}
		other.mu.Unlock()
	}
	if running >= 8 {
		m.mu.Unlock()
		return errors.New("eight agents are already running; stop or finish one first")
	}
	c.mu.Lock()
	if c.state.Review == nil && len(text) > 200000 {
		c.mu.Unlock()
		m.mu.Unlock()
		return errors.New("message exceeds 200,000 bytes")
	}
	if busyChat(c.state.Status) {
		c.mu.Unlock()
		m.mu.Unlock()
		return errors.New("wait for this response or stop it before sending another message")
	}
	if c.state.Review != nil && len(c.state.Messages) > 0 {
		c.mu.Unlock()
		m.mu.Unlock()
		return errors.New("capture a fresh diff to run another review")
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.done = make(chan struct{})
	done := c.done
	c.state.Status = "running"
	c.state.Closed = false
	c.state.Error = ""
	c.state.ActiveModel = ""
	c.state.Approvals = []ChatApproval{}
	c.pending = map[string]*pendingApproval{}
	c.dismissed = map[string]bool{}
	if c.state.Title == "New conversation" {
		c.state.Title = shorten(strings.ReplaceAll(text, "\n", " "), 80)
	}
	state := cloneState(c.state)
	c.change()
	c.mu.Unlock()
	m.mu.Unlock()
	display := text
	if state.Review != nil {
		display = "Review this working-tree snapshot with " + state.Tool + ".\nSnapshot: " + state.Review.Snapshot.Hash[:12]
	}
	c.message(ChatMessage{ID: uuid.NewString(), Role: "user", Kind: "text", Text: display}, false)
	m.saveOrWarn(c)
	go func() {
		defer close(done)
		defer cancel()
		var result string
		execution, cleanup, err := reviewDirectory(state)
		if err == nil {
			defer cleanup()
			state = execution
		}
		if err == nil {
			if state.Tool == "codex" {
				result, err = m.runCodex(ctx, c, state, text)
			} else {
				result, err = m.runClaude(ctx, c, state, text)
			}
		}
		interrupted := ctx.Err() != nil
		cancel() // Retire approval handlers before allowing a new turn.
		c.mu.Lock()
		c.cancel = nil
		c.pending = map[string]*pendingApproval{}
		c.state.Approvals = []ChatApproval{}
		switch {
		case interrupted:
			c.state.Status = "stopped"
			c.state.Error = "Response stopped. Send a message to continue."
		case err != nil:
			c.state.Status = "error"
			c.state.Error = err.Error()
		default:
			c.state.Status = "idle"
		}
		if err == nil && !interrupted && c.state.Review != nil {
			if err := completeReview(c.state.Review, result); err != nil {
				c.state.Error = err.Error()
				c.state.Status = "error"
			}
		}
		for i := range c.state.Messages {
			if c.state.Messages[i].Status == "running" {
				c.state.Messages[i].Status = "finished"
			}
		}
		c.change()
		c.mu.Unlock()
		m.saveOrWarn(c)
	}()
	return nil
}
func (c *chatSession) ask(ctx context.Context, view ChatApproval) (approvalReply, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	pending := &pendingApproval{View: view, Reply: make(chan approvalReply, 1), Cancel: cancel}
	c.mu.Lock()
	if ctx.Err() != nil {
		c.mu.Unlock()
		return approvalReply{}, ctx.Err()
	}
	if !busyChat(c.state.Status) || c.state.Status == "stopping" {
		c.mu.Unlock()
		return approvalReply{}, errors.New("turn is no longer accepting approvals")
	}
	if c.dismissed[view.ID] {
		c.mu.Unlock()
		return approvalReply{}, errors.New("approval request already resolved")
	}
	if c.pending[view.ID] != nil {
		c.mu.Unlock()
		return approvalReply{}, errors.New("duplicate approval request")
	}
	c.pending[view.ID] = pending
	c.state.Approvals = append(c.state.Approvals, view)
	c.state.Status = "waiting"
	c.change()
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		if c.pending[view.ID] != pending {
			c.mu.Unlock()
			return
		}
		delete(c.pending, view.ID)
		for i, a := range c.state.Approvals {
			if a.ID == view.ID {
				c.state.Approvals = append(c.state.Approvals[:i], c.state.Approvals[i+1:]...)
				break
			}
		}
		if len(c.pending) == 0 && c.state.Status == "waiting" {
			c.state.Status = "running"
		}
		c.change()
		c.mu.Unlock()
	}()
	select {
	case reply := <-pending.Reply:
		return reply, nil
	case <-ctx.Done():
		return approvalReply{}, ctx.Err()
	}
}
func (m *chatManager) approve(id, request string, allow bool, answers map[string]string) error {
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
	if allow {
		for _, q := range p.View.Questions {
			if strings.TrimSpace(answers[q.ID]) == "" {
				return fmt.Errorf("answer %s before continuing", q.Text)
			}
			if len(answers[q.ID]) > 10000 {
				return errors.New("answer is too long")
			}
		}
	}
	select {
	case p.Reply <- approvalReply{Allow: allow, Answers: answers}:
		p.Answered = true
		return nil
	default:
		return errors.New("request already answered")
	}
}
func (m *chatManager) stop(id string) error {
	c, err := m.get(id)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if c.cancel != nil {
		c.state.Status = "stopping"
		c.cancel()
		c.change()
	}
	c.mu.Unlock()
	return nil
}

// Closing hides the tab, preserving review links and saved metadata.
func (m *chatManager) remove(id string) error {
	c, err := m.get(id)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if busyChat(c.state.Status) {
		c.mu.Unlock()
		return errors.New("stop this conversation before closing it")
	}
	c.state.Closed = true
	c.change()
	c.mu.Unlock()
	return m.save(c)
}
func (m *chatManager) close() {
	m.mu.Lock()
	m.closed = true
	m.cancelDiscovery()
	items := []*chatSession{}
	for _, c := range m.items {
		items = append(items, c)
	}
	m.mu.Unlock()
	for _, c := range items {
		c.mu.Lock()
		if c.cancel != nil {
			c.cancel()
		}
		done := c.done
		c.mu.Unlock()
		if done != nil {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
			}
		}
	}
}
