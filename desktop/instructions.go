package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Project instructions are the files each agent reads for standing guidance:
// CLAUDE.md for Claude Code and AGENTS.md for Codex, in the project and in the
// user's home. Only these known names can be read or written here; files in
// the project can be edited, user-level ones are only shown.

// InstructionFile is one instructions file an agent reads for a project.
type InstructionFile struct {
	Agent string `json:"agent"`
	// Scope is "project" or "user".
	Scope string `json:"scope"`
	// Name is the project-relative name, or the path under the home folder.
	Name     string `json:"name"`
	Path     string `json:"path"`
	Exists   bool   `json:"exists"`
	Size     int64  `json:"size"`
	Editable bool   `json:"editable"`
}

// InstructionText is a project instructions file to edit. Version changes
// whenever the content does, so a save can tell the file changed meanwhile.
type InstructionText struct {
	Name    string `json:"name"`
	Text    string `json:"text"`
	Exists  bool   `json:"exists"`
	Version string `json:"version"`
}

const instructionLimit = 200 << 10

type instructionName struct{ agent, name string }

// The files in a project each agent reads, in the order they are shown.
var projectInstructions = []instructionName{
	{"claude", "CLAUDE.md"},
	{"claude", ".claude/CLAUDE.md"},
	{"claude", "CLAUDE.local.md"},
	{"codex", "AGENTS.md"},
}

// The user-level files, under the home folder.
var userInstructions = []instructionName{
	{"claude", ".claude/CLAUDE.md"},
	{"codex", ".codex/AGENTS.md"},
}

// userHome is the home folder; tests replace it.
var userHome = os.UserHomeDir

func instructionsRoot(ctx context.Context, project string) (string, error) {
	dir, err := projectDirectory(project)
	if err != nil {
		return "", err
	}
	if root, err := repoRoot(ctx, dir); err == nil {
		return root, nil
	}
	return dir, nil
}

func contentVersion(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

func describe(agent, scope, name, path string, editable bool) InstructionFile {
	file := InstructionFile{Agent: agent, Scope: scope, Name: name, Path: path, Editable: editable}
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
		file.Exists, file.Size = true, info.Size()
	}
	return file
}

func projectInstructionFiles(ctx context.Context, project string) ([]InstructionFile, error) {
	root, err := instructionsRoot(ctx, project)
	if err != nil {
		return nil, err
	}
	files := []InstructionFile{}
	for _, known := range projectInstructions {
		files = append(files, describe(known.agent, "project", known.name, filepath.Join(root, filepath.FromSlash(known.name)), true))
	}
	if home, err := userHome(); err == nil {
		for _, known := range userInstructions {
			files = append(files, describe(known.agent, "user", "~/"+known.name, filepath.Join(home, filepath.FromSlash(known.name)), false))
		}
	}
	return files, nil
}

// projectInstructionPath resolves one of the known project names, refusing
// anything else and anything that is a link out of the project.
func projectInstructionPath(ctx context.Context, project, name string) (string, error) {
	root, err := instructionsRoot(ctx, project)
	if err != nil {
		return "", err
	}
	for _, known := range projectInstructions {
		if known.name != name {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(name))
		if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
			return "", fmt.Errorf("%s is not a plain file, so it is not edited here", name)
		}
		return path, nil
	}
	return "", fmt.Errorf("%q is not an instructions file ais edits", name)
}

func readInstruction(ctx context.Context, project, name string) (InstructionText, error) {
	path, err := projectInstructionPath(ctx, project, name)
	if err != nil {
		return InstructionText{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return InstructionText{Name: name, Version: ""}, nil
	}
	if err != nil {
		return InstructionText{}, err
	}
	if len(data) > instructionLimit {
		return InstructionText{}, fmt.Errorf("%s is larger than 200 KB; edit it in your editor", name)
	}
	return InstructionText{Name: name, Text: string(data), Exists: true, Version: contentVersion(data)}, nil
}

// writeInstruction saves a project instructions file, refusing when it
// changed on disk since it was read (version is what was read; "" for a new
// file). The file is replaced in one step.
func writeInstruction(ctx context.Context, project, name, text, version string) (InstructionText, error) {
	if len(text) > instructionLimit {
		return InstructionText{}, errors.New("instructions are limited to 200 KB here")
	}
	path, err := projectInstructionPath(ctx, project, name)
	if err != nil {
		return InstructionText{}, err
	}
	current, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if version != "" {
			return InstructionText{}, fmt.Errorf("%s was removed since you opened it; nothing was saved", name)
		}
	case err != nil:
		return InstructionText{}, err
	case contentVersion(current) != version:
		return InstructionText{}, fmt.Errorf("%s changed on disk since you opened it; nothing was saved. Close it and open it again", name)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return InstructionText{}, err
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".ais-instructions-*")
	if err != nil {
		return InstructionText{}, err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.WriteString(text); err != nil {
		temp.Close()
		return InstructionText{}, err
	}
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return InstructionText{}, err
	}
	if err := temp.Close(); err != nil {
		return InstructionText{}, err
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return InstructionText{}, err
	}
	return InstructionText{Name: name, Text: text, Exists: true, Version: contentVersion([]byte(text))}, nil
}

// ProjectInstructions lists the instructions files each agent reads for a
// project, with whether each exists.
func (a *App) ProjectInstructions(project string) ([]InstructionFile, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()
	return projectInstructionFiles(ctx, project)
}

// ReadInstruction reads one of a project's instructions files to edit.
func (a *App) ReadInstruction(project, name string) (InstructionText, error) {
	if err := a.ready(); err != nil {
		return InstructionText{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()
	return readInstruction(ctx, project, name)
}

// WriteInstruction saves one of a project's instructions files.
func (a *App) WriteInstruction(project, name, text, version string) (InstructionText, error) {
	if err := a.ready(); err != nil {
		return InstructionText{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()
	return writeInstruction(ctx, project, name, text, version)
}

// RevealInstruction shows the folder of an instructions file in Finder:
// how user-level files are opened.
func (a *App) RevealInstruction(project, name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()
	files, err := projectInstructionFiles(ctx, project)
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.Name == name {
			return a.openFolder(filepath.Dir(file.Path))
		}
	}
	return fmt.Errorf("%q is not an instructions file ais knows", name)
}
