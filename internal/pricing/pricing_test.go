package pricing

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/caanmert/ai-session-tool/internal/model"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestLookupLongestPrefix(t *testing.T) {
	tab := Defaults()
	tests := map[string]float64{ // model -> input price
		"claude-opus-5-5":           4,
		"claude-opus-5":             5,
		"claude-opus-5-20260101":    5,
		"claude-opus-4-5@20251101":  5,
		"claude-opus-4-6[1m]":       5,
		"claude-fable-5-2":          8,
		"claude-mythos-5-2":         8,
		"claude-fable-5-1":          10,
		"claude-sonnet-4-5":         3,
		"claude-haiku-5-5":          0.10,
		"claude-haiku-4-5-20251001": 1,
	}
	for id, want := range tests {
		p, ok := tab.Lookup(id)
		if !ok || p.Input != want {
			t.Errorf("Lookup(%q) = %+v, %v; want input %v", id, p, ok, want)
		}
	}
	for _, id := range []string{"gpt-5.5-codex", "claude-opus-55", "", "<synthetic>"} {
		if _, ok := tab.Lookup(id); ok {
			t.Errorf("Lookup(%q) should have no price", id)
		}
	}
	if p, _ := tab.Lookup("claude-fable-5-1"); p.CacheRead != 0.25 {
		t.Errorf("fable 5.1 must not inherit fable 5's cache read price: %v", p.CacheRead)
	}
}

func TestCacheReadRates(t *testing.T) {
	// Several current models read cache below the usual 0.1x input.
	for id, want := range map[string]float64{
		"claude-fable-5-2":  0.20,
		"claude-opus-5-5":   0.20,
		"claude-sonnet-5-5": 0.10,
		"claude-haiku-5-5":  0.01,
	} {
		if p, ok := Defaults().Lookup(id); !ok || p.CacheRead != want {
			t.Errorf("%s cache read = %v (%v), want %v", id, p.CacheRead, ok, want)
		}
	}
}

func TestBuiltInPricesSkipLaterPointReleases(t *testing.T) {
	// A newer Claude release must not silently borrow an older one's price.
	for _, id := range []string{"claude-fable-5-3", "claude-opus-5-6", "claude-haiku-5-5-1"} {
		if p, ok := Defaults().Lookup(id); ok {
			t.Errorf("Lookup(%q) = %+v; a point release without its own price should be unpriced", id, p)
		}
	}
	// A prefix in the config file is a deliberate family price.
	path := filepath.Join(t.TempDir(), "config.toml")
	write(t, path, "[prices.\"gpt-5\"]\ninput = 1\noutput = 8\n\n[prices.\"claude-fable-5\"]\ninput = 9\noutput = 45\n")
	tab, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"gpt-5", "gpt-5-codex", "gpt-5.4", "gpt-5.5-codex", "claude-fable-5-3"} {
		if _, ok := tab.Lookup(id); !ok {
			t.Errorf("configured prefix should price %q", id)
		}
	}
	if p, _ := tab.Lookup("claude-fable-5-2"); p.Input != 8 {
		t.Errorf("an exact built-in price beats a shorter configured prefix: %+v", p)
	}
}

func TestHaikuLongPrompt(t *testing.T) {
	tests := []struct {
		model string
		u     model.Usage
		want  bool
	}{
		{"claude-haiku-5-5", model.Usage{Input: 100_000}, false}, // "over" 100,000
		{"claude-haiku-5-5", model.Usage{Input: 100_001}, true},
		{"claude-haiku-5-5", model.Usage{Input: 40_000, CacheRead: 50_000, CacheWrite: 10_001}, true},
		{"claude-haiku-5-5", model.Usage{Input: 100_000, Output: 90_000}, false}, // output is not prompt
		{"claude-opus-5-5", model.Usage{Input: 900_000}, false},                  // flat-priced model
		{"gpt-5.5-codex", model.Usage{Input: 900_000}, false},
	}
	for _, tt := range tests {
		if got := LongPrompt(tt.model, tt.u); got != tt.want {
			t.Errorf("LongPrompt(%s, %+v) = %v, want %v", tt.model, tt.u, got, tt.want)
		}
	}
}

func TestEntryCostUsesLongPromptPrices(t *testing.T) {
	p, _ := Defaults().Lookup("claude-haiku-5-5")
	u := model.Usage{Input: 1_000_000, Output: 1_000_000, CacheRead: 1_000_000, CacheWrite: 2_000_000, CacheWrite1h: 1_000_000}
	// Up to 100K: 0.10 in, 0.50 out, 0.01 read, 0.125 5m write, 0.20 1h write.
	if got, want := p.EntryCost(model.UsageEntry{Usage: u}), 0.10+0.50+0.01+0.125+0.20; !near(got, want) {
		t.Errorf("short prompt cost = %v, want %v", got, want)
	}
	// Over 100K: 0.50 in, 2.50 out, 0.05 read, 0.625 5m write, 1.00 1h write.
	if got, want := p.EntryCost(model.UsageEntry{LongPrompt: true, Usage: u}), 0.50+2.50+0.05+0.625+1.00; !near(got, want) {
		t.Errorf("long prompt cost = %v, want %v", got, want)
	}
	flat, _ := Defaults().Lookup("claude-opus-5-5")
	if flat.EntryCost(model.UsageEntry{LongPrompt: true, Usage: u}) != flat.Cost(u) {
		t.Error("a model without long-prompt prices must use its only price")
	}
}

func TestCost(t *testing.T) {
	p, _ := Defaults().Lookup("claude-opus-5-5") // $4 in, $20 out, $0.20 cache read
	u := model.Usage{Input: 1_000_000, Output: 1_000_000, CacheRead: 10_000_000, CacheWrite: 2_000_000, CacheWrite1h: 1_000_000}
	// 4 + 20 + 10*0.2 + 1*(1.25*4) + 1*(2*4)
	if got, want := p.Cost(u), 4+20+2+5+8.0; !near(got, want) {
		t.Errorf("Cost = %v, want %v", got, want)
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if tab, err := Load(path); err != nil || tab.Source != "" {
		t.Fatalf("missing config: %v, %q", err, tab.Source)
	}

	write(t, path, `
[prices."gpt-5.5-codex"]
input = 1.25
output = 10
cache_read = 0.125

[prices."claude-opus-5-5"]   # override a default
input = 3
output = 15
cache_read = 0.3
`)
	tab, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := tab.Lookup("gpt-5.5-codex"); !ok || p.Output != 10 {
		t.Errorf("configured model: %+v %v", p, ok)
	}
	if p, _ := tab.Lookup("claude-opus-5-5"); p.Input != 3 {
		t.Errorf("override: %+v", p)
	}
	if p, _ := tab.Lookup("claude-haiku-4-5"); p.Input != 1 {
		t.Errorf("defaults must stay: %+v", p)
	}
	if tab.Source != path {
		t.Errorf("Source = %q", tab.Source)
	}

	write(t, path, "[prices.x]\ninput = 1\nouptut = 2\n")
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "ouptut") {
		t.Errorf("typo in config should be reported: %v", err)
	}
	write(t, path, "[prices.x]\ninput = -1\n")
	if _, err := Load(path); err == nil {
		t.Error("negative price accepted")
	}
	write(t, path, "not toml [")
	if _, err := Load(path); err == nil {
		t.Error("broken toml accepted")
	}
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}
