package cli

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
	"github.com/caanmert/ai-session-tool/internal/provider/codex"
	"github.com/caanmert/ai-session-tool/internal/trash"
)

type liveProvider struct {
	provider.Provider
	states []model.LiveState
	err    error
}

func (p *liveProvider) Live(context.Context) ([]model.LiveState, error) { return p.states, p.err }

func TestTrashRefreshesLiveStateBeforeMutating(t *testing.T) {
	ctx := context.Background()
	p := &liveProvider{Provider: codex.New(t.TempDir())}
	dir := t.TempDir()
	a := &App{DataDir: func() (string, error) { return dir, nil }, Providers: func() []provider.Provider { return []provider.Provider{p} }}
	tr, err := a.trash()
	if err != nil {
		t.Fatal(err)
	}
	tr.CodexBin = "true"
	calls := 0
	tr.Run = func(*exec.Cmd) ([]byte, error) { calls++; return nil, nil }
	s := model.Session{Tool: model.ToolCodex, ID: "session"} // stale snapshot says idle
	p.states = []model.LiveState{{SessionID: s.ID, PID: 42}}
	if _, err := tr.Put(ctx, s); !errors.Is(err, trash.ErrLive) {
		t.Fatalf("live: %v", err)
	}
	p.states = nil
	p.err = errors.New("process inspection unavailable")
	if _, err := tr.Put(ctx, s); !errors.Is(err, p.err) {
		t.Fatalf("unknown: %v", err)
	}
	if calls != 0 {
		t.Fatal("mutated while live/unknown")
	}
	res, err := provider.Scan(ctx, []provider.Provider{p})
	if err != nil || len(res.Warnings) != 1 {
		t.Fatalf("missing live warning: %+v, %v", res, err)
	}
	p.err = nil
	e, err := tr.Put(ctx, s)
	if err != nil || calls != 1 {
		t.Fatalf("idle archive: %v, calls=%d", err, calls)
	}
	p.states = []model.LiveState{{SessionID: s.ID, PID: 43}}
	if err := tr.Delete(ctx, e); !errors.Is(err, trash.ErrLive) {
		t.Fatalf("live delete: %v", err)
	}
	p.states = nil
	p.err = errors.New("inspection failed")
	if err := tr.Delete(ctx, e); !errors.Is(err, p.err) {
		t.Fatalf("unknown delete: %v", err)
	}
	if calls != 1 {
		t.Fatal("delete ran while live/unknown")
	}
}
