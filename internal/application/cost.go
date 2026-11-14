package application

import (
	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/pricing"
)

// loadPrices returns the user's pricing table, or the built-in prices when
// the config is missing or broken. This matches `ais stats`.
func loadPrices() *pricing.Table {
	path, err := pricing.ConfigPath()
	if err != nil {
		return pricing.Defaults()
	}
	table, err := pricing.Load(path)
	if err != nil || table == nil {
		return pricing.Defaults()
	}
	return table
}

// sessionCost prices a session's usage per model and time slot, the same way
// `ais stats` does. It returns nil when nothing can be priced, and partial
// when some usage has no known price.
func sessionCost(prices *pricing.Table, s model.Session) (cost *float64, partial bool) {
	entries := s.Breakdown
	if len(entries) == 0 && !s.Usage.IsZero() {
		entries = []model.UsageEntry{{Slot: model.SlotOf(s.UpdatedAt), Model: s.Model, Usage: s.Usage}}
	}
	total, priced := 0.0, false
	for _, entry := range entries {
		price, ok := prices.Lookup(entry.Model)
		if !ok {
			partial = true
			continue
		}
		priced = true
		total += price.EntryCost(entry)
	}
	if !priced {
		return nil, false
	}
	return &total, partial
}
