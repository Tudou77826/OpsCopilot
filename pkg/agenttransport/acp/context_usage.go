package acp

import "encoding/json"

// ContextUsage is iCode's current session-window gauge, not cumulative billing
// tokens. Unknown usage must never be presented as an empty context window.
type ContextUsage struct {
	Known bool  `json:"known"`
	Used  int64 `json:"used"`
	Size  int64 `json:"size"`
}

func (c *Conversation) ContextUsage() ContextUsage {
	c.usageMu.RLock()
	defer c.usageMu.RUnlock()
	return c.usage
}

func (c *Conversation) resetContextUsage() {
	c.usageMu.Lock()
	defer c.usageMu.Unlock()
	c.usage = ContextUsage{}
}

// Called only after Interactions checks the bound session and live turn.
// Standard usage_update already excludes iCode sub-agent usage.
func (c *Conversation) observeContextUsage(raw json.RawMessage) {
	var params struct {
		Update struct {
			Type string `json:"sessionUpdate"`
			Used *int64 `json:"used"`
			Size *int64 `json:"size"`
		} `json:"update"`
	}
	if json.Unmarshal(raw, &params) != nil || params.Update.Type != "usage_update" {
		return
	}
	u := params.Update
	const maxSafeInteger = 1<<53 - 1
	if u.Used == nil || u.Size == nil || *u.Used < 0 || *u.Size < 0 || *u.Used > maxSafeInteger || *u.Size > maxSafeInteger {
		return
	}
	c.usageMu.Lock()
	defer c.usageMu.Unlock()
	c.usage = ContextUsage{Known: *u.Size > 0, Used: *u.Used, Size: *u.Size}
}
