package assistanttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

type callReceiptKey struct{}

func hasCallReceipt(ctx context.Context) bool { return ctx.Value(callReceiptKey{}) == true }

// Calls with resource/task-specific receipts keep those richer records.
func hasDetailedReceipt(name string) bool {
	switch name {
	case "terminal_execute", "database_execute", "database_open", "file_open", "file_transfer":
		return true
	}
	return false
}

var callTitles = map[string]string{
	"terminal_execute": "执行远程命令", "database_execute": "执行 SQL", "database_open": "打开数据库页签", "file_open": "打开文件页签", "file_transfer": "传输文件",
	"workspace_list": "查看终端与连接", "workspace_open": "打开终端页签", "workspace_select": "切换终端页签",
	"terminal_context": "读取终端屏幕", "terminal_take": "接管终端", "terminal_release": "交还终端",
	"database_list": "查看数据库与驱动", "database_create": "保存数据库连接", "database_take": "接管数据库页签", "database_release": "交还数据库页签", "database_context": "查看数据库上下文", "database_tables": "查询数据表", "database_structure": "查询表结构", "database_close": "关闭数据库连接",
	"file_workspaces": "查看文件工作区", "file_take": "接管文件页签", "file_release": "交还文件页签", "file_tasks": "查看传输任务", "file_list": "浏览文件目录", "file_cancel": "取消文件传输",
	"local_directories": "查看本地项目目录", "local_add_directories": "添加本地项目目录",
	"knowledge_list": "浏览定位经验", "knowledge_search": "搜索定位经验", "knowledge_read": "阅读定位经验",
	"quick_command_list": "查询快捷指令列表", "quick_command_read": "读取快捷指令",
}

func (w *WorkspaceTools) beginCallReceipt(name, argument string) func(any, error) {
	if hasDetailedReceipt(name) {
		revision, start := w.turn.OperationRevision(), time.Now()
		return func(result any, err error) {
			if w.turn.OperationRevision() == revision {
				w.newCallReceipt(name, argument, start)(result, err)
			}
		}
	}
	return w.newCallReceipt(name, argument, time.Now())
}

func (w *WorkspaceTools) newCallReceipt(name, argument string, start time.Time) func(any, error) {
	title := callTitles[name]
	if title == "" {
		title = "应用工具调用"
	}
	key := fmt.Sprintf("call-%d", w.receiptSequence.Add(1))
	if name == "file_tasks" {
		key = "file-task-status"
	}
	op := assistantcontrol.Operation{Kind: "application", Status: "running", Stage: title, Action: title, Location: "OpsCopilot"}
	if strings.HasPrefix(name, "knowledge_") {
		op.Kind = "knowledge"
	}
	if strings.HasPrefix(name, "quick_command_") {
		op.Kind = "reference"
	}
	var args map[string]json.RawMessage
	_ = json.Unmarshal([]byte(argument), &args)
	// Whitelist read selectors; never copy arbitrary arguments or connection forms.
	labels := map[string]string{"query": "检索词", "service": "服务", "module": "模块", "database": "数据库", "table": "表", "path": "路径", "side": "位置"}
	for _, field := range []string{"query", "service", "module", "database", "table", "path", "side"} {
		var value string
		if json.Unmarshal(args[field], &value) == nil && value != "" {
			op.Action += "\n" + labels[field] + "：" + value
		}
	}
	if strings.HasSuffix(name, "_take") || strings.HasSuffix(name, "_release") {
		op.Action = ""
	}
	w.turn.ObserveOperation(key, title, op)
	return func(result any, err error) {
		op.Measured, op.DurationMs = true, time.Since(start).Milliseconds()
		op.Status = "completed"
		if err != nil {
			op.Status, op.Detail, op.Outcome, op.Next = "failed", err.Error(), "本次调用未完成。", "核对原因后再继续。"
		} else {
			op.Result = callResultSummary(name, result)
		}
		if prerequisite, ok := result.(map[string]string); ok && strings.HasPrefix(prerequisite["code"], "ssh_") {
			op.Status, op.Result = "not_executed", prerequisite["message"]
		}
		w.turn.ObserveOperation(key, title, op)
	}
}

func callResultSummary(name string, result any) string {
	encoded, err := json.Marshal(result)
	if err != nil {
		return "调用已完成"
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(encoded, &fields) != nil {
		var rows []json.RawMessage
		if json.Unmarshal(encoded, &rows) == nil {
			return fmt.Sprintf("返回 %d 项", len(rows))
		}
		return "调用已完成"
	}
	if name == "quick_command_read" {
		var c ReferenceCommand
		if json.Unmarshal(fields["command"], &c) == nil {
			return c.Name + "\n" + c.Content
		}
	}
	if name == "knowledge_read" {
		var title, source, content string
		_ = json.Unmarshal(fields["title"], &title)
		_ = json.Unmarshal(fields["source"], &source)
		_ = json.Unmarshal(fields["content"], &content)
		return title + " · " + source + "\n" + content
	}
	var parts []string
	var count int
	if raw, ok := fields["total"]; ok && json.Unmarshal(raw, &count) == nil {
		parts = append(parts, fmt.Sprintf("共 %d 条", count))
	}
	labels := map[string]string{"items": "本页", "hits": "匹配经验", "weak_matches": "相近经验", "tabs": "页签", "connections": "连接", "profiles": "数据库配置", "drivers": "驱动", "gateways": "跳板节点", "projects": "项目", "directories": "目录", "entries": "目录条目", "files": "文件", "tables": "数据表", "columns": "字段", "tasks": "任务"}
	for _, key := range []string{"items", "hits", "weak_matches", "tabs", "connections", "profiles", "drivers", "gateways", "projects", "directories", "entries", "files", "tables", "columns", "tasks"} {
		var entries []json.RawMessage
		if json.Unmarshal(fields[key], &entries) != nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s：%d 项", labels[key], len(entries)))
		if key == "items" || key == "hits" || key == "tables" {
			for i, row := range entries {
				if i >= 10 {
					break
				}
				var item struct {
					Name  string `json:"name"`
					Title string `json:"title"`
				}
				if json.Unmarshal(row, &item) == nil {
					if item.Name != "" {
						parts = append(parts, item.Name)
					} else if item.Title != "" {
						parts = append(parts, item.Title)
					}
				}
			}
		}
	}
	if len(parts) == 0 {
		return "调用已完成"
	}
	return strings.Join(parts, "\n")
}
