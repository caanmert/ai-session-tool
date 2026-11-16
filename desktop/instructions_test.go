package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectInstructions(t *testing.T) {
	repo := newGitRepo(t)
	home := t.TempDir()
	original := userHome
	userHome = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHome = original })
	ctx := context.Background()
	writeFile(t, filepath.Join(repo.dir, "CLAUDE.md"), "# Rules\n")
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, ".codex", "AGENTS.md"), "user rules\n")
	// A subfolder of the project lists the files at the repository root.
	sub := filepath.Join(repo.dir, "src")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	files, err := projectInstructionFiles(ctx, sub)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, f := range files {
		got = append(got, f.Agent+" "+f.Scope+" "+f.Name+" "+map[bool]string{true: "exists", false: "missing"}[f.Exists]+" "+map[bool]string{true: "edit", false: "view"}[f.Editable])
	}
	want := "claude project CLAUDE.md exists edit, claude project .claude/CLAUDE.md missing edit, claude project CLAUDE.local.md missing edit, codex project AGENTS.md missing edit, claude user ~/.claude/CLAUDE.md missing view, codex user ~/.codex/AGENTS.md exists view"
	if strings.Join(got, ", ") != want {
		t.Fatalf("files:\n got %s\nwant %s", strings.Join(got, ", "), want)
	}
}

func TestEditInstructions(t *testing.T) {
	repo := newGitRepo(t)
	ctx := context.Background()
	text, err := readInstruction(ctx, repo.dir, "AGENTS.md")
	if err != nil || text.Exists || text.Version != "" {
		t.Fatalf("missing file: %+v %v", text, err)
	}
	saved, err := writeInstruction(ctx, repo.dir, ".claude/CLAUDE.md", "Use tabs.\n", "")
	if err != nil || readText(t, filepath.Join(repo.dir, ".claude", "CLAUDE.md")) != "Use tabs.\n" {
		t.Fatalf("new file: %+v %v", saved, err)
	}
	// A save with the version read succeeds; a stale one is refused.
	again, err := writeInstruction(ctx, repo.dir, ".claude/CLAUDE.md", "Use spaces.\n", saved.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeInstruction(ctx, repo.dir, ".claude/CLAUDE.md", "Stale\n", saved.Version); err == nil || !strings.Contains(err.Error(), "changed on disk") {
		t.Fatalf("stale save: %v", err)
	}
	writeFile(t, filepath.Join(repo.dir, ".claude", "CLAUDE.md"), "edited elsewhere\n")
	if _, err := writeInstruction(ctx, repo.dir, ".claude/CLAUDE.md", "Mine\n", again.Version); err == nil {
		t.Fatal("a change on disk must be refused")
	}
	if readText(t, filepath.Join(repo.dir, ".claude", "CLAUDE.md")) != "edited elsewhere\n" {
		t.Fatal("a refused save must change nothing")
	}
	// A file created meanwhile is not overwritten as new.
	writeFile(t, filepath.Join(repo.dir, "AGENTS.md"), "someone else\n")
	if _, err := writeInstruction(ctx, repo.dir, "AGENTS.md", "mine\n", ""); err == nil {
		t.Fatal("a file created meanwhile must not be overwritten")
	}
	// Only known names, and no links.
	for _, name := range []string{"../escape.md", "README.md", "/etc/hosts", ".git/config"} {
		if _, err := readInstruction(ctx, repo.dir, name); err == nil {
			t.Fatalf("%q must be refused", name)
		}
		if _, err := writeInstruction(ctx, repo.dir, name, "x", ""); err == nil {
			t.Fatalf("%q must not be written", name)
		}
	}
	outside := filepath.Join(t.TempDir(), "secret.md")
	writeFile(t, outside, "secret\n")
	if err := os.Symlink(outside, filepath.Join(repo.dir, "CLAUDE.local.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := readInstruction(ctx, repo.dir, "CLAUDE.local.md"); err == nil || !strings.Contains(err.Error(), "not a plain file") {
		t.Fatalf("a link must be refused: %v", err)
	}
	if _, err := writeInstruction(ctx, repo.dir, "CLAUDE.md", strings.Repeat("x", instructionLimit+1), ""); err == nil {
		t.Fatal("an oversized file must be refused")
	}
}
