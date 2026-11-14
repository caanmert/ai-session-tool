package application

import (
	"math"
	"testing"

	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/pricing"
)

func TestSessionCost(t *testing.T) {
	prices := pricing.Defaults()
	million := model.Usage{Input: 1_000_000, Output: 1_000_000}

	// claude-sonnet-5-5: $2 in + $10 out per million tokens.
	known := model.Session{Model: "claude-sonnet-5-5", Usage: million}
	cost, partial := sessionCost(prices, known)
	if cost == nil || math.Abs(*cost-12) > 1e-9 || partial {
		t.Fatalf("priced session: %v partial=%v", cost, partial)
	}

	// A model without a price is never guessed.
	unknown := model.Session{Model: "some-new-model", Usage: million}
	if cost, partial := sessionCost(prices, unknown); cost != nil || partial {
		t.Fatalf("unpriced session should have no cost: %v partial=%v", cost, partial)
	}

	// Mixed usage is priced where possible and marked partial.
	mixed := model.Session{Breakdown: []model.UsageEntry{
		{Model: "claude-sonnet-5-5", Usage: million},
		{Model: "some-new-model", Usage: million},
	}}
	cost, partial = sessionCost(prices, mixed)
	if cost == nil || math.Abs(*cost-12) > 1e-9 || !partial {
		t.Fatalf("mixed session: %v partial=%v", cost, partial)
	}

	if cost, _ := sessionCost(prices, model.Session{}); cost != nil {
		t.Fatalf("a session without usage has no cost: %v", *cost)
	}
}
