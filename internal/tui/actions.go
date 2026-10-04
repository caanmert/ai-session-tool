package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/caanmert/ai-session-tool/internal/meta"
	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/render"
)

// promptKind is the question the footer is asking.
type promptKind int

const (
	promptNone promptKind = iota
	promptRename
	promptTag
	promptTrash
)

// actionDoneMsg reports a finished action on session key.
type actionDoneMsg struct {
	key    string
	kind   string // rename, tag, pin, archive, trash
	err    error
	title  string   // rename
	add    []string // tag
	remove []string // tag
	on     bool     // pin, archive
}

func (m *Model) startPrompt(kind promptKind) tea.Cmd {
	s, ok := m.current()
	if !ok {
		return nil
	}
	if m.d.Actions == nil {
		return m.flash("renaming, tagging and the trash need ais's data directory", true)
	}
	in := textinput.New()
	in.Width = max(m.width-20, 10)
	switch kind {
	case promptRename:
		in.Prompt = "rename: "
		in.Placeholder = "empty restores the tool's title"
		in.SetValue(s.Title)
		in.CursorEnd()
	case promptTag:
		in.Prompt = "tags: "
		in.Placeholder = "bug +auth -old   (add, or -tag to remove)"
	case promptTrash:
		if s.Live != nil {
			return m.flash("this session is running; quit it before trashing", true)
		}
	}
	m.prompt, m.input = kind, in
	m.help = false
	return m.input.Focus()
}

func (m Model) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.prompt == promptTrash {
		switch msg.String() {
		case "y", "Y":
			m.prompt = promptNone
			return m, m.act("trash", func(a Actions, ctx context.Context, s model.Session) actionDoneMsg {
				return actionDoneMsg{err: a.Trash(ctx, s)}
			})
		case "ctrl+c":
			return m, tea.Quit
		default:
			m.prompt = promptNone
			return m, nil
		}
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.prompt = promptNone
		return m, nil
	case "enter":
		value := m.input.Value()
		kind := m.prompt
		m.prompt = promptNone
		if kind == promptRename {
			title := strings.TrimSpace(value)
			return m, m.act("rename", func(a Actions, ctx context.Context, s model.Session) actionDoneMsg {
				return actionDoneMsg{err: a.Rename(ctx, s, title), title: title}
			})
		}
		add, remove, err := parseTagEdit(value)
		if err != nil {
			return m, m.flash(err.Error(), true)
		}
		if len(add)+len(remove) == 0 {
			return m, nil
		}
		return m, m.act("tag", func(a Actions, ctx context.Context, s model.Session) actionDoneMsg {
			return actionDoneMsg{err: a.Tag(ctx, s, add, remove), add: add, remove: remove}
		})
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// parseTagEdit reads "bug +auth -old": plain and + words add, - removes.
func parseTagEdit(s string) (add, remove []string, err error) {
	for _, f := range strings.Fields(s) {
		target := &add
		if rest, ok := strings.CutPrefix(f, "-"); ok {
			f, target = rest, &remove
		} else {
			f = strings.TrimPrefix(f, "+")
		}
		tag, err := meta.NormalizeTag(f)
		if err != nil {
			return nil, nil, err
		}
		*target = append(*target, tag)
	}
	return add, remove, nil
}

// act runs an action on the selected session in the background.
func (m *Model) act(kind string, run func(Actions, context.Context, model.Session) actionDoneMsg) tea.Cmd {
	s, ok := m.current()
	if !ok {
		return nil
	}
	if m.d.Actions == nil {
		return m.flash("renaming, tagging and the trash need ais's data directory", true)
	}
	actions := m.d.Actions
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		msg := run(actions, ctx, s)
		msg.key, msg.kind = s.Key(), kind
		return msg
	}
}

// toggle flips pin or archive on the selected session.
func (m *Model) toggle(kind string) tea.Cmd {
	s, ok := m.current()
	if !ok {
		return nil
	}
	if kind == "pin" {
		on := !s.Pinned
		return m.act(kind, func(a Actions, ctx context.Context, s model.Session) actionDoneMsg {
			return actionDoneMsg{err: a.SetPinned(ctx, s, on), on: on}
		})
	}
	on := !s.Archived
	return m.act(kind, func(a Actions, ctx context.Context, s model.Session) actionDoneMsg {
		return actionDoneMsg{err: a.SetArchived(ctx, s, on), on: on}
	})
}

// applyAction mirrors a successful action in the loaded sessions, so the
// list updates without a rescan.
func (m *Model) applyAction(msg actionDoneMsg) tea.Cmd {
	if msg.err != nil {
		return m.flash(msg.kind+" failed: "+msg.err.Error(), true)
	}
	i := -1
	for j := range m.sessions {
		if m.sessions[j].Key() == msg.key {
			i = j
			break
		}
	}
	if i < 0 {
		return nil
	}
	s := &m.sessions[i]
	short := render.ShortID(s.ID)
	var note string
	switch msg.kind {
	case "rename":
		switch {
		case msg.title == "" && s.OriginalTitle != "":
			s.Title, s.OriginalTitle = s.OriginalTitle, ""
		case msg.title != "" && s.OriginalTitle == "":
			s.OriginalTitle, s.Title = s.Title, msg.title
		case msg.title != "":
			s.Title = msg.title
		}
		note = "renamed to " + s.Title
	case "tag":
		tags := map[string]bool{}
		for _, t := range s.Tags {
			tags[t] = true
		}
		for _, t := range msg.add {
			tags[t] = true
		}
		for _, t := range msg.remove {
			delete(tags, t)
		}
		s.Tags = s.Tags[:0]
		for t := range tags {
			s.Tags = append(s.Tags, t)
		}
		sortStrings(s.Tags)
		note = "tags updated"
	case "pin":
		s.Pinned = msg.on
		note = map[bool]string{true: "pinned " + short, false: "unpinned " + short}[msg.on]
	case "archive":
		s.Archived = msg.on
		note = map[bool]string{true: "archived " + short + " (is:archived shows it)", false: "unarchived " + short}[msg.on]
	case "trash":
		m.sessions = append(m.sessions[:i], m.sessions[i+1:]...)
		note = "moved to the trash (ais restore " + short + ")"
	}
	meta.Sort(m.sessions)
	m.shownID = ""
	m.applyFilter()
	return tea.Batch(m.flash(note, false), m.selectionChanged())
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
