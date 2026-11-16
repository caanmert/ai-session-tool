package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// A comparison gives the same task to both agents. Each works in its own Git
// worktree and branch, started from the project's committed HEAD, so neither
// can touch the other's files or the user's working tree. Nothing reaches the
// project until the user copies one side into it.

// ComparisonSide is one agent's attempt.
type ComparisonSide struct {
	Tool     string `json:"tool"`
	ChatID   string `json:"chatId"`
	Worktree string `json:"worktree"`
	Branch   string `json:"branch"`
}

// Comparison is the record of one comparison.
type Comparison struct {
	ID        string           `json:"id"`
	Project   string           `json:"project"`
	Base      string           `json:"base"`
	Prompt    string           `json:"prompt"`
	CreatedAt time.Time        `json:"createdAt"`
	Sides     []ComparisonSide `json:"sides"`
}

// ComparisonStart is what starting a comparison returns: the record and the
// two conversations, which are already working.
type ComparisonStart struct {
	Comparison Comparison  `json:"comparison"`
	Chats      []ChatState `json:"chats"`
	// Problem is set when a side could not be started; its conversation is
	// open so the task can be sent again.
	Problem string `json:"problem,omitempty"`
}

// ComparisonSideView is one side as the compare view shows it.
type ComparisonSideView struct {
	ComparisonSide
	Status  string        `json:"status"`
	Changes ChangeSummary `json:"changes"`
}

// ComparisonView is the compare view.
type ComparisonView struct {
	ID      string               `json:"id"`
	Project string               `json:"project"`
	Base    string               `json:"base"`
	Prompt  string               `json:"prompt"`
	Sides   []ComparisonSideView `json:"sides"`
}

func (m *chatManager) comparisonDir() (string, error) {
	if m.dataDir == "" {
		return "", errors.New("comparisons need the ais data folder")
	}
	return filepath.Join(m.dataDir, "comparisons"), nil
}

func (m *chatManager) saveComparison(record Comparison) error {
	dir, err := m.comparisonDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	temp := filepath.Join(dir, record.ID+".json.tmp")
	if err := os.WriteFile(temp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(temp, filepath.Join(dir, record.ID+".json"))
}

func (m *chatManager) loadComparison(id string) (Comparison, error) {
	dir, err := m.comparisonDir()
	if err != nil {
		return Comparison{}, err
	}
	if !strings.HasPrefix(id, "cmp-") || strings.ContainsAny(id, `/\.`) {
		return Comparison{}, errors.New("unknown comparison")
	}
	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return Comparison{}, errors.New("this comparison no longer exists")
	}
	var record Comparison
	if err := json.Unmarshal(data, &record); err != nil || record.ID != id {
		return Comparison{}, errors.New("this comparison's record is damaged")
	}
	return record, nil
}

func (record Comparison) side(tool string) (ComparisonSide, error) {
	for _, side := range record.Sides {
		if side.Tool == tool {
			return side, nil
		}
	}
	return ComparisonSide{}, fmt.Errorf("this comparison has no %s side", tool)
}

func (m *chatManager) startComparison(ctx context.Context, project, text string) (ComparisonStart, error) {
	if strings.TrimSpace(text) == "" {
		return ComparisonStart{}, errors.New("describe the task to compare")
	}
	if len(text) > 200000 {
		return ComparisonStart{}, errors.New("the task is longer than 200,000 bytes")
	}
	if _, err := m.comparisonDir(); err != nil {
		return ComparisonStart{}, err
	}
	root, err := repoRoot(ctx, project)
	if err != nil {
		return ComparisonStart{}, err
	}
	base, err := gitRun(ctx, root, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return ComparisonStart{}, err
	}
	id, err := newComparisonID()
	if err != nil {
		return ComparisonStart{}, err
	}
	record := Comparison{ID: id, Project: root, Base: base, Prompt: text, CreatedAt: time.Now().UTC()}
	dir := filepath.Join(m.dataDir, "worktrees", record.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ComparisonStart{}, err
	}
	// A failed setup leaves nothing behind.
	undo := func() {
		for _, side := range record.Sides {
			_, _ = gitRun(context.Background(), root, "worktree", "remove", "--force", side.Worktree)
			_, _ = gitRun(context.Background(), root, "branch", "--delete", "--force", side.Branch)
		}
		_ = os.RemoveAll(dir)
	}
	name := filepath.Base(root)
	for _, tool := range []string{"claude", "codex"} {
		side := ComparisonSide{Tool: tool, Branch: "ais-compare/" + strings.TrimPrefix(record.ID, "cmp-") + "/" + tool, Worktree: filepath.Join(dir, name+"-"+tool)}
		if _, err := gitRun(ctx, root, "worktree", "add", "-b", side.Branch, side.Worktree, base); err != nil {
			undo()
			return ComparisonStart{}, fmt.Errorf("the %s worktree could not be created: %w", toolName(tool), err)
		}
		record.Sides = append(record.Sides, side)
	}
	chats := []ChatState{}
	for i := range record.Sides {
		state, err := m.create(record.Sides[i].Worktree, record.Sides[i].Tool, "", "", nil)
		if err != nil {
			for _, chat := range chats {
				_ = m.remove(chat.ID)
			}
			undo()
			return ComparisonStart{}, err
		}
		record.Sides[i].ChatID = state.ID
		// Keep the path the conversation uses, with symbolic links resolved.
		record.Sides[i].Worktree = state.Project
		chats = append(chats, state)
	}
	if err := m.saveComparison(record); err != nil {
		for _, chat := range chats {
			_ = m.remove(chat.ID)
		}
		undo()
		return ComparisonStart{}, err
	}
	for i, side := range record.Sides {
		c, _ := m.get(side.ChatID)
		c.mu.Lock()
		c.state.Comparison = record.ID
		c.change()
		chats[i] = cloneState(c.state)
		c.mu.Unlock()
		m.saveOrWarn(c)
	}
	result := ComparisonStart{Comparison: record, Chats: chats}
	problems := []string{}
	for i, side := range record.Sides {
		if err := m.send(side.ChatID, text); err != nil {
			problems = append(problems, fmt.Sprintf("%s could not start: %s", toolName(side.Tool), err))
			continue
		}
		state, _ := m.read(side.ChatID, -1)
		chats[i] = *state.State
	}
	if len(problems) > 0 {
		result.Problem = strings.Join(problems, ". ") + ". Its conversation is open, so you can send the task again there."
	}
	return result, nil
}

// newComparisonID makes a random comparison ID; replaced in tests.
var newComparisonID = func() (string, error) {
	var nonce [6]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return "cmp-" + hex.EncodeToString(nonce[:]), nil
}

func toolName(tool string) string {
	if tool == "claude" {
		return "Claude Code"
	}
	return "Codex"
}

func (m *chatManager) comparisonView(ctx context.Context, id string) (ComparisonView, error) {
	record, err := m.loadComparison(id)
	if err != nil {
		return ComparisonView{}, err
	}
	view := ComparisonView{ID: record.ID, Project: record.Project, Base: record.Base, Prompt: record.Prompt}
	for _, side := range record.Sides {
		item := ComparisonSideView{ComparisonSide: side, Status: "closed"}
		if c, err := m.get(side.ChatID); err == nil {
			c.mu.Lock()
			item.Status = c.state.Status
			c.mu.Unlock()
		}
		if _, err := os.Stat(side.Worktree); err != nil {
			item.Changes = ChangeSummary{Reason: "This side's worktree is missing.", Files: []ChangedFile{}}
		} else {
			item.Changes = summarizeChangesFrom(ctx, side.Worktree, record.Base)
		}
		view.Sides = append(view.Sides, item)
	}
	return view, nil
}

func (m *chatManager) comparisonDiff(ctx context.Context, id, tool string) (ReviewSnapshot, error) {
	record, err := m.loadComparison(id)
	if err != nil {
		return ReviewSnapshot{}, err
	}
	side, err := record.side(tool)
	if err != nil {
		return ReviewSnapshot{}, err
	}
	return captureReviewFrom(ctx, side.Worktree, record.Base)
}

// sidePatch is one side's changes against the comparison's base commit.
func sidePatch(ctx context.Context, record Comparison, side ComparisonSide) (string, []string, error) {
	return patchFrom(ctx, side.Worktree, record.Base)
}

func (m *chatManager) useComparisonSide(ctx context.Context, id, tool string) ([]string, error) {
	record, err := m.loadComparison(id)
	if err != nil {
		return nil, err
	}
	side, err := record.side(tool)
	if err != nil {
		return nil, err
	}
	if c, err := m.get(side.ChatID); err == nil {
		c.mu.Lock()
		busy := busyChat(c.state.Status)
		c.mu.Unlock()
		if busy {
			return nil, fmt.Errorf("wait for %s to finish before using its version", toolName(tool))
		}
	}
	patch, files, err := sidePatch(ctx, record, side)
	if err != nil {
		return nil, fmt.Errorf("%s's changes could not be read: %w", toolName(tool), err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s has not changed anything", toolName(tool))
	}
	// Never mix the agent's changes into files the user has changed too, and
	// change all files or none.
	if err := applyPatch(ctx, record.Project, patch, files, toolName(tool)+"'s changes"); err != nil {
		return nil, err
	}
	if c, err := m.get(side.ChatID); err == nil {
		c.message(ChatMessage{ID: fmt.Sprintf("use/%d", time.Now().UnixNano()), Role: "system", Kind: "text", Text: fmt.Sprintf("This version was copied into %s (%d files). Nothing was committed.", record.Project, len(files))}, false)
		m.saveOrWarn(c)
	}
	return files, nil
}

func (m *chatManager) removeComparison(ctx context.Context, id string) error {
	record, err := m.loadComparison(id)
	if err != nil {
		return err
	}
	for _, side := range record.Sides {
		if c, err := m.get(side.ChatID); err == nil {
			c.mu.Lock()
			busy := busyChat(c.state.Status)
			c.mu.Unlock()
			if busy {
				return fmt.Errorf("stop %s before removing the comparison", toolName(side.Tool))
			}
		}
	}
	problems := []string{}
	for _, side := range record.Sides {
		if _, err := os.Stat(side.Worktree); err == nil {
			if _, err := gitRun(ctx, record.Project, "worktree", "remove", "--force", side.Worktree); err != nil {
				problems = append(problems, err.Error())
				continue
			}
		}
		_, _ = gitRun(ctx, record.Project, "worktree", "prune")
		_, _ = gitRun(ctx, record.Project, "branch", "--delete", "--force", side.Branch)
		if c, err := m.get(side.ChatID); err == nil {
			_ = m.remove(c.state.ID)
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("the comparison was not fully removed: %s", strings.Join(problems, "; "))
	}
	_ = os.Remove(filepath.Join(m.dataDir, "worktrees", record.ID))
	dir, _ := m.comparisonDir()
	return os.Remove(filepath.Join(dir, record.ID+".json"))
}

// StartComparison gives the same task to both agents, each in its own
// worktree and branch from the project's committed HEAD.
func (a *App) StartComparison(project, text string) (ComparisonStart, error) {
	if err := a.chatReady(); err != nil {
		return ComparisonStart{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	return a.chats.startComparison(ctx, project, text)
}

// ComparisonOverview shows both sides of a comparison and what each changed.
func (a *App) ComparisonOverview(id string) (ComparisonView, error) {
	if err := a.chatReady(); err != nil {
		return ComparisonView{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return a.chats.comparisonView(ctx, id)
}

// ComparisonDiff is one side's diff against the comparison's starting commit.
func (a *App) ComparisonDiff(id, tool string) (ReviewSnapshot, error) {
	if err := a.chatReady(); err != nil {
		return ReviewSnapshot{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.chats.comparisonDiff(ctx, id, tool)
}

// UseComparisonSide copies one side's changes into the project's working
// tree. Nothing is committed, and the other side stays in its worktree.
func (a *App) UseComparisonSide(id, tool string) ([]string, error) {
	if err := a.chatReady(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, time.Minute)
	defer cancel()
	return a.chats.useComparisonSide(ctx, id, tool)
}

// RemoveComparison deletes both worktrees and branches. Changes that were not
// copied or committed elsewhere are lost; the app asks first.
func (a *App) RemoveComparison(id string) error {
	if err := a.chatReady(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(a.ctx, time.Minute)
	defer cancel()
	return a.chats.removeComparison(ctx, id)
}
