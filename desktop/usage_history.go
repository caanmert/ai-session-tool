package main

import (
	"errors"
	"sort"
	"time"
)

// Usage history adds up the figures ais saved with each finished turn (see
// chat_turn_stats.go): how long the agents worked, the tokens they reported
// and Claude Code's cost estimate, by day, project and agent. It reads only
// what is already saved; nothing is asked of the agents.

// UsageTotals adds up turns.
type UsageTotals struct {
	Turns   int     `json:"turns"`
	Ms      int64   `json:"ms"`
	Tokens  int64   `json:"tokens"`
	CostUSD float64 `json:"costUsd"`
}

func (t *UsageTotals) add(turn TurnStats) {
	t.Turns++
	t.Ms += turn.Ms
	t.Tokens += turn.Tokens
	t.CostUSD += turn.CostUSD
}

// UsageDay is one local calendar day, as YYYY-MM-DD.
type UsageDay struct {
	Day string `json:"day"`
	UsageTotals
}

// UsageGroup is the totals for one project folder or one agent.
type UsageGroup struct {
	Key string `json:"key"`
	UsageTotals
}

// UsageHistory covers the last Days days, today included. Days lists every
// day, oldest first, with or without turns.
type UsageHistory struct {
	Days     []UsageDay   `json:"days"`
	Projects []UsageGroup `json:"projects"`
	Agents   []UsageGroup `json:"agents"`
	Total    UsageTotals  `json:"total"`
}

// usageHistory sums the turns that started within the last days days,
// counted in now's time zone.
func (m *chatManager) usageHistory(days int, now time.Time) (UsageHistory, error) {
	if days < 1 || days > 366 {
		return UsageHistory{}, errors.New("choose between 1 and 366 days")
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := today.AddDate(0, 0, 1-days)
	history := UsageHistory{Days: make([]UsageDay, days), Projects: []UsageGroup{}, Agents: []UsageGroup{}}
	index := map[string]int{}
	for i := range history.Days {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		history.Days[i].Day = day
		index[day] = i
	}
	projects := map[string]*UsageTotals{}
	agents := map[string]*UsageTotals{}
	m.mu.Lock()
	sessions := make([]*chatSession, 0, len(m.items))
	for _, c := range m.items {
		sessions = append(sessions, c)
	}
	m.mu.Unlock()
	for _, c := range sessions {
		c.mu.Lock()
		project, tool := c.state.Project, c.state.Tool
		for _, message := range c.state.Messages {
			if message.Turn == nil || message.At == "" {
				continue
			}
			at, err := time.Parse(time.RFC3339, message.At)
			if err != nil {
				continue
			}
			i, ok := index[at.In(now.Location()).Format("2006-01-02")]
			if !ok {
				continue
			}
			history.Days[i].add(*message.Turn)
			history.Total.add(*message.Turn)
			if projects[project] == nil {
				projects[project] = &UsageTotals{}
			}
			projects[project].add(*message.Turn)
			if agents[tool] == nil {
				agents[tool] = &UsageTotals{}
			}
			agents[tool].add(*message.Turn)
		}
		c.mu.Unlock()
	}
	history.Projects = groups(projects)
	history.Agents = groups(agents)
	return history, nil
}

// groups lists totals by time worked, most first.
func groups(totals map[string]*UsageTotals) []UsageGroup {
	list := []UsageGroup{}
	for key, total := range totals {
		list = append(list, UsageGroup{Key: key, UsageTotals: *total})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Ms != list[j].Ms {
			return list[i].Ms > list[j].Ms
		}
		return list[i].Key < list[j].Key
	})
	return list
}

// UsageHistory adds up the time, tokens and estimated cost of the turns run
// in ais over the last days days, from what was saved with each turn.
func (a *App) UsageHistory(days int) (UsageHistory, error) {
	if err := a.chatReady(); err != nil {
		return UsageHistory{}, err
	}
	return a.chats.usageHistory(days, time.Now())
}
