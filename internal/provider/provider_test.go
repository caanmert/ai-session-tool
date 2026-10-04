package provider_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
)

// fake is an in-memory provider.
type fake struct {
	tool     model.Tool
	sessions map[string]model.Session // by path
	empty    []string
	live     []model.LiveState
}

func (f *fake) Tool() model.Tool { return f.tool }
func (f *fake) Root() string     { return "/fake" }
func (f *fake) Discover(context.Context) ([]provider.FileRef, error) {
	var refs []provider.FileRef
	for p := range f.sessions {
		refs = append(refs, provider.FileRef{Path: p})
	}
	for _, p := range f.empty {
		refs = append(refs, provider.FileRef{Path: p})
	}
	return refs, nil
}
func (f *fake) Parse(_ context.Context, r provider.FileRef) (model.Session, []provider.Warning, error) {
	s, ok := f.sessions[r.Path]
	if !ok {
		return model.Session{}, []provider.Warning{{Path: r.Path, Line: 1, Msg: "bad"}}, provider.ErrEmpty
	}
	return s, nil, nil
}
func (f *fake) Transcript(context.Context, model.Session) ([]model.Message, error) { return nil, nil }
func (f *fake) Live(context.Context) ([]model.LiveState, error)                    { return f.live, nil }
func (f *fake) ResumeCmd(model.Session, bool) (*exec.Cmd, error)                   { return nil, nil }
func (f *fake) NewCmd(string) *exec.Cmd                                            { return nil }

func at(h int) time.Time { return time.Date(2026, 10, 1, h, 0, 0, 0, time.UTC) }

func TestScanSortsAttachesLiveAndCountsEmpty(t *testing.T) {
	claude := &fake{
		tool: model.ToolClaude,
		sessions: map[string]model.Session{
			"/a": {Tool: model.ToolClaude, ID: "aaa", UpdatedAt: at(1)},
			"/b": {Tool: model.ToolClaude, ID: "bbb", UpdatedAt: at(3)},
		},
		empty: []string{"/empty"},
		live:  []model.LiveState{{PID: 7, SessionID: "aaa", Status: "idle"}},
	}
	codex := &fake{
		tool: model.ToolCodex,
		sessions: map[string]model.Session{
			"/c": {Tool: model.ToolCodex, ID: "aaa", UpdatedAt: at(2)}, // same id, other tool: not live
		},
	}
	res, err := provider.Scan(context.Background(), []provider.Provider{claude, codex})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, s := range res.Sessions {
		order = append(order, string(s.Tool)+":"+s.ID)
	}
	if got := strings.Join(order, " "); got != "claude:bbb codex:aaa claude:aaa" {
		t.Fatalf("order = %s", got)
	}
	if res.Sessions[2].Live == nil || res.Sessions[2].Live.PID != 7 {
		t.Errorf("claude:aaa should be live")
	}
	if res.Sessions[1].Live != nil {
		t.Errorf("codex:aaa must not pick up claude's live state")
	}
	if res.Empty != 1 || len(res.Warnings) != 1 {
		t.Errorf("Empty = %d, Warnings = %v", res.Empty, res.Warnings)
	}
}

func TestFind(t *testing.T) {
	sessions := []model.Session{
		{ID: "abc123", Title: "one"},
		{ID: "abd456", Title: "two"},
		{ID: "ab", Title: "exact"},
	}
	if s, err := provider.Find(sessions, "ABC"); err != nil || s.Title != "one" {
		t.Errorf("Find(ABC) = %v, %v", s.Title, err)
	}
	if s, err := provider.Find(sessions, "ab"); err != nil || s.Title != "exact" {
		t.Errorf("exact match should win: %v, %v", s.Title, err)
	}
	if _, err := provider.Find(sessions, "a"); err == nil || !strings.Contains(err.Error(), "matches 3 sessions") {
		t.Errorf("Find(a) err = %v, want ambiguity", err)
	}
	if _, err := provider.Find(sessions, "zzz"); err == nil {
		t.Error("Find(zzz) should fail")
	}
}
