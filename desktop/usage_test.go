package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The shape Claude Code 2.1.325 answered get_usage with, trimmed.
const claudeUsageAnswer = `{
  "session": {"total_cost_usd": 0},
  "subscription_type": "pro",
  "rate_limits_available": true,
  "rate_limits": {
    "five_hour": {"utilization": 52, "resets_at": "2026-11-16T10:50:00.001781+00:00", "locked_reason": null},
    "seven_day": {"utilization": 87, "resets_at": "2026-11-16T16:59:59.001807+00:00", "locked_reason": null},
    "seven_day_opus": null,
    "extra_usage": {"is_enabled": false, "user_disabled": true},
    "limits": [
      {"kind": "session", "group": "session", "percent": 52, "resets_at": "2026-11-16T10:50:00.001781+00:00", "severity": "normal", "is_active": false, "scope": null},
      {"kind": "weekly_all", "group": "weekly", "percent": 87, "resets_at": "2026-11-16T16:59:59.001807+00:00", "severity": "warning", "is_active": false, "scope": null},
      {"kind": "weekly_scoped", "group": "weekly", "percent": 88, "resets_at": "2026-11-16T17:00:00.002106+00:00", "severity": "warning", "is_active": true, "scope": {"model": {"display_name": "Fable", "id": null}, "surface": null}}
    ],
    "seven_day_breakdown": {"rows": [
      {"key": "claude_code", "display_name": "Claude Code", "percent": 95},
      {"key": "chat", "display_name": "Chats", "percent": 1},
      {"key": "other", "display_name": "Other", "percent": 0}
    ]}
  },
  "behaviors": {}
}`

// The shape Codex 0.169.0 answered account/rateLimits/read with, trimmed.
const codexUsageAnswer = `{
  "accountId": "acct-test",
  "ordinaryUsageAllowed": true,
  "rateLimitResetCredits": {"availableCount": 3, "credits": [
    {"id": "a", "status": "available", "resetType": "codexRateLimits", "grantedAt": 1793477240, "expiresAt": 1796161497, "title": "Full reset"},
    {"id": "b", "status": "available", "resetType": "codexRateLimits", "grantedAt": 1793477240, "expiresAt": 1796069240, "title": "Full reset"},
    {"id": "c", "status": "redeemed", "resetType": "codexRateLimits", "grantedAt": 1793477240, "expiresAt": 1790000000}
  ]},
  "rateLimitUpsell": null,
  "rateLimits": {"planType": "plus", "primary": {"usedPercent": 37, "resetsAt": 1795210962, "windowDurationMins": 10080}, "secondary": null},
  "rateLimitsByLimitId": {
    "codex": {"limitId": "codex", "planType": "plus", "primary": {"usedPercent": 37, "resetsAt": 1795210962, "windowDurationMins": 10080}, "secondary": null, "rateLimitReachedType": null, "spendControlReached": false}
  }
}`

func limitsText(usage AgentUsage) string {
	parts := []string{}
	for _, limit := range usage.Limits {
		parts = append(parts, fmt.Sprintf("%s=%g%% %s %s", limit.Label, limit.Percent, limit.Level, limit.ResetsAt))
	}
	return strings.Join(parts, "; ")
}

func TestParseClaudeUsage(t *testing.T) {
	usage := parseClaudeUsage(object(json.RawMessage(claudeUsageAnswer)))
	want := "5-hour session=52% normal 2026-11-16T10:50:00Z; Weekly · all models=87% warning 2026-11-16T16:59:59Z; Weekly · Fable=88% warning 2026-11-16T17:00:00Z"
	if got := limitsText(usage); got != want {
		t.Fatalf("limits:\n got %s\nwant %s", got, want)
	}
	if usage.Plan != "Pro" || usage.Blocked || usage.Reason != "" || usage.ExtraUsage != "off" {
		t.Fatalf("usage: %+v", usage)
	}
	if usage.Limits[0].Window != "session" || usage.Limits[2].Window != "weekly" {
		t.Fatalf("windows: %+v", usage.Limits)
	}
	// Parts with no share are left out.
	if fmt.Sprint(usage.Breakdown) != "[{Claude Code 95} {Chats 1}]" {
		t.Fatalf("breakdown: %+v", usage.Breakdown)
	}

	// An older answer without the list, with the week used up.
	older := parseClaudeUsage(object(json.RawMessage(`{"subscription_type":"max","rate_limits":{
		"five_hour":{"utilization":12.5,"resets_at":"2026-11-16T10:50:00Z"},
		"seven_day":{"utilization":100,"resets_at":"2026-11-18T09:00:00Z","locked_reason":"weekly_limit"},
		"seven_day_sonnet":{"utilization":40}}}`)))
	want = "5-hour session=12.5% normal 2026-11-16T10:50:00Z; Weekly · all models=100% reached 2026-11-18T09:00:00Z; Weekly · Sonnet=40% normal "
	if got := limitsText(older); got != want || !older.Blocked || older.Plan != "Max" || older.ExtraUsage != "" {
		t.Fatalf("older answer:\n got %s (%+v)\nwant %s", got, older, want)
	}

	// Signed in with an API key: no plan limits.
	none := parseClaudeUsage(object(json.RawMessage(`{"subscription_type":null,"rate_limits_available":false,"rate_limits":null}`)))
	if len(none.Limits) != 0 || !strings.Contains(none.Reason, "not to API keys") {
		t.Fatalf("no limits: %+v", none)
	}
	if empty := parseClaudeUsage(object(json.RawMessage(`{}`))); !strings.Contains(empty.Reason, "did not report") {
		t.Fatalf("empty answer: %+v", empty)
	}
	// A limit that is reached blocks, whatever severity it reports.
	reached := parseClaudeUsage(object(json.RawMessage(`{"rate_limits":{"limits":[{"kind":"session","group":"session","percent":100,"severity":"critical"},{"kind":"weekly_all","group":"weekly","percent":91,"severity":"critical"}]}}`)))
	if got := limitsText(reached); got != "5-hour session=100% reached ; Weekly · all models=91% warning " || !reached.Blocked {
		t.Fatalf("reached: %s %+v", got, reached)
	}
}

func TestParseCodexUsage(t *testing.T) {
	usage := parseCodexUsage(object(json.RawMessage(codexUsageAnswer)))
	if got := limitsText(usage); got != "Weekly=37% normal 2026-11-20T21:42:42Z" {
		t.Fatalf("limits: %s", got)
	}
	if usage.Plan != "Plus" || usage.Blocked || usage.Reason != "" {
		t.Fatalf("usage: %+v", usage)
	}
	// The first available reset to expire; a redeemed one doesn't count.
	if usage.ResetCredits == nil || usage.ResetCredits.Available != 3 || usage.ResetCredits.NextExpiry != "2026-11-30T20:07:20Z" {
		t.Fatalf("reset credits: %+v", usage.ResetCredits)
	}

	// Two windows, a second bucket, and usage not allowed.
	both := parseCodexUsage(object(json.RawMessage(`{"ordinaryUsageAllowed":false,"rateLimits":{},"rateLimitsByLimitId":{
		"zeta":{"limitName":"Spark","primary":{"usedPercent":5,"windowDurationMins":1440}},
		"codex":{"planType":"pro","primary":{"usedPercent":100,"resetsAt":1795210962,"windowDurationMins":300},"secondary":{"usedPercent":64,"windowDurationMins":10080},"rateLimitReachedType":"rate_limit_reached"}}}`)))
	want := "5-hour session=100% reached 2026-11-20T21:42:42Z; Weekly=64% normal ; 1-day · Spark=5% normal "
	if got := limitsText(both); got != want || !both.Blocked || both.Plan != "Pro" || both.ResetCredits != nil {
		t.Fatalf("two windows:\n got %s (%+v)\nwant %s", got, both, want)
	}
	// Only the older single view.
	single := parseCodexUsage(object(json.RawMessage(`{"rateLimits":{"planType":"unknown","primary":{"usedPercent":81,"windowDurationMins":90}}}`)))
	if got := limitsText(single); got != "90-minute=81% warning " || single.Plan != "" {
		t.Fatalf("single view: %s %+v", got, single)
	}
	if empty := parseCodexUsage(object(json.RawMessage(`{"rateLimits":{"primary":null}}`))); !strings.Contains(empty.Reason, "did not report") {
		t.Fatalf("empty answer: %+v", empty)
	}
}

// TestUsageAgentProcess stands in for an agent answering a usage read. It
// logs every request it receives, so the tests can check that nothing but the
// read was sent.
func TestUsageAgentProcess(t *testing.T) {
	marker := -1
	for i, arg := range os.Args {
		if arg == "ais-fake-usage" {
			marker = i
			break
		}
	}
	if marker < 0 {
		return
	}
	provider, mode := os.Args[marker+1], os.Args[marker+2]
	logFile, _ := os.OpenFile(os.Getenv("AIS_FAKE_USAGE_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	encoder := json.NewEncoder(os.Stdout)
	if mode == "crash" {
		fmt.Fprintln(os.Stderr, "not signed in")
		os.Exit(1)
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		v := object(scanner.Bytes())
		if provider == "claude" {
			request := object(v["request"])
			fmt.Fprintf(logFile, "%s %s\n", str(v["type"]), str(request["subtype"]))
			if str(v["type"]) != "control_request" {
				os.Exit(5)
			}
			id := str(v["request_id"])
			switch {
			case mode == "silent":
			case str(request["subtype"]) == "get_usage" && mode == "error":
				_ = encoder.Encode(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "error", "request_id": id, "error": "Unknown control request subtype: get_usage"}})
			case str(request["subtype"]) == "get_usage":
				_ = encoder.Encode(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": id, "response": json.RawMessage(claudeUsageAnswer)}})
			default:
				_ = encoder.Encode(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": id, "response": map[string]any{}}})
			}
			continue
		}
		method := str(v["method"])
		fmt.Fprintln(logFile, method)
		switch {
		case mode == "silent" || method == "initialized":
		case method == "initialize":
			// A notification arrives before the answer, as in real use.
			_ = encoder.Encode(map[string]any{"method": "account/updated", "params": map[string]any{}})
			_ = encoder.Encode(map[string]any{"id": 1, "result": map[string]any{"userAgent": "fake"}})
		case method == "account/rateLimits/read" && mode == "error":
			_ = encoder.Encode(map[string]any{"id": 2, "error": map[string]any{"code": -32600, "message": "codex account authentication required to read rate limits"}})
		case method == "account/rateLimits/read":
			_ = encoder.Encode(map[string]any{"id": 2, "result": json.RawMessage(codexUsageAnswer)})
		default:
			os.Exit(5)
		}
	}
	os.Exit(0)
}

func usageChats(t *testing.T, modes map[string]string) (*chatManager, *atomic.Int32, string) {
	t.Helper()
	m := newChats(t.TempDir())
	t.Cleanup(m.close)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	log := t.TempDir() + "/requests.log"
	t.Setenv("AIS_FAKE_USAGE_LOG", log)
	starts := &atomic.Int32{}
	m.command = func(provider string, args ...string) *exec.Cmd {
		starts.Add(1)
		mode := modes[provider]
		if mode == "missing" {
			return exec.Command("ais-missing-agent-for-tests")
		}
		return exec.Command(executable, append([]string{"-test.run=^TestUsageAgentProcess$", "--", "ais-fake-usage", provider, mode}, args...)...)
	}
	return m, starts, log
}

func TestUsageLimitsReadsBothAgents(t *testing.T) {
	m, starts, log := usageChats(t, map[string]string{"claude": "ok", "codex": "ok"})
	ctx := context.Background()
	overview := m.usageLimits(ctx, false)
	if len(overview.Agents) != 2 || overview.Agents[0].Tool != "claude" || overview.Agents[1].Tool != "codex" {
		t.Fatalf("agents: %+v", overview.Agents)
	}
	claude, codex := overview.Agents[0], overview.Agents[1]
	if len(claude.Limits) != 3 || claude.Reason != "" || len(codex.Limits) != 1 || codex.Reason != "" {
		t.Fatalf("limits: %+v / %+v", claude, codex)
	}
	if _, err := time.Parse(time.RFC3339, claude.ReadAt); err != nil {
		t.Fatalf("read time: %q", claude.ReadAt)
	}
	// Only the read was asked for: no turn, no thread, no reset used.
	data, _ := os.ReadFile(log)
	requests := strings.Fields(strings.ReplaceAll(string(data), "\n", " "))
	got := strings.Join(requests, ",")
	for _, want := range []string{"control_request,initialize", "control_request,get_usage", "initialize", "initialized", "account/rateLimits/read"} {
		if !strings.Contains(got, want) {
			t.Fatalf("requests %s lack %s", got, want)
		}
	}
	if len(requests) != 7 {
		t.Fatalf("unexpected requests: %s", got)
	}
	// Claude Code is started with no tools, MCP servers or slash commands.
	if strings.Join(claudeQueryArgs, " ") != `-p --input-format stream-json --output-format stream-json --verbose --tools  --strict-mcp-config --mcp-config {"mcpServers":{}} --disable-slash-commands` {
		t.Fatalf("query options changed: %v", claudeQueryArgs)
	}

	// A second read within the minute reuses the first.
	if m.usageLimits(ctx, false); starts.Load() != 2 {
		t.Fatalf("a fresh read must be reused: %d starts", starts.Load())
	}
	// A forced read right after another one reuses it too.
	if m.usageLimits(ctx, true); starts.Load() != 2 {
		t.Fatalf("a forced read right after a read: %d starts", starts.Load())
	}
	for _, cache := range m.usage {
		cache.at = cache.at.Add(-10 * time.Second)
	}
	if m.usageLimits(ctx, true); starts.Load() != 4 {
		t.Fatalf("refresh must read again: %d starts", starts.Load())
	}
	if m.usageLimits(ctx, false); starts.Load() != 4 {
		t.Fatalf("still fresh: %d starts", starts.Load())
	}
}

func TestUsageLimitsExplainsFailures(t *testing.T) {
	original := usageReadLimit
	usageReadLimit = 2 * time.Second
	t.Cleanup(func() { usageReadLimit = original })
	for _, tc := range []struct {
		claude, codex string
		want          [2]string
	}{
		{"error", "error", [2]string{"Claude Code could not report its limits: Unknown control request subtype: get_usage", "Codex could not report its limits: codex account authentication required to read rate limits"}},
		{"missing", "crash", [2]string{"Claude Code is not installed.", "not signed in"}},
		{"silent", "silent", [2]string{"Claude Code did not report its limits within 2 seconds.", "Codex did not report its limits within 2 seconds."}},
	} {
		m, _, _ := usageChats(t, map[string]string{"claude": tc.claude, "codex": tc.codex})
		started := time.Now()
		overview := m.usageLimits(context.Background(), false)
		if time.Since(started) > 6*time.Second {
			t.Fatalf("%s/%s: both agents must be read at once", tc.claude, tc.codex)
		}
		for i, agent := range overview.Agents {
			if len(agent.Limits) != 0 || !strings.Contains(agent.Reason, tc.want[i]) || agent.ReadAt == "" {
				t.Fatalf("%s: %+v, want reason %q", agent.Tool, agent, tc.want[i])
			}
		}
	}

	m, starts, _ := usageChats(t, map[string]string{"claude": "ok", "codex": "ok"})
	m.close()
	if overview := m.usageLimits(context.Background(), true); !strings.Contains(overview.Agents[0].Reason, "closing") || starts.Load() != 0 {
		t.Fatalf("closing: %+v", overview)
	}
}
