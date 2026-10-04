package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/caanmert/ai-session-tool/internal/runner"
)

func TestRunReviewCommands(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = repo
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	git("-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "base")
	r := runner.Run{ID: "ais-review", Repository: repo, Base: git("rev-parse", "HEAD")}
	for _, name := range []string{"task-1", "task-2"} {
		path := filepath.Join(dir, name)
		git("worktree", "add", "-b", name, path, r.Base)
		if err := os.WriteFile(filepath.Join(path, name+".txt"), []byte("work from "+name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		r.Tasks = append(r.Tasks, runner.Task{Window: name, Branch: name, Path: path})
	}
	dataDir := filepath.Join(dir, "data")
	manifestDir := filepath.Join(dataDir, "runs", r.ID)
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "run.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncat <<'PANES'\n%11\t0\t\t\t1\t0\ttask-1\trenamed\n%12\t1\t0\t\t0\t0\ttask-2\ttask-2\nPANES\n"
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	runReview := func(args ...string) (string, error) {
		return runIn(t, filepath.Join(dir, "index.db"), dataDir, append([]string{"run"}, args...)...)
	}
	out, err := runReview("status", "ais-rev", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var status runner.RunStatus
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Tasks) != 2 || !status.Tasks[0].Attention || status.Tasks[1].State != "exited" || len(status.Tasks[0].Files) != 1 {
		t.Fatal(out)
	}
	out, err = runReview("status", r.ID, "--task", "task-1")
	if err != nil || !strings.Contains(out, "event") || !strings.Contains(out, "Attach: ais run attach ais-review --task task-1") || strings.Contains(out, "task-2") {
		t.Fatalf("status = %s, %v", out, err)
	}
	out, err = runReview("diff", r.ID, "--task", "task-1")
	if err != nil || !strings.Contains(out, "+work from task-1") || strings.Contains(out, "task-2") {
		t.Fatalf("diff = %s, %v", out, err)
	}
	out, err = runReview("diff", r.ID, "--stat")
	if err != nil || !strings.Contains(out, "task-1.txt") || !strings.Contains(out, "task-2.txt") || strings.Contains(out, "@@") {
		t.Fatalf("stat = %s, %v", out, err)
	}
	for _, sub := range []string{"status", "diff", "attach", "review"} {
		if _, err := runReview(sub, r.ID, "--task", "task-99"); err == nil {
			t.Fatalf("%s accepted unknown task", sub)
		}
	}
	if _, err := runReview("review", r.ID); err == nil || !strings.Contains(err.Error(), "requires a terminal") {
		t.Fatalf("non-terminal review: %v", err)
	}
	for _, interval := range []string{"-1s", "10ms"} {
		if _, err := runReview("review", r.ID, "--refresh", interval); err == nil || !strings.Contains(err.Error(), "refresh must") {
			t.Fatalf("invalid refresh: %v", err)
		}
	}
	if err := os.Rename(r.Tasks[1].Path, r.Tasks[1].Path+".moved"); err != nil {
		t.Fatal(err)
	}
	out, err = runReview("diff", r.ID)
	if err == nil || !strings.Contains(out, "+work from task-1") || !strings.Contains(out, "Could not read diff") {
		t.Fatalf("partial diff = %s, %v", out, err)
	}
}
