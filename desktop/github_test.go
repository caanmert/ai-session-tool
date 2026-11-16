package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fakeGitHubLists struct {
	calls []string
	fail  map[string]bool
}

func (f *fakeGitHubLists) run(_ context.Context, _ string, args ...string) (string, error) {
	command := strings.Join(args[:2], " ")
	f.calls = append(f.calls, strings.Join(args, " "))
	if f.fail[command] {
		return "", errors.New("HTTP 502: server error")
	}
	switch command {
	case "repo view":
		return `{"nameWithOwner":"me/app","url":"https://github.com/me/app"}`, nil
	case "issue list":
		return `[{"number":12,"title":"Retry checkout","url":"https://github.com/me/app/issues/12","author":{"login":"ana"},"labels":[{"name":"bug","color":"d73a4a"}],"updatedAt":"2026-11-15T10:00:00Z"}]`, nil
	case "pr list":
		return `[{"number":7,"title":"Add retries","url":"https://github.com/me/app/pull/7","author":{"login":"bo"},"headRefName":"ais/retry","isDraft":true,"reviewDecision":"CHANGES_REQUESTED",
			"statusCheckRollup":[{"__typename":"CheckRun","status":"COMPLETED","conclusion":"SUCCESS"},{"__typename":"CheckRun","status":"COMPLETED","conclusion":"FAILURE"}],"updatedAt":"2026-11-15T11:00:00Z"}]`, nil
	case "label list":
		return `[{"name":"bug","color":"d73a4a","description":"Something isn't working"},{"name":"-flaky","color":"ededed","description":""}]`, nil
	case "issue create":
		return "https://github.com/me/app/issues/13", nil
	case "issue view":
		return `{"number":12,"title":"Retry checkout","body":"Timeouts charge twice.\n\n## Done means\n- one charge","url":"https://github.com/me/app/issues/12"}`, nil
	}
	return "", errors.New("unexpected gh call: " + command)
}

func TestGitHubOverview(t *testing.T) {
	repo := newGitRepo(t)
	m, _ := gitChat(t, repo.dir)
	github := &fakeGitHubLists{}
	m.gh = github.run
	ctx := context.Background()

	overview, err := m.githubOverview(ctx, repo.dir)
	if err != nil || overview.Reason != "" || overview.Partial != "" {
		t.Fatalf("overview: %+v %v", overview, err)
	}
	if overview.Repository != "me/app" || len(overview.Issues) != 1 || len(overview.Pulls) != 1 {
		t.Fatalf("lists: %+v", overview)
	}
	issue, pull := overview.Issues[0], overview.Pulls[0]
	if issue.Number != 12 || issue.Author != "ana" || strings.Join(issue.Labels, ",") != "bug" {
		t.Fatalf("issue: %+v", issue)
	}
	if pull.Branch != "ais/retry" || !pull.IsDraft || pull.Review != "CHANGES_REQUESTED" || pull.Checks != "failing" {
		t.Fatalf("pull request: %+v", pull)
	}
	// Only reads: every call is a view or a list.
	for _, call := range github.calls {
		if !strings.Contains(call, " list ") && !strings.Contains(call, " view ") && !strings.HasPrefix(call, "repo view") {
			t.Fatalf("unexpected call: %s", call)
		}
	}

	github.fail = map[string]bool{"pr list": true}
	overview, _ = m.githubOverview(ctx, repo.dir)
	if overview.Reason != "" || len(overview.Issues) != 1 || !strings.Contains(overview.Partial, "Pull requests: HTTP 502") {
		t.Fatalf("one list failing: %+v", overview)
	}
	github.fail = map[string]bool{"pr list": true, "issue list": true}
	if overview, _ = m.githubOverview(ctx, repo.dir); !strings.Contains(overview.Reason, "could not be read") {
		t.Fatalf("both lists failing: %+v", overview)
	}
	github.fail = map[string]bool{"repo view": true}
	if overview, _ = m.githubOverview(ctx, repo.dir); !strings.Contains(overview.Reason, "HTTP 502") {
		t.Fatalf("no repository: %+v", overview)
	}
	if overview, _ = m.githubOverview(ctx, t.TempDir()); !strings.Contains(overview.Reason, "not a Git repository") {
		t.Fatalf("plain folder: %+v", overview)
	}
	if _, err := m.githubOverview(ctx, "relative/path"); err == nil {
		t.Fatal("a relative path must be refused")
	}
}

func TestGitHubIssueDetail(t *testing.T) {
	repo := newGitRepo(t)
	m, _ := gitChat(t, repo.dir)
	m.gh = (&fakeGitHubLists{}).run
	detail, err := m.githubIssue(context.Background(), repo.dir, 12)
	if err != nil || detail.Title != "Retry checkout" || !strings.Contains(detail.Body, "Done means") {
		t.Fatalf("detail: %+v %v", detail, err)
	}
	if _, err := m.githubIssue(context.Background(), repo.dir, 13); err == nil {
		t.Fatal("a different issue in the answer must be refused")
	}
	if _, err := m.githubIssue(context.Background(), repo.dir, 0); err == nil {
		t.Fatal("issue number zero must be refused")
	}
}

func TestCheckSummary(t *testing.T) {
	for _, tc := range []struct {
		checks string
		want   string
	}{
		{`[]`, ""},
		{`null`, ""},
		{`[{"status":"COMPLETED","conclusion":"SUCCESS"},{"state":"SUCCESS"},{"conclusion":"SKIPPED"}]`, "passing"},
		{`[{"status":"IN_PROGRESS","conclusion":""},{"conclusion":"SUCCESS"}]`, "running"},
		{`[{"state":"PENDING"}]`, "running"},
		{`[{"status":"IN_PROGRESS"},{"conclusion":"FAILURE"}]`, "failing"},
		{`[{"state":"ERROR"}]`, "failing"},
	} {
		if got := checkSummary(json.RawMessage(tc.checks)); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.checks, got, tc.want)
		}
	}
}
