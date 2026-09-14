package shellsidecar

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"opscopilot/pkg/ai"
	"opscopilot/pkg/config"
	"opscopilot/pkg/llm"
	"opscopilot/pkg/remote"
)

// AIConfigService：AI 接入配置的 sidecar 持久化（数据目录内 ai-config.json）。
//
// 密钥边界（方案 D1）：密钥只在 sidecar 后台感知——
//   - Status/saveConfig 返回值永不含明文密钥，只带 configured + 尾端提示；
//   - Save 时 apiKey 为空表示保留已存密钥；
//   - 文件未配置时可回退读取 LLM_API_KEY / LLM_BASE_URL 等环境变量（与 Wails/CLI 同名），
//     文件配置优先于环境变量。
type AIConfigService struct {
	desktopRoot string
	path        string
	mu          sync.RWMutex
	session     *aiConfigFile
}

// AIConfigStatus 是 shell.ai.getConfig 的返回形态：脱敏，永不含明文密钥。
type AIConfigStatus struct {
	Configured   bool   `json:"configured"`
	KeyHint      string `json:"keyHint,omitempty"`
	BaseURL      string `json:"baseURL"`
	FastModel    string `json:"fastModel"`
	ComplexModel string `json:"complexModel"`
	Source       string `json:"source"` // "file" | "env" | "none"
}

// aiConfigFile 是落盘形态。
type aiConfigFile struct {
	BaseURL      string `json:"baseURL,omitempty"`
	APIKey       string `json:"apiKey,omitempty"`
	FastModel    string `json:"fastModel,omitempty"`
	ComplexModel string `json:"complexModel,omitempty"`
}

// AIConfigUpdate 是 shell.ai.saveConfig 的参数形态；ApiKey 为空 = 保留已存密钥。
type AIConfigUpdate struct {
	ApiKey       string `json:"apiKey"`
	BaseURL      string `json:"baseURL"`
	FastModel    string `json:"fastModel"`
	ComplexModel string `json:"complexModel"`
}

const (
	aiDefaultBaseURL  = "https://api.deepseek.com/v1"
	aiDefaultModel    = "deepseek-chat"
	aiSourceFile      = "file"
	aiSourceEnv       = "env"
	aiSourceNone      = "none"
	aiKeyHintPrefix   = "…"
	aiKeyHintMinChars = 4
)

func NewAIConfigService(dataDir string) (*AIConfigService, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	return &AIConfigService{path: filepath.Join(dataDir, "ai-config.json")}, nil
}

// Status 返回脱敏配置状态。文件缺失/损坏时回退环境变量与默认值。
func (s *AIConfigService) Status() AIConfigStatus {
	file := s.load()
	envKey := os.Getenv("LLM_API_KEY")

	key := file.APIKey
	source := aiSourceFile
	if key == "" {
		key = envKey
		source = aiSourceEnv
	}
	if key == "" {
		source = aiSourceNone
	}
	s.mu.RLock()
	if s.session != nil {
		source = "session"
	}
	s.mu.RUnlock()

	baseURL := firstNonEmpty(file.BaseURL, os.Getenv("LLM_BASE_URL"), aiDefaultBaseURL)
	fastModel := firstNonEmpty(file.FastModel, os.Getenv("LLM_FAST_MODEL"), aiDefaultModel)
	complexModel := firstNonEmpty(file.ComplexModel, os.Getenv("LLM_COMPLEX_MODEL"), aiDefaultModel)

	return AIConfigStatus{
		Configured:   key != "",
		KeyHint:      maskKey(key),
		BaseURL:      baseURL,
		FastModel:    fastModel,
		ComplexModel: complexModel,
		Source:       source,
	}
}

// Save 落盘配置；更新值为空的字段一律保留已存值（与 apiKey 同语义，
// 避免调用方只改部分字段时误清其余配置）。返回保存后的脱敏状态。
func (s *AIConfigService) Save(update AIConfigUpdate) (AIConfigStatus, error) {
	if s.desktopRoot != "" {
		return AIConfigStatus{}, errors.New("Teams 模型覆盖仅限本次进程；永久配置请在本地 Ops 中保存")
	}
	file := s.load()
	if v := strings.TrimSpace(update.ApiKey); v != "" {
		file.APIKey = v
	}
	if v := strings.TrimSpace(update.BaseURL); v != "" {
		file.BaseURL = v
	}
	if v := strings.TrimSpace(update.FastModel); v != "" {
		file.FastModel = v
	}
	if v := strings.TrimSpace(update.ComplexModel); v != "" {
		file.ComplexModel = v
	}

	data, err := json.Marshal(file)
	if err != nil {
		return AIConfigStatus{}, err
	}
	// 含密钥文件：收紧为属主可读写。
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return AIConfigStatus{}, err
	}
	return s.Status(), nil
}

// load 读取落盘配置；缺失/损坏时返回空结构（回退逻辑在 Status）。
func (s *AIConfigService) load() aiConfigFile {
	s.mu.RLock()
	if s.session != nil {
		value := *s.session
		s.mu.RUnlock()
		return value
	}
	s.mu.RUnlock()
	var file aiConfigFile
	if s.desktopRoot != "" {
		m := config.NewManagerWithDir(s.desktopRoot)
		m.SetReadOnly(true)
		if m.Load() != nil {
			return file
		}
		c := m.Config.LLM
		return aiConfigFile{BaseURL: c.BaseURL, APIKey: c.APIKey, FastModel: c.FastModel, ComplexModel: c.ComplexModel}
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return file
	}
	_ = json.Unmarshal(data, &file)
	return file
}

// SaveSession configures this process only. Teams never persists API secrets.
func (s *AIConfigService) SaveSession(update AIConfigUpdate) AIConfigStatus {
	file := s.load()
	if update.ApiKey != "" {
		file.APIKey = update.ApiKey
	}
	if update.BaseURL != "" {
		file.BaseURL = update.BaseURL
	}
	if update.FastModel != "" {
		file.FastModel = update.FastModel
	}
	if update.ComplexModel != "" {
		file.ComplexModel = update.ComplexModel
	}
	s.mu.Lock()
	s.session = &file
	s.mu.Unlock()
	return s.Status()
}

// errAINotConfigured 在未配置密钥（文件与环境变量均无）时返回。
var errAINotConfigured = errors.New("AI 未配置：请先在 Shell 设置中填写 API 密钥")

// buildFastProvider 按当前配置（文件优先、env 回退）构造快速模型 provider。
// 每次调用现建，避免保存后缓存失效问题。
func (s *AIConfigService) buildFastProvider() (llm.Provider, error) {
	file := s.load()
	key := firstNonEmpty(file.APIKey, os.Getenv("LLM_API_KEY"))
	if key == "" {
		return nil, errAINotConfigured
	}
	baseURL := firstNonEmpty(file.BaseURL, os.Getenv("LLM_BASE_URL"), aiDefaultBaseURL)
	model := firstNonEmpty(file.FastModel, os.Getenv("LLM_FAST_MODEL"), aiDefaultModel)
	return llm.NewOpenAIProvider(key, baseURL, model), nil
}

// buildComplexProvider 同 buildFastProvider，取复杂模型（诊断/Agent 任务）。
func (s *AIConfigService) buildComplexProvider() (llm.Provider, error) {
	file := s.load()
	key := firstNonEmpty(file.APIKey, os.Getenv("LLM_API_KEY"))
	if key == "" {
		return nil, errAINotConfigured
	}
	baseURL := firstNonEmpty(file.BaseURL, os.Getenv("LLM_BASE_URL"), aiDefaultBaseURL)
	model := firstNonEmpty(file.ComplexModel, os.Getenv("LLM_COMPLEX_MODEL"), aiDefaultModel)
	return llm.NewOpenAIProvider(key, baseURL, model), nil
}

// GenerateCommand 自然语言 → Linux 命令（迭代 B 单发形态）。
func (s *AIConfigService) GenerateCommand(request string) (*ai.CommandQueryResult, error) {
	fast, err := s.buildFastProvider()
	if err != nil {
		return nil, err
	}
	return ai.NewAIService(fast, fast, nil).GenerateLinuxCommand(request)
}

// ParseConnectIntent 自然语言 → 连接配置（智能连接）。
func (s *AIConfigService) ParseConnectIntent(input string) ([]remote.ConnectConfig, error) {
	fast, err := s.buildFastProvider()
	if err != nil {
		return nil, err
	}
	return ai.NewAIService(fast, fast, nil).ParseConnectIntent(input)
}

func maskKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= aiKeyHintMinChars {
		return aiKeyHintPrefix + strings.Repeat("*", len(key))
	}
	return aiKeyHintPrefix + key[len(key)-aiKeyHintMinChars:]
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
