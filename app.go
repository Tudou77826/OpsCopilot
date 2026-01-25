package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"opscopilot/pkg/ai"
	"opscopilot/pkg/completion"
	"opscopilot/pkg/config"
	"opscopilot/pkg/filetransfer"
	"opscopilot/pkg/javamonitor"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/llm"
	"opscopilot/pkg/secretstore"
	"opscopilot/pkg/session"
	"opscopilot/pkg/session_recorder"
	"opscopilot/pkg/sessionmanager"
	"opscopilot/pkg/sshclient"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"
)

// App struct
type App struct {
	ctx               context.Context
	sessionMgr        *session.Manager
	savedSessionMgr   *sessionmanager.Manager
	secretStore       secretstore.SecretStore
	aiService         *ai.AIService
	configMgr         *config.Manager
	recorder          *session_recorder.Recorder
	completionService *completion.Service
	activeConfigs     map[string]ConnectConfig
	isForceQuitting   bool // Flag to skip confirmation on force quit
	ftMu              sync.Mutex
	ftCancels         map[string]context.CancelFunc
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

	// Initialize Recorder
	recorder := session_recorder.NewRecorder("sessions")

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

	return &App{
		sessionMgr:        session.NewManager(),
		savedSessionMgr:   savedMgr,
		secretStore:       secretstore.NewKeyringStore(),
		aiService:         aiService,
		configMgr:         configMgr,
		recorder:          recorder,
		completionService: completionService,
		activeConfigs:     make(map[string]ConnectConfig),
		isForceQuitting:   false,
		ftCancels:         make(map[string]context.CancelFunc),
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

// beforeClose is called before the application closes
// Returns true to prevent close, false to allow close
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	// If this is a forced quit, skip confirmation and allow close
	if a.isForceQuitting {
		log.Println("[beforeClose] Force quitting, allowing close")
		return false
	}

	// Check if there are active terminal sessions
	activeSessions := a.sessionMgr.List()
	hasTerminals := len(activeSessions) > 0

	// Check if there's an ongoing troubleshooting session
	hasTroubleshooting := a.recorder.GetCurrentSession() != nil

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
	// No active work, allow close
	return false
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
	sessionID := a.sessionMgr.Add(client, stdin, sshSession)

	// Store config mapping for duplication
	a.activeConfigs[sessionID] = config

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

		// Record input
		a.recordInput(sessionID, data)
	}
}

// recordInput cleans and records terminal input
func (a *App) recordInput(sessionID string, data string) {
	if a.recorder == nil {
		return
	}

	// Pass raw data to recorder, which now uses LineBuffer to handle ANSI codes and editing
	a.recorder.AddEvent("terminal_input", data, map[string]interface{}{
		"session_id": sessionID,
	})
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
	if len(sessionIDs) == 0 {
		return
	}
	a.sessionMgr.Broadcast(sessionIDs, data)

	// Record broadcast input using specialized method for deduplication
	if a.recorder != nil {
		a.recorder.AddBroadcastInput(sessionIDs, data)
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

// AskTroubleshoot handles the troubleshooting request from frontend and returns structured JSON
func (a *App) AskTroubleshoot(problem string) string {
	// 1. Load knowledge
	knowledgeDir := a.resolveKnowledgeBase()
	contextContent, err := knowledge.LoadAll(knowledgeDir)
	if err != nil {
		log.Printf("[App] Warning: Failed to load knowledge from %s: %v", knowledgeDir, err)
		contextContent = ""
	}

	// 2. Call AIService
	answer, err := a.aiService.AskTroubleshoot(problem, contextContent)
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
	if te != nil && te.Code == filetransfer.ErrorCodeSFTPNotSupported {
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
		return mustJSON(ftResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeSFTPNotSupported, Message: "对端未开启 SFTP，且未安装 scp"}})
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
			if te, ok := opErr.(*filetransfer.TransferError); ok && te.Code == filetransfer.ErrorCodeSFTPNotSupported {
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
				} else if checkErr == nil && !ok {
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

// HasActiveWork checks if there are active terminal sessions or ongoing troubleshooting session
func (a *App) HasActiveWork() map[string]interface{} {
	hasTerminals := len(a.sessionMgr.List()) > 0
	hasTroubleshooting := a.recorder.GetCurrentSession() != nil

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

func (a *App) ListJavaProcesses(sessionID string) string {
	sess, ok := a.sessionMgr.Get(sessionID)
	if !ok || sess == nil || sess.Client == nil {
		return "Error: Session not found"
	}

	procs, err := javamonitor.ListJavaProcesses(sess.Client)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	data, err := json.Marshal(procs)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return string(data)
}

func (a *App) GetJavaMonitorSnapshot(sessionID string, pid int) string {
	sess, ok := a.sessionMgr.Get(sessionID)
	if !ok || sess == nil || sess.Client == nil {
		return "Error: Session not found"
	}
	if pid <= 0 {
		return "Error: Invalid pid"
	}

	snap, err := javamonitor.GetSnapshot(sess.Client, pid)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return string(data)
}

func (a *App) GetJavaTopCPUThreads(sessionID string, pid int) string {
	sess, ok := a.sessionMgr.Get(sessionID)
	if !ok || sess == nil || sess.Client == nil {
		return "Error: Session not found"
	}
	if pid <= 0 {
		return "Error: Invalid pid"
	}
	list, err := javamonitor.GetTopCPUThreads(sess.Client, pid, 3)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	data, err := json.Marshal(list)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return string(data)
}

func (a *App) GetJavaThreadStateCounts(sessionID string, pid int) string {
	sess, ok := a.sessionMgr.Get(sessionID)
	if !ok || sess == nil || sess.Client == nil {
		return "Error: Session not found"
	}
	if pid <= 0 {
		return "Error: Invalid pid"
	}
	c, err := javamonitor.GetThreadStateCounts(sess.Client, pid)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return string(data)
}
