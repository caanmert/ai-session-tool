package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func changesRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=ais", "-c", "user.email=ais@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	write("keep.txt", "one\ntwo\nthree\n")
	write("remove.txt", "gone\n")
	write(".gitignore", "ignored.log\n")
	git("add", "-A")
	git("commit", "-q", "-m", "initial")
	write("keep.txt", "one\nTWO\nthree\nfour\n")
	if err := os.Remove(filepath.Join(dir, "remove.txt")); err != nil {
		t.Fatal(err)
	}
	write("staged.txt", "a\nb\n")
	git("add", "staged.txt")
	write("new file.txt", "x\ny\nz")
	write("blob.bin", "a\x00b")
	write("ignored.log", "noise\n")
	return dir
}

func TestSummarizeChanges(t *testing.T) {
	summary := summarizeChanges(context.Background(), changesRepo(t))
	if !summary.Available || summary.Truncated {
		t.Fatalf("expected an available summary: %+v", summary)
	}
	got := map[string]ChangedFile{}
	for _, file := range summary.Files {
		got[file.Path] = file
	}
	want := map[string]ChangedFile{
		"keep.txt":     {Path: "keep.txt", Status: "M", Added: 2, Removed: 1},
		"remove.txt":   {Path: "remove.txt", Status: "D", Removed: 1},
		"staged.txt":   {Path: "staged.txt", Status: "A", Added: 2},
		"new file.txt": {Path: "new file.txt", Status: "?", Added: 3},
		"blob.bin":     {Path: "blob.bin", Status: "?", Binary: true},
	}
	if len(got) != len(want) {
		t.Fatalf("files: %+v", summary.Files)
	}
	for path, file := range want {
		if got[path] != file {
			t.Errorf("%s: got %+v, want %+v", path, got[path], file)
		}
	}
	if summary.Added != 7 || summary.Removed != 2 {
		t.Errorf("totals: +%d -%d", summary.Added, summary.Removed)
	}
}

func TestSummarizeChangesUnavailable(t *testing.T) {
	plain := summarizeChanges(context.Background(), t.TempDir())
	if plain.Available || !strings.Contains(plain.Reason, "not a Git repository") || plain.Files == nil {
		t.Fatalf("a plain folder is a normal state, not an error: %+v", plain)
	}
	empty := t.TempDir()
	if out, err := exec.Command("git", "-C", empty, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	fresh := summarizeChanges(context.Background(), empty)
	if fresh.Available || !strings.Contains(fresh.Reason, "first commit") {
		t.Fatalf("a repository without commits: %+v", fresh)
	}
}

func TestChatChangesAndDiff(t *testing.T) {
	m := newChats("")
	t.Cleanup(m.close)
	state, err := m.create(changesRepo(t), "codex", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := m.changes(context.Background(), state.ID)
	if err != nil || len(summary.Files) != 5 {
		t.Fatalf("changes: %+v %v", summary, err)
	}
	snapshot, err := m.diff(context.Background(), state.ID)
	if err != nil || len(snapshot.Files) == 0 {
		t.Fatalf("diff: %v", err)
	}
	if _, err := m.changes(context.Background(), "missing"); err == nil {
		t.Fatal("an unknown conversation should be an error")
	}
}
