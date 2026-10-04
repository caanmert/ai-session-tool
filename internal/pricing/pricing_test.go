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
		"claude-fable-5-1":          10,
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
