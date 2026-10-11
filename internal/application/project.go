// Package application contains shared session and project operations for the CLI and desktop app.
package application

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/caanmert/ai-session-tool/internal/model"
)

type Projects struct{ LookPath func(string) (string, error) }

func (p Projects) lookup(name string) (string, error) {
	if p.LookPath != nil {
		return p.LookPath(name)
	}
	return exec.LookPath(name)
}

func ValidateTool(tool string) error {
	if tool != "" && tool != string(model.ToolClaude) && tool != string(model.ToolCodex) {
		return fmt.Errorf("unknown --tool %q: use claude or codex", tool)
	}
	return nil
}

func Directory(path string) (string, error) {
	dir, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("project directory: %w", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project path is not a directory: %s", dir)
	}
	return dir, nil
}

// Root returns the worktree root, so separate worktrees and nested
// repositories stay separate projects. Without Git, matching is directory-only.
func (p Projects) Root(ctx context.Context, dir string) string {
	git, err := p.lookup("git")
	if err != nil {
		return dir
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, git, "-C", dir, "rev-parse", "--show-toplevel")
	// A caller's Git overrides must not redirect project detection elsewhere.
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_PREFIX":
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	out, err := cmd.Output()
	if err != nil {
		return dir
	}
	root, err := Directory(strings.TrimSuffix(string(out), "\n"))
	if err != nil {
		return dir
	}
	return root
}

func (p Projects) Filter(ctx context.Context, dir, tool string, sessions []model.Session) ([]model.Session, string, error) {
	root := p.Root(ctx, dir)
	matching := map[string]bool{}
	var result []model.Session
	for _, s := range sessions {
		if err := ctx.Err(); err != nil {
			return nil, root, err
		}
		if s.Archived || s.CWD == "" || (tool != "" && string(s.Tool) != tool) {
			continue
		}
		match, checked := matching[s.CWD]
		if !checked {
			cwd, err := Directory(s.CWD)
			if err == nil {
				// Most stored projects are unrelated. Avoid invoking Git for them.
				rel, err := filepath.Rel(root, cwd)
				if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
					match = p.Root(ctx, cwd) == root
				}
			}
			matching[s.CWD] = match
		}
		if match {
			result = append(result, s)
		}
	}
	// Pins affect the browser, but --latest must follow actual update time.
	sort.Slice(result, func(i, j int) bool {
		if !result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].UpdatedAt.After(result[j].UpdatedAt)
		}
		return result[i].Key() < result[j].Key()
	})
	return result, root, ctx.Err()
}
