// Package tui is the interactive session browser: a filterable list of
// every session on top, a live preview of the selected one below, and
// keys to resume, fork or start sessions without leaving the terminal.
package tui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/caanmert/ai-session-tool/internal/launch"
	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
	"github.com/caanmert/ai-session-tool/internal/render"
	"github.com/caanmert/ai-session-tool/internal/ui"
)

// Deps are the TUI's connections to the outside world, swappable in tests.
type Deps struct {
	Providers []provider.Provider
	Now       func() time.Time
	// Theme decides colors; build it for the real terminal so the color
	// profile matches (e.g. 256 colors in macOS Terminal).
	Theme *ui.Theme
	// Exec hands the terminal to a command and reports back when it exits.
	Exec func(*exec.Cmd, tea.ExecCallback) tea.Cmd
	// Copy puts text on the clipboard.
	Copy func(string) error
}

// Run starts the TUI and blocks until the user quits.
func Run(ctx context.Context, d Deps) error {
	if d.Theme != nil {
		d.Theme.Prime()
	}
	_, err := tea.NewProgram(New(d), tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	if err != nil && ctx.Err() != nil {
		return nil // interrupted
	}
	return err
}

type focus int

const (
	focusList focus = iota
	focusPreview
)

// maxPreviewMessages bounds how much of a long session the preview renders.
const maxPreviewMessages = 400

// Model is the Bubble Tea model.
type Model struct {
	d Deps

	width, height int

	sessions []model.Session
	view     []int // indexes into sessions that pass the filter
	cursor   int   // position in view
	top      int   // first list row on screen
	selected string

	loading  bool
	scanned  bool
	warnings int
	err      error
	spinner  spinner.Model

	filter    textinput.Model
	filtering bool
	query     query

	preview viewport.Model
	focus   focus
	cache   map[string]*previewEntry
	shownID string // session the preview currently shows
	shownW  int    // width it was rendered at

	status      string
	statusError bool
	statusSeq   int
	help        bool
}

type previewEntry struct {
	msgs     []model.Message
	err      error
	rendered map[int]string // by width
}

// New builds the model.
func New(d Deps) Model {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Exec == nil {
		d.Exec = tea.ExecProcess
	}
	if d.Theme == nil {
		d.Theme = ui.New(io.Discard, ui.Never)
	}
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "words, t:codex, p:api, b:main, is:live"
	return Model{
		d:       d,
		loading: true,
		spinner: sp,
		filter:  ti,
		preview: viewport.New(0, 0),
		cache:   map[string]*previewEntry{},
	}
}

// Messages.
type (
	scanMsg struct {
		res provider.ScanResult
		err error
	}
	transcriptMsg struct {
		id   string
		msgs []model.Message
		err  error
	}
	selectMsg     struct{ id string } // debounced: load the preview if still selected
	execDoneMsg   struct{ err error }
	clearStatusMg struct{ seq int }
)

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.scan())
}

func (m Model) scan() tea.Cmd {
	providers := m.d.Providers
	return func() tea.Msg {
		res, err := provider.Scan(context.Background(), providers)
		return scanMsg{res, err}
	}
}

func (m Model) loadTranscript(s model.Session) tea.Cmd {
	p := provider.For(m.d.Providers, s.Tool)
	return func() tea.Msg {
		if p == nil {
			return transcriptMsg{id: s.ID, err: fmt.Errorf("no provider for %s", s.Tool)}
		}
		msgs, err := p.Transcript(context.Background(), s)
		return transcriptMsg{id: s.ID, msgs: msgs, err: err}
	}
}

// current returns the selected session.
func (m Model) current() (model.Session, bool) {
	if m.cursor < 0 || m.cursor >= len(m.view) {
		return model.Session{}, false
	}
	return m.sessions[m.view[m.cursor]], true
}

// applyFilter recomputes the visible rows, keeping the selection on the
// same session when it is still visible.
func (m *Model) applyFilter() {
	m.view = m.view[:0]
	for i, s := range m.sessions {
		if m.query.match(s) {
			m.view = append(m.view, i)
		}
	}
	m.cursor = 0
	for i, idx := range m.view {
		if m.sessions[idx].ID == m.selected {
			m.cursor = i
			break
		}
	}
	m.clampScroll()
}

// layout sizes in rows.
func (m Model) layout() (listH, previewH int) {
	if m.height < 12 { // too short for a preview: header, list, footer
		return max(m.height-2, 1), 0
	}
	avail := m.height - 3 // header, separator, footer
	listH = min(max(len(m.view), 1), max(avail*2/5, 5))
	return listH, avail - listH
}

func (m *Model) clampScroll() {
	listH, _ := m.layout()
	m.cursor = min(max(m.cursor, 0), max(len(m.view)-1, 0))
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+listH {
		m.top = m.cursor - listH + 1
	}
	m.top = min(max(m.top, 0), max(len(m.view)-listH, 0))
}

// move changes the selection and schedules a preview load.
func (m *Model) move(delta int) tea.Cmd {
	m.cursor += delta
	m.clampScroll()
	return m.selectionChanged()
}

func (m *Model) selectionChanged() tea.Cmd {
	s, ok := m.current()
	if !ok {
		m.selected = ""
		m.refreshPreview()
		return nil
	}
	m.selected = s.ID
	m.refreshPreview()
	if _, cached := m.cache[s.ID]; cached {
		return nil
	}
	id := s.ID
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return selectMsg{id} })
}

// refreshPreview shows the selected session in the preview pane, rendering
// its transcript at the current width when it is loaded.
func (m *Model) refreshPreview() {
	_, previewH := m.layout()
	m.preview.Width, m.preview.Height = m.width, previewH
	s, ok := m.current()
	if !ok {
		m.preview.SetContent("")
		m.shownID = ""
		return
	}
	width := max(m.width-2, 20)
	if m.shownID == s.ID && m.shownW == width {
		return
	}
	t := m.d.Theme
	var b strings.Builder
	b.WriteString(render.Meta(t, m.d.Now(), s, width))
	b.WriteString("\n")
	e := m.cache[s.ID]
	switch {
	case e == nil:
		b.WriteString("\n" + t.Faint("loading transcript…"))
	case e.err != nil:
		b.WriteString("\n" + t.Error("could not read transcript: "+e.err.Error()))
	default:
		if e.rendered == nil {
			e.rendered = map[int]string{}
		}
		body, ok := e.rendered[width]
		if !ok {
			var buf bytes.Buffer
			msgs := e.msgs
			if len(msgs) > maxPreviewMessages {
				msgs = msgs[:maxPreviewMessages]
			}
			render.Transcript(&buf, t, s.Tool, msgs, render.Options{}, width)
			if len(e.msgs) > maxPreviewMessages {
				fmt.Fprintf(&buf, "\n%s\n", t.Faint(fmt.Sprintf("… %d more messages — ais show %s", len(e.msgs)-maxPreviewMessages, render.ShortID(s.ID))))
			}
			body = buf.String()
			e.rendered[width] = body
		}
		b.WriteString(body)
	}
	m.preview.SetContent(padLeft(b.String(), " "))
	if m.shownID != s.ID {
		m.preview.GotoTop()
	}
	m.shownID, m.shownW = s.ID, width
}

func padLeft(s, pad string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = pad + l
		}
	}
	return strings.Join(lines, "\n")
}

func (m *Model) flash(text string, isErr bool) tea.Cmd {
	m.status, m.statusError = text, isErr
	m.statusSeq++
	seq := m.statusSeq
	return tea.Tick(4*time.Second, func(time.Time) tea.Msg { return clearStatusMg{seq} })
}

// launch hands the terminal to a resume/fork/new command.
func (m *Model) launch(build func(provider.Provider, model.Session) (*exec.Cmd, error)) tea.Cmd {
	s, ok := m.current()
	if !ok {
		return nil
	}
	p := provider.For(m.d.Providers, s.Tool)
	if p == nil {
		return m.flash("no provider for "+string(s.Tool), true)
	}
	cmd, err := build(p, s)
	if err == nil {
		err = launch.Check(cmd)
	}
	if err != nil {
		return m.flash(err.Error(), true)
	}
	return m.d.Exec(cmd, func(err error) tea.Msg { return execDoneMsg{err} })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.filter.Width = max(m.width-4, 10)
		m.clampScroll()
		m.refreshPreview()
		return m, nil

	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case scanMsg:
		m.loading, m.scanned = false, true
		if msg.err != nil {
			m.err = msg.err
			return m, m.flash("scan failed: "+msg.err.Error(), true)
		}
		m.err = nil
		m.sessions = msg.res.Sessions
		m.warnings = len(msg.res.Warnings)
		m.cache = map[string]*previewEntry{}
		m.shownID = ""
		m.applyFilter()
		return m, m.selectionChanged()

	case selectMsg:
		s, ok := m.current()
		if !ok || s.ID != msg.id {
			return m, nil
		}
		if _, cached := m.cache[s.ID]; cached {
			return m, nil
		}
		m.cache[s.ID] = nil // loading
		return m, m.loadTranscript(s)

	case transcriptMsg:
		m.cache[msg.id] = &previewEntry{msgs: msg.msgs, err: msg.err}
		if msg.id == m.selected {
			m.shownID = "" // force a re-render with the transcript
			m.refreshPreview()
		}
		return m, nil

	case execDoneMsg:
		m.loading = true
		cmds := []tea.Cmd{m.scan(), m.spinner.Tick}
		if msg.err != nil {
			cmds = append(cmds, m.flash("session exited: "+msg.err.Error(), true))
		} else {
			cmds = append(cmds, m.flash("back from the session", false))
		}
		return m, tea.Batch(cmds...)

	case clearStatusMg:
		if msg.seq == m.statusSeq {
			m.status = ""
		}
		return m, nil

	case tea.KeyMsg:
		if m.filtering {
			return m.updateFilter(msg)
		}
		return m.updateKeys(msg)
	}
	return m, nil
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.filtering = false
		m.filter.Blur()
		m.filter.SetValue("")
		m.query = parseQuery("")
		m.applyFilter()
		return m, m.selectionChanged()
	case "enter":
		m.filtering = false
		m.filter.Blur()
		return m, nil
	case "up", "ctrl+p":
		return m, m.move(-1)
	case "down", "ctrl+n":
		return m, m.move(1)
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	if q := parseQuery(m.filter.Value()); fmt.Sprint(q) != fmt.Sprint(m.query) {
		m.query = q
		m.applyFilter()
		return m, tea.Batch(cmd, m.selectionChanged())
	}
	return m, cmd
}

func (m Model) updateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Fast typing or a paste can deliver "/" and the query as one event:
	// open the filter and hand it the rest.
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && msg.Runes[0] == '/' && !msg.Paste {
		m.filtering, m.help, m.focus = true, false, focusList
		focusCmd := m.filter.Focus()
		next, cmd := m.updateFilter(tea.KeyMsg{Type: tea.KeyRunes, Runes: msg.Runes[1:]})
		return next, tea.Batch(focusCmd, cmd)
	}
	key := msg.String()
	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "?":
		m.help = !m.help
		return m, nil
	case "tab":
		if m.focus == focusList {
			m.focus = focusPreview
		} else {
			m.focus = focusList
		}
		return m, nil
	case "/":
		m.filtering, m.help = true, false
		m.focus = focusList
		return m, m.filter.Focus()
	case "esc":
		switch {
		case m.help:
			m.help = false
		case m.focus == focusPreview:
			m.focus = focusList
		case !m.query.empty():
			m.filter.SetValue("")
			m.query = parseQuery("")
			m.applyFilter()
			return m, m.selectionChanged()
		}
		return m, nil
	case "r", "ctrl+r":
		m.loading = true
		return m, tea.Batch(m.scan(), m.spinner.Tick)
	case "enter":
		return m, m.launch(func(p provider.Provider, s model.Session) (*exec.Cmd, error) { return p.ResumeCmd(s, false) })
	case "f":
		return m, m.launch(func(p provider.Provider, s model.Session) (*exec.Cmd, error) { return p.ResumeCmd(s, true) })
	case "n":
		return m, m.launch(func(p provider.Provider, s model.Session) (*exec.Cmd, error) { return p.NewCmd(s.CWD), nil })
	case "y":
		s, ok := m.current()
		if !ok {
			return m, nil
		}
		p := provider.For(m.d.Providers, s.Tool)
		cmd, err := p.ResumeCmd(s, false)
		if err != nil {
			return m, m.flash(err.Error(), true)
		}
		line := launch.ShellLine(cmd)
		if m.d.Copy == nil {
			return m, m.flash(line, false)
		}
		if err := m.d.Copy(line); err != nil {
			return m, m.flash("copy failed: "+err.Error(), true)
		}
		return m, m.flash("copied: "+line, false)
	}

	if m.focus == focusPreview {
		switch key {
		case "up", "k":
			m.preview.ScrollUp(1)
		case "down", "j":
			m.preview.ScrollDown(1)
		case "pgup", "ctrl+u", "b":
			m.preview.HalfPageUp()
		case "pgdown", "ctrl+d", " ", "space":
			m.preview.HalfPageDown()
		case "home", "g":
			m.preview.GotoTop()
		case "end", "G":
			m.preview.GotoBottom()
		}
		return m, nil
	}

	listH, _ := m.layout()
	switch key {
	case "up", "k", "ctrl+p":
		return m, m.move(-1)
	case "down", "j", "ctrl+n":
		return m, m.move(1)
	case "pgup", "ctrl+u":
		return m, m.move(-listH)
	case "pgdown", "ctrl+d":
		return m, m.move(listH)
	case "home", "g":
		return m, m.move(-len(m.view))
	case "end", "G":
		return m, m.move(len(m.view))
	case "J", "shift+down":
		m.preview.ScrollDown(3)
	case "K", "shift+up":
		m.preview.ScrollUp(3)
	}
	return m, nil
}
