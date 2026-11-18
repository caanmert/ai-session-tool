package main

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestAgentToolsFromClaudeCode(t *testing.T) {
	m := fakeChats(t, "tools")
	state := createChat(t, m, t.TempDir(), "claude", "")
	got, err := m.agentTools(context.Background(), state.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []AgentServer{
		{Name: "claude.ai Google Drive", State: "needs-auth", Detail: "Needs authentication"},
		{Name: "github", State: "connected", Detail: "Connected"},
		{Name: "local-db", State: "failed", Detail: "Failed to connect"},
		{Name: "plugin:engineering:google calendar", State: "off", Detail: "Not configured"},
		{Name: "shared", State: "pending", Detail: "Pending approval"},
	}
	if got.Tool != "claude" || !reflect.DeepEqual(got.Servers, want) {
		t.Fatalf("servers: %+v", got.Servers)
	}
	for _, server := range got.Servers {
		if strings.Contains(server.Name+server.Detail, "SECRET") || strings.Contains(server.Name+server.Detail, "http") {
			t.Fatalf("a command or URL leaked: %+v", server)
		}
	}
}

func TestAgentToolsFromCodex(t *testing.T) {
	m := fakeChats(t, "tools")
	state := createChat(t, m, t.TempDir(), "codex", "")
	got, err := m.agentTools(context.Background(), state.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []AgentServer{
		{Name: "broken", State: "failed", Detail: "spawn db-mcp: not found", Tools: []string{}},
		{Name: "linear", State: "needs-auth", Detail: "Needs sign-in", Tools: []string{}},
		{Name: "node_repl", State: "connected", Detail: "2 tools", Tools: []string{"eval", "js"}},
	}
	if got.Tool != "codex" || !reflect.DeepEqual(got.Servers, want) {
		t.Fatalf("servers: %+v", got.Servers)
	}
}

func TestAgentServerStates(t *testing.T) {
	if got := parseClaudeServers("No MCP servers configured. Use `claude mcp add` to add a server.\n"); len(got) != 0 {
		t.Fatalf("no servers: %+v", got)
	}
	many := map[string]any{}
	for i := 0; i < serverToolLimit+5; i++ {
		many[strings.Repeat("t", 1+i%7)+string(rune('a'+i%26))+strings.Repeat("x", i/26)] = map[string]any{}
	}
	server := codexServer(jsonObject{"name": mustJSON("big"), "tools": mustJSON(many)})
	if len(server.Tools) != serverToolLimit || server.More != 5 || server.Detail != "205 tools" {
		t.Fatalf("many tools: %d %d %q", len(server.Tools), server.More, server.Detail)
	}
	if s := codexServer(jsonObject{"name": mustJSON("idle")}); s.State != "unknown" || s.Detail != "No tools reported" {
		t.Fatalf("idle: %+v", s)
	}
	if s := codexServer(jsonObject{"name": mustJSON("off"), "runtimeStatus": mustJSON("disabled")}); s.State != "off" {
		t.Fatalf("disabled: %+v", s)
	}
	m := newChats(t.TempDir())
	t.Cleanup(m.close)
	review := createChat(t, m, t.TempDir(), "codex", "")
	c, _ := m.get(review.ID)
	c.mu.Lock()
	c.state.Review = &ReviewState{}
	c.mu.Unlock()
	if _, err := m.agentTools(context.Background(), review.ID); err == nil {
		t.Fatal("a review has no tools to check")
	}
}
