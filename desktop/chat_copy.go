package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/caanmert/ai-session-tool/internal/launch"
	"github.com/google/uuid"
)

// A conversation can work in its own copy of the project: a Git worktree on a
// new branch from the project's HEAD, kept in the app's data folder. The
// conversation's project is the copy, so the agent, changes, checkpoints, run
// panel and instructions all work there. Its changes reach the project only
// when the user brings them back, and the copy is removed only on request.

// WorkCopy describes the copy a conversation works in.
type WorkCopy struct {
	// Project is the original project's repository root.
	Project string `json:"project"`
	Base    string `json:"base"`
	Branch  string `json:"branch"`
	// Worktree is the copy's folder; the conversation's project.
	Worktree string `json:"worktree"`
	// Brought is a fingerprint of the changes last brought back, so removing
	// the copy can tell whether anything since would be lost.
	Brought string `json:"brought,omitempty"`
}

// patchFrom is a folder's changes against a commit, including commits on top
// of it and new files, as a binary-safe patch. A private index is used so the
// folder's own index is not touched.
func patchFrom(ctx context.Context, dir, base string) (patch string, files []string, err error) {
	temp, err := os.MkdirTemp("", "ais-patch-")
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(temp)
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(temp, "index")}
	if _, err := gitRunWith(ctx, dir, env, "read-tree", base); err != nil {
		return "", nil, err
	}
	if _, err := gitRunWith(ctx, dir, env, "add", "--all"); err != nil {
		return "", nil, err
	}
	names, err := gitRunWith(ctx, dir, env, "diff", "--cached", "--no-renames", "--name-only", base)
	if err != nil {
		return "", nil, err
	}
	if names == "" {
		return "", nil, nil
	}
	patch, err = gitRunWith(ctx, dir, env, "diff", "--cached", "--no-renames", "--binary", "--no-ext-diff", "--no-textconv", base)
	return patch + "\n", strings.Split(names, "\n"), err
}

// applyPatch applies a patch to a project as uncommitted changes: all files or
// none, and never into files the user has uncommitted changes in.
func applyPatch(ctx context.Context, project, patch string, files []string, who string) error {
	if own := summarizeChanges(ctx, project); own.Available {
		mine := map[string]bool{}
		for _, file := range own.Files {
			mine[file.Path] = true
		}
		overlap := []string{}
		for _, file := range files {
			if mine[file] {
				overlap = append(overlap, file)
			}
		}
		if len(overlap) > 0 {
			sort.Strings(overlap)
			return fmt.Errorf("your project has its own uncommitted changes in %s. Commit or set them aside first; nothing was changed", strings.Join(overlap, ", "))
		}
	}
	temp, err := os.MkdirTemp("", "ais-patch-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	patchFile := filepath.Join(temp, "changes.patch")
	if err := os.WriteFile(patchFile, []byte(patch), 0o600); err != nil {
		return err
	}
	if _, err := gitRun(ctx, project, "apply", "--check", "--binary", patchFile); err != nil {
		return fmt.Errorf("%s do not apply to your project as it is now, so nothing was changed: %w", who, err)
	}
	if _, err := gitRun(ctx, project, "apply", "--binary", patchFile); err != nil {
		return fmt.Errorf("%s could not be applied: %w", who, err)
	}
	return nil
}

func fingerprint(patch string) string {
	if patch == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(patch))
	return hex.EncodeToString(sum[:12])
}

// createCopy starts a conversation in a new worktree of the project.
func (m *chatManager) createCopy(ctx context.Context, project, tool string) (ChatState, error) {
	if m.dataDir == "" {
		return ChatState{}, errors.New("copies need the app's data folder")
	}
	root, err := repoRoot(ctx, project)
	if err != nil {
		return ChatState{}, errors.New("a separate copy needs a Git repository")
	}
	base, err := gitRun(ctx, root, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return ChatState{}, errors.New("a separate copy starts from the last commit; make a first commit")
	}
	short := strings.SplitN(uuid.NewString(), "-", 2)[0]
	dir := filepath.Join(m.dataDir, "copies", short)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ChatState{}, err
	}
	copy := WorkCopy{Project: root, Base: base, Branch: "ais/" + short, Worktree: filepath.Join(dir, filepath.Base(root))}
	if _, err := gitRun(ctx, root, "worktree", "add", "-b", copy.Branch, copy.Worktree, base); err != nil {
		_ = os.RemoveAll(dir)
		return ChatState{}, fmt.Errorf("the copy could not be created: %w", err)
	}
	undo := func() {
		_, _ = gitRun(context.Background(), root, "worktree", "remove", "--force", copy.Worktree)
		_, _ = gitRun(context.Background(), root, "branch", "--delete", "--force", copy.Branch)
		_ = os.RemoveAll(dir)
	}
	state, err := m.create(copy.Worktree, tool, "", "", nil)
	if err != nil {
		undo()
		return ChatState{}, err
	}
	c, err := m.get(state.ID)
	if err != nil {
		undo()
		return ChatState{}, err
	}
	c.mu.Lock()
	// Keep the path the conversation uses, with symbolic links resolved.
	copy.Worktree = c.state.Project
	c.state.Copy = &copy
	c.change()
	state = cloneState(c.state)
	c.mu.Unlock()
	m.saveOrWarn(c)
	return state, nil
}

func (m *chatManager) copyOf(id string) (*chatSession, WorkCopy, error) {
	c, err := m.get(id)
	if err != nil {
		return nil, WorkCopy{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Copy == nil {
		return nil, WorkCopy{}, errors.New("this conversation works in the project itself")
	}
	if busyChat(c.state.Status) {
		return nil, WorkCopy{}, errors.New("wait for the agent to finish, or stop it, first")
	}
	return c, *c.state.Copy, nil
}

// copyChanges is what bringing the copy back would change: its work against
// where it started, committed or not.
func (m *chatManager) copyChanges(ctx context.Context, id string) (ReviewSnapshot, error) {
	c, err := m.get(id)
	if err != nil {
		return ReviewSnapshot{}, err
	}
	c.mu.Lock()
	copy := c.state.Copy
	c.mu.Unlock()
	if copy == nil {
		return ReviewSnapshot{}, errors.New("this conversation works in the project itself")
	}
	return captureView(ctx, copy.Worktree, copy.Base)
}

// bringBack applies the copy's changes to the project as uncommitted changes.
func (m *chatManager) bringBack(ctx context.Context, id string) ([]string, error) {
	c, copy, err := m.copyOf(id)
	if err != nil {
		return nil, err
	}
	patch, files, err := patchFrom(ctx, copy.Worktree, copy.Base)
	if err != nil {
		return nil, fmt.Errorf("the copy's changes could not be read: %w", err)
	}
	if len(files) == 0 {
		return nil, errors.New("the copy has no changes to bring back")
	}
	if err := applyPatch(ctx, copy.Project, patch, files, "The copy's changes"); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.state.Copy != nil {
		c.state.Copy.Brought = fingerprint(patch)
	}
	c.mu.Unlock()
	c.message(ChatMessage{ID: fmt.Sprintf("bring/%d", time.Now().UnixNano()), Role: "system", Kind: "text", Text: fmt.Sprintf("Brought %d changed files back into %s as uncommitted changes. Nothing was committed.", len(files), copy.Project)}, false)
	m.saveOrWarn(c)
	return files, nil
}

// removeCopy deletes the copy's worktree and branch and closes the
// conversation. Changes not brought back are refused unless discard is set.
func (m *chatManager) removeCopy(ctx context.Context, id string, discard bool) error {
	c, copy, err := m.copyOf(id)
	if err != nil {
		return err
	}
	if !discard {
		if _, err := os.Stat(copy.Worktree); err == nil {
			patch, files, err := patchFrom(ctx, copy.Worktree, copy.Base)
			if err != nil {
				return fmt.Errorf("the copy's changes could not be checked, so it was kept: %w", err)
			}
			if len(files) > 0 && fingerprint(patch) != copy.Brought {
				return fmt.Errorf("the copy has changes that were not brought back (%d files); bring them back or choose to discard them", len(files))
			}
		}
	}
	if _, err := os.Stat(copy.Worktree); err == nil {
		if _, err := gitRun(ctx, copy.Project, "worktree", "remove", "--force", copy.Worktree); err != nil {
			return fmt.Errorf("the copy could not be removed: %w", err)
		}
	}
	_, _ = gitRun(ctx, copy.Project, "worktree", "prune")
	_, _ = gitRun(ctx, copy.Project, "branch", "--delete", "--force", copy.Branch)
	_ = os.Remove(filepath.Dir(copy.Worktree))
	c.mu.Lock()
	c.state.Closed = true
	c.change()
	c.mu.Unlock()
	return m.save(c)
}

// NewChatInCopy starts a conversation in a new copy of the project: a Git
// worktree on its own branch from the last commit.
func (a *App) NewChatInCopy(project, tool string) (ChatState, error) {
	if err := a.chatReady(); err != nil {
		return ChatState{}, err
	}
	cmd, err := a.service.NewSession(tool, project)
	if err != nil {
		return ChatState{}, err
	}
	if err = launch.Check(cmd); err != nil {
		return ChatState{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, time.Minute)
	defer cancel()
	return a.chats.createCopy(ctx, project, tool)
}

// CopyChanges shows what bringing a copy back would change. Changes nothing.
func (a *App) CopyChanges(chatID string) (ReviewSnapshot, error) {
	if err := a.chatReady(); err != nil {
		return ReviewSnapshot{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, time.Minute)
	defer cancel()
	return a.chats.copyChanges(ctx, chatID)
}

// BringBackCopy applies a copy's changes to the project, after the user
// confirmed them. Nothing is committed.
func (a *App) BringBackCopy(chatID string) ([]string, error) {
	if err := a.chatReady(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	return a.chats.bringBack(ctx, chatID)
}

// RemoveCopy deletes a copy and closes its conversation, after the user
// confirmed it.
func (a *App) RemoveCopy(chatID string, discard bool) error {
	if err := a.chatReady(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	return a.chats.removeCopy(ctx, chatID, discard)
}
