package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Skills are read from each agent the way it reports them, with no model
// turn: Claude Code lists its commands and skills in the initialize answer,
// and Codex answers skills/list. Using one is up to the message: "/name" for
// Claude Code, "$name" for Codex, which ais turns into a skill input item.

// AgentSkill is one skill or command an agent offers in a project.
type AgentSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Hint describes the arguments, when the agent gives one.
	Hint string `json:"hint,omitempty"`
	// Scope is where it comes from, such as repo, user or system (Codex).
	Scope string `json:"scope,omitempty"`
	// Path is the skill file; Codex needs it to use the skill.
	Path string `json:"path,omitempty"`
}

// SkillList is every skill an agent offers in a project. Reason says why
// none could be read.
type SkillList struct {
	Tool   string       `json:"tool"`
	Skills []AgentSkill `json:"skills"`
	Reason string       `json:"reason,omitempty"`
}

type skillCache struct {
	gate  chan struct{}
	value SkillList
	at    time.Time
}

const skillsFresh = 5 * time.Minute

// claudeSkillArgs start a session that only answers control requests, with
// commands and skills loaded so initialize lists them.
var claudeSkillArgs = []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--tools", "", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`}

func (m *chatManager) readSkills(ctx context.Context, tool, project string) (SkillList, error) {
	args := []string{"app-server", "--listen", "stdio://"}
	if tool == "claude" {
		args = claudeSkillArgs
	}
	cmd := m.command(tool, args...)
	cmd.Dir = project
	p, err := startJSONProcess(ctx, cmd)
	if err != nil {
		return SkillList{}, err
	}
	defer p.close()
	if tool == "claude" {
		err = p.send(map[string]any{"type": "control_request", "request_id": "ais-skills", "request": map[string]any{"subtype": "initialize"}})
	} else {
		err = p.send(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]string{"name": "ais_skills", "version": "0.1.0"}}})
	}
	if err != nil {
		return SkillList{}, err
	}
	list := SkillList{Tool: tool, Skills: []AgentSkill{}}
	for {
		select {
		case <-ctx.Done():
			return SkillList{}, ctx.Err()
		case packet, ok := <-p.packets:
			if !ok {
				return SkillList{}, p.failure()
			}
			if packet.Err != nil {
				return SkillList{}, packet.Err
			}
			v := packet.Value
			if tool == "claude" {
				response := object(v["response"])
				if str(v["type"]) != "control_response" || str(response["request_id"]) != "ais-skills" {
					continue
				}
				if str(response["subtype"]) == "error" {
					return SkillList{}, errors.New(str(response["error"]))
				}
				for _, raw := range array(object(response["response"])["commands"]) {
					entry := object(raw)
					if name := str(entry["name"]); name != "" {
						list.Skills = append(list.Skills, AgentSkill{Name: name, Description: str(entry["description"]), Hint: str(entry["argumentHint"])})
					}
				}
				return list, nil
			}
			if str(v["method"]) != "" {
				continue
			}
			if raw := v["error"]; len(raw) > 0 && string(raw) != "null" {
				return SkillList{}, errors.New(errorMessage(raw))
			}
			switch number(v["id"]) {
			case 1:
				if err = p.send(map[string]any{"method": "initialized", "params": map[string]any{}}); err == nil {
					err = p.send(map[string]any{"id": 2, "method": "skills/list", "params": map[string]any{"cwds": []string{project}}})
				}
				if err != nil {
					return SkillList{}, err
				}
			case 2:
				for _, rawEntry := range array(object(v["result"])["data"]) {
					for _, raw := range array(object(rawEntry)["skills"]) {
						entry := object(raw)
						if name := str(entry["name"]); name != "" && string(entry["enabled"]) != "false" {
							description := str(entry["shortDescription"])
							if description == "" {
								description = str(entry["description"])
							}
							list.Skills = append(list.Skills, AgentSkill{Name: name, Description: description, Scope: str(entry["scope"]), Path: str(entry["path"])})
						}
					}
				}
				return list, nil
			}
		}
	}
}

// skillList reads an agent's skills for a project, reusing a read from the
// last five minutes unless refresh is set.
func (m *chatManager) skillList(ctx context.Context, tool, project string, refresh bool) (SkillList, error) {
	if tool != "claude" && tool != "codex" {
		return SkillList{}, errors.New("unknown agent")
	}
	dir, err := projectDirectory(project)
	if err != nil {
		return SkillList{}, err
	}
	key := tool + "\x00" + dir
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return SkillList{}, errors.New("the app is closing")
	}
	if m.skillLists == nil {
		m.skillLists = map[string]*skillCache{}
	}
	cache := m.skillLists[key]
	if cache == nil {
		cache = &skillCache{gate: make(chan struct{}, 1)}
		m.skillLists[key] = cache
	}
	m.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	stop := context.AfterFunc(m.discoveryCtx, cancel)
	defer stop()
	select {
	case cache.gate <- struct{}{}:
		defer func() { <-cache.gate }()
	case <-ctx.Done():
		return SkillList{}, ctx.Err()
	}
	if !refresh && !cache.at.IsZero() && time.Since(cache.at) < skillsFresh {
		return cache.value, nil
	}
	list, err := m.readSkills(ctx, tool, dir)
	if err != nil {
		list = SkillList{Tool: tool, Skills: []AgentSkill{}}
		switch {
		case errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist):
			list.Reason = toolName(tool) + " is not installed."
		case errors.Is(err, context.DeadlineExceeded):
			list.Reason = toolName(tool) + " did not list its skills in time."
		default:
			list.Reason = toolName(tool) + " could not list its skills: " + shorten(err.Error(), 300)
		}
		// A failure is not kept, so the next look tries again.
		return list, nil
	}
	sort.SliceStable(list.Skills, func(i, j int) bool {
		return strings.ToLower(list.Skills[i].Name) < strings.ToLower(list.Skills[j].Name)
	})
	cache.value, cache.at = list, time.Now()
	return list, nil
}

// skillMention is "$name" in a Codex message, as its own word.
var skillMention = regexp.MustCompile(`(?:^|\s)\$([A-Za-z0-9][\w.:-]*)`)

// codexSkillInputs are the skill input items for the "$name" mentions in a
// message that name one of the project's skills. Unknown names stay text.
func (m *chatManager) codexSkillInputs(ctx context.Context, project, text string) []any {
	matches := skillMention.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	list, err := m.skillList(ctx, "codex", project, false)
	if err != nil {
		return nil
	}
	byName := map[string]AgentSkill{}
	for _, skill := range list.Skills {
		byName[skill.Name] = skill
	}
	items := []any{}
	seen := map[string]bool{}
	for _, match := range matches {
		name := strings.TrimRight(match[1], ".:,;")
		if skill, ok := byName[name]; ok && skill.Path != "" && !seen[name] {
			seen[name] = true
			items = append(items, map[string]any{"type": "skill", "name": skill.Name, "path": skill.Path})
		}
	}
	return items
}

// AgentSkills lists an agent's skills for a project. It starts no model turn.
func (a *App) AgentSkills(tool, project string, refresh bool) (SkillList, error) {
	if err := a.chatReady(); err != nil {
		return SkillList{}, err
	}
	list, err := a.chats.skillList(a.ctx, tool, project, refresh)
	if err != nil {
		return SkillList{}, fmt.Errorf("skills: %w", err)
	}
	return list, nil
}
