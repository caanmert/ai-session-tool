package ui

import (
	"io"
	"strings"
)

// Column describes one table column.
type Column struct {
	Header string
	Right  bool // right-align, for numbers
}

// Cell is one table cell: plain text plus an optional style applied after
// padding, so escape sequences never disturb alignment.
type Cell struct {
	Text  string
	Style func(string) string
}

// Table writes rows as aligned columns separated by two spaces. The last
// column is not padded, so long titles don't leave trailing spaces.
func (t *Theme) Table(w io.Writer, cols []Column, rows [][]Cell) error {
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = Width(c.Header)
	}
	for _, row := range rows {
		for i, c := range row {
			widths[i] = max(widths[i], Width(c.Text))
		}
	}

	var b strings.Builder
	line := func(cells []Cell) {
		for i, c := range cells {
			last := i == len(cells)-1
			pad := strings.Repeat(" ", widths[i]-Width(c.Text))
			text := c.Text
			if c.Style != nil {
				text = c.Style(text)
			}
			switch {
			case cols[i].Right:
				b.WriteString(pad + text)
			case last:
				b.WriteString(text)
			default:
				b.WriteString(text + pad)
			}
			if !last {
				b.WriteString("  ")
			}
		}
		b.WriteString("\n")
	}

	header := make([]Cell, len(cols))
	for i, c := range cols {
		header[i] = Cell{Text: c.Header, Style: func(s string) string { return t.Faint(t.Bold(s)) }}
	}
	line(header)
	for _, row := range rows {
		line(row)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
