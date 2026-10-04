// Package trash removes sessions from their tool without losing them.
//
// Claude Code keeps nothing but the transcript files, so a Claude session's
// files are moved into ais's trash and moved back on restore. Codex keeps
// its own state database, so ais asks Codex to do it: `codex archive`
// to trash, `codex unarchive` to restore, `codex delete` when emptying.
package trash

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/caanmert/ai-session-tool/internal/model"
)

// Method says how a session was trashed.
const (
	MethodMove         = "move"          // files moved into the trash
	MethodCodexArchive = "codex-archive" // `codex archive`
)

// Move is one file or directory moved into the trash.
type Move struct {
	From string `json:"from"` // original location
	To   string `json:"to"`   // location inside the trash
}

// Entry is a trashed session.
type Entry struct {
	Tool      model.Tool `json:"tool"`
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	CWD       string     `json:"cwd,omitempty"`
	TrashedAt time.Time  `json:"trashedAt"`
	Method    string     `json:"method"`
	Files     []Move     `json:"files,omitempty"`

	dir string
}

// Trash is a trash directory.
type Trash struct {
	Dir string
	// CodexBin is the codex executable; default "codex".
	CodexBin string
	// Run executes a codex command and returns its combined output.
	Run func(*exec.Cmd) ([]byte, error)
}

// New returns the trash at dir.
func New(dir string) *Trash {
	return &Trash{Dir: dir, CodexBin: "codex", Run: func(c *exec.Cmd) ([]byte, error) { return c.CombinedOutput() }}
}

// ErrLive refuses to trash a session whose agent is running.
var ErrLive = errors.New("session is running; quit it first")

func (t *Trash) entryDir(tool model.Tool, id string) string {
	return filepath.Join(t.Dir, string(tool)+"-"+id)
}

// Put moves a session to the trash.
func (t *Trash) Put(ctx context.Context, s model.Session) (Entry, error) {
	if s.Live != nil {
		return Entry{}, ErrLive
	}
	dir := t.entryDir(s.Tool, s.ID)
	if _, err := os.Stat(dir); err == nil {
		return Entry{}, fmt.Errorf("%s is already in the trash", s.ID)
	}
	e := Entry{Tool: s.Tool, ID: s.ID, Title: s.Title, CWD: s.CWD, TrashedAt: time.Now().UTC(), dir: dir}

	switch s.Tool {
	case model.ToolCodex:
		if err := t.codex(ctx, "archive", s.ID); err != nil {
			return Entry{}, err
		}
		e.Method = MethodCodexArchive
	default:
		e.Method = MethodMove
		if err := os.MkdirAll(filepath.Join(dir, "files"), 0o700); err != nil {
			return Entry{}, err
		}
		// The transcript, plus Claude's sibling folder (subagents, tool
		// output) when it exists.
		sources := []string{s.Path}
		if sib := strings.TrimSuffix(s.Path, ".jsonl"); sib != s.Path {
			if info, err := os.Stat(sib); err == nil && info.IsDir() {
				sources = append(sources, sib)
			}
		}
		for _, src := range sources {
			dst := filepath.Join(dir, "files", filepath.Base(src))
			if err := os.Rename(src, dst); err != nil {
				t.undo(e.Files)
				os.RemoveAll(dir)
				return Entry{}, fmt.Errorf("move %s to the trash: %w", src, err)
			}
			e.Files = append(e.Files, Move{From: src, To: dst})
		}
	}
	if err := writeManifest(dir, e); err != nil {
		if e.Method == MethodMove {
			t.undo(e.Files)
			os.RemoveAll(dir)
		}
		return Entry{}, err
	}
	return e, nil
}

// undo moves files back to where they came from, best effort.
func (t *Trash) undo(moves []Move) {
	for i := len(moves) - 1; i >= 0; i-- {
		_ = os.Rename(moves[i].To, moves[i].From)
	}
}

func writeManifest(dir string, e Entry) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o600)
}

// List returns trashed sessions, most recently trashed first.
func (t *Trash) List() ([]Entry, error) {
	dirs, err := os.ReadDir(t.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		dir := filepath.Join(t.Dir, d.Name())
		data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
		if err != nil {
			continue
		}
		var e Entry
		if json.Unmarshal(data, &e) != nil {
			continue
		}
		e.dir = dir
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TrashedAt.After(out[j].TrashedAt) })
	return out, nil
}

// Find returns the trashed session whose id starts with prefix.
func (t *Trash) Find(prefix string) (Entry, error) {
	entries, err := t.List()
	if err != nil {
		return Entry{}, err
	}
	prefix = strings.ToLower(prefix)
	var matches []Entry
	for _, e := range entries {
		if strings.ToLower(e.ID) == prefix {
			return e, nil
		}
		if strings.HasPrefix(strings.ToLower(e.ID), prefix) {
			matches = append(matches, e)
		}
	}
	switch len(matches) {
	case 0:
		return Entry{}, fmt.Errorf("nothing in the trash matches %q", prefix)
	case 1:
		return matches[0], nil
	}
	return Entry{}, fmt.Errorf("%q matches %d trashed sessions, use a longer prefix", prefix, len(matches))
}

// Restore puts a trashed session back where it was.
func (t *Trash) Restore(ctx context.Context, e Entry) error {
	switch e.Method {
	case MethodCodexArchive:
		if err := t.codex(ctx, "unarchive", e.ID); err != nil {
			return err
		}
	case MethodMove:
		for _, m := range e.Files {
			if _, err := os.Stat(m.From); err == nil {
				return fmt.Errorf("can't restore: %s exists again", m.From)
			}
		}
		for i, m := range e.Files {
			if err := os.MkdirAll(filepath.Dir(m.From), 0o700); err != nil {
				return err
			}
			if err := os.Rename(m.To, m.From); err != nil {
				for _, back := range e.Files[:i] { // put the trash back as it was
					_ = os.Rename(back.From, back.To)
				}
				return fmt.Errorf("restore %s: %w", m.From, err)
			}
		}
	default:
		return fmt.Errorf("unknown trash method %q", e.Method)
	}
	return os.RemoveAll(e.dir)
}

// Delete permanently removes a trashed session.
func (t *Trash) Delete(ctx context.Context, e Entry) error {
	if e.Method == MethodCodexArchive {
		if err := t.codex(ctx, "delete", e.ID, "--force"); err != nil {
			return err
		}
	}
	return os.RemoveAll(e.dir)
}

func (t *Trash) codex(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, t.CodexBin, args...)
	if cmd.Err != nil {
		return fmt.Errorf("codex %s: %w", args[0], cmd.Err)
	}
	out, err := t.Run(cmd)
	if err != nil {
		msg := strings.TrimSpace(string(bytes.TrimSpace(out)))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("codex %s %s: %s", args[0], args[1], msg)
	}
	return nil
}
