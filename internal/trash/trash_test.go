package trash

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/caanmert/ai-session-tool/internal/model"
)

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestClaudeRoundTrip(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	proj := filepath.Join(home, "projects", "-Users-me-api")
	transcript := filepath.Join(proj, "abc.jsonl")
	writeFile(t, transcript, "{}\n")
	writeFile(t, filepath.Join(proj, "abc", "subagents", "agent-1.jsonl"), "{}\n")
	writeFile(t, filepath.Join(proj, "other.jsonl"), "{}\n")

	tr := New(filepath.Join(t.TempDir(), "trash"))
	s := model.Session{Tool: model.ToolClaude, ID: "abc", Title: "Fix auth", Path: transcript, CWD: "/Users/me/api"}
	e, err := tr.Put(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if exists(transcript) || exists(filepath.Join(proj, "abc")) || len(e.Files) != 2 {
		t.Fatalf("files not moved: %+v", e.Files)
	}
	if !exists(filepath.Join(proj, "other.jsonl")) {
		t.Fatal("other sessions must stay")
	}
	if _, err := tr.Put(ctx, s); err == nil {
		t.Error("trashing twice should fail")
	}

	list, err := tr.List()
	if err != nil || len(list) != 1 || list[0].Title != "Fix auth" || list[0].Method != MethodMove {
		t.Fatalf("List = %+v, %v", list, err)
	}
	found, err := tr.Find("ab")
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.Restore(ctx, found); err != nil {
		t.Fatal(err)
	}
	if !exists(transcript) || !exists(filepath.Join(proj, "abc", "subagents", "agent-1.jsonl")) {
		t.Fatal("restore did not put the files back")
	}
	if list, _ := tr.List(); len(list) != 0 {
		t.Errorf("trash should be empty after restore: %+v", list)
	}
}

func TestRestoreRefusesToOverwrite(t *testing.T) {
	ctx := context.Background()
	transcript := filepath.Join(t.TempDir(), "abc.jsonl")
	writeFile(t, transcript, "old\n")
	tr := New(filepath.Join(t.TempDir(), "trash"))
	e, err := tr.Put(ctx, model.Session{Tool: model.ToolClaude, ID: "abc", Path: transcript})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, transcript, "new session with the same id\n")
	if err := tr.Restore(ctx, e); err == nil || !strings.Contains(err.Error(), "exists again") {
		t.Fatalf("restore over an existing file: %v", err)
	}
	if data, _ := os.ReadFile(transcript); string(data) != "new session with the same id\n" {
		t.Error("restore clobbered a file")
	}
}

func TestRefusesLiveSessions(t *testing.T) {
	tr := New(t.TempDir())
	_, err := tr.Put(context.Background(), model.Session{Tool: model.ToolClaude, ID: "x", Live: &model.LiveState{PID: 1}})
	if !errors.Is(err, ErrLive) {
		t.Errorf("err = %v", err)
	}
}

func TestFailedMoveRollsBack(t *testing.T) {
	transcript := filepath.Join(t.TempDir(), "gone.jsonl") // does not exist
	tr := New(filepath.Join(t.TempDir(), "trash"))
	if _, err := tr.Put(context.Background(), model.Session{Tool: model.ToolClaude, ID: "gone", Path: transcript}); err == nil {
		t.Fatal("expected an error")
	}
	if list, _ := tr.List(); len(list) != 0 {
		t.Errorf("failed trash left an entry: %+v", list)
	}
}

func TestCodexUsesCodexCommands(t *testing.T) {
	ctx := context.Background()
	var ran []string
	tr := New(filepath.Join(t.TempDir(), "trash"))
	tr.CodexBin = "true" // any executable on PATH; Run below never starts it
	tr.Run = func(c *exec.Cmd) ([]byte, error) {
		if c.Args[1] == "archive" {
			e, err := tr.Find(c.Args[2])
			if err != nil || !e.Pending {
				t.Fatalf("archive started without a recovery record: %+v, %v", e, err)
			}
		}
		ran = append(ran, strings.Join(c.Args[1:], " "))
		return nil, nil
	}
	s := model.Session{Tool: model.ToolCodex, ID: "c0de", Title: "Webhook retries"}
	e, err := tr.Put(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if e.Method != MethodCodexArchive {
		t.Errorf("method = %s", e.Method)
	}
	if err := tr.Restore(ctx, e); err != nil {
		t.Fatal(err)
	}
	e, _ = tr.Put(ctx, s)
	if err := tr.Delete(ctx, e); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ran, " | "); got != "archive c0de | unarchive c0de | archive c0de | delete c0de --force" {
		t.Errorf("codex commands: %s", got)
	}

	tr.Run = func(*exec.Cmd) ([]byte, error) { return []byte("thread not found\n"), errors.New("exit status 1") }
	if _, err := tr.Put(ctx, model.Session{Tool: model.ToolCodex, ID: "nope"}); err == nil || !strings.Contains(err.Error(), "thread not found") {
		t.Errorf("codex failure should surface its output: %v", err)
	}
	list, err := tr.List()
	if err != nil || len(list) != 1 || !list[0].Pending {
		t.Fatalf("failed archive must retain pending recovery record: %+v, %v", list, err)
	}
	tr.Run = func(*exec.Cmd) ([]byte, error) { t.Fatal("must not delete uncertain archive"); return nil, nil }
	if err := tr.Delete(ctx, list[0]); err == nil {
		t.Fatal("pending archive was deleted")
	}
	tr.Run = func(c *exec.Cmd) ([]byte, error) {
		if strings.Join(c.Args[1:], " ") != "unarchive nope" {
			t.Fatal(c.Args)
		}
		return nil, nil
	}
	if err := tr.Restore(ctx, list[0]); err != nil {
		t.Fatal(err)
	}
	if list, _ := tr.List(); len(list) != 0 {
		t.Fatal("restore left record")
	}
}

func TestCodexDoesNotArchiveWithoutRecoveryRecord(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	writeFile(t, blocked, "not a directory")
	tr := New(blocked)
	tr.CodexBin = "true"
	tr.Run = func(*exec.Cmd) ([]byte, error) { t.Fatal("archive ran without recovery record"); return nil, nil }
	if _, err := tr.Put(context.Background(), model.Session{Tool: model.ToolCodex, ID: "x"}); err == nil {
		t.Fatal("expected manifest write failure")
	}
}

func TestCodexFinalizeFailureKeepsRecoveryRecord(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}
	tr := New(t.TempDir())
	tr.CodexBin = "true"
	tr.Run = func(c *exec.Cmd) ([]byte, error) {
		dir := tr.entryDir(model.ToolCodex, c.Args[2])
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
		return nil, nil
	}
	e, err := tr.Put(context.Background(), model.Session{Tool: model.ToolCodex, ID: "x"})
	if err == nil || !strings.Contains(err.Error(), "could not finalize") || !e.Pending {
		t.Fatalf("finalization = %+v, %v", e, err)
	}
	saved, err := tr.Find("x")
	if err != nil || !saved.Pending {
		t.Fatalf("lost recovery record: %+v, %v", saved, err)
	}
	if err := os.Chmod(saved.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	tr.Run = func(c *exec.Cmd) ([]byte, error) {
		if c.Args[1] != "unarchive" {
			t.Fatal(c.Args)
		}
		return nil, nil
	}
	if err := tr.Restore(context.Background(), saved); err != nil {
		t.Fatal(err)
	}
}
