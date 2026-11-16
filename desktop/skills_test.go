package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestSkillsAgentProcess stands in for an agent listing its skills.
func TestSkillsAgentProcess(t *testing.T) {
	marker := -1
	for i, arg := range os.Args {
		if arg == "ais-fake-skills" {
			marker = i
		}
	}
	if marker < 0 {
		return
	}
	provider, mode := os.Args[marker+1], os.Args[marker+2]
	encoder := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		v := object(scanner.Bytes())
		if provider == "claude" {
			if str(object(v["request"])["subtype"]) != "initialize" {
				os.Exit(5)
			}
			_ = encoder.Encode(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": str(v["request_id"]), "response": map[string]any{
				"commands": []any{
					map[string]any{"name": "review", "description": "Review the diff", "argumentHint": "[files]"},
					map[string]any{"name": "Docs:write", "description": "Write docs"},
					map[string]any{"name": "", "description": "nameless"},
				}}}})
			continue
		}
		switch str(v["method"]) {
		case "initialize":
			_ = encoder.Encode(map[string]any{"id": 1, "result": map[string]any{}})
		case "initialized":
		case "skills/list":
			if mode == "error" {
				_ = encoder.Encode(map[string]any{"id": 2, "error": map[string]any{"message": "skills are unavailable"}})
				continue
			}
			cwds := array(object(v["params"])["cwds"])
			if len(cwds) != 1 {
				os.Exit(6)
			}
			_ = encoder.Encode(map[string]any{"id": 2, "result": map[string]any{"data": []any{map[string]any{"cwd": str(cwds[0]), "errors": []any{}, "skills": []any{
				map[string]any{"name": "probe", "description": "Long description", "shortDescription": "Short", "path": "/skills/probe/SKILL.md", "scope": "repo", "enabled": true},
				map[string]any{"name": "off", "description": "Disabled", "path": "/skills/off/SKILL.md", "scope": "user", "enabled": false},
				map[string]any{"name": "pdf:pdf", "description": "PDFs", "path": "/skills/pdf/SKILL.md", "scope": "user", "enabled": true},
			}}}}})
		default:
			os.Exit(5)
		}
	}
	os.Exit(0)
}

func skillsChats(t *testing.T, mode string) (*chatManager, *int) {
	t.Helper()
	m := newChats(t.TempDir())
	t.Cleanup(m.close)
	executable, _ := os.Executable()
	starts := new(int)
	m.command = func(provider string, args ...string) *exec.Cmd {
		*starts++
		return exec.Command(executable, append([]string{"-test.run=^TestSkillsAgentProcess$", "--", "ais-fake-skills", provider, mode}, args...)...)
	}
	return m, starts
}

func TestSkillLists(t *testing.T) {
	m, starts := skillsChats(t, "normal")
	project := t.TempDir()
	ctx := context.Background()
	claude, err := m.skillList(ctx, "claude", project, false)
	if err != nil || claude.Reason != "" || len(claude.Skills) != 2 {
		t.Fatalf("claude: %+v %v", claude, err)
	}
	// Sorted by name without regard to case; nameless entries are dropped.
	if claude.Skills[0].Name != "Docs:write" || claude.Skills[1].Hint != "[files]" {
		t.Fatalf("claude skills: %+v", claude.Skills)
	}
	codex, err := m.skillList(ctx, "codex", project, false)
	if err != nil || len(codex.Skills) != 2 || codex.Skills[1].Name != "probe" || codex.Skills[1].Description != "Short" || codex.Skills[1].Scope != "repo" {
		t.Fatalf("codex: %+v %v", codex, err)
	}
	// A read is reused; refresh reads again.
	before := *starts
	if _, _ = m.skillList(ctx, "codex", project, false); *starts != before {
		t.Fatal("a fresh list must be reused")
	}
	if _, _ = m.skillList(ctx, "codex", project, true); *starts != before+1 {
		t.Fatal("refresh must read again")
	}
	// "$name" of a known skill becomes a skill item; others stay text.
	items := m.codexSkillInputs(ctx, project, "use $probe and $pdf:pdf, then $probe again; $unknown $5 a$probe")
	if got := mustJSON(items); string(got) != `[{"name":"probe","path":"/skills/probe/SKILL.md","type":"skill"},{"name":"pdf:pdf","path":"/skills/pdf/SKILL.md","type":"skill"}]` {
		t.Fatalf("items: %s", got)
	}
	if items := m.codexSkillInputs(ctx, project, "no mentions"); len(items) != 0 {
		t.Fatalf("no mention: %v", items)
	}
	if _, err := m.skillList(ctx, "codex", "relative", false); err == nil {
		t.Fatal("a relative project must be refused")
	}
	if _, err := m.skillList(ctx, "other", project, false); err == nil {
		t.Fatal("an unknown agent must be refused")
	}
}

func TestSkillListFailureIsExplainedAndNotKept(t *testing.T) {
	m, starts := skillsChats(t, "error")
	project := t.TempDir()
	list, err := m.skillList(context.Background(), "codex", project, false)
	if err != nil || !strings.Contains(list.Reason, "skills are unavailable") || len(list.Skills) != 0 {
		t.Fatalf("failure: %+v %v", list, err)
	}
	before := *starts
	if _, _ = m.skillList(context.Background(), "codex", project, false); *starts != before+1 {
		t.Fatal("a failed read must not be reused")
	}
}

func TestCodexTurnUsesNamedSkill(t *testing.T) {
	m := fakeChats(t, "skill")
	state := createChat(t, m, t.TempDir(), "codex", "")
	dir, _ := projectDirectory(state.Project)
	// The conversation's project skills were already read.
	m.skillLists = map[string]*skillCache{"codex\x00" + dir: {gate: make(chan struct{}, 1), at: time.Now(), value: SkillList{Tool: "codex", Skills: []AgentSkill{{Name: "probe", Path: "/skills/probe/SKILL.md"}}}}}
	if err := m.send(state.ID, "$probe please"); err != nil {
		t.Fatal(err)
	}
	done := waitChat(t, m, state.ID, func(s ChatState) bool { return !busyChat(s.Status) })
	if done.Status != "idle" || done.Error != "" {
		t.Fatalf("turn: %s %s", done.Status, done.Error)
	}
}
