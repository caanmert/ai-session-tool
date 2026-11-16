package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Both agents get the same small task, each in its own worktree; one side is
// then copied into the project and the comparison is removed.
func TestLiveComparison(t *testing.T) {
	requireLiveAgents(t)
	repo := newGitRepo(t)
	m := newChats(t.TempDir())
	t.Cleanup(m.close)
	ctx := context.Background()
	task := "Create a file named hello.txt in the current directory containing exactly the word hello followed by a newline. Do nothing else, use no other tools, and do not ask questions."
	started, err := m.startComparison(ctx, repo.dir, task)
	if err != nil || started.Problem != "" {
		t.Fatalf("start: %v %s", err, started.Problem)
	}
	record := started.Comparison
	t.Cleanup(func() { _ = m.removeComparison(context.Background(), record.ID) })
	for _, side := range record.Sides {
		target := filepath.Join(side.Worktree, "hello.txt")
		finishLiveChat(t, m, side.ChatID, func(request ChatApproval) bool {
			// Grant only the one file in this side's own worktree.
			var input struct {
				FilePath string `json:"file_path"`
			}
			_ = json.Unmarshal([]byte(request.Detail), &input)
			return input.FilePath == target || (request.Title == "Allow file changes?" && strings.Contains(request.Detail, target))
		})
		data, err := os.ReadFile(target)
		t.Logf("%s wrote %q (%v)", side.Tool, data, err)
		if err != nil || strings.TrimSpace(string(data)) != "hello" {
			t.Fatalf("%s did not do the task in its worktree", side.Tool)
		}
	}
	if _, err := os.Stat(filepath.Join(repo.dir, "hello.txt")); !os.IsNotExist(err) {
		t.Fatal("an agent wrote into the project itself")
	}
	view, err := m.comparisonView(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, side := range view.Sides {
		if got := changedPaths(side.Changes); got != "? hello.txt" {
			t.Fatalf("%s changes: %s", side.Tool, got)
		}
	}
	if _, err := m.useComparisonSide(ctx, record.ID, "codex"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(repo.dir, "hello.txt")); strings.TrimSpace(string(data)) != "hello" {
		t.Fatalf("copied file: %q", data)
	}
	if err := m.removeComparison(ctx, record.ID); err != nil {
		t.Fatal(err)
	}
	for _, side := range record.Sides {
		if _, err := os.Stat(side.Worktree); !os.IsNotExist(err) {
			t.Fatalf("%s worktree was not removed", side.Tool)
		}
	}
}
