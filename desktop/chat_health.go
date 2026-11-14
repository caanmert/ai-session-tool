package main

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Versions of each agent's command-line tool that the authenticated checks in
// docs/DESKTOP.md last passed with. Update these when the checks are repeated.
var testedAgentVersions = map[string]string{
	"claude": "2.1.296",
	"codex":  "0.162.1",
}

// AgentHealth reports whether an installed agent still accepts what ais sends
// it. The checks only ask the tool for its version and help: no model is
// contacted and no usage is consumed.
type AgentHealth struct {
	Tool    string `json:"tool"`
	Version string `json:"version"`
	// Tested is the version the live checks last passed with.
	Tested string `json:"tested"`
	// Newer is set when the installed version is newer than Tested.
	Newer bool `json:"newer"`
	// Problem is set when the tool rejects an option ais depends on.
	Problem string `json:"problem,omitempty"`
}

var versionPattern = regexp.MustCompile(`\d+\.\d+\.\d+`)

// newerVersion reports whether a is a later release than b.
func newerVersion(a, b string) bool {
	left, right := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(left) && i < len(right); i++ {
		x, _ := strconv.Atoi(left[i])
		y, _ := strconv.Atoi(right[i])
		if x != y {
			return x > y
		}
	}
	return false
}

// claudeBaseArgs are the fixed options every Claude Code conversation starts
// with. Conversations and the compatibility check share them so they cannot
// drift apart.
var claudeBaseArgs = []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--include-partial-messages", "--permission-prompt-tool", "stdio", "--permission-mode", "default"}

// Asking for the version alongside the base options makes the tool validate
// them and exit without starting a session.
var claudeProbeArgs = append(append([]string{}, claudeBaseArgs...), "--version")

func (m *chatManager) run(ctx context.Context, tool string, args ...string) (string, error) {
	cmd := m.command(tool, args...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return out.String(), err
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-done
		return out.String(), ctx.Err()
	}
}

func (m *chatManager) health(ctx context.Context, tool string) AgentHealth {
	result := AgentHealth{Tool: tool, Tested: testedAgentVersions[tool]}
	version, err := m.run(ctx, tool, "--version")
	if err != nil {
		result.Problem = "Its version could not be read: " + shorten(strings.TrimSpace(version+" "+err.Error()), 300)
		return result
	}
	result.Version = versionPattern.FindString(version)
	result.Newer = result.Version != "" && result.Tested != "" && newerVersion(result.Version, result.Tested)
	probe := []string{"app-server", "--help"}
	if tool == "claude" {
		probe = claudeProbeArgs
	}
	if out, err := m.run(ctx, tool, probe...); err != nil {
		detail := strings.TrimSpace(out)
		if detail == "" {
			detail = err.Error()
		}
		result.Problem = "This version rejects an option ais relies on, so conversations may not start: " + shorten(detail, 300)
	}
	return result
}

// AgentHealth checks each installed agent's version and whether it still
// accepts the options ais uses. Agents that are not installed are skipped.
func (a *App) AgentHealth() ([]AgentHealth, error) {
	if err := a.chatReady(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	tools := []string{"codex", "claude"}
	results := make([]*AgentHealth, len(tools))
	var wait sync.WaitGroup
	for i, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			continue
		}
		wait.Add(1)
		go func() {
			defer wait.Done()
			health := a.chats.health(ctx, tool)
			results[i] = &health
		}()
	}
	wait.Wait()
	found := []AgentHealth{}
	for _, result := range results {
		if result != nil {
			found = append(found, *result)
		}
	}
	return found, nil
}

// RevealProject opens a conversation's project folder in Finder.
func (a *App) RevealProject(id string) error {
	if err := a.chatReady(); err != nil {
		return err
	}
	c, err := a.chats.get(id)
	if err != nil {
		return err
	}
	c.mu.Lock()
	project := c.state.Project
	c.mu.Unlock()
	return a.openFolder(project)
}
