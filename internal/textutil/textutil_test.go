package textutil

import "testing"

func TestTruncate(t *testing.T) {
	for _, tt := range []struct {
		in   string
		n    int
		want string
	}{
		{"héllo world", 5, "héll…"},
		{"short", 10, "short"},
		{"abc", 1, "…"},
		{"abc", 0, ""},
	} {
		if got := Truncate(tt.in, tt.n); got != tt.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}

func TestFirstLineAndCollapse(t *testing.T) {
	if got := FirstLine("\n  \n  title here \nmore"); got != "title here" {
		t.Errorf("FirstLine = %q", got)
	}
	if got := Collapse(" a \n\t b  c "); got != "a b c" {
		t.Errorf("Collapse = %q", got)
	}
}

func TestParseTime(t *testing.T) {
	if _, ok := ParseTime("nope"); ok {
		t.Error("ParseTime accepted garbage")
	}
	ts, ok := ParseTime("2026-10-02T14:30:00.123+02:00")
	if !ok || ts.Hour() != 12 || ts.Location().String() != "UTC" {
		t.Errorf("ParseTime = %v, %v", ts, ok)
	}
}

func TestTruncateMiddle(t *testing.T) {
	if got := TruncateMiddle("/a/very/long/path/file.jsonl", 15); got != "/a/very…e.jsonl" {
		t.Errorf("TruncateMiddle = %q", got)
	}
	if got := TruncateMiddle("short", 10); got != "short" {
		t.Errorf("TruncateMiddle = %q", got)
	}
}
