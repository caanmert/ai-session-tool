package main

import (
	"context"
	"testing"
	"time"
)

// Both installed agents report their plan limits. No model turn is started.
func TestLiveUsageLimits(t *testing.T) {
	requireLiveAgents(t)
	m := newChats(t.TempDir())
	t.Cleanup(m.close)
	overview := m.usageLimits(context.Background(), true)
	for _, agent := range overview.Agents {
		if agent.Reason != "" || len(agent.Limits) == 0 {
			t.Fatalf("%s reported no limits: %s", agent.Tool, agent.Reason)
		}
		t.Logf("%s, %s plan, blocked %v, extra usage %q, breakdown %v, reset credits %+v", agent.Tool, agent.Plan, agent.Blocked, agent.ExtraUsage, agent.Breakdown, agent.ResetCredits)
		for _, limit := range agent.Limits {
			t.Logf("  %s: %g%% (%s), resets %s", limit.Label, limit.Percent, limit.Level, limit.ResetsAt)
			if limit.Percent < 0 || limit.Percent > 100 || limit.Label == "" {
				t.Fatalf("unexpected limit: %+v", limit)
			}
			if limit.ResetsAt != "" {
				if _, err := time.Parse(time.RFC3339, limit.ResetsAt); err != nil {
					t.Fatalf("reset time: %v", err)
				}
			}
		}
	}
}
