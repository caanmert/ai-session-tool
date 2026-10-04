package tui

import (
	"testing"

	"github.com/caanmert/ai-session-tool/internal/model"
)

func TestQuery(t *testing.T) {
	api := model.Session{Tool: model.ToolClaude, ID: "abc123", Title: "Fix auth token refresh", CWD: "/Users/me/code/api", GitBranch: "feat/auth", Model: "claude-opus-5-5"}
	web := model.Session{Tool: model.ToolCodex, ID: "def456", Title: "Dark mode", FirstPrompt: "Add a toggle to the header", CWD: "/Users/me/code/web", GitBranch: "main", Live: &model.LiveState{PID: 1}}

	tests := []struct {
		q        string
		api, web bool
	}{
		{"", true, true},
		{"auth", true, false},
		{"AUTH token", true, false},
		{"auth dark", false, false}, // words AND together
		{"toggle", false, true},     // first prompt is searched
		{"t:codex", false, true},
		{"t:cl", true, false}, // tool prefix
		{"tool:claude refresh", true, false},
		{"p:web", false, true},
		{"b:feat", true, false},
		{"is:live", false, true},
		{"def4", false, true}, // id
		{"opus", true, false}, // model
		{"is:sleepy", false, false},
		{"p:", false, false}, // an empty value is a plain word, matching nothing here
	}
	for _, tt := range tests {
		q := parseQuery(tt.q)
		if got := q.match(api); got != tt.api {
			t.Errorf("%q matches api = %v, want %v", tt.q, got, tt.api)
		}
		if got := q.match(web); got != tt.web {
			t.Errorf("%q matches web = %v, want %v", tt.q, got, tt.web)
		}
	}
	if !parseQuery("  ").empty() || parseQuery("is:live").empty() {
		t.Error("empty()")
	}
}
