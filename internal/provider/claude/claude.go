// Package claude reads Claude Code sessions.
//
// Layout (Claude Code 2.x):
//
//	$CLAUDE_CONFIG_DIR (default ~/.claude)/
//	  projects/<encoded cwd>/<session id>.jsonl   one transcript per session
//	  sessions/<pid>.json                         one file per running process
//
// The encoded cwd folder name is lossy (several characters map to "-"), so
// the real cwd is always taken from the records inside the transcript.
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/proc"
	"github.com/caanmert/ai-session-tool/internal/provider"
)

// Provider reads sessions from a Claude Code config directory.
type Provider struct {
	root string
	// Bin is the claude executable used for resume/new; default "claude".
	Bin string
}

var _ provider.Provider = (*Provider)(nil)

// New returns a provider rooted at dir.
func New(dir string) *Provider {
	return &Provider{root: dir, Bin: "claude"}
}

// DefaultRoot returns $CLAUDE_CONFIG_DIR, or ~/.claude.
func DefaultRoot() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude"
	}
	return filepath.Join(home, ".claude")
}

func (p *Provider) Tool() model.Tool { return model.ToolClaude }
func (p *Provider) Root() string     { return p.root }

// Discover lists projects/*/*.jsonl. Subagent transcripts (agent-*.jsonl,
// and anything nested deeper than the project folder) are not sessions.
func (p *Provider) Discover(ctx context.Context) ([]provider.FileRef, error) {
	projects := filepath.Join(p.root, "projects")
	dirs, err := os.ReadDir(projects)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var refs []provider.FileRef
	for _, d := range dirs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !d.IsDir() {
			continue
		}
		dir := filepath.Join(projects, d.Name())
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || filepath.Ext(name) != ".jsonl" || strings.HasPrefix(name, "agent-") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			refs = append(refs, provider.FileRef{
				Path:    filepath.Join(dir, name),
				Size:    info.Size(),
				ModTime: info.ModTime(),
			})
		}
	}
	return refs, nil
}

// liveFile is the shape of sessions/<pid>.json.
type liveFile struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
	Status    string `json:"status"`
	Name      string `json:"name"`
	UpdatedAt int64  `json:"updatedAt"` // unix millis
}

// Live reads sessions/*.json and keeps entries whose process is still alive.
func (p *Provider) Live(ctx context.Context) ([]model.LiveState, error) {
	files, err := filepath.Glob(filepath.Join(p.root, "sessions", "*.json"))
	if err != nil {
		return nil, err
	}
	var out []model.LiveState
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var lf liveFile
		if json.Unmarshal(data, &lf) != nil || lf.SessionID == "" || !proc.Alive(lf.PID) {
			continue
		}
		st := model.LiveState{
			PID:       lf.PID,
			SessionID: lf.SessionID,
			CWD:       lf.CWD,
			Status:    lf.Status,
			Name:      lf.Name,
		}
		if lf.UpdatedAt > 0 {
			st.UpdatedAt = time.UnixMilli(lf.UpdatedAt).UTC()
		}
		out = append(out, st)
	}
	return out, nil
}

// ResumeCmd runs `claude --resume <id> [--fork-session]` in the session's cwd.
// The cwd matters: Claude Code only resumes sessions of the current project.
func (p *Provider) ResumeCmd(s model.Session, fork bool) (*exec.Cmd, error) {
	if s.CWD == "" {
		return nil, errors.New("session has no recorded working directory")
	}
	args := []string{"--resume", s.ID}
	if fork {
		args = append(args, "--fork-session")
	}
	cmd := exec.Command(p.Bin, args...)
	cmd.Dir = s.CWD
	return cmd, nil
}

// NewCmd starts a fresh Claude Code session in cwd.
func (p *Provider) NewCmd(cwd string) *exec.Cmd {
	cmd := exec.Command(p.Bin)
	cmd.Dir = cwd
	return cmd
}
