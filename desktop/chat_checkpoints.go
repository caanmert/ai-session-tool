package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Checkpoints let a turn be undone. Before each turn in a Git project, the
// working tree (tracked and untracked files, not ignored ones) is written as
// a commit object built from a private index, kept under refs/ais/. Nothing
// is committed on a branch, and the user's index, HEAD and stash list are
// never touched. Restoring writes back only the files that differ.

// Checkpoint is the working tree as it was at a point in a conversation.
type Checkpoint struct {
	ID string `json:"id"`
	// MessageID is the message it belongs to: the message that started a
	// turn, or the note left by a restore.
	MessageID string `json:"messageId"`
	Commit    string `json:"commit"`
	At        string `json:"at"`
	// Kind is "turn" for before a turn, "restore" for before a restore.
	Kind string `json:"kind"`
}

// RestoreChange is one file a restore changes back.
type RestoreChange struct {
	Path string `json:"path"`
	// Change is "restore" (put back the earlier content) or "remove" (a file
	// that did not exist then).
	Change string `json:"change"`
}

const checkpointLimit = 50

func checkpointRef(chatID, id string) string {
	return "refs/ais/checkpoints/" + chatID + "/" + id
}

// snapshot writes the working tree of root as a commit object and returns it.
func snapshot(ctx context.Context, root, message string) (string, error) {
	temp, err := os.MkdirTemp("", "ais-checkpoint-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temp)
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(temp, "index")}
	head, headErr := gitRun(ctx, root, "rev-parse", "--verify", "--quiet", "HEAD")
	if headErr == nil && head != "" {
		_, err = gitRunWith(ctx, root, env, "read-tree", head)
	} else {
		_, err = gitRunWith(ctx, root, env, "read-tree", "--empty")
	}
	if err != nil {
		return "", err
	}
	if _, err := gitRunWith(ctx, root, env, "add", "--all"); err != nil {
		return "", err
	}
	tree, err := gitRunWith(ctx, root, env, "write-tree")
	if err != nil {
		return "", err
	}
	args := []string{"-c", "user.name=ais", "-c", "user.email=ais@localhost", "commit-tree", tree, "-m", message}
	if headErr == nil && head != "" {
		args = append(args, "-p", head)
	}
	return gitRun(ctx, root, args...)
}

// addCheckpoint records the working tree for a message and keeps the
// newest checkpoints of the conversation.
func (m *chatManager) addCheckpoint(ctx context.Context, c *chatSession, project, messageID, kind string) (Checkpoint, error) {
	root, err := repoRoot(ctx, project)
	if err != nil {
		return Checkpoint{}, err
	}
	c.mu.Lock()
	chatID := c.state.ID
	c.mu.Unlock()
	commit, err := snapshot(ctx, root, "ais checkpoint for "+chatID)
	if err != nil {
		return Checkpoint{}, err
	}
	point := Checkpoint{ID: strings.ReplaceAll(uuid.NewString(), "-", "")[:16], MessageID: messageID, Commit: commit, At: chatNow().UTC().Format(time.RFC3339), Kind: kind}
	if _, err := gitRun(ctx, root, "update-ref", checkpointRef(chatID, point.ID), commit); err != nil {
		return Checkpoint{}, err
	}
	c.mu.Lock()
	c.state.Checkpoints = append(c.state.Checkpoints, point)
	dropped := []Checkpoint{}
	if extra := len(c.state.Checkpoints) - checkpointLimit; extra > 0 {
		dropped = append(dropped, c.state.Checkpoints[:extra]...)
		c.state.Checkpoints = append([]Checkpoint{}, c.state.Checkpoints[extra:]...)
	}
	c.change()
	c.mu.Unlock()
	for _, old := range dropped {
		_, _ = gitRun(ctx, root, "update-ref", "-d", checkpointRef(chatID, old.ID))
	}
	return point, nil
}

func (c *chatSession) checkpoint(id string) (Checkpoint, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, point := range c.state.Checkpoints {
		if point.ID == id {
			return point, true
		}
	}
	return Checkpoint{}, false
}

// restoreChanges lists what restoring to commit would change in root now.
func restoreChanges(ctx context.Context, root, commit string) ([]RestoreChange, error) {
	now, err := snapshot(ctx, root, "ais restore preview")
	if err != nil {
		return nil, err
	}
	out, err := gitRun(ctx, root, "diff-tree", "-r", "-z", "--no-renames", "--name-status", commit, now)
	if err != nil {
		return nil, err
	}
	changes := []RestoreChange{}
	parts := strings.Split(strings.TrimRight(out, "\x00"), "\x00")
	for i := 0; i+1 < len(parts); i += 2 {
		change := "restore"
		if parts[i] == "A" {
			change = "remove"
		}
		changes = append(changes, RestoreChange{Path: parts[i+1], Change: change})
	}
	return changes, nil
}

func (m *chatManager) checkpointTarget(ctx context.Context, chatID, pointID string) (*chatSession, Checkpoint, string, error) {
	c, err := m.get(chatID)
	if err != nil {
		return nil, Checkpoint{}, "", err
	}
	point, ok := c.checkpoint(pointID)
	if !ok {
		return nil, Checkpoint{}, "", errors.New("this checkpoint is no longer kept")
	}
	c.mu.Lock()
	project, review := c.state.Project, c.state.Review
	c.mu.Unlock()
	if review != nil {
		return nil, Checkpoint{}, "", errors.New("a review does not change files")
	}
	root, err := repoRoot(ctx, project)
	if err != nil {
		return nil, Checkpoint{}, "", err
	}
	return c, point, root, nil
}

// restorePlan lists the files a restore would change, without changing any.
func (m *chatManager) restorePlan(ctx context.Context, chatID, pointID string) ([]RestoreChange, error) {
	_, point, root, err := m.checkpointTarget(ctx, chatID, pointID)
	if err != nil {
		return nil, err
	}
	return restoreChanges(ctx, root, point.Commit)
}

// restore puts the working tree back as it was at a checkpoint. The state
// just before is kept as a checkpoint of its own, so the restore can be
// undone; the user's index is not changed.
func (m *chatManager) restore(ctx context.Context, chatID, pointID string) ([]RestoreChange, error) {
	c, point, root, err := m.checkpointTarget(ctx, chatID, pointID)
	if err != nil {
		return nil, err
	}
	c.settingsMu.Lock()
	defer c.settingsMu.Unlock()
	c.mu.Lock()
	busy := busyChat(c.state.Status)
	project := c.state.Project
	c.mu.Unlock()
	if busy {
		return nil, errors.New("wait for the response to finish or stop it before restoring")
	}
	changes, err := restoreChanges(ctx, root, point.Commit)
	if err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return changes, nil
	}
	note := uuid.NewString()
	if _, err := m.addCheckpoint(ctx, c, project, note, "restore"); err != nil {
		return nil, fmt.Errorf("the current files could not be kept first, so nothing was restored: %w", err)
	}
	if err := writeBack(ctx, root, point.Commit, changes); err != nil {
		return nil, err
	}
	text := fmt.Sprintf("Restored %d %s to how they were before that turn. The files as they were just before restoring are kept: undo this restore to get them back.", len(changes), map[bool]string{true: "file", false: "files"}[len(changes) == 1])
	c.message(ChatMessage{ID: note, Role: "system", Kind: "text", Text: text}, false)
	m.saveOrWarn(c)
	return changes, nil
}

// writeBack makes the listed paths in root match commit: earlier content is
// written back, and files that did not exist then are removed.
func writeBack(ctx context.Context, root, commit string, changes []RestoreChange) error {
	temp, err := os.MkdirTemp("", "ais-restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(temp, "index")}
	restore := []string{}
	for _, change := range changes {
		if change.Change == "remove" {
			path, err := safeJoin(root, change.Path)
			if err != nil {
				return err
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}
		restore = append(restore, change.Path)
	}
	if len(restore) > 0 {
		if _, err := gitRunWith(ctx, root, env, "read-tree", commit); err != nil {
			return err
		}
		args := append([]string{"checkout-index", "--force", "--"}, restore...)
		if _, err := gitRunWith(ctx, root, env, args...); err != nil {
			return err
		}
	}
	return nil
}

// safeJoin joins a repository path to root, refusing anything that leaves it.
func safeJoin(root, rel string) (string, error) {
	path := filepath.Join(root, filepath.FromSlash(rel))
	inside, err := filepath.Rel(root, path)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("%q is outside the project", rel)
	}
	return path, nil
}

// discardFile undoes the uncommitted changes to one file: it goes back to
// the last commit, or is removed if it is new. The state before is kept as a
// checkpoint. The user's index is not changed.
func (m *chatManager) discardFile(ctx context.Context, chatID, path string) error {
	c, err := m.get(chatID)
	if err != nil {
		return err
	}
	c.settingsMu.Lock()
	defer c.settingsMu.Unlock()
	c.mu.Lock()
	busy, project, review := busyChat(c.state.Status), c.state.Project, c.state.Review
	c.mu.Unlock()
	switch {
	case review != nil:
		return errors.New("a review does not change files")
	case busy:
		return errors.New("wait for the response to finish or stop it before discarding changes")
	}
	root, err := repoRoot(ctx, project)
	if err != nil {
		return err
	}
	target, err := safeJoin(root, path)
	if err != nil {
		return err
	}
	rel := filepath.ToSlash(strings.TrimPrefix(target, root+string(filepath.Separator)))
	head, headErr := gitRun(ctx, root, "rev-parse", "--verify", "--quiet", "HEAD")
	inHead := false
	if headErr == nil && head != "" {
		_, err := gitRun(ctx, root, "cat-file", "-e", head+":"+rel)
		inHead = err == nil
	}
	note := uuid.NewString()
	if _, err := m.addCheckpoint(ctx, c, project, note, "restore"); err != nil {
		return fmt.Errorf("the current file could not be kept first, so nothing was discarded: %w", err)
	}
	change := RestoreChange{Path: rel, Change: "remove"}
	if inHead {
		change.Change = "restore"
	}
	if err := writeBack(ctx, root, head, []RestoreChange{change}); err != nil {
		return err
	}
	c.message(ChatMessage{ID: note, Role: "system", Kind: "text", Text: "Discarded the uncommitted changes to " + rel + ". Undo this to get them back."}, false)
	m.saveOrWarn(c)
	return nil
}

// RestorePlan lists the files restoring a checkpoint would change.
func (a *App) RestorePlan(chatID, checkpointID string) ([]RestoreChange, error) {
	if err := a.chatReady(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 60*time.Second)
	defer cancel()
	return a.chats.restorePlan(ctx, chatID, checkpointID)
}

// RestoreCheckpoint puts the files back as they were at a checkpoint, after
// the user confirmed the list from RestorePlan.
func (a *App) RestoreCheckpoint(chatID, checkpointID string) ([]RestoreChange, error) {
	if err := a.chatReady(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	return a.chats.restore(ctx, chatID, checkpointID)
}

// DiscardFile undoes the uncommitted changes to one file, after the user
// confirmed it.
func (a *App) DiscardFile(chatID, path string) error {
	if err := a.chatReady(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(a.ctx, time.Minute)
	defer cancel()
	return a.chats.discardFile(ctx, chatID, path)
}
