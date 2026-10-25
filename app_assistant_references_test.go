package main

import (
	"context"
	"testing"

	"opscopilot/pkg/assistantskills"
	"opscopilot/pkg/config"
)

func TestAssistantReferenceCommandsShareUserSavedSources(t *testing.T) {
	a := &App{ctx: context.Background(), configMgr: config.NewManagerWithDir(t.TempDir())}
	if err := a.configMgr.AddQuickCommand(config.QuickCommand{ID: "logs", Name: "日志", Group: "支付", Content: "tail /var/log/app.log"}); err != nil {
		t.Fatal(err)
	}
	command := assistantskills.Command{Name: "openSql", Description: "打开数据库", Template: "连接 {{business}}", Fields: []assistantskills.Field{{Key: "business", Label: "业务", Kind: "text", Required: true}}}
	if err := a.SaveAssistantCommand(command); err != nil {
		t.Fatal(err)
	}
	items, err := a.assistantReferenceCommands()
	if err != nil || len(items) != 2 {
		t.Fatal(items, err)
	}
	if items[0].Kind != "terminal" || items[0].ID != "logs" || items[0].Content != "tail /var/log/app.log" || items[1].Kind != "assistant" || !items[1].Enabled || len(items[1].Fields) != 1 || !items[1].Fields[0].Required {
		t.Fatal(items)
	}
	items[0].Content = "mutated response"
	if a.configMgr.QuickCommandsSnapshot()[0].Content != "tail /var/log/app.log" {
		t.Fatal("response aliases saved config")
	}
	if err := a.RemoveAssistantCommand(command.Name); err != nil {
		t.Fatal(err)
	}
	items, err = a.assistantReferenceCommands()
	if err != nil || len(items) != 1 {
		t.Fatal("removed command remains visible", items, err)
	}
}
