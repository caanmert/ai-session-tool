package stats

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/pricing"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func entry(slot, modelName string, in, out int64) model.UsageEntry {
	return model.UsageEntry{Slot: model.SlotOf(at(slot)), Model: modelName, Usage: model.Usage{Input: in, Output: out}}
}

var sessions = []model.Session{
	{Tool: model.ToolClaude, ID: "a", CWD: "/code/api", Breakdown: []model.UsageEntry{
		entry("2026-10-01T23:50:00Z", "claude-opus-5-5", 1_000_000, 0), // Oct 1 in UTC, Oct 2 in Berlin
		entry("2026-10-02T09:00:00Z", "claude-haiku-4-5", 1_000_000, 1_000_000),
	}},
	{Tool: model.ToolCodex, ID: "b", CWD: "/code/web", Breakdown: []model.UsageEntry{
		entry("2026-10-02T10:00:00Z", "gpt-5.5-codex", 500, 500),
	}},
	{Tool: model.ToolClaude, ID: "c", CWD: "/code/api", Breakdown: []model.UsageEntry{
		entry("2026-09-01T10:00:00Z", "claude-opus-5-5", 0, 1_000_000),
	}},
}

func keys(rows []Row) string {
	var k []string
	for _, r := range rows {
		k = append(k, r.Key)
	}
	return strings.Join(k, ",")
}

func TestByDayRespectsTimeZone(t *testing.T) {
	prices := pricing.Defaults()
	rep, err := Build(sessions, prices, Options{By: "day", Loc: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	if got := keys(rep.Rows); got != "2026-09-01,2026-10-01,2026-10-02" {
		t.Errorf("UTC days: %s", got)
	}
	berlin, _ := time.LoadLocation("Europe/Berlin")
	rep, _ = Build(sessions, prices, Options{By: "day", Loc: berlin})
	if got := keys(rep.Rows); got != "2026-09-01,2026-10-02" {
		t.Errorf("Berlin days: %s", got)
	}
	if rep.Rows[1].Sessions != 2 {
		t.Errorf("Oct 2 sessions = %d, want 2", rep.Rows[1].Sessions)
	}
}

func TestCostsAndUnpriced(t *testing.T) {
	rep, err := Build(sessions, pricing.Defaults(), Options{By: "model", Loc: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	// Biggest first: haiku 2M, opus 2M (tie -> name), codex 1k.
	if got := keys(rep.Rows); got != "claude-haiku-4-5,claude-opus-5-5,gpt-5.5-codex" {
		t.Errorf("model rows: %s", got)
	}
	cost := map[string]float64{}
	for _, r := range rep.Rows {
		cost[r.Key] = r.Cost
	}
	if math.Abs(cost["claude-opus-5-5"]-24) > 1e-9 || math.Abs(cost["claude-haiku-4-5"]-6) > 1e-9 {
		t.Errorf("costs: %v", cost)
	}
	codex := rep.Rows[2]
	if codex.Priced || codex.Complete || codex.Cost != 0 {
		t.Errorf("codex row should be unpriced: %+v", codex)
	}
	if rep.Total.Complete || !rep.Total.Priced || math.Abs(rep.Total.Cost-30) > 1e-9 || rep.Total.Sessions != 3 {
		t.Errorf("total: %+v", rep.Total)
	}
	if strings.Join(rep.Unpriced, ",") != "gpt-5.5-codex" {
		t.Errorf("unpriced: %v", rep.Unpriced)
	}
}

func TestFiltersAndGroups(t *testing.T) {
	prices := pricing.Defaults()
	rep, _ := Build(sessions, prices, Options{By: "project", Since: at("2026-09-15T00:00:00Z"), Loc: time.UTC})
	if got := keys(rep.Rows); got != "/code/api,/code/web" || rep.Total.Sessions != 2 {
		t.Errorf("since + project: %s, %d sessions", got, rep.Total.Sessions)
	}
	rep, _ = Build(sessions, prices, Options{By: "tool", Tool: model.ToolCodex, Loc: time.UTC})
	if got := keys(rep.Rows); got != "codex" {
		t.Errorf("tool filter: %s", got)
	}
	rep, _ = Build(sessions, prices, Options{By: "month", Loc: time.UTC})
	if got := keys(rep.Rows); got != "2026-09,2026-10" {
		t.Errorf("months: %s", got)
	}
	rep, _ = Build(sessions, prices, Options{By: "week", Loc: time.UTC})
	if got := keys(rep.Rows); got != "2026-08-31,2026-09-28" { // Mondays
		t.Errorf("weeks: %s", got)
	}
	if _, err := Build(sessions, prices, Options{By: "hour"}); err == nil {
		t.Error("unknown grouping accepted")
	}
}

func TestPricedTotalWithoutUnpricedUsage(t *testing.T) {
	rep, _ := Build(sessions[:1], pricing.Defaults(), Options{By: "tool", Loc: time.UTC})
	if !rep.Total.Complete || !rep.Rows[0].Complete {
		t.Errorf("all usage priced: total %+v", rep.Total)
	}
}
