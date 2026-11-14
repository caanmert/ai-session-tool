package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// ChangedFile is one uncommitted file in a conversation's project.
// Status is M (modified), A (added), D (deleted) or ? (untracked).
type ChangedFile struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Binary  bool   `json:"binary,omitempty"`
}

// ChangeSummary lists working-tree changes against HEAD. It is a read-only
// view: nothing is staged, committed or reverted. When the project cannot be
// inspected, Available is false and Reason says why.
type ChangeSummary struct {
	Available bool          `json:"available"`
	Reason    string        `json:"reason,omitempty"`
	Files     []ChangedFile `json:"files"`
	Added     int           `json:"added"`
	Removed   int           `json:"removed"`
	Truncated bool          `json:"truncated,omitempty"`
}

const (
	changedFileLimit   = 200
	untrackedReadLimit = 256 * 1024
)

func summarizeChanges(ctx context.Context, dir string) ChangeSummary {
	unavailable := func(reason string) ChangeSummary {
		return ChangeSummary{Reason: reason, Files: []ChangedFile{}}
	}
	raw, err := reviewGit(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return unavailable("This project is not a Git repository.")
	}
	root := strings.TrimSuffix(string(raw), "\n")
	if _, err := reviewGit(ctx, root, "rev-parse", "--verify", "HEAD"); err != nil {
		return unavailable("Changes appear after the project's first commit.")
	}
	statuses, err := reviewGit(ctx, root, "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--name-status", "-z", "HEAD", "--")
	if err != nil {
		return unavailable("Changes could not be read: " + err.Error())
	}
	counts, err := reviewGit(ctx, root, "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--numstat", "-z", "HEAD", "--")
	if err != nil {
		return unavailable("Changes could not be read: " + err.Error())
	}
	summary := ChangeSummary{Available: true, Files: []ChangedFile{}}
	byPath := map[string]int{}
	add := func(file ChangedFile) bool {
		if len(summary.Files) >= changedFileLimit {
			summary.Truncated = true
			return false
		}
		byPath[file.Path] = len(summary.Files)
		summary.Files = append(summary.Files, file)
		return true
	}
	// --name-status -z: status NUL path NUL ...
	fields := strings.Split(string(statuses), "\x00")
	for i := 0; i+1 < len(fields); i += 2 {
		status := fields[i]
		if status == "" {
			break
		}
		if !add(ChangedFile{Path: fields[i+1], Status: status[:1]}) {
			break
		}
	}
	// --numstat -z: added TAB removed TAB path NUL ("-" for binary files)
	for _, line := range strings.Split(string(counts), "\x00") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		index, ok := byPath[parts[2]]
		if !ok {
			continue
		}
		file := &summary.Files[index]
		if parts[0] == "-" {
			file.Binary = true
			continue
		}
		file.Added, _ = strconv.Atoi(parts[0])
		file.Removed, _ = strconv.Atoi(parts[1])
	}
	untracked, err := reviewGit(ctx, root, "ls-files", "--others", "--exclude-standard", "-z")
	if err == nil {
		filesystem, openErr := os.OpenRoot(root)
		if openErr == nil {
			defer filesystem.Close()
			for _, name := range strings.Split(string(untracked), "\x00") {
				if name == "" {
					continue
				}
				file := ChangedFile{Path: name, Status: "?"}
				file.Added, file.Binary = countNewLines(filesystem, name)
				if !add(file) {
					break
				}
			}
		}
	} else {
		summary.Truncated = true
	}
	for _, file := range summary.Files {
		summary.Added += file.Added
		summary.Removed += file.Removed
	}
	return summary
}

// countNewLines reports the line count of a new text file. Large, binary and
// non-regular files report no lines rather than being read in full.
func countNewLines(filesystem *os.Root, name string) (lines int, binary bool) {
	info, err := filesystem.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() > untrackedReadLimit {
		return 0, err == nil && info.Mode().IsRegular()
	}
	f, err := filesystem.Open(name)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, untrackedReadLimit))
	if err != nil || bytes.IndexByte(content, 0) >= 0 {
		return 0, err == nil
	}
	lines = bytes.Count(content, []byte("\n"))
	if len(content) > 0 && content[len(content)-1] != '\n' {
		lines++
	}
	return lines, false
}

func (m *chatManager) changes(ctx context.Context, id string) (ChangeSummary, error) {
	c, err := m.get(id)
	if err != nil {
		return ChangeSummary{}, err
	}
	c.mu.Lock()
	project := c.state.Project
	c.mu.Unlock()
	return summarizeChanges(ctx, project), nil
}

// diff captures the full working-tree diff for review in the app.
func (m *chatManager) diff(ctx context.Context, id string) (ReviewSnapshot, error) {
	c, err := m.get(id)
	if err != nil {
		return ReviewSnapshot{}, err
	}
	c.mu.Lock()
	project := c.state.Project
	c.mu.Unlock()
	if project == "" {
		return ReviewSnapshot{}, errors.New("this conversation has no project folder")
	}
	return captureReview(ctx, project)
}

// ChatChanges lists the uncommitted files in a conversation's project.
func (a *App) ChatChanges(id string) (ChangeSummary, error) {
	if err := a.chatReady(); err != nil {
		return ChangeSummary{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	return a.chats.changes(ctx, id)
}

// ChatDiff returns the uncommitted diff of a conversation's project.
func (a *App) ChatDiff(id string) (ReviewSnapshot, error) {
	if err := a.chatReady(); err != nil {
		return ReviewSnapshot{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.chats.diff(ctx, id)
}
