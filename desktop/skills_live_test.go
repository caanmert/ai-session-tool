package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSkill puts a test skill in a project folder.
func writeSkill(t *testing.T, dir, folder, name string) {
	t.Helper()
	path := filepath.Join(dir, folder, name)
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: Test skill for ais. Use only when asked for it by name.\n---\n\nWhen this skill is used, reply with exactly the word BANANA-" + name + " and nothing else.\n"
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Each agent lists a project skill and runs it when a message names it.
func TestLiveSkills(t *testing.T) {
	requireLiveAgents(t)
	for _, tc := range []struct{ tool, folder, message string }{
		{"claude", ".claude/skills", "/ais-probe"},
		{"codex", ".codex/skills", "$ais-probe"},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			project := newGitRepo(t).dir
			writeSkill(t, project, tc.folder, "ais-probe")
			m := newChats(t.TempDir())
			t.Cleanup(m.close)
			list, err := m.skillList(context.Background(), tc.tool, project, true)
			if err != nil || list.Reason != "" {
				t.Fatalf("list: %v %s", err, list.Reason)
			}
			found := false
			for _, skill := range list.Skills {
				found = found || skill.Name == "ais-probe"
			}
			t.Logf("%s lists %d skills; test skill found: %v", tc.tool, len(list.Skills), found)
			if !found {
				t.Fatal("the project skill is not listed")
			}
			s := createChat(t, m, project, tc.tool, "")
			if err := m.send(s.ID, tc.message); err != nil {
				t.Fatal(err)
			}
			done := finishLiveChat(t, m, s.ID, func(ChatApproval) bool { return false })
			reply := ""
			for _, message := range done.Messages {
				if message.Role == "assistant" && message.Kind == "text" {
					reply += message.Text
				}
			}
			t.Logf("%s replied %q", tc.tool, shorten(reply, 200))
			if !strings.Contains(reply, "BANANA-ais-probe") {
				t.Fatal("the skill did not run")
			}
		})
	}
}
