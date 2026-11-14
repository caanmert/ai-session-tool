package main

import (
	"path/filepath"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// PickFiles lets the user choose files to mention in a message to an agent.
// It returns paths the agent can open: relative to the conversation's project
// when the file is inside it, absolute otherwise. Nothing is read or copied.
func (a *App) PickFiles(id string) ([]string, error) {
	if err := a.chatReady(); err != nil {
		return nil, err
	}
	c, err := a.chats.get(id)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	project := c.state.Project
	c.mu.Unlock()
	files, err := wailsruntime.OpenMultipleFilesDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:            "Choose files to mention",
		DefaultDirectory: project,
	})
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, mentionPath(project, file))
	}
	return paths, nil
}

// mentionPath shortens a chosen file to a project-relative path when it lies
// inside the project, so the agent sees the same path it uses itself.
func mentionPath(project, file string) string {
	if project == "" {
		return file
	}
	rel, err := filepath.Rel(project, file)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return file
	}
	return rel
}
