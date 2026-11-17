package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Real child processes exercise the streaming bridge without credentials or model calls.
func TestAgentProcess(t *testing.T) {
	marker := -1
	for i, arg := range os.Args {
		if arg == "ais-fake" {
			marker = i
			break
		}
	}
	if marker < 0 {
		return
	}
	provider, mode := os.Args[marker+1], os.Args[marker+2]
	cliArgs := strings.Join(os.Args[marker+3:], " ")
	decoder := json.NewDecoder(bufio.NewReader(os.Stdin))
	encoder := json.NewEncoder(os.Stdout)
	read := func() jsonObject {
		var v jsonObject
		if decoder.Decode(&v) != nil {
			os.Exit(2)
		}
		return v
	}
	emit := func(v any) {
		if encoder.Encode(v) != nil {
			os.Exit(3)
		}
	}
	require := func(ok bool, message string) {
		if !ok {
			fmt.Fprintln(os.Stderr, message)
			os.Exit(4)
		}
	}
	result := "Hello from " + provider
	if mode == "review" {
		result = `{"summary":"One issue","findings":[{"priority":"P2","file":"app.txt","line":1,"title":"Check result","body":"Handle the changed result."}]}`
	}
	messageID := fmt.Sprintf("message-%d", os.Getpid())
	if provider == "codex" {
		init := read()
		require(str(init["method"]) == "initialize", "expected initialize")
		emit(map[string]any{"id": 1, "result": map[string]any{}})
		require(str(read()["method"]) == "initialized", "expected initialized")

		if str(object(object(init["params"])["clientInfo"])["name"]) == "ais_model_picker" {
			require(str(read()["method"]) == "config/read", "expected config read")
			emit(map[string]any{"id": 2, "result": map[string]any{"config": map[string]any{"model": "model-a", "model_reasoning_effort": "medium"}}})
			for page := 0; page < 2; page++ {
				request := read()
				require(str(request["method"]) == "model/list", "expected model list")
				id := "model-a"
				cursor := "next"
				if page == 1 {
					id = "model-b"
					cursor = ""
					require(str(object(request["params"])["cursor"]) == "next", "missing pagination cursor")
				}
				data := []any{map[string]any{"model": id, "displayName": id, "description": "Test model", "isDefault": page == 0, "defaultReasoningEffort": "medium", "supportedReasoningEfforts": []any{map[string]string{"reasoningEffort": "low"}, map[string]string{"reasoningEffort": "medium"}, map[string]string{"reasoningEffort": "high"}}}}
				if mode == "no-models" {
					data = []any{}
					cursor = ""
				}
				emit(map[string]any{"id": 3, "result": map[string]any{"data": data, "nextCursor": cursor}})
				if cursor == "" {
					break
				}
			}
			for {
				time.Sleep(time.Second)
			}
		}
		thread := read()
		params := object(thread["params"])
		require(str(thread["method"]) == "thread/start" || str(thread["method"]) == "thread/resume", "expected thread operation")
		if mode == "review" {
			require(str(params["sandbox"]) == "read-only" && str(params["approvalPolicy"]) == "never", "review permission configuration")
		}
		if mode == "writer" {
			// Another Codex program has this conversation open.
			emit(map[string]any{"id": 2, "error": map[string]any{"code": -32600, "message": "thread native-thread already has an active writer"}})
			for {
				time.Sleep(time.Second)
			}
		}
		if mode == "resume" {
			require(str(thread["method"]) == "thread/resume" && str(params["threadId"]) == "native-thread", "expected saved thread")
		}
		if mode == "plan" {
			require(str(params["sandbox"]) == "read-only" && str(params["approvalPolicy"]) == "on-request", "plan mode must be read-only")
		} else if mode != "review" {
			require(str(params["sandbox"]) == "workspace-write", "default sandbox")
		}
		model := "model-a"
		if str(params["model"]) != "" {
			model = str(params["model"])
		}
		if mode == "settings" {
			require(model == "model-b", "wrong selected model")
			require(str(object(params["config"])["model_reasoning_effort"]) == "high", "missing thread effort")
		}
		if mode == "default-settings" {
			require(model == "model-a", "default model was not restored")
		}
		if mode == "websearch" {
			require(str(object(params["config"])["web_search"]) == "live", "web search must reach the thread config")
		} else if _, set := object(params["config"])["web_search"]; set {
			require(false, "web search must not be set unless chosen")
		}
		emit(map[string]any{"id": 2, "result": map[string]any{"model": model, "thread": map[string]any{"id": "native-thread"}}})
		turnRequest := read()
		require(str(turnRequest["method"]) == "turn/start", "expected turn start")
		if mode == "image" {
			input := array(object(turnRequest["params"])["input"])
			require(len(input) == 2 && str(object(input[0])["type"]) == "text" && str(object(input[1])["type"]) == "localImage", "expected text and one local image")
			data, err := os.ReadFile(str(object(input[1])["path"]))
			require(err == nil && len(data) > 8 && string(data[1:4]) == "PNG", "the image file must exist and be the PNG that was attached")
		} else if mode == "skill" {
			input := array(object(turnRequest["params"])["input"])
			require(len(input) == 2 && str(object(input[1])["type"]) == "skill" && str(object(input[1])["name"]) == "probe" && str(object(input[1])["path"]) == "/skills/probe/SKILL.md", "expected the text and one skill item")
		} else {
			require(len(array(object(turnRequest["params"])["input"])) == 1, "a plain message has only text")
		}
		if mode == "settings" {
			require(str(object(turnRequest["params"])["model"]) == "model-b" && str(object(turnRequest["params"])["effort"]) == "high", "missing turn settings")
		}
		if mode == "reroute" {
			emit(map[string]any{"method": "model/rerouted", "params": map[string]any{"toModel": "model-fallback"}})
		}
		emit(map[string]any{"id": 3, "result": map[string]any{"turn": map[string]any{"id": "turn"}}})
		if mode == "hold" {
			for {
				time.Sleep(time.Second)
			}
		}
		if mode == "invalid" {
			fmt.Println("not json")
			for {
				time.Sleep(time.Second)
			}
		}
		if mode == "approve" || mode == "deny" {
			emit(map[string]any{"id": 99, "method": "item/commandExecution/requestApproval", "params": map[string]any{"command": "echo example", "availableDecisions": []string{"accept", "decline"}}})
			response := read()
			want := "accept"
			if mode == "deny" {
				want = "decline"
			}
			require(number(response["id"]) == 99 && str(object(response["result"])["decision"]) == want, "wrong approval reply")
		}
		if mode == "wide" || mode == "narrow" {
			// The shape real Codex sends: plain decisions plus one that carries
			// the rule it would add. An unknown decision must be ignored.
			decisions := []any{"accept", "cancel"}
			if mode == "wide" {
				decisions = []any{"accept", map[string]any{"acceptWithExecpolicyAmendment": map[string]any{"execpolicy_amendment": []string{"npm", "test"}}}, map[string]any{"somethingNew": true}, "cancel"}
			}
			emit(map[string]any{"id": 98, "method": "item/commandExecution/requestApproval", "params": map[string]any{"command": "npm test", "availableDecisions": decisions}})
			response := read()
			decision := object(response["result"])["decision"]
			if mode == "wide" {
				words := array(object(object(decision)["acceptWithExecpolicyAmendment"])["execpolicy_amendment"])
				require(number(response["id"]) == 98 && len(words) == 2 && str(words[0]) == "npm", "the offered rule must be sent back unchanged")
			} else {
				require(number(response["id"]) == 98 && str(decision) == "accept", "wrong one-time approval reply")
			}
		}
		emit(map[string]any{"method": "item/agentMessage/delta", "params": map[string]any{"itemId": messageID, "delta": "Hello "}})
		emit(map[string]any{"method": "item/completed", "params": map[string]any{"item": map[string]any{"id": messageID, "type": "agentMessage", "text": result}}})
		if mode == "context" {
			emit(map[string]any{"method": "thread/tokenUsage/updated", "params": map[string]any{"threadId": "native-thread", "turnId": "turn", "tokenUsage": map[string]any{
				"last":  map[string]any{"totalTokens": 52000, "inputTokens": 48000, "cachedInputTokens": 30000, "outputTokens": 4000, "reasoningOutputTokens": 2000},
				"total": map[string]any{"totalTokens": 90000, "inputTokens": 80000, "cachedInputTokens": 50000, "outputTokens": 10000, "reasoningOutputTokens": 5000}, "modelContextWindow": 272000}}})
		}
		emit(map[string]any{"method": "turn/completed", "params": map[string]any{"turn": map[string]any{"id": "turn", "status": "completed"}}})
	} else {
		init := read()
		require(str(init["type"]) == "control_request", "expected control initialize")
		if str(init["request_id"]) == "ais-models" {
			models := []any{map[string]any{"value": "default", "resolvedModel": "model-a", "displayName": "Default", "supportsEffort": true, "supportedEffortLevels": []string{"low", "medium", "high"}}, map[string]any{"value": "model-b", "displayName": "Model B", "supportsEffort": true, "supportedEffortLevels": []string{"low", "medium", "high"}}, map[string]any{"value": "fast-model", "displayName": "Fast"}}
			if mode == "no-models" {
				models = []any{}
			}
			emit(map[string]any{"type": "control_response", "response": map[string]any{"request_id": "ais-models", "subtype": "success", "response": map[string]any{"models": models}}})
			for {
				time.Sleep(time.Second)
			}
		}
		model := "model-a"
		if mode == "settings" {
			require(strings.Contains(cliArgs, "--model model-b") && strings.Contains(cliArgs, "--effort high"), "missing Claude model settings")
			model = "model-b"
		}
		if mode == "default-settings" {
			require(strings.Contains(cliArgs, "--model default") && !strings.Contains(cliArgs, "--effort"), "Claude default settings not restored")
		}
		if mode == "reroute" {
			model = "model-fallback"
		}
		emit(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": "ais-initialize"}})
		userInput := read()
		require(str(userInput["type"]) == "user", "expected user input")
		content := object(userInput["message"])["content"]
		if mode == "image" {
			blocks := array(content)
			require(len(blocks) == 2 && str(object(blocks[0])["type"]) == "text", "expected text and one image block")
			source := object(object(blocks[1])["source"])
			require(str(object(blocks[1])["type"]) == "image" && str(source["type"]) == "base64" && str(source["media_type"]) == "image/png" && len(str(source["data"])) > 20, "expected an inline PNG")
		} else {
			require(str(content) != "", "a plain message is sent as a string")
		}
		if mode == "resume" {
			require(strings.Contains(cliArgs, "--resume native-thread"), "expected saved Claude session")
		}
		if mode == "review" {
			require(strings.Contains(cliArgs, "--tools  --strict-mcp-config"), "review tools must be disabled")
		}
		if mode == "hold" {
			for {
				time.Sleep(time.Second)
			}
		}
		if mode == "approve" || mode == "deny" {
			emit(map[string]any{"type": "control_request", "request_id": "approval", "request": map[string]any{"subtype": "can_use_tool", "tool_name": "Write", "input": map[string]any{"file_path": "app.txt", "content": "example"}}})
			response := object(object(read()["response"])["response"])
			want := "allow"
			if mode == "deny" {
				want = "deny"
			}
			require(str(response["behavior"]) == want, "wrong Claude approval reply")
			if mode == "approve" {
				require(str(object(response["updatedInput"])["content"]) == "example", "must preserve tool input")
			}
		}
		if mode == "plan" {
			require(strings.Contains(cliArgs, "--permission-mode plan"), "missing plan mode")
			emit(map[string]any{"type": "control_request", "request_id": "plan", "request": map[string]any{"subtype": "can_use_tool", "tool_name": "ExitPlanMode", "input": map[string]any{"plan": "1. Add retries"},
				"permission_suggestions": []any{map[string]any{"type": "setMode", "mode": "acceptEdits", "destination": "session"}}}})
			response := object(object(read()["response"])["response"])
			_, widened := response["updatedPermissions"]
			require(str(response["behavior"]) == "allow" && !widened, "plan approval must not change permissions")
		} else if mode == "autoedit" {
			require(strings.Contains(cliArgs, "--permission-mode acceptEdits"), "missing auto-edit mode")
		} else if mode != "review" {
			require(strings.Contains(cliArgs, "--permission-mode default"), "missing default mode")
		}
		if mode == "wide" || mode == "narrow" {
			suggestions := []any{map[string]any{"type": "addRules", "behavior": "allow", "destination": "localSettings", "rules": []any{map[string]any{"toolName": "Bash", "ruleContent": "npm test:*"}}}}
			if mode == "narrow" {
				// A suggestion that is not a plain allow rule must not be offered.
				suggestions = []any{map[string]any{"type": "setMode", "mode": "bypassPermissions", "destination": "session"}}
			}
			emit(map[string]any{"type": "control_request", "request_id": "scoped", "request": map[string]any{"subtype": "can_use_tool", "tool_name": "Bash", "input": map[string]any{"command": "npm test"}, "permission_suggestions": suggestions}})
			response := object(object(read()["response"])["response"])
			updated := array(response["updatedPermissions"])
			if mode == "wide" {
				require(str(response["behavior"]) == "allow" && len(updated) == 1 && str(object(updated[0])["destination"]) == "localSettings", "suggested rule must be returned")
			} else {
				require(str(response["behavior"]) == "allow" && len(updated) == 0, "one-time approval must not add rules")
			}
		}
		emit(map[string]any{"type": "stream_event", "event": map[string]any{"type": "message_start", "message": map[string]any{"id": messageID}}})
		emit(map[string]any{"type": "stream_event", "event": map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": "Hello "}}})
		assistant := map[string]any{"id": messageID, "model": model, "content": []any{map[string]any{"type": "text", "text": result}}}
		final := map[string]any{"type": "result", "is_error": false, "result": result}
		if mode == "context" {
			assistant["usage"] = map[string]any{"input_tokens": 1200, "cache_creation_input_tokens": 3000, "cache_read_input_tokens": 40000, "output_tokens": 800}
			// A sub-agent's smaller model is listed too; the answering model's window counts.
			final["modelUsage"] = map[string]any{model: map[string]any{"contextWindow": 200000}, "small-helper": map[string]any{"contextWindow": 100000}}
		}
		emit(map[string]any{"type": "assistant", "message": assistant})
		emit(final)
	}
	// CLI connections are persistent; the bridge must reap them after a turn.
	for {
		time.Sleep(time.Second)
	}
}
func fakeChats(t *testing.T, mode string) *chatManager {
	t.Helper()
	m := newChats(t.TempDir())
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	m.command = func(provider string, args ...string) *exec.Cmd {
		return exec.Command(executable, append([]string{"-test.run=^TestAgentProcess$", "--", "ais-fake", provider, mode}, args...)...)
	}
	t.Cleanup(m.close)
	return m
}
func waitChat(t *testing.T, m *chatManager, id string, condition func(ChatState) bool) ChatState {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		update, err := m.read(id, -1)
		if err != nil {
			t.Fatal(err)
		}
		if condition(*update.State) {
			return *update.State
		}
		time.Sleep(10 * time.Millisecond)
	}
	update, _ := m.read(id, -1)
	t.Fatalf("conversation did not reach expected state: %+v", update.State)
	return ChatState{}
}
func createChat(t *testing.T, m *chatManager, project, provider, session string) ChatState {
	t.Helper()
	state, err := m.create(project, provider, session, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return state
}
func TestStructuredConversations(t *testing.T) {
	for _, provider := range []string{"codex", "claude"} {
		for _, mode := range []string{"normal", "resume", "approve", "deny"} {
			t.Run(provider+"/"+mode, func(t *testing.T) {
				m := fakeChats(t, mode)
				session := ""
				if mode == "resume" {
					session = "native-thread"
				}
				state := createChat(t, m, t.TempDir(), provider, session)
				if err := m.send(state.ID, "Help me"); err != nil {
					t.Fatal(err)
				}
				if mode == "approve" || mode == "deny" {
					pending := waitChat(t, m, state.ID, func(s ChatState) bool { return len(s.Approvals) == 1 })
					if err := m.send(state.ID, "duplicate"); err == nil {
						t.Fatal("allowed concurrent turn")
					}
					if err := m.approve(state.ID, pending.Approvals[0].ID, mode == "approve", nil); err != nil {
						t.Fatal(err)
					}
					if err := m.approve(state.ID, pending.Approvals[0].ID, true, nil); err == nil {
						t.Fatal("allowed duplicate approval")
					}
				}
				done := waitChat(t, m, state.ID, func(s ChatState) bool { return !busyChat(s.Status) })
				if done.Status != "idle" || done.Error != "" || len(done.Messages) != 2 || done.Messages[1].Text != "Hello from "+provider {
					t.Fatalf("unexpected result: %+v", done)
				}
				c, _ := m.get(state.ID)
				<-c.done
				reloaded := newChats(m.dataDir)
				if err := reloaded.load(); err != nil {
					t.Fatal(err)
				}
				saved, err := reloaded.read(state.ID, -1)
				if err != nil || saved.State.SessionID == "" || len(saved.State.Messages) != 2 {
					t.Fatalf("not persisted: %+v %v", saved, err)
				}
				if err := m.remove(state.ID); err != nil {
					t.Fatal(err)
				}
				if len(m.list()) != 0 {
					t.Fatal("closed tab still listed")
				}
				if _, err := m.read(state.ID, -1); err != nil {
					t.Fatal("closing lost review linkage")
				}
			})
		}
	}
}
func TestStopAndMalformedProtocol(t *testing.T) {
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			m := fakeChats(t, "hold")
			state := createChat(t, m, t.TempDir(), provider, "")
			if err := m.send(state.ID, "work"); err != nil {
				t.Fatal(err)
			}
			if err := m.stop(state.ID); err != nil {
				t.Fatal(err)
			}
			done := waitChat(t, m, state.ID, func(s ChatState) bool { return s.Status == "stopped" })
			if len(done.Approvals) != 0 {
				t.Fatal("stale approval")
			}
		})
	}
	m := fakeChats(t, "invalid")
	state := createChat(t, m, t.TempDir(), "codex", "")
	_ = m.send(state.ID, "work")
	done := waitChat(t, m, state.ID, func(s ChatState) bool { return s.Status == "error" })
	if !strings.Contains(done.Error, "invalid JSON") {
		t.Fatal(done.Error)
	}
}

func TestClaudeStopBeforeFirstResponse(t *testing.T) {
	for _, nativeID := range []string{"", "existing-session"} {
		t.Run("session="+nativeID, func(t *testing.T) {
			m := fakeChats(t, "hold")
			s := createChat(t, m, t.TempDir(), "claude", nativeID)
			if err := m.send(s.ID, "first prompt"); err != nil {
				t.Fatal(err)
			}
			waitChat(t, m, s.ID, func(s ChatState) bool { return s.SessionID != "" })
			// A crash before the first response must not restore a provisional ID.
			disk := newChats(m.dataDir)
			defer disk.close()
			if err := disk.load(); err != nil {
				t.Fatal(err)
			}
			saved, _ := disk.read(s.ID, -1)
			if saved.State.SessionID != nativeID {
				t.Fatal("persisted a Claude session before it became resumable")
			}
			if err := m.stop(s.ID); err != nil {
				t.Fatal(err)
			}
			stopped := waitChat(t, m, s.ID, func(s ChatState) bool { return s.Status == "stopped" })
			if stopped.SessionID != nativeID {
				t.Fatal("stop kept a provisional ID or discarded an existing session")
			}
		})
	}
}
func TestCanceledApprovalDoesNotReviveTurn(t *testing.T) {
	m := newChats("")
	s := createChat(t, m, t.TempDir(), "codex", "")
	c, _ := m.get(s.ID)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.ask(ctx, ChatApproval{ID: "old"}); err == nil {
		t.Fatal("accepted canceled approval")
	}
	update, _ := m.read(s.ID, -1)
	if update.State.Status != "idle" || len(update.State.Approvals) != 0 {
		t.Fatal("stale handler changed state")
	}
}
func TestInterruptedStateRecovery(t *testing.T) {
	m := newChats(t.TempDir())
	s := createChat(t, m, t.TempDir(), "claude", "")
	c, _ := m.get(s.ID)
	c.state.Status = "waiting"
	c.state.Approvals = []ChatApproval{{ID: "old"}}
	if err := m.save(c); err != nil {
		t.Fatal(err)
	}
	reloaded := newChats(m.dataDir)
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	update, _ := reloaded.read(s.ID, -1)
	if update.State.Status != "stopped" || len(update.State.Approvals) != 0 {
		t.Fatal("did not recover interrupted run")
	}
	info, err := os.Stat(filepath.Join(m.dataDir, s.ID+".json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("chat storage permissions")
	}
}

func TestResolvedApprovalBeforeHandlerStarts(t *testing.T) {
	m := newChats("")
	state := createChat(t, m, t.TempDir(), "codex", "")
	c, _ := m.get(state.ID)
	c.state.Status = "running"
	c.dismissApproval("resolved")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := c.ask(ctx, ChatApproval{ID: "resolved"}); err == nil || !strings.Contains(err.Error(), "already resolved") {
		t.Fatalf("resolved request was presented: %v", err)
	}
	if len(c.state.Approvals) != 0 {
		t.Fatal("resolved approval is visible")
	}
}
