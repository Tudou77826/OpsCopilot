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

// writeSkillWithVersion 构造一个已安装的 SKILL.md，frontmatter 带 version 字段。
func writeSkillWithVersion(t *testing.T, dir, version string) {
	t.Helper()
	targetDir := skillDir(dir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: opscopilot-ops\nversion: '" + version + "'\ndescription: test\n---\n# body\n"
	if err := os.WriteFile(filepath.Join(targetDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestReadSkillTemplate_HasPlaceholders(t *testing.T) {
	tpl, err := readSkillTemplate()
	if err != nil {
		t.Fatalf("读取模板失败: %v", err)
	}
	if !strings.Contains(tpl, "{{OPSCOPILOT_BIN}}") {
		t.Errorf("模板缺少 {{OPSCOPILOT_BIN}} 占位符")
	}
	if !strings.Contains(tpl, "{{OPSCOPILOT_VERSION}}") {
		t.Errorf("模板缺少 {{OPSCOPILOT_VERSION}} 占位符")
	}
}

func TestRenderSkillTemplate_ReplacesAll(t *testing.T) {
	// 模板中带引号的占位符（命令调用）、不带引号的（正文提及）、版本号都应被替换
	tpl := `version: '{{OPSCOPILOT_VERSION}}' call "{{OPSCOPILOT_BIN}}" exec, 详见 {{OPSCOPILOT_BIN}} 说明`
	out := renderSkillTemplate(tpl, `/some path/opscopilot.exe`)

	// 占位符应被全部替换，不留残余
	if strings.Contains(out, "{{OPSCOPILOT_BIN}}") || strings.Contains(out, "{{OPSCOPILOT_VERSION}}") {
		t.Errorf("渲染后仍含占位符: %q", out)
	}
	// 版本号应被替换为当前 Version
	if !strings.Contains(out, "version: '"+Version+"'") {
		t.Errorf("版本号未被正确替换: %q", out)
	}
	// 带引号占位符替换后应为 "/some path/opscopilot.exe"（路径反斜杠不被转义）
	if !strings.Contains(out, `"/some path/opscopilot.exe"`) {
		t.Errorf("带引号占位符替换结果不符: %q", out)
	}
	// Windows 路径的反斜杠不应被转义为 \\
	if strings.Contains(out, `\\`) {
		t.Errorf("路径反斜杠被错误转义: %q", out)
	}
	// exe 路径应出现两处（一次带引号，一次不带）
	if c := strings.Count(out, `/some path/opscopilot.exe`); c != 2 {
		t.Errorf("期望两处 exe 路径替换，实际 %d 处: %q", c, out)
	}
}

func TestParseSkillVersion(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "带单引号",
			content: "---\nname: x\nversion: '1.8.0'\ndescription: y\n---\n# body",
			want:    "1.8.0",
		},
		{
			name:    "带双引号",
			content: "---\nname: x\nversion: \"1.8.0\"\ndescription: y\n---\n# body",
			want:    "1.8.0",
		},
		{
			name:    "无引号",
			content: "---\nname: x\nversion: 1.8.0\ndescription: y\n---\n# body",
			want:    "1.8.0",
		},
		{
			name:    "无 frontmatter",
			content: "# just a markdown\nno frontmatter here",
			want:    "",
		},
		{
			name:    "frontmatter 无 version",
			content: "---\nname: x\ndescription: y\n---\n# body",
			want:    "",
		},
		{
			name:    "正文里的 version 字样不应被误读",
			content: "---\nname: x\nversion: '1.8.0'\n---\nversion: 9.9.9",
			want:    "1.8.0",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSkillVersion(tc.content)
			if got != tc.want {
				t.Errorf("parseSkillVersion = %q, 期望 %q", got, tc.want)
			}
		})
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
	writeSkillWithVersion(t, dir, Version) // 与当前 Version 一致

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
	writeSkillWithVersion(t, dir, "v0.0.1-old")

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

func TestInstallSkill_CreatesFileWithExePathAndVersion(t *testing.T) {
	dir := t.TempDir()
	app := &App{}
	raw := app.InstallSkill(dir)
	m := parseSkillJSON(t, raw)

	if m["success"] != true {
		t.Fatalf("安装失败: %v", m)
	}

	skillPath := filepath.Join(skillDir(dir), "SKILL.md")
	if m["path"] != skillPath {
		t.Errorf("期望 path=%s, 实际 %v", skillPath, m["path"])
	}
	if m["version"] != Version {
		t.Errorf("期望 version=%s, 实际 %v", Version, m["version"])
	}

	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("读取生成的 SKILL.md 失败: %v", err)
	}
	s := string(content)

	// 不应含任何占位符
	if strings.Contains(s, "{{OPSCOPILOT_BIN}}") || strings.Contains(s, "{{OPSCOPILOT_VERSION}}") {
		t.Errorf("生成的 SKILL.md 仍含占位符:\n%s", s)
	}

	// 应在 frontmatter 含当前版本号
	parsed := parseSkillVersion(s)
	if parsed != Version {
		t.Errorf("生成文件的 version 字段 = %q, 期望 %s", parsed, Version)
	}

	// 应包含当前 exe 的绝对路径（os.Executable() 在测试中返回 go test 的临时 exe）
	exePath, _ := os.Executable()
	if !strings.Contains(s, exePath) {
		t.Errorf("生成的 SKILL.md 未包含当前 exe 路径 %q", exePath)
	}

	// 不应再生成独立的 .version 文件
	if _, err := os.Stat(filepath.Join(skillDir(dir), ".version")); !os.IsNotExist(err) {
		t.Errorf("不应再生成 .version 文件")
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

// 兼容旧安装：CheckSkillStatus 应能处理仅有 .version 文件、无 frontmatter version 的旧 SKILL.md。
// 这种情况解析出空 version，归为 outdated（提示可更新）。
func TestCheckSkillStatus_LegacyNoFrontmatterVersion(t *testing.T) {
	dir := t.TempDir()
	targetDir := skillDir(dir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	// 模拟旧版 SKILL.md：无 frontmatter version 字段
	legacy := "# OpsCopilot\n\n无 version 字段的旧文件"
	if err := os.WriteFile(filepath.Join(targetDir, "SKILL.md"), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	app := &App{}
	raw := app.CheckSkillStatus(dir)
	m := parseSkillJSON(t, raw)

	// 旧文件无 version → 解析为空 → 归为 outdated（可更新）
	if m["state"] != "outdated" {
		t.Errorf("无 version 的旧文件应归为 outdated, 实际 %v", m["state"])
	}
	if m["installed"] != "" {
		t.Errorf("旧文件 installed 应为空, 实际 %v", m["installed"])
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
