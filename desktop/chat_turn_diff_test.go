package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTurnDiff(t *testing.T) {
	repo := newGitRepo(t)
	m, state := gitChat(t, repo.dir)
	c, _ := m.get(state.ID)
	ctx := context.Background()
	// The user's own uncommitted work before the turn is not part of the diff.
	writeFile(t, filepath.Join(repo.dir, "a.txt"), "one, by the user\n")
	point, err := m.addCheckpoint(ctx, c, repo.dir, "turn-1", "turn")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo.dir, "b.txt"), "two, by the agent\n")
	writeFile(t, filepath.Join(repo.dir, "added.go"), "package x\n")
	if err := os.Remove(filepath.Join(repo.dir, "c.txt")); err != nil {
		t.Fatal(err)
	}
	status := git(t, repo.dir, "status", "--porcelain")
	diff, err := m.turnDiff(ctx, state.ID, point.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, f := range diff.Files {
		got = append(got, f.Path)
	}
	if strings.Join(got, ",") != "added.go,b.txt,c.txt" {
		t.Fatalf("files: %v", got)
	}
	if !strings.Contains(diff.Files[1].Patch, "-two\n+two, by the agent") || strings.Contains(diff.Diff, "by the user") {
		t.Fatalf("patch:\n%s", diff.Diff)
	}
	// Reading the diff changes nothing.
	if git(t, repo.dir, "status", "--porcelain") != status {
		t.Fatal("a turn diff must not change the working tree or index")
	}
	if _, err := m.turnDiff(ctx, state.ID, "missing"); err == nil {
		t.Fatal("an unknown checkpoint must be refused")
	}
}
