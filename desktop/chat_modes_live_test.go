package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const liveCommandPrompt = "We are testing the application's approval dialog. Run exactly /usr/bin/printf AIS_APPROVAL_731 using the shell tool. Do not use any other command or tool. If denied, do not retry or work around the denial; reply DENIED. If allowed, reply DONE."

// Choosing Claude Code's own suggested rule must make Claude Code write it to
// the project's local settings, and stop it asking for the same command.
func TestLiveAlwaysAllowInProject(t *testing.T) {
	requireLiveAgents(t)
	m := newChats(t.TempDir())
	t.Cleanup(m.close)
	project := t.TempDir()
	s := createChat(t, m, project, "claude", "")
	if err := m.send(s.ID, liveCommandPrompt); err != nil {
		t.Fatal(err)
	}
	pending := waitLiveChat(t, m, s.ID, func(s ChatState) bool { return len(s.Approvals) > 0 || !busyChat(s.Status) })
	if len(pending.Approvals) != 1 {
		t.Fatalf("expected one approval, got %+v", pending)
	}
	request := pending.Approvals[0]
	if !strings.Contains(request.Detail, "/usr/bin/printf AIS_APPROVAL_731") {
		t.Fatalf("unexpected command (not approved): %+v", request)
	}
	if len(request.Options) != 1 || request.Options[0].Label != "Always allow in this project" || !strings.Contains(request.Options[0].Detail, "/usr/bin/printf AIS_APPROVAL_731") {
		t.Fatalf("expected Claude Code to offer a project rule for exactly this command: %+v", request.Options)
	}
	if err := m.approveOption(s.ID, request.ID, request.Options[0].ID); err != nil {
		t.Fatal(err)
	}
	waitLiveChat(t, m, s.ID, func(s ChatState) bool { return len(s.Approvals) == 0 })
	requireLiveReply(t, finishLiveChat(t, m, s.ID, nil), "DONE")

	// Claude Code, not ais, wrote the rule into its own project settings.
	settings, err := os.ReadFile(filepath.Join(project, ".claude", "settings.local.json"))
	if err != nil || !strings.Contains(string(settings), "AIS_APPROVAL_731") {
		t.Fatalf("the rule was not saved in the project's Claude Code settings: %s %v", settings, err)
	}
	t.Logf("saved by Claude Code: %s", strings.Join(strings.Fields(string(settings)), " "))

	// The same command now runs without another request.
	requireLiveReply(t, liveTurn(t, m, s.ID, liveCommandPrompt), "DONE")
}

// In plan mode nothing may be changed: Claude Code presents a plan to approve
// and Codex runs in a read-only sandbox. Both are declined here.
func TestLivePlanMode(t *testing.T) {
	requireLiveAgents(t)
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			m := newChats(t.TempDir())
			t.Cleanup(m.close)
			project := t.TempDir()
			s := createChat(t, m, project, provider, "")
			if err := m.setMode(s.ID, modePlan); err != nil {
				t.Fatal(err)
			}
			if err := m.send(s.ID, "Create a file named NOTES.md in the current directory containing the single word hello. Do not ask me any questions. If you cannot or may not write the file, say so briefly and stop."); err != nil {
				t.Fatal(err)
			}
			sawPlan := false
			answered := map[string]bool{}
			done := waitLiveChat(t, m, s.ID, func(s ChatState) bool {
				for _, request := range s.Approvals {
					if answered[request.ID] {
						continue
					}
					answered[request.ID] = true
					sawPlan = sawPlan || request.Plan != ""
					t.Logf("%s asked (declined): %s plan=%q", provider, request.Title, shorten(request.Plan, 200))
					// Nothing is granted in this check.
					if err := m.approve(s.ID, request.ID, false, nil); err != nil {
						t.Fatal(err)
					}
				}
				return !busyChat(s.Status)
			})
			if done.Status != "idle" {
				t.Fatalf("turn ended %s: %s", done.Status, done.Error)
			}
			if _, err := os.Stat(filepath.Join(project, "NOTES.md")); err == nil {
				t.Fatal("plan mode wrote a file")
			}
			if provider == "claude" && !sawPlan {
				t.Log("Claude Code did not present a plan for approval in this run; it still wrote nothing")
			}
			if done.Mode != modePlan {
				t.Fatalf("a declined plan must stay in plan mode, got %q", done.Mode)
			}
			t.Logf("%s wrote nothing in plan mode; last reply: %s", provider, shorten(done.Messages[len(done.Messages)-1].Text, 200))
		})
	}
}
