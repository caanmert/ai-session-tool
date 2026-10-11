package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
	"github.com/caanmert/ai-session-tool/internal/provider/claude"
	"github.com/caanmert/ai-session-tool/internal/provider/codex"
)

type projectProvider struct {
	provider.Provider
	sessions []model.Session
}

func (p projectProvider) Discover(context.Context) ([]provider.FileRef, error) {
	var refs []provider.FileRef
	for _, s := range p.sessions {
		refs = append(refs, provider.FileRef{Path: s.ID})
	}
	return refs, nil
}

func (p projectProvider) Parse(_ context.Context, f provider.FileRef) (model.Session, []provider.Warning, error) {
	for _, s := range p.sessions {
		if s.ID == f.Path {
			return s, nil, nil
		}
	}
	return model.Session{}, nil, provider.ErrEmpty
}

func (p projectProvider) Live(context.Context) ([]model.LiveState, error) { return nil, nil }

func projectApp(t *testing.T, sessions ...model.Session) (*App, *bytes.Buffer, **exec.Cmd) {
	t.Helper()
	bin := t.TempDir()
	for _, tool := range []string{"codex", "claude"} {
		if err := os.WriteFile(filepath.Join(bin, tool), []byte("#!/bin/sh\nexit 87\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var out bytes.Buffer
	var launched *exec.Cmd
	providers := []provider.Provider{}
	for _, p := range []provider.Provider{claude.New(t.TempDir()), codex.New(t.TempDir())} {
		var own []model.Session
		for _, s := range sessions {
			if s.Tool == p.Tool() {
				own = append(own, s)
			}
		}
		providers = append(providers, projectProvider{Provider: p, sessions: own})
	}
	return &App{
		In: strings.NewReader(""), Out: &out, Err: &bytes.Buffer{},
		Now:         func() time.Time { return time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC) },
		Providers:   func() []provider.Provider { return providers },
		Interactive: func() bool { return true },
		Exec:        func(cmd *exec.Cmd) error { launched = cmd; return nil },
	}, &out, &launched
}

func projectCommand(app *App, args ...string) error {
	cmd := NewRootCmd(app)
	cmd.SetArgs(args)
	return cmd.ExecuteContext(context.Background())
}

func TestNewStartsInCurrentDirectoryAndPrintDoesNotExecute(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	canonical, err := projectDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	app, out, launched := projectApp(t)
	if err := projectCommand(app, "new", "--tool", "codex"); err != nil {
		t.Fatal(err)
	}
	if *launched == nil || (*launched).Dir != canonical || strings.Join((*launched).Args, " ") != "codex" {
		t.Fatalf("unexpected launch: %#v", *launched)
	}
	*launched = nil
	app.Interactive = func() bool { return false }
	if err := projectCommand(app, "new", "--tool", "claude", "--print"); err != nil {
		t.Fatal(err)
	}
	if *launched != nil || !strings.Contains(out.String(), "&& claude") {
		t.Fatalf("print launched an agent or returned wrong command: %s", out)
	}
	if err := projectCommand(app, "new", "--tool", "codex"); err == nil || !strings.Contains(err.Error(), "requires a terminal") {
		t.Fatalf("non-interactive launch: %v", err)
	}
}

func TestNewAgentSelection(t *testing.T) {
	app, _, launched := projectApp(t)
	dir := t.TempDir()
	app.Choose = func(_ context.Context, _ string, labels []string) (int, error) {
		if strings.Join(labels, ",") != "claude,codex" {
			t.Fatal(labels)
		}
		return 1, nil
	}
	if err := projectCommand(app, "new", "--project", dir); err != nil {
		t.Fatal(err)
	}
	if (*launched).Args[0] != "codex" {
		t.Fatal((*launched).Args)
	}
	*launched = nil
	app.Choose = func(context.Context, string, []string) (int, error) { return 0, errChoiceCanceled }
	if err := projectCommand(app, "new", "--project", dir); err != nil || *launched != nil {
		t.Fatalf("cancel: %v, %v", err, *launched)
	}
	app.LookPath = func(name string) (string, error) {
		if name == "codex" {
			return exec.LookPath(name)
		}
		return "", exec.ErrNotFound
	}
	if err := projectCommand(app, "new", "--project", dir); err != nil || *launched == nil {
		t.Fatalf("single installed agent: %v", err)
	}
	app.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	if err := projectCommand(app, "new", "--project", dir); err == nil || !strings.Contains(err.Error(), "ais setup") {
		t.Fatalf("missing agents: %v", err)
	}
	for _, args := range [][]string{{"new", "--tool", "unknown"}, {"new", "--tool", "codex", "--project", filepath.Join(dir, "missing")}} {
		if err := projectCommand(app, args...); err == nil {
			t.Fatal("expected error for", args)
		}
	}
}

func testSession(id, dir string, tool model.Tool, age time.Duration) model.Session {
	return model.Session{ID: id, CWD: dir, Tool: tool, Title: "Session " + id, UpdatedAt: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC).Add(-age)}
}

func TestContinueScopesSortsAndSelects(t *testing.T) {
	dir, elsewhere := t.TempDir(), t.TempDir()
	sessions := []model.Session{
		testSession("older", dir, model.ToolClaude, time.Hour),
		testSession("newest", dir, model.ToolCodex, time.Minute),
		testSession("unrelated", elsewhere, model.ToolCodex, 0),
		testSession("archived", dir, model.ToolCodex, 0),
	}
	sessions[0].Pinned, sessions[3].Archived = true, true
	app, out, launched := projectApp(t, sessions...)
	app.Choose = func(_ context.Context, _ string, labels []string) (int, error) {
		if len(labels) != 2 || !strings.Contains(labels[0], "newest") || !strings.Contains(labels[1], "older") {
			t.Fatal(labels)
		}
		return 1, nil
	}
	if err := projectCommand(app, "continue", "--project", dir); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*launched).Args, " ") != "claude --resume older" || (*launched).Dir != dir {
		t.Fatalf("resume: %#v", *launched)
	}
	*launched = nil
	app.Interactive = func() bool { return false }
	if err := projectCommand(app, "continue", "--project", dir, "--latest", "--print"); err != nil {
		t.Fatal(err)
	}
	if *launched != nil || !strings.Contains(out.String(), "codex resume newest") {
		t.Fatal(out.String())
	}
	out.Reset()
	if err := projectCommand(app, "continue", "OLD", "--project", dir, "--tool", "claude", "--print"); err != nil || !strings.Contains(out.String(), "--resume older") {
		t.Fatalf("id filter: %s, %v", out, err)
	}
	for _, args := range [][]string{
		{"continue", "unrelated", "--project", dir, "--print"},
		{"continue", "archived", "--project", dir, "--print"},
		{"continue", "--project", dir, "--print"},
		{"continue", "newest", "--latest", "--project", dir},
		{"continue", "--tool", "invalid", "--project", dir},
	} {
		if err := projectCommand(app, args...); err == nil {
			t.Fatal("expected failure:", args)
		}
	}
}

func TestContinueGitWorktreesNestedReposAndSymlinks(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	repo, sub, nested, worktree := filepath.Join(root, "repo"), filepath.Join(root, "repo", "src"), filepath.Join(root, "repo", "nested"), filepath.Join(root, "worktree")
	for _, dir := range []string{repo, sub, nested} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %s, %v", out, err)
		}
	}
	git(repo, "init")
	git(repo, "commit", "--allow-empty", "-m", "fixture")
	git(repo, "worktree", "add", "-b", "other", worktree)
	git(nested, "init")
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(repo, alias); err != nil {
		t.Fatal(err)
	}
	app, out, _ := projectApp(t,
		testSession("root-session", repo, model.ToolCodex, time.Minute),
		testSession("sub-session", sub, model.ToolClaude, 2*time.Minute),
		testSession("nested-session", nested, model.ToolCodex, 0),
		testSession("worktree-session", worktree, model.ToolCodex, 0),
	)
	// Stale Git environment must not redirect project detection to the worktree.
	t.Setenv("GIT_WORK_TREE", worktree)
	t.Setenv("GIT_DIR", filepath.Join(repo, ".git"))
	if err := projectCommand(app, "continue", "--project", filepath.Join(alias, "src"), "--latest", "--print"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "codex resume root-session") {
		t.Fatal(out.String())
	}
	out.Reset()
	if err := projectCommand(app, "continue", "--project", worktree, "--latest", "--print"); err != nil || !strings.Contains(out.String(), "codex resume worktree-session") {
		t.Fatalf("separate worktree: %s, %v", out, err)
	}
	out.Reset()
	if err := projectCommand(app, "continue", "--project", nested, "--latest", "--print"); err != nil || !strings.Contains(out.String(), "codex resume nested-session") {
		t.Fatalf("nested repository: %s, %v", out, err)
	}
	app.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	out.Reset()
	if err := projectCommand(app, "continue", "--project", sub, "--latest", "--print"); err != nil || !strings.Contains(out.String(), "claude --resume sub-session") {
		t.Fatalf("without git: %s, %v", out, err)
	}
}

func TestContinueEmptyAndAmbiguousIDs(t *testing.T) {
	dir := t.TempDir()
	app, _, _ := projectApp(t)
	if err := projectCommand(app, "continue", "--project", dir, "--print"); err == nil || !strings.Contains(err.Error(), "ais new") {
		t.Fatalf("empty: %v", err)
	}
	sessions := []model.Session{{ID: "abc", Tool: model.ToolCodex}, {ID: "abc", Tool: model.ToolClaude}, {ID: "abcdef", Tool: model.ToolCodex}}
	if _, err := projectSessionID(sessions, "abc"); err == nil {
		t.Fatal("duplicate cross-tool ID should be ambiguous")
	}
	if i, err := projectSessionID(sessions[1:], "abc"); err != nil || i != 0 {
		t.Fatalf("exact match must win: %d, %v", i, err)
	}
}

func TestPickerNavigationCancellationAndControls(t *testing.T) {
	m := choiceModel{title: "Choose", options: []string{"one", "bad\x1b[31m\nname", "three"}, width: 80, height: 16}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(choiceModel)
	if m.cursor != 1 || strings.Contains(m.View(), "\x1b") {
		t.Fatal(m.View())
	}
	next, quit := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !next.(choiceModel).chosen || quit == nil {
		t.Fatal("Enter did not select")
	}
	app, _, launched := projectApp(t)
	app.Choose = func(context.Context, string, []string) (int, error) { return 0, errChoiceCanceled }
	_, err := app.choose(context.Background(), "choose", []string{"one", "two"})
	if !errors.Is(err, errChoiceCanceled) || *launched != nil {
		t.Fatalf("cancellation: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := app.choose(ctx, "choose", []string{"one"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
