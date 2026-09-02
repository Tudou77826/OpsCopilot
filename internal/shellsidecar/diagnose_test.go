package shellsidecar

import (
	"context"
	"errors"

	"opscopilot/pkg/recorder"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDiagnoseStartUnconfigured(t *testing.T) {
	t.Setenv("LLM_API_KEY", "")
	aiCfg, err := NewAIConfigService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewDiagnoseService(aiCfg, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Start(StartInput{Problem: "磁盘打满"}); !strings.Contains(err.Error(), "AI 未配置") {
		t.Fatalf("未配置时应报可读错误，得到 %v", err)
	}
	// 知识目录随服务创建
	if dir := svc.KnowledgeDir(); dir == "" {
		t.Fatal("知识目录应为数据目录下 knowledge/")
	}
}

func TestDiagnoseDispatchValidation(t *testing.T) {
	aiCfg, err := NewAIConfigService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewDiagnoseService(aiCfg, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	api := &ControlAPI{Diagnose: svc}
	params := []byte(`{"problem":"  "}`)
	if _, rpcErr := api.dispatch(context.Background(), &rpcRequest{Method: "shell.diagnose.start", Params: params}); rpcErr == nil {
		t.Fatal("空 problem 应报参数错误")
	}
	if _, rpcErr := api.dispatch(context.Background(), &rpcRequest{Method: "shell.diagnose.cancel", Params: []byte(`{"runId":"diag-unknown"}`)}); rpcErr != nil {
		t.Fatalf("未知 runId 取消应幂等成功，得到 %v", rpcErr)
	}
}

func TestDiagnoseCancelLifecycle(t *testing.T) {
	t.Setenv("LLM_API_KEY", "sk-diag-test-0000")
	// 端点指向不可达地址：任务会一直等待拨号/响应，期间取消应产生 canceled 事件。
	t.Setenv("LLM_BASE_URL", "http://127.0.0.1:1/v1")
	dataDir := t.TempDir()
	aiCfg, err := NewAIConfigService(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewDiagnoseService(aiCfg, dataDir, nil)
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var events []map[string]any
	svc.SetNotify(func(method string, params any) {
		if method != "shell.diagnose.event" {
			return
		}
		mu.Lock()
		events = append(events, params.(map[string]any))
		mu.Unlock()
	})

	start, err := svc.Start(StartInput{Problem: "磁盘打满如何排查"})
	if err != nil {
		t.Fatal(err)
	}
	runId := start.RunID
	if !strings.HasPrefix(runId, "diag-") {
		t.Fatalf("runId 应有 diag- 前缀，得到 %q", runId)
	}
	svc.Cancel(runId)

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := false
		for _, e := range events {
			if e["runId"] == runId && e["kind"] == "canceled" {
				done = true
			}
		}
		mu.Unlock()
		if done {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("取消后应收到 canceled 事件，收到: %v", events)
}

func TestDiagnoseKnowledgeDirCreated(t *testing.T) {
	dataDir := t.TempDir()
	aiCfg, err := NewAIConfigService(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewDiagnoseService(aiCfg, dataDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Join(svc.KnowledgeDir())); err != nil || !st.IsDir() {
		t.Fatalf("知识目录应已创建: %v", err)
	}
}

func TestDiagnoseCaseLifecycleAndArchive(t *testing.T) {
	dataDir := t.TempDir()
	aiCfg, err := NewAIConfigService(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	rec := recorder.NewRecorder(filepath.Join(dataDir, "recordings"))
	svc, err := NewDiagnoseService(aiCfg, dataDir, rec)
	if err != nil {
		t.Fatal(err)
	}
	if !svc.CasesAvailable() {
		t.Fatal("接入 recorder 后案例能力应可用")
	}

	// 启动诊断并绑定案例（AI 配置缺 key 会让 Start 报错——先落一个假 key）
	if _, err := aiCfg.Save(AIConfigUpdate{ApiKey: "sk-case-test-0000"}); err != nil {
		t.Fatal(err)
	}
	start, err := svc.Start(StartInput{
		Problem: "磁盘打满", TerminalID: "term-x", Host: "h1", User: "u1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if start.CaseID == "" {
		t.Fatal("无并发录制时案例应绑定成功")
	}
	// 终端键入进入排查时间线（走 recorder 原生入口，与脚本录制钩子同路径）
	if err := rec.RecordInput("term-x", "df -h"); err != nil {
		t.Fatal(err)
	}
	// 案例创建时按值拷贝会话，录制中的命令在 StopCase 重新拷贝时落进案例（与 Wails 同设计）；
	// 录制期间只断言活跃状态。
	if st := svc.CaseStatus(); !st.IsActive {
		t.Fatalf("案例状态应活跃，得到 %+v", st)
	}

	// 结束案例 → 落盘
	cs, err := svc.StopCase("日志未轮转占满", "结论：清理并配置轮转")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs.Commands) != 1 || cs.Commands[0].Content != "df -h" {
		t.Fatalf("案例应含录制的命令，得到 %+v", cs.Commands)
	}
	if svc.CaseStatus().IsActive {
		t.Fatal("结束后不应有活跃案例")
	}

	// 无已停止案例前归档应报错？——上面已停止，直接归档
	path, err := svc.Archive(ArchiveInputJSON{Service: "磁盘治理", Module: "通用", Conclusion: "结论：清理并配置轮转"})
	if err != nil {
		t.Fatalf("归档失败: %v", err)
	}
	if !strings.Contains(path, "archive") {
		t.Fatalf("归档应写入 archive/ 子目录，得到 %q", path)
	}
	// AppendRecord 返回相对知识目录的路径；文件本体在 knowledgeDir 下。
	absPath := filepath.Join(svc.KnowledgeDir(), path)
	if _, err := os.Stat(absPath); err != nil {
		t.Fatalf("归档文件应存在: %v", err)
	}
	// 归档记录是知识库视角的结论沉淀（标题/索引字段/结论原文），命令清单在案例 JSON 中持久化。
	data, _ := os.ReadFile(absPath)
	if !strings.Contains(string(data), "排查记录") || !strings.Contains(string(data), "清理并配置轮转") {
		t.Fatalf("归档记录应含标题与结论，得到: %s", string(data)[:min(200, len(data))])
	}
}

func TestDiagnoseArchiveWithoutStoppedCase(t *testing.T) {
	dataDir := t.TempDir()
	aiCfg, err := NewAIConfigService(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	rec := recorder.NewRecorder(filepath.Join(dataDir, "recordings"))
	svc, err := NewDiagnoseService(aiCfg, dataDir, rec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Archive(ArchiveInputJSON{Service: "s"}); err == nil {
		t.Fatal("无已停止案例时归档应报错")
	}
}

func TestDiagnoseWithoutRecorderDegrades(t *testing.T) {
	aiCfg, err := NewAIConfigService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewDiagnoseService(aiCfg, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if svc.CasesAvailable() {
		t.Fatal("未接 recorder 时案例能力应不可用")
	}
	if _, err := svc.StopCase("r", "c"); !errors.Is(err, errCaseUnavailable) {
		t.Fatalf("StopCase 应报案例不可用，得到 %v", err)
	}
	if svc.CaseStatus().IsActive {
		t.Fatal("无案例能力时状态应为非活跃")
	}
}
