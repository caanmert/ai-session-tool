package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

// age renders how long ago t was, compactly: "now", "5m", "3h", "4d", "2w",
// or a date for anything older than about two months.
func age(now, t time.Time) string {
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

// tokens renders a token count as 950, 12.3k, 4.1M, 1.2B.
func tokens(n int64) string {
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

// truncate shortens s to at most n runes, ending with "…" when cut.
func truncate(s string, n int) string {
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

// width returns the terminal width of w, or 0 when w is not a terminal.
func width(w io.Writer) int {
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return 0
	}
	cols, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0
	}
	return cols
}

// parseSince accepts a duration like 30m, 12h, 7d, 2w or a date 2006-01-02
// and returns the earliest time it allows.
func parseSince(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t, nil
	}
	if len(s) >= 2 {
		n, err := strconv.Atoi(s[:len(s)-1])
		if err == nil && n >= 0 {
			unit := map[byte]time.Duration{'m': time.Minute, 'h': time.Hour, 'd': 24 * time.Hour, 'w': 7 * 24 * time.Hour}[s[len(s)-1]]
			if unit != 0 {
				return now.Add(-time.Duration(n) * unit), nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("invalid --since %q: use e.g. 30m, 12h, 7d, 2w or 2006-01-02", s)
}
