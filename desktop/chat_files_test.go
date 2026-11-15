package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, dir string, files ...string) {
	t.Helper()
	for _, file := range files {
		path := filepath.Join(dir, file)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSearchProjectFilesInGit(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	writeTree(t, dir,
		"src/checkout.ts", "src/checkout.test.ts", "src/payments/retry.ts",
		"docs/checkout notes.md", "README.md", ".gitignore", "build/out.js", "node_modules/pkg/index.js")
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("build/\nnode_modules/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	got, err := searchProjectFiles(ctx, dir, "Checkout")
	if err != nil {
		t.Fatal(err)
	}
	// Names starting with the query come first, shortest first; a match in
	// the name beats a match that is only in the folder.
	want := []string{"src/checkout.ts", "src/checkout.test.ts", "docs/checkout notes.md"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v, want %v", got, want)
	}

	// Ignored files are left out; untracked files are included.
	if all, _ := searchProjectFiles(ctx, dir, ""); len(all) != 6 {
		t.Fatalf("expected 6 files without ignored ones, got %v", all)
	}
	if ignored, _ := searchProjectFiles(ctx, dir, "out.js"); len(ignored) != 0 {
		t.Fatalf("ignored file returned: %v", ignored)
	}
	// Letters in order still find a file.
	if fuzzy, _ := searchProjectFiles(ctx, dir, "rtry"); len(fuzzy) != 1 || fuzzy[0] != "src/payments/retry.ts" {
		t.Fatalf("fuzzy: %v", fuzzy)
	}
	if none, err := searchProjectFiles(ctx, dir, "zzzz"); err != nil || len(none) != 0 {
		t.Fatalf("no match: %v %v", none, err)
	}
}

func TestSearchProjectFilesOutsideGit(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, "notes.md", "app/main.go", ".cache/secret", "node_modules/pkg/main.go")
	got, err := searchProjectFiles(context.Background(), dir, "main")
	if err != nil || len(got) != 1 || got[0] != "app/main.go" {
		t.Fatalf("a plain folder is walked without hidden and dependency folders: %v %v", got, err)
	}
}

func TestSearchProjectFilesLimit(t *testing.T) {
	dir := t.TempDir()
	files := []string{}
	for i := 0; i < fileMatchLimit+15; i++ {
		files = append(files, filepath.Join("pkg", strings.Repeat("a", i+1)+".go"))
	}
	writeTree(t, dir, files...)
	got, err := searchProjectFiles(context.Background(), dir, "a")
	if err != nil || len(got) != fileMatchLimit || got[0] != "pkg/a.go" {
		t.Fatalf("results are capped, best first: %d %v", len(got), err)
	}
}
