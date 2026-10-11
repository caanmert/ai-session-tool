package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func reviewRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
	}
	git("init", "-q")
	git("config", "user.email", "test@example.invalid")
	git("config", "user.name", "Test")
	writeReviewFile(t, dir, "app.txt", "before\n")
	git("add", ".")
	git("commit", "-qm", "initial")
	writeReviewFile(t, dir, "app.txt", "after\n")
	return dir
}
func writeReviewFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestReviewSnapshotAndStalePreview(t *testing.T) {
	dir := reviewRepo(t)
	writeReviewFile(t, dir, "new file.txt", "untracked\n")
	writeReviewFile(t, dir, "binary.bin", "\x00\xff")
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, "outside")); err != nil {
		t.Fatal(err)
	}
	snapshot, err := captureReview(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Files) != 2 || len(snapshot.Warnings) != 2 || !strings.Contains(snapshot.Diff, "+untracked") || strings.Contains(snapshot.Diff, "root:") {
		t.Fatalf("unexpected snapshot %+v", snapshot)
	}
	repeat, err := captureReview(context.Background(), dir)
	if err != nil || repeat.Hash != snapshot.Hash {
		t.Fatal("unstable snapshot")
	}
	m := newChats("")
	source := createChat(t, m, dir, "codex", "")
	draft, err := m.prepareReview(context.Background(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	writeReviewFile(t, dir, "app.txt", "changed again\n")
	if _, err := m.startReview(context.Background(), draft.ID); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("stale preview accepted: %v", err)
	}
	if _, err := m.startReview(context.Background(), draft.ID); err == nil {
		t.Fatal("consumed preview reused")
	}
}
func TestCrossProviderReviewAndHandoff(t *testing.T) {
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			dir := reviewRepo(t)
			m := fakeChats(t, "review")
			command := m.command
			m.command = func(tool string, args ...string) *exec.Cmd {
				cmd := command(tool, args...)
				if tool == provider {
					cmd.Args[5] = "normal"
				}
				return cmd
			}
			source := createChat(t, m, dir, provider, "")
			draft, err := m.prepareReview(context.Background(), source.ID)
			if err != nil {
				t.Fatal(err)
			}
			if draft.Tool == provider {
				t.Fatal("reviewer must be other provider")
			}
			review, err := m.startReview(context.Background(), draft.ID)
			if err != nil {
				t.Fatal(err)
			}
			completed := waitChat(t, m, review.ID, func(s ChatState) bool { return !busyChat(s.Status) })
			if completed.Status != "idle" || !completed.Review.Complete || len(completed.Review.Findings) != 1 {
				t.Fatalf("review failed %+v", completed)
			}
			c, _ := m.get(review.ID)
			<-c.done
			selected := []string{completed.Review.Findings[0].ID}
			// Closing the source tab must not break the review's handoff.
			if err := m.remove(source.ID); err != nil {
				t.Fatal(err)
			}
			id, err := m.sendFindings(context.Background(), review.ID, selected)
			if err != nil || id != source.ID {
				t.Fatalf("handoff: %s %v", id, err)
			}
			done := waitChat(t, m, source.ID, func(s ChatState) bool { return !busyChat(s.Status) })
			if done.Status != "idle" || done.Closed || !strings.Contains(done.Messages[0].Text, "Handle the changed result") {
				t.Fatalf("missing fix request %+v", done)
			}
			if _, err := m.sendFindings(context.Background(), review.ID, selected); err == nil {
				t.Fatal("duplicate finding sent")
			}
			writeReviewFile(t, dir, "app.txt", "new change\n")
			if _, err := m.sendFindings(context.Background(), review.ID, selected); err == nil || !strings.Contains(err.Error(), "changed") {
				t.Fatalf("stale handoff accepted %v", err)
			}
		})
	}
}
func TestReviewRejectsInvalidReferences(t *testing.T) {
	for _, result := range []string{
		`{"summary":"Bad","findings":[{"priority":"P2","file":"../secret","line":1,"title":"Bad","body":"Bad"}]}`,
		`{"summary":"Bad","findings":[{"priority":"P2","file":"app.txt","line":9999,"title":"Bad","body":"Bad"}]}`,
		`{"summary":"Bad","findings":[{"priority":"P0","file":"app.txt","line":1,"title":"Bad","body":"Bad"}]}`,
	} {
		r := &ReviewState{Snapshot: ReviewSnapshot{Files: []ReviewFile{{Path: "app.txt", Patch: "@@ -1 +1 @@\n-before\n+after\n"}}}}
		if completeReview(r, result) == nil || r.Complete {
			t.Fatal("accepted invalid finding")
		}
	}
}
func TestReviewIsolatedDirectory(t *testing.T) {
	original := t.TempDir()
	state, cleanup, err := reviewDirectory(ChatState{Project: original, Review: &ReviewState{}})
	if err != nil {
		t.Fatal(err)
	}
	if state.Project == original {
		t.Fatal("review running in working tree")
	}
	entries, err := os.ReadDir(state.Project)
	if err != nil || len(entries) != 0 {
		t.Fatal("review context is not empty")
	}
	cleanup()
	if _, err := os.Stat(state.Project); !os.IsNotExist(err) {
		t.Fatal("review directory leaked")
	}
}
