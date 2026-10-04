// Package render turns sessions and transcripts into text, shared by the
// CLI commands and the TUI.
package render

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Age renders how long ago t was, compactly: "now", "5m", "3h", "4d", "2w",
// or a date for anything older than about two months.
func Age(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	}
	return t.Local().Format("2006-01-02")
}

// Ago renders "just now" or "<age> ago".
func Ago(now, t time.Time) string {
	if a := Age(now, t); a != "now" {
		return a + " ago"
	}
	return "just now"
}

// Tokens renders a token count as 950, 12.3k, 4.1M, 1.2B.
func Tokens(n int64) string {
	f := float64(n)
	switch {
	case n < 1_000:
		return strconv.FormatInt(n, 10)
	case n < 1_000_000:
		return trimZero(f/1e3) + "k"
	case n < 1_000_000_000:
		return trimZero(f/1e6) + "M"
	}
	return trimZero(f/1e9) + "B"
}

func trimZero(f float64) string {
	return strings.TrimSuffix(strconv.FormatFloat(f, 'f', 1, 64), ".0")
}

// Plural renders a count with the right noun form: "1 prompt", "2 prompts".
func Plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// ShortID is the id prefix shown in lists; any unique prefix resolves.
func ShortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// VersionLabel renders a tool version as "v1.2.3", or "" when unknown.
func VersionLabel(v string) string {
	if v == "" {
		return ""
	}
	return "v" + v
}
