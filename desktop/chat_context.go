package main

import "encoding/json"

// ContextUsage is how much of the model's context a conversation fills,
// from the agent's own token counts for its latest response. Window is zero
// until the agent reports it.
type ContextUsage struct {
	Used   int64 `json:"used"`
	Window int64 `json:"window,omitempty"`
}

func int64Of(raw json.RawMessage) int64 {
	value, _ := numberValue(raw)
	return int64(value)
}

// reportContext records the latest context use. Zero leaves a value as it
// was, because the two halves can arrive in different messages.
func (c *chatSession) reportContext(used, window int64) {
	if used <= 0 && window <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	next := ContextUsage{}
	if c.state.Context != nil {
		next = *c.state.Context
	}
	if used > 0 {
		next.Used = used
	}
	if window > 0 {
		next.Window = window
	}
	if c.state.Context != nil && *c.state.Context == next {
		return
	}
	c.state.Context = &next
	c.change()
}

// claudeContextUsed is everything a Claude response held in context: its
// input, cached or not, and what it wrote.
func claudeContextUsed(usage jsonObject) int64 {
	return int64Of(usage["input_tokens"]) + int64Of(usage["cache_creation_input_tokens"]) +
		int64Of(usage["cache_read_input_tokens"]) + int64Of(usage["output_tokens"])
}

// claudeContextWindow reads the window of the model that answered from a
// result's modelUsage, or the largest one listed when that model isn't.
func claudeContextWindow(modelUsage jsonObject, model string) int64 {
	if window := int64Of(object(modelUsage[model])["contextWindow"]); model != "" && window > 0 {
		return window
	}
	largest := int64(0)
	for _, raw := range modelUsage {
		if window := int64Of(object(raw)["contextWindow"]); window > largest {
			largest = window
		}
	}
	return largest
}

// codexContextUsage reads thread/tokenUsage/updated: the last request's
// tokens, less its reasoning, against the model's window, as Codex counts it.
func codexContextUsage(params jsonObject) (used, window int64) {
	usage := object(params["tokenUsage"])
	last := object(usage["last"])
	used = int64Of(last["totalTokens"]) - int64Of(last["reasoningOutputTokens"])
	return used, int64Of(usage["modelContextWindow"])
}
