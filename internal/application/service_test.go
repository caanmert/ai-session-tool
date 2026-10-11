package application

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/caanmert/ai-session-tool/internal/meta"
	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
	"github.com/caanmert/ai-session-tool/internal/provider/claude"
)

func fixtureService(t *testing.T) (*Service, model.Session) {
	t.Helper()
	root := t.TempDir()
	project := filepath.Join(root, "project with spaces")
	transcripts := filepath.Join(root, "claude", "projects", "demo")
	for _, path := range []string{project, transcripts} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	id := "11111111-1111-4111-8111-111111111111"
	rows := []map[string]any{
		{"type": "summary", "summary": "Retry webhook delivery"},
		{"type": "user", "cwd": project, "sessionId": id, "timestamp": "2026-10-04T10:00:00Z", "message": map[string]any{"role": "user", "content": "Handle transient gateway failures with jitter."}},
		{"type": "assistant", "cwd": project, "sessionId": id, "timestamp": "2026-10-04T10:01:00Z", "message": map[string]any{"role": "assistant", "content": []map[string]string{{"type": "text", "text": "Added exponential backoff."}}}},
	}
	var body strings.Builder
	for _, row := range rows {
		data, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		body.Write(data)
		body.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(transcripts, id+".jsonl"), []byte(body.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	p := claude.New(filepath.Join(root, "claude"))
	s := NewService([]provider.Provider{p}, filepath.Join(root, "index.db"), filepath.Join(root, "data"))
	t.Cleanup(func() { _ = s.Close() })
	snapshot, err := s.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Sessions) != 1 {
		t.Fatalf("snapshot: %+v", snapshot)
	}
	return s, snapshot.Sessions[0].Session
}

func TestServiceSearchPreviewAndFreshResume(t *testing.T) {
	s, session := fixtureService(t)
	ctx := context.Background()
	keys, err := s.Search(ctx, "jitter")
	if err != nil || len(keys) != 1 || keys[0] != session.Key() {
		t.Fatalf("body search: %v %v", keys, err)
	}
	preview, err := s.Transcript(ctx, session.Key())
	if err != nil || len(preview.Messages) != 2 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	cmd, err := s.Resume(ctx, session.Key())
	if err != nil || cmd.Dir != session.CWD || cmd.Args[2] != session.ID {
		t.Fatalf("resume: %v %v", cmd, err)
	}
	if _, err := s.Resume(ctx, "codex/"+session.ID); err == nil {
		t.Fatal("cross-tool key accepted")
	}
	store, err := meta.Open(filepath.Join(s.dataDir, "ais.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SetArchived(ctx, session, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resume(ctx, session.Key()); err == nil {
		t.Fatal("resumed session archived after initial snapshot")
	}
	if len(s.snapshot.Sessions) != 0 {
		t.Fatal("archived session still visible")
	}
}

func TestRecentProjectsAndReadOnlyTranscript(t *testing.T) {
	s, session := fixtureService(t)
	before, err := os.ReadFile(session.Path)
	if err != nil {
		t.Fatal(err)
	}
	path, err := s.AddProject(context.Background(), session.CWD)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddProject(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Projects) != 1 || !snapshot.Projects[0].Recent || snapshot.Projects[0].Sessions != 1 {
		t.Fatalf("projects: %+v", snapshot.Projects)
	}
	recent, err := s.readRecent()
	if err != nil || len(recent) != 1 {
		t.Fatalf("recent: %v %v", recent, err)
	}
	after, err := os.ReadFile(session.Path)
	if err != nil || string(before) != string(after) {
		t.Fatal("transcript changed")
	}
	if _, err := s.NewSession("codex", filepath.Join(path, "missing")); err == nil {
		t.Fatal("missing project accepted")
	}
	if _, err := s.NewSession("shell", path); err == nil {
		t.Fatal("unknown tool accepted")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Refresh(context.Background()); err == nil {
		t.Fatal("closed service reopened")
	}
}

type previewProvider struct {
	provider.Provider
	messages []model.Message
}

func (p previewProvider) Transcript(context.Context, model.Session) ([]model.Message, error) {
	return p.messages, nil
}

func TestPreviewLimitsAndConcurrentReads(t *testing.T) {
	s, session := fixtureService(t)
	messages := make([]model.Message, 205)
	for i := range messages {
		messages[i] = model.Message{Role: model.RoleAssistant, Kind: model.KindText, Text: "hello"}
	}
	messages[204].Text = strings.Repeat("€", 7000)
	s.providers[0] = previewProvider{Provider: s.providers[0], messages: messages}
	preview, err := s.Transcript(context.Background(), session.Key())
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Truncated || len(preview.Messages) != 200 || !utf8.ValidString(preview.Messages[199].Text) || len(preview.Messages[199].Text) > 20100 {
		t.Fatal("preview limits not enforced")
	}
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Search(context.Background(), "retry"); err != nil {
				t.Error(err)
			}
			if _, err := s.Transcript(context.Background(), session.Key()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
