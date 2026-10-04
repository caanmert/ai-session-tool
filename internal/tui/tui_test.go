package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/caanmert/ai-session-tool/internal/model"
	"github.com/caanmert/ai-session-tool/internal/provider"
	"github.com/caanmert/ai-session-tool/internal/provider/claude"
	"github.com/caanmert/ai-session-tool/internal/provider/codex"
	"github.com/caanmert/ai-session-tool/internal/ui"
)

var testNow = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

type harness struct {
	t      *testing.T
	m      Model
	execs  []*exec.Cmd
	copied []string
}

func fixtureProviders(t *testing.T) []provider.Provider {
	root, err := filepath.Abs("../../testdata")
	if err != nil {
		t.Fatal(err)
	}
	cp := codex.New(filepath.Join(root, "codex"))
	cp.OpenFiles = func(context.Context) ([]byte, error) { return nil, nil }
	return []provider.Provider{claude.New(filepath.Join(root, "claude")), cp}
}

// start builds a model over providers, sizes it and completes the scan.
func start(t *testing.T, providers []provider.Provider, w, h int) *harness {
	t.Helper()
	hs := &harness{t: t}
	hs.m = New(Deps{
		Providers: providers,
		Now:       func() time.Time { return testNow },
		Theme:     ui.New(nil, ui.Never),
		Exec: func(c *exec.Cmd, cb tea.ExecCallback) tea.Cmd {
			hs.execs = append(hs.execs, c)
			return func() tea.Msg { return cb(nil) }
		},
		Copy: func(s string) error { hs.copied = append(hs.copied, s); return nil },
	})
	hs.send(tea.WindowSizeMsg{Width: w, Height: h})
	hs.send(hs.m.scan()())
	return hs
}

func (h *harness) send(msg tea.Msg) tea.Cmd {
	h.t.Helper()
	next, cmd := h.m.Update(msg)
	h.m = next.(Model)
	return cmd
}

func (h *harness) keys(s string) {
	h.t.Helper()
	for _, r := range s {
		h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func (h *harness) key(t tea.KeyType) tea.Cmd { return h.send(tea.KeyMsg{Type: t}) }

// loadPreview delivers the transcript of the selected session.
func (h *harness) loadPreview() {
	h.t.Helper()
	s, ok := h.m.current()
	if !ok {
		h.t.Fatal("nothing selected")
	}
	cmd := h.send(selectMsg{s.ID})
	if cmd == nil {
		return // already cached
	}
	h.send(cmd())
}

func (h *harness) selected() string {
	s, _ := h.m.current()
	return s.ID
}

func TestInitialView(t *testing.T) {
	h := start(t, fixtureProviders(t), 120, 30)
	view := h.m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 30 {
		t.Fatalf("view has %d lines, want 30", len(lines))
	}
	for _, want := range []string{"8 sessions", "claude 3", "codex 5", "> ", "Webhook retries", "Dark mode", "enter resume"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if !strings.HasPrefix(lines[1], "> ") || !strings.Contains(lines[1], "Webhook retries") {
		t.Errorf("newest session should be selected first: %q", lines[1])
	}
	if !strings.Contains(view, "loading transcript") {
		t.Error("preview should say it is loading before the transcript arrives")
	}
	h.loadPreview()
	if view := h.m.View(); !strings.Contains(view, "Add retries to the webhook sender.") {
		t.Errorf("preview lacks the transcript:\n%s", view)
	}
}

func TestViewFitsEveryTerminalSize(t *testing.T) {
	for _, size := range [][2]int{{50, 8}, {60, 10}, {80, 24}, {120, 30}, {200, 60}} {
		h := start(t, fixtureProviders(t), size[0], size[1])
		h.loadPreview()
		lines := strings.Split(h.m.View(), "\n")
		if len(lines) != size[1] {
			t.Errorf("%dx%d: %d lines", size[0], size[1], len(lines))
		}
		for i, l := range lines {
			if w := ui.Width(l); w > size[0] {
				t.Errorf("%dx%d: line %d is %d wide: %q", size[0], size[1], i, w, l)
			}
		}
	}
}

func TestNavigation(t *testing.T) {
	h := start(t, fixtureProviders(t), 120, 30)
	order := []string{"aaaaaaaa", "99999999", "22222222", "11111111", "44444444", "cccccccc", "bbbbbbbb", "dddddddd"}
	for i, want := range order {
		if !strings.HasPrefix(h.selected(), want) {
			t.Fatalf("step %d: selected %s, want %s", i, h.selected(), want)
		}
		h.keys("j")
	}
	h.keys("j") // stays on the last row
	if !strings.HasPrefix(h.selected(), "dddddddd") {
		t.Errorf("moved past the end: %s", h.selected())
	}
	h.keys("g")
	if !strings.HasPrefix(h.selected(), "aaaaaaaa") {
		t.Errorf("g should go to the first row: %s", h.selected())
	}
	h.keys("G")
	if !strings.HasPrefix(h.selected(), "dddddddd") {
		t.Errorf("G should go to the last row: %s", h.selected())
	}
}

func TestFilter(t *testing.T) {
	h := start(t, fixtureProviders(t), 120, 30)
	h.keys("/t:claude")
	if !h.m.filtering || len(h.m.view) != 3 {
		t.Fatalf("filtering %v, %d rows, want 3 claude rows", h.m.filtering, len(h.m.view))
	}
	if view := h.m.View(); !strings.Contains(view, "3 of 8 sessions") || strings.Contains(view, "Webhook retries") {
		t.Errorf("filtered view:\n%s", view)
	}
	h.key(tea.KeyEnter) // keep the filter, leave the input
	if h.m.filtering || len(h.m.view) != 3 || !strings.Contains(h.m.View(), "filter: t:claude") {
		t.Errorf("enter should keep the filter")
	}
	h.key(tea.KeyEsc) // clear it
	if len(h.m.view) != 8 {
		t.Errorf("esc should clear the filter, %d rows", len(h.m.view))
	}

	// Typed quickly, "/" and the query arrive as one event.
	h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/terraform")})
	if !h.m.filtering || len(h.m.view) != 1 || !strings.HasPrefix(h.selected(), "dddddddd") {
		t.Errorf("burst filter: filtering %v, rows %d, selected %s", h.m.filtering, len(h.m.view), h.selected())
	}

	h.key(tea.KeyEsc)
	h.keys("/nothing-matches")
	if !strings.Contains(h.m.View(), "no sessions match") {
		t.Error("empty result should say so")
	}
}

func TestFilterKeepsSelection(t *testing.T) {
	h := start(t, fixtureProviders(t), 120, 30)
	h.keys("jj") // 22222222, a claude session
	h.keys("/t:claude")
	if !strings.HasPrefix(h.selected(), "22222222") {
		t.Errorf("selection should stay on the same session when it still matches: %s", h.selected())
	}
}

// memProvider serves fixed sessions, for testing the launch keys.
type memProvider struct{ sessions []model.Session }

func (p memProvider) Tool() model.Tool { return model.ToolCodex }
func (p memProvider) Root() string     { return "" }
func (p memProvider) Discover(context.Context) ([]provider.FileRef, error) {
	var refs []provider.FileRef
	for _, s := range p.sessions {
		refs = append(refs, provider.FileRef{Path: s.ID})
	}
	return refs, nil
}
func (p memProvider) Parse(_ context.Context, f provider.FileRef) (model.Session, []provider.Warning, error) {
	for _, s := range p.sessions {
		if s.ID == f.Path {
			return s, nil, nil
		}
	}
	return model.Session{}, nil, provider.ErrEmpty
}
func (p memProvider) Transcript(context.Context, model.Session) ([]model.Message, error) {
	return []model.Message{{Role: model.RoleUser, Kind: model.KindText, Text: "hello"}}, nil
}
func (p memProvider) Live(context.Context) ([]model.LiveState, error) { return nil, nil }
func (p memProvider) ResumeCmd(s model.Session, fork bool) (*exec.Cmd, error) {
	sub := "resume"
	if fork {
		sub = "fork"
	}
	return fakeCmd(s.CWD, "codex", sub, s.ID), nil
}
func (p memProvider) NewCmd(cwd string) *exec.Cmd { return fakeCmd(cwd, "codex") }

// fakeCmd builds a command without a PATH lookup; tests never run it.
func fakeCmd(dir string, args ...string) *exec.Cmd {
	return &exec.Cmd{Path: "/bin/" + args[0], Args: args, Dir: dir}
}

func TestLaunchKeys(t *testing.T) {
	dir := t.TempDir()
	p := memProvider{sessions: []model.Session{
		{Tool: model.ToolCodex, ID: "here", Title: "Has a directory", CWD: dir, UpdatedAt: testNow},
		{Tool: model.ToolCodex, ID: "gone", Title: "Directory deleted", CWD: filepath.Join(dir, "deleted"), UpdatedAt: testNow.Add(-time.Hour)},
	}}
	h := start(t, []provider.Provider{p}, 100, 30)

	tests := []struct {
		key  string
		args string
	}{
		{"enter", "codex resume here"},
		{"f", "codex fork here"},
		{"n", "codex"},
	}
	for _, tt := range tests {
		h.execs = nil
		var cmd tea.Cmd
		if tt.key == "enter" {
			cmd = h.key(tea.KeyEnter)
		} else {
			cmd = h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
		}
		if len(h.execs) != 1 {
			t.Fatalf("%s: %d commands launched", tt.key, len(h.execs))
		}
		if got := strings.Join(h.execs[0].Args, " "); got != tt.args || h.execs[0].Dir != dir {
			t.Errorf("%s: ran %q in %q, want %q in %q", tt.key, got, h.execs[0].Dir, tt.args, dir)
		}
		// When the agent exits, the TUI rescans and says so.
		h.send(cmd())
		if !h.m.loading || h.m.status != "back from the session" {
			t.Errorf("%s: after exit loading=%v status=%q", tt.key, h.m.loading, h.m.status)
		}
		h.send(h.m.scan()())
	}

	h.keys("y")
	if len(h.copied) != 1 || h.copied[0] != "cd "+dir+" && codex resume here" {
		t.Errorf("copied %q", h.copied)
	}

	h.keys("j") // the session whose directory is gone
	h.execs = nil
	h.key(tea.KeyEnter)
	if len(h.execs) != 0 || !strings.Contains(h.m.status, "project directory is gone") || !h.m.statusError {
		t.Errorf("missing directory: execs %d, status %q", len(h.execs), h.m.status)
	}
}

func TestPreviewFocusAndHelp(t *testing.T) {
	h := start(t, fixtureProviders(t), 120, 30)
	h.loadPreview()
	h.key(tea.KeyTab)
	if h.m.focus != focusPreview || !strings.Contains(h.m.View(), "scrolling") {
		t.Fatal("tab should focus the preview")
	}
	before := h.selected()
	h.keys("jjj")
	if h.selected() != before {
		t.Error("j in the preview must scroll it, not move the selection")
	}
	h.key(tea.KeyTab)
	if h.m.focus != focusList {
		t.Error("tab should return to the list")
	}

	h.keys("?")
	if view := h.m.View(); !strings.Contains(view, "── keys") || !strings.Contains(view, "is:live") {
		t.Errorf("help view:\n%s", view)
	}
	h.key(tea.KeyEsc)
	if h.m.help {
		t.Error("esc should close the help")
	}
}

func TestQuit(t *testing.T) {
	h := start(t, fixtureProviders(t), 120, 30)
	cmd := h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("q returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q should quit")
	}
}

func TestCachedLoadThenSync(t *testing.T) {
	full, err := provider.Scan(context.Background(), fixtureProviders(t))
	if err != nil {
		t.Fatal(err)
	}
	cached := provider.ScanResult{Sessions: full.Sessions[2:3]} // a stale cache with one session

	h := &harness{t: t}
	h.m = New(Deps{Providers: fixtureProviders(t), Now: func() time.Time { return testNow }, Theme: ui.New(nil, ui.Never)})
	h.send(tea.WindowSizeMsg{Width: 120, Height: 30})

	next := h.send(scanMsg{res: cached, cached: true})
	if next == nil || !h.m.loading {
		t.Fatal("a cached load must be followed by a sync")
	}
	if view := h.m.View(); !strings.Contains(view, "1 sessions") || !strings.Contains(view, "Dark mode") {
		t.Errorf("cached sessions should show at once:\n%s", view)
	}
	h.send(scanMsg{res: full})
	if h.m.loading || len(h.m.view) != 8 {
		t.Errorf("after sync: loading %v, %d rows", h.m.loading, len(h.m.view))
	}
	if !strings.HasPrefix(h.selected(), "22222222") {
		t.Errorf("selection should stay on the session shown from cache: %s", h.selected())
	}

	// An empty cache (first run) keeps "scanning" until the sync lands.
	h2 := &harness{t: t}
	h2.m = New(Deps{Providers: fixtureProviders(t), Theme: ui.New(nil, ui.Never)})
	h2.send(tea.WindowSizeMsg{Width: 120, Height: 30})
	h2.send(scanMsg{cached: true})
	if h2.m.scanned || !strings.Contains(h2.m.View(), "scanning sessions") {
		t.Error("empty cache should not show an empty list")
	}
}

func TestFilterMatchesConversationText(t *testing.T) {
	var asked []string
	h := &harness{t: t}
	h.m = New(Deps{
		Providers: fixtureProviders(t),
		Now:       func() time.Time { return testNow },
		Theme:     ui.New(nil, ui.Never),
		Search: func(_ context.Context, words string) (map[string]bool, error) {
			asked = append(asked, words)
			if words == "delay" {
				return map[string]bool{"codex/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa": true}, nil
			}
			return nil, nil
		},
	})
	h.send(tea.WindowSizeMsg{Width: 120, Height: 30})
	h.send(h.m.scan()())

	h.keys("/delay") // only in a reply, not in any title or prompt
	if len(h.m.view) != 0 {
		t.Fatalf("metadata alone should not match: %d rows", len(h.m.view))
	}
	cmd := h.send(searchDueMsg{"delay"}) // the debounce fired
	h.send(cmd())
	if len(asked) != 1 || len(h.m.view) != 1 || !strings.HasPrefix(h.selected(), "aaaaaaaa") {
		t.Fatalf("asked %v, rows %d, selected %s", asked, len(h.m.view), h.selected())
	}
	if !strings.Contains(h.m.View(), "1 of 8 sessions (1 by conversation)") {
		t.Errorf("header should explain the match:\n%s", h.m.View())
	}

	// A stale result (the query moved on) is ignored.
	h.keys("x")
	h.send(searchMsg{text: "delay", hits: map[string]bool{"codex/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa": true}})
	if len(h.m.view) != 0 {
		t.Errorf("stale search result applied: %d rows", len(h.m.view))
	}

	// Structured parts still apply to conversation matches.
	h.key(tea.KeyEsc)
	h.keys("/t:claude delay")
	h.send(searchMsg{text: "delay", hits: map[string]bool{"codex/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa": true}})
	if len(h.m.view) != 0 {
		t.Errorf("t:claude must exclude a codex conversation match: %d rows", len(h.m.view))
	}
}

// fakeActions records calls and can fail on demand.
type fakeActions struct {
	calls []string
	fail  error
}

func (f *fakeActions) Rename(_ context.Context, s model.Session, title string) error {
	f.calls = append(f.calls, "rename "+s.ID[:8]+" "+title)
	return f.fail
}
func (f *fakeActions) Tag(_ context.Context, s model.Session, add, remove []string) error {
	f.calls = append(f.calls, "tag "+s.ID[:8]+" +"+strings.Join(add, ",")+" -"+strings.Join(remove, ","))
	return f.fail
}
func (f *fakeActions) SetPinned(_ context.Context, s model.Session, on bool) error {
	f.calls = append(f.calls, fmt.Sprintf("pin %s %v", s.ID[:8], on))
	return f.fail
}
func (f *fakeActions) SetArchived(_ context.Context, s model.Session, on bool) error {
	f.calls = append(f.calls, fmt.Sprintf("archive %s %v", s.ID[:8], on))
	return f.fail
}
func (f *fakeActions) Trash(_ context.Context, s model.Session) error {
	f.calls = append(f.calls, "trash "+s.ID[:8])
	return f.fail
}

// run executes a command and feeds its message back, as Bubble Tea would.
func (h *harness) run(cmd tea.Cmd) {
	h.t.Helper()
	if cmd == nil {
		h.t.Fatal("expected a command")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				if m, ok := c().(actionDoneMsg); ok {
					h.send(m)
				}
			}
		}
		return
	}
	h.send(msg)
}

func startWithActions(t *testing.T) (*harness, *fakeActions) {
	acts := &fakeActions{}
	h := &harness{t: t}
	h.m = New(Deps{Providers: fixtureProviders(t), Now: func() time.Time { return testNow }, Theme: ui.New(nil, ui.Never), Actions: acts})
	h.send(tea.WindowSizeMsg{Width: 120, Height: 30})
	h.send(h.m.scan()())
	return h, acts
}

func TestOrganizeKeys(t *testing.T) {
	h, acts := startWithActions(t)
	h.keys("G") // dddddddd, the oldest
	h.run(h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")}))
	if !strings.HasPrefix(h.selected(), "dddddddd") || h.m.cursor != 0 {
		t.Errorf("pinning should move the session to the top and keep it selected: %s at %d", h.selected(), h.m.cursor)
	}
	if lines := strings.Split(h.m.View(), "\n"); !strings.Contains(lines[1], "★ Bump terraform") {
		t.Errorf("pinned row: %q", lines[1])
	}

	h.keys("t")
	if h.m.prompt != promptTag {
		t.Fatal("t should ask for tags")
	}
	h.keys("infra +ops")
	h.run(h.key(tea.KeyEnter))
	if s, _ := h.m.current(); strings.Join(s.Tags, ",") != "infra,ops" || !strings.Contains(h.m.View(), "#infra #ops") {
		t.Errorf("tags = %v", s.Tags)
	}
	h.keys("t-ops")
	h.run(h.key(tea.KeyEnter))
	if s, _ := h.m.current(); strings.Join(s.Tags, ",") != "infra" {
		t.Errorf("after -ops tags = %v", s.Tags)
	}

	h.keys("r")
	if h.m.prompt != promptRename || h.m.input.Value() != "Bump terraform to 1.9" {
		t.Fatalf("rename should start from the current title: %q", h.m.input.Value())
	}
	h.send(tea.KeyMsg{Type: tea.KeyCtrlU}) // clear the line
	h.keys("Terraform 1.9 upgrade")
	h.run(h.key(tea.KeyEnter))
	if s, _ := h.m.current(); s.Title != "Terraform 1.9 upgrade" || s.OriginalTitle != "Bump terraform to 1.9" {
		t.Errorf("renamed: %q (was %q)", s.Title, s.OriginalTitle)
	}
	h.keys("r")
	h.send(tea.KeyMsg{Type: tea.KeyCtrlU})
	h.run(h.key(tea.KeyEnter)) // empty restores the tool's title
	if s, _ := h.m.current(); s.Title != "Bump terraform to 1.9" || s.OriginalTitle != "" {
		t.Errorf("reset title: %q / %q", s.Title, s.OriginalTitle)
	}

	h.keys("r")
	h.key(tea.KeyEsc)
	if h.m.prompt != promptNone || len(acts.calls) != 5 {
		t.Errorf("esc should cancel without acting: %v", acts.calls)
	}

	want := []string{"pin dddddddd true", "tag dddddddd +infra,ops -", "tag dddddddd + -ops",
		"rename dddddddd Terraform 1.9 upgrade", "rename dddddddd "}
	if got := strings.Join(acts.calls[:5], " | "); got != strings.Join(want, " | ") {
		t.Errorf("calls:\n%s\nwant:\n%s", got, strings.Join(want, " | "))
	}
}

func TestArchiveAndTrashKeys(t *testing.T) {
	h, acts := startWithActions(t)
	h.run(h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})) // archive aaaaaaaa
	if len(h.m.view) != 7 || strings.HasPrefix(h.selected(), "aaaaaaaa") {
		t.Errorf("archived session should leave the list: %d rows, selected %s", len(h.m.view), h.selected())
	}
	if !strings.Contains(h.m.View(), "7 sessions") || !strings.Contains(h.m.View(), "1 archived") {
		t.Errorf("header:\n%s", strings.Split(h.m.View(), "\n")[0])
	}
	h.keys("/is:archived")
	if len(h.m.view) != 1 || !strings.HasPrefix(h.selected(), "aaaaaaaa") {
		t.Errorf("is:archived should show it: %d rows", len(h.m.view))
	}
	h.key(tea.KeyEsc)

	h.keys("d")
	if h.m.prompt != promptTrash || !strings.Contains(h.m.View(), "to the trash? y/n") {
		t.Fatal("d should ask before trashing")
	}
	h.keys("n")
	if h.m.prompt != promptNone || len(acts.calls) != 1 {
		t.Errorf("n should cancel: %v", acts.calls)
	}
	first := h.selected()
	h.keys("d")
	h.run(h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}))
	if len(h.m.view) != 6 || h.selected() == first || !strings.Contains(h.m.status, "moved to the trash") {
		t.Errorf("after trash: %d rows, status %q", len(h.m.view), h.m.status)
	}

	acts.fail = errors.New("disk full")
	h.run(h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")}))
	if !h.m.statusError || !strings.Contains(h.m.status, "pin failed: disk full") {
		t.Errorf("a failed action should say so: %q", h.m.status)
	}
	if s, _ := h.m.current(); s.Pinned {
		t.Error("a failed pin must not pin")
	}
}

func TestOrganizeWithoutActions(t *testing.T) {
	h := start(t, fixtureProviders(t), 120, 30)
	h.keys("r")
	if h.m.prompt != promptNone || !h.m.statusError {
		t.Error("without a data directory, organizing should explain why it can't")
	}
}

func TestLiveSessionsCannotBeTrashed(t *testing.T) {
	h, acts := startWithActions(t)
	h.m.sessions[h.m.view[0]].Live = &model.LiveState{PID: 1}
	h.keys("d")
	if h.m.prompt != promptNone || !strings.Contains(h.m.status, "running") || len(acts.calls) != 0 {
		t.Errorf("live session: prompt %v, status %q", h.m.prompt, h.m.status)
	}
}

func TestStatsPanel(t *testing.T) {
	h := start(t, fixtureProviders(t), 120, 34)
	h.keys("s")
	view := h.m.View()
	totalRow := regexp.MustCompile(`(?m)^\s+total\s+(\d+)\s+(\S+)\s+(\S+)`)
	if !strings.Contains(view, "── stats · by day") || !strings.Contains(view, "Last 30 days of all sessions") {
		t.Fatalf("stats panel:\n%s", view)
	}
	if m := totalRow.FindStringSubmatch(view); m == nil || m[1] != "6" || m[2] != "35k" || m[3] != "$0.04+" {
		t.Errorf("total row %v in:\n%s", m, view)
	}

	h.keys("/t:claude")
	h.key(tea.KeyEnter)
	view = h.m.View()
	if m := totalRow.FindStringSubmatch(view); !strings.Contains(view, "the filtered sessions") || m == nil || m[1] != "3" || m[2] != "9.6k" || m[3] != "$0.04" {
		t.Errorf("stats should follow the filter, total %v:\n%s", m, view)
	}

	for _, want := range []string{"by model", "by project"} {
		h.keys("s")
		if !strings.Contains(h.m.View(), "── stats · "+want) {
			t.Errorf("s should cycle to %s", want)
		}
	}
	h.keys("s")
	if h.m.statsBy != "" || strings.Contains(h.m.View(), "── stats") {
		t.Error("s after project should go back to the preview")
	}
	h.keys("s")
	h.key(tea.KeyEsc)
	if h.m.statsBy != "" {
		t.Error("esc should close the stats")
	}
}
