package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type ModelSettings struct {
	Model      string `json:"model"`
	Effort     string `json:"effort"`
	Configured bool   `json:"configured"`
}
type ModelOption struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Efforts       []string `json:"efforts"`
	DefaultEffort string   `json:"defaultEffort"`
}
type ModelCatalog struct {
	Models        []ModelOption `json:"models"`
	DefaultModel  string        `json:"defaultModel"`
	DefaultEffort string        `json:"defaultEffort"`
}
type modelCache struct {
	gate    chan struct{}
	catalog ModelCatalog
	expires time.Time
}

func (m *chatManager) models(ctx context.Context, tool, project string, refresh bool) (ModelCatalog, error) {
	if tool != "codex" && tool != "claude" {
		return ModelCatalog{}, errors.New("unknown provider")
	}
	key := tool + "\x00" + project
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ModelCatalog{}, errors.New("the app is closing")
	}
	if m.catalogs == nil {
		m.catalogs = map[string]*modelCache{}
	}
	cached := m.catalogs[key]
	if cached == nil {
		cached = &modelCache{gate: make(chan struct{}, 1)}
		m.catalogs[key] = cached
	}
	m.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	stop := context.AfterFunc(m.discoveryCtx, cancel)
	defer stop()
	select {
	case cached.gate <- struct{}{}:
		defer func() { <-cached.gate }()
	case <-ctx.Done():
		return ModelCatalog{}, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return ModelCatalog{}, err
	}
	if !refresh && time.Now().Before(cached.expires) {
		return cached.catalog, nil
	}
	// Review discovery uses the same empty-directory scope as review execution.
	if project == "" {
		dir, err := os.MkdirTemp("", "ais-models-*")
		if err != nil {
			return ModelCatalog{}, err
		}
		defer os.RemoveAll(dir)
		project = dir
	}
	catalog, err := m.discoverModels(ctx, tool, project)
	if err != nil {
		return ModelCatalog{}, err
	}
	if len(catalog.Models) == 0 {
		return ModelCatalog{}, errors.New("agent did not report a model catalog; update or sign in to its CLI")
	}
	cached.catalog = catalog
	cached.expires = time.Now().Add(5 * time.Minute)
	return catalog, nil
}
func (m *chatManager) discoverModels(ctx context.Context, tool, project string) (ModelCatalog, error) {
	args := []string{"app-server", "--listen", "stdio://"}
	if tool == "claude" {
		args = []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--tools", "", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--disable-slash-commands"}
	}
	cmd := m.command(tool, args...)
	cmd.Dir = project
	p, err := startJSONProcess(ctx, cmd)
	if err != nil {
		return ModelCatalog{}, err
	}
	defer p.close()
	send := func(v any) error { return p.send(v) }
	if tool == "codex" {
		err = send(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]string{"name": "ais_model_picker", "version": "0.1.0"}}})
	} else {
		err = send(map[string]any{"type": "control_request", "request_id": "ais-models", "request": map[string]any{"subtype": "initialize"}})
	}
	if err != nil {
		return ModelCatalog{}, err
	}
	catalog := ModelCatalog{Models: []ModelOption{}}
	seen := map[string]bool{}
	pages := 0
	for {
		select {
		case <-ctx.Done():
			return ModelCatalog{}, fmt.Errorf("model discovery: %w", ctx.Err())
		case packet, ok := <-p.packets:
			if !ok {
				return ModelCatalog{}, p.failure()
			}
			if packet.Err != nil {
				return ModelCatalog{}, packet.Err
			}
			v := packet.Value
			if tool == "claude" {
				if str(v["type"]) != "control_response" {
					continue
				}
				response := object(v["response"])
				if str(response["request_id"]) != "ais-models" {
					continue
				}
				if str(response["subtype"]) == "error" {
					return ModelCatalog{}, fmt.Errorf("Claude model discovery: %s", str(response["error"]))
				}
				for _, raw := range array(object(response["response"])["models"]) {
					entry := object(raw)
					id := str(entry["value"])
					if id == "" || seen[id] {
						continue
					}
					seen[id] = true
					option := ModelOption{ID: id, Name: str(entry["displayName"]), Description: str(entry["description"]), Efforts: []string{}}
					if boolean(entry["supportsEffort"]) {
						for _, e := range array(entry["supportedEffortLevels"]) {
							if effort := str(e); validEffort(effort) {
								option.Efforts = append(option.Efforts, effort)
							}
						}
					}
					if id == "default" {
						option.ID = ""
						catalog.DefaultModel = str(entry["resolvedModel"])
					}
					catalog.Models = append(catalog.Models, option)
				}
				return catalog, nil
			}
			if str(v["method"]) != "" {
				continue
			}
			if raw := v["error"]; len(raw) > 0 && string(raw) != "null" {
				return ModelCatalog{}, fmt.Errorf("Codex model discovery: %s", pretty(raw))
			}
			result := object(v["result"])
			switch number(v["id"]) {
			case 1:
				if err = send(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
					return ModelCatalog{}, err
				}
				if err = send(map[string]any{"id": 2, "method": "config/read", "params": map[string]any{"cwd": project, "includeLayers": false}}); err != nil {
					return ModelCatalog{}, err
				}
			case 2:
				config := object(result["config"])
				catalog.DefaultModel = str(config["model"])
				catalog.DefaultEffort = str(config["model_reasoning_effort"])
				if err = send(map[string]any{"id": 3, "method": "model/list", "params": map[string]any{"limit": 100, "includeHidden": false}}); err != nil {
					return ModelCatalog{}, err
				}
			case 3:
				pages++
				for _, raw := range array(result["data"]) {
					entry := object(raw)
					id := str(entry["model"])
					if id == "" || boolean(entry["hidden"]) || seen[id] {
						continue
					}
					seen[id] = true
					option := ModelOption{ID: id, Name: str(entry["displayName"]), Description: str(entry["description"]), DefaultEffort: str(entry["defaultReasoningEffort"]), Efforts: []string{}}
					for _, rawEffort := range array(entry["supportedReasoningEfforts"]) {
						if effort := str(object(rawEffort)["reasoningEffort"]); validEffort(effort) {
							option.Efforts = append(option.Efforts, effort)
						}
					}
					if catalog.DefaultModel == "" && boolean(entry["isDefault"]) {
						catalog.DefaultModel = id
					}
					catalog.Models = append(catalog.Models, option)
				}
				cursor := str(result["nextCursor"])
				if cursor == "" {
					return catalog, nil
				}
				if pages >= 20 {
					return ModelCatalog{}, errors.New("model catalog exceeded its page limit")
				}
				if err = send(map[string]any{"id": 3, "method": "model/list", "params": map[string]any{"cursor": cursor, "limit": 100, "includeHidden": false}}); err != nil {
					return ModelCatalog{}, err
				}
			}
		}
	}
}
func validEffort(value string) bool {
	switch value {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra", "ultracode":
		return true
	}
	return false
}
func (catalog ModelCatalog) option(id string) (ModelOption, bool) {
	for _, option := range catalog.Models {
		if option.ID == id {
			return option, true
		}
	}
	if id == "" {
		for _, option := range catalog.Models {
			if option.ID == catalog.DefaultModel {
				return option, true
			}
		}
	}
	return ModelOption{}, false
}
func (catalog ModelCatalog) validate(settings ModelSettings) error {
	if settings.Model == "" && settings.Effort == "" {
		return nil
	}
	option, ok := catalog.option(settings.Model)
	if !ok {
		return errors.New("selected model is no longer offered by the agent; refresh models or choose Agent default")
	}
	if settings.Effort != "" {
		for _, effort := range option.Efforts {
			if effort == settings.Effort {
				return nil
			}
		}
		return errors.New("selected reasoning effort is not supported by this model")
	}
	return nil
}
func (m *chatManager) settings(ctx context.Context, id string, settings ModelSettings, reviewer bool) error {
	if len(settings.Model) > 256 || strings.ContainsAny(settings.Model, "\x00\r\n") || (settings.Effort != "" && !validEffort(settings.Effort)) {
		return errors.New("invalid model settings")
	}
	c, err := m.get(id)
	if err != nil {
		return err
	}
	c.settingsMu.Lock()
	defer c.settingsMu.Unlock()
	c.mu.Lock()
	state := c.state
	c.mu.Unlock()
	if busyChat(state.Status) {
		return errors.New("finish or stop the response before changing model settings")
	}
	if state.Review != nil {
		return errors.New("choose reviewer settings in the original conversation")
	}
	tool, project := state.Tool, state.Project
	if reviewer {
		tool = otherProvider(tool)
		project = ""
	}
	if settings.Model != "" || settings.Effort != "" {
		catalog, err := m.models(ctx, tool, project, false)
		if err != nil {
			return err
		}
		if err = catalog.validate(settings); err != nil {
			return err
		}
	}
	c.mu.Lock()
	if busyChat(c.state.Status) {
		c.mu.Unlock()
		return errors.New("finish or stop the response before changing model settings")
	}
	settings.Configured = true
	if reviewer {
		c.state.ReviewerSettings = settings
	} else {
		c.state.Settings = settings
	}
	c.change()
	c.mu.Unlock()
	if err := m.save(c); err != nil {
		c.mu.Lock()
		c.state.Settings = state.Settings
		c.state.ReviewerSettings = state.ReviewerSettings
		c.change()
		c.mu.Unlock()
		return err
	}
	return nil
}
func otherProvider(tool string) string {
	if tool == "codex" {
		return "claude"
	}
	return "codex"
}
func (c *chatSession) reportModel(model string) {
	if model == "" {
		return
	}
	c.mu.Lock()
	c.state.ActiveModel = model
	c.change()
	c.mu.Unlock()
}
