package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These tests use signed-in CLIs and consume model usage. They are deliberately
// separate from the credential-free fixture suite and metadata-only discovery.
func requireLiveAgents(t *testing.T) {
	t.Helper()
	if os.Getenv("AIS_TEST_LIVE_AGENTS") != "1" {
		t.Skip("set AIS_TEST_LIVE_AGENTS=1 to run authenticated model turns")
	}
}

func liveState(t *testing.T, m *chatManager, id string) ChatState {
	t.Helper()
	u, err := m.read(id, -1)
	if err != nil {
		t.Fatal(err)
	}
	return *u.State
}

func waitLiveChat(t *testing.T, m *chatManager, id string, condition func(ChatState) bool) ChatState {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		s := liveState(t, m, id)
		if condition(s) {
			return s
		}
		if s.Status == "error" {
			t.Fatalf("%s: %s", s.Tool, s.Error)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("live turn timed out: %+v", liveState(t, m, id))
	return ChatState{}
}

func liveTurn(t *testing.T, m *chatManager, id, prompt string) ChatState {
	t.Helper()
	if err := m.send(id, prompt); err != nil {
		t.Fatal(err)
	}
	return finishLiveChat(t, m, id, nil)
}

func finishLiveChat(t *testing.T, m *chatManager, id string, approval func(ChatApproval) bool) ChatState {
	t.Helper()
	answered := map[string]bool{}
	s := waitLiveChat(t, m, id, func(s ChatState) bool {
		for _, request := range s.Approvals {
			if answered[request.ID] {
				continue
			}
			if approval == nil || !approval(request) {
				t.Fatalf("unexpected approval (not granted): %+v", request)
			}
			if err := m.approve(id, request.ID, true, nil); err != nil {
				t.Fatal(err)
			}
			answered[request.ID] = true
		}
		return !busyChat(s.Status)
	})
	if s.Status != "idle" || s.Error != "" {
		t.Fatalf("%s turn ended %s: %s", s.Tool, s.Status, s.Error)
	}
	// Wait for the final persistence step as well as the visible idle state.
	c, _ := m.get(id)
	c.mu.Lock()
	done := c.done
	c.mu.Unlock()
	<-done
	return s
}

func TestLiveCrossProviderReview(t *testing.T) {
	requireLiveAgents(t)
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "test@example.invalid"}, {"config", "user.name", "AIS smoke test"}} {
				if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
					t.Fatalf("git: %s %v", out, err)
				}
			}
			writeReviewFile(t, dir, "app.cjs", "// An adult is anyone aged at least 18.\nmodule.exports = age => age >= 18;\n")
			const check = "const assert = require('node:assert/strict');\nconst adult = require('./app.cjs');\nassert.equal(adult(17), false);\nassert.equal(adult(18), true);\nassert.equal(adult(19), true);\nconsole.log('AIS_CHECK_PASSED');\n"
			writeReviewFile(t, dir, "check.cjs", check)
			for _, args := range [][]string{{"add", "."}, {"commit", "-qm", "initial"}} {
				if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
					t.Fatalf("git: %s %v", out, err)
				}
			}
			m := newChats(t.TempDir())
			t.Cleanup(m.close)
			source := createChat(t, m, dir, provider, "")
			started := liveTurn(t, m, source.ID, "This is a temporary smoke-test project. Do not use tools yet. Reply READY. When I later send a review finding, you may edit only app.cjs and run node check.cjs to verify it. Do not change the test or use other projects.")
			writeReviewFile(t, dir, "app.cjs", "// An adult is anyone aged at least 18.\nmodule.exports = age => age > 18;\n")
			draft, err := m.prepareReview(context.Background(), source.ID)
			if err != nil {
				t.Fatal(err)
			}
			review, err := m.startReview(context.Background(), draft.ID)
			if err != nil {
				t.Fatal(err)
			}
			completed := finishLiveChat(t, m, review.ID, nil)
			if completed.Review == nil || !completed.Review.Complete || len(completed.Review.Findings) == 0 {
				t.Fatal("review missed the deliberate age-18 regression")
			}
			finding := completed.Review.Findings[0]
			t.Logf("%s reviewer: %s:%d %s", review.Tool, finding.File, finding.Line, finding.Title)
			if _, err = m.sendFindings(context.Background(), review.ID, []string{finding.ID}); err != nil {
				t.Fatal(err)
			}
			fixed := finishLiveChat(t, m, source.ID, func(a ChatApproval) bool {
				// Never grant arbitrary shell commands or writes during a live test.
				var input struct {
					FilePath string `json:"file_path"`
					Command  string `json:"command"`
				}
				if json.Unmarshal([]byte(a.Detail), &input) != nil {
					return false
				}
				return ((a.Title == "Allow Edit?" || a.Title == "Allow Write?") && input.FilePath == filepath.Join(source.Project, "app.cjs")) ||
					(a.Title == "Allow Bash?" && input.Command == "node check.cjs")
			})
			cmd := exec.Command("node", "check.cjs")
			cmd.Dir = dir
			if actual, err := os.ReadFile(filepath.Join(dir, "check.cjs")); err != nil || string(actual) != check {
				t.Fatal("coding agent changed the independent test")
			}
			if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), "AIS_CHECK_PASSED") {
				t.Fatalf("review fix did not pass the independent check: %s %v", out, err)
			}
			if fixed.SessionID != started.SessionID {
				t.Fatal("handoff lost original conversation")
			}
			t.Logf("%s coding agent fixed the regression; independent check passed", provider)
		})
	}
}

func requireLiveReply(t *testing.T, s ChatState, token string) {
	t.Helper()
	for i := len(s.Messages) - 1; i >= 0; i-- {
		msg := s.Messages[i]
		if msg.Role == "assistant" && msg.Kind == "text" {
			if !strings.Contains(msg.Text, token) || msg.Model == "" || s.SessionID == "" {
				t.Fatalf("missing expected reply, model, or native session: %+v", msg)
			}
			t.Logf("%s: model=%s, reply=%q", s.Tool, msg.Model, msg.Text)
			return
		}
	}
	t.Fatal("no assistant reply")
}

func TestLiveAgentApproval(t *testing.T) {
	requireLiveAgents(t)
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			m := newChats(t.TempDir())
			t.Cleanup(m.close)
			if provider == "claude" {
				m.command = func(name string, args ...string) *exec.Cmd {
					return exec.Command(name, append(args, "--settings", `{"permissions":{"ask":["Bash"]}}`)...)
				}
			}
			s := createChat(t, m, t.TempDir(), provider, "")
			for _, allow := range []bool{false, true} {
				prompt := "We are testing the application's approval dialog. Run exactly /usr/bin/printf AIS_APPROVAL_731 using the shell tool. Do not use any other command or tool. Request escalated execution (sandbox_permissions=require_escalated) if the tool supports it, to exercise approval. If denied, do not retry or work around the denial; reply DENIED. If allowed, reply DONE."
				if err := m.send(s.ID, prompt); err != nil {
					t.Fatal(err)
				}
				pending := waitLiveChat(t, m, s.ID, func(s ChatState) bool { return len(s.Approvals) > 0 || !busyChat(s.Status) })
				if len(pending.Approvals) != 1 {
					t.Fatalf("expected one approval, got %+v", pending)
				}
				request := pending.Approvals[0]
				var detail struct {
					Command string `json:"command"`
				}
				if err := json.Unmarshal([]byte(request.Detail), &detail); err != nil {
					t.Fatal(err)
				}
				safeCommand := detail.Command == "/usr/bin/printf AIS_APPROVAL_731" || detail.Command == "/bin/zsh -lc '/usr/bin/printf AIS_APPROVAL_731'" || detail.Command == "/bin/bash -lc '/usr/bin/printf AIS_APPROVAL_731'"
				if !safeCommand || (request.Title != "Run command?" && request.Title != "Allow Bash?") {
					t.Fatalf("unexpected command (not approved): %+v", request)
				}
				if err := m.approve(s.ID, request.ID, allow, nil); err != nil {
					t.Fatal(err)
				}
				// The answered request can remain visible until its handler runs.
				waitLiveChat(t, m, s.ID, func(s ChatState) bool { return len(s.Approvals) == 0 })
				completed := finishLiveChat(t, m, s.ID, nil)
				want := "DENIED"
				if allow {
					want = "DONE"
				}
				requireLiveReply(t, completed, want)
				t.Logf("%s approval allow=%t completed", provider, allow)
			}
		})
	}
}

func TestLiveAgentConversation(t *testing.T) {
	requireLiveAgents(t)
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			m := newChats(t.TempDir())
			t.Cleanup(m.close)
			s := createChat(t, m, t.TempDir(), provider, "")
			catalog, err := m.models(context.Background(), provider, s.Project, true)
			if err != nil {
				t.Fatal(err)
			}
			selection := ModelSettings{Model: catalog.DefaultModel}
			if _, ok := catalog.option(selection.Model); !ok {
				for _, option := range catalog.Models {
					if option.ID != "" {
						selection.Model = option.ID
						break
					}
				}
			}
			if option, ok := catalog.option(selection.Model); ok {
				for _, effort := range option.Efforts {
					if effort == "low" {
						selection.Effort = effort
					}
				}
			}
			if err = m.settings(context.Background(), s.ID, selection, false); err != nil {
				t.Fatal(err)
			}
			first := liveTurn(t, m, s.ID, "Do not use tools. Remember the token AIS_ORCHID_731 for this conversation. Reply with only that token.")
			requireLiveReply(t, first, "AIS_ORCHID_731")
			resumed := liveTurn(t, m, s.ID, "Do not use tools. What token did I ask you to remember? Reply with only the token.")
			requireLiveReply(t, resumed, "AIS_ORCHID_731")
			if resumed.SessionID != first.SessionID {
				t.Fatal("resume changed native session")
			}
			m.close()
			reopened := newChats(m.dataDir)
			t.Cleanup(reopened.close)
			if err = reopened.load(); err != nil {
				t.Fatal(err)
			}
			saved := liveState(t, reopened, s.ID)
			if saved.Settings != first.Settings || saved.SessionID != first.SessionID {
				t.Fatal("restart lost model settings or session")
			}
			afterRestart := liveTurn(t, reopened, s.ID, "Do not use tools. Repeat the remembered token, and nothing else.")
			requireLiveReply(t, afterRestart, "AIS_ORCHID_731")
			if err = reopened.send(s.ID, "Do not use tools. Write the integers from 1 to 10000, one per line, without abbreviations."); err != nil {
				t.Fatal(err)
			}
			waitLiveChat(t, reopened, s.ID, func(s ChatState) bool {
				return len(s.Messages) > len(afterRestart.Messages)+1
			})
			if err = reopened.stop(s.ID); err != nil {
				t.Fatal(err)
			}
			stopped := waitLiveChat(t, reopened, s.ID, func(s ChatState) bool { return !busyChat(s.Status) })
			if stopped.Status != "stopped" {
				t.Fatalf("stop ended with %s", stopped.Status)
			}
			continued := liveTurn(t, reopened, s.ID, "Stop counting. Do not use tools. Repeat the original remembered token only.")
			requireLiveReply(t, continued, "AIS_ORCHID_731")
		})
	}
}

func TestLiveStopFirstTurn(t *testing.T) {
	requireLiveAgents(t)
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			m := newChats(t.TempDir())
			t.Cleanup(m.close)
			s := createChat(t, m, t.TempDir(), provider, "")
			if err := m.send(s.ID, "Do not use tools. Reply READY."); err != nil {
				t.Fatal(err)
			}
			waitLiveChat(t, m, s.ID, func(s ChatState) bool { return s.SessionID != "" })
			if err := m.stop(s.ID); err != nil {
				t.Fatal(err)
			}
			stopped := waitLiveChat(t, m, s.ID, func(s ChatState) bool { return !busyChat(s.Status) })
			if stopped.Status != "stopped" {
				t.Fatalf("first turn finished before stop was exercised: %s", stopped.Status)
			}
			resumed := liveTurn(t, m, s.ID, "Do not use tools. Reply only AIS_RECOVERED.")
			requireLiveReply(t, resumed, "AIS_RECOVERED")
		})
	}
}
