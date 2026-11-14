package application

import (
	"context"
	"errors"
	"unicode/utf8"
)

// AnnotationChange describes edits to a session's ais annotations. Nil fields
// are left as they are. Annotations belong to ais: the agent's own transcript
// is never modified.
type AnnotationChange struct {
	// Title renames the session in ais. An empty title restores the agent's.
	Title *string `json:"title,omitempty"`
	// Pinned keeps the session at the top of lists.
	Pinned     *bool    `json:"pinned,omitempty"`
	AddTags    []string `json:"addTags,omitempty"`
	RemoveTags []string `json:"removeTags,omitempty"`
}

const maxTitleLength = 200

// Annotate applies a change to one session and returns the refreshed snapshot.
func (s *Service) Annotate(ctx context.Context, key string, change AnnotationChange) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.find(key)
	if err != nil {
		return Snapshot{}, err
	}
	if s.store == nil {
		return Snapshot{}, errors.New("annotations are unavailable; refresh and try again")
	}
	if change.Title != nil {
		if utf8.RuneCountInString(*change.Title) > maxTitleLength {
			return Snapshot{}, errors.New("titles can be at most 200 characters")
		}
		if err := s.store.SetTitle(ctx, session, *change.Title); err != nil {
			return Snapshot{}, err
		}
	}
	if change.Pinned != nil {
		if err := s.store.SetPinned(ctx, session, *change.Pinned); err != nil {
			return Snapshot{}, err
		}
	}
	if len(change.AddTags) > 0 || len(change.RemoveTags) > 0 {
		if err := s.store.Tag(ctx, session, change.AddTags, change.RemoveTags); err != nil {
			return Snapshot{}, err
		}
	}
	return s.refresh(ctx)
}
