package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"opscopilot/pkg/connectionstore"
)

// newImportTestApp 构造只填充会话树管理器的 App，用于覆盖导入边界。
func newImportTestApp(t *testing.T, workDir string) *App {
	t.Helper()
	store := connectionstore.NewStoreWithPath(filepath.Join(workDir, "sessions.json"))
	if err := store.Load(); err != nil {
		t.Fatalf("加载会话树失败: %v", err)
	}
	return &App{savedSessionMgr: store}
}

const cleanSSHXsh = "[SessionInfo]\nVersion=8.1\n" +
	"[CONNECTION]\nProtocol=SSH\nHost=10.9.9.9\nPort=22\n" +
	"[CONNECTION:AUTHENTICATION]\nUserName=root\nPassword=\nUserKey=\n"

const unsupportedProtocolXsh = "[SessionInfo]\nVersion=8.1\n" +
	"[CONNECTION]\nProtocol=RLOGIN\nHost=10.9.9.10\nPort=513\n" +
	"[CONNECTION:AUTHENTICATION]\nUserName=ops\nPassword=\nUserKey=\n"

func writeSessionDir(t *testing.T, root, name, fileName, content string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("建目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(content), 0o644); err != nil {
		t.Fatalf("写夹具失败: %v", err)
	}
	return dir
}

// 导入结果里的集合字段必须是空集合而不是 null。
//
// Go 的 nil slice/map 会序列化成 null，前端按数组/对象处理就会抛渲染异常，
// 而 React 的渲染异常会卸载整棵组件树——实机上的表现是"点击『分析并预览』就黑屏"。
// 这个用例把契约钉在边界上，不再依赖前端容错。
func TestXshellImportResult_CollectionsAreNeverNull(t *testing.T) {
	root := t.TempDir()
	app := newImportTestApp(t, root)

	// 最干净的输入：能解析、不产生任何告警 —— warnings 应为空数组而非 null。
	cleanDir := writeSessionDir(t, root, "clean", "web-1.xsh", cleanSSHXsh)
	analysis, err := app.AnalyzeXshellImport(cleanDir, ImportOptions{})
	if err != nil {
		t.Fatalf("AnalyzeXshellImport(clean) 失败: %v", err)
	}
	assertJSONFieldIsEmptyCollection(t, "analysis(clean)", analysis, "warnings")
	assertJSONFieldIsNotEmpty(t, "analysis(clean)", analysis, "protocols")

	report, err := app.ApplyXshellImport(cleanDir, ImportOptions{})
	if err != nil {
		t.Fatalf("ApplyXshellImport(clean) 失败: %v", err)
	}
	assertJSONFieldIsEmptyCollection(t, "report(clean)", report, "warnings")

	// 全部协议都不支持：protocols 应为空对象而非 null（这是空 map 的触发路径）。
	unsupportedDir := writeSessionDir(t, root, "unsupported", "legacy-1.xsh", unsupportedProtocolXsh)
	analysis2, err := app.AnalyzeXshellImport(unsupportedDir, ImportOptions{})
	if err != nil {
		t.Fatalf("AnalyzeXshellImport(unsupported) 失败: %v", err)
	}
	assertJSONFieldIsEmptyCollection(t, "analysis(unsupported)", analysis2, "protocols")
	assertJSONFieldIsNotEmpty(t, "analysis(unsupported)", analysis2, "warnings")
}

// 空目录（没有任何 .xsh）也必须给出非 null 的 warnings。
func TestXshellImportResult_EmptyDirectoryStillReturnsCollections(t *testing.T) {
	root := t.TempDir()
	app := newImportTestApp(t, root)

	analysis, err := app.AnalyzeXshellImport(root, ImportOptions{})
	if err != nil {
		t.Fatalf("AnalyzeXshellImport 失败: %v", err)
	}
	assertJSONFieldIsNotEmpty(t, "analysis(空目录)", analysis, "warnings")
	assertJSONFieldIsNotEmpty(t, "analysis(空目录)", analysis, "protocols")
}

// 导入结果不能因为集合字段是 null 而让前端崩溃——这里用与前端相同的解构方式复算一遍：
// 只要字段是 null，前端的 .length / Object.keys 就会抛异常。
func TestXshellImportResult_MatchesFrontendExpectations(t *testing.T) {
	root := t.TempDir()
	app := newImportTestApp(t, root)
	dir := writeSessionDir(t, root, "clean", "web-1.xsh", cleanSSHXsh)

	analysis, err := app.AnalyzeXshellImport(dir, ImportOptions{})
	if err != nil {
		t.Fatalf("AnalyzeXshellImport 失败: %v", err)
	}

	payload := toJSONMap(t, analysis)
	warnings, ok := payload["warnings"].([]any)
	if !ok {
		t.Fatalf("warnings 不是数组（前端会抛渲染异常）: %#v", payload["warnings"])
	}
	protocols, ok := payload["protocols"].(map[string]any)
	if !ok {
		t.Fatalf("protocols 不是对象（前端会抛渲染异常）: %#v", payload["protocols"])
	}
	// 前端会做 warnings.map(...) 与 Object.entries(protocols)，两者对空集合都安全。
	_ = warnings
	_ = protocols
}

func toJSONMap(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	return decoded
}

// assertJSONFieldIsEmptyCollection 断言字段存在、不是 null，且是空集合。
func assertJSONFieldIsEmptyCollection(t *testing.T, label string, value any, field string) {
	t.Helper()
	payload := toJSONMap(t, value)
	got, ok := payload[field]
	if !ok {
		t.Errorf("%s 缺少字段 %s", label, field)
		return
	}
	if got == nil {
		t.Errorf("%s 的 %s 是 null；前端按集合处理会抛渲染异常", label, field)
		return
	}
	switch typed := got.(type) {
	case []any:
		if len(typed) != 0 {
			t.Errorf("%s 的 %s 期望空集合，实际 %#v", label, field, typed)
		}
	case map[string]any:
		if len(typed) != 0 {
			t.Errorf("%s 的 %s 期望空集合，实际 %#v", label, field, typed)
		}
	default:
		t.Errorf("%s 的 %s 不是集合: %#v", label, field, got)
	}
}

// assertJSONFieldIsNotEmpty 断言字段存在且不是 null（内容不限）。
func assertJSONFieldIsNotEmpty(t *testing.T, label string, value any, field string) {
	t.Helper()
	payload := toJSONMap(t, value)
	got, ok := payload[field]
	if !ok {
		t.Errorf("%s 缺少字段 %s", label, field)
		return
	}
	if got == nil {
		t.Errorf("%s 的 %s 是 null；前端按集合处理会抛渲染异常", label, field)
	}
}
