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

func TestNotify(t *testing.T) {
	type note struct{ title, body string }
	var notes []note
	app := &App{notify: func(title, body string) { notes = append(notes, note{title, body}) }}
	if err := app.Notify("Rotate keys", "Claude Code needs approval"); err == nil {
		t.Fatal("expected an error before startup")
	}
	app.ctx = context.Background()
	if err := app.Notify("  Rotate\nkeys\x00 ", strings.Repeat("long ", 80)); err != nil {
		t.Fatal(err)
	}
	if err := app.Notify("   ", "body"); err == nil {
		t.Fatal("a notification without a title should be refused")
	}
	if len(notes) != 1 || notes[0].title != "Rotate keys" || len([]rune(notes[0].body)) != 160 || !strings.HasSuffix(notes[0].body, "…") {
		t.Fatalf("notes: %+v", notes)
	}
}

// The test binary is not an app bundle. Posting must do nothing rather than
// reach for a notification center that does not exist for it.
func TestPostNotificationOutsideABundle(t *testing.T) {
	postNotification("ais", "This is never shown.")
}
