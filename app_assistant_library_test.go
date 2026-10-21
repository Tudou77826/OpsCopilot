package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/assistantskills"
	"opscopilot/pkg/config"
)

func TestAssistantLibraryPreparePreservesEditedProseAndValidatesRequiredFields(t *testing.T) {
	a := &App{ctx: context.Background(), configMgr: config.NewManagerWithDir(t.TempDir())}
	source := t.TempDir()
	os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: discovery\ndescription: Discover resource endpoints.\n---\nUse the application tools.\n"), 0600)
	if err := a.ImportAssistantSkill(source, false, ""); err != nil {
		t.Fatal(err)
	}
	command := assistantskills.Command{Name: "sqlSessionInfoGetAndCreate", Description: "打开查询", Skill: "discovery", Template: "查询 {{business}}", Fields: []assistantskills.Field{{Key: "business", Label: "业务", Kind: "text", Required: true}}}
	if err := a.SaveAssistantCommand(command); err != nil {
		t.Fatal(err)
	}
	if _, err := a.PrepareAssistantCommand(command.Name, command.Template, nil); err == nil {
		t.Fatal("missing required field accepted")
	}
	question, err := a.PrepareAssistantCommand(command.Name, "查询 {{business}}，仅看今天的数据", map[string]string{"business": "订单"})
	if err != nil || !strings.Contains(question, "Skill「discovery」") || !strings.Contains(question, "查询 订单，仅看今天的数据") {
		t.Fatal(question, err)
	}
	if _, err = a.PrepareAssistantCommand(command.Name, "查询 {{unknown}}", map[string]string{"unknown": "订单"}); err == nil {
		t.Fatal("edited unknown slot accepted")
	}
	if err = a.EnableAssistantSkill("discovery", false); err != nil {
		t.Fatal(err)
	}
	if _, err = a.PrepareAssistantCommand(command.Name, command.Template, map[string]string{"business": "订单"}); err == nil {
		t.Fatal("disabled Skill dispatched")
	}
	command.Skill = ""
	command.Fields[0].Kind = "connection"
	if err = a.SaveAssistantCommand(command); err != nil {
		t.Fatal(err)
	}
	if _, err = a.PrepareAssistantCommand(command.Name, command.Template, map[string]string{"business": "missing-id"}); err == nil {
		t.Fatal("invented saved connection accepted")
	}
}
func TestAssistantLibraryMutationCannotRaceActiveTurn(t *testing.T) {
	a := &App{ctx: context.Background(), configMgr: config.NewManagerWithDir(t.TempDir())}
	turn, _ := a.assistantCoordinator().Begin(a.ctx, assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	command := assistantskills.Command{Name: "hello", Description: "问候", Template: "你好"}
	if err := a.SaveAssistantCommand(command); !errors.Is(err, assistantcontrol.ErrBusy) {
		t.Fatal(err)
	}
	if _, err := a.AssistantLibrary(); err != nil {
		t.Fatal("read should remain available", err)
	}
	turn.Finish("completed", "")
	if err := a.SaveAssistantCommand(command); err != nil {
		t.Fatal(err)
	}
	if question, err := a.PrepareAssistantCommand(command.Name, command.Template, nil); err != nil || question != "你好" {
		t.Fatal(question, err)
	}
}
