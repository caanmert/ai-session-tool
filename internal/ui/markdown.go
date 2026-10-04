package ui

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
)

// Markdown renders assistant replies. On plain output it returns the text
// unchanged.
type Markdown struct {
	r *glamour.TermRenderer
}

// Markdown returns a renderer that wraps at width columns.
func (t *Theme) Markdown(width int) *Markdown {
	if !t.Color {
		return &Markdown{}
	}
	cfg := styles.DarkStyleConfig
	if !t.r.HasDarkBackground() {
		cfg = styles.LightStyleConfig
	}
	// Transcripts supply their own spacing and indentation.
	noMargin := uint(0)
	cfg.Document.Margin = &noMargin
	cfg.Document.BlockPrefix = ""
	cfg.Document.BlockSuffix = ""
	cfg.Document.Color = nil
	cfg.Paragraph = ansi.StyleBlock{}

	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(cfg),
		glamour.WithColorProfile(t.profile),
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
	)
	if err != nil {
		return &Markdown{}
	}
	return &Markdown{r: r}
}

// Render returns s rendered as markdown, without surrounding blank lines.
func (m *Markdown) Render(s string) string {
	if m.r == nil {
		return s
	}
	out, err := m.r.Render(s)
	if err != nil {
		return s
	}
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}
