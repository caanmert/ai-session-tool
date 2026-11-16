package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The issues and pull requests page reads GitHub through the GitHub CLI the
// user is signed in to. The only change it makes is creating an issue, and
// only when the user confirms one in the New issue dialog.

const githubListLimit = 50

// GitHubIssue is an open issue as the page lists it.
type GitHubIssue struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	URL       string   `json:"url"`
	Author    string   `json:"author"`
	Labels    []string `json:"labels"`
	UpdatedAt string   `json:"updatedAt"`
}

// GitHubPull is an open pull request as the page lists it.
type GitHubPull struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Author    string `json:"author"`
	Branch    string `json:"branch"`
	IsDraft   bool   `json:"isDraft"`
	UpdatedAt string `json:"updatedAt"`
	// Review is APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED or empty.
	Review string `json:"review"`
	// Checks summarizes the checks: passing, failing, running or empty.
	Checks string `json:"checks"`
}

// GitHubOverview is the page for one project. When Reason is set, nothing
// could be read and Reason says why.
type GitHubOverview struct {
	Reason     string        `json:"reason,omitempty"`
	Repository string        `json:"repository,omitempty"`
	URL        string        `json:"url,omitempty"`
	Issues     []GitHubIssue `json:"issues"`
	Pulls      []GitHubPull  `json:"pulls"`
	// Partial is set when one of the two lists could not be read.
	Partial string `json:"partial,omitempty"`
}

// GitHubIssueDetail is an issue with its description, for starting a task.
type GitHubIssueDetail struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	URL    string `json:"url"`
}

type ghAuthor struct {
	Login string `json:"login"`
}

// checkSummary reduces a pull request's checks to one word. A failure wins
// over anything still running; nothing reported means no word.
func checkSummary(raw json.RawMessage) string {
	var checks []struct {
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		State      string `json:"state"`
	}
	if json.Unmarshal(raw, &checks) != nil || len(checks) == 0 {
		return ""
	}
	failing, running := false, false
	for _, check := range checks {
		result := strings.ToUpper(check.Conclusion)
		if result == "" {
			result = strings.ToUpper(check.State)
		}
		switch result {
		case "FAILURE", "ERROR", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE":
			failing = true
		case "SUCCESS", "NEUTRAL", "SKIPPED":
		default:
			// Queued, in progress, expected, or still pending.
			running = true
		}
	}
	switch {
	case failing:
		return "failing"
	case running:
		return "running"
	}
	return "passing"
}

func projectDirectory(project string) (string, error) {
	if !filepath.IsAbs(project) {
		return "", errors.New("choose a project")
	}
	info, err := os.Stat(project)
	if err != nil || !info.IsDir() {
		return "", errors.New("the project folder is not available")
	}
	return filepath.Clean(project), nil
}

type githubRepo struct {
	NameWithOwner string `json:"nameWithOwner"`
	URL           string `json:"url"`
}

// githubRepository finds the GitHub repository of a project folder. When it
// can't, the reason says why in words for the page.
func (m *chatManager) githubRepository(ctx context.Context, dir string) (githubRepo, string) {
	if _, err := gitRun(ctx, dir, "rev-parse", "--show-toplevel"); err != nil {
		return githubRepo{}, "This project is not a Git repository, so it has no GitHub issues or pull requests."
	}
	view, err := m.gh(ctx, dir, "repo", "view", "--json", "nameWithOwner,url")
	if err != nil {
		return githubRepo{}, "GitHub could not be read through the GitHub CLI: " + err.Error()
	}
	var repo githubRepo
	if json.Unmarshal([]byte(view), &repo) != nil || repo.NameWithOwner == "" {
		return githubRepo{}, "The GitHub CLI did not report a repository for this project."
	}
	return repo, ""
}

func (m *chatManager) githubOverview(ctx context.Context, project string) (GitHubOverview, error) {
	overview := GitHubOverview{Issues: []GitHubIssue{}, Pulls: []GitHubPull{}}
	dir, err := projectDirectory(project)
	if err != nil {
		return overview, err
	}
	repo, reason := m.githubRepository(ctx, dir)
	if reason != "" {
		overview.Reason = reason
		return overview, nil
	}
	overview.Repository, overview.URL = repo.NameWithOwner, repo.URL
	limit := strconv.Itoa(githubListLimit)
	problems := []string{}
	issues, err := m.gh(ctx, dir, "issue", "list", "--repo", repo.NameWithOwner, "--state", "open", "--limit", limit, "--json", "number,title,url,author,labels,updatedAt")
	if err == nil {
		var listed []struct {
			Number    int      `json:"number"`
			Title     string   `json:"title"`
			URL       string   `json:"url"`
			Author    ghAuthor `json:"author"`
			UpdatedAt string   `json:"updatedAt"`
			Labels    []struct {
				Name string `json:"name"`
			} `json:"labels"`
		}
		if err = json.Unmarshal([]byte(issues), &listed); err == nil {
			for _, item := range listed {
				labels := []string{}
				for _, label := range item.Labels {
					labels = append(labels, label.Name)
				}
				overview.Issues = append(overview.Issues, GitHubIssue{Number: item.Number, Title: item.Title, URL: item.URL, Author: item.Author.Login, Labels: labels, UpdatedAt: item.UpdatedAt})
			}
		}
	}
	if err != nil {
		problems = append(problems, "issues: "+err.Error())
	}
	pulls, err := m.gh(ctx, dir, "pr", "list", "--repo", repo.NameWithOwner, "--state", "open", "--limit", limit, "--json", "number,title,url,author,headRefName,isDraft,reviewDecision,statusCheckRollup,updatedAt")
	if err == nil {
		var listed []struct {
			Number         int             `json:"number"`
			Title          string          `json:"title"`
			URL            string          `json:"url"`
			Author         ghAuthor        `json:"author"`
			HeadRefName    string          `json:"headRefName"`
			IsDraft        bool            `json:"isDraft"`
			ReviewDecision string          `json:"reviewDecision"`
			Checks         json.RawMessage `json:"statusCheckRollup"`
			UpdatedAt      string          `json:"updatedAt"`
		}
		if err = json.Unmarshal([]byte(pulls), &listed); err == nil {
			for _, item := range listed {
				overview.Pulls = append(overview.Pulls, GitHubPull{Number: item.Number, Title: item.Title, URL: item.URL, Author: item.Author.Login, Branch: item.HeadRefName, IsDraft: item.IsDraft, UpdatedAt: item.UpdatedAt, Review: item.ReviewDecision, Checks: checkSummary(item.Checks)})
			}
		}
	}
	if err != nil {
		problems = append(problems, "pull requests: "+err.Error())
	}
	if len(problems) == 2 {
		overview.Reason = "GitHub could not be read through the GitHub CLI. " + strings.Join(problems, " ")
	} else if len(problems) == 1 {
		overview.Partial = upperFirst(problems[0])
	}
	return overview, nil
}

func (m *chatManager) githubIssue(ctx context.Context, project string, number int) (GitHubIssueDetail, error) {
	dir, err := projectDirectory(project)
	if err != nil {
		return GitHubIssueDetail{}, err
	}
	if number <= 0 {
		return GitHubIssueDetail{}, errors.New("choose an issue")
	}
	out, err := m.gh(ctx, dir, "issue", "view", strconv.Itoa(number), "--json", "number,title,body,url")
	if err != nil {
		return GitHubIssueDetail{}, errors.New("the issue could not be read: " + err.Error())
	}
	var detail GitHubIssueDetail
	if json.Unmarshal([]byte(out), &detail) != nil || detail.Number != number {
		return GitHubIssueDetail{}, errors.New("the GitHub CLI returned something unexpected for this issue")
	}
	detail.Body = shorten(detail.Body, 20000)
	return detail, nil
}

// GitHubOverview lists a project's open issues and pull requests. It only
// reads from GitHub.
func (a *App) GitHubOverview(project string) (GitHubOverview, error) {
	if err := a.chatReady(); err != nil {
		return GitHubOverview{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.chats.githubOverview(ctx, project)
}

// GitHubIssue reads one issue's title and description, for starting a task.
func (a *App) GitHubIssue(project string, number int) (GitHubIssueDetail, error) {
	if err := a.chatReady(); err != nil {
		return GitHubIssueDetail{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return a.chats.githubIssue(ctx, project, number)
}

// GitHub's own limits for a new issue.
const (
	issueTitleLimit = 256
	issueBodyLimit  = 65536
	labelListLimit  = 200
)

// GitHubLabel is a label a new issue can have.
type GitHubLabel struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// GitHubIssueForm is what the New issue dialog needs: where the issue goes
// and the labels it can have. When Reason is set, no issue can be created
// and Reason says why.
type GitHubIssueForm struct {
	Reason     string        `json:"reason,omitempty"`
	Repository string        `json:"repository,omitempty"`
	URL        string        `json:"url,omitempty"`
	Labels     []GitHubLabel `json:"labels"`
	// Partial is set when the labels could not be read; issues can still be
	// created without labels.
	Partial string `json:"partial,omitempty"`
}

// GitHubIssueCreated is the issue GitHub made.
type GitHubIssueCreated struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
}

func (m *chatManager) githubLabels(ctx context.Context, dir, repo string) ([]GitHubLabel, error) {
	out, err := m.gh(ctx, dir, "label", "list", "--repo", repo, "--limit", strconv.Itoa(labelListLimit), "--json", "name,color,description")
	if err != nil {
		return nil, err
	}
	labels := []GitHubLabel{}
	if err := json.Unmarshal([]byte(out), &labels); err != nil {
		return nil, errors.New("the GitHub CLI returned labels in an unexpected form")
	}
	return labels, nil
}

func (m *chatManager) githubIssueForm(ctx context.Context, project string) (GitHubIssueForm, error) {
	form := GitHubIssueForm{Labels: []GitHubLabel{}}
	dir, err := projectDirectory(project)
	if err != nil {
		return form, err
	}
	repo, reason := m.githubRepository(ctx, dir)
	if reason != "" {
		form.Reason = reason
		return form, nil
	}
	form.Repository, form.URL = repo.NameWithOwner, repo.URL
	labels, err := m.githubLabels(ctx, dir, repo.NameWithOwner)
	if err != nil {
		form.Partial = "Labels could not be read: " + err.Error()
		return form, nil
	}
	form.Labels = labels
	return form, nil
}

// createGitHubIssue creates one issue through the GitHub CLI the user is
// signed in to. Everything is checked before anything is sent; it never
// assigns people, sets a milestone or adds the issue to a project.
func (m *chatManager) createGitHubIssue(ctx context.Context, project, title, body string, labels []string) (GitHubIssueCreated, error) {
	dir, err := projectDirectory(project)
	if err != nil {
		return GitHubIssueCreated{}, err
	}
	title = strings.TrimSpace(title)
	switch {
	case title == "":
		return GitHubIssueCreated{}, errors.New("give the issue a title")
	case strings.ContainsAny(title, "\r\n"):
		return GitHubIssueCreated{}, errors.New("the title must be one line")
	case utf8.RuneCountInString(title) > issueTitleLimit:
		return GitHubIssueCreated{}, fmt.Errorf("the title is longer than %d characters", issueTitleLimit)
	case utf8.RuneCountInString(body) > issueBodyLimit:
		return GitHubIssueCreated{}, fmt.Errorf("the description is longer than GitHub allows (%d characters)", issueBodyLimit)
	}
	repo, reason := m.githubRepository(ctx, dir)
	if reason != "" {
		return GitHubIssueCreated{}, errors.New(reason)
	}
	args := []string{"issue", "create", "--repo", repo.NameWithOwner, "--title=" + title, "--body=" + body}
	if len(labels) > 0 {
		known, err := m.githubLabels(ctx, dir, repo.NameWithOwner)
		if err != nil {
			return GitHubIssueCreated{}, errors.New("labels could not be checked: " + err.Error())
		}
		names := map[string]bool{}
		for _, label := range known {
			names[label.Name] = true
		}
		seen := map[string]bool{}
		for _, label := range labels {
			if !names[label] {
				return GitHubIssueCreated{}, fmt.Errorf("%s has no label named %q", repo.NameWithOwner, label)
			}
			if !seen[label] {
				seen[label] = true
				// The = form keeps a value that starts with "-" a value.
				args = append(args, "--label="+label)
			}
		}
	}
	out, err := m.gh(ctx, dir, args...)
	if err != nil {
		return GitHubIssueCreated{}, errors.New("GitHub did not create the issue: " + err.Error())
	}
	// gh prints the new issue's address.
	prefix := strings.TrimSuffix(repo.URL, "/") + "/issues/"
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if number, err := strconv.Atoi(strings.TrimPrefix(line, prefix)); strings.HasPrefix(line, prefix) && err == nil && number > 0 {
			return GitHubIssueCreated{Number: number, URL: line}, nil
		}
	}
	return GitHubIssueCreated{}, errors.New("the issue may have been created, but the GitHub CLI answered unexpectedly: " + shorten(out, 300))
}

// GitHubIssueForm reads where a new issue would go and the labels it can
// have. It only reads.
func (a *App) GitHubIssueForm(project string) (GitHubIssueForm, error) {
	if err := a.chatReady(); err != nil {
		return GitHubIssueForm{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.chats.githubIssueForm(ctx, project)
}

// CreateGitHubIssue creates an issue in the project's GitHub repository. The
// app calls it only after the user confirms in the New issue dialog.
func (a *App) CreateGitHubIssue(project, title, body string, labels []string) (GitHubIssueCreated, error) {
	if err := a.chatReady(); err != nil {
		return GitHubIssueCreated{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 60*time.Second)
	defer cancel()
	return a.chats.createGitHubIssue(ctx, project, title, body, labels)
}
