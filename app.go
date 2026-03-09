package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"

	"opscopilot/pkg/ai"
	"opscopilot/pkg/completion"
	"opscopilot/pkg/config"
	"opscopilot/pkg/filetransfer"
	"opscopilot/pkg/llm"
	"opscopilot/pkg/mcp"
	"opscopilot/pkg/recorder"
	"opscopilot/pkg/script"
	"opscopilot/pkg/secretstore"
	"opscopilot/pkg/session"
	"opscopilot/pkg/sessionmanager"
	"opscopilot/pkg/sshclient"
	"opscopilot/pkg/troubleshoot"
)

// App struct
type App struct {
	ctx               context.Context
	sessionMgr        *session.Manager
	savedSessionMgr   *sessionmanager.Manager
	secretStore       secretstore.SecretStore
	aiService         *ai.AIService
	configMgr         *config.Manager
	coreRecorder      *recorder.Recorder     // 统一录制引擎
	troubleMgr        *troubleshoot.Manager  // 故障排查管理器
	scriptMgr         *script.Manager        // 脚本管理器
	completionService *completion.Service
	mcpManager        *mcp.Manager           // MCP 服务器管理器
	activeConfigs     map[string]ConnectConfig
	isForceQuitting   bool // Flag to skip confirmation on force quit
	ftMu              sync.Mutex
	ftCancels         map[string]context.CancelFunc
	sessionStates     map[string]*SessionState // 会话状态追踪
	sessionStateMu    sync.RWMutex
}

// NewApp creates a new App application struct
func NewApp() *App {
	configMgr := config.NewManager()
	if err := configMgr.Load(); err != nil {
		fmt.Printf("Warning: Failed to load config: %v\n", err)
	}

	// Initialize LLM provider using loaded config
	llmConfig := configMgr.Config.LLM
	// Use OpenAIProvider by default, fallback to DeepSeek compatible
	fastModel := llmConfig.FastModel
	if fastModel == "" {
		fastModel = llmConfig.Model
	}
	if fastModel == "" {
		fastModel = "deepseek-chat"
	}
	complexModel := llmConfig.ComplexModel
	if complexModel == "" {
		complexModel = "glm46"
	}
	fastProvider := llm.NewOpenAIProvider(llmConfig.APIKey, llmConfig.BaseURL, fastModel)
	complexProvider := llm.NewOpenAIProvider(llmConfig.APIKey, llmConfig.BaseURL, complexModel)
	aiService := ai.NewAIService(fastProvider, complexProvider, configMgr)

	// Initialize Core Recorder Engine (统一录制器)
	recordingsPath := filepath.Join(configMgr.Config.Log.Dir, "recordings")
	coreRecorder := recorder.NewRecorder(recordingsPath)

	// Initialize Session Manager (will be used in App)
	sessionMgrInstance := session.NewManager()

	// Initialize Troubleshoot Manager
	troubleMgr := troubleshoot.NewManager(coreRecorder, recordingsPath)

	// Initialize Script Manager (pass nil for now, will set in App)
	scriptMgr := script.NewManager(coreRecorder, recordingsPath, nil)

	// Initialize Saved Session Manager
	savedMgr := sessionmanager.NewManager()
	if err := savedMgr.Load(); err != nil {
		fmt.Printf("Warning: Failed to load saved sessions: %v\n", err)
	}

	// Initialize Completion Service
	completionDB, err := completion.NewDatabase()
	if err != nil {
		fmt.Printf("Warning: Failed to initialize completion database: %v\n", err)
	}
	completionService := completion.NewService(completionDB)

	app := &App{
		sessionMgr:        sessionMgrInstance,
		savedSessionMgr:   savedMgr,
		secretStore:       secretstore.NewKeyringStore(),
		aiService:         aiService,
		configMgr:         configMgr,
		coreRecorder:      coreRecorder,
		troubleMgr:        troubleMgr,
		scriptMgr:         scriptMgr,
		completionService: completionService,
		activeConfigs:     make(map[string]ConnectConfig),
		isForceQuitting:   false,
		ftCancels:         make(map[string]context.CancelFunc),
		sessionStates:     make(map[string]*SessionState),
	}

	// Set the CommandSender to app itself
	scriptMgr.SetCommandSender(app)

	return app
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// 初始化日志文件
	logDir := a.configMgr.Config.Log.Dir

	// 如果配置的目录是相对路径，转换为绝对路径
	if !filepath.IsAbs(logDir) {
		// 优先尝试获取可执行文件所在目录
		execPath, err := os.Executable()
		var baseDir string
		if err == nil {
			baseDir = filepath.Dir(execPath)
		} else {
			// 回退到工作目录
			baseDir, _ = os.Getwd()
		}
		logDir = filepath.Join(baseDir, logDir)
	}

	// Debug print
	fmt.Printf("[Startup] Initializing log in directory: %s\n", logDir)

	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Printf("[Startup] Failed to create log directory: %v\n", err)
		return
	}

	logFile := filepath.Join(logDir, "opscopilot.log")
	fmt.Printf("[Startup] Log file path: %s\n", logFile)

	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("[Startup] Failed to open log file: %v\n", err)
		return
	}

	// 同时输出到文件和控制台
	// 注意：在 Windows GUI 应用中（wails build），os.Stdout 可能不可见或无效
	// 为了确保安全，我们先测试写入
	if _, err := f.WriteString(fmt.Sprintf("[Startup] Log initialized at %s\n", logDir)); err != nil {
		fmt.Printf("[Startup] Failed to write test log: %v\n", err)
	}

	// 根据环境变量判断是否启用控制台输出
	// 在开发模式下 (OPSCOPILOT_DEV_MODE=true)，我们希望同时看到控制台和文件日志
	// 在生产模式下 (build)，Stdout 可能无效，因此只输出到文件
	if os.Getenv("OPSCOPILOT_DEV_MODE") == "true" {
		fmt.Println("[Startup] Dev mode detected: Enabling console + file logging")
		multiWriter := io.MultiWriter(os.Stdout, f)
		log.SetOutput(multiWriter)
	} else {
		// 生产模式：仅文件
		log.SetOutput(f)
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	log.Println("App started")

	// 初始化 MCP 管理器
	// MCP 配置文件放在可执行文件所在目录（根目录）
	var mcpConfigPath string
	if execPath, err := os.Executable(); err == nil {
		mcpConfigPath = filepath.Join(filepath.Dir(execPath), "mcp.json")
	} else {
		mcpConfigPath = filepath.Join(logDir, "mcp.json") // 回退到 log 目录
	}
	log.Printf("[MCP] Config path: %s", mcpConfigPath)
	a.mcpManager = mcp.NewManager(mcpConfigPath)
	if err := a.mcpManager.Load(); err != nil {
		log.Printf("[MCP] Failed to load MCP config: %v", err)
	} else {
		// 启动所有配置的 MCP 服务器
		if err := a.mcpManager.StartAll(); err != nil {
			log.Printf("[MCP] Failed to start MCP servers: %v", err)
		} else {
			log.Println("[MCP] MCP servers initialized successfully")
		}
		// 将 MCP 管理器设置到 AIService
		a.aiService.SetMCPManager(a.mcpManager)
	}
}

// beforeClose is called before the application closes
// Returns true to prevent close, false to allow close
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	// If this is a forced quit, skip confirmation and allow close
	if a.isForceQuitting {
		log.Println("[beforeClose] Force quitting, allowing close")
		a.cleanupMCPClient()
		return false
	}

	// Check if there are active terminal sessions
	activeSessions := a.sessionMgr.List()
	hasTerminals := len(activeSessions) > 0

	// Check if there's an ongoing troubleshooting session
	hasTroubleshooting := a.coreRecorder.GetCurrentSession() != nil

	// If there's any active work, we need to ask for confirmation
	if hasTerminals || hasTroubleshooting {
		log.Printf("[beforeClose] Active work detected: terminals=%d, troubleshooting=%v", len(activeSessions), hasTroubleshooting)

		// Emit event to frontend to show custom confirmation dialog
		var message string
		if hasTerminals && hasTroubleshooting {
			message = fmt.Sprintf("您有 %d 个活跃的终端连接和一个正在进行的问题排查会话。关闭应用将断开所有连接并丢失未保存的排查记录。", len(activeSessions))
		} else if hasTerminals {
			message = fmt.Sprintf("您有 %d 个活跃的终端连接。关闭应用将断开所有连接。", len(activeSessions))
		} else {
			message = "您有一个正在进行的问题排查会话。关闭应用将丢失未保存的排查记录。"
		}

		runtime.EventsEmit(ctx, "confirm-close", map[string]interface{}{
			"message":            message,
			"hasTerminals":       hasTerminals,
			"terminalCount":      len(activeSessions),
			"hasTroubleshooting": hasTroubleshooting,
		})

		// Always prevent close, let frontend handle confirmation
		return true
	}

	log.Println("[beforeClose] No active work, allowing close")
	a.cleanupMCPClient()
	// No active work, allow close
	return false
}

// cleanupMCPClient 清理 MCP 客户端资源
func (a *App) cleanupMCPClient() {
	if a.mcpManager != nil {
		log.Println("[MCP] Stopping MCP servers")
		if err := a.mcpManager.StopAll(); err != nil {
			log.Printf("[MCP] Error stopping MCP servers: %v", err)
		}
	}
}

// GetMCPStatus 获取 MCP 服务器状态（供前端调用）
func (a *App) GetMCPStatus() string {
	if a.mcpManager == nil {
		log.Println("[GetMCPStatus] MCP Manager is nil")
		return `{"servers": {}}`
	}

	status := a.mcpManager.GetStatus()
	log.Printf("[GetMCPStatus] Returning status: %+v", status)
	result := map[string]interface{}{
		"servers": status,
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		log.Printf("[GetMCPStatus] Failed to marshal: %v", err)
		return fmt.Sprintf(`{"error": "Failed to marshal status: %v"}`, err)
	}

	log.Printf("[GetMCPStatus] Returning JSON: %s", string(jsonBytes))
	return string(jsonBytes)
}

// ForceQuit forces the application to quit without confirmation
func (a *App) ForceQuit() {
	log.Println("[ForceQuit] Setting force quit flag and calling runtime.Quit()")

	// Set flag to skip confirmation on next beforeClose call
	a.isForceQuitting = true

	// Trigger quit
	runtime.Quit(a.ctx)
}

type ConnectConfig struct {
	Name         string         `json:"name"`
	Host         string         `json:"host"`
	Port         int            `json:"port"`
	User         string         `json:"user"`
	Password     string         `json:"password"`
	RootPassword string         `json:"rootPassword"`
	Bastion      *ConnectConfig `json:"bastion"`
	Group        string         `json:"group"`
}

// DisconnectReason 会话断开原因
type DisconnectReason string

const (
	DisconnectNormal  DisconnectReason = "normal"  // 用户主动关闭
	DisconnectError   DisconnectReason = "error"   // 连接错误
	DisconnectEOF     DisconnectReason = "eof"     // 远程关闭
	DisconnectTimeout DisconnectReason = "timeout" // 超时
)

// SessionState 会话状态追踪
type SessionState struct {
	ID               string
	Config           ConnectConfig
	Status           string // "active", "disconnected"
	DisconnectReason string
}

type ConnectResult struct {
	Success   bool   `json:"success"`
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
}

func (a *App) Connect(config ConnectConfig) ConnectResult {
	return a.ConnectWithID(config, "")
}

// ConnectWithID connects with a specific sessionID (for reconnection)
func (a *App) ConnectWithID(config ConnectConfig, specifiedSessionID string) ConnectResult {
	// 尝试从 SecretStore 保存密码（如果提供了）
	if config.Password != "" {
		_ = a.secretStore.Set("OpsCopilot-SSH", config.Host+":"+config.User, config.Password)
	}

	clientConfig := &sshclient.ConnectConfig{
		Host:         config.Host,
		Port:         config.Port,
		User:         config.User,
		Password:     config.Password,
		RootPassword: config.RootPassword,
		Group:        config.Group,
	}

	// 递归构建 Bastion 配置
	if config.Bastion != nil {
		clientConfig.Bastion = &sshclient.ConnectConfig{
			Host:     config.Bastion.Host,
			Port:     config.Bastion.Port,
			User:     config.Bastion.User,
			Password: config.Bastion.Password,
		}
		// 保存 Bastion 密码
		if config.Bastion.Password != "" {
			_ = a.secretStore.Set("OpsCopilot-SSH", config.Bastion.Host+":"+config.Bastion.User, config.Bastion.Password)
		}
	}

	client, err := sshclient.NewClient(clientConfig)
	if err != nil {
		return ConnectResult{Success: false, Message: fmt.Sprintf("Error connecting: %v", err)}
	}

	// Start shell with default size
	var sshSession *ssh.Session
	var stdin io.WriteCloser
	var stdout io.Reader

	if config.RootPassword != "" {
		sshSession, stdin, stdout, err = client.StartShellWithSudo(120, 30, config.RootPassword)
	} else {
		sshSession, stdin, stdout, err = client.StartShell(120, 30)
	}

	if err != nil {
		client.Close()
		return ConnectResult{Success: false, Message: fmt.Sprintf("Error starting shell: %v", err)}
	}

	// Add to session manager (with SSH session for resizing)
	var sessionID string
	if specifiedSessionID != "" {
		a.sessionMgr.AddWithID(specifiedSessionID, client, stdin, sshSession)
		sessionID = specifiedSessionID
	} else {
		sessionID = a.sessionMgr.Add(client, stdin, sshSession)
	}

	// Store config mapping for duplication
	a.activeConfigs[sessionID] = config

	// Store session state for reconnection
	a.storeSessionState(sessionID, config)

	// Auto-save session to persistent storage
	if err := a.savedSessionMgr.Upsert(*clientConfig, config.Group); err != nil {
		fmt.Printf("Warning: Failed to auto-save session: %v\n", err)
	}

	// Read loop
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if err != nil {
				var reason DisconnectReason
				var message string

				if err == io.EOF {
					reason = DisconnectEOF
					message = "远程主机关闭了连接"
				} else {
					reason = DisconnectError
					message = fmt.Sprintf("连接错误: %v", err)
				}

				// 发送错误消息到终端
				if err != io.EOF {
					runtime.EventsEmit(a.ctx, "terminal-data:"+sessionID, fmt.Sprintf("\r\n[断开] %s\r\n", message))
				} else {
					runtime.EventsEmit(a.ctx, "terminal-data:"+sessionID, "\r\n[断开] 连接已关闭\r\n")
				}

				// 发送断开事件（保留会话，不关闭tab）
				runtime.EventsEmit(a.ctx, "session-disconnected", map[string]interface{}{
					"sessionId": sessionID,
					"reason":    string(reason),
					"message":   message,
					"timestamp": time.Now().Unix(),
				})

				// 更新会话状态
				a.updateSessionState(sessionID, "disconnected", string(reason))

				// 从会话管理器移除（清理SSH资源）
				a.sessionMgr.Remove(sessionID)
				break
			}
			if n > 0 {
				dataStr := string(buf[:n])
				runtime.EventsEmit(a.ctx, "terminal-data:"+sessionID, dataStr)

				// Record output
				if a.coreRecorder != nil {
					a.coreRecorder.AddEvent("terminal_output", dataStr, map[string]interface{}{
						"session_id": sessionID,
					})
				}
			}
		}
	}()

	return ConnectResult{Success: true, SessionID: sessionID, Message: "Connected"}
}

// ResizeTerminal resizes the PTY for a given session
func (a *App) ResizeTerminal(sessionID string, cols int, rows int) {
	if err := a.sessionMgr.Resize(sessionID, cols, rows); err != nil {
		log.Printf("[ResizeTerminal] Failed to resize session %s: %v", sessionID, err)
	}
}

func (a *App) Write(sessionID string, data string) {
	sess, ok := a.sessionMgr.Get(sessionID)
	if ok && sess.Stdin != nil {
		_, err := sess.Stdin.Write([]byte(data))
		if err != nil {
			runtime.EventsEmit(a.ctx, "terminal-data:"+sessionID, fmt.Sprintf("\r\nWrite Error: %v\r\n", err))
		}

		// 记录输入到录制器（内部使用 LineBuffer 处理）
		a.recordInput(sessionID, data)
	}
}

// recordInput 记录终端输入到录制器
func (a *App) recordInput(sessionID string, data string) {
	if a.coreRecorder == nil {
		return
	}

	// Pass raw data to recorder, which uses LineBuffer to handle ANSI codes and editing
	// Returns the committed line if Enter was pressed
	line, committed, err := a.coreRecorder.AddEvent("terminal_input", data, map[string]interface{}{
		"session_id": sessionID,
	})
	if err != nil {
		log.Printf("[recordInput] Error recording input: %v", err)
		return
	}

	// 如果命令被提交，发送事件到前端（用于脚本录制）
	if committed && line != "" {
		runtime.EventsEmit(a.ctx, "script-command-recorded", line)
		log.Printf("[ScriptRecording] Recorded command from session %s: %q", sessionID, line)
	}
}

// StartSession starts a new troubleshooting session
func (a *App) StartSession(problem string) string {
	// TODO: Get list of active context files if available
	contextFiles := []string{}
	session := a.coreRecorder.StartSession(problem, contextFiles)
	return session.ID
}

// StopSession stops the current troubleshooting session, generates conclusion, and saves it
func (a *App) StopSession(rootCause string, conclusion string) string {
	currentSession := a.coreRecorder.GetCurrentSession()
	if currentSession == nil {
		return "Error: No active session"
	}

	// If conclusion is empty, generate it using AI (legacy behavior or fallback)
	if conclusion == "" {
		// Serialize timeline for AI
		timelineBytes, _ := json.Marshal(currentSession.Timeline)
		timelineStr := string(timelineBytes)

		// Generate Conclusion using AI
		var err error
		conclusion, err = a.aiService.GenerateConclusion(timelineStr, rootCause)
		if err != nil {
			log.Printf("Failed to generate conclusion: %v", err)
			conclusion = "Failed to generate conclusion via AI."
		}
	}

	// Stop and Save Session (JSON)
	if err := a.coreRecorder.StopSession(rootCause, conclusion); err != nil {
		return fmt.Sprintf("Error saving session: %v", err)
	}

	// Append to troubleshooting_history.md in docs directory
	if err := a.appendConclusionToDocs(conclusion); err != nil {
		log.Printf("Failed to append conclusion to docs: %v", err)
		return fmt.Sprintf("Session saved, but failed to update history docs: %v", err)
	}

	return conclusion
}

// CancelSession 取消当前故障排查会话（不保存，仅清除状态）
func (a *App) CancelSession() string {
	if err := a.coreRecorder.CancelSession(); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return "cancelled"
}

// appendConclusionToDocs appends the conclusion to the troubleshooting history markdown file
func (a *App) appendConclusionToDocs(conclusion string) error {
	docsDir := a.resolveKnowledgeBase()
	historyFile := filepath.Join(docsDir, "troubleshooting_history.md")

	// Ensure docs directory exists (it should, but just in case)
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		return fmt.Errorf("failed to create docs directory: %w", err)
	}

	f, err := os.OpenFile(historyFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open history file: %w", err)
	}
	defer f.Close()

	// Add timestamp header
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("\n\n## 故障记录 [%s]\n\n%s\n\n---\n", timestamp, conclusion)

	if _, err := f.WriteString(entry); err != nil {
		return fmt.Errorf("failed to write to history file: %w", err)
	}

	return nil
}

func (a *App) Broadcast(sessionIDs []string, data string) {
	if len(sessionIDs) == 0 {
		return
	}
	a.sessionMgr.Broadcast(sessionIDs, data)

	// Record broadcast input using specialized method for deduplication
	if a.coreRecorder != nil {
		a.coreRecorder.AddBroadcastInput(sessionIDs, data)
	}
}

func (a *App) CloseSession(sessionID string) {
	a.sessionMgr.Remove(sessionID)
}

func (a *App) ParseIntent(input string) ([]ConnectConfig, error) {
	configs, err := a.aiService.ParseConnectIntent(input)
	if err != nil {
		return nil, err
	}

	// Convert pkg/sshclient.ConnectConfig to App.ConnectConfig
	var result []ConnectConfig
	for _, c := range configs {
		appConfig := ConnectConfig{
			Name:         c.Name,
			Host:         c.Host,
			Port:         c.Port,
			User:         c.User,
			Password:     c.Password,
			RootPassword: c.RootPassword,
		}

		if c.Bastion != nil {
			appConfig.Bastion = &ConnectConfig{
				Name:     c.Bastion.Name,
				Host:     c.Bastion.Host,
				Port:     c.Bastion.Port,
				User:     c.Bastion.User,
				Password: c.Bastion.Password,
			}
		}

		result = append(result, appConfig)
	}

	return result, nil
}

// resolveKnowledgeBase finds the knowledge base directory
// Priority:
// 1. Configured Directory (if set)
// 2. "docs" in Executable Directory
// 3. "docs" in Working Directory
// 4. "knowledge" in Executable Directory
// 5. "knowledge" in Working Directory
func (a *App) resolveKnowledgeBase() string {
	// 1. Configured Directory
	if configuredDir := a.configMgr.Config.Docs.Dir; configuredDir != "" {
		if _, err := os.Stat(configuredDir); err == nil {
			return configuredDir
		}
		// If configured dir is invalid, fall through to auto-discovery
		log.Printf("Configured docs directory not found: %s, falling back to auto-discovery", configuredDir)
	}

	candidates := []string{"docs", "knowledge"}
	pathsToCheck := []string{}

	// 1. Executable Directory
	if execPath, err := os.Executable(); err == nil {
		pathsToCheck = append(pathsToCheck, filepath.Dir(execPath))
	}

	// 2. Working Directory
	if wd, err := os.Getwd(); err == nil {
		pathsToCheck = append(pathsToCheck, wd)
	}

	for _, dirName := range candidates {
		for _, basePath := range pathsToCheck {
			fullPath := filepath.Join(basePath, dirName)
			if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
				return fullPath
			}
		}
	}

	return "docs"
}

// AskAI handles the Q&A request from frontend
func (a *App) AskAI(question string) string {
	// 1. Resolve knowledge directory
	knowledgeDir := a.resolveKnowledgeBase()

	// 2. Call AIService with Agent mode
	answer, err := a.aiService.AskWithContext(a.ctx, question, knowledgeDir)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return answer
}

// AskTroubleshoot handles the troubleshooting request from frontend
// enableExternal: whether to enable MCP tools (controlled by user toggle in UI)
// 当 enableExternal 为 true 且配置了 MCP 服务器时，Agent 会自动使用 MCP 工具进行诊断
func (a *App) AskTroubleshoot(problem string, enableExternal bool) string {
	knowledgeDir := a.resolveKnowledgeBase()
	log.Printf("[AskTroubleshoot] Problem: %s, EnableMCP: %v", problem, enableExternal)
	answer, err := a.aiService.AskTroubleshoot(a.ctx, problem, knowledgeDir, enableExternal)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return answer
}

func (a *App) GetSettings() config.AppConfig {
	return *a.configMgr.Config
}

func (a *App) SaveSettings(cfg config.AppConfig) string {
	// Update config in memory
	*a.configMgr.Config = cfg

	// Save to disk
	if err := a.configMgr.Save(); err != nil {
		return fmt.Sprintf("Failed to save settings: %v", err)
	}

	// Update AI Service Provider
	llmConfig := cfg.LLM
	fastModel := llmConfig.FastModel
	if fastModel == "" {
		fastModel = llmConfig.Model
	}
	if fastModel == "" {
		fastModel = "deepseek-chat"
	}
	complexModel := llmConfig.ComplexModel
	if complexModel == "" {
		complexModel = "glm46"
	}
	fastProvider := llm.NewOpenAIProvider(llmConfig.APIKey, llmConfig.BaseURL, fastModel)
	complexProvider := llm.NewOpenAIProvider(llmConfig.APIKey, llmConfig.BaseURL, complexModel)
	a.aiService.UpdateProviders(fastProvider, complexProvider)

	return ""
}

func (a *App) ImportConfigFromDirectory(dirPath string) string {
	if err := a.configMgr.ImportFromDirectory(dirPath); err != nil {
		msg := a.configMgr.LastImportMessage()
		if msg != "" {
			return msg
		}
		return fmt.Sprintf("导入失败: %v", err)
	}

	cfg := *a.configMgr.Config
	llmConfig := cfg.LLM
	fastModel := llmConfig.FastModel
	if fastModel == "" {
		fastModel = llmConfig.Model
	}
	if fastModel == "" {
		fastModel = "deepseek-chat"
	}
	complexModel := llmConfig.ComplexModel
	if complexModel == "" {
		complexModel = "glm46"
	}
	fastProvider := llm.NewOpenAIProvider(llmConfig.APIKey, llmConfig.BaseURL, fastModel)
	complexProvider := llm.NewOpenAIProvider(llmConfig.APIKey, llmConfig.BaseURL, complexModel)
	a.aiService.UpdateProviders(fastProvider, complexProvider)

	if err := a.savedSessionMgr.Load(); err != nil {
		log.Printf("Failed to reload sessions after import: %v", err)
	}

	return a.configMgr.LastImportMessage()
}

type ftResponse struct {
	OK      bool                         `json:"ok"`
	Message string                       `json:"message,omitempty"`
	Error   *filetransfer.TransferError  `json:"error,omitempty"`
	TaskID  string                       `json:"taskId,omitempty"`
	Entries []filetransfer.Entry         `json:"entries,omitempty"`
	Entry   *filetransfer.Entry          `json:"entry,omitempty"`
	Result  *filetransfer.TransferResult `json:"result,omitempty"`
}

type localFSResponse struct {
	OK      bool                        `json:"ok"`
	Message string                      `json:"message,omitempty"`
	Error   *filetransfer.TransferError `json:"error,omitempty"`
	Entries []filetransfer.Entry        `json:"entries,omitempty"`
	Entry   *filetransfer.Entry         `json:"entry,omitempty"`
}

type remoteFSResponse struct {
	OK      bool                        `json:"ok"`
	Message string                      `json:"message,omitempty"`
	Error   *filetransfer.TransferError `json:"error,omitempty"`
	Content string                      `json:"content,omitempty"`
}

func (a *App) LocalList(localPath string) string {
	p := strings.TrimSpace(localPath)
	if p == "" || p == "." {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			p = home
		} else if wd, err := os.Getwd(); err == nil && wd != "" {
			p = wd
		} else {
			p = "."
		}
	}
	p = filepath.Clean(p)

	entries, err := os.ReadDir(p)
	if err != nil {
		return mustJSON(localFSResponse{OK: false, Error: toTransferErr(err)})
	}

	out := make([]filetransfer.Entry, 0, len(entries))
	for _, de := range entries {
		fi, err := de.Info()
		if err != nil {
			continue
		}
		out = append(out, filetransfer.Entry{
			Path:    filepath.Join(p, de.Name()),
			Name:    de.Name(),
			IsDir:   de.IsDir(),
			Size:    fi.Size(),
			Mode:    uint32(fi.Mode()),
			ModTime: fi.ModTime(),
		})
	}
	return mustJSON(localFSResponse{OK: true, Entries: out})
}

func (a *App) LocalStat(localPath string) string {
	p := filepath.Clean(strings.TrimSpace(localPath))
	if p == "" {
		return mustJSON(localFSResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotFound, Message: "路径为空"}})
	}
	fi, err := os.Stat(p)
	if err != nil {
		return mustJSON(localFSResponse{OK: false, Error: toTransferErr(err)})
	}
	e := filetransfer.Entry{
		Path:    p,
		Name:    filepath.Base(p),
		IsDir:   fi.IsDir(),
		Size:    fi.Size(),
		Mode:    uint32(fi.Mode()),
		ModTime: fi.ModTime(),
	}
	return mustJSON(localFSResponse{OK: true, Entry: &e})
}

func (a *App) LocalMkdir(localPath string) string {
	p := filepath.Clean(strings.TrimSpace(localPath))
	if p == "" {
		return mustJSON(localFSResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotFound, Message: "路径为空"}})
	}
	if err := os.MkdirAll(p, 0755); err != nil {
		return mustJSON(localFSResponse{OK: false, Error: toTransferErr(err)})
	}
	return mustJSON(localFSResponse{OK: true})
}

func (a *App) LocalRemove(localPath string) string {
	p := filepath.Clean(strings.TrimSpace(localPath))
	if p == "" {
		return mustJSON(localFSResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotFound, Message: "路径为空"}})
	}
	if err := os.RemoveAll(p); err != nil {
		return mustJSON(localFSResponse{OK: false, Error: toTransferErr(err)})
	}
	return mustJSON(localFSResponse{OK: true})
}

func (a *App) LocalRename(oldPath, newPath string) string {
	oldP := filepath.Clean(strings.TrimSpace(oldPath))
	newP := filepath.Clean(strings.TrimSpace(newPath))
	if oldP == "" || newP == "" {
		return mustJSON(localFSResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotFound, Message: "路径为空"}})
	}
	if err := os.Rename(oldP, newP); err != nil {
		return mustJSON(localFSResponse{OK: false, Error: toTransferErr(err)})
	}
	return mustJSON(localFSResponse{OK: true})
}

func (a *App) FTRemoteMkdir(sessionID, remotePath string) string {
	c, closeFn, _, err := a.getPreferredTransferSSHClient(sessionID)
	if err != nil {
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	defer closeFn()

	if strings.HasPrefix(a.getTransferMode(sessionID), "scp") {
		return mustJSON(remoteFSResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotSupported, Message: "SCP 模式不支持远端目录操作"}})
	}

	tr := filetransfer.NewSFTPTransport(c)
	if err := tr.Mkdir(context.Background(), remotePath); err != nil {
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	return mustJSON(remoteFSResponse{OK: true})
}

func (a *App) FTRemoteRename(sessionID, oldPath, newPath string) string {
	c, closeFn, _, err := a.getPreferredTransferSSHClient(sessionID)
	if err != nil {
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	defer closeFn()

	if strings.HasPrefix(a.getTransferMode(sessionID), "scp") {
		return mustJSON(remoteFSResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotSupported, Message: "SCP 模式不支持远端重命名"}})
	}

	tr := filetransfer.NewSFTPTransport(c)
	if err := tr.Rename(context.Background(), oldPath, newPath); err != nil {
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	return mustJSON(remoteFSResponse{OK: true})
}

func (a *App) FTRemoteRemove(sessionID, remotePath string) string {
	c, closeFn, _, err := a.getPreferredTransferSSHClient(sessionID)
	if err != nil {
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	defer closeFn()

	if strings.HasPrefix(a.getTransferMode(sessionID), "scp") {
		return mustJSON(remoteFSResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotSupported, Message: "SCP 模式不支持远端删除"}})
	}

	tr := filetransfer.NewSFTPTransport(c)
	if err := tr.Remove(context.Background(), remotePath, true); err != nil {
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	return mustJSON(remoteFSResponse{OK: true})
}

func (a *App) FTRemoteReadFile(sessionID, remotePath string, maxBytes int64) string {
	c, closeFn, _, err := a.getPreferredTransferSSHClient(sessionID)
	if err != nil {
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	defer closeFn()

	if strings.HasPrefix(a.getTransferMode(sessionID), "scp") {
		return mustJSON(remoteFSResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotSupported, Message: "SCP 模式不支持远端文件直读"}})
	}

	tr := filetransfer.NewSFTPTransport(c)
	b, err := tr.ReadFile(context.Background(), remotePath, maxBytes)
	if err != nil {
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	return mustJSON(remoteFSResponse{OK: true, Content: string(b)})
}

func (a *App) FTRemoteWriteFile(sessionID, remotePath string, content string) string {
	c, closeFn, _, err := a.getPreferredTransferSSHClient(sessionID)
	if err != nil {
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	defer closeFn()

	if strings.HasPrefix(a.getTransferMode(sessionID), "scp") {
		return mustJSON(remoteFSResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotSupported, Message: "SCP 模式不支持远端文件直写"}})
	}

	tr := filetransfer.NewSFTPTransport(c)
	if err := tr.WriteFile(context.Background(), remotePath, []byte(content)); err != nil {
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	return mustJSON(remoteFSResponse{OK: true})
}

func (a *App) getPreferredTransferSSHClient(sessionID string) (*ssh.Client, func(), string, error) {
	sess, ok := a.sessionMgr.Get(sessionID)
	if !ok || sess.Client == nil || sess.Client.SSHClient() == nil {
		return nil, nil, "", &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotFound, Message: "会话不存在"}
	}

	base := sess.Client.SSHClient()
	baseClose := func() {}
	identity := "login"

	cfg, ok := a.activeConfigs[sessionID]
	if !ok {
		return base, baseClose, identity, nil
	}

	if cfg.RootPassword == "" {
		return base, baseClose, identity, nil
	}
	if strings.EqualFold(cfg.User, "root") {
		return base, baseClose, "root", nil
	}

	rootCfg := &sshclient.ConnectConfig{
		Host:     cfg.Host,
		Port:     cfg.Port,
		User:     "root",
		Password: cfg.RootPassword,
	}
	if cfg.Bastion != nil {
		rootCfg.Bastion = &sshclient.ConnectConfig{
			Host:     cfg.Bastion.Host,
			Port:     cfg.Bastion.Port,
			User:     cfg.Bastion.User,
			Password: cfg.Bastion.Password,
		}
	}

	rootClient, err := sshclient.NewClient(rootCfg)
	if err != nil || rootClient == nil || rootClient.SSHClient() == nil {
		return base, baseClose, identity, nil
	}
	return rootClient.SSHClient(), func() { _ = rootClient.Close() }, "root", nil
}

func (a *App) getTransferMode(sessionID string) string {
	c, closeFn, _, err := a.getPreferredTransferSSHClient(sessionID)
	if err != nil {
		return ""
	}
	defer closeFn()

	sftpTr := filetransfer.NewSFTPTransport(c)
	_, _, sftpErr := sftpTr.Check(context.Background())
	if sftpErr == nil {
		return "sftp"
	}
	te := toTransferErr(sftpErr)
	if te != nil && te.Code == filetransfer.ErrorCodeSFTPNotSupported {
		scpTr := filetransfer.NewSCPTransport(c)
		ok, _, err := scpTr.Check(context.Background())
		if err == nil && ok {
			return "scp"
		}
	}
	return ""
}

func (a *App) FTList(sessionID, remotePath string) string {
	c, closeFn, _, err := a.getPreferredTransferSSHClient(sessionID)
	if err != nil {
		return mustJSON(ftResponse{OK: false, Error: toTransferErr(err)})
	}
	defer closeFn()

	tr := filetransfer.NewSFTPTransport(c)
	entries, err := tr.List(context.Background(), remotePath)
	if err != nil {
		te := toTransferErr(err)
		return mustJSON(ftResponse{OK: false, Error: te})
	}
	return mustJSON(ftResponse{OK: true, Entries: entries})
}

func (a *App) FTStat(sessionID, remotePath string) string {
	c, closeFn, _, err := a.getPreferredTransferSSHClient(sessionID)
	if err != nil {
		return mustJSON(ftResponse{OK: false, Error: toTransferErr(err)})
	}
	defer closeFn()

	tr := filetransfer.NewSFTPTransport(c)
	entry, err := tr.Stat(context.Background(), remotePath)
	if err != nil {
		te := toTransferErr(err)
		return mustJSON(ftResponse{OK: false, Error: te})
	}
	return mustJSON(ftResponse{OK: true, Entry: &entry})
}

func (a *App) FTUpload(sessionID, localPath, remotePath string) string {
	return a.startFileTransferTask(sessionID, "upload", localPath, remotePath)
}

func (a *App) FTDownload(sessionID, remotePath, localPath string) string {
	return a.startFileTransferTask(sessionID, "download", localPath, remotePath)
}

func (a *App) FTCancel(taskID string) string {
	a.ftMu.Lock()
	cancel, ok := a.ftCancels[taskID]
	a.ftMu.Unlock()
	if !ok {
		return mustJSON(ftResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotFound, Message: "任务不存在"}})
	}
	cancel()
	return mustJSON(ftResponse{OK: true, Message: "已取消"})
}

func (a *App) FTCheck(sessionID string) string {
	c, closeFn, identity, err := a.getPreferredTransferSSHClient(sessionID)
	if err != nil {
		return mustJSON(ftResponse{OK: false, Error: toTransferErr(err)})
	}
	defer closeFn()

	sftpTr := filetransfer.NewSFTPTransport(c)
	_, _, sftpErr := sftpTr.Check(context.Background())
	if sftpErr == nil {
		if identity == "root" {
			return mustJSON(ftResponse{OK: true, Message: "sftp(root)"})
		}
		return mustJSON(ftResponse{OK: true, Message: "sftp(login)"})
	}

	te := toTransferErr(sftpErr)
	if te != nil && (te.Code == filetransfer.ErrorCodeSFTPNotSupported || te.Code == filetransfer.ErrorCodeUnknown || te.Code == filetransfer.ErrorCodeNetwork) {
		scpTr := filetransfer.NewSCPTransport(c)
		ok, _, err := scpTr.Check(context.Background())
		if err != nil {
			return mustJSON(ftResponse{OK: false, Error: toTransferErr(err)})
		}
		if ok {
			if identity == "root" {
				return mustJSON(ftResponse{OK: true, Message: "scp(root)"})
			}
			return mustJSON(ftResponse{OK: true, Message: "scp(login)"})
		}
		if te.Code == filetransfer.ErrorCodeSFTPNotSupported {
			return mustJSON(ftResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeSFTPNotSupported, Message: "对端未开启 SFTP，且未安装 scp"}})
		}
	}
	return mustJSON(ftResponse{OK: false, Error: te})
}

func (a *App) startFileTransferTask(sessionID, op, localPath, remotePath string) string {
	_, _, _, err := a.getPreferredTransferSSHClient(sessionID)
	if err != nil {
		return mustJSON(ftResponse{OK: false, Error: toTransferErr(err)})
	}

	taskID := uuid.New().String()
	ctx, cancel := context.WithCancel(context.Background())

	a.ftMu.Lock()
	a.ftCancels[taskID] = cancel
	a.ftMu.Unlock()

	go func() {
		defer func() {
			a.ftMu.Lock()
			delete(a.ftCancels, taskID)
			a.ftMu.Unlock()
		}()

		c, closeFn, identity, err := a.getPreferredTransferSSHClient(sessionID)
		if err != nil {
			if a.ctx != nil {
				te := toTransferErr(err)
				runtime.EventsEmit(a.ctx, "file-transfer-done", map[string]any{
					"taskId":    taskID,
					"sessionId": sessionID,
					"ok":        false,
					"code":      te.Code,
					"message":   te.Message,
				})
			}
			return
		}
		defer closeFn()

		sftpTr := filetransfer.NewSFTPTransport(c)
		progressFn := func(p filetransfer.Progress) {
			if a.ctx == nil {
				return
			}
			runtime.EventsEmit(a.ctx, "file-transfer-progress", map[string]any{
				"taskId":     taskID,
				"sessionId":  sessionID,
				"bytesDone":  p.BytesDone,
				"bytesTotal": p.BytesTotal,
				"speedBps":   p.SpeedBps,
			})
		}

		var (
			res   filetransfer.TransferResult
			opErr error
		)
		if op == "upload" {
			res, opErr = sftpTr.Upload(ctx, localPath, remotePath, progressFn)
		} else {
			res, opErr = sftpTr.Download(ctx, remotePath, localPath, progressFn)
		}

		usedTransport := "sftp"
		if identity == "root" {
			usedTransport = "sftp(root)"
		} else {
			usedTransport = "sftp(login)"
		}

		if opErr != nil {
			te := toTransferErr(opErr)
			if te != nil && (te.Code == filetransfer.ErrorCodeSFTPNotSupported || te.Code == filetransfer.ErrorCodeUnknown || te.Code == filetransfer.ErrorCodeNetwork) {
				scpTr := filetransfer.NewSCPTransport(c)
				ok, _, checkErr := scpTr.Check(ctx)
				if checkErr == nil && ok {
					if op == "upload" {
						res, opErr = scpTr.Upload(ctx, localPath, remotePath, progressFn)
					} else {
						res, opErr = scpTr.Download(ctx, remotePath, localPath, progressFn)
					}
					if identity == "root" {
						usedTransport = "scp(root)"
					} else {
						usedTransport = "scp(login)"
					}
				} else if checkErr == nil && !ok && te.Code == filetransfer.ErrorCodeSFTPNotSupported {
					opErr = &filetransfer.TransferError{Code: filetransfer.ErrorCodeSFTPNotSupported, Message: "对端未开启 SFTP，且未安装 scp"}
				} else if checkErr != nil {
					opErr = checkErr
				}
			}
		}

		if a.ctx == nil {
			return
		}
		if opErr != nil {
			te := toTransferErr(opErr)
			runtime.EventsEmit(a.ctx, "file-transfer-done", map[string]any{
				"taskId":    taskID,
				"sessionId": sessionID,
				"ok":        false,
				"code":      te.Code,
				"message":   te.Message,
			})
			return
		}
		runtime.EventsEmit(a.ctx, "file-transfer-done", map[string]any{
			"taskId":    taskID,
			"sessionId": sessionID,
			"ok":        true,
			"bytes":     res.Bytes,
			"message":   "完成 (" + usedTransport + ")",
		})
	}()

	return mustJSON(ftResponse{OK: true, TaskID: taskID})
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"ok":false,"error":{"code":"UNKNOWN","message":"marshal failed"}}`
	}
	return string(b)
}

func toTransferErr(err error) *filetransfer.TransferError {
	if err == nil {
		return nil
	}
	if te, ok := err.(*filetransfer.TransferError); ok {
		return te
	}
	return &filetransfer.TransferError{Code: filetransfer.ErrorCodeUnknown, Message: err.Error()}
}

func (a *App) GetHighlightRules() []config.HighlightRule {
	return a.configMgr.Config.HighlightRules
}

func (a *App) SaveHighlightRules(rules []config.HighlightRule) string {
	a.configMgr.SetHighlightRules(rules)
	if err := a.configMgr.Save(); err != nil {
		return fmt.Sprintf("Error saving config: %v", err)
	}
	return ""
}

// LoadQuickCommands returns the list of quick commands from config
func (a *App) LoadQuickCommands() []config.QuickCommand {
	return a.configMgr.Config.QuickCommands
}

// SaveQuickCommands updates and saves quick commands
func (a *App) SaveQuickCommands(commands []config.QuickCommand) string {
	a.configMgr.SetQuickCommands(commands)
	if err := a.configMgr.Save(); err != nil {
		return fmt.Sprintf("Error saving config: %v", err)
	}
	return ""
}

// GetQuickCommandGroups returns a list of all unique groups from quick commands
func (a *App) GetQuickCommandGroups() []string {
	commands := a.configMgr.Config.QuickCommands
	groupMap := make(map[string]bool)

	for _, cmd := range commands {
		if cmd.Group == "" {
			groupMap["default"] = true
		} else {
			groupMap[cmd.Group] = true
		}
	}

	groups := make([]string, 0, len(groupMap))
	for group := range groupMap {
		groups = append(groups, group)
	}

	sort.Strings(groups)
	return groups
}

// PolishRootCause polishes the root cause description
func (a *App) PolishRootCause(input string) string {
	polished, err := a.aiService.PolishContent(input)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return polished
}

func (a *App) GenerateLinuxCommand(request string) string {
	result, err := a.aiService.GenerateLinuxCommand(request)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	b, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return string(b)
}

// GetSessionTimeline returns the current session data including timeline and problem
func (a *App) GetSessionTimeline() *recorder.RecordingSession {
	session := a.coreRecorder.GetCurrentSession()
	if session == nil {
		return nil
	}
	return session
}

// UpdateSessionTimeline updates the current session timeline
func (a *App) UpdateSessionTimeline(events []recorder.TimelineEvent) string {
	err := a.coreRecorder.UpdateTimeline(events)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return ""
}

// GenerateConclusionWithContext generates the conclusion using the provided context (e.g. edited markdown)
func (a *App) GenerateConclusionWithContext(contextStr string, rootCause string) string {
	// Generate Conclusion using AI with provided context
	conclusion, err := a.aiService.GenerateConclusion(contextStr, rootCause)
	if err != nil {
		log.Printf("Failed to generate conclusion: %v", err)
		return fmt.Sprintf("Error generating conclusion: %v", err)
	}
	return conclusion
}

// --- Saved Session Management ---

func (a *App) GetSavedSessions() []*sessionmanager.Session {
	return a.savedSessionMgr.GetSessions()
}

func (a *App) DeleteSavedSession(id string) string {
	if err := a.savedSessionMgr.DeleteSession(id); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return ""
}

func (a *App) DuplicateSession(sessionID string) ConnectResult {
	// 1. Get original session to ensure it exists
	_, ok := a.sessionMgr.Get(sessionID)
	if !ok {
		return ConnectResult{Success: false, Message: "Original session not found"}
	}

	// 2. Retrieve config
	config, ok := a.activeConfigs[sessionID]
	if !ok {
		return ConnectResult{Success: false, Message: "Session configuration not found"}
	}

	// 3. Connect using the same config
	// Note: This will prompt for password again if it wasn't saved in config (e.g. keyboard interactive),
	// but our ConnectConfig stores Password.
	return a.Connect(config)
}

func (a *App) RenameSavedSession(id, newName string) string {
	if err := a.savedSessionMgr.RenameSession(id, newName); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return ""
}

func (a *App) UpdateSavedSession(id string, config sshclient.ConnectConfig) string {
	if err := a.savedSessionMgr.UpdateSession(id, config, config.Group); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return ""
}

// HasActiveWork checks if there are active terminal sessions or ongoing troubleshooting session
func (a *App) HasActiveWork() map[string]interface{} {
	hasTerminals := len(a.sessionMgr.List()) > 0
	hasTroubleshooting := a.coreRecorder.GetCurrentSession() != nil

	return map[string]interface{}{
		"hasActiveTerminals":        hasTerminals,
		"hasTroubleshootingSession": hasTroubleshooting,
		"hasAnyWork":                hasTerminals || hasTroubleshooting,
	}
}

// GetCompletions returns command completion suggestions
func (a *App) GetCompletions(input string, cursor int) string {
	if a.completionService == nil {
		return "[]" // Return empty if service not initialized
	}

	req := completion.CompletionRequest{
		Input:  input,
		Cursor: cursor,
	}

	resp, err := a.completionService.GetCompletions(req)
	if err != nil {
		log.Printf("[GetCompletions] Error: %v", err)
		return "[]"
	}

	// Convert to JSON
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("[GetCompletions] JSON error: %v", err)
		return "[]"
	}

	return string(data)
}

// ========== Script Recording & Playback Methods ==========

// SendCommand 实现 script.CommandSender 接口
func (a *App) SendCommand(sessionID string, command string) error {
	sess, ok := a.sessionMgr.Get(sessionID)
	if !ok || sess.Stdin == nil {
		return fmt.Errorf("session not found or not ready: %s", sessionID)
	}

	_, err := sess.Stdin.Write([]byte(command))
	return err
}

// StartScriptRecording 开始脚本录制
func (a *App) StartScriptRecording(name, description, sessionID string) (*script.Script, error) {
	// 检查会话是否存在
	_, ok := a.sessionMgr.Get(sessionID)
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	// 从配置中获取主机信息（优先从 activeConfigs， 如果没有则从 sessionStates 获取）
	config, ok := a.activeConfigs[sessionID]
	if !ok {
		// 尝试从 sessionStates 获取
		a.sessionStateMu.RLock()
		state, exists := a.sessionStates[sessionID]
		a.sessionStateMu.RUnlock()
		if !exists {
			return nil, fmt.Errorf("session config not found: %s", sessionID)
		}
		config = state.Config
		// 恢复到 activeConfigs 以便后续使用
		a.activeConfigs[sessionID] = config
	}

	return a.scriptMgr.StartRecording(name, description, sessionID, config.Host, config.User)
}

// StopScriptRecording 停止脚本录制
func (a *App) StopScriptRecording() (*script.Script, error) {
	return a.scriptMgr.StopRecording()
}

// GetScriptList 获取脚本列表
func (a *App) GetScriptList() ([]*script.Script, error) {
	return a.scriptMgr.ListScripts()
}

// LoadScript 加载脚本
func (a *App) LoadScript(scriptID string) (*script.Script, error) {
	return a.scriptMgr.LoadScript(scriptID)
}

// UpdateScript 更新脚本
func (a *App) UpdateScript(scriptData *script.Script) error {
	return a.scriptMgr.UpdateScript(scriptData)
}

// DeleteScript 删除脚本
func (a *App) DeleteScript(scriptID string) error {
	return a.scriptMgr.DeleteScript(scriptID)
}

// ReplayScript 回放脚本
func (a *App) ReplayScript(scriptID, sessionID string) error {
	return a.scriptMgr.ReplayScript(scriptID, sessionID)
}

// ExportScript 导出脚本为Shell脚本
func (a *App) ExportScript(scriptID string) (string, error) {
	return a.scriptMgr.ExportScript(scriptID)
}

// GetScriptRecordingStatus 获取脚本录制状态
func (a *App) GetScriptRecordingStatus() script.ScriptStatus {
	return a.scriptMgr.GetRecordingStatus()
}

// storeSessionState 存储会话状态（在Connect成功后调用）
func (a *App) storeSessionState(sessionID string, config ConnectConfig) {
	a.sessionStateMu.Lock()
	defer a.sessionStateMu.Unlock()

	a.sessionStates[sessionID] = &SessionState{
		ID:     sessionID,
		Config: config,
		Status: "active",
	}
}

// updateSessionState 更新会话状态
func (a *App) updateSessionState(sessionID, status, reason string) {
	a.sessionStateMu.Lock()
	defer a.sessionStateMu.Unlock()

	if state, ok := a.sessionStates[sessionID]; ok {
		state.Status = status
		state.DisconnectReason = reason
	}
}

// ReconnectSession 重新连接断开的会话
func (a *App) ReconnectSession(sessionID string) ConnectResult {
	a.sessionStateMu.RLock()
	state, ok := a.sessionStates[sessionID]
	a.sessionStateMu.RUnlock()

	if !ok {
		return ConnectResult{
			Success: false,
			Message: "会话不存在或已过期",
		}
	}

	if state.Status != "disconnected" {
		return ConnectResult{
			Success: false,
			Message: "会话未断开，无法重连",
		}
	}

	// 使用原配置和原sessionID重新连接
	result := a.ConnectWithID(state.Config, sessionID)

	// 如果连接失败，保持disconnected状态
	// 如果连接成功，ConnectWithID已经更新了状态
	return result
}
