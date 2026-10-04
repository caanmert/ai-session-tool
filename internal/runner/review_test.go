package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func reviewGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitOutput(context.Background(), dir, args...)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func reviewWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func reviewCommit(t *testing.T, dir string) {
	t.Helper()
	reviewGit(t, dir, "add", "--all")
	reviewGit(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "-qm", "test change")
}

func reviewRun(t *testing.T) Run {
	t.Helper()
	repo := repository(t)
	for _, file := range []string{"committed.txt", "working.txt", "deleted.txt"} {
		reviewWrite(t, repo, file, "base\n")
	}
	reviewWrite(t, repo, ".gitignore", "*.log\n")
	reviewCommit(t, repo)
	base := reviewGit(t, repo, "rev-parse", "HEAD")
	path := filepath.Join(t.TempDir(), "task")
	reviewGit(t, repo, "worktree", "add", "-b", "review-task", path, base)
	return Run{ID: "ais-test", Repository: repo, Base: base, Tasks: []Task{{Window: "task-1", Branch: "review-task", Path: path}}}
}

func TestReviewIncludesAllWorkWithoutChangingIndex(t *testing.T) {
	ctx := context.Background()
	r := reviewRun(t)
	path := r.Tasks[0].Path
	reviewWrite(t, path, "committed.txt", "committed change\n")
	reviewCommit(t, path)
	reviewWrite(t, path, "staged.txt", "staged change\n")
	reviewGit(t, path, "add", "staged.txt")
	reviewWrite(t, path, "working.txt", "working change\n\n")
	if err := os.Remove(filepath.Join(path, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	unusual := "new \t\nfile "
	reviewWrite(t, path, unusual, "untracked change\n\n")
	reviewWrite(t, path, "binary", "\x00\x01\x02")
	reviewWrite(t, path, "empty", "")
	reviewWrite(t, path, "ignored.log", "must not appear")
	reviewWrite(t, filepath.Dir(path), "outside", "symlink target content must not be read")
	if err := os.Symlink("../outside", filepath.Join(path, "link")); err != nil {
		t.Fatal(err)
	}
	indexPath := reviewGit(t, path, "rev-parse", "--git-path", "index")
	// Review must not run external diff drivers from repository configuration.
	reviewGit(t, path, "config", "diff.external", "false")
	before, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	m := New(t.TempDir())
	// A stopped run can still be reviewed without tmux.
	m.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	status, err := m.Status(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	st := status.Tasks[0]
	if status.Warning == "" || st.State != "unknown" || st.Error != "" || st.CurrentBranch != "review-task" {
		t.Fatalf("status: %+v", status)
	}
	got := map[string]string{}
	for _, f := range st.Files {
		got[f.Path] = f.Status
	}
	want := map[string]string{"committed.txt": "M", "staged.txt": "A", "working.txt": "M", "deleted.txt": "D", unusual: "?", "binary": "?", "empty": "?", "link": "?"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("files = %#v, want %#v", got, want)
	}
	diff, err := m.Diff(ctx, r, r.Tasks[0], false)
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"+committed change", "+staged change", "+working change", "+untracked change", "deleted file mode", "Binary files", "b/empty", "+../outside", "new file mode 120000"} {
		if !strings.Contains(diff, content) {
			t.Errorf("patch missing %q:\n%s", content, diff)
		}
	}
	if strings.Contains(diff, "must not") {
		t.Fatal("included ignored file or symlink target")
	}
	stat, err := m.Diff(ctx, r, r.Tasks[0], true)
	if err != nil || !strings.Contains(stat, "committed.txt") || strings.Contains(stat, "@@") {
		t.Fatalf("stat = %q, %v", stat, err)
	}
	after, err := os.ReadFile(indexPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("review changed the Git index", err)
	}
}

func TestReviewMissingAndWrongWorktrees(t *testing.T) {
	r := reviewRun(t)
	m := New(t.TempDir())
	m.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	r.Tasks = append(r.Tasks, Task{Window: "task-2", Path: filepath.Join(t.TempDir(), "missing")})
	res, err := m.Status(context.Background(), r)
	if err != nil || res.Tasks[0].Error != "" || res.Tasks[1].Error == "" {
		t.Fatalf("partial status = %+v, %v", res, err)
	}
	if _, err := m.Diff(context.Background(), r, r.Tasks[1], false); err == nil {
		t.Fatal("missing worktree diff succeeded")
	}
	nested := filepath.Join(r.Repository, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Diff(context.Background(), r, Task{Path: nested}, false); err == nil {
		t.Fatal("diff silently inspected parent repository")
	}
	r.Base = "missing-commit"
	if _, err := m.Diff(context.Background(), r, r.Tasks[0], false); err == nil {
		t.Fatal("missing base succeeded")
	}
}

func TestReviewDetachedAndClean(t *testing.T) {
	r := reviewRun(t)
	reviewGit(t, r.Tasks[0].Path, "checkout", "--detach")
	m := New(t.TempDir())
	m.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	res, err := m.Status(context.Background(), r)
	if err != nil || res.Tasks[0].CurrentBranch != "(detached)" || len(res.Tasks[0].Files) != 0 {
		t.Fatalf("detached = %+v, %v", res, err)
	}
	diff, err := m.Diff(context.Background(), r, r.Tasks[0], false)
	if err != nil || diff != "" {
		t.Fatalf("clean diff = %q, %v", diff, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Status(ctx, r); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled status = %v", err)
	}
}

func TestDiffFileUsesLiteralPaths(t *testing.T) {
	r := reviewRun(t)
	path := r.Tasks[0].Path
	for _, name := range []string{"pick[1].txt", "pick1.txt", ":(glob)*.txt", "line\nname"} {
		reviewWrite(t, path, name, "content of "+name+"\n")
	}
	reviewCommit(t, path)
	reviewWrite(t, path, "untracked[1]", "untracked selected\n")
	reviewWrite(t, path, "untracked1", "untracked sibling\n")
	m := New(t.TempDir())
	for _, name := range []string{"pick[1].txt", ":(glob)*.txt", "line\nname"} {
		diff, err := m.DiffFile(context.Background(), r, r.Tasks[0], name)
		if err != nil || !strings.Contains(diff, "+content of "+strings.ReplaceAll(name, "\n", "\n+")) || strings.Count(diff, "diff --git") != 1 {
			t.Fatalf("%q: %q, %v", name, diff, err)
		}
	}
	diff, err := m.DiffFile(context.Background(), r, r.Tasks[0], "untracked[1]")
	if err != nil || !strings.Contains(diff, "+untracked selected") || strings.Contains(diff, "sibling") {
		t.Fatalf("untracked: %q, %v", diff, err)
	}
	for _, name := range []string{"../outside", "/tmp/outside"} {
		if _, err := m.DiffFile(context.Background(), r, r.Tasks[0], name); err == nil {
			t.Fatal("accepted outside path", name)
		}
	}
}

func TestPaneStatusAndTaskAttach(t *testing.T) {
	m := New(t.TempDir())
	m.LookPath = func(string) (string, error) { return "/test/tmux", nil }
	r := Run{ID: "ais-test", Tasks: []Task{{Window: "task-1"}, {Window: "task-2"}}}
	m.Command = func(_ context.Context, _, _ string, args ...string) (string, error) {
		if args[0] != "list-panes" || args[3] != "=ais-test" {
			t.Fatal(args)
		}
		return "%1\t0\t\t\t1\t0\ttask-1\trenamed\n%2\t1\t7\t\t0\t1\t\ttask-2", nil
	}
	states, err := m.panes(context.Background(), r)
	if err != nil || states["task-1"].State != "running" || !states["task-1"].Attention || states["task-1"].Quiet {
		t.Fatalf("states = %+v, %v", states, err)
	}
	st := states["task-2"]
	if st.State != "exited" || st.ExitCode == nil || *st.ExitCode != 7 || !st.Quiet {
		t.Fatal(st)
	}
	for _, inside := range []bool{true, false} {
		cmd, err := m.TaskAttachCmd(context.Background(), r, "task-1", inside)
		if err != nil || cmd.Args[len(cmd.Args)-1] != "%1" {
			t.Fatalf("attach = %v, %v", cmd, err)
		}
	}
	if _, err := m.TaskAttachCmd(context.Background(), r, "task-99", false); err == nil {
		t.Fatal("unknown task accepted")
	}
	for _, tc := range []struct {
		out         string
		err         error
		unavailable bool
	}{
		{err: errors.New("tmux list-panes: can't find session: ais-test")},
		{err: errors.New("tmux list-panes: can't find window: ais-test")},
		{err: errors.New("tmux list-panes: no server running on /tmp/socket")},
		{err: errors.New("tmux list-panes: error connecting to /tmp/socket (No such file or directory)")},
		{err: errors.New("tmux list-panes: error connecting to /tmp/socket (Permission denied)"), unavailable: true},
		{out: "malformed", unavailable: true},
	} {
		m.Command = func(context.Context, string, string, ...string) (string, error) { return tc.out, tc.err }
		got, err := m.panes(context.Background(), r)
		if (err != nil) != tc.unavailable || len(got) != 0 {
			t.Fatalf("%+v => %v, %v", tc, got, err)
		}
	}
}

func TestTmuxReviewStatus(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	r := reviewRun(t)
	socket := fmt.Sprintf("ais-review-%d-%d", os.Getpid(), time.Now().UnixNano())
	m := New(t.TempDir())
	m.Command = func(ctx context.Context, cwd, bin string, args ...string) (string, error) {
		if bin == "tmux" {
			args = append([]string{"-L", socket, "-f", "/dev/null"}, args...)
		}
		return command(ctx, cwd, bin, args...)
	}
	ctx := context.Background()
	runTmux := func(args ...string) string {
		out, err := m.Command(ctx, "", "tmux", args...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	t.Cleanup(func() { _, _ = m.Command(ctx, "", "tmux", "kill-server") })
	pane := runTmux("new-session", "-d", "-s", r.ID, "-n", "renamed", "-P", "-F", "#{pane_id}", "sleep", "60")
	runTmux("set-option", "-p", "-t", pane, "@ais_task", "task-1")
	runTmux("set-option", "-w", "-t", pane, "remain-on-exit", "on")
	res, err := m.Status(ctx, r)
	if err != nil || res.Warning != "" || res.Tasks[0].State != "running" || res.Tasks[0].Pane != pane {
		t.Fatalf("running = %+v, %v", res, err)
	}
	runTmux("respawn-pane", "-k", "-t", pane, "sh", "-c", "exit 7")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		res, err = m.Status(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		if res.Tasks[0].State == "exited" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	st := res.Tasks[0]
	if st.State != "exited" || st.ExitCode == nil || *st.ExitCode != 7 {
		t.Fatalf("exited = %+v", st)
	}
	runTmux("new-session", "-d", "-s", "another-session", "sleep", "60")
	runTmux("kill-session", "-t", "="+r.ID)
	res, err = m.Status(ctx, r)
	if err != nil || res.Warning != "" || res.Tasks[0].State != "missing" || res.Tasks[0].Error != "" {
		t.Fatalf("stopped = %+v, %v", res, err)
	}
}
