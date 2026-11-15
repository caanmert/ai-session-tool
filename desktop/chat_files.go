package main

import (
	"bufio"
	"bytes"
	"context"
	"io/fs"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	fileMatchLimit = 30
	// fileScanLimit bounds how many paths are examined in one search.
	fileScanLimit = 200000
)

// fileMatch ranks a path against a query: lower is better, -1 is no match.
// A match in the file name beats a match elsewhere in the path, and a name
// that starts with the query beats one that merely contains it.
func fileMatch(path, query string) int {
	if query == "" {
		return strings.Count(path, "/")
	}
	lower := strings.ToLower(path)
	name := lower[strings.LastIndex(lower, "/")+1:]
	switch {
	case strings.HasPrefix(name, query):
		return 0
	case strings.Contains(name, query):
		return 1
	case strings.Contains(lower, query):
		return 2
	}
	// Letters in order, as in "chkts" for "checkout.ts".
	i := 0
	for _, r := range lower {
		if i < len(query) && r == rune(query[i]) {
			i++
		}
	}
	if i == len(query) {
		return 3
	}
	return -1
}

type rankedFile struct {
	path string
	rank int
}

// bestFiles keeps the best fileMatchLimit matches, shortest path first within
// a rank so nearby files come before deeply nested ones.
func bestFiles(found []rankedFile) []string {
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].rank != found[j].rank {
			return found[i].rank < found[j].rank
		}
		if len(found[i].path) != len(found[j].path) {
			return len(found[i].path) < len(found[j].path)
		}
		return found[i].path < found[j].path
	})
	if len(found) > fileMatchLimit {
		found = found[:fileMatchLimit]
	}
	paths := make([]string, len(found))
	for i, file := range found {
		paths[i] = file.path
	}
	return paths
}

func nulSplit(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.IndexByte(data, 0); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// searchProjectFiles finds files in a project by name. In a Git repository it
// lists tracked and untracked files and honors .gitignore; elsewhere it walks
// the folder, skipping hidden and dependency directories. Paths are relative
// to dir. Nothing is read from the files themselves.
func searchProjectFiles(ctx context.Context, dir, query string) ([]string, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	found := []rankedFile{}
	consider := func(path string) {
		if rank := fileMatch(path, query); rank >= 0 {
			found = append(found, rankedFile{path, rank})
		}
	}
	cmd := exec.CommandContext(ctx, "git", "--no-pager", "-C", dir, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	stdout, err := cmd.StdoutPipe()
	if err == nil && cmd.Start() == nil {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		scanner.Split(nulSplit)
		seen := 0
		for scanner.Scan() {
			consider(scanner.Text())
			if seen++; seen >= fileScanLimit {
				_ = cmd.Process.Kill() // enough paths examined
				break
			}
		}
		// Git answered if it exited cleanly or was stopped at the limit. Its
		// answer stands even when nothing matches, so ignored files are never
		// picked up by the walk below.
		if waitErr := cmd.Wait(); waitErr == nil || seen >= fileScanLimit || ctx.Err() != nil {
			return bestFiles(found), ctx.Err()
		}
		found = found[:0]
	}
	// Not a Git repository: walk the folder instead.
	seen := 0
	walkErr := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are skipped
		}
		if ctx.Err() != nil || seen >= fileScanLimit {
			return filepath.SkipAll
		}
		name := entry.Name()
		if entry.IsDir() {
			if path != dir && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		seen++
		if rel, err := filepath.Rel(dir, path); err == nil {
			consider(filepath.ToSlash(rel))
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return bestFiles(found), ctx.Err()
}

// ProjectFiles finds files in a conversation's project whose path matches
// query, best matches first. It is used to mention files in a message.
func (a *App) ProjectFiles(id, query string) ([]string, error) {
	if err := a.chatReady(); err != nil {
		return nil, err
	}
	c, err := a.chats.get(id)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	project := c.state.Project
	c.mu.Unlock()
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
	defer cancel()
	return searchProjectFiles(ctx, project, query)
}
