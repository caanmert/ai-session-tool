package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

var errChoiceCanceled = errors.New("selection canceled")

type choiceModel struct {
	title   string
	options []string
	cursor  int
	width   int
	height  int
	chosen  bool
}

func (m choiceModel) Init() tea.Cmd { return nil }

func (m choiceModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.cursor = max(0, m.cursor-1)
		case "down", "j":
			m.cursor = min(len(m.options)-1, m.cursor+1)
		case "enter":
			m.chosen = true
			return m, tea.Quit
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m choiceModel) View() string {
	if m.chosen {
		return ""
	}
	rows := min(10, max(1, m.height-4))
	start := max(0, m.cursor-rows+1)
	var lines []string
	lines = append(lines, pickerText(m.title))
	for i := start; i < min(start+rows, len(m.options)); i++ {
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}
		lines = append(lines, prefix+pickerText(m.options[i]))
	}
	lines = append(lines, fmt.Sprintf("↑/↓ select · Enter open · Esc cancel  (%d/%d)", m.cursor+1, len(m.options)))
	for i, line := range lines {
		// Truncate after escaping transcript-provided controls.
		lines[i] = ansi.Truncate(line, max(m.width, 1), "…")
	}
	return strings.Join(lines, "\n") + "\n"
}

// Keep transcript titles and directory names from emitting terminal controls.
func pickerText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}

func (a *App) choose(ctx context.Context, title string, options []string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if a.Interactive == nil || !a.Interactive() {
		return 0, errors.New("selection requires a terminal; specify --tool for new, or a session ID / --latest for continue")
	}
	var selected int
	var err error
	if a.Choose != nil {
		selected, err = a.Choose(ctx, title, options)
	} else {
		var result tea.Model
		result, err = tea.NewProgram(choiceModel{title: title, options: options, width: 100, height: 16}, tea.WithInput(a.In), tea.WithOutput(a.Err), tea.WithContext(ctx)).Run()
		if err == nil {
			m := result.(choiceModel)
			if !m.chosen {
				return 0, errChoiceCanceled
			}
			selected = m.cursor
		}
	}
	if err != nil {
		return 0, err
	}
	if selected < 0 || selected >= len(options) {
		return 0, errors.New("invalid selection")
	}
	return selected, nil
}
