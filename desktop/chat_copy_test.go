package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopyLifecycle(t *testing.T) {
	repo := newGitRepo(t)
	m := newChats(t.TempDir())
	t.Cleanup(m.close)
	ctx := context.Background()
	// The user's own uncommitted work stays in the project and out of the copy.
	writeFile(t, filepath.Join(repo.dir, "keep.txt"), "four, the user's\n")
	state, err := m.createCopy(ctx, repo.dir, "codex")
	if err != nil {
		t.Fatal(err)
	}
	copy := state.Copy
	if copy == nil || state.Project != copy.Worktree || !strings.HasPrefix(copy.Branch, "ais/") {
		t.Fatalf("copy: %+v project %s", copy, state.Project)
	}
	if readText(t, filepath.Join(copy.Worktree, "keep.txt")) != "four\n" {
		t.Fatal("the copy starts from the last commit")
	}
	if got := git(t, repo.dir, "branch", "--show-current"); got != "main" {
		t.Fatalf("the project's branch must not change: %q", got)
	}
	// The agent works in the copy: a commit and uncommitted edits.
	writeFile(t, filepath.Join(copy.Worktree, "a.txt"), "one, from the copy\n")
	git(t, copy.Worktree, "add", "a.txt")
	git(t, copy.Worktree, "-c", "user.name=ais", "-c", "user.email=ais@example.invalid", "commit", "-q", "-m", "copy work")
	writeFile(t, filepath.Join(copy.Worktree, "new.go"), "package x\n")
	writeFile(t, filepath.Join(copy.Worktree, "b.txt"), "two, edited\n")

	diff, err := m.copyChanges(ctx, state.ID)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{}
	for _, f := range diff.Files {
		paths = append(paths, f.Path)
	}
	if strings.Join(paths, ",") != "a.txt,b.txt,new.go" {
		t.Fatalf("copy changes: %v", paths)
	}
	// Removing with changes not brought back is refused.
	if err := m.removeCopy(ctx, state.ID, false); err == nil || !strings.Contains(err.Error(), "not brought back") {
		t.Fatalf("remove before bringing back: %v", err)
	}
	files, err := m.bringBack(ctx, state.ID)
	if err != nil || len(files) != 3 {
		t.Fatalf("bring back: %v %v", files, err)
	}
	if readText(t, filepath.Join(repo.dir, "a.txt")) != "one, from the copy\n" || readText(t, filepath.Join(repo.dir, "new.go")) != "package x\n" || readText(t, filepath.Join(repo.dir, "keep.txt")) != "four, the user's\n" {
		t.Fatal("the copy's changes must arrive uncommitted beside the user's own")
	}
	if strings.Contains(git(t, repo.dir, "log", "--oneline"), "copy work") {
		t.Fatal("nothing is committed in the project")
	}
	// Brought back: removing is allowed and leaves no worktree or branch.
	if err := m.removeCopy(ctx, state.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(copy.Worktree); !os.IsNotExist(err) {
		t.Fatal("the copy's folder must be gone")
	}
	if strings.Contains(git(t, repo.dir, "branch", "--list", copy.Branch), "ais/") {
		t.Fatal("the copy's branch must be gone")
	}
	c, _ := m.get(state.ID)
	if !c.state.Closed {
		t.Fatal("the conversation closes with its copy")
	}
}

func TestCopyRefusals(t *testing.T) {
	repo := newGitRepo(t)
	m := newChats(t.TempDir())
	t.Cleanup(m.close)
	ctx := context.Background()
	if _, err := m.createCopy(ctx, t.TempDir(), "codex"); err == nil {
		t.Fatal("a folder that is not a repository has no copy")
	}
	state, err := m.createCopy(ctx, repo.dir, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.bringBack(ctx, state.ID); err == nil || !strings.Contains(err.Error(), "no changes") {
		t.Fatalf("empty copy: %v", err)
	}
	writeFile(t, filepath.Join(state.Copy.Worktree, "a.txt"), "copy\n")
	// The user changed the same file: nothing is mixed in.
	writeFile(t, filepath.Join(repo.dir, "a.txt"), "mine\n")
	if _, err := m.bringBack(ctx, state.ID); err == nil || !strings.Contains(err.Error(), "own uncommitted changes in a.txt") {
		t.Fatalf("overlap: %v", err)
	}
	// The project moved on in a way the copy's change does not fit.
	git(t, repo.dir, "checkout", "--", "a.txt")
	writeFile(t, filepath.Join(repo.dir, "a.txt"), "rewritten upstream\n")
	git(t, repo.dir, "commit", "-q", "-am", "upstream")
	if _, err := m.bringBack(ctx, state.ID); err == nil || !strings.Contains(err.Error(), "do not apply") {
		t.Fatalf("conflict: %v", err)
	}
	if readText(t, filepath.Join(repo.dir, "a.txt")) != "rewritten upstream\n" {
		t.Fatal("a refused bring-back changes nothing")
	}
	// A busy conversation is left alone.
	c, _ := m.get(state.ID)
	c.mu.Lock()
	c.state.Status = "running"
	c.mu.Unlock()
	if _, err := m.bringBack(ctx, state.ID); err == nil || !strings.Contains(err.Error(), "wait for the agent") {
		t.Fatalf("busy: %v", err)
	}
	c.mu.Lock()
	c.state.Status = "idle"
	c.mu.Unlock()
	// Discarding removes it anyway.
	if err := m.removeCopy(ctx, state.ID, true); err != nil {
		t.Fatal(err)
	}
	plain := createChat(t, m, repo.dir, "codex", "")
	if _, err := m.bringBack(ctx, plain.ID); err == nil || !strings.Contains(err.Error(), "project itself") {
		t.Fatalf("not a copy: %v", err)
	}
}
