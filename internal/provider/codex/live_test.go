package codex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLiveOpenRollouts(t *testing.T) {
	root := t.TempDir()
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	p := New(root)
	rollout := "rollout-2026-10-04T08-00-00-" + idLegacy + ".jsonl"
	path := filepath.Join(realRoot, "sessions", "folder with spaces\nand newline", rollout)
	p.OpenFiles = func(context.Context) ([]byte, error) {
		return []byte(fmt.Sprintf("p123\x00\nf7\x00n%s\x00\nf8\x00n%s\x00\np456\x00n%s\x00\nn%s\x00\np0\x00n%s\x00\n",
			path, path, filepath.Join(realRoot+"-other", "sessions", rollout), filepath.Join(realRoot, "notes.txt"), path)), nil
	}
	states, err := p.Live(context.Background())
	if err != nil || len(states) != 1 || states[0].PID != 123 || states[0].SessionID != idLegacy || states[0].Status != "running" {
		t.Fatalf("Live = %+v, %v", states, err)
	}
}

func TestLiveErrorsAreNotIdle(t *testing.T) {
	p := New(t.TempDir())
	want := errors.New("process inspection denied")
	p.OpenFiles = func(context.Context) ([]byte, error) { return nil, want }
	if _, err := p.Live(context.Background()); !errors.Is(err, want) {
		t.Fatalf("Live = %v", err)
	}
}

func TestOpenFilesNoProcessesAndUnavailable(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	if _, err := codexOpenFiles(context.Background()); err == nil {
		t.Fatal("missing lsof must report unavailable")
	}
	path := filepath.Join(bin, "lsof")
	for _, tc := range []struct {
		script    string
		wantError bool
	}{
		{"#!/bin/sh\nexit 1\n", false},
		{"#!/bin/sh\necho 'permission denied' >&2\nexit 1\n", true},
		{"#!/bin/sh\necho 'incomplete process listing' >&2\nexit 0\n", true},
	} {
		if err := os.WriteFile(path, []byte(tc.script), 0o700); err != nil {
			t.Fatal(err)
		}
		_, err := codexOpenFiles(context.Background())
		if (err != nil) != tc.wantError {
			t.Fatalf("%q: %v", tc.script, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := codexOpenFiles(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
}

func TestLiveArchivedAndDeletedRollout(t *testing.T) {
	root := t.TempDir()
	realRoot, _ := filepath.EvalSymlinks(root)
	p := New(root)
	p.OpenFiles = func(context.Context) ([]byte, error) {
		path := filepath.Join(realRoot, "archived_sessions", "rollout-2026-10-04T08-00-00-"+idLegacy+".jsonl")
		return []byte("p123\x00n" + path + " (deleted)\x00"), nil
	}
	states, err := p.Live(context.Background())
	if err != nil || len(states) != 1 || !strings.EqualFold(states[0].SessionID, idLegacy) {
		t.Fatalf("Live = %v, %v", states, err)
	}
}
