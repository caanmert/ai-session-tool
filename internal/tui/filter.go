package tui

import (
	"strings"

	"github.com/caanmert/ai-session-tool/internal/model"
)

// query is a parsed filter. Every part must match (AND):
//
//	words          text in the title, prompts, project, branch, id or model
//	t:codex        tool (also tool:)
//	p:api          project path contains (also project:)
//	b:main         git branch contains (also branch:)
//	is:live        agent running now
type query struct {
	words    []string
	tool     string
	project  string
	branch   string
	liveOnly bool
}

func parseQuery(s string) query {
	var q query
	for _, f := range strings.Fields(strings.ToLower(s)) {
		key, val, ok := strings.Cut(f, ":")
		if ok && val != "" {
			switch key {
			case "t", "tool":
				q.tool = val
				continue
			case "p", "project":
				q.project = val
				continue
			case "b", "branch":
				q.branch = val
				continue
			case "is":
				if val == "live" || val == "running" {
					q.liveOnly = true
					continue
				}
			}
		}
		q.words = append(q.words, f)
	}
	return q
}

func (q query) empty() bool {
	return len(q.words) == 0 && q.tool == "" && q.project == "" && q.branch == "" && !q.liveOnly
}

func (q query) match(s model.Session) bool {
	if q.tool != "" && !strings.HasPrefix(string(s.Tool), q.tool) {
		return false
	}
	if q.project != "" && !strings.Contains(strings.ToLower(s.CWD), q.project) {
		return false
	}
	if q.branch != "" && !strings.Contains(strings.ToLower(s.GitBranch), q.branch) {
		return false
	}
	if q.liveOnly && s.Live == nil {
		return false
	}
	if len(q.words) == 0 {
		return true
	}
	hay := strings.ToLower(strings.Join([]string{
		s.Title, s.FirstPrompt, s.LastPrompt, s.CWD, s.GitBranch, s.ID, s.Model, string(s.Tool),
	}, "\n"))
	for _, w := range q.words {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}
