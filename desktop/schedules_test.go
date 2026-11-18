package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNextRun(t *testing.T) {
	zone := time.FixedZone("UTC+3", 3*3600)
	friday := time.Date(2026, 11, 20, 18, 0, 0, 0, zone) // a Friday
	cases := []struct {
		days, at string
		after    time.Time
		want     time.Time
	}{
		{"daily", "18:00", friday.Add(-time.Minute), friday},
		{"daily", "18:00", friday, friday.AddDate(0, 0, 1)},
		{"weekdays", "18:00", friday, friday.AddDate(0, 0, 3)},
		{"sun", "09:30", friday, time.Date(2026, 11, 22, 9, 30, 0, 0, zone)},
		{"fri", "18:00", friday, friday.AddDate(0, 0, 7)},
	}
	for _, c := range cases {
		if got := nextRun(Schedule{Days: c.days, Time: c.at}, c.after); !got.Equal(c.want) {
			t.Fatalf("%s %s after %v: %v, want %v", c.days, c.at, c.after, got, c.want)
		}
	}
	if !nextRun(Schedule{Days: "daily", Time: "bad"}, friday).IsZero() {
		t.Fatal("a bad time never runs")
	}
	york, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no time zone data")
	}
	// 02:30 does not exist on 8 March 2026 in New York; the run moves on
	// rather than going back an hour, and stays a day after the one before.
	before := time.Date(2026, 3, 7, 2, 30, 0, 0, york)
	spring := nextRun(Schedule{Days: "daily", Time: "02:30"}, before)
	if spring.Day() != 8 || spring.Hour() != 3 || spring.Minute() != 30 {
		t.Fatalf("spring forward: %v", spring)
	}
	// 18:00 the day after the change is still 18:00 local.
	if got := nextRun(Schedule{Days: "daily", Time: "18:00"}, time.Date(2026, 3, 8, 12, 0, 0, 0, york)); got.Hour() != 18 || got.Day() != 8 {
		t.Fatalf("after the change: %v", got)
	}
}

func TestCheckSchedule(t *testing.T) {
	project := t.TempDir()
	good := Schedule{Name: " Evening review ", Project: project, Tool: "codex", Prompt: " Review today's changes ", Mode: modePlan, Days: "weekdays", Time: "18:00"}
	task := good
	if err := checkSchedule(&task); err != nil || task.Name != "Evening review" || task.Prompt != "Review today's changes" {
		t.Fatalf("good task: %v %+v", err, task)
	}
	for name, change := range map[string]func(*Schedule){
		"name":    func(s *Schedule) { s.Name = " " },
		"prompt":  func(s *Schedule) { s.Prompt = "" },
		"tool":    func(s *Schedule) { s.Tool = "other" },
		"mode":    func(s *Schedule) { s.Mode = modeAutoEdit },
		"days":    func(s *Schedule) { s.Days = "someday" },
		"time":    func(s *Schedule) { s.Time = "25:00" },
		"short":   func(s *Schedule) { s.Time = "9:30" },
		"project": func(s *Schedule) { s.Project = filepath.Join(project, "missing") },
	} {
		task := good
		change(&task)
		if err := checkSchedule(&task); err == nil {
			t.Fatalf("%s: a bad task was accepted", name)
		}
	}
}

type fakeClock struct{ now time.Time }

func (c *fakeClock) get() time.Time { return c.now }

func TestSchedulerRunsOnlyWhileOpen(t *testing.T) {
	zone := time.FixedZone("UTC+3", 3*3600)
	clock := &fakeClock{now: time.Date(2026, 11, 18, 17, 0, 0, 0, zone)}
	path := filepath.Join(t.TempDir(), "schedules.json")
	runs := []string{}
	busy := map[string]bool{}
	var fail error
	setup := func() *scheduler {
		s := newScheduler(path, clock.get)
		if err := s.load(); err != nil {
			t.Fatal(err)
		}
		s.run = func(task Schedule) (string, error) {
			if fail != nil {
				return "", fail
			}
			runs = append(runs, task.Name)
			return "chat-" + clock.now.Format("02-1504"), nil
		}
		s.busy = func(chat string) bool { return busy[chat] }
		return s
	}
	s := setup()
	task, err := s.put(Schedule{Name: "Evening review", Project: t.TempDir(), Tool: "codex", Prompt: "Review", Mode: modePlan, Days: "daily", Time: "18:00"})
	if err != nil {
		t.Fatal(err)
	}
	if task.Enabled || task.ID == "" {
		t.Fatalf("a new task starts off: %+v", task)
	}
	at := func(day, hour, minute int) {
		clock.now = time.Date(2026, 11, day, hour, minute, 5, 0, zone)
		s.tick()
	}
	last := func() Schedule { return s.view()[0] }
	at(18, 18, 0)
	if len(runs) != 0 {
		t.Fatal("a task that is off never runs")
	}
	if err := s.enable(task.ID, true); err != nil {
		t.Fatal(err)
	}
	// Turned on after 18:00: the next run is tomorrow, not now.
	if next := last().Next; !strings.HasPrefix(next, "2026-11-19T18:00:00") {
		t.Fatalf("next: %s", next)
	}
	at(18, 18, 1)
	at(19, 17, 59)
	if len(runs) != 0 {
		t.Fatalf("ran early: %v", runs)
	}
	at(19, 18, 0)
	at(19, 18, 2)
	if len(runs) != 1 || last().LastResult != "started" || last().LastChat != "chat-19-1800" {
		t.Fatalf("one run on time: %v %+v", runs, last())
	}
	// The Mac slept through the next one: it is missed, not run late.
	at(20, 18, 30)
	if len(runs) != 1 || last().LastResult != "missed" || !strings.Contains(last().LastNote, "on time") {
		t.Fatalf("late: %v %+v", runs, last())
	}
	// The previous run is still going: skipped.
	busy["chat-19-1800"] = true
	at(21, 18, 0)
	if len(runs) != 1 || last().LastResult != "skipped" {
		t.Fatalf("busy: %v %+v", runs, last())
	}
	busy = map[string]bool{}
	// An agent that cannot start: skipped, with the reason.
	fail = errors.New("eight agents are already running")
	at(22, 18, 0)
	if last().LastResult != "skipped" || !strings.Contains(last().LastNote, "eight agents") {
		t.Fatalf("failed start: %+v", last())
	}
	fail = nil
	// ais closes before 18:00 on the 23rd and opens again at 18:01: the run
	// is missed, and opening ais starts nothing.
	clock.now = time.Date(2026, 11, 23, 18, 1, 0, 0, zone)
	s = setup()
	s.tick()
	if len(runs) != 1 || last().LastResult != "missed" || last().LastNote != "ais was not open." {
		t.Fatalf("after a restart: %v %+v", runs, last())
	}
	at(24, 18, 0)
	if len(runs) != 2 {
		t.Fatalf("runs again the next day: %v", runs)
	}
	// Changing a task that is on counts from the change.
	clock.now = time.Date(2026, 11, 25, 18, 5, 0, 0, zone)
	changed := last()
	changed.Time = "18:03"
	if _, err := s.put(changed); err != nil {
		t.Fatal(err)
	}
	s.tick()
	if len(runs) != 2 {
		t.Fatalf("a changed time already past must not run: %v", runs)
	}
	// Run now works whether on or off.
	if err := s.enable(task.ID, false); err != nil {
		t.Fatal(err)
	}
	if chat, err := s.runNow(task.ID); err != nil || chat == "" || len(runs) != 3 {
		t.Fatalf("run now: %q %v %v", chat, err, runs)
	}
	if err := s.remove(task.ID); err != nil || len(s.view()) != 0 {
		t.Fatalf("remove: %v", err)
	}
	if err := s.remove(task.ID); err == nil {
		t.Fatal("removing twice must fail")
	}
}

func TestScheduledConversation(t *testing.T) {
	m := fakeChats(t, "plan")
	project := t.TempDir()
	state, err := m.runSchedule(Schedule{Name: "Evening review", Project: project, Tool: "codex", Prompt: "Review today's changes", Mode: modePlan})
	if err != nil {
		t.Fatal(err)
	}
	if state.Title != "Evening review" || state.Schedule != "Evening review" || state.Mode != modePlan {
		t.Fatalf("scheduled chat: %+v", state)
	}
	done := waitChat(t, m, state.ID, func(s ChatState) bool { return s.Status == "idle" && len(s.Messages) >= 2 })
	if done.Messages[0].Text != "Review today's changes" || done.Title != "Evening review" {
		t.Fatalf("sent: %+v", done.Messages)
	}
	if m.chatBusy(state.ID) || m.chatBusy("missing") {
		t.Fatal("busy check")
	}
}
