package acp

import (
	"encoding/json"
	"strings"
	"sync"

	"opscopilot/pkg/assistantcontrol"
)

// The display feed consumes standard ACP observations only. It cannot dispatch
// tools, answer permission requests or change the hash-only native audit.
type executionFeed struct {
	mu     sync.Mutex
	items  map[string]assistantcontrol.Operation
	titles map[string]string
}

func (f *executionFeed) observe(turn *assistantcontrol.Turn, raw json.RawMessage) {
	if len(raw) > 8<<20 {
		return
	}
	var p struct {
		Update struct {
			ID      string                     `json:"toolCallId"`
			Title   string                     `json:"title"`
			Kind    string                     `json:"kind"`
			Status  string                     `json:"status"`
			Input   map[string]json.RawMessage `json:"rawInput"`
			Output  json.RawMessage            `json:"rawOutput"`
			Content []struct {
				Type    string `json:"type"`
				Content struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"content"`
		} `json:"update"`
	}
	if json.Unmarshal(raw, &p) != nil || p.Update.ID == "" || len(p.Update.ID) > 128 {
		return
	}
	u := p.Update
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.items == nil {
		f.items = map[string]assistantcontrol.Operation{}
		f.titles = map[string]string{}
	}
	op, known := f.items[u.ID]
	var script, skill string
	_ = json.Unmarshal(u.Input["script_name"], &script)
	_ = json.Unmarshal(u.Input["skill_name"], &skill)
	skillScript := script != "" && skill != ""
	skillRead := skill != "" && !skillScript
	if !known {
		// MCP workspace calls are kind=other and have richer host-owned receipts.
		// Questions/thinking have separate presentation; Skill reads and searches
		// are real observations and must remain visible.
		if len(f.items) >= 128 || (u.Kind != "execute" && u.Kind != "read" && u.Kind != "edit" && u.Kind != "search" && !skillScript && !skillRead) {
			return
		}
		op = assistantcontrol.Operation{Kind: "local", Location: "本机", Status: "running", Stage: map[string]string{"execute": "执行命令", "read": "读取文件", "edit": "写入文件"}[u.Kind]}
		if u.Kind == "search" {
			op.Stage = "检索资料"
		}
		if skillRead {
			op.Kind = "reference"
			op.Stage = "读取 Skill"
			op.Action = skill
		}
		if skillScript {
			op.Stage = "运行脚本"
			op.Action = skill + " / " + script
		}
	}
	if known && (op.Status == "completed" || op.Status == "failed") && u.Status != "completed" && u.Status != "failed" {
		return
	}
	if u.Title != "" {
		f.titles[u.ID] = assistantcontrol.DisplayExcerpt(u.Title, 512)
	}
	for _, key := range []string{"command", "path", "file_path", "resource_path", "query"} {
		var text string
		if json.Unmarshal(u.Input[key], &text) == nil && text != "" {
			op.Action = text
			break
		}
	}
	if op.Action == "" {
		op.Action = f.titles[u.ID]
	}
	var blocks []string
	for _, block := range u.Content {
		if block.Type == "content" && block.Content.Type == "text" && block.Content.Text != "" {
			blocks = append(blocks, block.Content.Text)
		}
	}
	if len(blocks) > 0 {
		op.Result = strings.Join(blocks, "\n")
	} else {
		var text string
		if json.Unmarshal(u.Output, &text) == nil && text != "" {
			op.Result = text
		}
	}
	switch u.Status {
	case "completed":
		op.Status = "completed"
	case "failed":
		op.Status = "failed"
	case "in_progress", "pending":
		op.Status = "running"
	}
	// Keep only the bounded display projection between progress updates.
	op.Action = assistantcontrol.DisplayExcerpt(op.Action, 4096)
	op.Result = assistantcontrol.DisplayExcerpt(op.Result, 6144)
	f.items[u.ID] = op
	turn.ObserveOperation("local-"+u.ID, op.Stage, op)
}
