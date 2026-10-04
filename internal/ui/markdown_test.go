package ui

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

var escapes = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func TestMarkdownRendersOnlyWithColor(t *testing.T) {
	src := "## Plan\n\n- **first** step\n- second step\n\n```go\nfmt.Println(\"hi\")\n```"

	plain := New(&bytes.Buffer{}, Never).Markdown(60).Render(src)
	if plain != src {
		t.Errorf("plain output must be the raw text, got %q", plain)
	}

	out := New(&bytes.Buffer{}, Always).Markdown(60).Render(src)
	if !escapes.MatchString(out) {
		t.Fatal("expected escape sequences in colored markdown")
	}
	text := escapes.ReplaceAllString(out, "")
	for _, want := range []string{"Plan", "• first step", `fmt.Println("hi")`} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered markdown lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "**first**") || strings.Contains(text, "```") {
		t.Errorf("markdown syntax was not rendered:\n%s", text)
	}
	if strings.HasPrefix(text, "\n") || strings.HasSuffix(text, "\n") {
		t.Errorf("rendered markdown should be trimmed: %q", text)
	}
}
