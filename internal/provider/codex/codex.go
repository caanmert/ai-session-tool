// Package codex reads Codex CLI sessions ("threads").
//
// Layout (checked against openai/codex, codex-rs/rollout):
//
//	$CODEX_HOME (default ~/.codex)/
//	  sessions/YYYY/MM/DD/rollout-<ts>-<thread id>[_<rollout id>].jsonl[.zst]
//	  archived_sessions/rollout-….jsonl[.zst]
//	  session_index.jsonl        append-only {id, thread_name}; newest wins
//
// Rollouts older than a week are compressed to .jsonl.zst; a plain .jsonl
// next to its .zst sibling wins. Reverting a thread starts a new rollout
// file with the same thread id and a distinct rollout id; the newest file
// is the active one, and it reaches older history through
// session_meta.history_base.
package codex

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/caanmert/ai-session-tool/internal/jsonl"
	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
)

// Provider reads sessions from a Codex home directory.
type Provider struct {
	root string
	// Bin is the codex executable used for resume/fork/new; default "codex".
	Bin string

	mu       sync.Mutex
	loaded   bool
	rollouts map[string]string // rollout id -> path, active and archived
	names    map[string]string // thread id -> user-given name
}

var _ provider.Provider = (*Provider)(nil)

// New returns a provider rooted at dir.
func New(dir string) *Provider {
	return &Provider{root: dir, Bin: "codex"}
}

// DefaultRoot returns $CODEX_HOME, or ~/.codex.
func DefaultRoot() string {
	if d := os.Getenv("CODEX_HOME"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".codex"
	}
	return filepath.Join(home, ".codex")
}

func (p *Provider) Tool() model.Tool { return model.ToolCodex }
func (p *Provider) Root() string     { return p.root }

// rolloutName is a parsed rollout file name.
type rolloutName struct {
	stamp   string // YYYY-MM-DDThh-mm-ss, sorts chronologically
	thread  string
	rollout string
}

// parseRolloutName parses "rollout-<ts>-<thread>[_<rollout>].jsonl[.zst]".
func parseRolloutName(name string) (rolloutName, bool) {
	name = strings.TrimSuffix(name, ".zst")
	core, ok := strings.CutPrefix(name, "rollout-")
	if !ok {
		return rolloutName{}, false
	}
	core, ok = strings.CutSuffix(core, ".jsonl")
	if !ok || len(core) < 21 || core[19] != '-' {
		return rolloutName{}, false
	}
	ids := core[20:]
	thread, rollout, found := strings.Cut(ids, "_")
	if !found {
		rollout = thread
	}
	if thread == "" || rollout == "" {
		return rolloutName{}, false
	}
	return rolloutName{stamp: core[:19], thread: thread, rollout: rollout}, true
}

type rolloutFile struct {
	path string
	name rolloutName
	info fs.FileInfo
}

// walk lists the rollout files under dir, applying Codex's rule that a
// plain .jsonl hides its compressed .jsonl.zst sibling.
func walk(ctx context.Context, dir string) ([]rolloutFile, error) {
	var out []rolloutFile
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && path == dir {
				return filepath.SkipDir
			}
			return nil // unreadable subtree: skip it, keep going
		}
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if d.IsDir() {
			return nil
		}
		n, ok := parseRolloutName(d.Name())
		if !ok {
			return nil
		}
		if strings.HasSuffix(path, ".zst") {
			if _, err := os.Stat(strings.TrimSuffix(path, ".zst")); err == nil {
				return nil
			}
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		out = append(out, rolloutFile{path: path, name: n, info: info})
		return nil
	})
	return out, err
}

// Discover lists the active rollout of every thread under sessions/ and
// refreshes the rollout-id index and thread names.
func (p *Provider) Discover(ctx context.Context) ([]provider.FileRef, error) {
	active, err := walk(ctx, filepath.Join(p.root, "sessions"))
	if err != nil {
		return nil, err
	}
	archived, err := walk(ctx, filepath.Join(p.root, "archived_sessions"))
	if err != nil {
		return nil, err
	}

	rollouts := map[string]string{}
	for _, f := range append(archived, active...) {
		rollouts[f.name.rollout] = f.path
	}
	newest := map[string]rolloutFile{}
	for _, f := range active {
		cur, ok := newest[f.name.thread]
		if !ok || f.name.stamp > cur.name.stamp ||
			(f.name.stamp == cur.name.stamp && filepath.Base(f.path) > filepath.Base(cur.path)) {
			newest[f.name.thread] = f
		}
	}

	p.mu.Lock()
	p.rollouts = rollouts
	p.names = readNames(filepath.Join(p.root, "session_index.jsonl"))
	p.loaded = true
	p.mu.Unlock()

	refs := make([]provider.FileRef, 0, len(newest))
	for _, f := range newest {
		refs = append(refs, provider.FileRef{Path: f.path, Size: f.info.Size(), ModTime: f.info.ModTime()})
	}
	return refs, nil
}

// index returns the rollout-id index and thread names, building them on
// first use when Parse or Transcript runs without a prior Discover.
func (p *Provider) index(ctx context.Context) (rollouts, names map[string]string) {
	p.mu.Lock()
	loaded := p.loaded
	p.mu.Unlock()
	if !loaded {
		_, _ = p.Discover(ctx)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.rollouts, p.names
}

// readNames reads session_index.jsonl; later entries override earlier ones.
func readNames(path string) map[string]string {
	names := map[string]string{}
	_ = jsonl.EachFile(path, func(_ int, line []byte) error {
		var e struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
		}
		if json.Unmarshal(line, &e) == nil && e.ID != "" {
			names[e.ID] = e.ThreadName
		}
		return nil
	})
	return names
}

// Live is not implemented: Codex keeps no registry of running processes.
func (p *Provider) Live(context.Context) ([]model.LiveState, error) { return nil, nil }

// ResumeCmd runs `codex resume <id>` (or `codex fork <id>`) in the
// session's cwd, so the resumed thread keeps its project directory.
func (p *Provider) ResumeCmd(s model.Session, fork bool) (*exec.Cmd, error) {
	if s.CWD == "" {
		return nil, errors.New("session has no recorded working directory")
	}
	sub := "resume"
	if fork {
		sub = "fork"
	}
	cmd := exec.Command(p.Bin, sub, s.ID)
	cmd.Dir = s.CWD
	return cmd, nil
}

// NewCmd starts a fresh Codex session in cwd.
func (p *Provider) NewCmd(cwd string) *exec.Cmd {
	cmd := exec.Command(p.Bin)
	cmd.Dir = cwd
	return cmd
}
