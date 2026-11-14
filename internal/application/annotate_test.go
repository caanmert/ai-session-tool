package application

import (
	"context"
	"strings"
	"testing"
)

func TestAnnotate(t *testing.T) {
	s, session := fixtureService(t)
	ctx := context.Background()
	title, pinned := "  Webhook retries  ", true
	snapshot, err := s.Annotate(ctx, session.Key(), AnnotationChange{Title: &title, Pinned: &pinned, AddTags: []string{"#Backend", "urgent"}})
	if err != nil {
		t.Fatal(err)
	}
	got := snapshot.Sessions[0]
	if got.Title != "Webhook retries" || !got.Pinned || strings.Join(got.Tags, ",") != "backend,urgent" {
		t.Fatalf("annotations not applied: title=%q pinned=%v tags=%v", got.Title, got.Pinned, got.Tags)
	}

	// Nil fields are left alone; an empty title restores the agent's title.
	restore, unpinned := "", false
	snapshot, err = s.Annotate(ctx, session.Key(), AnnotationChange{Title: &restore, RemoveTags: []string{"urgent"}})
	if err != nil {
		t.Fatal(err)
	}
	got = snapshot.Sessions[0]
	if got.Title != "Retry webhook delivery" || !got.Pinned || strings.Join(got.Tags, ",") != "backend" {
		t.Fatalf("partial change: title=%q pinned=%v tags=%v", got.Title, got.Pinned, got.Tags)
	}
	if snapshot, err = s.Annotate(ctx, session.Key(), AnnotationChange{Pinned: &unpinned}); err != nil || snapshot.Sessions[0].Pinned {
		t.Fatalf("unpin: %v %+v", err, snapshot.Sessions)
	}

	long := strings.Repeat("x", 201)
	for name, change := range map[string]AnnotationChange{
		"long title": {Title: &long},
		"bad tag":    {AddTags: []string{"not a tag!"}},
	} {
		if _, err := s.Annotate(ctx, session.Key(), change); err == nil {
			t.Errorf("%s should be rejected", name)
		}
	}
	if _, err := s.Annotate(ctx, "claude/missing", AnnotationChange{Pinned: &pinned}); err == nil {
		t.Error("an unknown session should be rejected")
	}
	// A rejected change leaves the saved annotations as they were.
	after, _ := s.Refresh(ctx)
	if after.Sessions[0].Title != "Retry webhook delivery" || strings.Join(after.Sessions[0].Tags, ",") != "backend" {
		t.Fatalf("rejected changes leaked: %+v", after.Sessions[0])
	}
}
