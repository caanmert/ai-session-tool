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
//	#bug           carries your tag
//	is:live        agent running now
//	is:pinned      pinned by you
//	is:archived    archived by you (archived sessions are hidden otherwise)
type query struct {
	words      []string
	tool       string
	project    string
	branch     string
	tags       []string
	liveOnly   bool
	pinnedOnly bool
	archived   bool
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
				switch val {
				case "live", "running":
					q.liveOnly = true
					continue
				case "pinned":
					q.pinnedOnly = true
					continue
				case "archived":
					q.archived = true
					continue
				}
			}
		}
		if tag, ok := strings.CutPrefix(f, "#"); ok && tag != "" {
			q.tags = append(q.tags, tag)
			continue
		}
		q.words = append(q.words, f)
	}
	return q
}

func (q query) empty() bool {
	return len(q.words) == 0 && len(q.tags) == 0 && q.tool == "" && q.project == "" && q.branch == "" &&
		!q.liveOnly && !q.pinnedOnly && !q.archived
}

// text is the free words, for a conversation search.
func (q query) text() string { return strings.Join(q.words, " ") }

func (q query) match(s model.Session) bool { return q.matchFields(s) && q.matchWords(s) }

// matchFields checks the t:, p:, b: and is: parts.
func (q query) matchFields(s model.Session) bool {
	if q.tool != "" && !strings.HasPrefix(string(s.Tool), q.tool) {
		return false
	}
	if q.project != "" && !strings.Contains(strings.ToLower(s.CWD), q.project) {
		return false
	}
	if q.branch != "" && !strings.Contains(strings.ToLower(s.GitBranch), q.branch) {
		return false
	}
	if q.liveOnly && s.Live == nil || q.pinnedOnly && !s.Pinned || q.archived != s.Archived {
		return false
	}
	for _, want := range q.tags {
		found := false
		for _, tag := range s.Tags {
			if strings.HasPrefix(tag, want) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// matchWords checks the free words against the session's metadata.
func (q query) matchWords(s model.Session) bool {
	if len(q.words) == 0 {
		return true
	}
	hay := strings.ToLower(strings.Join([]string{
		s.Title, s.OriginalTitle, s.FirstPrompt, s.LastPrompt, s.CWD, s.GitBranch, s.ID, s.Model, string(s.Tool),
		strings.Join(s.Tags, " "),
	}, "\n"))
	for _, w := range q.words {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}
