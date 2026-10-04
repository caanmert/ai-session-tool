// Package ui styles terminal output. A Theme is bound to one output stream:
// on a color terminal it renders colors, borders and markdown; otherwise
// (pipes, files, NO_COLOR, --color=never) every helper returns plain text,
// so scripts and golden tests see stable output.
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/muesli/termenv"
	"golang.org/x/term"

	"github.com/caanmert/ai-session-tool/internal/model"
)

// Mode is the --color setting.
type Mode int

const (
	Auto Mode = iota
	Always
	Never
)

// ParseMode parses "auto", "always" or "never".
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(s) {
	case "", "auto":
		return Auto, nil
	case "always", "yes", "force":
		return Always, nil
	case "never", "no", "none":
		return Never, nil
	}
	return Auto, fmt.Errorf("invalid --color %q: use auto, always or never", s)
}

// Theme renders styled text for one output stream.
type Theme struct {
	// Color is true when output may contain escape sequences.
	Color bool
	// Width is the terminal width, or 0 when unknown.
	Width int

	r       *lipgloss.Renderer
	profile termenv.Profile
}

// New returns a theme for w.
func New(w io.Writer, mode Mode) *Theme {
	tty, width := terminal(w)
	t := &Theme{Width: width, r: lipgloss.NewRenderer(w)}
	switch mode {
	case Never:
		t.Color = false
	case Always:
		t.Color = true
	default:
		t.Color = tty && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	}
	switch {
	case !t.Color:
		t.profile = termenv.Ascii
	case tty:
		t.profile = t.r.ColorProfile()
	default:
		t.profile = termenv.TrueColor // forced color into a pipe or buffer
	}
	t.r.SetColorProfile(t.profile)
	switch strings.ToLower(os.Getenv("AIS_THEME")) {
	case "dark":
		t.r.SetHasDarkBackground(true)
	case "light":
		t.r.SetHasDarkBackground(false)
	}
	return t
}

// Prime settles anything that needs to ask the terminal (its background
// color, for light/dark styles) now. Call it before a full-screen program
// starts reading input, which would otherwise swallow the terminal's reply.
// AIS_THEME=dark|light skips the question.
func (t *Theme) Prime() {
	if t.Color {
		t.r.SetHasDarkBackground(t.r.HasDarkBackground())
	}
}

func terminal(w io.Writer) (tty bool, width int) {
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return false, 0
	}
	cols, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return true, 0
	}
	return true, cols
}

// Palette. Each color adapts to light and dark terminal backgrounds.
var (
	colorClaude = lipgloss.AdaptiveColor{Light: "#C15F3C", Dark: "#D97757"}
	colorCodex  = lipgloss.AdaptiveColor{Light: "#0E8A6D", Dark: "#3CC9A4"}
	colorUser   = lipgloss.AdaptiveColor{Light: "#2563EB", Dark: "#7AA2F7"}
	colorID     = lipgloss.AdaptiveColor{Light: "#0369A1", Dark: "#7DCFFF"}
	colorBranch = lipgloss.AdaptiveColor{Light: "#7C3AED", Dark: "#BB9AF7"}
	colorFaint  = lipgloss.AdaptiveColor{Light: "#8A8F98", Dark: "#6E7681"}
	colorBorder = lipgloss.AdaptiveColor{Light: "#D0D4DA", Dark: "#3B4048"}
	colorOK     = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#73DACA"}
	colorWarn   = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#E0AF68"}
	colorError  = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F7768E"}
	colorLive   = lipgloss.AdaptiveColor{Light: "#16A34A", Dark: "#9ECE6A"}
)

// Style returns a new lipgloss style bound to this theme's renderer.
func (t *Theme) Style() lipgloss.Style { return t.r.NewStyle() }

func (t *Theme) fg(c lipgloss.TerminalColor, s string) string {
	return t.render(t.r.NewStyle().Foreground(c), s)
}

// render styles s line by line, so multi-line text isn't padded into a
// rectangle the way lipgloss renders blocks.
func (t *Theme) render(st lipgloss.Style, s string) string {
	if !t.Color || s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = st.Render(l)
		}
	}
	return strings.Join(lines, "\n")
}

// ToolColor is the brand color of a tool.
func ToolColor(tool model.Tool) lipgloss.TerminalColor {
	if tool == model.ToolCodex {
		return colorCodex
	}
	return colorClaude
}

// Tool renders a tool name in its color.
func (t *Theme) Tool(tool model.Tool, s string) string { return t.fg(ToolColor(tool), s) }

func (t *Theme) User(s string) string   { return t.fg(colorUser, s) }
func (t *Theme) ID(s string) string     { return t.fg(colorID, s) }
func (t *Theme) Branch(s string) string { return t.fg(colorBranch, s) }
func (t *Theme) Faint(s string) string  { return t.fg(colorFaint, s) }
func (t *Theme) OK(s string) string     { return t.fg(colorOK, s) }
func (t *Theme) Warn(s string) string   { return t.fg(colorWarn, s) }
func (t *Theme) Error(s string) string  { return t.fg(colorError, s) }
func (t *Theme) Live(s string) string   { return t.fg(colorLive, s) }

// Bold renders s in bold.
func (t *Theme) Bold(s string) string { return t.render(t.r.NewStyle().Bold(true), s) }

// Italic renders s faint and italic.
func (t *Theme) Italic(s string) string {
	return t.render(t.r.NewStyle().Italic(true).Foreground(colorFaint), s)
}

// Age colors a relative time by how recent it is: within the hour stands
// out, today is normal, older fades.
func (t *Theme) Age(now, ts time.Time, s string) string {
	switch d := now.Sub(ts); {
	case d < time.Hour:
		return t.Live(s)
	case d < 24*time.Hour:
		return s
	}
	return t.Faint(s)
}

// Card draws a rounded box around body, at most width columns wide.
func (t *Theme) Card(body string, width int) string {
	return t.r.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(width - 2). // the border adds one column on each side
		Render(body)
}

// Bar prefixes every line of body with a colored vertical bar, the way
// chat UIs set off a speaker's message.
func (t *Theme) Bar(c lipgloss.TerminalColor, body string, width int) string {
	bar := t.fg(c, "┃") + " "
	lines := strings.Split(wordwrap.String(body, max(width-2, 10)), "\n")
	for i, l := range lines {
		lines[i] = bar + strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

// UserColor is the color of "you" in transcripts.
func UserColor() lipgloss.TerminalColor { return colorUser }

// Width of s in terminal cells, ignoring escape sequences.
func Width(s string) int { return lipgloss.Width(s) }

// Home abbreviates the user's home directory to ~ for display.
func Home(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || home == "/" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + path[len(home):]
	}
	return path
}
