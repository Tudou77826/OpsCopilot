package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parseSkillJSON 解析后端返回的 JSON 字符串，方便断言。
func parseSkillJSON(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("返回的不是合法 JSON: %v\nraw=%s", err, raw)
	}
	return m
}

func TestReadSkillTemplate_HasPlaceholders(t *testing.T) {
	tpl, err := readSkillTemplate()
	if err != nil {
		t.Fatalf("读取模板失败: %v", err)
	}
	if !strings.Contains(tpl, "{{OPSCOPILOT_BIN}}") {
		t.Errorf("模板缺少 {{OPSCOPILOT_BIN}} 占位符")
	}
}

func TestRenderSkillTemplate_ReplacesAll(t *testing.T) {
	// 模板中带引号的占位符（命令调用）和不带引号的（正文提及）都应被替换
	tpl := `call "{{OPSCOPILOT_BIN}}" exec, 详见 {{OPSCOPILOT_BIN}} 说明`
	out := renderSkillTemplate(tpl, `/some path/opscopilot.exe`)

	// 占位符应被全部替换，不留残余
	if strings.Contains(out, "{{OPSCOPILOT_BIN}}") {
		t.Errorf("渲染后仍含占位符: %q", out)
	}
	// 带引号占位符替换后应为 "/some path/opscopilot.exe"（路径反斜杠不被转义）
	if !strings.Contains(out, `"/some path/opscopilot.exe"`) {
		t.Errorf("带引号占位符替换结果不符: %q", out)
	}
	// Windows 路径的反斜杠不应被转义为 \\
	if strings.Contains(out, `\\`) {
		t.Errorf("路径反斜杠被错误转义: %q", out)
	}
	// 出现两次占位符都应被替换（一次带引号，一次不带）
	if c := strings.Count(out, `/some path/opscopilot.exe`); c != 2 {
		t.Errorf("期望两处 exe 路径替换，实际 %d 处: %q", c, out)
	}
}

func TestCheckSkillStatus_NotInstalled(t *testing.T) {
	dir := t.TempDir()
	app := &App{}
	raw := app.CheckSkillStatus(dir)
	m := parseSkillJSON(t, raw)

	if m["success"] != true {
		t.Fatalf("期望 success=true, 实际 %v", m["success"])
	}
	if m["state"] != "not_installed" {
		t.Errorf("期望 state=not_installed, 实际 %v", m["state"])
	}
	if m["builtin"] != Version {
		t.Errorf("期望 builtin=%s, 实际 %v", Version, m["builtin"])
	}
	if m["installed"] != "" {
		t.Errorf("期望 installed 为空, 实际 %v", m["installed"])
	}
}

func TestCheckSkillStatus_UpToDate(t *testing.T) {
	dir := t.TempDir()
	targetDir := skillDir(dir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	// 写入与当前 Version 一致的版本标记
	if err := os.WriteFile(filepath.Join(targetDir, skillVersionFile), []byte(Version), 0644); err != nil {
		t.Fatal(err)
	}

	app := &App{}
	raw := app.CheckSkillStatus(dir)
	m := parseSkillJSON(t, raw)

	if m["state"] != "up_to_date" {
		t.Errorf("期望 state=up_to_date, 实际 %v", m["state"])
	}
	if m["installed"] != Version {
		t.Errorf("期望 installed=%s, 实际 %v", Version, m["installed"])
	}
}

func TestCheckSkillStatus_Outdated(t *testing.T) {
	dir := t.TempDir()
	targetDir := skillDir(dir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	// 写入旧版本号
	if err := os.WriteFile(filepath.Join(targetDir, skillVersionFile), []byte("v0.0.1-old"), 0644); err != nil {
		t.Fatal(err)
	}

	app := &App{}
	raw := app.CheckSkillStatus(dir)
	m := parseSkillJSON(t, raw)

	if m["state"] != "outdated" {
		t.Errorf("期望 state=outdated, 实际 %v", m["state"])
	}
	if m["installed"] != "v0.0.1-old" {
		t.Errorf("期望 installed=v0.0.1-old, 实际 %v", m["installed"])
	}
}

func TestCheckSkillStatus_EmptyDir(t *testing.T) {
	app := &App{}
	raw := app.CheckSkillStatus("   ")
	m := parseSkillJSON(t, raw)
	if m["success"] != false {
		t.Errorf("空目录应返回失败, 实际 %v", m)
	}
}

func TestInstallSkill_CreatesFilesWithExePath(t *testing.T) {
	dir := t.TempDir()
	app := &App{}
	raw := app.InstallSkill(dir)
	m := parseSkillJSON(t, raw)

	if m["success"] != true {
		t.Fatalf("安装失败: %v", m)
	}

	// 校验生成的路径
	skillPath := filepath.Join(skillDir(dir), "SKILL.md")
	if m["path"] != skillPath {
		t.Errorf("期望 path=%s, 实际 %v", skillPath, m["path"])
	}
	if m["version"] != Version {
		t.Errorf("期望 version=%s, 实际 %v", Version, m["version"])
	}

	// SKILL.md 应存在且不含占位符
	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("读取生成的 SKILL.md 失败: %v", err)
	}
	s := string(content)
	if strings.Contains(s, "{{OPSCOPILOT_BIN}}") || strings.Contains(s, "{{OPSCOPILOT_VERSION}}") {
		t.Errorf("生成的 SKILL.md 仍含占位符:\n%s", s)
	}

	// 应包含当前 test binary 的绝对路径（os.Executable() 在测试中返回 go test 的临时 exe）
	exePath, _ := os.Executable()
	if !strings.Contains(s, exePath) {
		t.Errorf("生成的 SKILL.md 未包含当前 exe 路径 %q", exePath)
	}

	// 版本标记文件应与当前 Version 一致
	vData, err := os.ReadFile(filepath.Join(skillDir(dir), skillVersionFile))
	if err != nil {
		t.Fatalf("读取版本标记失败: %v", err)
	}
	if strings.TrimSpace(string(vData)) != Version {
		t.Errorf("版本标记 = %q, 期望 %s", string(vData), Version)
	}
}

func TestInstallSkill_IdempotentUpdate(t *testing.T) {
	dir := t.TempDir()
	app := &App{}

	// 第一次安装
	if m := parseSkillJSON(t, app.InstallSkill(dir)); m["success"] != true {
		t.Fatalf("首次安装失败: %v", m)
	}
	// 再次安装（覆盖）不应报错
	if m := parseSkillJSON(t, app.InstallSkill(dir)); m["success"] != true {
		t.Fatalf("二次安装失败: %v", m)
	}
	// 安装后状态应为 up_to_date
	if m := parseSkillJSON(t, app.CheckSkillStatus(dir)); m["state"] != "up_to_date" {
		t.Errorf("安装后状态应为 up_to_date, 实际 %v", m["state"])
	}
}

func TestInstallSkill_EmptyDir(t *testing.T) {
	app := &App{}
	raw := app.InstallSkill("   ")
	m := parseSkillJSON(t, raw)
	if m["success"] != false {
		t.Errorf("空目录应返回失败, 实际 %v", m)
	}
}

func TestWriteAtomic_ContentAndPerm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	want := []byte("hello atomic")

	if err := writeAtomic(path, want, 0644); err != nil {
		t.Fatalf("writeAtomic 失败: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("内容不一致: got %q want %q", got, want)
	}

	// 临时文件应已被清理
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("临时文件未清理: %v", err)
	}
}
