package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
)

func TestMixedMessageFormats(t *testing.T) {
	row := func(ts, kind string, payload map[string]any) map[string]any {
		return map[string]any{"timestamp": ts, "type": kind, "payload": payload}
	}
	old := []map[string]any{
		row("2026-10-03T08:00:00Z", "session_meta", map[string]any{"id": idLegacy, "cwd": "/p"}),
		row("2026-10-03T08:00:01Z", "event_msg", map[string]any{"type": "user_message", "message": "First prompt"}),
		row("2026-10-03T08:00:02Z", "response_item", map[string]any{"type": "message", "role": "assistant", "content": []textPart{{Type: "output_text", Text: "First reply"}}}),
		row("2026-10-03T08:00:02Z", "event_msg", map[string]any{"type": "agent_message", "message": "First reply"}),
	}
	newer := []map[string]any{
		row("2026-10-03T09:00:01Z", "event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "UserMessage", "content": []textPart{{Type: "text", Text: "Second prompt"}}}}),
		row("2026-10-03T09:00:01Z", "response_item", map[string]any{"type": "message", "role": "user", "content": []textPart{{Type: "input_text", Text: "Second prompt"}}}),
		row("2026-10-03T09:00:02Z", "event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "AgentMessage", "content": []textPart{{Type: "text", Text: "Second reply"}}}}),
	}
	for _, chained := range []bool{false, true} {
		name := "one rollout"
		if chained {
			name = "history base"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "sessions")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "rollout-2026-10-03T08-00-00-"+idLegacy+".jsonl")
			if chained {
				writeLines(t, path, old...)
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(dir, "rollout-2026-10-03T09-00-00-"+idLegacy+"_new.jsonl")
				meta := row("2026-10-03T09:00:00Z", "session_meta", map[string]any{"id": idLegacy, "cwd": "/p", "history_base": map[string]any{"thread_id": idLegacy, "end_byte_offset": info.Size()}})
				writeLines(t, path, append([]map[string]any{meta}, newer...)...)
			} else {
				writeLines(t, path, append(append([]map[string]any{}, old...), newer...)...)
			}
			p := New(root)
			s, warns, err := p.Parse(context.Background(), provider.FileRef{Path: path})
			if err != nil || len(warns) != 0 {
				t.Fatalf("Parse = %v, %v", warns, err)
			}
			if s.UserTurns != 2 || s.AssistantTurns != 2 || s.FirstPrompt != "First prompt" || s.LastPrompt != "Second prompt" {
				t.Fatalf("mixed session: %+v", s)
			}
			msgs, err := p.Transcript(context.Background(), s)
			if err != nil || len(msgs) != 4 {
				t.Fatalf("Transcript = %+v, %v", msgs, err)
			}
		})
	}
}

func TestDuplicateCopiesAndRepeatedMessages(t *testing.T) {
	now := time.Now()
	var sc scan
	add := func(src source, role model.Role, text, turn string, offset time.Duration) {
		sc.turn = turn
		sc.add(src, model.Message{Role: role, Kind: model.KindText, Text: text, Time: now.Add(offset)})
	}
	add(srcUserResp, model.RoleUser, "again", "one", 0)
	add(srcUserItem, model.RoleUser, "again", "one", time.Second)
	add(srcUserEvent, model.RoleUser, "again", "one", 2*time.Second)
	add(srcAsstResp, model.RoleAssistant, "ok", "one", 3*time.Second)
	add(srcAsstEvent, model.RoleAssistant, "ok", "one", 4*time.Second)
	// Same text in another turn is still a real message, even if adjacent.
	add(srcAsstItem, model.RoleAssistant, "ok", "two", 4*time.Second)
	add(srcUserItem, model.RoleUser, "again", "two", 5*time.Second)
	add(srcUserItem, model.RoleUser, "again", "two", 6*time.Second)
	// A legacy-only message much later must not be mistaken for a copy.
	add(srcUserEvent, model.RoleUser, "again", "", time.Hour)
	if msgs := sc.messages(); len(msgs) != 6 {
		t.Fatalf("messages = %+v", msgs)
	}
}

func TestItemCompletedTurnIDsPreserveRepeatedReplies(t *testing.T) {
	var sc scan
	now := time.Now()
	for _, turn := range []string{"one", "two"} {
		payload, err := json.Marshal(map[string]any{"type": "item_completed", "turn_id": turn,
			"item": map[string]any{"type": "AgentMessage", "content": []textPart{{Type: "text", Text: "ok"}}}})
		if err != nil {
			t.Fatal(err)
		}
		if err := sc.event(payload, now); err != nil {
			t.Fatal(err)
		}
		// A response-item copy can arrive well after the item event.
		sc.add(srcAsstResp, model.Message{Role: model.RoleAssistant, Kind: model.KindText, Text: "ok", Time: now.Add(time.Minute)})
	}
	if msgs := sc.messages(); len(msgs) != 2 {
		t.Fatalf("messages = %+v", msgs)
	}
}
