package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseMode(t *testing.T) {
	for in, want := range map[string]Mode{"": Auto, "auto": Auto, "ALWAYS": Always, "never": Never} {
		if got, err := ParseMode(in); err != nil || got != want {
			t.Errorf("ParseMode(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseMode("sometimes"); err == nil {
		t.Error("ParseMode accepted an invalid value")
	}
}

func TestNonTerminalIsPlain(t *testing.T) {
	th := New(&bytes.Buffer{}, Auto)
	if th.Color {
		t.Fatal("a buffer is not a terminal: auto mode must not color")
	}
	if got := th.Bold(th.ID("x")) + th.Tool("codex", "y") + th.Faint("z"); got != "xyz" {
		t.Errorf("plain theme styled its output: %q", got)
	}
}

func TestTableAlignsStyledCells(t *testing.T) {
	cols := []Column{{Header: "ID"}, {Header: "N", Right: true}, {Header: "TITLE"}}
	rows := func(th *Theme) [][]Cell {
		return [][]Cell{
			{{Text: "abc", Style: th.ID}, {Text: "7", Style: th.Faint}, {Text: "first"}},
			{{Text: "a", Style: th.ID}, {Text: "1234", Style: th.Faint}, {Text: "second"}},
		}
	}
	var plain, colored bytes.Buffer
	pt, ct := New(&plain, Never), New(&colored, Always)
	if err := pt.Table(&plain, cols, rows(pt)); err != nil {
		t.Fatal(err)
	}
	if err := ct.Table(&colored, cols, rows(ct)); err != nil {
		t.Fatal(err)
	}

	want := "ID      N  TITLE\nabc     7  first\na    1234  second\n"
	if plain.String() != want {
		t.Errorf("plain table:\n%s\nwant:\n%s", plain.String(), want)
	}
	if !escapes.MatchString(colored.String()) {
		t.Fatal("colored table has no escape sequences")
	}
	if got := escapes.ReplaceAllString(colored.String(), ""); got != want {
		t.Errorf("colored table misaligned once escapes are removed:\n%s", got)
	}
}

func TestBarWrapsWithoutTrailingSpaces(t *testing.T) {
	th := New(&bytes.Buffer{}, Always)
	out := escapes.ReplaceAllString(th.Bar(UserColor(), "one two three four five six", 14), "")
	for _, l := range strings.Split(out, "\n") {
		if !strings.HasPrefix(l, "┃ ") || strings.HasSuffix(l, " ") || Width(l) > 14 {
			t.Errorf("bad bar line %q", l)
		}
	}
}

func TestHome(t *testing.T) {
	t.Setenv("HOME", "/Users/dev")
	for in, want := range map[string]string{
		"/Users/dev":          "~",
		"/Users/dev/code/api": "~/code/api",
		"/Users/devops/x":     "/Users/devops/x",
		"/tmp":                "/tmp",
	} {
		if got := Home(in); got != want {
			t.Errorf("Home(%q) = %q, want %q", in, got, want)
		}
	}
}
