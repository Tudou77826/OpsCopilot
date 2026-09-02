package shellsidecar

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"opscopilot/pkg/ai"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/recorder"
	"opscopilot/pkg/troubleshoot"
)

// DiagnoseService：AI 故障诊断的 sidecar 侧长任务服务（迭代 C）。
//
// 形态（方案 §3.1）：异步可取消长任务 + JSON-RPC notification 流式推送。
// 事件契约（中性形状，遗留 R7）：method=shell.diagnose.event，
// params={runId, kind: status|context|token|done|concl-done|canceled|error,
// stage?, message?, usedTokens?, maxTokens?, result?, text?}——
// pkg/ai 的内部工具调用轨迹不进入契约。
// 知识目录固定为数据目录 knowledge/（markdown + catalog 构建），随诊断启动重建；
// 案例经 pkg/troubleshoot 持久化（与脚本服务共享 recorder，终端键入进入排查时间线）。
type DiagnoseService struct {
	ai           *AIConfigService
	knowledgeDir string
	// cases 为 nil 时案例能力不可用（未接脚本服务的 recorder）。
	cases *troubleshoot.Manager

	mu              sync.Mutex
	runs            map[string]context.CancelFunc
	lastStoppedCase *troubleshoot.Case

	notify func(method string, params any)
}

type diagnoseRunKey struct{}

// StartInput 是 shell.diagnose.start 的参数；terminalId/host/user 供案例绑定录制会话。
type StartInput struct {
	Problem    string   `json:"problem"`
	TerminalID string   `json:"terminalId,omitempty"`
	Host       string   `json:"host,omitempty"`
	User       string   `json:"user,omitempty"`
	Context    []string `json:"context,omitempty"`
}

// StartResult 是 shell.diagnose.start 的返回；caseId 为空表示案例未绑定
//（已有案例进行中或脚本录制占用 recorder，诊断本身不受影响）。
type StartResult struct {
	RunID  string `json:"runId"`
	CaseID string `json:"caseId,omitempty"`
}

// ArchiveInput 是 shell.diagnose.archive 的参数；结案报告文本由宿主流式生成后传入。
type ArchiveInputJSON struct {
	Service    string `json:"service"`
	Module     string `json:"module"`
	RootCause  string `json:"rootCause"`
	Conclusion string `json:"conclusion"`
}

// NewDiagnoseService 构建诊断服务并安装全局 agent 事件转发（进程内一次）。
// rec 可为 nil（案例能力降级：诊断/结案/取消可用，案例与归档不可用）。
func NewDiagnoseService(aiCfg *AIConfigService, dataDir string, rec *recorder.Recorder) (*DiagnoseService, error) {
	knowledgeDir := filepath.Join(dataDir, "knowledge")
	if err := os.MkdirAll(knowledgeDir, 0o755); err != nil {
		return nil, err
	}
	s := &DiagnoseService{
		ai:           aiCfg,
		knowledgeDir: knowledgeDir,
		runs:         map[string]context.CancelFunc{},
	}
	if rec != nil {
		s.cases = troubleshoot.NewManager(rec, filepath.Join(dataDir, "cases"))
	}
	ai.SetEventEmitter(s.forwardAgentEvent)
	return s, nil
}

// SetNotify 注入通知写出端（与 FTService 同模式）。
func (s *DiagnoseService) SetNotify(fn func(method string, params any)) { s.notify = fn }

// KnowledgeDir 返回知识目录路径（宿主/测试可查）。
func (s *DiagnoseService) KnowledgeDir() string { return s.knowledgeDir }

// CasesAvailable 报告案例能力是否接线。
func (s *DiagnoseService) CasesAvailable() bool { return s.cases != nil }

// forwardAgentEvent 把 pkg/ai 的 agent:status / agent:context 转成中性契约。
// 仅转发由本服务发起的运行（经 ctx 携带 runId），其它调用方不经过本契约。
func (s *DiagnoseService) forwardAgentEvent(ctx context.Context, name string, args ...interface{}) {
	runId, _ := ctx.Value(diagnoseRunKey{}).(string)
	if runId == "" {
		return
	}
	payload, _ := args[0].(map[string]string)
	if s.notify == nil {
		return
	}
	switch name {
	case "agent:status":
		s.emit(map[string]any{
			"runId": runId, "kind": "status",
			"stage": payload["stage"], "message": payload["message"],
		})
	case "agent:context":
		s.emit(map[string]any{
			"runId": runId, "kind": "context",
			"usedTokens": payload["usedTokens"], "maxTokens": payload["maxTokens"],
		})
	}
}

func (s *DiagnoseService) emit(params map[string]any) {
	if s.notify != nil {
		s.notify("shell.diagnose.event", params)
	}
}

func (s *DiagnoseService) newRunId(prefix string) (string, error) {
	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(raw), nil
}

// Start 启动一次诊断（异步长任务），可选绑定排查案例（共享 recorder 录终端键入）。
func (s *DiagnoseService) Start(input StartInput) (*StartResult, error) {
	fast, err := s.ai.buildFastProvider()
	if err != nil {
		return nil, err
	}
	complexP, err := s.ai.buildComplexProvider()
	if err != nil {
		return nil, err
	}

	runId, err := s.newRunId("diag-")
	if err != nil {
		return nil, err
	}
	result := &StartResult{RunID: runId}

	// 案例绑定失败不阻塞诊断（已有案例/录制占用时降级为无案例运行）。
	if s.cases != nil {
		if cs, caseErr := s.cases.StartCase(input.Problem, input.Context, input.TerminalID, input.Host, input.User); caseErr == nil {
			result.CaseID = cs.ID
		}
	}

	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), diagnoseRunKey{}, runId))
	s.mu.Lock()
	s.runs[runId] = cancel
	s.mu.Unlock()

	go func() {
		defer func() {
			cancel()
			s.mu.Lock()
			delete(s.runs, runId)
			s.mu.Unlock()
		}()

		svc := ai.NewAIService(fast, complexP, nil)
		// 知识目录缺失/为空时 catalog 为空，agent 仍可运行（无知识上下文）。
		_ = svc.UpdateCatalog(s.knowledgeDir)

		res, askErr := svc.AskTroubleshoot(ctx, input.Problem, s.knowledgeDir)
		switch {
		case askErr != nil && (errors.Is(askErr, context.Canceled) || ctx.Err() != nil):
			s.emit(map[string]any{"runId": runId, "kind": "canceled", "message": "诊断已取消"})
		case askErr != nil:
			s.emit(map[string]any{"runId": runId, "kind": "error", "message": askErr.Error()})
		default:
			s.emit(map[string]any{"runId": runId, "kind": "done", "result": res, "caseId": result.CaseID})
		}
	}()
	return result, nil
}

// Cancel 取消一次运行中的诊断；runId 未知时静默（幂等）。
func (s *DiagnoseService) Cancel(runId string) {
	s.mu.Lock()
	cancel := s.runs[runId]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// StopCase 结束当前排查案例并落盘（rootCause/conclusion 由宿主提供，结论文案可来自流式结案）。
func (s *DiagnoseService) StopCase(rootCause, conclusion string) (*troubleshoot.Case, error) {
	if s.cases == nil {
		return nil, errCaseUnavailable
	}
	cs, err := s.cases.StopCase(rootCause, conclusion)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.lastStoppedCase = cs
	s.mu.Unlock()
	return cs, nil
}

// CaseStatus 返回当前案例状态（案例能力未接线时 IsActive=false）。
func (s *DiagnoseService) CaseStatus() troubleshoot.CaseStatus {
	if s.cases == nil {
		return troubleshoot.CaseStatus{}
	}
	return s.cases.GetStatus()
}

// timeline 从当前/最近案例拼结案时间线（命令序列）。
func (s *DiagnoseService) timeline() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	cs := s.lastStoppedCase
	if cs == nil {
		if s.cases != nil {
			cs = s.cases.GetCurrentCase()
		}
	}
	if cs == nil || len(cs.Commands) == 0 {
		return "(无命令记录)"
	}
	var b strings.Builder
	for _, cmd := range cs.Commands {
		fmt.Fprintf(&b, "- %s\n", cmd.Content)
	}
	return b.String()
}

// Conclusion 流式生成结案报告：token 事件增量推送，concl-done 携带全文。
func (s *DiagnoseService) Conclusion(rootCause string) (string, error) {
	fast, err := s.ai.buildFastProvider()
	if err != nil {
		return "", err
	}
	runId, err := s.newRunId("concl-")
	if err != nil {
		return "", err
	}
	timeline := s.timeline()

	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), diagnoseRunKey{}, runId))
	s.mu.Lock()
	s.runs[runId] = cancel
	s.mu.Unlock()

	go func() {
		defer func() {
			cancel()
			s.mu.Lock()
			delete(s.runs, runId)
			s.mu.Unlock()
		}()
		svc := ai.NewAIService(fast, fast, nil)
		full, genErr := svc.GenerateConclusionStream(ctx, timeline, rootCause, func(token string) {
			s.emit(map[string]any{"runId": runId, "kind": "token", "text": token})
		})
		switch {
		case genErr != nil && (errors.Is(genErr, context.Canceled) || ctx.Err() != nil):
			s.emit(map[string]any{"runId": runId, "kind": "canceled", "message": "结案生成已取消"})
		case genErr != nil:
			s.emit(map[string]any{"runId": runId, "kind": "error", "message": genErr.Error()})
		default:
			s.emit(map[string]any{"runId": runId, "kind": "concl-done", "text": full})
		}
	}()
	return runId, nil
}

// Archive 把最近一次已停止案例写入知识目录（archive/ 子目录），返回文件路径。
// 写入后知识目录随下一次诊断启动重建 catalog。
func (s *DiagnoseService) Archive(input ArchiveInputJSON) (string, error) {
	if s.cases == nil {
		return "", errCaseUnavailable
	}
	s.mu.Lock()
	cs := s.lastStoppedCase
	s.mu.Unlock()
	if cs == nil {
		return "", errors.New("没有可归档的案例：请先结束一次排查案例")
	}
	if strings.TrimSpace(input.Service) == "" {
		return "", errors.New("service 不能为空")
	}
	rootCause := input.RootCause
	if rootCause == "" {
		rootCause = cs.RootCause
	}
	path, err := knowledge.AppendRecord(s.knowledgeDir, &knowledge.ArchiveInput{
		Session:    &cs.RecordingSession,
		Conclusion: input.Conclusion,
		Service:    input.Service,
		Module:     input.Module,
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

var errCaseUnavailable = errors.New("案例能力不可用：脚本录制服务未接线")
