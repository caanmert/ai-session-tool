package main

import (
	"context"
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
