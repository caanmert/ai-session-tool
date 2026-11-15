package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func finishFake(t *testing.T, m *chatManager, id, provider string) ChatState {
	t.Helper()
	done := waitChat(t, m, id, func(s ChatState) bool { return !busyChat(s.Status) })
	if done.Status != "idle" || done.Error != "" || done.Messages[len(done.Messages)-1].Text != "Hello from "+provider {
		t.Fatalf("unexpected result: %+v", done)
	}
	c, _ := m.get(id)
	<-c.done
	return done
}

func TestApprovalOptions(t *testing.T) {
	for _, provider := range []string{"codex", "claude"} {
		// The agent offers a wider grant: it is shown, and choosing it is what
		// the agent receives.
		t.Run(provider+"/offered", func(t *testing.T) {
			m := fakeChats(t, "wide")
			state := createChat(t, m, t.TempDir(), provider, "")
			if err := m.send(state.ID, "Run the tests"); err != nil {
				t.Fatal(err)
			}
			pending := waitChat(t, m, state.ID, func(s ChatState) bool { return len(s.Approvals) == 1 })
			options := pending.Approvals[0].Options
			if len(options) != 1 {
				t.Fatalf("expected one offered option: %+v", pending.Approvals[0])
			}
			if provider == "claude" && (options[0].Label != "Always allow in this project" || !strings.Contains(options[0].Detail, "Bash(npm test:*)")) {
				t.Fatalf("the option must name the rule and where it is kept: %+v", options[0])
			}
			if provider == "codex" && (options[0].Label != "Always allow this command" || !strings.Contains(options[0].Detail, "npm test")) {
				t.Fatalf("option: %+v", options[0])
			}
			if err := m.approveOption(state.ID, pending.Approvals[0].ID, "something-else"); err == nil {
				t.Fatal("an option the agent did not offer must be refused")
			}
			if err := m.approveOption(state.ID, pending.Approvals[0].ID, options[0].ID); err != nil {
				t.Fatal(err)
			}
			if err := m.approveOption(state.ID, pending.Approvals[0].ID, options[0].ID); err == nil {
				t.Fatal("allowed a second answer")
			}
			finishFake(t, m, state.ID, provider)
		})
		// The agent offers nothing safe to widen: only a one-time answer exists.
		t.Run(provider+"/not offered", func(t *testing.T) {
			m := fakeChats(t, "narrow")
			state := createChat(t, m, t.TempDir(), provider, "")
			if err := m.send(state.ID, "Run the tests"); err != nil {
				t.Fatal(err)
			}
			pending := waitChat(t, m, state.ID, func(s ChatState) bool { return len(s.Approvals) == 1 })
			if len(pending.Approvals[0].Options) != 0 {
				t.Fatalf("no option should be offered: %+v", pending.Approvals[0].Options)
			}
			if err := m.approveOption(state.ID, pending.Approvals[0].ID, claudeRuleOption); err == nil {
				t.Fatal("a wider grant must be refused when none was offered")
			}
			if err := m.approve(state.ID, pending.Approvals[0].ID, true, nil); err != nil {
				t.Fatal(err)
			}
			finishFake(t, m, state.ID, provider)
		})
	}
}

func TestPermissionModes(t *testing.T) {
	t.Run("codex/plan runs read-only", func(t *testing.T) {
		m := fakeChats(t, "plan")
		state := createChat(t, m, t.TempDir(), "codex", "")
		if err := m.setMode(state.ID, modeAutoEdit); err == nil {
			t.Fatal("Codex has no separate auto-edit mode")
		}
		if err := m.setMode(state.ID, modePlan); err != nil {
			t.Fatal(err)
		}
		if err := m.send(state.ID, "Plan it"); err != nil {
			t.Fatal(err)
		}
		finishFake(t, m, state.ID, "codex")
	})
	t.Run("claude/plan asks to approve the plan, then leaves plan mode", func(t *testing.T) {
		m := fakeChats(t, "plan")
		state := createChat(t, m, t.TempDir(), "claude", "")
		if err := m.setMode(state.ID, modePlan); err != nil {
			t.Fatal(err)
		}
		if err := m.send(state.ID, "Plan it"); err != nil {
			t.Fatal(err)
		}
		if err := m.setMode(state.ID, modeDefault); err == nil {
			t.Fatal("the mode must not change during a response")
		}
		pending := waitChat(t, m, state.ID, func(s ChatState) bool { return len(s.Approvals) == 1 })
		request := pending.Approvals[0]
		if request.Title != "Approve this plan?" || request.Plan != "1. Add retries" || len(request.Options) != 0 {
			t.Fatalf("plan approval: %+v", request)
		}
		if err := m.approve(state.ID, request.ID, true, nil); err != nil {
			t.Fatal(err)
		}
		done := finishFake(t, m, state.ID, "claude")
		if done.Mode != modeDefault {
			t.Fatalf("an approved plan should leave plan mode, got %q", done.Mode)
		}
	})
	t.Run("claude/auto-edit is passed to the agent and saved", func(t *testing.T) {
		m := fakeChats(t, "autoedit")
		state := createChat(t, m, t.TempDir(), "claude", "")
		if err := m.setMode(state.ID, "bypassPermissions"); err == nil {
			t.Fatal("only listed modes may be set")
		}
		if err := m.setMode(state.ID, modeAutoEdit); err != nil {
			t.Fatal(err)
		}
		if err := m.send(state.ID, "Edit it"); err != nil {
			t.Fatal(err)
		}
		done := finishFake(t, m, state.ID, "claude")
		if done.Mode != modeAutoEdit {
			t.Fatalf("mode: %q", done.Mode)
		}
		reloaded := newChats(m.dataDir)
		t.Cleanup(reloaded.close)
		if err := reloaded.load(); err != nil {
			t.Fatal(err)
		}
		saved, _ := reloaded.read(state.ID, -1)
		if saved.State.Mode != modeAutoEdit {
			t.Fatalf("the mode should survive a restart: %q", saved.State.Mode)
		}
	})
}

func TestChatModesAndSuggestions(t *testing.T) {
	if got := chatModes("claude"); len(got) != 3 || got[1].ID != modeDefault {
		t.Fatalf("claude modes: %+v", got)
	}
	if got := chatModes("codex"); len(got) != 2 {
		t.Fatalf("codex modes: %+v", got)
	}
	suggest := func(v any) (ApprovalOption, bool) {
		data, _ := json.Marshal(v)
		return claudeSuggestionOption(data)
	}
	rule := map[string]any{"type": "addRules", "behavior": "allow", "destination": "session", "rules": []any{map[string]any{"toolName": "Edit"}}}
	if option, ok := suggest([]any{rule}); !ok || option.Label != "Allow for this conversation" || !strings.Contains(option.Detail, "Edit") {
		t.Fatalf("session rule: %+v %v", option, ok)
	}
	for name, bad := range map[string]any{
		"none":                []any{},
		"deny rule":           []any{map[string]any{"type": "addRules", "behavior": "deny", "destination": "session", "rules": []any{map[string]any{"toolName": "Edit"}}}},
		"mode change":         []any{map[string]any{"type": "setMode", "mode": "bypassPermissions", "destination": "session"}},
		"mixed":               []any{rule, map[string]any{"type": "addDirectories", "directories": []string{"/"}, "destination": "session"}},
		"unknown destination": []any{map[string]any{"type": "addRules", "behavior": "allow", "destination": "elsewhere", "rules": []any{map[string]any{"toolName": "Edit"}}}},
		"two destinations":    []any{rule, map[string]any{"type": "addRules", "behavior": "allow", "destination": "userSettings", "rules": []any{map[string]any{"toolName": "Edit"}}}},
		"no rules":            []any{map[string]any{"type": "addRules", "behavior": "allow", "destination": "session", "rules": []any{}}},
		"not a list":          "addRules",
	} {
		if option, ok := suggest(bad); ok {
			t.Errorf("%s must not be offered: %+v", name, option)
		}
	}
}
