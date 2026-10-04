package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/caanmert/ai-session-tool/internal/jsonl"
	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
	"github.com/caanmert/ai-session-tool/internal/textutil"
)

// line is the envelope of every rollout record.
type line struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type sessionMeta struct {
	ID           string          `json:"id"`
	CWD          string          `json:"cwd"`
	CLIVersion   string          `json:"cli_version"`
	Source       json.RawMessage `json:"source"`
	ThreadSource string          `json:"thread_source"`
	ParentThread string          `json:"parent_thread_id"`
	HistoryBase  *historyBase    `json:"history_base"`
	Git          *struct {
		Branch string `json:"branch"`
	} `json:"git"`
}

// historyBase points at an immutable prefix of this thread's history in
// another rollout file. Despite its name, thread_id holds a rollout id.
type historyBase struct {
	RolloutID     string `json:"thread_id"`
	EndByteOffset int64  `json:"end_byte_offset"`
}

type tokenUsage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	CacheWriteInputTokens int64 `json:"cache_write_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
}

// toModel converts OpenAI-style usage, where input_tokens already includes
// cached reads and cache writes, into disjoint buckets.
func (u tokenUsage) toModel() model.Usage {
	return model.Usage{
		Input:      max(u.InputTokens-u.CachedInputTokens-u.CacheWriteInputTokens, 0),
		Output:     u.OutputTokens,
		CacheRead:  u.CachedInputTokens,
		CacheWrite: u.CacheWriteInputTokens,
	}
}

type textPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// userMessageBegin marks where the typed request starts in older prompts
// that were prefixed with context (codex-rs protocol USER_MESSAGE_BEGIN).
const userMessageBegin = "## My request for Codex:"

// injectedRe matches context Codex injects as user-role messages
// (<environment_context>, <user_instructions>, AGENTS.md, ...).
var injectedRe = regexp.MustCompile(`^(<[a-z_]+[ >]|# AGENTS\.md instructions)`)

func cleanPrompt(s string) string {
	if i := strings.Index(s, userMessageBegin); i >= 0 {
		s = s[i+len(userMessageBegin):]
	}
	return strings.TrimSpace(s)
}

// Where a message came from. Codex records the same conversation in more
// than one form; the scan picks one source per role to avoid duplicates.
type source int

const (
	srcOther     source = iota
	srcUserItem         // event_msg item_completed UserMessage (paginated rollouts)
	srcUserEvent        // event_msg user_message (legacy rollouts)
	srcUserResp         // response_item message role=user (fallback, filtered)
	srcAsstResp         // response_item message role=assistant
	srcAsstItem         // event_msg item_completed AgentMessage
	srcAsstEvent        // event_msg agent_message
	numSources
)

type entry struct {
	src source
	msg model.Message
}

// scan is everything read from one thread's rollout chain.
type scan struct {
	meta       *sessionMeta // the active rollout's own session_meta
	entries    []entry
	counts     [numSources]int
	started    time.Time
	updated    time.Time
	model      string
	lastCWD    string
	usage      cumulative // from token_usage_record (thread totals)
	usageEvent cumulative // from token_count events (thread totals)
	warns      []provider.Warning
}

func (sc *scan) add(src source, m model.Message) {
	sc.counts[src]++
	sc.entries = append(sc.entries, entry{src, m})
}

// userSource and asstSource choose one representation per role.
func (sc *scan) userSource() source {
	for _, s := range []source{srcUserItem, srcUserEvent, srcUserResp} {
		if sc.counts[s] > 0 {
			return s
		}
	}
	return srcUserItem
}

func (sc *scan) asstSource() source {
	for _, s := range []source{srcAsstResp, srcAsstItem, srcAsstEvent} {
		if sc.counts[s] > 0 {
			return s
		}
	}
	return srcAsstResp
}

// messages returns the transcript with duplicate representations removed.
func (sc *scan) messages() []model.Message {
	us, as := sc.userSource(), sc.asstSource()
	var out []model.Message
	for _, e := range sc.entries {
		switch e.src {
		case srcOther, us, as:
			out = append(out, e.msg)
		}
	}
	return out
}

// segment is one file of a thread's history: the active rollout, preceded
// by any history_base prefixes (read only up to limit bytes).
type segment struct {
	path  string
	limit int64 // < 0: whole file
}

// readMeta returns the first session_meta of a rollout, which is the one
// that belongs to it (later ones can be copied from fork history).
func readMeta(path string) (*sessionMeta, error) {
	var meta *sessionMeta
	errFound := errors.New("found")
	n := 0
	err := jsonl.EachFile(path, func(_ int, raw []byte) error {
		if n++; n > 50 {
			return errFound
		}
		var l line
		if json.Unmarshal(raw, &l) != nil || l.Type != "session_meta" {
			return nil
		}
		var m sessionMeta
		if json.Unmarshal(l.Payload, &m) == nil {
			meta = &m
		}
		return errFound
	})
	if err != nil && !errors.Is(err, errFound) {
		return nil, err
	}
	return meta, nil
}

func hidden(m *sessionMeta) bool {
	if m.ParentThread != "" {
		return true
	}
	switch m.ThreadSource {
	case "subagent", "guardian_review", "memory_consolidation":
		return true
	}
	// source is "cli", "vscode", "exec", ... or {"subagent": ...},
	// {"internal": ...}, {"custom": "..."}.
	var obj map[string]json.RawMessage
	if json.Unmarshal(m.Source, &obj) == nil {
		_, sub := obj["subagent"]
		_, internal := obj["internal"]
		return sub || internal
	}
	return false
}

// chain resolves the history_base prefixes of a rollout, oldest first.
func (p *Provider) chain(ctx context.Context, path string, meta *sessionMeta) ([]segment, []provider.Warning) {
	segs := []segment{{path: path, limit: -1}}
	if meta == nil || meta.HistoryBase == nil {
		return segs, nil
	}
	rollouts, _ := p.index(ctx)
	seen := map[string]bool{path: true}
	var warns []provider.Warning
	for hb := meta.HistoryBase; hb != nil && hb.RolloutID != ""; {
		base, ok := rollouts[hb.RolloutID]
		if !ok || seen[base] {
			warns = append(warns, provider.Warning{Path: path, Msg: "history base rollout " + hb.RolloutID + " not found; showing later history only"})
			break
		}
		seen[base] = true
		segs = append([]segment{{path: base, limit: hb.EndByteOffset}}, segs...)
		bm, err := readMeta(base)
		if err != nil || bm == nil {
			break
		}
		hb = bm.HistoryBase
	}
	return segs, warns
}

// read scans the whole history of the rollout at path.
func (p *Provider) read(ctx context.Context, path string) (*scan, error) {
	meta, err := readMeta(path)
	if err != nil {
		return nil, err
	}
	sc := &scan{meta: meta}
	if meta != nil && hidden(meta) {
		return sc, nil // Parse drops it; skip reading a possibly long history
	}
	segs, warns := p.chain(ctx, path, meta)
	sc.warns = warns
	for _, seg := range segs {
		if seg.limit == 0 {
			continue
		}
		if err := sc.readSegment(ctx, seg); err != nil {
			return nil, err
		}
	}
	return sc, nil
}

func (sc *scan) readSegment(ctx context.Context, seg segment) error {
	rc, err := jsonl.Open(seg.path)
	if err != nil {
		return err
	}
	defer rc.Close()
	var r io.Reader = rc
	if seg.limit > 0 {
		r = io.LimitReader(rc, seg.limit)
	}
	return jsonl.Each(r, func(n int, raw []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var l line
		if err := json.Unmarshal(raw, &l); err != nil {
			sc.warns = append(sc.warns, provider.Warning{Path: seg.path, Line: n, Msg: "malformed JSON: " + err.Error()})
			return nil
		}
		ts, hasTime := textutil.ParseTime(l.Timestamp)
		if hasTime {
			if sc.started.IsZero() || ts.Before(sc.started) {
				sc.started = ts
			}
			if ts.After(sc.updated) {
				sc.updated = ts
			}
		}
		if err := sc.record(l, ts); err != nil {
			sc.warns = append(sc.warns, provider.Warning{Path: seg.path, Line: n, Msg: fmt.Sprintf("unreadable %s: %v", l.Type, err)})
		}
		return nil
	})
}

// record handles one rollout line. Unknown types are ignored.
func (sc *scan) record(l line, ts time.Time) error {
	switch l.Type {
	case "turn_context":
		var tc struct {
			CWD   string `json:"cwd"`
			Model string `json:"model"`
		}
		if err := json.Unmarshal(l.Payload, &tc); err != nil {
			return err
		}
		if tc.Model != "" {
			sc.model = tc.Model
		}
		if tc.CWD != "" {
			sc.lastCWD = tc.CWD
		}
	case "token_usage_record":
		var rec struct {
			Thread *tokenUsage `json:"thread_token_usage"`
		}
		if err := json.Unmarshal(l.Payload, &rec); err != nil {
			return err
		}
		if rec.Thread != nil {
			sc.usage.observe(rec.Thread.toModel(), sc.at(ts), sc.model)
		}
	case "event_msg":
		return sc.event(l.Payload, ts)
	case "response_item":
		return sc.responseItem(l.Payload, ts)
	}
	return nil
}

func (sc *scan) event(payload json.RawMessage, ts time.Time) error {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &head); err != nil {
		return err
	}
	switch head.Type {
	case "user_message":
		var ev struct {
			Message     string   `json:"message"`
			Images      []string `json:"images"`
			LocalImages []string `json:"local_images"`
		}
		if err := json.Unmarshal(payload, &ev); err != nil {
			return err
		}
		text := cleanPrompt(ev.Message)
		if text == "" && len(ev.Images)+len(ev.LocalImages) > 0 {
			text = "[image]"
		}
		if text != "" {
			sc.add(srcUserEvent, model.Message{Role: model.RoleUser, Kind: model.KindText, Text: text, Time: ts})
		}
	case "agent_message":
		var ev struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(payload, &ev); err != nil {
			return err
		}
		if strings.TrimSpace(ev.Message) != "" {
			sc.add(srcAsstEvent, model.Message{Role: model.RoleAssistant, Kind: model.KindText, Text: ev.Message, Time: ts})
		}
	case "item_completed":
		var ev struct {
			Item struct {
				Type    string     `json:"type"`
				Content []textPart `json:"content"`
			} `json:"item"`
		}
		if err := json.Unmarshal(payload, &ev); err != nil {
			return err
		}
		var texts []string
		images := 0
		for _, c := range ev.Item.Content {
			switch strings.ToLower(c.Type) {
			case "text":
				texts = append(texts, c.Text)
			case "image", "local_image":
				images++
			}
		}
		text := strings.Join(texts, "\n")
		switch ev.Item.Type {
		case "UserMessage":
			text = cleanPrompt(text)
			if text == "" && images > 0 {
				text = "[image]"
			}
			if text != "" {
				sc.add(srcUserItem, model.Message{Role: model.RoleUser, Kind: model.KindText, Text: text, Time: ts})
			}
		case "AgentMessage":
			if strings.TrimSpace(text) != "" {
				sc.add(srcAsstItem, model.Message{Role: model.RoleAssistant, Kind: model.KindText, Text: text, Time: ts})
			}
		}
	case "token_count":
		var ev struct {
			Info *struct {
				Total *tokenUsage `json:"total_token_usage"`
			} `json:"info"`
			tokenUsage // very old rollouts put the counts on the event itself
		}
		if err := json.Unmarshal(payload, &ev); err != nil {
			return err
		}
		var u model.Usage
		switch {
		case ev.Info != nil && ev.Info.Total != nil:
			u = ev.Info.Total.toModel()
		case ev.InputTokens > 0 || ev.OutputTokens > 0:
			u = ev.toModel()
		default:
			return nil
		}
		sc.usageEvent.observe(u, sc.at(ts), sc.model)
	}
	return nil
}

// at is when a usage record happened: its own timestamp, else the latest
// one seen so far.
func (sc *scan) at(ts time.Time) time.Time {
	if ts.IsZero() {
		return sc.updated
	}
	return ts
}

// cumulative turns a series of running thread totals into per-slot,
// per-model usage: each step's increase is attributed to the model in use
// when it was recorded.
type cumulative struct {
	seen    bool
	prev    model.Usage
	total   model.Usage
	entries []model.UsageEntry
}

func (c *cumulative) observe(cur model.Usage, at time.Time, modelName string) {
	c.seen = true
	d := model.Usage{
		Input:      cur.Input - c.prev.Input,
		Output:     cur.Output - c.prev.Output,
		CacheRead:  cur.CacheRead - c.prev.CacheRead,
		CacheWrite: cur.CacheWrite - c.prev.CacheWrite,
	}
	c.prev = cur
	if d.Input < 0 || d.Output < 0 || d.CacheRead < 0 || d.CacheWrite < 0 {
		return // the totals restarted: count from the new baseline
	}
	c.total = c.total.Add(d)
	c.entries = model.AddUsage(c.entries, model.SlotOf(at), modelName, d)
}

func (sc *scan) responseItem(payload json.RawMessage, ts time.Time) error {
	var head struct {
		Type string `json:"type"`
		Role string `json:"role"`
	}
	if err := json.Unmarshal(payload, &head); err != nil {
		return err
	}
	// Fields differ in shape between item types, so each known type is
	// decoded into its own struct.
	tool := func(name, text string) {
		sc.add(srcOther, model.Message{Role: model.RoleAssistant, Kind: model.KindToolUse, ToolName: name, Text: text, Time: ts})
	}
	switch head.Type {
	case "message":
		var item struct {
			Content []textPart `json:"content"`
		}
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		var texts []string
		for _, c := range item.Content {
			if c.Type == "input_text" || c.Type == "output_text" {
				texts = append(texts, c.Text)
			}
		}
		text := strings.TrimSpace(strings.Join(texts, "\n"))
		if text == "" {
			return nil
		}
		switch head.Role {
		case "user":
			if injectedRe.MatchString(text) {
				return nil
			}
			sc.add(srcUserResp, model.Message{Role: model.RoleUser, Kind: model.KindText, Text: cleanPrompt(text), Time: ts})
		case "assistant":
			sc.add(srcAsstResp, model.Message{Role: model.RoleAssistant, Kind: model.KindText, Text: text, Time: ts})
		}
	case "reasoning":
		var item struct {
			Summary []textPart `json:"summary"`
		}
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		var texts []string
		for _, p := range item.Summary {
			texts = append(texts, p.Text)
		}
		sc.add(srcOther, model.Message{Role: model.RoleAssistant, Kind: model.KindThinking, Text: strings.Join(texts, "\n"), Time: ts})
	case "function_call":
		var item struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		tool(item.Name, argsSummary(item.Arguments))
	case "custom_tool_call":
		var item struct {
			Name  string `json:"name"`
			Input string `json:"input"`
		}
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		tool(item.Name, inputSummary(item.Input))
	case "local_shell_call":
		var item struct {
			Action struct {
				Command []string `json:"command"`
			} `json:"action"`
		}
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		tool("shell", commandSummary(item.Action.Command))
	case "web_search_call":
		var item struct {
			Action *struct {
				Query string `json:"query"`
			} `json:"action"`
		}
		_ = json.Unmarshal(payload, &item) // the action shape varies; the query is optional
		q := ""
		if item.Action != nil {
			q = item.Action.Query
		}
		tool("web_search", q)
	case "function_call_output", "custom_tool_call_output":
		var item struct {
			Output json.RawMessage `json:"output"`
		}
		if err := json.Unmarshal(payload, &item); err != nil {
			return err
		}
		sc.add(srcOther, model.Message{Role: model.RoleUser, Kind: model.KindToolResult, Text: outputText(item.Output), Time: ts})
	}
	return nil
}

// argsSummary renders a function call's JSON arguments as one short line.
func argsSummary(args string) string {
	var in map[string]any
	if json.Unmarshal([]byte(args), &in) != nil {
		return textutil.Truncate(textutil.Collapse(args), 160)
	}
	if cmd, ok := in["command"].([]any); ok {
		var parts []string
		for _, c := range cmd {
			if s, ok := c.(string); ok {
				parts = append(parts, s)
			}
		}
		return commandSummary(parts)
	}
	for _, key := range []string{"command", "cmd", "path", "file_path", "query", "pattern", "url"} {
		if v, ok := in[key].(string); ok && v != "" {
			return textutil.Truncate(textutil.Collapse(v), 160)
		}
	}
	return textutil.Truncate(textutil.Collapse(args), 160)
}

// commandSummary renders an argv, unwrapping `bash -lc "<script>"`.
func commandSummary(argv []string) string {
	if len(argv) == 3 && (argv[1] == "-lc" || argv[1] == "-c") {
		return textutil.Truncate(textutil.Collapse(argv[2]), 160)
	}
	return textutil.Truncate(textutil.Collapse(strings.Join(argv, " ")), 160)
}

var patchFileRe = regexp.MustCompile(`(?m)^\*\*\* (?:Add|Update|Delete) File: (.+)$`)

// inputSummary renders a free-form tool input; for apply_patch it lists the
// files the patch touches.
func inputSummary(input string) string {
	if m := patchFileRe.FindAllStringSubmatch(input, -1); m != nil {
		var files []string
		for _, f := range m {
			files = append(files, strings.TrimSpace(f[1]))
		}
		return textutil.Truncate(strings.Join(files, ", "), 160)
	}
	return textutil.Truncate(textutil.FirstLine(input), 160)
}

// outputText flattens a tool output, which is a string or a content list.
func outputText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []textPart
	if json.Unmarshal(raw, &parts) == nil {
		var texts []string
		for _, p := range parts {
			if p.Text != "" {
				texts = append(texts, p.Text)
			}
		}
		return strings.Join(texts, "\n")
	}
	var obj struct {
		Content string `json:"content"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		return obj.Content
	}
	return ""
}

const (
	maxPromptRunes = 500
	maxTitleRunes  = 200
)

// Parse summarises the thread whose active rollout is f.
func (p *Provider) Parse(ctx context.Context, f provider.FileRef) (model.Session, []provider.Warning, error) {
	sc, err := p.read(ctx, f.Path)
	if err != nil {
		return model.Session{}, nil, err
	}
	if sc.meta != nil && hidden(sc.meta) {
		return model.Session{}, sc.warns, provider.ErrHidden
	}

	s := model.Session{Tool: model.ToolCodex, Path: f.Path, Model: sc.model}
	if name, ok := parseRolloutName(filepath.Base(f.Path)); ok {
		s.ID = name.thread
	}
	if m := sc.meta; m != nil {
		if m.ID != "" {
			s.ID = m.ID
		}
		s.CWD = m.CWD
		s.Version = m.CLIVersion
		if m.Git != nil {
			s.GitBranch = m.Git.Branch
		}
	}
	if s.CWD == "" {
		s.CWD = sc.lastCWD
	}

	us, as := sc.userSource(), sc.asstSource()
	var firstRaw string
	for _, e := range sc.entries {
		if e.src != us {
			continue
		}
		if firstRaw == "" {
			firstRaw = e.msg.Text
			s.FirstPrompt = textutil.Truncate(textutil.Collapse(e.msg.Text), maxPromptRunes)
		}
		s.LastPrompt = textutil.Truncate(textutil.Collapse(e.msg.Text), maxPromptRunes)
	}
	s.UserTurns, s.AssistantTurns = sc.counts[us], sc.counts[as]
	if s.UserTurns == 0 && s.AssistantTurns == 0 {
		return model.Session{}, sc.warns, provider.ErrEmpty
	}

	// Prefer per-response usage records; older rollouts only have
	// token_count events.
	usage := sc.usage
	if !usage.seen {
		usage = sc.usageEvent
	}
	s.Usage, s.Breakdown = usage.total, usage.entries
	s.StartedAt, s.UpdatedAt = sc.started, sc.updated
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = f.ModTime.UTC()
	}
	if s.StartedAt.IsZero() {
		s.StartedAt = s.UpdatedAt
	}

	_, names := p.index(ctx)
	s.Title = textutil.Truncate(textutil.FirstNonEmpty(
		textutil.FirstLine(names[s.ID]),
		textutil.FirstLine(firstRaw),
		"(untitled)",
	), maxTitleRunes)
	return s, sc.warns, nil
}

// Transcript returns the thread's conversation, including history reached
// through history_base.
func (p *Provider) Transcript(ctx context.Context, s model.Session) ([]model.Message, error) {
	sc, err := p.read(ctx, s.Path)
	if err != nil {
		return nil, err
	}
	return sc.messages(), nil
}
