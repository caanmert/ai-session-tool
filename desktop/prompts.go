package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Saved prompts are reusable messages, kept with the app's data rather than
// in a project. Until the user changes the list, a few starters are offered;
// once saved, the user's list is used as it is, even when empty.

// SavedPrompt is a reusable message. Its text may hold {project}, {branch}
// and {changed files}, which the app fills in when it is chosen.
type SavedPrompt struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Text string `json:"text"`
}

const (
	promptLimit     = 100
	promptNameLimit = 80
	promptTextLimit = 20000
)

var starterPrompts = []SavedPrompt{
	{ID: "starter-review", Name: "Review for problems", Text: "Review the uncommitted changes in {project} for bugs, security problems and missing error handling. List what you find with file and line; don't change anything yet.\n\nChanged files: {changed files}"},
	{ID: "starter-tests", Name: "Write tests for the changes", Text: "Write tests for the changes on {branch}: {changed files}. Follow the project's existing test style, then run them."},
	{ID: "starter-explain", Name: "Explain the changes", Text: "Explain what the changes in {changed files} do and why, in plain words, as if for a reviewer new to this code."},
}

func readPrompts(path string) ([]SavedPrompt, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return append([]SavedPrompt(nil), starterPrompts...), nil
	}
	if err != nil {
		return nil, err
	}
	var prompts []SavedPrompt
	if err := json.Unmarshal(data, &prompts); err != nil {
		return nil, fmt.Errorf("saved prompts could not be read: %w", err)
	}
	if prompts == nil {
		prompts = []SavedPrompt{}
	}
	return prompts, nil
}

func writePrompts(path string, prompts []SavedPrompt) error {
	if len(prompts) > promptLimit {
		return fmt.Errorf("keep at most %d saved prompts", promptLimit)
	}
	seen := map[string]bool{}
	for i, p := range prompts {
		p.Name, p.Text = strings.TrimSpace(p.Name), strings.TrimSpace(p.Text)
		switch {
		case p.ID == "" || seen[p.ID]:
			return errors.New("each saved prompt needs its own ID")
		case p.Name == "" || p.Text == "":
			return errors.New("a saved prompt needs a name and text")
		case utf8.RuneCountInString(p.Name) > promptNameLimit:
			return fmt.Errorf("a saved prompt's name is limited to %d characters", promptNameLimit)
		case len(p.Text) > promptTextLimit:
			return errors.New("a saved prompt is limited to 20,000 characters")
		}
		seen[p.ID] = true
		prompts[i] = p
	}
	if prompts == nil {
		prompts = []SavedPrompt{}
	}
	data, err := json.MarshalIndent(prompts, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".prompts-*")
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
	return os.Rename(temp.Name(), path)
}

// SavedPrompts lists the saved prompts, or the starters until the list is saved.
func (a *App) SavedPrompts() ([]SavedPrompt, error) {
	if a.promptsPath == "" {
		return nil, errors.New("the app is still starting")
	}
	return readPrompts(a.promptsPath)
}

// SetSavedPrompts replaces the saved prompts.
func (a *App) SetSavedPrompts(prompts []SavedPrompt) error {
	if a.promptsPath == "" {
		return errors.New("the app is still starting")
	}
	return writePrompts(a.promptsPath, prompts)
}
