package runview

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/caanmert/ai-session-tool/internal/runner"
	"github.com/caanmert/ai-session-tool/internal/ui"
)

type harness struct {
	t        *testing.T
	m        Model
	attached []string
	diffs    []string
	snapshot runner.RunStatus
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t}
	r := runner.Run{ID: "ais-review", Base: strings.Repeat("a", 40), Tasks: []runner.Task{
		{Window: "task-1", Branch: "ais-review/task-1", Path: "/work/task-1"},
		{Window: "task-2", Branch: "ais-review/task-2", Path: "/work/task-2"},
	}}
	h.snapshot = runner.RunStatus{ID: r.ID, Base: r.Base, Tasks: []runner.TaskStatus{
		{Task: r.Tasks[0], State: "running", Attention: true, CurrentBranch: r.Tasks[0].Branch, Files: []runner.FileChange{{Status: "M", Path: "main.go"}, {Status: "?", Path: "a\nfile.txt"}}},
		{Task: r.Tasks[1], State: "exited", Quiet: true, CurrentBranch: r.Tasks[1].Branch, Files: []runner.FileChange{{Status: "A", Path: "README.md"}}},
	}}
	h.m = New(Deps{Run: r, Theme: ui.New(nil, ui.Never),
		Status: func(context.Context) (runner.RunStatus, error) {
			res := h.snapshot
			res.Tasks = append([]runner.TaskStatus{}, h.snapshot.Tasks...)
			for i := range res.Tasks {
				res.Tasks[i].Files = append([]runner.FileChange{}, res.Tasks[i].Files...)
			}
			return res, nil
		},
		Diff: func(_ context.Context, t runner.Task, file string) (string, error) {
			h.diffs = append(h.diffs, t.Window+":"+file)
			return "diff --git a/file b/file\n--- a/file\n+++ b/file\n@@ -1 +1 @@\n-old\n+" + t.Window + ":" + file + "\n" + strings.Repeat(" context line\n", 80), nil
		},
		Attach: func(_ context.Context, task string) (*exec.Cmd, error) {
			h.attached = append(h.attached, task)
			return exec.Command("true"), nil
		},
		Exec: func(_ *exec.Cmd, cb tea.ExecCallback) tea.Cmd { return func() tea.Msg { return cb(nil) } },
	})
	h.send(tea.WindowSizeMsg{Width: 120, Height: 30})
	h.drain(h.m.Init())
	return h
}

func (h *harness) send(msg tea.Msg) tea.Cmd {
	h.t.Helper()
	m, cmd := h.m.Update(msg)
	h.m = m.(Model)
	return cmd
}

func (h *harness) drain(cmd tea.Cmd) {
	h.t.Helper()
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			h.drain(c)
		}
		return
	}
	h.drain(h.send(msg))
}

func (h *harness) key(key string) tea.Cmd {
	h.t.Helper()
	types := map[string]tea.KeyType{"tab": tea.KeyTab, "enter": tea.KeyEnter, "down": tea.KeyDown, "up": tea.KeyUp, "esc": tea.KeyEsc}
	if kind, ok := types[key]; ok {
		return h.send(tea.KeyMsg{Type: kind})
	}
	return h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}

func TestNavigateFilesAndAttach(t *testing.T) {
	h := newHarness(t)
	if h.m.loading || h.m.diffLoading || !strings.Contains(h.m.diffText, "task-1:") {
		t.Fatal("initial load failed")
	}
	h.drain(h.key("down"))
	if h.m.task != 1 || !strings.Contains(h.m.diffText, "task-2:") {
		t.Fatal("task selection failed")
	}
	h.drain(h.key("tab"))
	h.drain(h.key("down"))
	if h.m.filename() != "README.md" || h.diffs[len(h.diffs)-1] != "task-2:README.md" {
		t.Fatal(h.diffs)
	}
	h.drain(h.key("enter"))
	if h.m.focus != diffFocus || len(h.attached) != 0 {
		t.Fatal("file enter should focus diff")
	}
	h.drain(h.key("j"))
	if h.m.preview.YOffset == 0 {
		t.Fatal("diff did not scroll")
	}
	h.drain(h.key("a"))
	if len(h.attached) != 1 || h.attached[0] != "task-2" || h.m.attaching {
		t.Fatal("attach/return failed", h.attached)
	}
	if h.m.task != 1 || h.m.filename() != "README.md" {
		t.Fatal("attach return lost selection")
	}
	if h.m.notice != "Back in review" {
		t.Fatal(h.m.notice)
	}
}

func TestRefreshPreservesSelectionAndScroll(t *testing.T) {
	h := newHarness(t)
	h.drain(h.key("tab"))
	h.drain(h.key("down"))
	h.drain(h.key("enter"))
	h.m.preview.ScrollDown(10)
	offset := h.m.preview.YOffset
	h.snapshot.Tasks[0].Files = []runner.FileChange{{Status: "?", Path: "before.txt"}, {Status: "M", Path: "main.go"}}
	h.drain(h.key("r"))
	if h.m.filename() != "main.go" || h.m.file != 2 || h.m.preview.YOffset != offset {
		t.Fatal("refresh moved selection or scroll", h.m.file, h.m.preview.YOffset)
	}
	h.snapshot.Tasks[0].Files = nil
	h.drain(h.key("r"))
	if h.m.file != 0 || h.m.preview.YOffset != 0 {
		t.Fatal("removed file did not return to all changes")
	}
}

func TestStaleDiffAndCancellation(t *testing.T) {
	h := newHarness(t)
	first := h.key("down") // task 2 diff is pending
	seq := h.m.diffSeq
	second := h.key("up") // task 1 supersedes it
	h.drain(second)
	before := h.m.diffText
	h.send(diffMsg{seq: seq, text: "stale task 2"})
	if h.m.diffText != before {
		t.Fatal("stale diff replaced selected task")
	}
	_ = first // simulated request is intentionally never executed
	var captured context.Context
	h.m.d.Diff = func(ctx context.Context, _ runner.Task, _ string) (string, error) {
		captured = ctx
		return "", ctx.Err()
	}
	cmd := h.key("down")
	h.key("q")
	msg := cmd().(diffMsg)
	if captured == nil || !errors.Is(msg.err, context.Canceled) {
		t.Fatalf("quit did not cancel diff: %v", msg.err)
	}
}

func TestRefreshSchedulingAndErrors(t *testing.T) {
	h := newHarness(t)
	h.m.d.Refresh = time.Second
	oldSeq := h.m.statusSeq
	cmd := h.key("r")
	if !h.m.loading || cmd == nil {
		t.Fatal("refresh did not start")
	}
	if next := h.send(refreshMsg{seq: oldSeq}); next != nil {
		t.Fatal("stale timer started another scan")
	}
	if next := h.key("r"); next != nil {
		t.Fatal("overlapping scan")
	}
	h.send(statusMsg{seq: h.m.statusSeq, err: errors.New("git unavailable")})
	if h.m.loading || !strings.Contains(h.m.View(), "Refresh failed") || len(h.m.status.Tasks) != 2 {
		t.Fatal("refresh failure hid previous snapshot")
	}
	h.m.d.Attach = func(context.Context, string) (*exec.Cmd, error) { return nil, errors.New("pane gone") }
	attach := h.key("a")
	h.send(attach())
	if h.m.attaching || !strings.Contains(h.m.notice, "pane gone") || !h.m.loading {
		t.Fatal("attach failure did not restore refresh")
	}
}

func TestRenderingSizesAndControlCharacters(t *testing.T) {
	h := newHarness(t)
	for _, size := range [][2]int{{120, 30}, {80, 24}, {30, 12}, {20, 5}, {1, 1}, {0, 0}} {
		h.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := h.m.View()
		if size[0] == 0 {
			if view != "" {
				t.Fatal(view)
			}
			continue
		}
		lines := strings.Split(view, "\n")
		if len(lines) != size[1] {
			t.Fatalf("%v: got %d rows", size, len(lines))
		}
		for _, line := range lines {
			if ui.Width(line) > size[0] {
				t.Fatalf("%v: row too wide: %q", size, line)
			}
		}
	}
	h.send(tea.WindowSizeMsg{Width: 120, Height: 30})
	h.send(diffMsg{seq: h.m.diffSeq, text: "+bad\x1b]52;c;payload\a\rtext\n"})
	view := h.m.View()
	if strings.ContainsAny(view, "\x1b\a\r") || !strings.Contains(view, `\x1b`) {
		t.Fatalf("unsafe patch rendered: %q", view)
	}
	h.key("?")
	if !strings.Contains(h.m.View(), "Review never") && !strings.Contains(h.m.View(), "without staging") {
		t.Fatal("help missing")
	}
}

func TestPreviewLimitsAndUnavailableWorktree(t *testing.T) {
	h := newHarness(t)
	h.send(diffMsg{seq: h.m.diffSeq, text: strings.Repeat("+line\n", maxDiffLines+100)})
	if !strings.Contains(h.m.diffText, "Preview truncated") || len(strings.Split(h.m.diffText, "\n")) > maxDiffLines+5 {
		t.Fatal("line limit missing")
	}
	if got := limitDiff(strings.Repeat("x", maxDiffBytes+100)); len(got) > maxDiffBytes+150 || !strings.Contains(got, "Preview truncated") {
		t.Fatal("byte limit missing")
	}
	h.snapshot.Tasks[0].Error = "worktree missing"
	h.drain(h.key("r"))
	if !strings.Contains(h.m.View(), "worktree missing") {
		t.Fatal("worktree error hidden")
	}
	h.snapshot.Tasks = nil
	h.drain(h.key("r"))
	if !strings.Contains(h.m.View(), "No task selected") || h.m.diffText != "" {
		t.Fatal("empty run retained old patch")
	}
}

func TestInitialTask(t *testing.T) {
	h := newHarness(t)
	d := h.m.d
	d.InitialTask = "task-2"
	m := New(d)
	st, ok := m.current()
	if !ok || st.Task.Window != "task-2" {
		t.Fatal(fmt.Sprint(st))
	}
}
