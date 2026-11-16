package main

import (
	"context"
	"strings"
	"testing"
)

func TestGitHubIssueForm(t *testing.T) {
	repo := newGitRepo(t)
	m, _ := gitChat(t, repo.dir)
	github := &fakeGitHubLists{}
	m.gh = github.run
	ctx := context.Background()

	form, err := m.githubIssueForm(ctx, repo.dir)
	if err != nil || form.Reason != "" || form.Repository != "me/app" || len(form.Labels) != 2 || form.Labels[0].Name != "bug" || form.Labels[0].Color != "d73a4a" {
		t.Fatalf("form: %+v %v", form, err)
	}
	for _, call := range github.calls {
		if strings.Contains(call, "create") {
			t.Fatalf("reading the form must not create anything: %s", call)
		}
	}
	// Labels are optional: the form still works without them.
	github.fail = map[string]bool{"label list": true}
	if form, _ = m.githubIssueForm(ctx, repo.dir); form.Reason != "" || !strings.Contains(form.Partial, "Labels could not be read") {
		t.Fatalf("labels failing: %+v", form)
	}
	if form, _ = m.githubIssueForm(ctx, t.TempDir()); !strings.Contains(form.Reason, "not a Git repository") {
		t.Fatalf("plain folder: %+v", form)
	}
}

func TestCreateGitHubIssue(t *testing.T) {
	repo := newGitRepo(t)
	m, _ := gitChat(t, repo.dir)
	github := &fakeGitHubLists{}
	m.gh = github.run
	ctx := context.Background()

	created, err := m.createGitHubIssue(ctx, repo.dir, "  Retry checkout twice  ", "Timeouts charge twice.\n\n- one charge", []string{"bug", "-flaky", "bug"})
	if err != nil || created.Number != 13 || created.URL != "https://github.com/me/app/issues/13" {
		t.Fatalf("created: %+v %v", created, err)
	}
	last := github.calls[len(github.calls)-1]
	// One value per flag in the = form, so text that starts with "-" stays text.
	want := "issue create --repo me/app --title=Retry checkout twice --body=Timeouts charge twice.\n\n- one charge --label=bug --label=-flaky"
	if last != want {
		t.Fatalf("gh call:\n got %q\nwant %q", last, want)
	}
	for _, call := range github.calls {
		for _, never := range []string{"--assignee", "--project", "--milestone", "--web"} {
			if strings.Contains(call, never) {
				t.Fatalf("unexpected %s in %s", never, call)
			}
		}
	}

	// Everything is checked before anything is sent.
	for _, tc := range []struct {
		title, body string
		labels      []string
		want        string
	}{
		{"   ", "", nil, "give the issue a title"},
		{"Two\nlines", "", nil, "one line"},
		{strings.Repeat("t", 257), "", nil, "longer than 256"},
		{"Fine", strings.Repeat("b", 65537), nil, "longer than GitHub allows"},
		{"Fine", "", []string{"no-such-label"}, `no label named "no-such-label"`},
	} {
		github.calls = nil
		if _, err := m.createGitHubIssue(ctx, repo.dir, tc.title, tc.body, tc.labels); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%q: got %v, want %q", tc.title, err, tc.want)
		}
		for _, call := range github.calls {
			if strings.HasPrefix(call, "issue create") {
				t.Fatalf("%q: an issue was created anyway", tc.title)
			}
		}
	}
	// A title of 256 characters and the longest description are allowed.
	if _, err := m.createGitHubIssue(ctx, repo.dir, strings.Repeat("é", 256), strings.Repeat("b", 65536), nil); err != nil {
		t.Fatalf("limits are inclusive: %v", err)
	}

	github.fail = map[string]bool{"issue create": true}
	if _, err := m.createGitHubIssue(ctx, repo.dir, "Fine", "", nil); err == nil || !strings.Contains(err.Error(), "GitHub did not create the issue: HTTP 502") {
		t.Fatalf("gh failing: %v", err)
	}
	if _, err := m.createGitHubIssue(ctx, t.TempDir(), "Fine", "", nil); err == nil || !strings.Contains(err.Error(), "not a Git repository") {
		t.Fatalf("plain folder: %v", err)
	}
}
