package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// What changed since a message: the diff between the checkpoint taken before
// that message's turn and the files now. Read-only; it snapshots the working
// tree without touching the index, branch or stash.

// checkpointDiff returns the changes from a checkpoint to the files now.
func checkpointDiff(ctx context.Context, root, commit string) (ReviewSnapshot, error) {
	now, err := snapshot(ctx, root, "ais turn diff")
	if err != nil {
		return ReviewSnapshot{}, err
	}
	result := ReviewSnapshot{Root: root, Base: commit, Hash: now, Files: []ReviewFile{}, Warnings: []string{}}
	names, err := gitRun(ctx, root, "diff-tree", "-r", "-z", "--no-renames", "--name-only", commit, now)
	if err != nil {
		return ReviewSnapshot{}, err
	}
	for _, name := range strings.Split(names, "\x00") {
		if name == "" {
			continue
		}
		if len(result.Files) >= 100 {
			result.Warnings = append(result.Warnings, "Only the first 100 changed files are shown.")
			break
		}
		patch, err := gitRun(ctx, root, "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--unified=20", commit, now, "--", name)
		if err != nil {
			return ReviewSnapshot{}, err
		}
		if strings.Contains(patch, "\nBinary files ") || strings.HasPrefix(patch, "Binary files ") {
			result.Warnings = append(result.Warnings, "Binary contents omitted: "+name)
		}
		if !utf8.ValidString(patch) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s has non-UTF-8 changes and is not shown.", name))
			continue
		}
		if len(result.Diff)+len(patch) > reviewLimit {
			result.Warnings = append(result.Warnings, "The diff is larger than 512 KiB; later files are not shown.")
			break
		}
		result.Files = append(result.Files, ReviewFile{Path: name, Patch: patch})
		result.Diff += patch
	}
	return result, nil
}

func (m *chatManager) turnDiff(ctx context.Context, chatID, pointID string) (ReviewSnapshot, error) {
	_, point, root, err := m.checkpointTarget(ctx, chatID, pointID)
	if err != nil {
		return ReviewSnapshot{}, err
	}
	if point.Commit == "" {
		return ReviewSnapshot{}, errors.New("this checkpoint has no saved files")
	}
	return checkpointDiff(ctx, root, point.Commit)
}

// TurnDiff shows what changed in the project since a checkpoint, such as the
// one taken before a message's turn. Changes nothing.
func (a *App) TurnDiff(chatID, checkpointID string) (ReviewSnapshot, error) {
	if err := a.chatReady(); err != nil {
		return ReviewSnapshot{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, time.Minute)
	defer cancel()
	return a.chats.turnDiff(ctx, chatID, checkpointID)
}
