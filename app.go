package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"opscopilot/pkg/ai"
	"opscopilot/pkg/config"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/llm"
	"opscopilot/pkg/secretstore"
	"opscopilot/pkg/session"
	"opscopilot/pkg/session_recorder"
	"opscopilot/pkg/sessionmanager"
	"opscopilot/pkg/sshclient"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx             context.Context
	sessionMgr      *session.Manager
	savedSessionMgr *sessionmanager.Manager
	secretStore     secretstore.SecretStore
	aiService       *ai.AIService
	configMgr       *config.Manager
	recorder        *session_recorder.Recorder
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
	provider := llm.NewOpenAIProvider(llmConfig.APIKey, llmConfig.BaseURL, llmConfig.Model)
	aiService := ai.NewAIService(provider, configMgr)

	// Initialize Recorder
	recorder := session_recorder.NewRecorder("sessions")

	// Initialize Saved Session Manager
	savedMgr := sessionmanager.NewManager()
	if err := savedMgr.Load(); err != nil {
		fmt.Printf("Warning: Failed to load saved sessions: %v\n", err)
	}

	return &App{
		sessionMgr:      session.NewManager(),
		savedSessionMgr: savedMgr,
		secretStore:     secretstore.NewKeyringStore(),
		aiService:       aiService,
		configMgr:       configMgr,
		recorder:        recorder,
	}
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

type ConnectResult struct {
	Success   bool   `json:"success"`
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
}

func (a *App) Connect(config ConnectConfig) ConnectResult {
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
	var stdin io.WriteCloser
	var stdout io.Reader

	if config.RootPassword != "" {
		_, stdin, stdout, err = client.StartShellWithSudo(120, 30, config.RootPassword)
	} else {
		_, stdin, stdout, err = client.StartShell(120, 30)
	}

	if err != nil {
		client.Close()
		return ConnectResult{Success: false, Message: fmt.Sprintf("Error starting shell: %v", err)}
	}

	// Add to session manager
	sessionID := a.sessionMgr.Add(client, stdin)

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
				if err != io.EOF {
					runtime.EventsEmit(a.ctx, "terminal-data:"+sessionID, fmt.Sprintf("\r\nConnection Error: %v\r\n", err))
				} else {
					runtime.EventsEmit(a.ctx, "terminal-data:"+sessionID, "\r\nConnection Closed\r\n")
				}
				// Notify session closed
				runtime.EventsEmit(a.ctx, "session-closed", sessionID)
				a.sessionMgr.Remove(sessionID)
				break
			}
			if n > 0 {
				dataStr := string(buf[:n])
				runtime.EventsEmit(a.ctx, "terminal-data:"+sessionID, dataStr)

				// Record output
				if a.recorder != nil {
					a.recorder.AddEvent("terminal_output", dataStr, map[string]interface{}{
						"session_id": sessionID,
					})
				}
			}
		}
	}()

	return ConnectResult{Success: true, SessionID: sessionID, Message: "Connected"}
}

func (a *App) Write(sessionID string, data string) {
	sess, ok := a.sessionMgr.Get(sessionID)
	if ok && sess.Stdin != nil {
		_, err := sess.Stdin.Write([]byte(data))
		if err != nil {
			runtime.EventsEmit(a.ctx, "terminal-data:"+sessionID, fmt.Sprintf("\r\nWrite Error: %v\r\n", err))
		}

		// Record input
		if a.recorder != nil {
			a.recorder.AddEvent("terminal_input", data, map[string]interface{}{
				"session_id": sessionID,
			})
		}
	}
}

// StartSession starts a new troubleshooting session
func (a *App) StartSession(problem string) string {
	// TODO: Get list of active context files if available
	contextFiles := []string{}
	session := a.recorder.StartSession(problem, contextFiles)
	return session.ID
}

// StopSession stops the current troubleshooting session, generates conclusion, and saves it
func (a *App) StopSession(rootCause string, conclusion string) string {
	currentSession := a.recorder.GetCurrentSession()
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
	if err := a.recorder.StopSession(rootCause, conclusion); err != nil {
		return fmt.Sprintf("Error saving session: %v", err)
	}

	// Append to troubleshooting_history.md in docs directory
	if err := a.appendConclusionToDocs(conclusion); err != nil {
		log.Printf("Failed to append conclusion to docs: %v", err)
		return fmt.Sprintf("Session saved, but failed to update history docs: %v", err)
	}

	return conclusion
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
	a.sessionMgr.Broadcast(sessionIDs, data)
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
	// 1. Load knowledge
	knowledgeDir := a.resolveKnowledgeBase()

	// We attempt to load knowledge. If it fails or dir doesn't exist, we pass empty context.
	// The prompt handles "answer based on general knowledge" if context is missing.
	contextContent, err := knowledge.LoadAll(knowledgeDir)
	if err != nil {
		log.Printf("[App] Warning: Failed to load knowledge from %s: %v", knowledgeDir, err)
		contextContent = ""
	}

	// 2. Call AIService
	answer, err := a.aiService.AskWithContext(question, contextContent)
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
	newProvider := llm.NewOpenAIProvider(llmConfig.APIKey, llmConfig.BaseURL, llmConfig.Model)
	a.aiService.UpdateProvider(newProvider)

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

// PolishRootCause polishes the root cause description
func (a *App) PolishRootCause(input string) string {
	polished, err := a.aiService.PolishContent(input)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return polished
}

// GetSessionTimeline returns the current session data including timeline and problem
func (a *App) GetSessionTimeline() *session_recorder.TroubleshootingSession {
	session := a.recorder.GetCurrentSession()
	if session == nil {
		return nil
	}
	return session
}

// UpdateSessionTimeline updates the current session timeline
func (a *App) UpdateSessionTimeline(events []session_recorder.TimelineEvent) string {
	err := a.recorder.UpdateTimeline(events)
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

func (a *App) RenameSavedSession(id, newName string) string {
	if err := a.savedSessionMgr.RenameSession(id, newName); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return ""
}
