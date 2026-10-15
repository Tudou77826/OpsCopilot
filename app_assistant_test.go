package main

import (
	"context"
	"encoding/json"
	"opscopilot/pkg/ai"
	"opscopilot/pkg/assistantsettings"
	"opscopilot/pkg/companion"
	"opscopilot/pkg/config"
	"opscopilot/pkg/llm"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssistantPersonaPersistsAndReachesOnlyCompanionRequest(t *testing.T) {
	mgr := config.NewManagerWithDir(t.TempDir())
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	provider := &llm.MockProvider{Response: "回答"}
	app := &App{ctx: context.Background(), configMgr: mgr, aiService: ai.NewAIService(provider, provider, mgr)}
	original, err := app.GetAssistantSettings()
	if err != nil {
		t.Fatal(err)
	}
	custom := "你叫小圆，先给结论，再简短解释。"
	saved, err := app.SaveAssistantSettings(custom, original.Revision)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := assistantsettings.New(filepath.Join(mgr.Directory(), "assistant.json")).Read()
	if err != nil || reopened.PersonaPrompt != custom {
		t.Fatal("preference did not persist", err)
	}
	if got := app.AskCompanionAI("你好"); got != "回答" {
		t.Fatal(got)
	}
	system := provider.LastMessages[0].Content
	if !strings.Contains(system, custom) || !strings.Contains(system, "功能边界") || strings.Contains(system, "空steps") {
		t.Fatal("persona or plain Q&A rules missing", system)
	}
	if got := app.AskAI("一般知识问答"); got != "回答" {
		t.Fatal(got)
	}
	if strings.Contains(provider.LastMessages[0].Content, custom) {
		t.Fatal("persona leaked into legacy callers")
	}
	if _, err = app.SaveAssistantSettings("new", original.Revision); err == nil {
		t.Fatal("accepted stale settings")
	}
	if !strings.Contains(app.AskCompanionAI("再次提问"), "回答") {
		t.Fatal("stale save changed runtime")
	}
	if _, err = app.SaveAssistantSettings("", saved.Revision); err != nil {
		t.Fatal(err)
	}
	app.AskCompanionAI("恢复默认后")
	if !strings.Contains(provider.LastMessages[0].Content, assistantsettings.DefaultPersona) || strings.Contains(provider.LastMessages[0].Content, custom) {
		t.Fatal("default reset did not reach model")
	}
}
func TestHatchedSpeciesPersonaKeepsCustomOverride(t *testing.T) {
	mgr := config.NewManagerWithDir(t.TempDir())
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	provider := &llm.MockProvider{Response: "回答"}
	pet := companion.NewDormant(filepath.Join(mgr.Directory(), "companion.json"))
	pet.SetEnabled(true)
	app := &App{ctx: context.Background(), configMgr: mgr, companion: pet, aiService: ai.NewAIService(provider, provider, mgr)}
	snap, err := pet.Hatch(0)
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := app.GetAssistantSettings()
	if settings.DefaultPrompt != companion.PersonaFor(snap.Pet.SpeciesID) {
		t.Fatal("wrong preset")
	}
	app.AskCompanionAI("你好")
	if !strings.Contains(provider.LastMessages[0].Content, companion.PersonaFor(snap.Pet.SpeciesID)) {
		t.Fatal("species persona missing")
	}
	saved, err := app.SaveAssistantSettings("自定义性格优先", settings.Revision)
	if err != nil {
		t.Fatal(err)
	}
	// Only this isolated test fixture is funded; the public API never accepts XP.
	savePath := filepath.Join(mgr.Directory(), "companion.json")
	raw, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatal(err)
	}
	var fixture map[string]any
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	fixture["pet"].(map[string]any)["xp"] = companion.RehatchCost
	raw, err = json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(savePath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	previousSpecies := snap.Pet.SpeciesID
	snap, err = app.CompanionHatch(snap.Revision)
	if err != nil || snap.Pet.SpeciesID == previousSpecies {
		t.Fatal("paid hatch did not change species", err)
	}
	app.AskCompanionAI("切换后")
	if !strings.Contains(provider.LastMessages[0].Content, "自定义性格优先") || strings.Contains(provider.LastMessages[0].Content, companion.PersonaFor(snap.Pet.SpeciesID)) {
		t.Fatal("custom persona overwritten")
	}
	if _, err = app.SaveAssistantSettings(companion.PersonaFor(snap.Pet.SpeciesID), saved.Revision); err != nil {
		t.Fatal(err)
	}
	app.AskCompanionAI("恢复默认")
	if !strings.Contains(provider.LastMessages[0].Content, companion.PersonaFor(snap.Pet.SpeciesID)) {
		t.Fatal("reset did not follow species")
	}
	current, _ := app.GetAssistantSettings()
	neutral, err := app.SaveAssistantSettings(assistantsettings.DefaultPersona, current.Revision)
	if err != nil || neutral.PersonaPrompt != assistantsettings.DefaultPersona {
		t.Fatal("explicit neutral override was lost", err)
	}
	app.AskCompanionAI("通用性格")
	if !strings.Contains(provider.LastMessages[0].Content, assistantsettings.DefaultPersona) || strings.Contains(provider.LastMessages[0].Content, companion.PersonaFor(snap.Pet.SpeciesID)) {
		t.Fatal("neutral override did not win")
	}
}
