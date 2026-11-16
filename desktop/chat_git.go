package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Committing and opening pull requests change the user's repository and, for
// a pull request, GitHub. Both happen only when the user confirms them in the
// app; nothing here runs on its own.

// boundedBuffer keeps at most limit bytes and drops the rest, so a noisy hook
// cannot use unbounded memory.
type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.Len(); room > 0 {
		if len(p) > room {
			b.Buffer.Write(p[:room])
		} else {
			b.Buffer.Write(p)
		}
	}
	return len(p), nil
}

// gitRun runs git for a change the user asked for. It never prompts for
// credentials, and an error carries git's own message (including hook output).
func gitRun(ctx context.Context, dir string, args ...string) (string, error) {
	return gitRunWith(ctx, dir, nil, args...)
}

// gitRunWith is gitRun with extra environment, such as a private index.
func gitRunWith(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-pager", "--literal-pathspecs", "-c", "core.fsmonitor=false", "-C", dir}, args...)...)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(name, "GIT_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(append(cmd.Env, "GIT_TERMINAL_PROMPT=0"), env...)
	stdout, stderr := &boundedBuffer{limit: 1 << 20}, &boundedBuffer{limit: 64 << 10}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		return "", errors.New(shorten(message, 4000))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// runGH runs the GitHub CLI without prompts. The user's own sign-in is used.
func runGH(ctx context.Context, dir string, args ...string) (string, error) {
	path, err := exec.LookPath("gh")
	if err != nil {
		return "", errGHMissing
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1", "NO_COLOR=1")
	stdout, stderr := &boundedBuffer{limit: 4 << 20}, &boundedBuffer{limit: 64 << 10}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", errors.New(shorten(message, 2000))
	}
	return strings.TrimSpace(stdout.String()), nil
}

var errGHMissing = errors.New("the GitHub CLI (gh) is not installed. Install it and run `gh auth login`")

// repoRoot returns the top of the Git repository that holds dir.
func repoRoot(ctx context.Context, dir string) (string, error) {
	if dir == "" {
		return "", errors.New("this conversation has no project folder")
	}
	root, err := gitRun(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New("this project is not a Git repository")
	}
	if _, err := gitRun(ctx, root, "rev-parse", "--verify", "HEAD"); err != nil {
		return "", errors.New("this repository has no commits yet; make the first commit in a terminal")
	}
	return root, nil
}

func currentBranch(ctx context.Context, root string) (string, error) {
	branch, err := gitRun(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || branch == "" {
		return "", errors.New("the repository is not on a branch (detached HEAD); check out a branch first")
	}
	return branch, nil
}

// defaultBranch is the branch pull requests usually go to: the one origin's
// HEAD points at, or a local main or master.
func defaultBranch(ctx context.Context, root string) string {
	if ref, err := gitRun(ctx, root, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if _, name, ok := strings.Cut(ref, "/"); ok && name != "" {
			return name
		}
	}
	for _, name := range []string{"main", "master"} {
		if branchExists(ctx, root, name) {
			return name
		}
	}
	return ""
}

func branchExists(ctx context.Context, root, name string) bool {
	_, err := gitRun(ctx, root, "rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

var notSlug = regexp.MustCompile(`[^a-z0-9]+`)

// suggestBranch turns a conversation title into a branch name that does not
// exist yet, such as ais/add-retries-to-checkout.
func suggestBranch(ctx context.Context, root, title string) string {
	slug := strings.Trim(notSlug.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(slug) > 40 {
		slug = slug[:40]
		if cut := strings.LastIndex(slug, "-"); cut > 20 {
			slug = slug[:cut]
		}
		slug = strings.Trim(slug, "-")
	}
	if slug == "" {
		slug = "changes"
	}
	name := "ais/" + slug
	for i := 2; branchExists(ctx, root, name) && i < 100; i++ {
		name = fmt.Sprintf("ais/%s-%d", slug, i)
	}
	return name
}

// CommitPlan is what the commit dialog shows before anything happens.
type CommitPlan struct {
	Branch        string        `json:"branch"`
	DefaultBranch string        `json:"defaultBranch,omitempty"`
	OnDefault     bool          `json:"onDefault"`
	Files         []ChangedFile `json:"files"`
	Added         int           `json:"added"`
	Removed       int           `json:"removed"`
	Truncated     bool          `json:"truncated,omitempty"`
	Message       string        `json:"message"`
	NewBranch     string        `json:"newBranch"`
}

// CommitRequest is what the user confirmed in the commit dialog.
type CommitRequest struct {
	Message string   `json:"message"`
	Files   []string `json:"files"`
	// NewBranch, when set, is created from HEAD and the commit is made there.
	NewBranch string `json:"newBranch,omitempty"`
}

// CommitResult describes a commit that was made.
type CommitResult struct {
	Commit  string `json:"commit"`
	Branch  string `json:"branch"`
	Subject string `json:"subject"`
	Files   int    `json:"files"`
}

func (m *chatManager) commitPlan(ctx context.Context, id string) (CommitPlan, error) {
	c, err := m.get(id)
	if err != nil {
		return CommitPlan{}, err
	}
	c.mu.Lock()
	project, title, review := c.state.Project, c.state.Title, c.state.Review != nil
	c.mu.Unlock()
	if review {
		return CommitPlan{}, errors.New("a review only reads a snapshot; commit from the original conversation")
	}
	root, err := repoRoot(ctx, project)
	if err != nil {
		return CommitPlan{}, err
	}
	branch, err := currentBranch(ctx, root)
	if err != nil {
		return CommitPlan{}, err
	}
	summary := summarizeChanges(ctx, root)
	if !summary.Available {
		return CommitPlan{}, errors.New(summary.Reason)
	}
	base := defaultBranch(ctx, root)
	message := strings.TrimSpace(title)
	if message == "" || message == "New conversation" {
		message = ""
	}
	return CommitPlan{
		Branch: branch, DefaultBranch: base, OnDefault: base != "" && branch == base,
		Files: summary.Files, Added: summary.Added, Removed: summary.Removed, Truncated: summary.Truncated,
		Message: message, NewBranch: suggestBranch(ctx, root, title),
	}, nil
}

func (m *chatManager) commit(ctx context.Context, id string, request CommitRequest) (CommitResult, error) {
	c, err := m.get(id)
	if err != nil {
		return CommitResult{}, err
	}
	// Hold the conversation so no new response starts while committing.
	c.settingsMu.Lock()
	defer c.settingsMu.Unlock()
	c.mu.Lock()
	project, status, review := c.state.Project, c.state.Status, c.state.Review != nil
	c.mu.Unlock()
	if review {
		return CommitResult{}, errors.New("a review only reads a snapshot; commit from the original conversation")
	}
	if busyChat(status) {
		return CommitResult{}, errors.New("wait for the agent to finish, or stop it, before committing")
	}
	message := strings.TrimSpace(strings.ReplaceAll(request.Message, "\r\n", "\n"))
	if message == "" {
		return CommitResult{}, errors.New("write a commit message")
	}
	if len(message) > 20000 || strings.ContainsRune(message, 0) {
		return CommitResult{}, errors.New("the commit message is too long or contains invalid characters")
	}
	root, err := repoRoot(ctx, project)
	if err != nil {
		return CommitResult{}, err
	}
	branch, err := currentBranch(ctx, root)
	if err != nil {
		return CommitResult{}, err
	}
	summary := summarizeChanges(ctx, root)
	if !summary.Available {
		return CommitResult{}, errors.New(summary.Reason)
	}
	known := map[string]string{}
	for _, file := range summary.Files {
		known[file.Path] = file.Status
	}
	// Only files that have uncommitted changes right now can be committed.
	selected, untracked, seen := []string{}, []string{}, map[string]bool{}
	for _, path := range request.Files {
		status, ok := known[path]
		if !ok {
			return CommitResult{}, fmt.Errorf("%s has no uncommitted changes; refresh and try again", path)
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		selected = append(selected, path)
		if status == "?" {
			untracked = append(untracked, path)
		}
	}
	if len(selected) == 0 {
		return CommitResult{}, errors.New("choose at least one file to commit")
	}
	newBranch := strings.TrimSpace(request.NewBranch)
	if newBranch != "" {
		if _, err := gitRun(ctx, root, "check-ref-format", "--branch", newBranch); err != nil || strings.HasPrefix(newBranch, "-") {
			return CommitResult{}, fmt.Errorf("%q is not a valid branch name", newBranch)
		}
		if branchExists(ctx, root, newBranch) {
			return CommitResult{}, fmt.Errorf("a branch named %s already exists", newBranch)
		}
		if _, err := gitRun(ctx, root, "switch", "--create", newBranch); err != nil {
			return CommitResult{}, fmt.Errorf("could not create branch %s: %w", newBranch, err)
		}
	}
	// Put the repository back as it was if the commit is not made, for
	// example because a hook rejected it.
	undo := func() {
		if len(untracked) > 0 {
			_, _ = gitRun(context.Background(), root, append([]string{"reset", "--quiet", "--"}, untracked...)...)
		}
		if newBranch != "" {
			if _, err := gitRun(context.Background(), root, "switch", branch); err == nil {
				_, _ = gitRun(context.Background(), root, "branch", "--delete", "--force", newBranch)
			}
		}
	}
	if len(untracked) > 0 {
		if _, err := gitRun(ctx, root, append([]string{"add", "--"}, untracked...)...); err != nil {
			undo()
			return CommitResult{}, fmt.Errorf("could not add new files: %w", err)
		}
	}
	// --only commits the working-tree state of exactly these files and keeps
	// anything else that was staged out of this commit. Hooks and signing run
	// as they would in a terminal.
	if _, err := gitRun(ctx, root, append([]string{"commit", "--only", "--message", message, "--"}, selected...)...); err != nil {
		undo()
		return CommitResult{}, fmt.Errorf("the commit was not made: %w", err)
	}
	hash, _ := gitRun(ctx, root, "rev-parse", "--short", "HEAD")
	subject, _, _ := strings.Cut(message, "\n")
	if newBranch != "" {
		branch = newBranch
	}
	noun := "files"
	if len(selected) == 1 {
		noun = "file"
	}
	c.message(ChatMessage{ID: "commit/" + hash, Role: "system", Kind: "text", Text: fmt.Sprintf("Committed %s on %s: %s (%d %s)", hash, branch, subject, len(selected), noun)}, false)
	m.saveOrWarn(c)
	return CommitResult{Commit: hash, Branch: branch, Subject: subject, Files: len(selected)}, nil
}

// PullRequestInfo is an open pull request on GitHub.
type PullRequestInfo struct {
	Number  int    `json:"number"`
	URL     string `json:"url"`
	Title   string `json:"title,omitempty"`
	IsDraft bool   `json:"isDraft,omitempty"`
}

// PullRequestPlan is what the pull request dialog shows before anything is
// pushed. When Reason is set, nothing can be done and Reason says why.
type PullRequestPlan struct {
	Reason      string           `json:"reason,omitempty"`
	Repository  string           `json:"repository,omitempty"`
	Remote      string           `json:"remote,omitempty"`
	Branch      string           `json:"branch,omitempty"`
	Base        string           `json:"base,omitempty"`
	Commits     []string         `json:"commits"`
	Unpushed    int              `json:"unpushed"`
	Uncommitted int              `json:"uncommitted"`
	Existing    *PullRequestInfo `json:"existing,omitempty"`
	Title       string           `json:"title,omitempty"`
	Body        string           `json:"body,omitempty"`
}

// PullRequestRequest is what the user confirmed in the pull request dialog.
type PullRequestRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Draft bool   `json:"draft"`
}

// PullRequestResult describes what was pushed and opened.
type PullRequestResult struct {
	Number  int    `json:"number"`
	URL     string `json:"url"`
	Created bool   `json:"created"`
	Pushed  int    `json:"pushed"`
}

// pullRequestPlan works out what a pull request would involve. It returns
// the repository root with the plan; when the plan has a Reason, nothing can
// be done and the root may be empty.
func (m *chatManager) pullRequestPlan(ctx context.Context, id string) (PullRequestPlan, string, error) {
	c, err := m.get(id)
	if err != nil {
		return PullRequestPlan{}, "", err
	}
	c.mu.Lock()
	project, title, review := c.state.Project, c.state.Title, c.state.Review != nil
	c.mu.Unlock()
	plan := PullRequestPlan{Commits: []string{}}
	if review {
		plan.Reason = "A review only reads a snapshot. Open pull requests from the original conversation."
		return plan, "", nil
	}
	root, err := repoRoot(ctx, project)
	if err != nil {
		plan.Reason = upperFirst(err.Error()) + "."
		return plan, "", nil
	}
	stop := func(reason string) (PullRequestPlan, string, error) {
		plan.Reason = reason
		return plan, root, nil
	}
	branch, err := currentBranch(ctx, root)
	if err != nil {
		return stop(upperFirst(err.Error()) + ".")
	}
	plan.Branch = branch
	remote, err := gitRun(ctx, root, "config", "--get", "branch."+branch+".remote")
	if err != nil || remote == "" || remote == "." {
		remote = "origin"
	}
	if _, err := gitRun(ctx, root, "remote", "get-url", remote); err != nil {
		return stop("This repository has no remote named " + remote + " to push to.")
	}
	plan.Remote = remote
	view, err := m.gh(ctx, root, "repo", "view", "--json", "nameWithOwner,defaultBranchRef")
	if err != nil {
		return stop("GitHub could not be reached through the GitHub CLI: " + err.Error())
	}
	var repo struct {
		NameWithOwner    string `json:"nameWithOwner"`
		DefaultBranchRef struct {
			Name string `json:"name"`
		} `json:"defaultBranchRef"`
	}
	if json.Unmarshal([]byte(view), &repo) != nil || repo.NameWithOwner == "" || repo.DefaultBranchRef.Name == "" {
		return stop("The GitHub CLI did not report this repository.")
	}
	plan.Repository, plan.Base = repo.NameWithOwner, repo.DefaultBranchRef.Name
	if branch == plan.Base {
		return stop("You are on " + plan.Base + ", the branch pull requests go into. Commit on a new branch first; the commit dialog can create one.")
	}
	if listed, err := m.gh(ctx, root, "pr", "list", "--repo", plan.Repository, "--head", branch, "--state", "open", "--json", "number,url,title,isDraft", "--limit", "1"); err == nil {
		var open []PullRequestInfo
		if json.Unmarshal([]byte(listed), &open) == nil && len(open) > 0 {
			plan.Existing = &open[0]
		}
	}
	// Commits on this branch that are not on the base branch yet.
	baseRef := remote + "/" + plan.Base
	if _, err := gitRun(ctx, root, "rev-parse", "--verify", "--quiet", "refs/remotes/"+baseRef); err == nil {
		if log, err := gitRun(ctx, root, "log", "--reverse", "--format=%s", "--max-count=50", baseRef+"..HEAD"); err == nil && log != "" {
			plan.Commits = strings.Split(log, "\n")
		}
	} else if subject, err := gitRun(ctx, root, "log", "-1", "--format=%s"); err == nil {
		plan.Commits = []string{subject}
	}
	// Commits the remote does not have yet.
	pushedRef := ""
	if upstream, err := gitRun(ctx, root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); err == nil {
		pushedRef = upstream
	} else if _, err := gitRun(ctx, root, "rev-parse", "--verify", "--quiet", "refs/remotes/"+remote+"/"+branch); err == nil {
		pushedRef = remote + "/" + branch
	}
	if pushedRef != "" {
		count, _ := gitRun(ctx, root, "rev-list", "--count", pushedRef+"..HEAD")
		plan.Unpushed, _ = strconv.Atoi(count)
	} else {
		plan.Unpushed = len(plan.Commits)
	}
	if summary := summarizeChanges(ctx, root); summary.Available {
		plan.Uncommitted = len(summary.Files)
	}
	if plan.Existing == nil && len(plan.Commits) == 0 {
		return stop("This branch has no commits that are not on " + plan.Base + " yet.")
	}
	if len(plan.Commits) == 1 {
		plan.Title = plan.Commits[0]
		plan.Body, _ = gitRun(ctx, root, "log", "-1", "--format=%b")
	} else if len(plan.Commits) > 1 {
		plan.Title = strings.TrimSpace(title)
		if plan.Title == "" || plan.Title == "New conversation" {
			plan.Title = plan.Commits[len(plan.Commits)-1]
		}
		lines := []string{}
		for _, subject := range plan.Commits {
			lines = append(lines, "- "+subject)
		}
		plan.Body = strings.Join(lines, "\n")
	}
	return plan, root, nil
}

var pullNumber = regexp.MustCompile(`/pull/(\d+)`)

func (m *chatManager) createPullRequest(ctx context.Context, id string, request PullRequestRequest) (PullRequestResult, error) {
	plan, root, err := m.pullRequestPlan(ctx, id)
	if err != nil {
		return PullRequestResult{}, err
	}
	if plan.Reason != "" {
		return PullRequestResult{}, errors.New(plan.Reason)
	}
	title := strings.TrimSpace(request.Title)
	body := strings.ReplaceAll(request.Body, "\r\n", "\n")
	if plan.Existing == nil && title == "" {
		return PullRequestResult{}, errors.New("write a title for the pull request")
	}
	if len(title) > 256 || len(body) > 60000 || strings.ContainsRune(title+body, 0) {
		return PullRequestResult{}, errors.New("the title or description is too long or contains invalid characters")
	}
	if plan.Existing != nil && plan.Unpushed == 0 {
		return PullRequestResult{Number: plan.Existing.Number, URL: plan.Existing.URL}, nil
	}
	// A plain push: never forced, so nothing on the remote is overwritten.
	if _, err := gitRun(ctx, root, "push", "--set-upstream", plan.Remote, "HEAD:refs/heads/"+plan.Branch); err != nil {
		return PullRequestResult{}, fmt.Errorf("the branch was not pushed: %w", err)
	}
	c, _ := m.get(id)
	if plan.Existing != nil {
		if c != nil {
			c.message(ChatMessage{ID: fmt.Sprintf("push/%d/%d", plan.Existing.Number, time.Now().UnixNano()), Role: "system", Kind: "text", Text: fmt.Sprintf("Pushed %d commits to pull request #%d: %s", plan.Unpushed, plan.Existing.Number, plan.Existing.URL)}, false)
			m.saveOrWarn(c)
		}
		return PullRequestResult{Number: plan.Existing.Number, URL: plan.Existing.URL, Pushed: plan.Unpushed}, nil
	}
	args := []string{"pr", "create", "--repo", plan.Repository, "--head", plan.Branch, "--base", plan.Base, "--title", title, "--body", body}
	if request.Draft {
		args = append(args, "--draft")
	}
	out, err := m.gh(ctx, root, args...)
	if err != nil {
		return PullRequestResult{}, fmt.Errorf("the branch was pushed, but the pull request was not opened: %w", err)
	}
	link := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "https://") {
			link = strings.TrimSpace(line)
		}
	}
	number := 0
	if match := pullNumber.FindStringSubmatch(link); match != nil {
		number, _ = strconv.Atoi(match[1])
	}
	if c != nil {
		c.message(ChatMessage{ID: fmt.Sprintf("pr/%d", number), Role: "system", Kind: "text", Text: fmt.Sprintf("Opened pull request #%d into %s: %s", number, plan.Base, link)}, false)
		m.saveOrWarn(c)
	}
	return PullRequestResult{Number: number, URL: link, Created: true, Pushed: plan.Unpushed}, nil
}

func upperFirst(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

// CommitPlan describes the commit the dialog would make: branch and files.
func (a *App) CommitPlan(id string) (CommitPlan, error) {
	if err := a.chatReady(); err != nil {
		return CommitPlan{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	return a.chats.commitPlan(ctx, id)
}

// Commit makes a local commit of the chosen files. Nothing is pushed.
func (a *App) Commit(id string, request CommitRequest) (CommitResult, error) {
	if err := a.chatReady(); err != nil {
		return CommitResult{}, err
	}
	// Hooks such as linters can take a while.
	ctx, cancel := context.WithTimeout(a.ctx, 3*time.Minute)
	defer cancel()
	return a.chats.commit(ctx, id, request)
}

// PullRequestPlan describes what pushing and opening a pull request would do.
func (a *App) PullRequestPlan(id string) (PullRequestPlan, error) {
	if err := a.chatReady(); err != nil {
		return PullRequestPlan{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	plan, _, err := a.chats.pullRequestPlan(ctx, id)
	return plan, err
}

// CreatePullRequest pushes the branch and opens a pull request, or pushes to
// the one that is already open for the branch.
func (a *App) CreatePullRequest(id string, request PullRequestRequest) (PullRequestResult, error) {
	if err := a.chatReady(); err != nil {
		return PullRequestResult{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 3*time.Minute)
	defer cancel()
	return a.chats.createPullRequest(ctx, id, request)
}

// OpenURL opens a web page, such as a pull request, in the default browser.
// Only https addresses without embedded credentials are opened.
func (a *App) OpenURL(link string) error {
	if a.ctx == nil {
		return errors.New("the app is still starting")
	}
	parsed, err := url.Parse(link)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return errors.New("only https links can be opened")
	}
	wailsruntime.BrowserOpenURL(a.ctx, parsed.String())
	return nil
}
