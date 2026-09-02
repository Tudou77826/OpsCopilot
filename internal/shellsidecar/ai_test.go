package shellsidecar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"opscopilot/pkg/completion"
)

func TestAIConfigSaveAndGetMasked(t *testing.T) {
	service, err := NewAIConfigService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const plaintext = "sk-1234567890abcdef"
	status, err := service.Save(AIConfigUpdate{
		ApiKey:       plaintext,
		BaseURL:      "https://llm.example.com/v1",
		FastModel:    "fast-1",
		ComplexModel: "complex-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Configured {
		t.Fatal("保存密钥后 configured 应为 true")
	}
	if status.KeyHint != "…cdef" {
		t.Fatalf("keyHint 应为尾端 4 字符提示，得到 %q", status.KeyHint)
	}
	if status.Source != aiSourceFile {
		t.Fatalf("source 应为 file，得到 %q", status.Source)
	}
	// 脱敏断言：状态 JSON 序列化结果不得包含明文密钥。
	data, _ := json.Marshal(status)
	if strings.Contains(string(data), plaintext) {
		t.Fatalf("getConfig 响应泄露明文密钥: %s", data)
	}
}

func TestAIConfigEmptyKeyKeepsExisting(t *testing.T) {
	service, err := NewAIConfigService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Save(AIConfigUpdate{
		ApiKey:       "sk-original-key-9999",
		BaseURL:      "https://llm.example.com/v1",
		ComplexModel: "complex-1",
	}); err != nil {
		t.Fatal(err)
	}
	// 不带密钥的保存（用户只改模型）：密钥与其余未提供字段均保留。
	status, err := service.Save(AIConfigUpdate{ApiKey: "", FastModel: "fast-2"})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Configured || status.KeyHint != "…9999" {
		t.Fatalf("空 apiKey 保存应保留原密钥，得到 configured=%v hint=%q", status.Configured, status.KeyHint)
	}
	if status.FastModel != "fast-2" {
		t.Fatalf("模型应已更新，得到 %q", status.FastModel)
	}
	if status.BaseURL != "https://llm.example.com/v1" || status.ComplexModel != "complex-1" {
		t.Fatalf("未提供的字段应保留原值，得到 baseURL=%q complex=%q", status.BaseURL, status.ComplexModel)
	}
}

func TestAIConfigCorruptFileFallsBack(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "ai-config.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	service, err := NewAIConfigService(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	status := service.Status()
	if status.Configured {
		t.Fatal("损坏文件应回退为未配置")
	}
	if status.BaseURL != aiDefaultBaseURL || status.FastModel != aiDefaultModel {
		t.Fatalf("损坏文件应回退默认端点/模型，得到 %+v", status)
	}
	if status.Source != aiSourceNone {
		t.Fatalf("source 应为 none，得到 %q", status.Source)
	}
}

func TestAIConfigEnvFallback(t *testing.T) {
	t.Setenv("LLM_API_KEY", "sk-env-key-abcdef")
	t.Setenv("LLM_BASE_URL", "https://env.example.com/v1")
	service, err := NewAIConfigService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	status := service.Status()
	if !status.Configured || status.Source != aiSourceEnv {
		t.Fatalf("应经环境变量回退为已配置，得到 %+v", status)
	}
	if status.KeyHint != "…cdef" {
		t.Fatalf("env 密钥提示错误: %q", status.KeyHint)
	}
	if status.BaseURL != "https://env.example.com/v1" {
		t.Fatalf("env 端点应生效，得到 %q", status.BaseURL)
	}
}

func TestShellCompletionDispatch(t *testing.T) {	db, err := completion.NewDatabase()
	if err != nil {
		t.Skipf("补全数据库初始化失败: %v", err)
	}
	api := &ControlAPI{Completion: completion.NewService(db)}
	// pkg/completion 既有行为：完整命令名命中后返回选项建议（与 Wails 链一致）。
	params, _ := json.Marshal(map[string]any{"input": "grep", "cursor": 4})
	result, rpcErr := api.dispatch(context.Background(), &rpcRequest{Method: "shell.completion", Params: params})
	if rpcErr != nil {
		t.Fatalf("shell.completion 失败: %v", rpcErr)
	}
	resp, ok := result.(*completion.CompletionResponse)
	if !ok || resp == nil || len(resp.Suggestions) == 0 {
		t.Fatalf("grep 应产生补全建议，得到 %+v", result)
	}
	// 结果 JSON 形态须与共享 CompletionData 同构（snake_case 字段）。
	data, _ := json.Marshal(resp)
	for _, field := range []string{`"suggestions"`, `"replace_from"`, `"replace_to"`, `"display_text"`} {
		if !strings.Contains(string(data), field) {
			t.Fatalf("补全结果缺少字段 %s: %s", field, data)
		}
	}
	// 未接线时报 notEnabled。
	empty := &ControlAPI{}
	if _, rpcErr := empty.dispatch(context.Background(), &rpcRequest{Method: "shell.completion", Params: params}); rpcErr == nil {
		t.Fatal("Completion 未接线时应报错")
	}
}

func TestAIGenerateCommandUnconfigured(t *testing.T) {
	t.Setenv("LLM_API_KEY", "")
	service, err := NewAIConfigService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GenerateCommand("列出所有文件"); !errors.Is(err, errAINotConfigured) {
		t.Fatalf("未配置时应报 errAINotConfigured，得到 %v", err)
	}
	if _, err := service.ParseConnectIntent("连到 1.2.3.4"); !errors.Is(err, errAINotConfigured) {
		t.Fatalf("未配置时应报 errAINotConfigured，得到 %v", err)
	}
}

func TestAIGenerateCommandDispatchValidation(t *testing.T) {
	service, err := NewAIConfigService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	api := &ControlAPI{AI: service}
	// 空 request → 参数错误（而非穿透到 LLM）
	params, _ := json.Marshal(map[string]any{"request": "  "})
	if _, rpcErr := api.dispatch(context.Background(), &rpcRequest{Method: "shell.ai.generateCommand", Params: params}); rpcErr == nil {
		t.Fatal("空 request 应报参数错误")
	}
	// 未配置密钥 → 服务端错误（errAINotConfigured 文案对用户可读）
	params2, _ := json.Marshal(map[string]any{"request": "列出文件"})
	_, rpcErr := api.dispatch(context.Background(), &rpcRequest{Method: "shell.ai.generateCommand", Params: params2})
	if rpcErr == nil {
		t.Fatal("未配置时应报错")
	}
	if !strings.Contains(fmt.Sprint(rpcErr), "AI 未配置") {
		t.Fatalf("错误文案应可读，得到 %v", rpcErr)
	}
	// parseIntent 空 input 同理
	params3, _ := json.Marshal(map[string]any{"input": ""})
	if _, rpcErr := api.dispatch(context.Background(), &rpcRequest{Method: "shell.ai.parseIntent", Params: params3}); rpcErr == nil {
		t.Fatal("空 input 应报参数错误")
	}
}

func TestSettingsCommandQueryShortcutDefault(t *testing.T) {
	service, err := NewSettingsService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// 无文件：取默认
	s, err := service.Get()
	if err != nil {
		t.Fatal(err)
	}
	if s.CommandQueryShortcut != "Ctrl+K" {
		t.Fatalf("默认快捷键应为 Ctrl+K，得到 %q", s.CommandQueryShortcut)
	}
	// 保存自定义值后回读
	s.CommandQueryShortcut = "Ctrl+Shift+P"
	if err := service.Save(s); err != nil {
		t.Fatal(err)
	}
	got, err := service.Get()
	if err != nil {
		t.Fatal(err)
	}
	if got.CommandQueryShortcut != "Ctrl+Shift+P" {
		t.Fatalf("快捷键应保持，得到 %q", got.CommandQueryShortcut)
	}
	// 旧文件无该字段：回退默认
	path := filepath.Join(t.TempDir(), "shell-settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark","completionDelay":100}`), 0o644); err != nil {
		t.Fatal(err)
	}
	svc2, err := NewSettingsService(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	// NewSettingsService 用传入目录；直接构造读取
	got2, err := svc2.Get()
	if err != nil {
		t.Fatal(err)
	}
	if got2.CommandQueryShortcut != "Ctrl+K" {
		t.Fatalf("旧文件缺字段应回退默认，得到 %q", got2.CommandQueryShortcut)
	}
}
