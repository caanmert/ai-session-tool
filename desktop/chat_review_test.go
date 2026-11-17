package main

import (
	"context"
	"fmt"
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

// Views show what fits and say what was left out; reviews refuse instead of
// reviewing part of the changes.
func TestViewIsPartialWhereReviewRefuses(t *testing.T) {
	ctx := context.Background()
	t.Run("many files", func(t *testing.T) {
		dir := reviewRepo(t)
		for i := range 120 {
			writeReviewFile(t, dir, fmt.Sprintf("new-%03d.txt", i), "x\n")
		}
		if _, err := captureReview(ctx, dir); err == nil || !strings.Contains(err.Error(), "at most 100 changed files, and there are 121") {
			t.Fatalf("review accepted too many files: %v", err)
		}
		view, err := captureView(ctx, dir, "HEAD")
		if err != nil {
			t.Fatal(err)
		}
		if len(view.Files) != 100 || view.Total != 121 || !view.Partial || view.Files[0].Path != "app.txt" {
			t.Fatalf("unexpected view: %d files, total %d, partial %v", len(view.Files), view.Total, view.Partial)
		}
		if strings.Join(view.Warnings, "|") != "Only the first 100 of 121 changed files are shown." {
			t.Fatalf("unexpected warnings %q", view.Warnings)
		}
	})
	t.Run("large files", func(t *testing.T) {
		dir := reviewRepo(t)
		big := strings.Repeat("a line of text that repeats\n", 20000)
		writeReviewFile(t, dir, "app.txt", big)
		writeReviewFile(t, dir, "big-new.txt", big)
		writeReviewFile(t, dir, "small.txt", "small\n")
		if _, err := captureReview(ctx, dir); err == nil || !strings.Contains(err.Error(), "512 KiB") {
			t.Fatalf("review accepted a large diff: %v", err)
		}
		view, err := captureView(ctx, dir, "HEAD")
		if err != nil {
			t.Fatal(err)
		}
		if len(view.Files) != 1 || view.Files[0].Path != "small.txt" || view.Total != 3 || !view.Partial {
			t.Fatalf("unexpected view %+v %q %d", view.Files, view.Warnings, view.Total)
		}
		want := "app.txt has changes larger than 512 KiB and is not shown.|big-new.txt is larger than 512 KiB and is not shown."
		if strings.Join(view.Warnings, "|") != want {
			t.Fatalf("unexpected warnings %q", view.Warnings)
		}
	})
	t.Run("no changes", func(t *testing.T) {
		dir := reviewRepo(t)
		writeReviewFile(t, dir, "app.txt", "before\n")
		view, err := captureView(ctx, dir, "HEAD")
		if err != nil || len(view.Files) != 0 || view.Total != 0 || view.Partial {
			t.Fatalf("unexpected view %+v %v", view, err)
		}
		if _, err := captureReview(ctx, dir); err == nil {
			t.Fatal("review of nothing accepted")
		}
	})
}
