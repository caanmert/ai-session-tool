package main

import (
	"math"
	"time"
)

// How long each turn took, and what it used where the agent reports it. The
// figures are kept on the message that started the turn.

// TurnStats describes one finished turn.
type TurnStats struct {
	// Ms is how long the turn ran, from sending to its end.
	Ms int64 `json:"ms"`
	// Tokens is what the agent reported processing in the turn, cached input
	// included. Zero when it reported nothing.
	Tokens int64 `json:"tokens,omitempty"`
	// CostUSD is Claude Code's own estimate at API prices; not a charge on a
	// subscription. Codex reports none.
	CostUSD float64 `json:"costUsd,omitempty"`
}

// turnUsage gathers what the running turn reported; guarded by chatSession.mu.
type turnUsage struct {
	tokens int64
	cost   float64
	// codexBase is Codex's running total before the turn, once known.
	codexBase int64
	haveBase  bool
}

// reportTurn records the turn's tokens and cost so far. Zero leaves a value.
func (c *chatSession) reportTurn(tokens int64, cost float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if tokens > 0 {
		c.usage.tokens = tokens
	}
	if cost > 0 {
		c.usage.cost = cost
	}
}

// reportCodexTokens reads thread/tokenUsage/updated. Its total covers the
// whole conversation; the first update of a turn shows the total before the
// turn, less that update's own request.
func (c *chatSession) reportCodexTokens(params jsonObject) {
	usage := object(params["tokenUsage"])
	total := int64Of(object(usage["total"])["totalTokens"])
	if total <= 0 {
		return
	}
	c.mu.Lock()
	if !c.usage.haveBase {
		c.usage.codexBase = max(0, total-int64Of(object(usage["last"])["totalTokens"]))
		c.usage.haveBase = true
	}
	tokens := total - c.usage.codexBase
	c.mu.Unlock()
	c.reportTurn(tokens, 0)
}

// reportClaudeResult reads a result's tokens across every model it used, and
// Claude Code's cost estimate.
func (c *chatSession) reportClaudeResult(result jsonObject) {
	tokens := int64(0)
	for _, raw := range object(result["modelUsage"]) {
		model := object(raw)
		tokens += int64Of(model["inputTokens"]) + int64Of(model["outputTokens"]) +
			int64Of(model["cacheReadInputTokens"]) + int64Of(model["cacheCreationInputTokens"])
	}
	if tokens == 0 {
		tokens = claudeContextUsed(object(result["usage"]))
	}
	cost, _ := numberValue(result["total_cost_usd"])
	if math.IsNaN(cost) || math.IsInf(cost, 0) || cost < 0 {
		cost = 0
	}
	c.reportTurn(tokens, cost)
}

// finishTurn keeps the turn's figures on the message that started it.
func (c *chatSession) finishTurn(messageID string, started time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	stats := &TurnStats{Ms: time.Since(started).Milliseconds(), Tokens: c.usage.tokens, CostUSD: c.usage.cost}
	c.usage = turnUsage{}
	for i := range c.state.Messages {
		if c.state.Messages[i].ID == messageID {
			c.state.Messages[i].Turn = stats
			c.state.Messages[i].rev = c.state.Version + 1
			c.change()
			return
		}
	}
}
