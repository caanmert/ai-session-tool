package main

import (
	"context"
	"strings"
	"testing"
)

func TestSetAttention(t *testing.T) {
	type call struct {
		count int
		alert bool
	}
	var calls []call
	app := &App{dockAttention: func(count int, alert bool) { calls = append(calls, call{count, alert}) }}
	if err := app.SetAttention(1, true); err == nil {
		t.Fatal("expected an error before startup")
	}
	app.ctx = context.Background()
	if err := app.SetAttention(2, true); err != nil {
		t.Fatal(err)
	}
	if err := app.SetAttention(-3, false); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0] != (call{2, true}) || calls[1] != (call{0, false}) {
		t.Fatalf("calls: %+v", calls)
	}
}

func TestQuitConfirmation(t *testing.T) {
	one, message := quitConfirmation(1)
	many, _ := quitConfirmation(3)
	if one != "Quit with 1 agent running?" || many != "Quit with 3 agents running?" {
		t.Fatalf("titles: %q, %q", one, many)
	}
	for _, kept := range []string{"already edited stay", "conversations", "drafts"} {
		if !strings.Contains(message, kept) {
			t.Errorf("message should say %q is kept: %s", kept, message)
		}
	}
}
