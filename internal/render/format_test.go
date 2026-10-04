package render

import (
	"testing"
	"time"
)

func TestAge(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for d, want := range map[time.Duration]string{
		10 * time.Second: "now", 5 * time.Minute: "5m", 3 * time.Hour: "3h",
		4 * 24 * time.Hour: "4d", 20 * 24 * time.Hour: "2w",
	} {
		if got := Age(now, now.Add(-d)); got != want {
			t.Errorf("Age(%v) = %s, want %s", d, got, want)
		}
	}
	if got := Ago(now, now); got != "just now" {
		t.Errorf("Ago = %q", got)
	}
}

func TestTokensAndPlural(t *testing.T) {
	for n, want := range map[int64]string{0: "0", 999: "999", 1000: "1k", 12345: "12.3k", 4_100_000: "4.1M", 1_200_000_000: "1.2B"} {
		if got := Tokens(n); got != want {
			t.Errorf("Tokens(%d) = %s, want %s", n, got, want)
		}
	}
	if Plural(1, "reply", "replies") != "1 reply" || Plural(0, "reply", "replies") != "0 replies" {
		t.Error("Plural")
	}
}
