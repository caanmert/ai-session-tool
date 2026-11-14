package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/caanmert/ai-session-tool/internal/application"
	"github.com/caanmert/ai-session-tool/internal/launch"
	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/runner"
)

type RunPatch struct {
	Patch     string `json:"patch"`
	Truncated bool   `json:"truncated"`
}

func (a *App) RunAttachCommand(id, task string) (string, error) {
	if err := a.runsReady(); err != nil {
		return "", err
	}
	run, err := a.runs.Find(id)
	if err != nil {
		return "", err
	}
	if _, err := runner.SelectTasks(run, task); err != nil || task == "" {
		return "", errors.New("choose a task to attach")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	cmd, err := a.runs.TaskAttachCmd(ctx, run, task, false)
	if err != nil {
		return "", err
	}
	quoted := make([]string, len(cmd.Args))
	for i, arg := range cmd.Args {
		quoted[i] = launch.Quote(arg)
	}
	return strings.Join(quoted, " "), nil
}

func (a *App) runsReady() error {
	if err := a.ready(); err != nil {
		return err
	}
	if a.runs == nil {
		return errors.New("parallel runs are starting")
	}
	return nil
}

func (a *App) ListRuns() ([]runner.Run, error) {
	if err := a.runsReady(); err != nil {
		return nil, err
	}
	return a.runs.List()
}

func (a *App) RunStatus(id string) (runner.RunStatus, error) {
	if err := a.runsReady(); err != nil {
		return runner.RunStatus{}, err
	}
	run, err := a.runs.Find(id)
	if err != nil {
		return runner.RunStatus{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.runs.Status(ctx, run)
}

func (a *App) RunDiff(id, task, file string) (RunPatch, error) {
	if err := a.runsReady(); err != nil {
		return RunPatch{}, err
	}
	run, err := a.runs.Find(id)
	if err != nil {
		return RunPatch{}, err
	}
	selected, err := runner.SelectTasks(run, task)
	if err != nil || task == "" {
		return RunPatch{}, errors.New("choose a task to inspect")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	patch, err := a.runs.DiffFile(ctx, run, selected.Tasks[0], file)
	if err != nil {
		return RunPatch{}, err
	}
	const limit = 512 * 1024
	result := RunPatch{Patch: patch}
	if len(patch) > limit {
		result.Truncated = true
		result.Patch = patch[:limit]
		for !utf8.ValidString(result.Patch) {
			result.Patch = result.Patch[:len(result.Patch)-1]
		}
	}
	return result, nil
}

func (a *App) StartRun(project, tool string, prompts []string) (runner.Run, error) {
	if err := a.runsReady(); err != nil {
		return runner.Run{}, err
	}
	if strings.TrimSpace(project) == "" {
		return runner.Run{}, errors.New("choose a project folder")
	}
	if tool != "claude" && tool != "codex" {
		return runner.Run{}, errors.New("choose Claude Code or Codex")
	}
	for _, prompt := range prompts {
		if len(prompt) > 20000 {
			return runner.Run{}, errors.New("each task prompt is limited to 20,000 bytes")
		}
	}
	dir, err := application.Directory(project)
	if err != nil {
		return runner.Run{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	run, err := a.runs.Start(ctx, dir, model.Tool(tool), prompts, 0)
	if err != nil && run.ID != "" {
		return runner.Run{}, fmt.Errorf("run %s setup failed; saved work is preserved and can be inspected after refreshing: %w", run.ID, err)
	}
	return run, err
}

func (a *App) StopRun(id string) error {
	if err := a.runsReady(); err != nil {
		return err
	}
	run, err := a.runs.Find(id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	return a.runs.Stop(ctx, run)
}
