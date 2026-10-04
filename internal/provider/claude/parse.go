package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/caanmert/ai-session-tool/internal/jsonl"
	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
	"github.com/caanmert/ai-session-tool/internal/textutil"
)

// record is the subset of a transcript line we read. Unknown types and
// fields are ignored so new Claude Code versions don't break parsing.
type record struct {
	Type             string          `json:"type"`
	CWD              string          `json:"cwd"`
	GitBranch        string          `json:"gitBranch"`
	Timestamp        string          `json:"timestamp"`
	Version          string          `json:"version"`
	IsSidechain      bool            `json:"isSidechain"`
	IsMeta           bool            `json:"isMeta"`
	IsCompactSummary bool            `json:"isCompactSummary"`
	Summary          string          `json:"summary"`     // type "summary"
	CustomTitle      string          `json:"customTitle"` // type "custom-title"
	Message          json.RawMessage `json:"message"`     // types "user", "assistant"
}

type apiMessage struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Usage   *apiUsage       `json:"usage"`
	Content json.RawMessage `json:"content"`
}

type apiUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

func (u apiUsage) toModel() model.Usage {
	return model.Usage{
		Input:      u.InputTokens,
		Output:     u.OutputTokens,
		CacheRead:  u.CacheReadInputTokens,
		CacheWrite: u.CacheCreationInputTokens,
	}
}

type block struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Thinking string          `json:"thinking"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	Content  json.RawMessage `json:"content"` // tool_result: string or []block
}

// content decodes a message content field, which is either a plain string
// or an array of blocks.
func content(raw json.RawMessage) []block {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return nil
		}
		return []block{{Type: "text", Text: s}}
	}
	var blocks []block
	if json.Unmarshal(raw, &blocks) != nil {
		return nil
	}
	return blocks
}

const (
	maxPromptRunes = 500
	maxTitleRunes  = 200
)

// Parse summarises a transcript.
func (p *Provider) Parse(ctx context.Context, f provider.FileRef) (model.Session, []provider.Warning, error) {
	s := model.Session{
		Tool: model.ToolClaude,
		ID:   strings.TrimSuffix(filepath.Base(f.Path), ".jsonl"),
		Path: f.Path,
	}
	var (
		warns        []provider.Warning
		customTitle  string
		firstRaw     string // first prompt with line breaks, for the title
		firstCommand string
		usageByID    = map[string]model.Usage{}
		idOrder      []string
		anonUsage    model.Usage
		assistantIDs = map[string]bool{}
	)

	err := jsonl.EachFile(f.Path, func(n int, line []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var r record
		if err := json.Unmarshal(line, &r); err != nil {
			warns = append(warns, provider.Warning{Path: f.Path, Line: n, Msg: "malformed JSON: " + err.Error()})
			return nil
		}

		if ts, ok := textutil.ParseTime(r.Timestamp); ok {
			if s.StartedAt.IsZero() || ts.Before(s.StartedAt) {
				s.StartedAt = ts
			}
			if ts.After(s.UpdatedAt) {
				s.UpdatedAt = ts
			}
		}

		switch r.Type {
		case "summary":
			if r.Summary != "" {
				s.Summary = r.Summary
			}
			return nil
		case "custom-title":
			if r.CustomTitle != "" {
				customTitle = r.CustomTitle
			}
			return nil
		case "user", "assistant":
		default:
			return nil
		}

		if s.CWD == "" && r.CWD != "" {
			s.CWD = r.CWD
		}
		if r.GitBranch != "" {
			s.GitBranch = r.GitBranch
		}
		if r.Version != "" {
			s.Version = r.Version
		}

		var m apiMessage
		if len(r.Message) > 0 && json.Unmarshal(r.Message, &m) != nil {
			warns = append(warns, provider.Warning{Path: f.Path, Line: n, Msg: "unreadable message field"})
			return nil
		}

		if r.Type == "assistant" {
			// One API response is written as several records (one per content
			// block) that repeat the same usage; count each response once.
			if m.Usage != nil {
				if m.ID == "" {
					anonUsage = anonUsage.Add(m.Usage.toModel())
				} else {
					if _, seen := usageByID[m.ID]; !seen {
						idOrder = append(idOrder, m.ID)
					}
					usageByID[m.ID] = m.Usage.toModel()
				}
			}
			if r.IsSidechain {
				return nil
			}
			if m.Model != "" && m.Model != "<synthetic>" {
				s.Model = m.Model
			}
			key := m.ID
			if key == "" {
				key = fmt.Sprintf("line:%d", n)
			}
			if !assistantIDs[key] {
				assistantIDs[key] = true
				s.AssistantTurns++
			}
			return nil
		}

		// User record.
		if r.IsSidechain || r.IsMeta || r.IsCompactSummary {
			return nil
		}
		text, kind := promptText(content(m.Content))
		switch kind {
		case promptReal:
			s.UserTurns++
			if firstRaw == "" {
				firstRaw = text
				s.FirstPrompt = textutil.Truncate(textutil.Collapse(text), maxPromptRunes)
			}
			s.LastPrompt = textutil.Truncate(textutil.Collapse(text), maxPromptRunes)
		case promptCommand:
			if firstCommand == "" {
				firstCommand = text
			}
		}
		return nil
	})
	if err != nil {
		return model.Session{}, warns, err
	}

	if s.UserTurns == 0 && s.AssistantTurns == 0 {
		return model.Session{}, warns, provider.ErrEmpty
	}

	for _, id := range idOrder {
		s.Usage = s.Usage.Add(usageByID[id])
	}
	s.Usage = s.Usage.Add(anonUsage)

	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = f.ModTime.UTC()
	}
	if s.StartedAt.IsZero() {
		s.StartedAt = s.UpdatedAt
	}
	s.Title = textutil.FirstNonEmpty(
		textutil.FirstLine(customTitle),
		textutil.FirstLine(s.Summary),
		textutil.FirstLine(firstRaw),
		textutil.FirstLine(firstCommand),
		"(untitled)",
	)
	s.Title = textutil.Truncate(s.Title, maxTitleRunes)
	return s, warns, nil
}

type promptKind int

const (
	promptNone    promptKind = iota // tool results, interrupts, local command output
	promptReal                      // something the user typed
	promptCommand                   // a slash command, e.g. "/review src"
)

var (
	commandNameRe = regexp.MustCompile(`(?s)<command-name>(.*?)</command-name>`)
	commandArgsRe = regexp.MustCompile(`(?s)<command-args>(.*?)</command-args>`)
)

// noisePrefixes mark user records that Claude Code writes on the user's
// behalf (command output, caveats, interrupts) rather than typed prompts.
var noisePrefixes = []string{
	"<local-command-stdout>",
	"<local-command-stderr>",
	"<local-command-caveat>",
	"<bash-stdout>",
	"<bash-stderr>",
	"<system-reminder>",
	"<user-prompt-submit-hook>",
	"[Request interrupted by user",
	"Caveat: The messages below were generated",
}

// promptText extracts what the user typed from a user record's content.
func promptText(blocks []block) (string, promptKind) {
	var parts []string
	images := 0
	for _, b := range blocks {
		switch b.Type {
		case "text":
			parts = append(parts, b.Text)
		case "image":
			images++
		case "tool_result":
			return "", promptNone
		}
	}
	text := strings.TrimSpace(strings.Join(parts, "\n"))
	if text == "" {
		if images > 0 {
			return "[image]", promptReal
		}
		return "", promptNone
	}
	if m := commandNameRe.FindStringSubmatch(text); m != nil {
		cmd := strings.TrimSpace(m[1])
		if a := commandArgsRe.FindStringSubmatch(text); a != nil && strings.TrimSpace(a[1]) != "" {
			cmd += " " + strings.TrimSpace(a[1])
		}
		return cmd, promptCommand
	}
	for _, p := range noisePrefixes {
		if strings.HasPrefix(text, p) {
			return "", promptNone
		}
	}
	if strings.HasPrefix(text, "<bash-input>") {
		return "!" + strings.TrimSuffix(strings.TrimPrefix(text, "<bash-input>"), "</bash-input>"), promptCommand
	}
	return text, promptReal
}

// Transcript returns the main-chain conversation of s in file order.
func (p *Provider) Transcript(ctx context.Context, s model.Session) ([]model.Message, error) {
	var out []model.Message
	err := jsonl.EachFile(s.Path, func(_ int, line []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var r record
		if json.Unmarshal(line, &r) != nil || (r.Type != "user" && r.Type != "assistant") {
			return nil
		}
		if r.IsSidechain || r.IsMeta || r.IsCompactSummary {
			return nil
		}
		var m apiMessage
		if json.Unmarshal(r.Message, &m) != nil {
			return nil
		}
		ts, _ := textutil.ParseTime(r.Timestamp)
		blocks := content(m.Content)

		if r.Type == "user" {
			if text, kind := promptText(blocks); kind != promptNone {
				out = append(out, model.Message{Role: model.RoleUser, Kind: model.KindText, Text: text, Time: ts})
				return nil
			}
			for _, b := range blocks {
				if b.Type == "tool_result" {
					out = append(out, model.Message{Role: model.RoleUser, Kind: model.KindToolResult, Text: toolResultText(b.Content), Time: ts})
				}
			}
			return nil
		}

		for _, b := range blocks {
			msg := model.Message{Role: model.RoleAssistant, Time: ts}
			switch b.Type {
			case "text":
				if strings.TrimSpace(b.Text) == "" {
					continue
				}
				msg.Kind, msg.Text = model.KindText, b.Text
			case "thinking":
				msg.Kind, msg.Text = model.KindThinking, b.Thinking
			case "tool_use":
				msg.Kind, msg.ToolName, msg.Text = model.KindToolUse, b.Name, toolInputSummary(b.Input)
			default:
				continue
			}
			out = append(out, msg)
		}
		return nil
	})
	return out, err
}

// toolInputSummary renders a tool call's input as one short line.
func toolInputSummary(input json.RawMessage) string {
	var in map[string]any
	if json.Unmarshal(input, &in) != nil {
		return ""
	}
	for _, key := range []string{"command", "file_path", "path", "pattern", "url", "query", "description", "prompt"} {
		if v, ok := in[key].(string); ok && v != "" {
			return textutil.Truncate(textutil.Collapse(v), 160)
		}
	}
	compact, _ := json.Marshal(in)
	return textutil.Truncate(string(compact), 160)
}

func toolResultText(raw json.RawMessage) string {
	var parts []string
	for _, b := range content(raw) {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}
