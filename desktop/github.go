package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The issues and pull requests page reads GitHub through the GitHub CLI the
// user is signed in to. It never changes anything on GitHub.

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

func (m *chatManager) githubOverview(ctx context.Context, project string) (GitHubOverview, error) {
	overview := GitHubOverview{Issues: []GitHubIssue{}, Pulls: []GitHubPull{}}
	dir, err := projectDirectory(project)
	if err != nil {
		return overview, err
	}
	if _, err := gitRun(ctx, dir, "rev-parse", "--show-toplevel"); err != nil {
		overview.Reason = "This project is not a Git repository, so it has no GitHub issues or pull requests."
		return overview, nil
	}
	view, err := m.gh(ctx, dir, "repo", "view", "--json", "nameWithOwner,url")
	if err != nil {
		overview.Reason = "GitHub could not be read through the GitHub CLI: " + err.Error()
		return overview, nil
	}
	var repo struct {
		NameWithOwner string `json:"nameWithOwner"`
		URL           string `json:"url"`
	}
	if json.Unmarshal([]byte(view), &repo) != nil || repo.NameWithOwner == "" {
		overview.Reason = "The GitHub CLI did not report a repository for this project."
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
