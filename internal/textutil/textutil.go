// Package textutil has small string and time helpers shared by the
// providers and the CLI.
package textutil

import (
	"strings"
	"time"
)

// ParseTime parses an RFC 3339 timestamp and returns it in UTC.
func ParseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// Collapse replaces every run of whitespace with a single space.
func Collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// FirstLine returns the first non-blank line of s, trimmed.
func FirstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

// Truncate shortens s to at most n runes, ending with "…" when cut.
func Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

// TruncateMiddle shortens s to at most n runes by replacing its middle
// with "…", which keeps both ends of a path readable.
func TruncateMiddle(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n < 3 {
		return Truncate(s, n)
	}
	head := (n - 1) / 2
	tail := n - 1 - head
	return string(r[:head]) + "…" + string(r[len(r)-tail:])
}

// FirstNonEmpty returns the first non-empty value.
func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
