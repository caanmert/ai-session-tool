package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/caanmert/ai-session-tool/internal/application"
	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/runner"
)

func fixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func fixtureRunApp(t *testing.T) (*App, runner.Run) {
	t.Helper()
	repo, data := t.TempDir(), t.TempDir()
	fixtureGit(t, repo, "init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "app.txt"), []byte("before\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, repo, "add", "app.txt")
	fixtureGit(t, repo, "commit", "-qm", "base")
	base := fixtureGit(t, repo, "rev-parse", "HEAD")
	worktree := filepath.Join(t.TempDir(), "task")
	fixtureGit(t, repo, "worktree", "add", "-qb", "fixture-task", worktree, base)
	if err := os.WriteFile(filepath.Join(worktree, "app.txt"), []byte("after\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "odd [1].txt"), []byte("untracked\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run := runner.Run{ID: "ais-fixture", Tool: model.ToolCodex, Repository: repo, Base: base, CreatedAt: time.Now(), Tasks: []runner.Task{{Window: "task-1", Branch: "fixture-task", Path: worktree, Prompt: "Fixture task"}}}
	manager := runner.New(data)
	manager.LookPath = func(name string) (string, error) { return "/fixture/" + name, nil }
	manager.Command = func(_ context.Context, _ string, bin string, args ...string) (string, error) {
		if bin == "tmux" && args[0] == "list-panes" {
			return "%1\t1\t0\t\t1\t0\ttask-1\ttask-1", nil
		}
		return "", nil
	}
	if err := os.MkdirAll(filepath.Join(data, run.ID), 0700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(run)
	if err := os.WriteFile(filepath.Join(data, run.ID, "run.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	service := application.NewService(nil, "", "")
	t.Cleanup(func() { _ = service.Close() })
	return &App{ctx: context.Background(), service: service, runs: manager}, run
}

func TestRunDashboardInspectsWithoutChangingFiles(t *testing.T) {
	app, run := fixtureRunApp(t)
	before := fixtureGit(t, run.Tasks[0].Path, "--no-optional-locks", "status", "--porcelain")
	list, err := app.ListRuns()
	if err != nil || len(list) != 1 || list[0].ID != run.ID {
		t.Fatalf("saved runs: %+v, %v", list, err)
	}
	if command, err := app.RunAttachCommand(run.ID, "task-1"); err != nil || command != "tmux attach-session -t %1" {
		t.Fatalf("attach command: %s, %v", command, err)
	}
	status, err := app.RunStatus(run.ID)
	if err != nil || status.Tasks[0].State != "exited" || status.Tasks[0].ExitCode == nil || *status.Tasks[0].ExitCode != 0 || !status.Tasks[0].Attention || len(status.Tasks[0].Files) != 2 {
		t.Fatalf("status: %+v, %v", status, err)
	}
	patch, err := app.RunDiff(run.ID, "task-1", "odd [1].txt")
	if err != nil || !strings.Contains(patch.Patch, "+untracked") || strings.Contains(patch.Patch, "+after") {
		t.Fatalf("literal diff: %+v, %v", patch, err)
	}
	all, err := app.RunDiff(run.ID, "task-1", "")
	if err != nil || !strings.Contains(all.Patch, "+after") || !strings.Contains(all.Patch, "+untracked") {
		t.Fatalf("all changes: %+v, %v", all, err)
	}
	if _, err := app.RunDiff(run.ID, "task-1", "../outside"); err == nil {
		t.Fatal("escaped worktree")
	}
	if _, err := app.RunDiff(run.ID, "missing", ""); err == nil {
		t.Fatal("unknown task accepted")
	}
	if _, err := app.RunStatus("does-not-exist"); err == nil {
		t.Fatal("unknown run accepted")
	}
	if after := fixtureGit(t, run.Tasks[0].Path, "--no-optional-locks", "status", "--porcelain"); before != after {
		t.Fatal("inspection changed Git state")
	}
	app.runs.LookPath = func(string) (string, error) { return "", errors.New("tmux absent") }
	status, err = app.RunStatus(run.ID)
	if err != nil || status.Warning == "" || status.Tasks[0].State != "unknown" || len(status.Tasks[0].Files) != 2 {
		t.Fatalf("offline tmux: %+v, %v", status, err)
	}
}

func TestRunDashboardStopPreservesWorkAndBoundsPreview(t *testing.T) {
	app, run := fixtureRunApp(t)
	stopped := ""
	app.runs.Command = func(_ context.Context, _ string, bin string, args ...string) (string, error) {
		stopped = bin + " " + strings.Join(args, " ")
		return "", nil
	}
	if err := app.StopRun(run.ID); err != nil {
		t.Fatal(err)
	}
	if stopped != "tmux kill-session -t =ais-fixture" {
		t.Fatalf("stop target: %s", stopped)
	}
	if _, err := os.Stat(filepath.Join(run.Tasks[0].Path, "app.txt")); err != nil {
		t.Fatal("stop removed work")
	}
	if list, err := app.ListRuns(); err != nil || len(list) != 1 {
		t.Fatal("stop removed saved run")
	}
	if err := os.WriteFile(filepath.Join(run.Tasks[0].Path, "large.txt"), []byte(strings.Repeat("é", 300000)), 0600); err != nil {
		t.Fatal(err)
	}
	patch, err := app.RunDiff(run.ID, "task-1", "large.txt")
	if err != nil || !patch.Truncated || len(patch.Patch) > 512*1024 || !utf8.ValidString(patch.Patch) {
		t.Fatalf("bounded UTF-8 preview: bytes=%d truncated=%v err=%v", len(patch.Patch), patch.Truncated, err)
	}
}

func TestRunDashboardLaunchAndPartialFailureUseSharedRunner(t *testing.T) {
	app, existing := fixtureRunApp(t)
	tmuxCalls := 0
	fail := false
	app.runs.Command = func(ctx context.Context, dir, bin string, args ...string) (string, error) {
		if bin == "git" {
			cmd := exec.CommandContext(ctx, "git", args...)
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			return strings.TrimSpace(string(out)), err
		}
		tmuxCalls++
		if fail {
			return "", errors.New("fixture tmux failure")
		}
		return "%1", nil
	}
	if _, err := app.StartRun("", "codex", []string{"task"}); err == nil {
		t.Fatal("blank project accepted")
	}
	if _, err := app.StartRun(existing.Repository, "invalid", []string{"task"}); err == nil {
		t.Fatal("invalid provider accepted")
	}
	if _, err := app.StartRun(existing.Repository, "codex", []string{strings.Repeat("x", 20001)}); err == nil {
		t.Fatal("oversized prompt accepted")
	}
	if tmuxCalls != 0 {
		t.Fatal("invalid request launched tmux")
	}
	run, err := app.StartRun(existing.Repository, "codex", []string{"First task\nwith details", "Second task"})
	if err != nil || len(run.Tasks) != 2 || !run.Notifications || tmuxCalls == 0 {
		t.Fatalf("launch: %+v, %v", run, err)
	}
	for _, task := range run.Tasks {
		data, err := os.ReadFile(filepath.Join(task.Path, "app.txt"))
		if err != nil || string(data) != "before\n" {
			t.Fatal("worktree did not start from committed HEAD")
		}
	}
	fail = true
	if _, err := app.StartRun(existing.Repository, "claude", []string{"Preserve partial work"}); err == nil || !strings.Contains(err.Error(), "saved work is preserved") {
		t.Fatalf("partial setup: %v", err)
	}
	list, err := app.ListRuns()
	if err != nil || len(list) != 3 {
		t.Fatalf("partial manifest not discoverable: %d, %v", len(list), err)
	}
	for _, item := range list {
		for _, task := range item.Tasks {
			if _, err := os.Stat(task.Path); err != nil {
				t.Fatal("partial worktree removed")
			}
		}
	}
}
