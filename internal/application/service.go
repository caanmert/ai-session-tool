package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/caanmert/ai-session-tool/internal/index"
	"github.com/caanmert/ai-session-tool/internal/meta"
	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
)

type Session struct {
	model.Session
	Key     string `json:"key"`
	Project string `json:"project"`
}

type Project struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Sessions  int    `json:"sessions"`
	Available bool   `json:"available"`
	Recent    bool   `json:"recent"`
}

type Tool struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Available bool   `json:"available"`
}

type Snapshot struct {
	Sessions []Session `json:"sessions"`
	Projects []Project `json:"projects"`
	Tools    []Tool    `json:"tools"`
	Warnings []string  `json:"warnings"`
}

type Preview struct {
	Messages  []model.Message `json:"messages"`
	Truncated bool            `json:"truncated"`
}

// Service serializes access to shared providers, SQLite handles and snapshots.
// All state belongs to ais; providers' conversation files are read-only here.
type Service struct {
	mu        sync.Mutex
	providers []provider.Provider
	indexPath string
	dataDir   string
	ix        *index.Index
	store     *meta.Store
	snapshot  Snapshot
	loaded    bool
	closed    bool
}

func NewService(providers []provider.Provider, indexPath, dataDir string) *Service {
	return &Service{providers: providers, indexPath: indexPath, dataDir: dataDir}
}

func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	var errs []error
	if s.ix != nil {
		errs = append(errs, s.ix.Close())
		s.ix = nil
	}
	if s.store != nil {
		errs = append(errs, s.store.Close())
		s.store = nil
	}
	return errors.Join(errs...)
}

func (s *Service) Refresh(ctx context.Context) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refresh(ctx)
}

func (s *Service) refresh(ctx context.Context) (Snapshot, error) {
	if s.closed {
		return Snapshot{}, errors.New("session manager is closed")
	}
	result := Snapshot{Sessions: []Session{}, Projects: []Project{}, Tools: []Tool{}, Warnings: []string{}}
	if s.ix == nil && s.indexPath != "" {
		var err error
		s.ix, err = index.Open(s.indexPath)
		if err != nil {
			result.Warnings = append(result.Warnings, "Search index unavailable: "+err.Error())
		}
	}
	var scan provider.ScanResult
	var err error
	if s.ix != nil {
		scan, err = s.ix.Sync(ctx, s.providers)
	} else {
		scan, err = provider.Scan(ctx, s.providers)
	}
	if err != nil {
		return Snapshot{}, err
	}
	if len(scan.Warnings) > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%d transcript or live-state warnings. Run ais doctor for details.", len(scan.Warnings)))
	}
	if s.store == nil && s.dataDir != "" {
		s.store, err = meta.Open(filepath.Join(s.dataDir, "ais.db"))
		if err != nil {
			result.Warnings = append(result.Warnings, "Annotations unavailable: "+err.Error())
		}
	}
	if s.store != nil {
		annotations, err := s.store.All(ctx)
		if err != nil {
			result.Warnings = append(result.Warnings, "Annotations unavailable: "+err.Error())
		} else {
			meta.Apply(scan.Sessions, annotations)
		}
	}
	recent, err := s.readRecent()
	if err != nil {
		result.Warnings = append(result.Warnings, "Recent projects unavailable: "+err.Error())
	}
	projects := map[string]*Project{}
	add := func(path string, recent bool) {
		if _, ok := projects[path]; ok {
			return
		}
		_, err := Directory(path)
		projects[path] = &Project{Path: path, Name: filepath.Base(path), Available: err == nil, Recent: recent}
	}
	for _, path := range recent {
		add(path, true)
	}
	roots := map[string]string{}
	for _, session := range scan.Sessions {
		if err := ctx.Err(); err != nil {
			return Snapshot{}, err
		}
		if session.Archived {
			continue
		}
		root, ok := roots[session.CWD]
		if !ok {
			root = session.CWD
			if dir, err := Directory(session.CWD); session.CWD != "" && err == nil {
				root = (Projects{}).Root(ctx, dir)
			}
			roots[session.CWD] = root
		}
		if root != "" {
			add(root, false)
			projects[root].Sessions++
		}
		session.Breakdown = nil
		result.Sessions = append(result.Sessions, Session{Session: session, Key: session.Key(), Project: root})
	}
	for _, p := range projects {
		result.Projects = append(result.Projects, *p)
	}
	sort.Slice(result.Projects, func(i, j int) bool {
		a, b := result.Projects[i], result.Projects[j]
		if a.Recent != b.Recent {
			return a.Recent
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Path < b.Path
	})
	for _, p := range s.providers {
		cmd := p.NewCmd("")
		path, err := exec.LookPath(cmd.Args[0])
		result.Tools = append(result.Tools, Tool{Name: string(p.Tool()), Path: path, Available: err == nil})
	}
	s.snapshot, s.loaded = result, true
	return result, nil
}

func (s *Service) Search(ctx context.Context, query string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("session manager is closed")
	}
	if !s.loaded {
		if _, err := s.refresh(ctx); err != nil {
			return nil, err
		}
	}
	matched := map[string]bool{}
	if s.ix != nil && strings.TrimSpace(query) != "" {
		hits, err := s.ix.Search(ctx, query, 0)
		if err != nil {
			return nil, err
		}
		for _, hit := range hits {
			matched[index.Key(hit.Tool, hit.ID)] = true
		}
	}
	keys := []string{}
	for _, item := range s.snapshot.Sessions {
		text := strings.ToLower(strings.Join([]string{item.Title, item.CWD, item.GitBranch, item.Model, strings.Join(item.Tags, " "), string(item.Tool)}, " "))
		metadata := true
		for _, term := range strings.Fields(strings.ToLower(query)) {
			metadata = metadata && strings.Contains(text, term)
		}
		if metadata || matched[item.Key] {
			keys = append(keys, item.Key)
		}
	}
	return keys, nil
}

func (s *Service) find(key string) (model.Session, error) {
	if s.closed {
		return model.Session{}, errors.New("session manager is closed")
	}
	for _, item := range s.snapshot.Sessions {
		if item.Key == key {
			return item.Session, nil
		}
	}
	return model.Session{}, errors.New("session is no longer available; refresh the session list")
}

func (s *Service) Transcript(ctx context.Context, key string) (Preview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.find(key)
	if err != nil {
		return Preview{}, err
	}
	p := provider.For(s.providers, session.Tool)
	if p == nil {
		return Preview{}, errors.New("session provider unavailable")
	}
	messages, err := p.Transcript(ctx, session)
	if err != nil {
		return Preview{}, err
	}
	result := Preview{Messages: []model.Message{}}
	if len(messages) > 200 {
		messages = messages[len(messages)-200:]
		result.Truncated = true
	}
	for _, message := range messages {
		if len(message.Text) > 20000 {
			text := message.Text[:20000]
			for !utf8.ValidString(text) {
				text = text[:len(text)-1]
			}
			message.Text = text + "\n… Preview shortened."
			result.Truncated = true
		}
		result.Messages = append(result.Messages, message)
	}
	return result, nil
}

func (s *Service) NewSession(tool, project string) (*exec.Cmd, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("session manager is closed")
	}
	return NewCommand(s.providers, tool, project)
}

func (s *Service) Resume(ctx context.Context, key string) (*exec.Cmd, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Re-read annotations and transcript metadata before launching a cached selection.
	if _, err := s.refresh(ctx); err != nil {
		return nil, err
	}
	session, err := s.find(key)
	if err != nil {
		return nil, err
	}
	return ResumeCommand(s.providers, session)
}

func (s *Service) readRecent() ([]string, error) {
	if s.dataDir == "" {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(s.dataDir, "desktop-projects.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var paths []string
	err = json.Unmarshal(data, &paths)
	return paths, err
}

func (s *Service) AddProject(ctx context.Context, path string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", errors.New("session manager is closed")
	}
	dir, err := Directory(path)
	if err != nil {
		return "", err
	}
	dir = (Projects{}).Root(ctx, dir)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s.dataDir == "" {
		return "", errors.New("project storage is unavailable")
	}
	previous, err := s.readRecent()
	if err != nil {
		return "", err
	}
	paths := []string{dir}
	for _, p := range previous {
		if p != dir && len(paths) < 12 {
			paths = append(paths, p)
		}
	}
	data, err := json.MarshalIndent(paths, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.dataDir, 0o700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(s.dataDir, ".desktop-projects-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(f.Name(), filepath.Join(s.dataDir, "desktop-projects.json")); err != nil {
		return "", err
	}
	return dir, nil
}
