package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// runCodex runs one turn. A compacting turn asks Codex to summarize the
// conversation instead of sending a message.
func (m *chatManager) runCodex(ctx context.Context, c *chatSession, state ChatState, text string, images []savedImage, compact bool) (string, error) {
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
	// A conversation another Codex program has open would be refused; say so
	// without starting Codex.
	if state.SessionID != "" {
		if holder := codexHolder(ctx, state.SessionID); holder != "" {
			return "", openElsewhereIn(holder)
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
					return "", codexError("Codex", raw)
				}
				result := object(v["result"])
				switch number(v["id"]) {
				case 1:
					if err = p.send(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
						return "", err
					}
					params := map[string]any{"cwd": state.Project, "approvalPolicy": "on-request", "approvalsReviewer": "user", "sandbox": codexSandbox(state.Mode)}
					if state.Review != nil {
						params["sandbox"] = "read-only"
						params["approvalPolicy"] = "never"
					}
					config := map[string]any{}
					if settings.Configured {
						params["model"] = settings.Model
						if settings.Effort != "" {
							config["model_reasoning_effort"] = settings.Effort
						}
					}
					if state.WebSearch != "" && state.Review == nil {
						config["web_search"] = state.WebSearch
					}
					if len(config) > 0 {
						params["config"] = config
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
					if compact {
						if err = rpc(3, "thread/compact/start", map[string]any{"threadId": thread}); err != nil {
							return "", err
						}
						continue
					}
					// Codex reads attached images from the files saved with the conversation.
					input := []any{map[string]any{"type": "text", "text": text}}
					for _, image := range images {
						input = append(input, map[string]any{"type": "localImage", "path": image.Path})
					}
					// "$name" of one of the project's skills uses that skill.
					input = append(input, m.codexSkillInputs(ctx, state.Project, text)...)
					params := map[string]any{"threadId": thread, "input": input}
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
			case "thread/tokenUsage/updated":
				c.reportContext(codexContextUsage(params))
				c.reportCodexTokens(params)
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
				case "contextCompaction":
					if method == "item/completed" {
						c.compacted(id, 0)
					}
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
					return "", codexError("Codex turn failed", result["error"])
				}
				if str(result["status"]) == "interrupted" {
					return "", errors.New("Codex response interrupted")
				}
				return final, nil
			case "error":
				if !boolean(params["willRetry"]) {
					return "", codexError("Codex", params["error"])
				}
			}
		}
	}
}

// errOpenElsewhere is Codex refusing a conversation another program has open.
var errOpenElsewhere = errors.New("This Codex conversation is open in another Codex program, such as the Codex app or codex in a terminal, and Codex lets only one program write to a conversation at a time. Close it there, then send your message again")

// codexError reads an error Codex reported: its message in plain words rather
// than the JSON around it.
func codexError(prefix string, raw json.RawMessage) error {
	var reported struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &reported) != nil || strings.TrimSpace(reported.Message) == "" {
		return fmt.Errorf("%s: %s", prefix, pretty(raw))
	}
	if strings.Contains(reported.Message, "already has an active writer") {
		return errOpenElsewhere
	}
	return fmt.Errorf("%s: %s", prefix, strings.TrimSpace(reported.Message))
}

func codexRequest(ctx context.Context, c *chatSession, p *jsonProcess, id json.RawMessage, method string, params jsonObject, review bool) {
	send := func(result any) {
		if err := p.send(map[string]any{"id": id, "result": result}); err != nil {
			p.close()
		}
	}
	if observeApproval != nil {
		observeApproval("codex", params)
	}
	view := ChatApproval{ID: string(id), Title: "Agent approval", Detail: pretty(mustJSON(params)), Questions: []ChatQuestion{}}
	// Codex lists the decisions it accepts for this request. Offer the wider
	// ones it lists, and remember each exactly as given so the chosen one can
	// be sent back unchanged.
	wider := map[string]any{}
	if method != "item/tool/requestUserInput" {
		view.Options, wider = codexOptions(params["availableDecisions"])
	}
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
	if chosen, ok := wider[reply.Option]; reply.Allow && ok {
		send(map[string]any{"decision": chosen})
		return
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

const (
	codexSessionOption = "acceptForSession"
	codexCommandOption = "acceptWithExecpolicyAmendment"
)

// codexOptions turns the decisions Codex lists into the wider choices ais
// shows, with the value to send back for each. Unknown decisions are ignored,
// so nothing is offered that ais cannot describe.
func codexOptions(raw json.RawMessage) ([]ApprovalOption, map[string]any) {
	options, values := []ApprovalOption{}, map[string]any{}
	for _, choice := range array(raw) {
		if str(choice) == codexSessionOption {
			options = append(options, ApprovalOption{ID: codexSessionOption, Label: "Allow for this session", Detail: "Codex will not ask again for this in the current session."})
			values[codexSessionOption] = codexSessionOption
			continue
		}
		amendment, ok := object(choice)[codexCommandOption]
		if !ok {
			continue
		}
		words := []string{}
		for _, word := range array(object(amendment)["execpolicy_amendment"]) {
			words = append(words, str(word))
		}
		if len(words) == 0 || strings.TrimSpace(strings.Join(words, "")) == "" {
			continue
		}
		options = append(options, ApprovalOption{ID: codexCommandOption, Label: "Always allow this command", Detail: "Codex will add a rule to its own settings and stop asking for commands that start with: " + strings.Join(words, " ")})
		values[codexCommandOption] = choice
	}
	if len(options) == 0 {
		return nil, values
	}
	return options, values
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
