package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitRepo is a temporary repository on main with one commit and a bare
// remote named origin, isolated from the user's global hooks and signing.
type gitRepo struct {
	t      *testing.T
	dir    string
	remote string
}

func newGitRepo(t *testing.T) *gitRepo {
	t.Helper()
	r := &gitRepo{t: t, dir: t.TempDir(), remote: t.TempDir()}
	r.run("init", "-q", "-b", "main")
	for key, value := range map[string]string{"user.name": "ais", "user.email": "ais@example.invalid", "commit.gpgsign": "false", "core.hooksPath": filepath.Join(r.dir, ".git", "test-hooks")} {
		r.run("config", key, value)
	}
	r.write("a.txt", "one\n")
	r.write("b.txt", "two\n")
	r.write("c.txt", "three\n")
	r.write("keep.txt", "four\n")
	r.run("add", "-A")
	r.run("commit", "-q", "-m", "initial")
	if out, err := exec.Command("git", "init", "-q", "--bare", "-b", "main", r.remote).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	r.run("remote", "add", "origin", r.remote)
	r.run("push", "-q", "-u", "origin", "main")
	r.run("remote", "set-head", "origin", "main")
	return r
}

func (r *gitRepo) run(args ...string) string {
	r.t.Helper()
	out, err := exec.Command("git", append([]string{"-C", r.dir}, args...)...).CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r *gitRepo) write(name, content string) {
	r.t.Helper()
	path := filepath.Join(r.dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		r.t.Fatal(err)
	}
}

func (r *gitRepo) hook(name, script string) {
	r.t.Helper()
	dir := filepath.Join(r.dir, ".git", "test-hooks")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil {
		r.t.Fatal(err)
	}
}

// messy leaves the working tree with every kind of change: a modified file,
// another modified file that is already staged, a deleted file, a new file,
// and one more modified file that will not be selected.
func (r *gitRepo) messy() {
	r.write("a.txt", "one changed\n")
	r.write("b.txt", "two staged\n")
	r.run("add", "b.txt")
	if err := os.Remove(filepath.Join(r.dir, "c.txt")); err != nil {
		r.t.Fatal(err)
	}
	r.write("new.txt", "brand new\n")
	r.write("keep.txt", "four, not committed\n")
}

func gitChat(t *testing.T, dir string) (*chatManager, ChatState) {
	t.Helper()
	m := newChats("")
	t.Cleanup(m.close)
	m.gh = func(context.Context, string, ...string) (string, error) {
		return "", errors.New("gh is not used in this test")
	}
	state, err := m.create(dir, "claude", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := m.get(state.ID)
	c.mu.Lock()
	c.state.Title = "Add retries to the checkout request"
	c.mu.Unlock()
	return m, state
}

func TestCommitSelectedFiles(t *testing.T) {
	repo := newGitRepo(t)
	repo.messy()
	m, chat := gitChat(t, repo.dir)
	ctx := context.Background()

	plan, err := m.commitPlan(ctx, chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Branch != "main" || plan.DefaultBranch != "main" || !plan.OnDefault || len(plan.Files) != 5 {
		t.Fatalf("plan: %+v", plan)
	}
	if plan.Message != "Add retries to the checkout request" || plan.NewBranch != "ais/add-retries-to-the-checkout-request" {
		t.Fatalf("suggestions: %q %q", plan.Message, plan.NewBranch)
	}

	result, err := m.commit(ctx, chat.ID, CommitRequest{Message: "Retry checkout\n\nKeeps the request key.", Files: []string{"a.txt", "c.txt", "new.txt", "a.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Branch != "main" || result.Subject != "Retry checkout" || result.Files != 3 || result.Commit == "" {
		t.Fatalf("result: %+v", result)
	}
	if got := repo.run("show", "--name-status", "--format=%s", "HEAD"); got != "Retry checkout\n\nM\ta.txt\nD\tc.txt\nA\tnew.txt" {
		t.Fatalf("commit contents:\n%s", got)
	}
	// Work that was not selected is exactly as it was: still staged, or
	// still unstaged.
	if staged := repo.run("diff", "--cached", "--name-only"); staged != "b.txt" {
		t.Fatalf("other staged work changed: %q", staged)
	}
	if unstaged := repo.run("diff", "--name-only"); unstaged != "keep.txt" {
		t.Fatalf("unstaged work changed: %q", unstaged)
	}
	state, _ := m.read(chat.ID, -1)
	last := state.State.Messages[len(state.State.Messages)-1]
	if last.Role != "system" || !strings.Contains(last.Text, "Committed "+result.Commit+" on main: Retry checkout (3 files)") {
		t.Fatalf("the conversation should record the commit: %+v", last)
	}
}

func TestCommitOnNewBranch(t *testing.T) {
	repo := newGitRepo(t)
	repo.messy()
	m, chat := gitChat(t, repo.dir)
	before := repo.run("rev-parse", "main")
	result, err := m.commit(context.Background(), chat.ID, CommitRequest{Message: "Retry", Files: []string{"a.txt"}, NewBranch: "ais/retry"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Branch != "ais/retry" || repo.run("branch", "--show-current") != "ais/retry" {
		t.Fatalf("expected to be on the new branch: %+v", result)
	}
	if repo.run("rev-parse", "main") != before {
		t.Fatal("main must not move")
	}
}

func TestCommitHookFailureLeavesRepositoryAsItWas(t *testing.T) {
	repo := newGitRepo(t)
	repo.messy()
	repo.hook("pre-commit", "echo 'lint failed: a.txt' >&2\nexit 1")
	m, chat := gitChat(t, repo.dir)
	head := repo.run("rev-parse", "HEAD")
	status := repo.run("status", "--porcelain")
	_, err := m.commit(context.Background(), chat.ID, CommitRequest{Message: "Retry", Files: []string{"a.txt", "new.txt"}, NewBranch: "ais/retry"})
	if err == nil || !strings.Contains(err.Error(), "lint failed: a.txt") {
		t.Fatalf("the hook's message should be reported: %v", err)
	}
	if repo.run("rev-parse", "HEAD") != head || repo.run("branch", "--show-current") != "main" {
		t.Fatal("HEAD and branch must be restored")
	}
	if strings.Contains(repo.run("branch", "--list", "ais/retry"), "ais/retry") {
		t.Fatal("the new branch must be removed again")
	}
	if got := repo.run("status", "--porcelain"); got != status {
		t.Fatalf("working tree and index must be as before:\nbefore:\n%s\nafter:\n%s", status, got)
	}
}

func TestCommitRefusals(t *testing.T) {
	repo := newGitRepo(t)
	repo.messy()
	repo.run("branch", "taken")
	m, chat := gitChat(t, repo.dir)
	ctx := context.Background()
	for name, tc := range map[string]struct {
		request CommitRequest
		want    string
	}{
		"no message":       {CommitRequest{Message: "  ", Files: []string{"a.txt"}}, "write a commit message"},
		"no files":         {CommitRequest{Message: "x"}, "choose at least one file"},
		"unchanged file":   {CommitRequest{Message: "x", Files: []string{"README"}}, "no uncommitted changes"},
		"outside path":     {CommitRequest{Message: "x", Files: []string{"../secret"}}, "no uncommitted changes"},
		"bad branch name":  {CommitRequest{Message: "x", Files: []string{"a.txt"}, NewBranch: "bad..name"}, "not a valid branch name"},
		"option as branch": {CommitRequest{Message: "x", Files: []string{"a.txt"}, NewBranch: "-delete"}, "not a valid branch name"},
		"existing branch":  {CommitRequest{Message: "x", Files: []string{"a.txt"}, NewBranch: "taken"}, "already exists"},
	} {
		if _, err := m.commit(ctx, chat.ID, tc.request); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", name, err, tc.want)
		}
	}
	c, _ := m.get(chat.ID)
	c.mu.Lock()
	c.state.Status = "running"
	c.mu.Unlock()
	if _, err := m.commit(ctx, chat.ID, CommitRequest{Message: "x", Files: []string{"a.txt"}}); err == nil || !strings.Contains(err.Error(), "wait for the agent") {
		t.Fatalf("a running conversation must not commit: %v", err)
	}
	c.mu.Lock()
	c.state.Status = "idle"
	c.mu.Unlock()
	repo.run("checkout", "-q", "--detach")
	if _, err := m.commitPlan(ctx, chat.ID); err == nil || !strings.Contains(err.Error(), "detached HEAD") {
		t.Fatalf("detached HEAD: %v", err)
	}
	if head := repo.run("log", "--format=%s"); head != "initial" {
		t.Fatalf("nothing may have been committed: %q", head)
	}
}

// fakeGitHub stands in for the GitHub CLI and records what it was asked.
type fakeGitHub struct {
	calls  [][]string
	open   string
	failOn string
}

func (f *fakeGitHub) run(_ context.Context, _ string, args ...string) (string, error) {
	f.calls = append(f.calls, args)
	command := strings.Join(args[:2], " ")
	if command == f.failOn {
		return "", errors.New("HTTP 401: Bad credentials")
	}
	switch command {
	case "repo view":
		return `{"nameWithOwner":"me/app","defaultBranchRef":{"name":"main"}}`, nil
	case "pr list":
		if f.open == "" {
			return "[]", nil
		}
		return f.open, nil
	case "pr create":
		return "Creating pull request\nhttps://github.com/me/app/pull/7", nil
	}
	return "", errors.New("unexpected gh call: " + command)
}

func (f *fakeGitHub) created() []string {
	for _, call := range f.calls {
		if call[0] == "pr" && call[1] == "create" {
			return call
		}
	}
	return nil
}

func TestPullRequestFlow(t *testing.T) {
	repo := newGitRepo(t)
	m, chat := gitChat(t, repo.dir)
	github := &fakeGitHub{}
	m.gh = github.run
	ctx := context.Background()
	repo.run("switch", "-q", "-c", "ais/retry")
	repo.write("a.txt", "retry\n")
	repo.run("commit", "-q", "-am", "Retry checkout\n\nKeeps the request key.")
	repo.write("keep.txt", "uncommitted\n")

	plan, _, err := m.pullRequestPlan(ctx, chat.ID)
	if err != nil || plan.Reason != "" {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	if plan.Repository != "me/app" || plan.Base != "main" || plan.Branch != "ais/retry" || plan.Remote != "origin" {
		t.Fatalf("plan target: %+v", plan)
	}
	if plan.Unpushed != 1 || len(plan.Commits) != 1 || plan.Title != "Retry checkout" || plan.Body != "Keeps the request key." || plan.Uncommitted != 1 {
		t.Fatalf("plan contents: %+v", plan)
	}

	result, err := m.createPullRequest(ctx, chat.ID, PullRequestRequest{Title: "Retry checkout", Body: "Why", Draft: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.Number != 7 || result.URL != "https://github.com/me/app/pull/7" || result.Pushed != 1 {
		t.Fatalf("result: %+v", result)
	}
	if out, _ := exec.Command("git", "-C", repo.remote, "rev-parse", "refs/heads/ais/retry").Output(); strings.TrimSpace(string(out)) != repo.run("rev-parse", "HEAD") {
		t.Fatal("the branch should be on the remote")
	}
	want := "pr create --repo me/app --head ais/retry --base main --title Retry checkout --body Why --draft"
	if got := strings.Join(github.created(), " "); got != want {
		t.Fatalf("gh call:\n got %s\nwant %s", got, want)
	}

	// With a pull request already open, new commits are pushed to it and no
	// second one is opened.
	github.calls = nil
	github.open = `[{"number":7,"url":"https://github.com/me/app/pull/7","title":"Retry checkout"}]`
	repo.run("commit", "-q", "-am", "Add a test", "--allow-empty")
	plan, _, _ = m.pullRequestPlan(ctx, chat.ID)
	if plan.Existing == nil || plan.Existing.Number != 7 || plan.Unpushed != 1 {
		t.Fatalf("existing pull request: %+v", plan)
	}
	result, err = m.createPullRequest(ctx, chat.ID, PullRequestRequest{})
	if err != nil || result.Created || result.Number != 7 || result.Pushed != 1 || github.created() != nil {
		t.Fatalf("push to existing: %+v %v %v", result, err, github.calls)
	}
	// Nothing new to push: nothing happens.
	result, err = m.createPullRequest(ctx, chat.ID, PullRequestRequest{})
	if err != nil || result.Pushed != 0 || result.Number != 7 {
		t.Fatalf("up to date: %+v %v", result, err)
	}
}

func TestPullRequestPlanReasons(t *testing.T) {
	repo := newGitRepo(t)
	m, chat := gitChat(t, repo.dir)
	github := &fakeGitHub{}
	m.gh = github.run
	ctx := context.Background()
	reason := func() string {
		t.Helper()
		plan, _, err := m.pullRequestPlan(ctx, chat.ID)
		if err != nil {
			t.Fatal(err)
		}
		return plan.Reason
	}
	if got := reason(); !strings.Contains(got, "You are on main") {
		t.Fatalf("on the base branch: %q", got)
	}
	repo.run("switch", "-q", "-c", "ais/empty")
	if got := reason(); !strings.Contains(got, "no commits that are not on main") {
		t.Fatalf("nothing to propose: %q", got)
	}
	github.failOn = "repo view"
	if got := reason(); !strings.Contains(got, "Bad credentials") {
		t.Fatalf("gh failure: %q", got)
	}
	repo.run("remote", "remove", "origin")
	if got := reason(); !strings.Contains(got, "no remote named origin") {
		t.Fatalf("no remote: %q", got)
	}
	if _, err := m.createPullRequest(ctx, chat.ID, PullRequestRequest{Title: "x"}); err == nil {
		t.Fatal("nothing may be pushed when the plan has a reason")
	}
}
