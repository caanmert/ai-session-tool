package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/caanmert/ai-session-tool/internal/application"
	"github.com/caanmert/ai-session-tool/internal/index"
	"github.com/caanmert/ai-session-tool/internal/meta"
	"github.com/caanmert/ai-session-tool/internal/runner"
)

type App struct {
	ctx          context.Context
	service      *application.Service
	startupError error
	terminals    *terminals
	chats        *chatManager
	chatError    error
	runs         *runner.Manager
	// dockAttention updates the Dock icon; replaced in tests.
	dockAttention func(count int, alert bool)
}

func NewApp() *App { return &App{terminals: newTerminals(), dockAttention: setDockAttention} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// Finder does not run the user's shell startup files. Include common CLI
	// install locations without executing shell profiles or changing their files.
	path := filepath.SplitList(os.Getenv("PATH"))
	locations := []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin", "/usr/sbin", "/sbin"}
	if home, err := os.UserHomeDir(); err == nil {
		locations = append([]string{filepath.Join(home, ".local", "bin")}, locations...)
	}
	for _, dir := range locations {
		found := false
		for _, existing := range path {
			found = found || existing == dir
		}
		if !found {
			path = append(path, dir)
		}
	}
	_ = os.Setenv("PATH", strings.Join(path, string(os.PathListSeparator)))
	indexPath, err := index.DefaultPath()
	if err != nil {
		a.startupError = err
		return
	}
	dataDir, err := meta.DefaultDir()
	if err != nil {
		a.startupError = err
		return
	}
	a.service = application.NewService(application.DefaultProviders(), indexPath, dataDir)
	a.chats = newChats(filepath.Join(dataDir, "desktop-chats"))
	a.chatError = a.chats.load()
	a.runs = runner.New(filepath.Join(dataDir, "runs"))
}

func (a *App) shutdown(context.Context) {
	a.terminals.close()
	if a.chats != nil {
		a.chats.close()
	}
	if a.service != nil {
		_ = a.service.Close()
	}
}

func (a *App) ready() error {
	if a.startupError != nil {
		return a.startupError
	}
	if a.service == nil {
		return errors.New("session manager is starting; try again shortly")
	}
	return nil
}

// SetAppearance keeps the native window and WebView surfaces in the same mode.
func (a *App) SetAppearance(theme string) error {
	if theme != "light" && theme != "dark" {
		return errors.New("choose light or dark appearance")
	}
	if a.ctx == nil {
		return errors.New("the window is starting")
	}
	if theme == "dark" {
		wailsruntime.WindowSetDarkTheme(a.ctx)
		wailsruntime.WindowSetBackgroundColour(a.ctx, 14, 15, 17, 255) // --canvas, dark
	} else {
		wailsruntime.WindowSetLightTheme(a.ctx)
		wailsruntime.WindowSetBackgroundColour(a.ctx, 250, 250, 249, 255) // --canvas, light
	}
	return nil
}

// SetAttention shows how many conversations are waiting for the user on the
// Dock icon. With alert set, the icon also bounces once if the app is in the
// background. No notification permission is involved.
func (a *App) SetAttention(count int, alert bool) error {
	if a.ctx == nil {
		return errors.New("the app is still starting")
	}
	if count < 0 {
		count = 0
	}
	if a.dockAttention != nil {
		a.dockAttention(count, alert)
	}
	return nil
}

func (a *App) Refresh() (application.Snapshot, error) {
	if err := a.ready(); err != nil {
		return application.Snapshot{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	return a.service.Refresh(ctx)
}

// AnnotateSession renames, pins or tags a saved session in ais and returns the
// refreshed snapshot. The agent's own transcript is not modified.
func (a *App) AnnotateSession(key string, change application.AnnotationChange) (application.Snapshot, error) {
	if err := a.ready(); err != nil {
		return application.Snapshot{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	return a.service.Annotate(ctx, key, change)
}

func (a *App) Search(query string) ([]string, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return a.service.Search(ctx, query)
}

func (a *App) Transcript(key string) (application.Preview, error) {
	if err := a.ready(); err != nil {
		return application.Preview{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.service.Transcript(ctx, key)
}

func (a *App) PickProject() (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	dir, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "Choose a project folder"})
	if err != nil || dir == "" {
		return "", err
	}
	return a.service.AddProject(a.ctx, dir)
}

func (a *App) NewSession(project, tool string) (LiveSession, error) {
	if err := a.ready(); err != nil {
		return LiveSession{}, err
	}
	if project == "" {
		return LiveSession{}, errors.New("choose a project folder")
	}
	cmd, err := a.service.NewSession(tool, project)
	if err != nil {
		return LiveSession{}, err
	}
	return a.terminals.start(cmd, tool, "")
}

func (a *App) ResumeSession(key string) (LiveSession, error) {
	if err := a.ready(); err != nil {
		return LiveSession{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	cmd, err := a.service.Resume(ctx, key)
	if err != nil {
		return LiveSession{}, err
	}
	tool, _, _ := strings.Cut(key, "/")
	return a.terminals.start(cmd, tool, key)
}

func (a *App) LiveSessions() []LiveSession { return a.terminals.list() }
func (a *App) ReadTerminal(id string, cursor int64) (TerminalChunk, error) {
	return a.terminals.poll(id, cursor)
}
func (a *App) WriteTerminal(id, data string) error { return a.terminals.input(id, data) }
func (a *App) ResizeTerminal(id string, cols, rows int) error {
	return a.terminals.resize(id, cols, rows)
}
func (a *App) StopTerminal(id string) error  { return a.terminals.stop(id) }
func (a *App) CloseTerminal(id string) error { return a.terminals.remove(id) }

func (a *App) beforeClose(ctx context.Context) bool {
	running := 0
	for _, session := range a.terminals.list() {
		if !session.Exited {
			running++
		}
	}
	if a.chats != nil {
		for _, chat := range a.chats.list() {
			if busyChat(chat.Status) {
				running++
			}
		}
	}
	if running == 0 {
		return false
	}
	title, message := quitConfirmation(running)
	// The safe choice is both the default and the Escape action.
	answer, err := wailsruntime.MessageDialog(ctx, wailsruntime.MessageDialogOptions{
		Type: wailsruntime.QuestionDialog, Title: title, Message: message,
		Buttons: []string{"Cancel", "Quit and stop agents"}, DefaultButton: "Cancel", CancelButton: "Cancel",
	})
	return err != nil || answer != "Quit and stop agents"
}

// quitConfirmation says how many agents quitting would stop and what is kept.
func quitConfirmation(running int) (title, message string) {
	title = "Quit with 1 agent running?"
	if running != 1 {
		title = "Quit with " + strconv.Itoa(running) + " agents running?"
	}
	return title, "Quitting stops their responses. Files they already edited stay as they are, and your conversations, drafts and review snapshots are kept on this Mac."
}
