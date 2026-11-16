package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestGitHubIssueAndPullDetail(t *testing.T) {
	repo := newGitRepo(t)
	m, _ := gitChat(t, repo.dir)
	github := &fakeGitHubLists{}
	m.gh = github.run
	ctx := context.Background()

	issue, err := m.githubIssue(ctx, repo.dir, 12)
	if err != nil || issue.State != "OPEN" || issue.Author != "ana" || issue.CommentCount != 1 || issue.Comments[0].Author != "bo" || issue.Labels[0].Color != "d73a4a" || issue.Shortened {
		t.Fatalf("issue: %+v %v", issue, err)
	}
	pull, err := m.githubPull(ctx, repo.dir, 7)
	if err != nil || pull.Branch != "ais/retry" || pull.Base != "main" || !pull.IsDraft || pull.Review != "CHANGES_REQUESTED" || pull.ChangedFiles != 3 || pull.Additions != 40 {
		t.Fatalf("pull: %+v %v", pull, err)
	}
	if got := fmt.Sprint(pull.Checks); got != "[{test passing} {lint failing} {deploy running} {docs skipped}]" {
		t.Fatalf("checks: %s", got)
	}
	// Only reads.
	for _, call := range github.calls {
		if !strings.Contains(call, " view ") && !strings.HasPrefix(call, "repo view") {
			t.Fatalf("unexpected call: %s", call)
		}
	}
	if _, err := m.githubPull(ctx, repo.dir, 8); err == nil {
		t.Fatal("a different pull request in the answer must be refused")
	}
	if _, err := m.githubPull(ctx, repo.dir, 0); err == nil {
		t.Fatal("number zero must be refused")
	}
}

func TestGitHubDetailIsBounded(t *testing.T) {
	raw := ghDetail{Number: 1, Body: strings.Repeat("b", detailBodyLimit+50)}
	for i := 0; i < detailCommentLimit+5; i++ {
		raw.Comments = append(raw.Comments, struct {
			Author    ghAuthor `json:"author"`
			Body      string   `json:"body"`
			CreatedAt string   `json:"createdAt"`
		}{Author: ghAuthor{Login: fmt.Sprint(i)}, Body: strings.Repeat("c", commentBodyLimit+10)})
	}
	detail := raw.detail()
	if !detail.Shortened || len([]rune(detail.Body)) > detailBodyLimit+40 {
		t.Fatalf("body: shortened %v, %d", detail.Shortened, len(detail.Body))
	}
	// The latest comments are kept, and the count says how many there are.
	if len(detail.Comments) != detailCommentLimit || detail.Comments[0].Author != "5" || detail.CommentCount != detailCommentLimit+5 {
		t.Fatalf("comments: %d first %s count %d", len(detail.Comments), detail.Comments[0].Author, detail.CommentCount)
	}
	if len([]rune(detail.Comments[0].Body)) > commentBodyLimit+40 {
		t.Fatal("a long comment must be cut")
	}
	if got := checkList(json.RawMessage(`null`)); len(got) != 0 {
		t.Fatalf("no checks: %v", got)
	}
}
