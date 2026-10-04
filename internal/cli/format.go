package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

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
