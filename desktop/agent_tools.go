package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// An agent can use tools from MCP servers configured for it. On request, ais
// asks the conversation's agent which servers it has in the project and
// whether they work: Claude Code with `claude mcp list`, Codex with its app
// server's mcpServerStatus/list. Checking starts the servers, as a turn
// would, so it runs only when asked. Only names and states are shown, never a
// server's command, URL or arguments, which can hold secrets, and nothing is
// changed.

// AgentServer is one MCP server as the agent reports it.
type AgentServer struct {
	Name string `json:"name"`
	// State is connected, failed, needs-auth, pending, off or unknown.
	State string `json:"state"`
	// Detail is the agent's own words for the state, or its error.
	Detail string `json:"detail,omitempty"`
	// Tools names the server's tools when the agent lists them (Codex).
	Tools []string `json:"tools,omitempty"`
	// More counts tools left out of Tools.
	More int `json:"more,omitempty"`
}

// AgentTools is the agent's MCP servers for a conversation's project.
type AgentTools struct {
	Tool    string        `json:"tool"`
	Servers []AgentServer `json:"servers"`
}

const (
	agentToolsTimeout = 90 * time.Second
	agentToolsOutput  = 1 << 20
	serverToolLimit   = 200
)

// claudeServerState reads a status from `claude mcp list`, such as
// "✔ Connected" or "! Needs authentication".
func claudeServerState(status string) (state, detail string) {
	detail = strings.TrimSpace(strings.TrimLeft(status, "✔✓✗✘!⏸⚠ \t"))
	lower := strings.ToLower(detail)
	switch {
	case strings.Contains(lower, "connected") && !strings.Contains(lower, "failed"):
		return "connected", detail
	case strings.Contains(lower, "auth"):
		return "needs-auth", detail
	case strings.Contains(lower, "pending"):
		return "pending", detail
	case strings.Contains(lower, "failed") || strings.Contains(lower, "error"):
		return "failed", detail
	case strings.Contains(lower, "disabled") || strings.Contains(lower, "not configured"):
		return "off", detail
	}
	return "unknown", detail
}

// parseClaudeServers reads `claude mcp list`. Each server is a line
// "<name>: <command or URL> - <status>". A name can hold spaces and colons
// ("plugin:engineering:google calendar"), but not ": ", and a status follows
// the last " - ". The command or URL in between is dropped.
func parseClaudeServers(out string) []AgentServer {
	servers := []AgentServer{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		name, rest, ok := strings.Cut(line, ": ")
		if !ok || name == "" {
			continue
		}
		dash := strings.LastIndex(rest, " - ")
		if dash < 0 {
			continue
		}
		state, detail := claudeServerState(rest[dash+3:])
		servers = append(servers, AgentServer{Name: name, State: state, Detail: detail})
	}
	return servers
}

// codexServer reads one entry of mcpServerStatus/list.
func codexServer(entry jsonObject) AgentServer {
	server := AgentServer{Name: str(entry["name"])}
	tools := []string{}
	for name := range object(entry["tools"]) {
		tools = append(tools, name)
	}
	sort.Strings(tools)
	if len(tools) > serverToolLimit {
		server.More = len(tools) - serverToolLimit
		tools = tools[:serverToolLimit]
	}
	server.Tools = tools
	runtime := str(entry["runtimeStatus"])
	switch {
	case str(entry["toolsError"]) != "":
		server.State, server.Detail = "failed", shorten(str(entry["toolsError"]), 300)
	case runtime == "failed" || runtime == "cancelled":
		server.State, server.Detail = "failed", "Failed to start"
	case runtime == "authenticationRequired" || str(entry["authStatus"]) == "notLoggedIn":
		server.State, server.Detail = "needs-auth", "Needs sign-in"
	case runtime == "disabled":
		server.State, server.Detail = "off", "Disabled"
	case len(tools) > 0 || runtime == "connected":
		server.State = "connected"
		server.Detail = fmt.Sprintf("%d tools", len(tools)+server.More)
		if len(tools)+server.More == 1 {
			server.Detail = "1 tool"
		}
	default:
		server.State, server.Detail = "unknown", "No tools reported"
	}
	return server
}

// codexServers asks Codex's app server for its MCP servers, page by page.
func (m *chatManager) codexServers(ctx context.Context, project string) ([]AgentServer, error) {
	cmd := m.command("codex", "app-server", "--listen", "stdio://")
	cmd.Dir = project
	p, err := startJSONProcess(ctx, cmd)
	if err != nil {
		return nil, err
	}
	defer p.close()
	if err = p.send(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]string{"name": "ais_agent_tools", "version": "0.1.0"}}}); err != nil {
		return nil, err
	}
	servers := []AgentServer{}
	list := func(id int, cursor string) error {
		params := map[string]any{"detail": "toolsAndAuthOnly", "limit": 100}
		if cursor != "" {
			params["cursor"] = cursor
		}
		return p.send(map[string]any{"id": id, "method": "mcpServerStatus/list", "params": params})
	}
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("Codex did not list its MCP servers in time: %w", ctx.Err())
		case packet, ok := <-p.packets:
			if !ok {
				return nil, p.failure()
			}
			if packet.Err != nil {
				return nil, packet.Err
			}
			v := packet.Value
			if str(v["method"]) != "" {
				continue
			}
			if raw := v["error"]; len(raw) > 0 && string(raw) != "null" {
				return nil, codexError("Codex", raw)
			}
			id := number(v["id"])
			if id == 1 {
				if err = p.send(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
					return nil, err
				}
				if err = list(2, ""); err != nil {
					return nil, err
				}
				continue
			}
			result := object(v["result"])
			for _, raw := range array(result["data"]) {
				servers = append(servers, codexServer(object(raw)))
			}
			// Follow a few pages; a longer list is cut short.
			if next := str(result["nextCursor"]); next != "" && id < 6 {
				if err = list(id+1, next); err != nil {
					return nil, err
				}
				continue
			}
			return servers, nil
		}
	}
}

// claudeServers runs `claude mcp list` in the project.
func (m *chatManager) claudeServers(ctx context.Context, project string) ([]AgentServer, error) {
	cmd := m.command("claude", "mcp", "list")
	cmd.Dir = project
	var out bytes.Buffer
	cmd.Stdout = &boundedWriter{buf: &out, limit: agentToolsOutput}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			detail := strings.TrimSpace(out.String())
			if detail == "" {
				detail = err.Error()
			}
			return nil, fmt.Errorf("Claude Code could not list its MCP servers: %s", shorten(detail, 300))
		}
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-done
		return nil, fmt.Errorf("Claude Code did not list its MCP servers in time: %w", ctx.Err())
	}
	return parseClaudeServers(out.String()), nil
}

// boundedWriter keeps the first limit bytes written and drops the rest.
type boundedWriter struct {
	buf   *bytes.Buffer
	limit int
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	if room := w.limit - w.buf.Len(); room > 0 {
		w.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

// agentTools asks a conversation's agent for its MCP servers in the project.
func (m *chatManager) agentTools(ctx context.Context, id string) (AgentTools, error) {
	c, err := m.get(id)
	if err != nil {
		return AgentTools{}, err
	}
	c.mu.Lock()
	tool, project, review := c.state.Tool, c.state.Project, c.state.Review != nil
	c.mu.Unlock()
	if review {
		return AgentTools{}, errors.New("a review runs without tools")
	}
	ctx, cancel := context.WithTimeout(ctx, agentToolsTimeout)
	defer cancel()
	var servers []AgentServer
	if tool == "codex" {
		servers, err = m.codexServers(ctx, project)
	} else {
		servers, err = m.claudeServers(ctx, project)
	}
	if err != nil {
		return AgentTools{}, err
	}
	sort.SliceStable(servers, func(i, j int) bool { return strings.ToLower(servers[i].Name) < strings.ToLower(servers[j].Name) })
	return AgentTools{Tool: tool, Servers: servers}, nil
}

// AgentTools asks the conversation's agent which MCP servers it has in the
// project and whether they work. It starts the servers, as a turn would, and
// changes nothing.
func (a *App) AgentTools(chatID string) (AgentTools, error) {
	if err := a.chatReady(); err != nil {
		return AgentTools{}, err
	}
	return a.chats.agentTools(a.ctx, chatID)
}
