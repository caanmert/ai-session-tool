// Package stats aggregates token usage and estimated cost over sessions,
// by day, week, month, project, model or tool.
package stats

import (
	"fmt"
	"sort"
	"time"

	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/pricing"
)

// Dimensions stats can be grouped by.
var Dimensions = []string{"day", "week", "month", "project", "model", "tool"}

// Options choose what to aggregate.
type Options struct {
	By    string     // one of Dimensions
	Since time.Time  // zero: all time
	Tool  model.Tool // "": every tool
	Loc   *time.Location
}

// Row is one group.
type Row struct {
	Key      string      `json:"key"`
	Sessions int         `json:"sessions"`
	Usage    model.Usage `json:"usage"`
	Cost     float64     `json:"cost"`     // USD at list prices, for the priced part
	Priced   bool        `json:"priced"`   // some usage had a price
	Complete bool        `json:"complete"` // all usage had a price

	sessions map[string]bool
}

// Report is the aggregated result.
type Report struct {
	By       string    `json:"by"`
	Since    time.Time `json:"since,omitzero"`
	Rows     []Row     `json:"rows"`
	Total    Row       `json:"total"`
	Unpriced []string  `json:"unpricedModels,omitempty"` // models with usage but no price
}

// Valid reports whether by is a known dimension.
func Valid(by string) bool {
	for _, d := range Dimensions {
		if d == by {
			return true
		}
	}
	return false
}

// key returns the group of one usage entry.
func key(by string, s model.Session, e model.UsageEntry, loc *time.Location) string {
	t := e.Slot.In(loc)
	switch by {
	case "day":
		return t.Format("2006-01-02")
	case "week": // the Monday the week starts on
		offset := (int(t.Weekday()) + 6) % 7
		return t.AddDate(0, 0, -offset).Format("2006-01-02")
	case "month":
		return t.Format("2006-01")
	case "project":
		return s.CWD
	case "model":
		if e.Model == "" {
			return "(unknown)"
		}
		return e.Model
	}
	return string(s.Tool)
}

// Build aggregates the usage of sessions.
func Build(sessions []model.Session, prices *pricing.Table, o Options) (Report, error) {
	if !Valid(o.By) {
		return Report{}, fmt.Errorf("unknown grouping %q: use one of %v", o.By, Dimensions)
	}
	if o.Loc == nil {
		o.Loc = time.Local
	}
	rows := map[string]*Row{}
	total := &Row{Key: "total", Complete: true, sessions: map[string]bool{}}
	unpriced := map[string]bool{}

	add := func(r *Row, s model.Session, e model.UsageEntry, cost float64, priced bool) {
		r.sessions[s.Key()] = true
		r.Usage = r.Usage.Add(e.Usage)
		r.Cost += cost
		if priced {
			r.Priced = true
		} else {
			r.Complete = false
		}
	}
	for _, s := range sessions {
		if o.Tool != "" && s.Tool != o.Tool {
			continue
		}
		entries := s.Breakdown
		if len(entries) == 0 && !s.Usage.IsZero() { // defensive: no per-slot data
			entries = []model.UsageEntry{{Slot: model.SlotOf(s.UpdatedAt), Model: s.Model, Usage: s.Usage}}
		}
		for _, e := range entries {
			if !o.Since.IsZero() && e.Slot.Before(o.Since) {
				continue
			}
			price, priced := prices.Lookup(e.Model)
			cost := 0.0
			if priced {
				cost = price.Cost(e.Usage)
			} else {
				unpriced[keyOrUnknown(e.Model)] = true
			}
			k := key(o.By, s, e, o.Loc)
			r := rows[k]
			if r == nil {
				r = &Row{Key: k, Complete: true, sessions: map[string]bool{}}
				rows[k] = r
			}
			add(r, s, e, cost, priced)
			add(total, s, e, cost, priced)
		}
	}

	rep := Report{By: o.By, Since: o.Since}
	for _, r := range rows {
		r.Sessions = len(r.sessions)
		rep.Rows = append(rep.Rows, *r)
	}
	total.Sessions = len(total.sessions)
	rep.Total = *total
	for m := range unpriced {
		rep.Unpriced = append(rep.Unpriced, m)
	}
	sort.Strings(rep.Unpriced)

	switch o.By {
	case "day", "week", "month": // oldest first, like a chart
		sort.Slice(rep.Rows, func(i, j int) bool { return rep.Rows[i].Key < rep.Rows[j].Key })
	default: // biggest first
		sort.Slice(rep.Rows, func(i, j int) bool {
			a, b := rep.Rows[i], rep.Rows[j]
			if a.Usage.Total() != b.Usage.Total() {
				return a.Usage.Total() > b.Usage.Total()
			}
			return a.Key < b.Key
		})
	}
	return rep, nil
}

func keyOrUnknown(m string) string {
	if m == "" {
		return "(unknown)"
	}
	return m
}
