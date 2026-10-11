package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (m *chatManager) runCodex(ctx context.Context, c *chatSession, state ChatState, text string) (string, error) {
	settings := state.Settings
	if settings.Configured {
		project := state.Project
		if state.Review != nil {
			project = ""
		}
		catalog, err := m.models(ctx, "codex", project, false)
		if err != nil {
			return "", err
		}
		if err = catalog.validate(settings); err != nil {
			return "", err
		}
		if settings.Model == "" {
			settings.Model = catalog.DefaultModel
		}
		if settings.Model == "" {
			return "", errors.New("Codex did not report its default model; choose a model explicitly")
		}
		if settings.Effort == "" {
			option, _ := catalog.option(settings.Model)
			settings.Effort = option.DefaultEffort
			for _, effort := range option.Efforts {
				if effort == catalog.DefaultEffort {
					settings.Effort = effort
				}
			}
		}
	}
	cmd := m.command("codex", "app-server", "--listen", "stdio://")
	cmd.Dir = state.Project
	p, err := startJSONProcess(ctx, cmd)
	if err != nil {
		return "", err
	}
	defer p.close()
	rpc := func(id int, method string, params any) error {
		return p.send(map[string]any{"id": id, "method": method, "params": params})
	}
	if err = rpc(1, "initialize", map[string]any{"clientInfo": map[string]string{"name": "ais_desktop", "title": "ais", "version": "0.1.0"}, "capabilities": map[string]any{"experimentalApi": false}}); err != nil {
		return "", err
	}
	timer := time.NewTimer(60 * time.Second)
	defer timer.Stop()
	thread := state.SessionID
	turn := ""
	final := ""
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timer.C:
			return "", errors.New("Codex initialization timed out; check its installation and sign-in")
		case packet, ok := <-p.packets:
			if !ok {
				return "", p.failure()
			}
			if packet.Err != nil {
				return "", packet.Err
			}
			v := packet.Value
			method := str(v["method"])
			if method == "" {
				if raw := v["error"]; len(raw) > 0 && string(raw) != "null" {
					return "", fmt.Errorf("Codex: %s", pretty(raw))
				}
				result := object(v["result"])
				switch number(v["id"]) {
				case 1:
					if err = p.send(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
						return "", err
					}
					params := map[string]any{"cwd": state.Project, "approvalPolicy": "on-request", "approvalsReviewer": "user", "sandbox": "workspace-write"}
					if state.Review != nil {
						params["sandbox"] = "read-only"
						params["approvalPolicy"] = "never"
					}
					if settings.Configured {
						params["model"] = settings.Model
						if settings.Effort != "" {
							params["config"] = map[string]any{"model_reasoning_effort": settings.Effort}
						}
					}
					operation := "thread/start"
					if thread != "" {
						operation = "thread/resume"
						params["threadId"] = thread
					}
					if err = rpc(2, operation, params); err != nil {
						return "", err
					}
				case 2:
					thread = str(object(result["thread"])["id"])
					if thread == "" {
						return "", errors.New("Codex did not return a conversation ID")
					}
					c.sessionID(thread)
					c.reportModel(str(result["model"]))
					m.saveOrWarn(c)
					params := map[string]any{"threadId": thread, "input": []any{map[string]any{"type": "text", "text": text}}}
					if settings.Configured {
						params["model"] = settings.Model
						if settings.Effort != "" {
							params["effort"] = settings.Effort
						}
					}
					if state.Review != nil {
						params["outputSchema"] = reviewSchema()
					}
					if err = rpc(3, "turn/start", params); err != nil {
						return "", err
					}
				case 3:
					turn = str(object(result["turn"])["id"])
					timer.Stop()
				}
				continue
			}
			params := object(v["params"])
			if len(v["id"]) > 0 {
				go codexRequest(ctx, c, p, v["id"], method, params, state.Review != nil)
				continue
			}
			if id := str(params["threadId"]); id != "" && thread != "" && id != thread {
				continue
			}
			switch method {
			case "model/rerouted":
				c.reportModel(str(params["toModel"]))
			case "turn/started":
				turn = str(object(params["turn"])["id"])
				timer.Stop()
			case "item/agentMessage/delta":
				c.message(ChatMessage{ID: str(params["itemId"]), Role: "assistant", Kind: "text", Text: str(params["delta"])}, true)
			case "item/commandExecution/outputDelta":
				c.message(ChatMessage{ID: str(params["itemId"]), Role: "assistant", Kind: "tool", Text: str(params["delta"])}, true)
			case "item/started", "item/completed":
				item := object(params["item"])
				id := str(item["id"])
				kind := str(item["type"])
				status := "running"
				if method == "item/completed" {
					status = str(item["status"])
					if status == "" {
						status = "completed"
					}
				}
				switch kind {
				case "agentMessage":
					body := str(item["text"])
					if body != "" {
						c.message(ChatMessage{ID: id, Role: "assistant", Kind: "text", Text: body}, false)
						if method == "item/completed" {
							final = body
						}
					}
				case "commandExecution":
					c.message(ChatMessage{ID: id, Role: "assistant", Kind: "tool", Name: str(item["command"]), Text: str(item["aggregatedOutput"]), Status: status}, false)
				case "fileChange":
					c.message(ChatMessage{ID: id, Role: "assistant", Kind: "tool", Name: "File changes", Text: pretty(item["changes"]), Status: status}, false)
				case "mcpToolCall", "dynamicToolCall", "webSearch":
					c.message(ChatMessage{ID: id, Role: "assistant", Kind: "tool", Name: kind, Text: pretty(params["item"]), Status: status}, false)
				}
			case "serverRequest/resolved":
				c.dismissApproval(string(params["requestId"]))
			case "turn/completed":
				result := object(params["turn"])
				if id := str(result["id"]); turn != "" && id != "" && id != turn {
					continue
				}
				if str(result["status"]) == "failed" {
					return "", fmt.Errorf("Codex turn failed: %s", pretty(result["error"]))
				}
				if str(result["status"]) == "interrupted" {
					return "", errors.New("Codex response interrupted")
				}
				return final, nil
			case "error":
				if !boolean(params["willRetry"]) {
					return "", fmt.Errorf("Codex: %s", pretty(params["error"]))
				}
			}
		}
	}
}
func codexRequest(ctx context.Context, c *chatSession, p *jsonProcess, id json.RawMessage, method string, params jsonObject, review bool) {
	send := func(result any) {
		if err := p.send(map[string]any{"id": id, "result": result}); err != nil {
			p.close()
		}
	}
	view := ChatApproval{ID: string(id), Title: "Agent approval", Detail: pretty(mustJSON(params)), Questions: []ChatQuestion{}}
	switch method {
	case "item/commandExecution/requestApproval":
		view.Title = "Run command?"
	case "item/fileChange/requestApproval":
		view.Title = "Allow file changes?"
	case "item/tool/requestUserInput":
		view.Title = "The agent needs your input"
		for _, raw := range array(params["questions"]) {
			q := object(raw)
			options := []string{}
			for _, o := range array(q["options"]) {
				options = append(options, str(object(o)["label"]))
			}
			view.Questions = append(view.Questions, ChatQuestion{ID: str(q["id"]), Text: str(q["question"]), Options: options, Secret: boolean(q["isSecret"])})
		}
	default:
		_ = p.send(map[string]any{"id": id, "error": map[string]any{"code": -32601, "message": "ais does not support this request; no permission granted"}})
		c.message(ChatMessage{ID: "unsupported/" + string(id), Role: "system", Kind: "text", Text: "Unsupported Codex request: " + method + ". Update ais or use the agent CLI for this action."}, false)
		return
	}
	if review || len(view.Detail) > 50000 {
		if method == "item/tool/requestUserInput" {
			send(map[string]any{"answers": map[string]any{}})
		} else {
			send(map[string]any{"decision": "decline"})
		}
		return
	}
	reply, err := c.ask(ctx, view)
	if err != nil {
		return
	}
	if method == "item/tool/requestUserInput" {
		answers := map[string]any{}
		for _, q := range view.Questions {
			value := []string{}
			if reply.Allow {
				value = []string{reply.Answers[q.ID]}
			}
			answers[q.ID] = map[string]any{"answers": value}
		}
		send(map[string]any{"answers": answers})
		return
	}
	decision := "decline"
	if reply.Allow {
		decision = "accept"
	}
	// Only one-time decisions are supported. Never silently broaden a rule or session.
	if choices := array(params["availableDecisions"]); len(choices) > 0 {
		found := false
		for _, choice := range choices {
			found = found || str(choice) == decision
		}
		if !found {
			decision = "decline"
			c.message(ChatMessage{ID: "decision/" + string(id), Role: "system", Kind: "text", Text: "This request does not offer a one-time approval; it was declined."}, false)
		}
	}
	send(map[string]any{"decision": decision})
}
func mustJSON(value any) json.RawMessage { data, _ := json.Marshal(value); return data }
func (c *chatSession) dismissApproval(id string) {
	c.mu.Lock()
	if c.dismissed == nil {
		c.dismissed = map[string]bool{}
	}
	c.dismissed[id] = true
	p := c.pending[id]
	if p != nil && p.Cancel != nil {
		p.Cancel()
	}
	c.mu.Unlock()
}

// textOnly renders provider blocks without interpreting their contents as UI code.
func textOnly(raw json.RawMessage) string {
	if s := str(raw); s != "" {
		return s
	}
	parts := []string{}
	for _, block := range array(raw) {
		b := object(block)
		if text := str(b["text"]); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}
