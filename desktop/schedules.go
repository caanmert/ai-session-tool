package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/caanmert/ai-session-tool/internal/application"
	"github.com/caanmert/ai-session-tool/internal/launch"
	"github.com/google/uuid"
)

// A scheduled task sends a saved prompt to an agent at a set time: a new
// conversation in the task's project, titled with the task's name. Tasks run
// only while ais is open. An occurrence that passes while ais is closed, or
// that ais could not reach on time, is recorded as missed and never made up,
// so opening ais never starts a turn. Every task is created off and is turned
// on explicitly. A run asks for approvals like any conversation; nothing is
// approved for it.

// Schedule is one scheduled task.
type Schedule struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Project string `json:"project"`
	Tool    string `json:"tool"`
	Prompt  string `json:"prompt"`
	// Mode is the permission mode of each run: plan, or the agent's default.
	Mode string `json:"mode"`
	// Days is daily, weekdays, or one weekday: mon, tue … sun.
	Days string `json:"days"`
	// Time is the local time of day, HH:MM.
	Time    string `json:"time"`
	Enabled bool   `json:"enabled"`
	// Since is when the task was last turned on or changed. Times before it
	// are not considered.
	Since string `json:"since,omitempty"`
	// LastAt is the scheduled time last considered, and LastResult what came
	// of it: started, missed or skipped, with LastNote saying why.
	LastAt     string `json:"lastAt,omitempty"`
	LastResult string `json:"lastResult,omitempty"`
	LastNote   string `json:"lastNote,omitempty"`
	// LastChat is the conversation the latest run opened.
	LastChat string `json:"lastChat,omitempty"`
	// Next is when the task runs next, while on. Worked out on each read.
	Next string `json:"next,omitempty"`
}

const (
	scheduleLimit     = 50
	scheduleNameLimit = 80
	// scheduleGrace is how late a run may start, for example after the Mac
	// wakes. Later than that, the time counts as missed.
	scheduleGrace = 10 * time.Minute
	scheduleTick  = 30 * time.Second
)

var weekdays = map[string]time.Weekday{"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday, "thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday}

func scheduleDay(days string, day time.Weekday) bool {
	switch days {
	case "daily":
		return true
	case "weekdays":
		return day != time.Saturday && day != time.Sunday
	}
	want, ok := weekdays[days]
	return ok && want == day
}

func scheduleClock(value string) (hour, minute int, err error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil || len(value) != 5 {
		return 0, 0, errors.New("choose a time of day such as 09:30")
	}
	return parsed.Hour(), parsed.Minute(), nil
}

// nextRun is the first time the task is due strictly after after, in after's
// time zone. A clock time that a daylight saving change skips moves forward.
func nextRun(s Schedule, after time.Time) time.Time {
	hour, minute, err := scheduleClock(s.Time)
	if err != nil {
		return time.Time{}
	}
	for k := 0; k <= 8; k++ {
		candidate := time.Date(after.Year(), after.Month(), after.Day()+k, hour, minute, 0, 0, after.Location())
		// Go may place a skipped clock time an hour early; run after the gap.
		if candidate.Hour()*60+candidate.Minute() < hour*60+minute {
			candidate = candidate.Add(time.Hour)
		}
		if candidate.After(after) && scheduleDay(s.Days, candidate.Weekday()) {
			return candidate
		}
	}
	return time.Time{}
}

func parseStamp(value string) time.Time {
	t, _ := time.Parse(time.RFC3339, value)
	return t
}

// checkSchedule validates a task and tidies its text.
func checkSchedule(s *Schedule) error {
	s.Name, s.Prompt = strings.TrimSpace(s.Name), strings.TrimSpace(s.Prompt)
	switch {
	case s.Name == "":
		return errors.New("give the task a name")
	case utf8.RuneCountInString(s.Name) > scheduleNameLimit:
		return fmt.Errorf("a task's name is limited to %d characters", scheduleNameLimit)
	case s.Prompt == "":
		return errors.New("write the message the task sends")
	case len(s.Prompt) > promptTextLimit:
		return errors.New("a task's message is limited to 20,000 characters")
	case s.Tool != "claude" && s.Tool != "codex":
		return errors.New("choose Codex or Claude Code")
	case s.Mode != modePlan && s.Mode != modeDefault:
		return errors.New("a scheduled task runs in Plan mode or the agent's default mode")
	}
	if _, day := weekdays[s.Days]; !day && s.Days != "daily" && s.Days != "weekdays" {
		return errors.New("choose every day, weekdays, or one day of the week")
	}
	if _, _, err := scheduleClock(s.Time); err != nil {
		return err
	}
	dir, err := application.Directory(s.Project)
	if err != nil {
		return err
	}
	s.Project = dir
	return nil
}

// scheduler keeps the tasks and starts them when due.
type scheduler struct {
	mu   sync.Mutex
	path string
	list []Schedule
	// started is when ais started; a time before it was missed.
	started time.Time
	now     func() time.Time
	// run starts a task's conversation and returns its ID.
	run func(Schedule) (string, error)
	// busy says whether a conversation is still working.
	busy func(chat string) bool
}

func newScheduler(path string, now func() time.Time) *scheduler {
	return &scheduler{path: path, list: []Schedule{}, started: now(), now: now}
}

func (s *scheduler) load() error {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var list []Schedule
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("scheduled tasks could not be read: %w", err)
	}
	if list == nil {
		list = []Schedule{}
	}
	s.mu.Lock()
	s.list = list
	s.mu.Unlock()
	return nil
}

// save writes the list; the caller holds mu.
func (s *scheduler) save() error {
	for i := range s.list {
		s.list[i].Next = ""
	}
	data, err := json.MarshalIndent(s.list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(s.path), ".schedules-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), s.path)
}

func (s *scheduler) find(id string) int {
	for i := range s.list {
		if s.list[i].ID == id {
			return i
		}
	}
	return -1
}

// view lists the tasks with their next run.
func (s *scheduler) view() []Schedule {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	list := append([]Schedule{}, s.list...)
	for i := range list {
		list[i].Next = ""
		if list[i].Enabled {
			after := now
			if since := parseStamp(list[i].Since); since.After(after) {
				after = since
			}
			if next := nextRun(list[i], after); !next.IsZero() {
				list[i].Next = next.Format(time.RFC3339)
			}
		}
	}
	return list
}

// put adds a task, off, or changes one. A change to a task that is on
// counts from now, so it does not run for a time already passed.
func (s *scheduler) put(task Schedule) (Schedule, error) {
	if err := checkSchedule(&task); err != nil {
		return Schedule{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.find(task.ID)
	if task.ID == "" || i < 0 {
		if len(s.list) >= scheduleLimit {
			return Schedule{}, fmt.Errorf("keep at most %d scheduled tasks", scheduleLimit)
		}
		task = Schedule{ID: uuid.NewString(), Name: task.Name, Project: task.Project, Tool: task.Tool, Prompt: task.Prompt, Mode: task.Mode, Days: task.Days, Time: task.Time}
		s.list = append(s.list, task)
	} else {
		old := s.list[i]
		task.Enabled, task.LastAt, task.LastResult, task.LastNote, task.LastChat = old.Enabled, old.LastAt, old.LastResult, old.LastNote, old.LastChat
		task.Since = old.Since
		if task.Enabled {
			task.Since = s.now().Format(time.RFC3339)
		}
		s.list[i] = task
	}
	if err := s.save(); err != nil {
		return Schedule{}, err
	}
	return task, nil
}

func (s *scheduler) remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.find(id)
	if i < 0 {
		return errors.New("that scheduled task no longer exists")
	}
	s.list = append(s.list[:i], s.list[i+1:]...)
	return s.save()
}

// enable turns a task on or off. On counts from now.
func (s *scheduler) enable(id string, on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.find(id)
	if i < 0 {
		return errors.New("that scheduled task no longer exists")
	}
	s.list[i].Enabled = on
	if on {
		s.list[i].Since = s.now().Format(time.RFC3339)
	}
	return s.save()
}

// record notes what came of a scheduled time.
func (s *scheduler) record(id string, at time.Time, result, note, chat string) {
	i := s.find(id)
	if i < 0 {
		return
	}
	task := &s.list[i]
	task.LastAt = at.Format(time.RFC3339)
	task.LastResult, task.LastNote = result, note
	if chat != "" {
		task.LastChat = chat
	}
}

// start runs a task now and records it; the caller does not hold mu.
func (s *scheduler) start(task Schedule, at time.Time) (string, error) {
	if task.LastChat != "" && s.busy != nil && s.busy(task.LastChat) {
		err := errors.New("the previous run is still going")
		s.mu.Lock()
		s.record(task.ID, at, "skipped", "The previous run was still going.", "")
		_ = s.save()
		s.mu.Unlock()
		return "", err
	}
	chat, err := s.run(task)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.record(task.ID, at, "skipped", "Not started: "+err.Error(), "")
	} else {
		s.record(task.ID, at, "started", "", chat)
	}
	_ = s.save()
	return chat, err
}

// runNow starts a task at once, on or off.
func (s *scheduler) runNow(id string) (string, error) {
	s.mu.Lock()
	i := s.find(id)
	if i < 0 {
		s.mu.Unlock()
		return "", errors.New("that scheduled task no longer exists")
	}
	task := s.list[i]
	s.mu.Unlock()
	return s.start(task, s.now())
}

// tick starts the tasks that are due and records the ones that were missed.
func (s *scheduler) tick() {
	s.mu.Lock()
	now := s.now()
	type due struct {
		task Schedule
		at   time.Time
	}
	runs := []due{}
	changed := false
	for _, task := range s.list {
		if !task.Enabled {
			continue
		}
		after := parseStamp(task.Since)
		if last := parseStamp(task.LastAt); last.After(after) {
			after = last
		}
		at := nextRun(task, after)
		if at.IsZero() || at.After(now) {
			continue
		}
		// Only the latest time that has passed counts; earlier ones are gone.
		for n := 0; n < 400; n++ {
			next := nextRun(task, at)
			if next.IsZero() || next.After(now) {
				break
			}
			at = next
		}
		switch {
		case at.Before(s.started):
			s.record(task.ID, at, "missed", "ais was not open.", "")
			changed = true
		case now.Sub(at) > scheduleGrace:
			s.record(task.ID, at, "missed", "ais could not start it on time, for example while the Mac was asleep.", "")
			changed = true
		default:
			runs = append(runs, due{task, at})
		}
	}
	if changed {
		_ = s.save()
	}
	s.mu.Unlock()
	for _, r := range runs {
		_, _ = s.start(r.task, r.at)
	}
}

// loop checks the tasks until ctx ends.
func (s *scheduler) loop(ctx context.Context) {
	ticker := time.NewTicker(scheduleTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

// runSchedule opens a task's conversation and sends its message.
func (m *chatManager) runSchedule(task Schedule) (ChatState, error) {
	state, err := m.create(task.Project, task.Tool, "", "", nil)
	if err != nil {
		return ChatState{}, err
	}
	c, err := m.get(state.ID)
	if err != nil {
		return ChatState{}, err
	}
	c.mu.Lock()
	c.state.Title = task.Name
	c.state.Mode = task.Mode
	c.state.Schedule = task.Name
	c.change()
	c.mu.Unlock()
	if err := m.save(c); err != nil {
		_ = m.remove(state.ID)
		return ChatState{}, err
	}
	if err := m.send(state.ID, task.Prompt); err != nil {
		_ = m.remove(state.ID)
		return ChatState{}, err
	}
	c.mu.Lock()
	state = cloneState(c.state)
	c.mu.Unlock()
	return state, nil
}

// chatBusy says whether a conversation is still working.
func (m *chatManager) chatBusy(id string) bool {
	c, err := m.get(id)
	if err != nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return busyChat(c.state.Status)
}

func (a *App) schedulesReady() error {
	if err := a.chatReady(); err != nil {
		return err
	}
	if a.schedules == nil {
		return errors.New("scheduled tasks are starting")
	}
	return a.scheduleError
}

// Schedules lists the scheduled tasks with their next run.
func (a *App) Schedules() ([]Schedule, error) {
	if err := a.schedulesReady(); err != nil {
		return nil, err
	}
	return a.schedules.view(), nil
}

// SaveSchedule adds a task, off, or changes one. It never turns a task on.
func (a *App) SaveSchedule(task Schedule) (Schedule, error) {
	if err := a.schedulesReady(); err != nil {
		return Schedule{}, err
	}
	return a.schedules.put(task)
}

// SetScheduleEnabled turns a task on or off. On, it runs from its next time.
func (a *App) SetScheduleEnabled(id string, enabled bool) error {
	if err := a.schedulesReady(); err != nil {
		return err
	}
	return a.schedules.enable(id, enabled)
}

// DeleteSchedule removes a task. Conversations it opened stay.
func (a *App) DeleteSchedule(id string) error {
	if err := a.schedulesReady(); err != nil {
		return err
	}
	return a.schedules.remove(id)
}

// startSchedule is the scheduler's way to run a task: it checks the agent
// can start in the project, opens the conversation and tells the window.
func (a *App) startSchedule(task Schedule) (string, error) {
	cmd, err := a.service.NewSession(task.Tool, task.Project)
	if err != nil {
		return "", err
	}
	if err = launch.Check(cmd); err != nil {
		return "", err
	}
	state, err := a.chats.runSchedule(task)
	if err != nil {
		return "", err
	}
	if a.emit != nil {
		a.emit("schedule:started", state)
	}
	return state.ID, nil
}

// RunSchedule runs a task now and returns the conversation it opened.
func (a *App) RunSchedule(id string) (ChatState, error) {
	if err := a.schedulesReady(); err != nil {
		return ChatState{}, err
	}
	chat, err := a.schedules.runNow(id)
	if err != nil {
		return ChatState{}, err
	}
	c, err := a.chats.get(chat)
	if err != nil {
		return ChatState{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return cloneState(c.state), nil
}
