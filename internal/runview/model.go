// Package runview provides the interactive review screen for parallel runs.
package runview

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/caanmert/ai-session-tool/internal/launch"
	"github.com/caanmert/ai-session-tool/internal/runner"
	"github.com/caanmert/ai-session-tool/internal/ui"
)

type Deps struct {
	Context     context.Context
	Run         runner.Run
	InitialTask string
	Refresh     time.Duration
	Theme       *ui.Theme
	Status      func(context.Context) (runner.RunStatus, error)
	Diff        func(context.Context, runner.Task, string) (string, error)
	Attach      func(context.Context, string) (*exec.Cmd, error)
	Exec        func(*exec.Cmd, tea.ExecCallback) tea.Cmd
}

func Run(ctx context.Context, d Deps, in io.Reader, out io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	d.Context = ctx
	if d.Theme != nil {
		d.Theme.Prime()
	}
	_, err := tea.NewProgram(New(d), tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	if ctx.Err() != nil {
		return nil
	}
	return err
}

type focus int

const (
	tasksFocus focus = iota
	filesFocus
	diffFocus
)

type Model struct {
	d                               Deps
	width, height                   int
	status                          runner.RunStatus
	task, file, taskTop, fileTop    int
	focus                           focus
	loading, diffLoading, attaching bool
	statusSeq, diffSeq              int
	diffCancel                      context.CancelFunc
	diffKey, diffText               string
	diffError, statusError, notice  string
	preview                         viewport.Model
	help                            bool
}

type statusMsg struct {
	seq    int
	status runner.RunStatus
	err    error
}
type refreshMsg struct{ seq int }
type diffMsg struct {
	seq  int
	text string
	err  error
}
type attachMsg struct {
	cmd *exec.Cmd
	err error
}
type attachedMsg struct{ err error }

func New(d Deps) Model {
	if d.Context == nil {
		d.Context = context.Background()
	}
	if d.Theme == nil {
		d.Theme = ui.New(io.Discard, ui.Never)
	}
	if d.Exec == nil {
		d.Exec = tea.ExecProcess
	}
	m := Model{d: d, loading: true, statusSeq: 1, preview: viewport.New(0, 0)}
	for i, t := range d.Run.Tasks {
		m.status.Tasks = append(m.status.Tasks, runner.TaskStatus{Task: t, State: "unknown"})
		if t.Window == d.InitialTask {
			m.task = i
		}
	}
	return m
}

func (m Model) Init() tea.Cmd { return m.statusCmd() }

func (m Model) statusCmd() tea.Cmd {
	seq, load, parent := m.statusSeq, m.d.Status, m.d.Context
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 15*time.Second)
		defer cancel()
		res, err := load(ctx)
		return statusMsg{seq, res, err}
	}
}

func (m *Model) refresh() tea.Cmd {
	if m.loading || m.attaching {
		return nil
	}
	m.loading = true
	m.statusSeq++
	return m.statusCmd()
}

func (m Model) current() (runner.TaskStatus, bool) {
	if m.task < 0 || m.task >= len(m.status.Tasks) {
		return runner.TaskStatus{}, false
	}
	return m.status.Tasks[m.task], true
}

func (m Model) filename() string {
	st, ok := m.current()
	if !ok || m.file <= 0 || m.file > len(st.Files) {
		return ""
	}
	return st.Files[m.file-1].Path
}

func (m *Model) loadDiff() tea.Cmd {
	st, ok := m.current()
	if !ok {
		if m.diffCancel != nil {
			m.diffCancel()
		}
		m.diffSeq++
		m.diffKey, m.diffText, m.diffError = "", "", ""
		m.diffLoading = false
		m.preview.SetContent("No task selected.")
		return nil
	}
	file := m.filename()
	key := st.Task.Window + "\x00" + file
	if m.diffLoading && m.diffKey == key {
		return nil
	}
	if m.diffCancel != nil {
		m.diffCancel()
	}
	m.diffSeq++
	if m.diffKey != key {
		m.diffText = ""
		m.preview.GotoTop()
		m.preview.SetXOffset(0)
	}
	m.diffKey, m.diffError = key, ""
	if st.Error != "" {
		m.diffLoading, m.diffError = false, st.Error
		m.renderDiff()
		return nil
	}
	m.diffLoading = true
	m.renderDiff()
	ctx, cancel := context.WithTimeout(m.d.Context, 15*time.Second)
	m.diffCancel = cancel
	seq, load := m.diffSeq, m.d.Diff
	return func() tea.Msg { defer cancel(); text, err := load(ctx, st.Task, file); return diffMsg{seq, text, err} }
}

func (m *Model) attach() tea.Cmd {
	st, ok := m.current()
	if !ok || m.attaching {
		return nil
	}
	m.attaching, m.notice = true, "Opening "+st.Task.Window+"…"
	build, parent := m.d.Attach, m.d.Context
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 5*time.Second)
		defer cancel()
		cmd, err := build(ctx, st.Task.Window)
		if err == nil {
			err = launch.Check(cmd)
		}
		return attachMsg{cmd, err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
	case refreshMsg:
		if msg.seq == m.statusSeq {
			return m, m.refresh()
		}
	case statusMsg:
		if msg.seq != m.statusSeq {
			return m, nil
		}
		m.loading = false
		var next tea.Cmd
		if m.d.Refresh > 0 {
			seq := m.statusSeq
			next = tea.Tick(m.d.Refresh, func(time.Time) tea.Msg { return refreshMsg{seq} })
		}
		if msg.err != nil {
			m.statusError = msg.err.Error()
			return m, next
		}
		old, _ := m.current()
		file := m.filename()
		m.status, m.statusError = msg.status, ""
		m.task = 0
		for i, st := range m.status.Tasks {
			if st.Task.Window == old.Task.Window {
				m.task = i
				break
			}
		}
		m.file = 0
		if st, ok := m.current(); ok && file != "" {
			for i, f := range st.Files {
				if f.Path == file {
					m.file = i + 1
					break
				}
			}
		}
		m.resize()
		return m, tea.Batch(next, m.loadDiff())
	case diffMsg:
		if msg.seq != m.diffSeq {
			return m, nil
		}
		m.diffLoading = false
		m.diffText = limitDiff(msg.text)
		m.diffError = ""
		if msg.err != nil {
			m.diffError = msg.err.Error()
		}
		m.renderDiff()
	case attachMsg:
		if msg.err != nil {
			m.attaching = false
			m.notice = "Could not attach: " + msg.err.Error()
			return m, m.refresh()
		}
		return m, m.d.Exec(msg.cmd, func(err error) tea.Msg { return attachedMsg{err} })
	case attachedMsg:
		m.attaching = false
		m.notice = "Back in review"
		if msg.err != nil {
			m.notice = "Attach failed: " + msg.err.Error()
		}
		return m, m.refresh()
	case tea.KeyMsg:
		return m.keys(msg)
	}
	return m, nil
}

func (m Model) keys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "q", "ctrl+c":
		if m.diffCancel != nil {
			m.diffCancel()
		}
		return m, tea.Quit
	case "?":
		m.help = !m.help
		return m, nil
	case "esc":
		m.help = false
		m.focus = tasksFocus
		return m, nil
	}
	if m.help || m.attaching {
		return m, nil
	}
	switch key {
	case "tab":
		m.focus = (m.focus + 1) % 3
		return m, nil
	case "shift+tab":
		m.focus = (m.focus + 2) % 3
		return m, nil
	case "r", "ctrl+r":
		m.notice = ""
		return m, m.refresh()
	case "a":
		return m, m.attach()
	case "enter":
		if m.focus == filesFocus {
			m.focus = diffFocus
			return m, nil
		}
		return m, m.attach()
	}
	if m.focus == diffFocus {
		switch key {
		case "j", "down":
			m.preview.ScrollDown(1)
		case "k", "up":
			m.preview.ScrollUp(1)
		case "pgdown", "ctrl+d", " ":
			m.preview.HalfPageDown()
		case "pgup", "ctrl+u":
			m.preview.HalfPageUp()
		case "g", "home":
			m.preview.GotoTop()
		case "G", "end":
			m.preview.GotoBottom()
		case "h", "left":
			m.preview.ScrollLeft(8)
		case "l", "right":
			m.preview.ScrollRight(8)
		}
		return m, nil
	}
	delta := 0
	switch key {
	case "j", "down":
		delta = 1
	case "k", "up":
		delta = -1
	case "pgdown", "ctrl+d":
		delta = 5
	case "pgup", "ctrl+u":
		delta = -5
	case "g", "home":
		delta = -1000000
	case "G", "end":
		delta = 1000000
	}
	if delta == 0 {
		return m, nil
	}
	oldTask, oldFile := m.task, m.file
	if m.focus == tasksFocus {
		m.task = min(max(m.task+delta, 0), max(len(m.status.Tasks)-1, 0))
		if m.task != oldTask {
			m.file = 0
			m.fileTop = 0
		}
	} else if st, ok := m.current(); ok {
		m.file = min(max(m.file+delta, 0), len(st.Files))
	}
	m.resize()
	if oldTask != m.task || oldFile != m.file {
		return m, m.loadDiff()
	}
	return m, nil
}

func paneLabel(st runner.TaskStatus) string {
	label := st.State
	if st.ExitCode != nil {
		label += fmt.Sprintf(" (%d)", *st.ExitCode)
	}
	if st.Signal != "" {
		label += " signal " + st.Signal
	}
	if st.Attention {
		label += " · event"
	}
	if st.Quiet {
		label += " · silence"
	}
	return label
}
