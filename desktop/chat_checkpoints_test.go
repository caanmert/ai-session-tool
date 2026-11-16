package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return "<missing>"
	}
	return string(data)
}

func changeList(changes []RestoreChange) string {
	parts := []string{}
	for _, change := range changes {
		parts = append(parts, change.Change+" "+change.Path)
	}
	return strings.Join(parts, ", ")
}

func TestCheckpointRestoresATurn(t *testing.T) {
	repo := newGitRepo(t)
	m, state := gitChat(t, repo.dir)
	c, _ := m.get(state.ID)
	ctx := context.Background()
	// The user's own work before the turn: a staged edit, an untracked file
	// and an ignored file.
	writeFile(t, filepath.Join(repo.dir, ".gitignore"), "*.log\n")
	writeFile(t, filepath.Join(repo.dir, "a.txt"), "one, staged by the user\n")
	git(t, repo.dir, "add", "a.txt")
	writeFile(t, filepath.Join(repo.dir, "notes.md"), "mine\n")
	writeFile(t, filepath.Join(repo.dir, "build.log"), "ignored\n")
	headBefore := git(t, repo.dir, "rev-parse", "HEAD")
	statusBefore := git(t, repo.dir, "status", "--porcelain")

	point, err := m.addCheckpoint(ctx, c, repo.dir, "turn-1", "turn")
	if err != nil {
		t.Fatal(err)
	}
	// Taking a checkpoint changes nothing the user can see.
	if git(t, repo.dir, "status", "--porcelain") != statusBefore || git(t, repo.dir, "rev-parse", "HEAD") != headBefore || git(t, repo.dir, "stash", "list") != "" {
		t.Fatal("a checkpoint must not change the index, HEAD or stash")
	}

	// The agent's turn: edits, a new file, a deletion, and an ignored file.
	writeFile(t, filepath.Join(repo.dir, "b.txt"), "two, by the agent\n")
	writeFile(t, filepath.Join(repo.dir, "added.go"), "package x\n")
	if err := os.Remove(filepath.Join(repo.dir, "c.txt")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo.dir, "notes.md"), "mine, then the agent's\n")
	writeFile(t, filepath.Join(repo.dir, "agent.log"), "ignored too\n")

	plan, err := m.restorePlan(ctx, state.ID, point.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := changeList(plan); got != "remove added.go, restore b.txt, restore c.txt, restore notes.md" {
		t.Fatalf("plan: %s", got)
	}
	changes, err := m.restore(ctx, state.ID, point.ID)
	if err != nil || changeList(changes) != changeList(plan) {
		t.Fatalf("restore: %v %v", changes, err)
	}
	for path, want := range map[string]string{"b.txt": "two\n", "c.txt": "three\n", "added.go": "<missing>", "notes.md": "mine\n", "a.txt": "one, staged by the user\n", "build.log": "ignored\n", "agent.log": "ignored too\n"} {
		if got := readText(t, filepath.Join(repo.dir, path)); got != want {
			t.Fatalf("%s: %q, want %q", path, got, want)
		}
	}
	// The user's staged edit is still staged, and HEAD did not move.
	if staged := git(t, repo.dir, "diff", "--cached", "--name-only"); staged != "a.txt" || git(t, repo.dir, "rev-parse", "HEAD") != headBefore {
		t.Fatalf("index or HEAD changed: %q", staged)
	}
	// The restore left a note with a checkpoint of its own: it can be undone.
	update, _ := m.read(state.ID, -1)
	points := update.State.Checkpoints
	last := update.State.Messages[len(update.State.Messages)-1]
	if len(points) != 2 || points[1].Kind != "restore" || points[1].MessageID != last.ID || !strings.Contains(last.Text, "Restored 4 files") {
		t.Fatalf("restore note: %+v %+v", points, last)
	}
	if _, err := m.restore(ctx, state.ID, points[1].ID); err != nil {
		t.Fatal(err)
	}
	if got := readText(t, filepath.Join(repo.dir, "b.txt")); got != "two, by the agent\n" {
		t.Fatalf("undoing the restore: %q", got)
	}
	if got := readText(t, filepath.Join(repo.dir, "added.go")); got != "package x\n" {
		t.Fatalf("undoing the restore brings files back: %q", got)
	}
}

func TestCheckpointRefusals(t *testing.T) {
	repo := newGitRepo(t)
	m, state := gitChat(t, repo.dir)
	c, _ := m.get(state.ID)
	ctx := context.Background()
	point, err := m.addCheckpoint(ctx, c, repo.dir, "turn-1", "turn")
	if err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	c.state.Status = "running"
	c.mu.Unlock()
	if _, err := m.restore(ctx, state.ID, point.ID); err == nil || !strings.Contains(err.Error(), "wait for the response") {
		t.Fatalf("busy: %v", err)
	}
	if err := m.discardFile(ctx, state.ID, "a.txt"); err == nil {
		t.Fatal("discarding while busy must be refused")
	}
	c.mu.Lock()
	c.state.Status = "idle"
	c.mu.Unlock()
	if _, err := m.restore(ctx, state.ID, "gone"); err == nil {
		t.Fatal("an unknown checkpoint must be refused")
	}
	// Nothing changed: restoring does nothing and leaves no note.
	if changes, err := m.restore(ctx, state.ID, point.ID); err != nil || len(changes) != 0 {
		t.Fatalf("nothing to restore: %v %v", changes, err)
	}
	if err := m.discardFile(ctx, state.ID, "../outside.txt"); err == nil || !strings.Contains(err.Error(), "outside the project") {
		t.Fatalf("a path outside the project: %v", err)
	}
	// Outside Git there is no checkpoint.
	plain := t.TempDir()
	m2, other := gitChat(t, plain)
	c2, _ := m2.get(other.ID)
	if _, err := m2.addCheckpoint(ctx, c2, plain, "x", "turn"); err == nil {
		t.Fatal("a folder outside Git has no checkpoint")
	}
	// Only the newest checkpoints are kept.
	for i := 0; i < checkpointLimit+3; i++ {
		if _, err := m.addCheckpoint(ctx, c, repo.dir, "m", "turn"); err != nil {
			t.Fatal(err)
		}
	}
	update, _ := m.read(state.ID, -1)
	if len(update.State.Checkpoints) != checkpointLimit {
		t.Fatalf("kept %d checkpoints", len(update.State.Checkpoints))
	}
	refs := git(t, repo.dir, "for-each-ref", "--format=%(refname)", "refs/ais/checkpoints/")
	if strings.Count(refs, "\n")+1 != checkpointLimit {
		t.Fatalf("refs kept: %d", strings.Count(refs, "\n")+1)
	}
}

func TestDiscardOneFile(t *testing.T) {
	repo := newGitRepo(t)
	m, state := gitChat(t, repo.dir)
	ctx := context.Background()
	writeFile(t, filepath.Join(repo.dir, "a.txt"), "changed\n")
	writeFile(t, filepath.Join(repo.dir, "b.txt"), "changed too\n")
	writeFile(t, filepath.Join(repo.dir, "new.txt"), "new\n")
	if err := m.discardFile(ctx, state.ID, "a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := m.discardFile(ctx, state.ID, "new.txt"); err != nil {
		t.Fatal(err)
	}
	if got := git(t, repo.dir, "status", "--porcelain"); got != "M b.txt" {
		t.Fatalf("only the chosen files change: %q", got)
	}
	// Each discard can be undone from its checkpoint.
	update, _ := m.read(state.ID, -1)
	if len(update.State.Checkpoints) != 2 {
		t.Fatalf("checkpoints: %+v", update.State.Checkpoints)
	}
	if _, err := m.restore(ctx, state.ID, update.State.Checkpoints[1].ID); err != nil {
		t.Fatal(err)
	}
	if got := readText(t, filepath.Join(repo.dir, "new.txt")); got != "new\n" {
		t.Fatalf("undo discard: %q", got)
	}
}

func TestTurnTakesACheckpoint(t *testing.T) {
	repo := newGitRepo(t)
	m := fakeChats(t, "normal")
	state := createChat(t, m, repo.dir, "codex", "")
	if err := m.send(state.ID, "Help me"); err != nil {
		t.Fatal(err)
	}
	done := waitChat(t, m, state.ID, func(s ChatState) bool { return !busyChat(s.Status) })
	if len(done.Checkpoints) != 1 || done.Checkpoints[0].MessageID != done.Messages[0].ID || done.Checkpoints[0].Kind != "turn" {
		t.Fatalf("checkpoints: %+v", done.Checkpoints)
	}
}
