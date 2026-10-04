// Package provider defines the adapter interface each coding agent (Claude
// Code, Codex, ...) implements, plus helpers that work across all of them.
package provider

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/caanmert/ai-session-tool/internal/model"
)

// ErrEmpty means a transcript file holds no resumable conversation (for
// example a session that was opened and closed without a prompt).
var ErrEmpty = errors.New("no conversation in transcript")

// ErrHidden means a transcript is not a user-facing session, e.g. one
// written by a subagent or an internal background task.
var ErrHidden = errors.New("not a user-facing session")

// ErrUnsupported means the provider cannot perform the requested action.
var ErrUnsupported = errors.New("not supported by this tool")

// FileRef is a transcript file found by Discover.
type FileRef struct {
	Path    string
	Size    int64
	ModTime time.Time
}

// Warning is a non-fatal problem found while parsing a transcript.
type Warning struct {
	Path string
	Line int
	Msg  string
}

func (w Warning) String() string {
	if w.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", w.Path, w.Line, w.Msg)
	}
	return fmt.Sprintf("%s: %s", w.Path, w.Msg)
}

// Provider adapts one coding agent's on-disk session store.
type Provider interface {
	Tool() model.Tool
	// Root is the directory the provider reads from (shown by `ais doctor`).
	Root() string
	// Discover lists transcript files. A missing root is not an error.
	Discover(ctx context.Context) ([]FileRef, error)
	// Parse summarises one transcript. It returns ErrEmpty for files with no
	// conversation, ErrHidden for subagent and other internal transcripts,
	// and collects malformed lines as warnings.
	Parse(ctx context.Context, f FileRef) (model.Session, []Warning, error)
	// Transcript loads the renderable messages of a session.
	Transcript(ctx context.Context, s model.Session) ([]model.Message, error)
	// Live lists sessions whose agent process is running right now.
	Live(ctx context.Context) ([]model.LiveState, error)
	// ResumeCmd builds the command that resumes (or forks) s in its cwd.
	ResumeCmd(s model.Session, fork bool) (*exec.Cmd, error)
	// NewCmd builds the command that starts a fresh session in cwd.
	NewCmd(cwd string) *exec.Cmd
}

// Fingerprinter is implemented by providers whose parsed sessions also
// depend on files other than the transcripts (Codex keeps session names in
// a separate index). A change in the fingerprint invalidates every cached
// session of that tool.
type Fingerprinter interface {
	Fingerprint() string
}

// ScanResult is the outcome of scanning every provider.
type ScanResult struct {
	Sessions []model.Session
	Warnings []Warning
	Empty    int // transcripts skipped because they held no conversation
	Hidden   int // transcripts skipped because they are subagent or internal
}

// Scan discovers and parses every transcript of every provider in parallel,
// attaches live state, and returns sessions sorted newest first.
func Scan(ctx context.Context, providers []Provider) (ScanResult, error) {
	type job struct {
		p Provider
		f FileRef
	}
	var jobs []job
	for _, p := range providers {
		files, err := p.Discover(ctx)
		if err != nil {
			return ScanResult{}, fmt.Errorf("%s: discover: %w", p.Tool(), err)
		}
		for _, f := range files {
			jobs = append(jobs, job{p, f})
		}
	}

	var (
		mu  sync.Mutex
		res ScanResult
		wg  sync.WaitGroup
		ch  = make(chan job)
	)
	for range min(runtime.NumCPU(), max(len(jobs), 1)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				s, warns, err := j.p.Parse(ctx, j.f)
				mu.Lock()
				res.Warnings = append(res.Warnings, warns...)
				switch {
				case errors.Is(err, ErrEmpty):
					res.Empty++
				case errors.Is(err, ErrHidden):
					res.Hidden++
				case err != nil:
					res.Warnings = append(res.Warnings, Warning{Path: j.f.Path, Msg: err.Error()})
				default:
					res.Sessions = append(res.Sessions, s)
				}
				mu.Unlock()
			}
		}()
	}
	for _, j := range jobs {
		select {
		case ch <- j:
		case <-ctx.Done():
			close(ch)
			wg.Wait()
			return ScanResult{}, ctx.Err()
		}
	}
	close(ch)
	wg.Wait()

	AttachLive(ctx, providers, res.Sessions)
	SortByUpdated(res.Sessions)
	sort.Slice(res.Warnings, func(i, j int) bool {
		if res.Warnings[i].Path != res.Warnings[j].Path {
			return res.Warnings[i].Path < res.Warnings[j].Path
		}
		return res.Warnings[i].Line < res.Warnings[j].Line
	})
	return res, nil
}

// AttachLive sets Live on every session whose agent is running now.
func AttachLive(ctx context.Context, providers []Provider, sessions []model.Session) {
	live := map[model.Tool]map[string]model.LiveState{}
	for _, p := range providers {
		states, err := p.Live(ctx)
		if err != nil {
			continue // live state is best effort
		}
		m := map[string]model.LiveState{}
		for _, st := range states {
			m[st.SessionID] = st
		}
		live[p.Tool()] = m
	}
	for i := range sessions {
		sessions[i].Live = nil
		if st, ok := live[sessions[i].Tool][sessions[i].ID]; ok {
			sessions[i].Live = &st
		}
	}
}

// SortByUpdated sorts sessions newest first, breaking ties by ID.
func SortByUpdated(sessions []model.Session) {
	sort.SliceStable(sessions, func(i, j int) bool {
		a, b := sessions[i], sessions[j]
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		return a.ID < b.ID
	})
}

// Find returns the single session whose ID starts with prefix
// (case-insensitive). An exact match wins over prefix matches.
func Find(sessions []model.Session, prefix string) (model.Session, error) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return model.Session{}, errors.New("empty session id")
	}
	var matches []model.Session
	for _, s := range sessions {
		id := strings.ToLower(s.ID)
		if id == prefix {
			return s, nil
		}
		if strings.HasPrefix(id, prefix) {
			matches = append(matches, s)
		}
	}
	switch len(matches) {
	case 0:
		return model.Session{}, fmt.Errorf("no session matches %q", prefix)
	case 1:
		return matches[0], nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q matches %d sessions, use a longer prefix:", prefix, len(matches))
	for _, s := range matches {
		fmt.Fprintf(&b, "\n  %s  %s  %s", s.ID, s.Tool, s.Title)
	}
	return model.Session{}, errors.New(b.String())
}

// For returns the provider for tool t, or nil.
func For(providers []Provider, t model.Tool) Provider {
	for _, p := range providers {
		if p.Tool() == t {
			return p
		}
	}
	return nil
}
