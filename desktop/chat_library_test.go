package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestSavedWorkspacePersistsReopeningWithoutAgent(t *testing.T) {
	dir, project := t.TempDir(), t.TempDir()
	m := newChats(dir)
	defer m.close()
	source, err := m.create(project, "codex", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	review, err := m.create(project, "claude", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := m.get(review.ID)
	want := &ReviewState{SourceID: source.ID, Complete: true, Summary: "Private review summary",
		Snapshot: ReviewSnapshot{Diff: "PRIVATE_PATCH", Files: []ReviewFile{{Path: "app.go", Patch: "PRIVATE_PATCH"}}},
		Findings: []ReviewFinding{{ID: "finding", Title: "PRIVATE_FINDING", Sent: true}}}
	c.state.Review = want
	c.state.Messages = []ChatMessage{{Text: "PRIVATE_MESSAGE"}}
	if err := m.save(c); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{source.ID, review.ID} {
		if err := m.remove(id); err != nil {
			t.Fatal(err)
		}
	}
	if len(m.list()) != 0 {
		t.Fatal("closed records should not appear as open tabs")
	}
	library := m.library()
	if len(library) != 2 || library[0].ID != review.ID || !library[0].Closed ||
		!library[0].Complete || library[0].FindingCount != 1 || library[0].SentCount != 1 || library[0].FileCount != 1 {
		t.Fatalf("saved metadata: %+v", library)
	}
	data, _ := json.Marshal(library)
	if strings.Contains(string(data), "PRIVATE_") || strings.Contains(string(data), "Private review summary") {
		t.Fatal("library response exposed conversation or patch contents")
	}
	// Even a project that is no longer on disk must not prevent reading a review.
	if err := os.Remove(project); err != nil {
		t.Fatal(err)
	}
	restarted := newChats(dir)
	defer restarted.close()
	if err := restarted.load(); err != nil {
		t.Fatal(err)
	}
	reopened, err := restarted.reopen(review.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.ID != review.ID || reopened.Closed || !reflect.DeepEqual(reopened.Review, want) {
		t.Fatalf("review changed on reopen: %+v", reopened)
	}
	// A never-sent conversation also has to be recoverable: no native ID required.
	if state, err := restarted.reopen(source.ID); err != nil || state.SessionID != "" || state.Closed {
		t.Fatalf("reopening unsent conversation: %+v, %v", state, err)
	}
	if len(restarted.list()) != 2 {
		t.Fatal("reopening should restore the existing tabs")
	}
	again := newChats(dir)
	defer again.close()
	if err := again.load(); err != nil || len(again.list()) != 2 {
		t.Fatalf("reopened state did not survive restart: %v", err)
	}
	if _, err := restarted.reopen("../unknown"); err == nil {
		t.Fatal("unknown ID accepted")
	}
}

func TestSavedWorkspaceOpenLimitAndClosing(t *testing.T) {
	m := newChats("")
	defer m.close()
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("open-%d", i)
		m.items[id] = &chatSession{state: ChatState{ID: id}}
	}
	m.items["closed"] = &chatSession{state: ChatState{ID: "closed", Closed: true}}
	if _, err := m.reopen("closed"); err == nil || !strings.Contains(err.Error(), "maximum 100") {
		t.Fatalf("open tab limit: %v", err)
	}
	if _, err := m.reopen("open-0"); err != nil {
		t.Fatalf("already-open tab should be selectable: %v", err)
	}
	m.items["open-0"].state.Closed = true
	if _, err := m.reopen("closed"); err != nil {
		t.Fatal(err)
	}
	m.closed = true
	if _, err := m.reopen("closed"); err == nil {
		t.Fatal("reopened while app was closing")
	}
}

func TestSavedWorkspaceReopenSaveFailure(t *testing.T) {
	m := newChats("")
	defer m.close()
	state, err := m.create(t.TempDir(), "codex", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.remove(state.ID); err != nil {
		t.Fatal(err)
	}
	file := t.TempDir() + "/file"
	if err := os.WriteFile(file, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	m.dataDir = file
	if _, err := m.reopen(state.ID); err == nil {
		t.Fatal("save failure should be returned")
	}
	if len(m.list()) != 0 {
		t.Fatal("failed reopen should leave the tab closed")
	}
}
