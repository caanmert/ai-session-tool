// Package pricing estimates what token usage would cost at API list prices.
//
// Built-in prices cover current Claude models (Anthropic first-party API
// rates as of 2026-11). Anything else, including the OpenAI models Codex
// uses, is priced only when you add it to the config file:
//
//	# ~/.config/ais/config.toml
//	[prices."gpt-5.5-codex"]   # a model id or id prefix; the longest match wins
//	input = 1.25               # USD per million tokens
//	output = 10.0
//	cache_read = 0.125
//	# cache_write / cache_write_1h default to 1.25x / 2x input
//
// A prefix in the config file prices every id that continues it after a
// separator: "gpt-5" also prices "gpt-5.4" and "gpt-5.5-codex". Built-in
// prices also cover
// dated ids ("claude-haiku-4-5-20251001") but not a later point release:
// "claude-fable-5" does not price "claude-fable-5-3", which is reported as
// unpriced until it has its own price.
package pricing

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/caanmert/ai-session-tool/internal/model"
)

// Price is USD per million tokens.
type Price struct {
	Input        float64 `toml:"input"`
	Output       float64 `toml:"output"`
	CacheRead    float64 `toml:"cache_read"`
	CacheWrite   float64 `toml:"cache_write"`    // 5-minute TTL; default 1.25x input
	CacheWrite1h float64 `toml:"cache_write_1h"` // 1-hour TTL; default 2x input
	// Long, when set, replaces these prices for every token of a request
	// whose prompt (input plus cache reads and writes) is over LongOver
	// tokens. Only built-in prices have it.
	Long     *Price `toml:"-"`
	LongOver int64  `toml:"-"`
}

func (p Price) withDefaults() Price {
	if p.CacheWrite == 0 {
		p.CacheWrite = 1.25 * p.Input
	}
	if p.CacheWrite1h == 0 {
		p.CacheWrite1h = 2 * p.Input
	}
	return p
}

// Cost is what u costs at price p, in USD.
func (p Price) Cost(u model.Usage) float64 {
	p = p.withDefaults()
	short := u.CacheWrite - u.CacheWrite1h
	return (float64(u.Input)*p.Input +
		float64(u.Output)*p.Output +
		float64(u.CacheRead)*p.CacheRead +
		float64(short)*p.CacheWrite +
		float64(u.CacheWrite1h)*p.CacheWrite1h) / 1e6
}

// EntryCost is what e costs at price p, in USD, using the long-prompt
// prices for usage from requests over the threshold.
func (p Price) EntryCost(e model.UsageEntry) float64 {
	if e.LongPrompt && p.Long != nil {
		return p.Long.Cost(e.Usage)
	}
	return p.Cost(e.Usage)
}

// defaults are Anthropic first-party API list prices, checked 2026-11-13
// against https://platform.claude.com/docs/en/about-claude/pricing. Cache
// writes are 1.25x (5m) and 2x (1h) input; cache reads vary by model.
var defaults = map[string]Price{
	"claude-fable-5-2":  {Input: 8, Output: 40, CacheRead: 0.20},
	"claude-mythos-5-2": {Input: 8, Output: 40, CacheRead: 0.20},
	"claude-fable-5-1":  {Input: 10, Output: 50, CacheRead: 0.25},
	"claude-mythos-5-1": {Input: 10, Output: 50, CacheRead: 0.25},
	"claude-fable-5":    {Input: 10, Output: 50, CacheRead: 1.00},
	"claude-mythos-5":   {Input: 10, Output: 50, CacheRead: 1.00},
	"claude-opus-5-5":   {Input: 4, Output: 20, CacheRead: 0.20},
	"claude-opus-5":     {Input: 5, Output: 25, CacheRead: 0.50},
	"claude-opus-4-8":   {Input: 5, Output: 25, CacheRead: 0.50},
	"claude-opus-4-7":   {Input: 5, Output: 25, CacheRead: 0.50},
	"claude-opus-4-6":   {Input: 5, Output: 25, CacheRead: 0.50},
	"claude-opus-4-5":   {Input: 5, Output: 25, CacheRead: 0.50},
	"claude-sonnet-5-5": {Input: 2, Output: 10, CacheRead: 0.10},
	"claude-sonnet-5":   {Input: 2, Output: 10, CacheRead: 0.20},
	"claude-sonnet-4-6": {Input: 3, Output: 15, CacheRead: 0.30},
	"claude-sonnet-4-5": {Input: 3, Output: 15, CacheRead: 0.30},
	"claude-haiku-5-5": {Input: 0.10, Output: 0.50, CacheRead: 0.01,
		LongOver: 100_000, Long: &Price{Input: 0.50, Output: 2.50, CacheRead: 0.05}},
	"claude-haiku-4-5": {Input: 1, Output: 5, CacheRead: 0.10},
}

// builtin classifies requests as transcripts are read, so the split does
// not depend on what the config file says.
var builtin = Defaults()

// LongPrompt reports whether a request to modelID with usage u is billed at
// the model's long-prompt prices.
func LongPrompt(modelID string, u model.Usage) bool {
	p, ok := builtin.Lookup(modelID)
	return ok && p.Long != nil && u.Prompt() > p.LongOver
}

// Table looks up prices by model id.
type Table struct {
	prices map[string]Price
	// configured holds the ids priced by the config file, which also cover
	// later point releases.
	configured map[string]bool
	// Source says where custom prices came from ("" when none).
	Source string
}

// Defaults returns the built-in table.
func Defaults() *Table {
	t := &Table{prices: map[string]Price{}, configured: map[string]bool{}}
	for k, v := range defaults {
		t.prices[k] = v
	}
	return t
}

// Lookup returns the price for a model id: the longest priced id that is a
// prefix of it, so "claude-opus-5" does not swallow "claude-opus-5-5" and
// dated ids like "claude-haiku-4-5-20251001" still match. A built-in price
// does not cover a later point release, which has no price until added.
func (t *Table) Lookup(modelID string) (Price, bool) {
	best := ""
	for prefix := range t.prices {
		if strings.HasPrefix(modelID, prefix) && len(prefix) > len(best) && boundary(modelID, prefix, !t.configured[prefix]) {
			best = prefix
		}
	}
	if best == "" {
		return Price{}, false
	}
	return t.prices[best], true
}

// boundary requires the match to end at a separator, so "claude-opus-5"
// matches "claude-opus-5-20260101" but not "claude-opus-55". When strict,
// it also rejects a point release, so "claude-opus-5" does not match
// "claude-opus-5-1".
func boundary(id, prefix string, strict bool) bool {
	rest := id[len(prefix):]
	if rest == "" {
		return true
	}
	switch rest[0] {
	case '-', '.':
		return !strict || !pointRelease(rest[1:])
	case '_', '@', ':', '[':
		return true
	}
	return false
}

// pointRelease reports whether s starts with a one- or two-digit number, a
// version like the "2" in "claude-fable-5-2", not a date like "20260101".
func pointRelease(s string) bool {
	n := 0
	for n < len(s) && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	return n >= 1 && n <= 2
}

// Models lists the ids with a price, sorted.
func (t *Table) Models() []string {
	out := make([]string, 0, len(t.prices))
	for k := range t.prices {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ConfigPath is $AIS_CONFIG, else $XDG_CONFIG_HOME/ais/config.toml, else
// ~/.config/ais/config.toml.
func ConfigPath() (string, error) {
	if p := os.Getenv("AIS_CONFIG"); p != "" {
		return p, nil
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "ais", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "ais", "config.toml"), nil
}

// Load returns the built-in prices overridden and extended by the config
// file at path. A missing file is not an error.
func Load(path string) (*Table, error) {
	t := Defaults()
	var cfg struct {
		Prices map[string]Price `toml:"prices"`
	}
	md, err := toml.DecodeFile(path, &cfg)
	if errors.Is(err, os.ErrNotExist) {
		return t, nil
	}
	if err != nil {
		return t, fmt.Errorf("%s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return t, fmt.Errorf("%s: unknown setting %q", path, undecoded[0].String())
	}
	for id, p := range cfg.Prices {
		if p.Input < 0 || p.Output < 0 || p.CacheRead < 0 || p.CacheWrite < 0 || p.CacheWrite1h < 0 {
			return t, fmt.Errorf("%s: negative price for %q", path, id)
		}
		t.prices[id] = p
		t.configured[id] = true
	}
	if len(cfg.Prices) > 0 {
		t.Source = path
	}
	return t, nil
}
