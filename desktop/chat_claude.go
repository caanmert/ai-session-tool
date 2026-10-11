package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
)

func (m *chatManager) runClaude(ctx context.Context, c *chatSession, state ChatState, text string) (string, error) {
	args := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--include-partial-messages", "--permission-prompt-tool", "stdio", "--permission-mode", "default"}
	if state.Settings.Configured {
		project := state.Project
		if state.Review != nil {
			project = ""
		}
		if state.Settings.Model != "" || state.Settings.Effort != "" {
			catalog, err := m.models(ctx, "claude", project, false)
			if err != nil {
				return "", err
			}
			if err = catalog.validate(state.Settings); err != nil {
				return "", err
			}
		}
		model := state.Settings.Model
		if model == "" {
			model = "default"
		}
		args = append(args, "--model", model)
		if state.Settings.Effort != "" {
			args = append(args, "--effort", state.Settings.Effort)
		}
	}
	sessionID := state.SessionID
	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	} else {
		sessionID = uuid.NewString()
		args = append(args, "--session-id", sessionID)
	}
	if state.Review != nil {
		schema, _ := json.Marshal(reviewSchema())
		args = append(args, "--tools", "", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--disable-slash-commands", "--json-schema", string(schema))
	}
	cmd := m.command("claude", args...)
	cmd.Dir = state.Project
	cmd.Env = append(os.Environ(), "CLAUDE_CODE_SDK_READS_SESSION_STATE=1")
	p, err := startJSONProcess(ctx, cmd)
	if err != nil {
		return "", err
	}
	defer p.close()
	if err = p.send(map[string]any{"type": "control_request", "request_id": "ais-initialize", "request": map[string]any{"subtype": "initialize"}}); err != nil {
		return "", err
	}
	timer := time.NewTimer(60 * time.Second)
	defer timer.Stop()
	messageID := ""
	final := ""
	visibleText := false
	resultReceived := false
	sessionState := ""
	started := false
	tools := map[string]string{}
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timer.C:
			return "", errors.New("Claude initialization timed out; check its installation and sign-in")
		case packet, ok := <-p.packets:
			if !ok {
				return "", p.failure()
			}
			if packet.Err != nil {
				return "", packet.Err
			}
			v := packet.Value
			switch str(v["type"]) {
			case "control_response":
				response := object(v["response"])
				if str(response["request_id"]) != "ais-initialize" {
					continue
				}
				if str(response["subtype"]) == "error" {
					return "", fmt.Errorf("Claude initialization: %s", str(response["error"]))
				}
				if !started {
					started = true
					c.sessionID(sessionID)
					m.saveOrWarn(c)
					if err = p.send(map[string]any{"type": "user", "session_id": sessionID, "parent_tool_use_id": nil, "message": map[string]any{"role": "user", "content": text}}); err != nil {
						return "", err
					}
					timer.Stop()
				}
			case "control_request":
				go claudeRequest(ctx, c, p, v, state.Review != nil)
			case "control_cancel_request":
				c.dismissApproval(str(v["request_id"]))
			case "system":
				if str(v["subtype"]) == "init" {
					c.reportModel(str(v["model"]))
				}
				if id := str(v["session_id"]); id != "" {
					c.sessionID(id)
				}
				if str(v["subtype"]) == "session_state_changed" {
					sessionState = str(v["state"])
					if sessionState == "idle" && resultReceived {
						return final, nil
					}
				}
			case "stream_event":
				event := object(v["event"])
				switch str(event["type"]) {
				case "message_start":
					if str(v["parent_tool_use_id"]) == "" {
						c.reportModel(str(object(event["message"])["model"]))
					}
					messageID = str(object(event["message"])["id"])
					resultReceived = false
				case "content_block_start":
					block := object(event["content_block"])
					id := messageID + "/" + strconv.Itoa(number(event["index"]))
					if str(block["type"]) == "tool_use" {
						tools[str(block["id"])] = id
						c.message(ChatMessage{ID: id, Role: "assistant", Kind: "tool", Name: str(block["name"]), Status: "running"}, false)
					}
				case "content_block_delta":
					id := messageID + "/" + strconv.Itoa(number(event["index"]))
					delta := object(event["delta"])
					switch str(delta["type"]) {
					case "text_delta":
						visibleText = true
						c.message(ChatMessage{ID: id, Role: "assistant", Kind: "text", Text: str(delta["text"])}, true)
					case "input_json_delta":
						c.message(ChatMessage{ID: id, Role: "assistant", Kind: "tool", Text: str(delta["partial_json"]), Status: "running"}, true)
					}
				}
			case "assistant":
				message := object(v["message"])
				if str(v["parent_tool_use_id"]) == "" {
					c.reportModel(str(message["model"]))
				}
				id := str(message["id"])
				if id == "" {
					id = uuid.NewString()
				}
				// Nested-agent output has its own message IDs; it cannot replace main text.
				for i, raw := range array(message["content"]) {
					block := object(raw)
					key := id + "/" + strconv.Itoa(i)
					switch str(block["type"]) {
					case "text":
						visibleText = true
						body := str(block["text"])
						c.message(ChatMessage{ID: key, Role: "assistant", Kind: "text", Text: body, Model: str(message["model"])}, false)
						if str(v["parent_tool_use_id"]) == "" {
							final = body
						}
					case "tool_use":
						tools[str(block["id"])] = key
						c.message(ChatMessage{ID: key, Role: "assistant", Kind: "tool", Name: str(block["name"]), Text: pretty(block["input"]), Status: "running"}, false)
					}
				}
			case "user":
				for _, raw := range array(object(v["message"])["content"]) {
					block := object(raw)
					if str(block["type"]) == "tool_result" {
						id := tools[str(block["tool_use_id"])]
						if id == "" {
							id = "tool/" + str(block["tool_use_id"])
						}
						status := "completed"
						if boolean(block["is_error"]) {
							status = "failed"
						}
						c.message(ChatMessage{ID: id, Role: "assistant", Kind: "tool", Text: "\n\n" + textOnly(block["content"]), Status: status}, true)
					}
				}
			case "result":
				if boolean(v["is_error"]) {
					return "", fmt.Errorf("Claude: %s %s", str(v["result"]), pretty(v["errors"]))
				}
				if raw := v["structured_output"]; len(raw) > 0 && string(raw) != "null" {
					final = string(raw)
				} else if body := str(v["result"]); body != "" {
					final = body
				}
				if !visibleText && final != "" {
					c.message(ChatMessage{ID: uuid.NewString(), Role: "assistant", Kind: "text", Text: final}, false)
					visibleText = true
				}
				resultReceived = true
				if sessionState == "" || sessionState == "idle" {
					return final, nil
				}
			}
		}
	}
}
func claudeRequest(ctx context.Context, c *chatSession, p *jsonProcess, v jsonObject, review bool) {
	id := str(v["request_id"])
	request := object(v["request"])
	respond := func(data any) {
		if err := p.send(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": id, "response": data}}); err != nil {
			p.close()
		}
	}
	if str(request["subtype"]) != "can_use_tool" {
		_ = p.send(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "error", "request_id": id, "error": "Unsupported control request"}})
		return
	}
	if review {
		respond(map[string]any{"behavior": "deny", "message": "This action is not available in this ais session."})
		return
	}
	name := str(request["tool_name"])
	input := object(request["input"])
	view := ChatApproval{ID: id, Title: "Allow " + name + "?", Detail: pretty(request["input"]), Questions: []ChatQuestion{}}
	if len(view.Detail) > 50000 {
		respond(map[string]any{"behavior": "deny", "message": "Approval details exceed the display limit; no permission granted."})
		return
	}
	if name == "AskUserQuestion" {
		view.Title = "Claude needs your input"
		for i, raw := range array(input["questions"]) {
			q := object(raw)
			opts := []string{}
			for _, option := range array(q["options"]) {
				o := object(option)
				opts = append(opts, str(o["label"]))
			}
			view.Questions = append(view.Questions, ChatQuestion{ID: strconv.Itoa(i), Text: str(q["question"]), Options: opts})
		}
	}
	reply, err := c.ask(ctx, view)
	if err != nil {
		return
	}
	if !reply.Allow {
		respond(map[string]any{"behavior": "deny", "message": "The user declined this action."})
		return
	}
	if len(view.Questions) > 0 {
		answers := map[string]string{}
		for _, q := range view.Questions {
			answers[q.Text] = reply.Answers[q.ID]
		}
		input["answers"] = mustJSON(answers)
	}
	respond(map[string]any{"behavior": "allow", "updatedInput": input})
}
