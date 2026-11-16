package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

// Plan usage limits, as each agent reports them for the account it is signed
// in to. Reading them starts no model turn and spends nothing: Claude Code is
// asked with its get_usage control request and Codex with its app server's
// account/rateLimits/read, and nothing else is sent.

// UsageLimit is one limit window, such as the 5-hour session or the week.
type UsageLimit struct {
	Label string `json:"label"`
	// Window is "session" or "weekly", or empty when the agent doesn't say.
	Window  string  `json:"window,omitempty"`
	Percent float64 `json:"percent"`
	// ResetsAt is when the window starts again, in RFC 3339, when reported.
	ResetsAt string `json:"resetsAt,omitempty"`
	// Level is normal, warning or reached.
	Level string `json:"level"`
}

// UsageShare is part of the week's usage spent in one place.
type UsageShare struct {
	Label   string  `json:"label"`
	Percent float64 `json:"percent"`
}

// ResetCredits are free full resets Codex offers. They are only shown here;
// using one is left to Codex itself.
type ResetCredits struct {
	Available int `json:"available"`
	// NextExpiry is when the first available reset expires, in RFC 3339.
	NextExpiry string `json:"nextExpiry,omitempty"`
}

// AgentUsage is what one agent reports. When Reason is set nothing could be
// read and Reason says why.
type AgentUsage struct {
	Tool   string       `json:"tool"`
	Plan   string       `json:"plan,omitempty"`
	Limits []UsageLimit `json:"limits"`
	// Blocked is set when the agent says ordinary usage is not allowed now.
	Blocked bool `json:"blocked,omitempty"`
	// ExtraUsage is "on" or "off" when Claude Code reports it.
	ExtraUsage   string        `json:"extraUsage,omitempty"`
	Breakdown    []UsageShare  `json:"breakdown,omitempty"`
	ResetCredits *ResetCredits `json:"resetCredits,omitempty"`
	Reason       string        `json:"reason,omitempty"`
	ReadAt       string        `json:"readAt"`
}

// UsageOverview is every agent's usage, Claude Code first.
type UsageOverview struct {
	Agents []AgentUsage `json:"agents"`
}

// A read is reused for a minute, so the sidebar and the page don't each start
// the agents, and a forced read right after another one reuses it too.
const (
	usageFresh    = time.Minute
	usageDebounce = 5 * time.Second
)

// usageReadLimit bounds one agent's answer; tests shorten it.
var usageReadLimit = 25 * time.Second

type usageCache struct {
	gate  chan struct{}
	value AgentUsage
	at    time.Time
}

var usageTools = []string{"claude", "codex"}

func numberValue(raw json.RawMessage) (float64, bool) {
	var value *float64
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return 0, false
	}
	return *value, true
}

// planName turns "pro" or "self_serve_business" into words.
func planName(plan string) string {
	if plan == "" || plan == "unknown" {
		return ""
	}
	words := strings.Fields(strings.ReplaceAll(plan, "_", " "))
	for i, word := range words {
		words[i] = upperFirst(word)
	}
	return strings.Join(words, " ")
}

// usageLevel prefers the agent's own severity and falls back to the percent.
func usageLevel(severity string, percent float64) string {
	if percent >= 100 {
		return "reached"
	}
	switch severity {
	case "normal", "warning":
		return severity
	case "":
		if percent >= 80 {
			return "warning"
		}
		return "normal"
	}
	return "warning"
}

func usageTime(raw json.RawMessage) string {
	if text := str(raw); text != "" {
		if at, err := time.Parse(time.RFC3339Nano, text); err == nil {
			return at.UTC().Round(time.Second).Format(time.RFC3339)
		}
		return ""
	}
	if seconds, ok := numberValue(raw); ok && seconds > 0 {
		return time.Unix(int64(seconds), 0).UTC().Format(time.RFC3339)
	}
	return ""
}

func displayName(raw json.RawMessage) string {
	if name := str(raw); name != "" {
		return name
	}
	return str(object(raw)["display_name"])
}

func claudeLimitLabel(kind, group string, scope jsonObject) string {
	switch kind {
	case "session":
		return "5-hour session"
	case "weekly_all":
		return "Weekly · all models"
	}
	if name := displayName(scope["model"]); name != "" {
		return "Weekly · " + name
	}
	if name := displayName(scope["surface"]); name != "" {
		return "Weekly · " + name
	}
	if group == "weekly" || group == "session" {
		return upperFirst(group)
	}
	return upperFirst(strings.ReplaceAll(kind, "_", " "))
}

// parseClaudeUsage reads the answer to Claude Code's get_usage request.
func parseClaudeUsage(answer jsonObject) AgentUsage {
	usage := AgentUsage{Tool: "claude", Limits: []UsageLimit{}, Plan: planName(str(answer["subscription_type"]))}
	if raw, ok := answer["rate_limits_available"]; ok && string(raw) == "false" {
		usage.Reason = "Claude Code reports no plan limits for this sign-in. Limits apply to Claude subscriptions, not to API keys."
		return usage
	}
	limits := object(answer["rate_limits"])
	for _, raw := range array(limits["limits"]) {
		entry := object(raw)
		percent, ok := numberValue(entry["percent"])
		if !ok {
			continue
		}
		group := str(entry["group"])
		window := ""
		if group == "session" || group == "weekly" {
			window = group
		}
		usage.Limits = append(usage.Limits, UsageLimit{
			Label:    claudeLimitLabel(str(entry["kind"]), group, object(entry["scope"])),
			Window:   window,
			Percent:  percent,
			ResetsAt: usageTime(entry["resets_at"]),
			Level:    usageLevel(str(entry["severity"]), percent),
		})
	}
	// Older versions only have the separate windows.
	older := []struct{ key, label, window string }{
		{"five_hour", "5-hour session", "session"},
		{"seven_day", "Weekly · all models", "weekly"},
		{"seven_day_opus", "Weekly · Opus", "weekly"},
		{"seven_day_sonnet", "Weekly · Sonnet", "weekly"},
	}
	fallback := len(usage.Limits) == 0
	for _, window := range older {
		entry := object(limits[window.key])
		locked := str(entry["locked_reason"]) != ""
		if locked {
			usage.Blocked = true
		}
		percent, ok := numberValue(entry["utilization"])
		if !fallback || !ok {
			continue
		}
		level := usageLevel("", percent)
		if locked {
			level = "reached"
		}
		usage.Limits = append(usage.Limits, UsageLimit{Label: window.label, Window: window.window, Percent: percent, ResetsAt: usageTime(entry["resets_at"]), Level: level})
	}
	if raw, ok := object(limits["extra_usage"])["is_enabled"]; ok {
		usage.ExtraUsage = map[bool]string{true: "on", false: "off"}[boolean(raw)]
	}
	for _, raw := range array(object(limits["seven_day_breakdown"])["rows"]) {
		row := object(raw)
		percent, ok := numberValue(row["percent"])
		if label := str(row["display_name"]); ok && label != "" && percent > 0 {
			usage.Breakdown = append(usage.Breakdown, UsageShare{Label: label, Percent: percent})
		}
	}
	for _, limit := range usage.Limits {
		if limit.Level == "reached" {
			usage.Blocked = true
		}
	}
	if len(usage.Limits) == 0 {
		usage.Reason = "Claude Code did not report any limits for this sign-in."
	}
	return usage
}

func codexWindow(minutes int) (label, window string) {
	switch {
	case minutes == 300:
		return "5-hour session", "session"
	case minutes == 7*24*60:
		return "Weekly", "weekly"
	case minutes > 0 && minutes%(24*60) == 0:
		return fmt.Sprintf("%d-day", minutes/(24*60)), ""
	case minutes > 0 && minutes%60 == 0:
		return fmt.Sprintf("%d-hour", minutes/60), ""
	case minutes > 0:
		return fmt.Sprintf("%d-minute", minutes), ""
	}
	return "Usage", ""
}

// parseCodexUsage reads the result of Codex's account/rateLimits/read.
func parseCodexUsage(result jsonObject) AgentUsage {
	usage := AgentUsage{Tool: "codex", Limits: []UsageLimit{}}
	type bucket struct {
		key      string
		snapshot jsonObject
	}
	buckets := []bucket{}
	byID := object(result["rateLimitsByLimitId"])
	keys := make([]string, 0, len(byID))
	for key := range byID {
		keys = append(keys, key)
	}
	// The main "codex" bucket first, then the rest by name.
	sort.Slice(keys, func(i, j int) bool {
		if (keys[i] == "codex") != (keys[j] == "codex") {
			return keys[i] == "codex"
		}
		return keys[i] < keys[j]
	})
	for _, key := range keys {
		buckets = append(buckets, bucket{key, object(byID[key])})
	}
	if len(buckets) == 0 {
		buckets = append(buckets, bucket{"", object(result["rateLimits"])})
	}
	for _, b := range buckets {
		if usage.Plan == "" {
			usage.Plan = planName(str(b.snapshot["planType"]))
		}
		suffix := ""
		if b.key != "" && b.key != "codex" {
			name := str(b.snapshot["limitName"])
			if name == "" {
				name = b.key
			}
			suffix = " · " + name
		}
		for _, side := range []string{"primary", "secondary"} {
			entry := object(b.snapshot[side])
			percent, ok := numberValue(entry["usedPercent"])
			if !ok {
				continue
			}
			minutes, _ := numberValue(entry["windowDurationMins"])
			label, window := codexWindow(int(minutes))
			usage.Limits = append(usage.Limits, UsageLimit{Label: label + suffix, Window: window, Percent: percent, ResetsAt: usageTime(entry["resetsAt"]), Level: usageLevel("", percent)})
		}
		if str(b.snapshot["rateLimitReachedType"]) != "" || string(b.snapshot["spendControlReached"]) == "true" {
			usage.Blocked = true
		}
	}
	if string(result["ordinaryUsageAllowed"]) == "false" {
		usage.Blocked = true
	}
	for _, limit := range usage.Limits {
		if limit.Level == "reached" {
			usage.Blocked = true
		}
	}
	credits := object(result["rateLimitResetCredits"])
	if count, ok := numberValue(credits["availableCount"]); ok && count > 0 {
		reset := &ResetCredits{Available: int(count)}
		var first time.Time
		for _, raw := range array(credits["credits"]) {
			credit := object(raw)
			expires, ok := numberValue(credit["expiresAt"])
			if str(credit["status"]) != "available" || !ok || expires <= 0 {
				continue
			}
			if at := time.Unix(int64(expires), 0).UTC(); first.IsZero() || at.Before(first) {
				first = at
			}
		}
		if !first.IsZero() {
			reset.NextExpiry = first.Format(time.RFC3339)
		}
		usage.ResetCredits = reset
	}
	if len(usage.Limits) == 0 {
		usage.Reason = "Codex did not report any limits for this sign-in."
	}
	return usage
}

// errorMessage pulls the message out of a JSON-RPC error.
func errorMessage(raw json.RawMessage) string {
	if message := str(object(raw)["message"]); message != "" {
		return message
	}
	return shorten(pretty(raw), 300)
}

// queryUsage asks one agent for its limits and nothing else.
func (m *chatManager) queryUsage(ctx context.Context, tool string) (AgentUsage, error) {
	args := []string{"app-server", "--listen", "stdio://"}
	if tool == "claude" {
		args = claudeQueryArgs
	}
	cmd := m.command(tool, args...)
	// Not a project: no project settings or hooks apply to the read.
	cmd.Dir = os.TempDir()
	p, err := startJSONProcess(ctx, cmd)
	if err != nil {
		return AgentUsage{}, err
	}
	defer p.close()
	if tool == "claude" {
		err = p.send(map[string]any{"type": "control_request", "request_id": "ais-usage-init", "request": map[string]any{"subtype": "initialize"}})
		if err == nil {
			err = p.send(map[string]any{"type": "control_request", "request_id": "ais-usage", "request": map[string]any{"subtype": "get_usage"}})
		}
	} else {
		err = p.send(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]string{"name": "ais_usage", "version": "0.1.0"}}})
	}
	if err != nil {
		return AgentUsage{}, err
	}
	for {
		select {
		case <-ctx.Done():
			return AgentUsage{}, ctx.Err()
		case packet, ok := <-p.packets:
			if !ok {
				return AgentUsage{}, p.failure()
			}
			if packet.Err != nil {
				return AgentUsage{}, packet.Err
			}
			v := packet.Value
			if tool == "claude" {
				if str(v["type"]) != "control_response" {
					continue
				}
				response := object(v["response"])
				id := str(response["request_id"])
				if str(response["subtype"]) == "error" && (id == "ais-usage" || id == "ais-usage-init") {
					return AgentUsage{}, errors.New(str(response["error"]))
				}
				if id == "ais-usage" {
					return parseClaudeUsage(object(response["response"])), nil
				}
				continue
			}
			if str(v["method"]) != "" {
				continue
			}
			if raw := v["error"]; len(raw) > 0 && string(raw) != "null" {
				return AgentUsage{}, errors.New(errorMessage(raw))
			}
			switch number(v["id"]) {
			case 1:
				if err = p.send(map[string]any{"method": "initialized", "params": map[string]any{}}); err == nil {
					err = p.send(map[string]any{"id": 2, "method": "account/rateLimits/read"})
				}
				if err != nil {
					return AgentUsage{}, err
				}
			case 2:
				return parseCodexUsage(object(v["result"])), nil
			}
		}
	}
}

func (m *chatManager) readUsage(ctx context.Context, tool string) AgentUsage {
	ctx, cancel := context.WithTimeout(ctx, usageReadLimit)
	defer cancel()
	usage, err := m.queryUsage(ctx, tool)
	if err != nil {
		name := toolName(tool)
		usage = AgentUsage{Tool: tool, Limits: []UsageLimit{}}
		switch {
		case errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist):
			usage.Reason = name + " is not installed."
		case errors.Is(err, context.DeadlineExceeded):
			usage.Reason = fmt.Sprintf("%s did not report its limits within %d seconds.", name, int(usageReadLimit.Seconds()))
		default:
			usage.Reason = name + " could not report its limits: " + shorten(err.Error(), 400)
		}
	}
	usage.ReadAt = time.Now().UTC().Format(time.RFC3339)
	return usage
}

func (m *chatManager) agentUsage(ctx context.Context, tool string, refresh bool) AgentUsage {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return AgentUsage{Tool: tool, Limits: []UsageLimit{}, Reason: "The app is closing."}
	}
	if m.usage == nil {
		m.usage = map[string]*usageCache{}
	}
	cache := m.usage[tool]
	if cache == nil {
		cache = &usageCache{gate: make(chan struct{}, 1)}
		m.usage[tool] = cache
	}
	m.mu.Unlock()
	select {
	case cache.gate <- struct{}{}:
		defer func() { <-cache.gate }()
	case <-ctx.Done():
		return AgentUsage{Tool: tool, Limits: []UsageLimit{}, Reason: toolName(tool) + " is still being read. Try again in a moment."}
	}
	if !cache.at.IsZero() {
		age := time.Since(cache.at)
		if age < usageDebounce || (!refresh && age < usageFresh) {
			return cache.value
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(m.discoveryCtx, cancel)
	defer stop()
	cache.value, cache.at = m.readUsage(ctx, tool), time.Now()
	return cache.value
}

// usageLimits reads both agents at once.
func (m *chatManager) usageLimits(ctx context.Context, refresh bool) UsageOverview {
	overview := UsageOverview{Agents: make([]AgentUsage, len(usageTools))}
	var wg sync.WaitGroup
	for i, tool := range usageTools {
		wg.Add(1)
		go func() {
			defer wg.Done()
			overview.Agents[i] = m.agentUsage(ctx, tool, refresh)
		}()
	}
	wg.Wait()
	return overview
}

// UsageLimits reports each agent's plan limits and when they reset. A read
// from the last minute is reused unless refresh is set.
func (a *App) UsageLimits(refresh bool) (UsageOverview, error) {
	if err := a.chatReady(); err != nil {
		return UsageOverview{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, usageReadLimit+5*time.Second)
	defer cancel()
	return a.chats.usageLimits(ctx, refresh), nil
}
