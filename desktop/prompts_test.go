package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSavedPrompts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "desktop-prompts.json")
	a := &App{promptsPath: path}
	got, err := a.SavedPrompts()
	if err != nil || len(got) != len(starterPrompts) {
		t.Fatalf("starters: %v %v", got, err)
	}
	// Removing every starter is kept: they are offered once.
	if err := a.SetSavedPrompts([]SavedPrompt{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.SavedPrompts(); len(got) != 0 {
		t.Fatalf("after removing all: %v", got)
	}
	mine := []SavedPrompt{{ID: "a", Name: "  Lint  ", Text: " Run the linter on {changed files}. "}}
	if err := a.SetSavedPrompts(mine); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.SavedPrompts(); len(got) != 1 || got[0].Name != "Lint" || got[0].Text != "Run the linter on {changed files}." {
		t.Fatalf("saved: %+v", got)
	}
	for _, bad := range [][]SavedPrompt{
		{{ID: "a", Name: "", Text: "x"}},
		{{ID: "a", Name: "x", Text: "  "}},
		{{ID: "a", Name: "x", Text: "y"}, {ID: "a", Name: "z", Text: "w"}},
		{{ID: "", Name: "x", Text: "y"}},
		{{ID: "a", Name: strings.Repeat("n", promptNameLimit+1), Text: "y"}},
		{{ID: "a", Name: "x", Text: strings.Repeat("t", promptTextLimit+1)}},
	} {
		if err := a.SetSavedPrompts(bad); err == nil {
			t.Fatalf("%+v must be refused", bad)
		}
	}
	if got, _ := a.SavedPrompts(); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("a refused save must keep the list: %+v", got)
	}
}
