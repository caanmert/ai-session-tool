package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func changedPaths(summary ChangeSummary) string {
	paths := []string{}
	for _, file := range summary.Files {
		paths = append(paths, file.Status+" "+file.Path)
	}
	return strings.Join(paths, ", ")
}

func TestComparisonLifecycle(t *testing.T) {
	repo := newGitRepo(t)
	m := fakeChats(t, "normal")
	ctx := context.Background()

	started, err := m.startComparison(ctx, repo.dir, "Add a greeting")
	if err != nil || started.Problem != "" {
		t.Fatalf("start: %+v %v", started, err)
	}
	record := started.Comparison
	if len(record.Sides) != 2 || len(started.Chats) != 2 || record.Base != git(t, repo.dir, "rev-parse", "HEAD") {
		t.Fatalf("record: %+v", record)
	}
	for i, side := range record.Sides {
		if git(t, side.Worktree, "branch", "--show-current") != side.Branch || !strings.HasPrefix(side.Branch, "ais-compare/") {
			t.Fatalf("%s worktree is not on its own branch", side.Tool)
		}
		if started.Chats[i].Comparison != record.ID || started.Chats[i].Project != side.Worktree || started.Chats[i].Tool != side.Tool {
			t.Fatalf("conversation not linked: %+v", started.Chats[i])
		}
		// Both agents received the task.
		done := waitChat(t, m, side.ChatID, func(s ChatState) bool { return !busyChat(s.Status) })
		if len(done.Messages) != 2 || done.Messages[0].Text != "Add a greeting" || done.Messages[1].Text != "Hello from "+side.Tool {
			t.Fatalf("%s conversation: %+v", side.Tool, done.Messages)
		}
		c, _ := m.get(side.ChatID)
		<-c.done
	}
	// The user's own working tree is not touched.
	if status := git(t, repo.dir, "status", "--porcelain"); status != "" {
		t.Fatalf("the project changed: %q", status)
	}

	// Simulate the agents' work. Claude Code also commits on its branch:
	// the comparison is against the starting commit, so that still counts.
	claude, _ := record.side("claude")
	codex, _ := record.side("codex")
	writeFile(t, filepath.Join(claude.Worktree, "a.txt"), "one, from claude\n")
	git(t, claude.Worktree, "-c", "user.name=a", "-c", "user.email=a@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-am", "claude edit")
	writeFile(t, filepath.Join(claude.Worktree, "hello.txt"), "hello\n")
	writeFile(t, filepath.Join(codex.Worktree, "hello.txt"), "hi\n")

	view, err := m.comparisonView(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := changedPaths(view.Sides[0].Changes); view.Sides[0].Tool != "claude" || got != "M a.txt, ? hello.txt" {
		t.Fatalf("claude changes: %s", got)
	}
	if got := changedPaths(view.Sides[1].Changes); got != "? hello.txt" {
		t.Fatalf("codex changes: %s", got)
	}
	if view.Sides[0].Status != "idle" || view.Prompt != "Add a greeting" {
		t.Fatalf("view: %+v", view)
	}
	diff, err := m.comparisonDiff(ctx, record.ID, "claude")
	if err != nil || len(diff.Files) != 2 {
		t.Fatalf("diff: %+v %v", diff.Files, err)
	}

	// The user's own change in a file the side touches blocks copying.
	writeFile(t, filepath.Join(repo.dir, "a.txt"), "one, from the user\n")
	if _, err := m.useComparisonSide(ctx, record.ID, "claude"); err == nil || !strings.Contains(err.Error(), "own uncommitted changes in a.txt") {
		t.Fatalf("overlap must be refused: %v", err)
	}
	if got := git(t, repo.dir, "status", "--porcelain"); got != "M a.txt" {
		t.Fatalf("a refused copy must change nothing: %q", got)
	}
	git(t, repo.dir, "checkout", "--", "a.txt")

	files, err := m.useComparisonSide(ctx, record.ID, "claude")
	if err != nil || strings.Join(files, ",") != "a.txt,hello.txt" {
		t.Fatalf("use claude: %v %v", files, err)
	}
	if data, _ := os.ReadFile(filepath.Join(repo.dir, "hello.txt")); string(data) != "hello\n" {
		t.Fatalf("hello.txt: %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(repo.dir, "a.txt")); string(data) != "one, from claude\n" {
		t.Fatalf("a.txt: %q", data)
	}
	if log := git(t, repo.dir, "log", "--format=%s"); log != "initial" {
		t.Fatalf("nothing may be committed in the project: %q", log)
	}
	// Codex's version now overlaps the copied file and is refused.
	if _, err := m.useComparisonSide(ctx, record.ID, "codex"); err == nil || !strings.Contains(err.Error(), "hello.txt") {
		t.Fatalf("second copy must be refused: %v", err)
	}

	if err := m.removeComparison(ctx, record.ID); err != nil {
		t.Fatal(err)
	}
	for _, side := range record.Sides {
		if _, err := os.Stat(side.Worktree); !os.IsNotExist(err) {
			t.Fatalf("%s worktree still exists", side.Tool)
		}
		if branches := git(t, repo.dir, "branch", "--list", side.Branch); branches != "" {
			t.Fatalf("%s branch still exists", side.Tool)
		}
		state, _ := m.read(side.ChatID, -1)
		if !state.State.Closed {
			t.Fatalf("%s conversation should be closed", side.Tool)
		}
	}
	if _, err := m.comparisonView(ctx, record.ID); err == nil {
		t.Fatal("the record should be gone")
	}
	// What was copied into the project stays.
	if got := git(t, repo.dir, "status", "--porcelain"); got != "M a.txt\n?? hello.txt" {
		t.Fatalf("project after removal: %q", got)
	}
}

func TestComparisonSetupFailureLeavesNothing(t *testing.T) {
	repo := newGitRepo(t)
	m := fakeChats(t, "normal")
	ctx := context.Background()
	original := newComparisonID
	newComparisonID = func() (string, error) { return "cmp-test", nil }
	t.Cleanup(func() { newComparisonID = original })
	// The second worktree cannot be made: its branch name is taken.
	git(t, repo.dir, "branch", "ais-compare/test/codex")
	if _, err := m.startComparison(ctx, repo.dir, "Task"); err == nil || !strings.Contains(err.Error(), "Codex worktree could not be created") {
		t.Fatalf("expected the second worktree to fail: %v", err)
	}
	if worktrees := git(t, repo.dir, "worktree", "list"); strings.Count(worktrees, "\n") != 0 {
		t.Fatalf("a half-made worktree was left: %s", worktrees)
	}
	if branch := git(t, repo.dir, "branch", "--list", "ais-compare/test/claude"); branch != "" {
		t.Fatal("the first side's branch was left")
	}
	if git(t, repo.dir, "branch", "--list", "ais-compare/test/codex") == "" {
		t.Fatal("a branch that existed before must not be deleted")
	}
	if _, err := os.Stat(filepath.Join(m.dataDir, "worktrees", "cmp-test")); !os.IsNotExist(err) {
		t.Fatal("the worktree folder was left")
	}
	if len(m.list()) != 0 {
		t.Fatal("no conversation may be left open")
	}
}

func TestComparisonRefusals(t *testing.T) {
	repo := newGitRepo(t)
	m := fakeChats(t, "normal")
	ctx := context.Background()
	if _, err := m.startComparison(ctx, repo.dir, "  "); err == nil {
		t.Fatal("an empty task must be refused")
	}
	if _, err := m.startComparison(ctx, t.TempDir(), "Task"); err == nil || !strings.Contains(err.Error(), "not a Git repository") {
		t.Fatalf("a plain folder must be refused: %v", err)
	}
	noData := newChats("")
	t.Cleanup(noData.close)
	if _, err := noData.startComparison(ctx, repo.dir, "Task"); err == nil {
		t.Fatal("comparisons need a data folder")
	}
	for _, id := range []string{"../../etc/passwd", "cmp-../x", "other"} {
		if _, err := m.comparisonView(ctx, id); err == nil {
			t.Fatalf("%q must be refused", id)
		}
	}

	started, err := m.startComparison(ctx, repo.dir, "Task")
	if err != nil {
		t.Fatal(err)
	}
	for _, side := range started.Comparison.Sides {
		waitChat(t, m, side.ChatID, func(s ChatState) bool { return !busyChat(s.Status) })
		c, _ := m.get(side.ChatID)
		<-c.done
	}
	if _, err := m.useComparisonSide(ctx, started.Comparison.ID, "codex"); err == nil || !strings.Contains(err.Error(), "has not changed anything") {
		t.Fatalf("nothing to copy: %v", err)
	}
	c, _ := m.get(started.Comparison.Sides[0].ChatID)
	c.mu.Lock()
	c.state.Status = "running"
	c.mu.Unlock()
	if _, err := m.useComparisonSide(ctx, started.Comparison.ID, "claude"); err == nil || !strings.Contains(err.Error(), "wait for Claude Code") {
		t.Fatalf("a working side must not be copied: %v", err)
	}
	if err := m.removeComparison(ctx, started.Comparison.ID); err == nil || !strings.Contains(err.Error(), "stop Claude Code") {
		t.Fatalf("a working comparison must not be removed: %v", err)
	}
	c.mu.Lock()
	c.state.Status = "idle"
	c.mu.Unlock()
}
