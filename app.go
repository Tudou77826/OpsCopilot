package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"

	"opscopilot/pkg/ai"
	"opscopilot/pkg/completion"
	"opscopilot/pkg/config"
	"opscopilot/pkg/core/security"
	"opscopilot/pkg/filetransfer"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/knowledge/patchstore"
	"opscopilot/pkg/llm"
	"opscopilot/pkg/logging"
	"opscopilot/pkg/recorder"
	"opscopilot/pkg/script"
	"opscopilot/pkg/secretstore"
	"opscopilot/pkg/session"
	"opscopilot/pkg/sessionmanager"
	"opscopilot/pkg/sshclient"
	"opscopilot/pkg/terminal"
	"opscopilot/pkg/troubleshoot"
	"opscopilot/pkg/updater"
)

// Version is set via -ldflags "-X main.Version=..." at build time.
var Version = "dev"

// App struct
type App struct {
	ctx               context.Context
	sessionMgr        *session.Manager
	savedSessionMgr   *sessionmanager.Manager
	secretStore       secretstore.SecretStore
	aiService         *ai.AIService
	configMgr         *config.Manager
	coreRecorder      *recorder.Recorder    // 统一录制引擎
	troubleMgr        *troubleshoot.Manager // 故障排查管理器
	scriptMgr         *script.Manager       // 脚本管理器
	completionService *completion.Service
	whitelistMgr      *security.WhitelistManager  // 命令白名单管理器
	fileAccessMgr     *security.FileAccessChecker // 文件访问控制管理器
	activeConfigs     map[string]ConnectConfig
	activeConfigsMu   sync.RWMutex // protects activeConfigs
	isForceQuitting   bool         // Flag to skip confirmation on force quit
	ftMu              sync.Mutex
	ftCancels         map[string]context.CancelFunc
	relayMu           sync.Mutex
	relayTransports   map[string]*filetransfer.RootRelayTransport
	shellTransports   map[string]*filetransfer.RootRelayTransport
	sessionStates     map[string]*SessionState // 会话状态追踪
	sessionStateMu    sync.RWMutex
	commandExtractors map[string]*terminal.CommandExtractor // 命令提取器（Tab 补全修正）
	extractorMu       sync.RWMutex                          // 命令提取器锁
	patchStoreMu      sync.RWMutex
	patchStore        patchstore.PatchStore    // 补丁存储（可选）
	feedbackStore     patchstore.FeedbackStore // 反馈存储（可选）
	patchSyncStatusMu sync.RWMutex
	patchSyncStatus   PatchSyncStatus
	patchSyncing      atomic.Bool
}

// NewApp creates a new App application struct
func NewApp() *App {
	configMgr := config.NewManager()
	if err := configMgr.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Failed to load config: %v\n", err)
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
	// 注入 Wails 事件发射器，使 agent 运行状态可推送到前端
	ai.SetEventEmitter(runtime.EventsEmit)

	// Initialize Core Recorder Engine (统一录制器)
	recordingsPath := filepath.Join(configMgr.Config.Log.Dir, "recordings")
	coreRecorder := recorder.NewRecorder(recordingsPath)

	// Scripts 使用独立目录（与 logs 分离）
	scriptsDir := configMgr.Config.Scripts.Dir
	if scriptsDir == "" {
		scriptsDir = filepath.Join(filepath.Dir(configMgr.Config.Log.Dir), "scripts")
	}

	// Initialize Session Manager (will be used in App)
	sessionMgrInstance := session.NewManager()

	// Initialize Troubleshoot Manager
	troubleMgr := troubleshoot.NewManager(coreRecorder, recordingsPath)

	// Initialize Script Manager (使用独立脚本目录)
	scriptMgr := script.NewManager(coreRecorder, scriptsDir, nil)

	// 迁移旧脚本数据并清理冗余的 recording 文件
	migrateScriptsFromLegacyPath(recordingsPath, scriptsDir)
	cleanupRedundantRecordingFiles(scriptsDir)

	// Initialize Saved Session Manager
	savedMgr := sessionmanager.NewManager()
	if err := savedMgr.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Failed to load saved sessions: %v\n", err)
	}

	// Initialize Completion Service
	completionDB, err := completion.NewDatabase()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Failed to initialize completion database: %v\n", err)
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
		relayTransports:   make(map[string]*filetransfer.RootRelayTransport),
		shellTransports:   make(map[string]*filetransfer.RootRelayTransport),
		sessionStates:     make(map[string]*SessionState),
	}

	// 初始化白名单管理器
	whitelistPath := "command_whitelist.json"
	if whitelistMgr, err := security.NewWhitelistManager(whitelistPath); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Failed to initialize whitelist manager: %v\n", err)
	} else {
		app.whitelistMgr = whitelistMgr
	}

	// 初始化文件访问控制管理器
	fileAccessPath := "file_access.json"
	if fileAccessMgr, err := security.NewFileAccessChecker(fileAccessPath); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Failed to initialize file access checker: %v\n", err)
	} else {
		app.fileAccessMgr = fileAccessMgr
	}

	// Set the CommandSender to app itself
	scriptMgr.SetCommandSender(app)

	// 构建知识库目录
	knowledgeDir := app.resolveKnowledgeBase()
	slog.Info("startup: resolved knowledge base", "dir", knowledgeDir)
	if knowledgeDir != "" {
		if err := aiService.UpdateCatalog(knowledgeDir); err != nil {
			slog.Error("startup: failed to build knowledge catalog", "error", err)
		} else if aiService.GetCatalog() != nil {
			slog.Info("startup: knowledge catalog built", "scenarios", aiService.GetCatalog().TotalScenarios())
		}
	}

	return app
}

// getConfig safely reads a session's ConnectConfig.
func (a *App) getConfig(sessionID string) (ConnectConfig, bool) {
	a.activeConfigsMu.RLock()
	cfg, ok := a.activeConfigs[sessionID]
	a.activeConfigsMu.RUnlock()
	return cfg, ok
}

// setConfig safely writes a session's ConnectConfig.
func (a *App) setConfig(sessionID string, cfg ConnectConfig) {
	a.activeConfigsMu.Lock()
	a.activeConfigs[sessionID] = cfg
	a.activeConfigsMu.Unlock()
}

func (a *App) getPatchStore() patchstore.PatchStore {
	a.patchStoreMu.RLock()
	defer a.patchStoreMu.RUnlock()
	return a.patchStore
}

func (a *App) setPatchStore(store patchstore.PatchStore) {
	a.patchStoreMu.Lock()
	a.patchStore = store
	a.patchStoreMu.Unlock()
}

func (a *App) updatePatchSyncStatus(update func(*PatchSyncStatus)) {
	a.patchSyncStatusMu.Lock()
	defer a.patchSyncStatusMu.Unlock()
	update(&a.patchSyncStatus)
}

func (a *App) getPatchSyncStatusSnapshot() PatchSyncStatus {
	a.patchSyncStatusMu.RLock()
	defer a.patchSyncStatusMu.RUnlock()
	return a.patchSyncStatus
}

func (a *App) refreshPatchSyncStatusConfig() {
	cfg := a.configMgr.Config.PatchStore
	remoteURL := strings.TrimSpace(cfg.RemoteURL)
	branch := strings.TrimSpace(cfg.Branch)
	if branch == "" {
		branch = "main"
	}

	pendingCount := 0
	if pendingStore := a.newPendingPatchStore(); pendingStore != nil {
		if patches, err := pendingStore.List(); err == nil {
			pendingCount = len(patches)
		}
	}

	a.updatePatchSyncStatus(func(status *PatchSyncStatus) {
		status.Enabled = cfg.Enabled
		status.Configured = remoteURL != ""
		status.RemoteURL = remoteURL
		status.Branch = branch
		status.PendingCount = pendingCount
		if !cfg.Enabled || remoteURL == "" {
			status.Running = false
		}
	})
}

func (a *App) pendingPatchStoreDir() string {
	return filepath.Join(filepath.Dir(a.configMgr.Config.Log.Dir), "patchstore-pending")
}

func (a *App) newPendingPatchStore() *patchstore.PendingStore {
	return patchstore.NewPendingStore(a.pendingPatchStoreDir())
}

func (a *App) buildArchivePatch(input *knowledge.ArchiveInput, archivedAt time.Time) *patchstore.Patch {
	record := input.Record
	if record == "" {
		record = knowledge.BuildArchiveRecord(input, archivedAt)
	}

	return &patchstore.Patch{
		ID:        uuid.New().String()[:8],
		Service:   input.Service,
		Module:    input.Module,
		Timestamp: archivedAt,
		Content:   strings.TrimSpace(record),
	}
}

// getRelayTransport returns a cached RootRelayTransport for the session.
func (a *App) getRelayTransport(sessionID string) *filetransfer.RootRelayTransport {
	a.relayMu.Lock()
	defer a.relayMu.Unlock()

	if a.relayTransports == nil {
		a.relayTransports = make(map[string]*filetransfer.RootRelayTransport)
	}

	if t, ok := a.relayTransports[sessionID]; ok {
		return t
	}

	sess, ok := a.sessionMgr.Get(sessionID)
	if !ok || sess.Client == nil || sess.Client.SSHClient() == nil {
		slog.Debug("ft getRelayTransport session not found or SSH client nil", "session", sessionID[:8])
		return nil
	}

	cfg, cfgOk := a.getConfig(sessionID)
	if !cfgOk || cfg.RootPassword == "" {
		slog.Debug("ft getRelayTransport session has no root password", "session", sessionID[:8])
		return nil
	}

	slog.Info("ft getRelayTransport created new RootRelayTransport", "session", sessionID[:8], "loginUser", cfg.User)
	t := filetransfer.NewRootRelayTransport(sess.Client.SSHClient(), cfg.RootPassword, cfg.User)
	if cfg.Bastion != nil {
		t.SetSkipRelay(true)
	}
	a.relayTransports[sessionID] = t
	return t
}

// closeRelayTransport closes and removes the cached relay transport for a session.
func (a *App) closeRelayTransport(sessionID string) {
	a.relayMu.Lock()
	defer a.relayMu.Unlock()
	if t, ok := a.relayTransports[sessionID]; ok {
		t.Close()
		delete(a.relayTransports, sessionID)
	}
	if t, ok := a.shellTransports[sessionID]; ok {
		t.Close()
		delete(a.shellTransports, sessionID)
	}
}

// getShellTransport returns a cached RootRelayTransport for root-identity sessions
// where SFTP is not available. It uses the root SSH client directly (no su needed).
func (a *App) getShellTransport(sessionID string, client *ssh.Client) *filetransfer.RootRelayTransport {
	a.relayMu.Lock()
	defer a.relayMu.Unlock()

	if a.shellTransports == nil {
		a.shellTransports = make(map[string]*filetransfer.RootRelayTransport)
	}
	if t, ok := a.shellTransports[sessionID]; ok {
		return t
	}

	// loginUser="" means we are already root, RootRelayTransport will skip su
	slog.Info("ft getShellTransport created new ShellTransport (root direct, skip su)", "session", sessionID[:8])
	t := filetransfer.NewRootRelayTransport(client, "", "")
	a.shellTransports[sessionID] = t
	return t
}

// GetVersion returns the current application version.
func (a *App) GetVersion() string {
	return Version
}

// GetReleaseHistory fetches published releases for the About panel's version log.
// Returns JSON with a "releases" array (newest first) or an "error" field on failure.
func (a *App) GetReleaseHistory() string {
	releases, err := updater.FetchReleaseHistory()
	if err != nil {
		slog.Warn("fetch release history failed", "error", err)
		result, _ := json.Marshal(map[string]interface{}{
			"error": err.Error(),
		})
		return string(result)
	}
	result, _ := json.Marshal(map[string]interface{}{
		"releases": releases,
	})
	return string(result)
}

// CheckUpdate checks GitHub for the latest release and returns update status as JSON.
func (a *App) CheckUpdate() string {
	status, err := updater.CheckForUpdate(Version)
	if err != nil {
		slog.Warn("check update failed", "error", err)
		result, _ := json.Marshal(map[string]interface{}{
			"hasUpdate":      false,
			"currentVersion": Version,
			"error":          err.Error(),
		})
		return string(result)
	}
	result, _ := json.Marshal(status)
	slog.Info("check update result", "hasUpdate", status.HasUpdate, "latest", status.LatestVer)
	return string(result)
}

// DoUpdate downloads the update, writes a manifest, and relaunches self in update mode.
func (a *App) DoUpdate(downloadURL string) string {
	slog.Info("update: starting", "url", downloadURL)

	exePath, err := os.Executable()
	if err != nil {
		return toJSONError(fmt.Sprintf("get exe path: %v", err))
	}
	exeDir := filepath.Dir(exePath)
	tempDir := filepath.Join(os.TempDir(), "opscopilot_update")
	slog.Info("update: paths resolved", "exePath", exePath, "exeDir", exeDir, "tempDir", tempDir)

	progressFn := func(p updater.DownloadProgress) {
		runtime.EventsEmit(a.ctx, "update-download-progress", map[string]interface{}{
			"bytesDownloaded": p.BytesDownloaded,
			"bytesTotal":      p.BytesTotal,
			"percentage":      p.Percentage,
			"speedBps":        p.SpeedBps,
		})
	}

	slog.Info("update: downloading and extracting...")
	extractedDir, err := updater.DownloadAndExtract(downloadURL, tempDir, progressFn)
	if err != nil {
		slog.Error("update: download/extract failed", "error", err)
		return toJSONError(fmt.Sprintf("download: %v", err))
	}
	slog.Info("update: download complete, extracted", "extractedDir", extractedDir)

	manifest := &updater.Manifest{
		ExtractedDir: extractedDir,
		AppDir:       exeDir,
		ExePath:      exePath,
		ParentPid:    os.Getpid(),
		Version:      Version,
		LogPath:      filepath.Join(exeDir, "update.log"),
	}
	manifestPath := filepath.Join(tempDir, "manifest.json")
	if err := updater.WriteManifest(manifest, manifestPath); err != nil {
		slog.Error("update: write manifest failed", "error", err)
		return toJSONError(fmt.Sprintf("write manifest: %v", err))
	}

	if err := launchSelfUpdate(exePath, manifestPath); err != nil {
		slog.Error("update: launch self-update failed", "error", err)
		return toJSONError(fmt.Sprintf("launch updater: %v", err))
	}
	slog.Info("update: self-update launched, scheduling quit")

	runtime.EventsEmit(a.ctx, "update-ready", map[string]interface{}{"ok": true})

	go func() {
		time.Sleep(2 * time.Second)
		a.isForceQuitting = true
		runtime.Quit(a.ctx)
	}()

	result, _ := json.Marshal(map[string]interface{}{"ok": true})
	return string(result)
}

// launchSelfUpdate starts the updater process (detached, hidden window).
func launchSelfUpdate(exePath, manifestPath string) error {
	cmd := exec.Command(exePath, "--self-update", manifestPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
	return cmd.Start()
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// 初始化命令提取器
	a.commandExtractors = make(map[string]*terminal.CommandExtractor)

	// 初始化日志：slog + lumberjack 轮转
	logging.Setup(logging.Config{
		Dir:        a.configMgr.Config.Log.Dir,
		Level:      a.configMgr.Config.Log.Level,
		DevMode:    os.Getenv("OPSCOPILOT_DEV_MODE") == "true",
		MaxSizeMB:  10,
		MaxBackups: 5,
		Compress:   true,
	})

	slog.Info("app started")

	// 初始化补丁存储（如果已配置）
	a.initPatchStore()

	// 初始化反馈存储（复用同一 Git 仓库）
	a.initFeedbackStore()

	// 清理自更新残留文件，并显示更新结果
	if execPath, err := os.Executable(); err == nil {
		appDir := filepath.Dir(execPath)
		if result, err := updater.CleanupAfterUpdate(appDir, updater.OSFS{}); err != nil {
			slog.Warn("post-update cleanup failed", "error", err)
		} else if result != nil {
			slog.Info("update result", "success", result.Success, "version", result.Version)
			runtime.EventsEmit(ctx, "update-completed", result)
		}
	}
}

// beforeClose is called before the application closes
// Returns true to prevent close, false to allow close
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	// If this is a forced quit, skip confirmation and allow close
	if a.isForceQuitting {
		slog.Info("beforeClose force quitting, allowing close")
		return false
	}

	// Check if there are active terminal sessions
	activeSessions := a.sessionMgr.List()
	hasTerminals := len(activeSessions) > 0

	// Check if there's an ongoing troubleshooting session
	hasTroubleshooting := a.coreRecorder.GetCurrentSession() != nil

	// If there's any active work, we need to ask for confirmation
	if hasTerminals || hasTroubleshooting {
		slog.Info("beforeClose active work detected", "terminals", len(activeSessions), "troubleshooting", hasTroubleshooting)

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

	slog.Info("beforeClose no active work, allowing close")
	// No active work, allow close
	return false
}

// ForceQuit forces the application to quit without confirmation
func (a *App) ForceQuit() {
	slog.Info("forceQuit setting flag and calling runtime.Quit()")

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

type PatchSyncStatus struct {
	Enabled         bool   `json:"enabled"`
	Configured      bool   `json:"configured"`
	Running         bool   `json:"running"`
	PendingCount    int    `json:"pendingCount"`
	LastSyncAt      string `json:"lastSyncAt,omitempty"`
	LastSyncSuccess bool   `json:"lastSyncSuccess"`
	LastSyncMessage string `json:"lastSyncMessage,omitempty"`
	RemoteURL       string `json:"remoteURL,omitempty"`
	Branch          string `json:"branch,omitempty"`
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
	a.setConfig(sessionID, config)

	// Store session state for reconnection
	a.storeSessionState(sessionID, config)

	// 创建命令提取器（用于 Tab 补全修正）
	a.extractorMu.Lock()
	a.commandExtractors[sessionID] = terminal.NewCommandExtractor()
	a.extractorMu.Unlock()

	// Auto-save session to persistent storage
	if err := a.savedSessionMgr.Upsert(*clientConfig, config.Group); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Failed to auto-save session: %v\n", err)
	}

	// Read loop
	go func() {
		buf := make([]byte, 32768)
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

				// 清理命令提取器
				a.extractorMu.Lock()
				delete(a.commandExtractors, sessionID)
				a.extractorMu.Unlock()

				// 从会话管理器移除（清理SSH资源）
				a.sessionMgr.Remove(sessionID)
				break
			}
			if n > 0 {
				dataStr := readTerminalChunk(stdout, buf, n)
				runtime.EventsEmit(a.ctx, "terminal-data:"+sessionID, dataStr)

				// Record output
				if a.coreRecorder != nil {
					a.coreRecorder.AddEvent("terminal_output", dataStr, map[string]interface{}{
						"session_id": sessionID,
					})
				}

				// 尝试从输出中提取命令（仅用于 Tab 补全修正）
				a.extractorMu.RLock()
				extractor, ok := a.commandExtractors[sessionID]
				a.extractorMu.RUnlock()

				if ok && extractor != nil {
					if cmd, found := extractor.ProcessOutput(dataStr); found {
						if a.coreRecorder != nil {
							// 更新最后一条命令（Tab 补全场景）
							if a.coreRecorder.UpdateLastCommand(sessionID, cmd) {
								slog.Debug("recorder corrected command via output (tab completion)", "cmd", cmd)
							}
						}
					}
				}
			}
		}
	}()

	return ConnectResult{Success: true, SessionID: sessionID, Message: "Connected"}
}

// ResizeTerminal resizes the PTY for a given session
func (a *App) ResizeTerminal(sessionID string, cols int, rows int) {
	if err := a.sessionMgr.Resize(sessionID, cols, rows); err != nil {
		slog.Warn("resizeTerminal failed to resize session", "session", sessionID, "error", err)
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
	result, err := a.coreRecorder.AddEvent("terminal_input", data, map[string]interface{}{
		"session_id": sessionID,
	})
	if err != nil {
		// "not recording" 是预期状态：用户未开启故障排查会话时，绝大多数终端输入都不在录制中。
		// 这不是错误，静默忽略，避免每个按键都刷一条 WARN 日志。
		if err.Error() == "not recording" {
			return
		}
		slog.Warn("recordInput error recording input", "error", err)
		return
	}

	// 获取命令提取器
	a.extractorMu.RLock()
	extractor, extractorOk := a.commandExtractors[sessionID]
	a.extractorMu.RUnlock()

	// 如果命令被提交且有内容
	if result.Committed && result.Line != "" {
		runtime.EventsEmit(a.ctx, "script-command-recorded", result.Line)
		slog.Debug("scriptRecording recorded command", "session", sessionID, "cmd", result.Line)

		// 设置待匹配的命令前缀（用于检测 Tab 补全修正）
		if extractorOk && extractor != nil {
			extractor.SetPendingInput(result.Line)
		}
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
			slog.Error("failed to generate conclusion", "error", err)
			conclusion = "Failed to generate conclusion via AI."
		}
	}

	// Stop and Save Session (JSON)
	if err := a.coreRecorder.StopSession(rootCause, conclusion); err != nil {
		return fmt.Sprintf("Error saving session: %v", err)
	}

	// Append to troubleshooting_history.md in docs directory
	if err := a.appendConclusionToDocs(conclusion); err != nil {
		slog.Error("failed to append conclusion to docs", "error", err)
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

// ServiceInfo 前端选择用的微服务信息
type ServiceInfo struct {
	Name    string       `json:"name"`
	Modules []ModuleInfo `json:"modules"`
}

// ModuleInfo 前端选择用的模块信息
type ModuleInfo struct {
	Name     string `json:"name"`
	FileName string `json:"fileName"`
}

// GetCatalogServices 返回 Catalog 中的 service/module 结构供前端选择
func (a *App) GetCatalogServices() []ServiceInfo {
	catalog := a.aiService.GetCatalog()

	// 如果 Catalog 为空，尝试重新构建一次（可能是首次启动时构建时机问题）
	if catalog == nil || len(catalog.Services) == 0 {
		knowledgeDir := a.resolveKnowledgeBase()
		slog.Info("GetCatalogServices: catalog empty, rebuilding", "knowledgeDir", knowledgeDir)
		if knowledgeDir != "" {
			if err := a.aiService.UpdateCatalog(knowledgeDir); err != nil {
				slog.Error("GetCatalogServices: failed to rebuild catalog", "error", err)
			} else {
				catalog = a.aiService.GetCatalog()
				slog.Info("GetCatalogServices: catalog rebuilt", "services", len(catalog.Services))
			}
		}
	}

	if catalog == nil {
		slog.Warn("GetCatalogServices: catalog is nil after rebuild attempt")
		return []ServiceInfo{} // 返回空数组而不是 nil，方便前端处理
	}

	result := make([]ServiceInfo, 0, len(catalog.Services))
	for _, svc := range catalog.Services {
		info := ServiceInfo{
			Name:    svc.Name,
			Modules: make([]ModuleInfo, 0, len(svc.Modules)),
		}
		for _, mod := range svc.Modules {
			// 从模块的场景条目中获取关联的文件名
			fileName := ""
			if len(mod.Scenarios) > 0 {
				fileName = mod.Scenarios[0].File
			}
			info.Modules = append(info.Modules, ModuleInfo{
				Name:     mod.Name,
				FileName: fileName,
			})
		}
		result = append(result, info)
	}
	slog.Info("GetCatalogServices: returning", "services", len(result))
	return result
}

// ArchiveSession 归档排查会话到指定文件
func (a *App) ArchiveSession(rootCause string, conclusion string, service string, module string, targetFile string) string {
	currentSession := a.coreRecorder.GetCurrentSession()
	if currentSession == nil {
		return toJSONError("No active session")
	}

	// 如果 conclusion 为空 → 调用 AI 生成
	if conclusion == "" {
		timelineBytes, err := json.Marshal(currentSession.Timeline)
		if err != nil {
			slog.Error("failed to marshal timeline", "error", err)
			return toJSONError("Failed to serialize timeline")
		}
		timelineStr := string(timelineBytes)
		conclusion, err = a.aiService.GenerateConclusion(timelineStr, rootCause)
		if err != nil {
			slog.Error("failed to generate conclusion", "error", err)
			return toJSONError("Failed to generate conclusion")
		}
	}

	// 追加归档记录到知识库文件
	knowledgeDir := a.resolveKnowledgeBase()
	archivedAt := time.Now()
	input := &knowledge.ArchiveInput{
		Session:    currentSession,
		Conclusion: conclusion,
		Service:    service,
		Module:     module,
		FilePath:   targetFile,
	}
	input.Record = knowledge.BuildArchiveRecord(input, archivedAt)

	var pendingStore *patchstore.PendingStore
	var patch *patchstore.Patch
	if store := a.getPatchStore(); store != nil {
		pendingStore = a.newPendingPatchStore()
		patch = a.buildArchivePatch(input, archivedAt)
		if err := pendingStore.Save(*patch); err != nil {
			slog.Error("failed to save pending patch", "error", err, "service", input.Service, "module", input.Module)
			return toJSONError("Failed to queue patch for sync")
		}
	}

	relPath, err := knowledge.AppendRecord(knowledgeDir, input)
	if err != nil {
		if pendingStore != nil && patch != nil {
			if rmErr := pendingStore.Delete(*patch); rmErr != nil {
				slog.Warn("failed to rollback pending patch after archive failure", "error", rmErr, "patch_id", patch.ID)
			}
		}
		slog.Error("failed to archive session", "error", err)
		return toJSONError("Failed to archive session")
	}

	// 保留 recorder JSON 保存
	if err := a.coreRecorder.StopSession(rootCause, conclusion); err != nil {
		slog.Error("failed to save session recording", "error", err)
	}

	// 重建 Catalog
	if err := a.aiService.UpdateCatalog(knowledgeDir); err != nil {
		slog.Error("failed to update catalog after archive", "error", err)
	}

	slog.Info("archiveSession archived", "path", relPath)

	// 上传补丁到共享存储（异步，不阻塞归档）
	if store := a.getPatchStore(); store != nil && pendingStore != nil && patch != nil {
		go a.uploadPatch(store, pendingStore, *patch)
	}

	result, _ := json.Marshal(map[string]interface{}{
		"success":    true,
		"conclusion": conclusion,
		"filePath":   relPath,
	})
	return string(result)
}

// toJSONError 返回标准错误 JSON
func toJSONError(msg string) string {
	result, _ := json.Marshal(map[string]interface{}{
		"success": false,
		"error":   msg,
	})
	return string(result)
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
	a.closeRelayTransport(sessionID)
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
		slog.Warn("configured docs directory not found, falling back to auto-discovery", "dir", configuredDir)
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
func (a *App) AskTroubleshoot(problem string) string {
	knowledgeDir := a.resolveKnowledgeBase()
	slog.Info("askTroubleshoot problem", "problem", problem)
	answer, err := a.aiService.AskTroubleshoot(a.ctx, problem, knowledgeDir)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return answer
}

func (a *App) GetSettings() config.AppConfig {
	return *a.configMgr.Config
}

func (a *App) GetPatchSyncStatus() string {
	a.refreshPatchSyncStatusConfig()
	status := a.getPatchSyncStatusSnapshot()
	data, _ := json.Marshal(status)
	return string(data)
}

func (a *App) RetryPatchSync() string {
	store := a.getPatchStore()
	if store == nil {
		a.refreshPatchSyncStatusConfig()
		return "补丁同步未启用或未配置 Git 仓库"
	}

	if err := a.runPatchSync(store); err != nil {
		return err.Error()
	}
	return ""
}

// --- 知识库浏览器 API ---

// GetKnowledgeTree 返回完整的 Catalog JSON（服务>模块>场景树）
func (a *App) GetKnowledgeTree() string {
	catalog := a.aiService.GetCatalog()
	if catalog == nil || len(catalog.Services) == 0 {
		knowledgeDir := a.resolveKnowledgeBase()
		if knowledgeDir != "" {
			if err := a.aiService.UpdateCatalog(knowledgeDir); err != nil {
				slog.Error("GetKnowledgeTree: failed to build catalog", "error", err)
			} else {
				catalog = a.aiService.GetCatalog()
			}
		}
	}
	if catalog == nil {
		return `{"version":0,"services":[]}`
	}
	data, err := json.Marshal(catalog)
	if err != nil {
		return `{"version":0,"services":[]}`
	}
	return string(data)
}

// GetKnowledgeFileContent 读取知识库中指定文件的内容
func (a *App) GetKnowledgeFileContent(relPath string) string {
	knowledgeDir := a.resolveKnowledgeBase()
	if knowledgeDir == "" {
		return toJSONError("知识库未找到")
	}
	content, err := knowledge.ReadFile(knowledgeDir, relPath)
	if err != nil {
		result, _ := json.Marshal(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return string(result)
	}
	result, _ := json.Marshal(map[string]interface{}{
		"success": true,
		"content": content,
	})
	return string(result)
}

// GetKnowledgeScenarioContent 按行范围读取文件中的指定场景段落
func (a *App) GetKnowledgeScenarioContent(relPath string, lineStart int, lineEnd int) string {
	knowledgeDir := a.resolveKnowledgeBase()
	if knowledgeDir == "" {
		return toJSONError("知识库未找到")
	}
	content, err := knowledge.ReadFile(knowledgeDir, relPath)
	if err != nil {
		result, _ := json.Marshal(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return string(result)
	}
	lines := strings.Split(content, "\n")
	start := lineStart - 1
	if start < 0 {
		start = 0
	}
	end := lineEnd
	if end > len(lines) {
		end = len(lines)
	}
	if start >= len(lines) {
		start = len(lines)
	}
	section := strings.Join(lines[start:end], "\n")
	result, _ := json.Marshal(map[string]interface{}{
		"success": true,
		"content": section,
	})
	return string(result)
}

// GetPatchFeedback 获取某个 patch 的所有评分和 Issue
func (a *App) GetPatchFeedback(patchID string) string {
	fs := a.feedbackStore
	if fs == nil {
		return `{"feedback":[]}`
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	feedback, err := fs.GetFeedback(ctx, patchID)
	if err != nil {
		slog.Error("GetPatchFeedback failed", "error", err, "patchID", patchID)
		return `{"feedback":[]}`
	}
	result, _ := json.Marshal(map[string]interface{}{
		"feedback": feedback,
	})
	return string(result)
}

// RatePatch 为当前用户对某个 patch 提交评分
func (a *App) RatePatch(patchID string, score int, comment string) string {
	if score < 1 || score > 5 {
		return toJSONError("评分必须在 1-5 之间")
	}

	user := a.getCurrentUser()

	fs := a.feedbackStore
	if fs == nil {
		return toJSONError("反馈存储未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 查找该用户已有的反馈（保留 issues）
	existing, err := fs.GetFeedback(ctx, patchID)
	if err != nil {
		slog.Error("RatePatch: failed to get existing feedback", "error", err, "patchID", patchID)
		return toJSONError("获取已有反馈失败")
	}
	var fb patchstore.UserFeedback
	for _, e := range existing {
		if e.User == user {
			fb = e
			break
		}
	}
	if fb.PatchID == "" {
		fb = patchstore.UserFeedback{
			PatchID: patchID,
			User:    user,
		}
	}
	fb.Rating = &patchstore.Rating{
		Score:     score,
		Comment:   comment,
		Timestamp: time.Now(),
	}

	if err := fs.SaveFeedback(ctx, fb); err != nil {
		slog.Error("RatePatch failed", "error", err, "patchID", patchID)
		return toJSONError(fmt.Sprintf("评分保存失败: %v", err))
	}

	result, _ := json.Marshal(map[string]interface{}{"success": true})
	return string(result)
}

// ReportPatchIssue 为某个 patch 提交 Issue
func (a *App) ReportPatchIssue(patchID string, issueType string, priority string, title string, description string) string {
	if strings.TrimSpace(title) == "" {
		return toJSONError("标题不能为空")
	}
	if len(description) > 5000 {
		return toJSONError("描述过长")
	}
	validTypes := map[string]bool{"bug": true, "outdated": true, "suggestion": true}
	if !validTypes[issueType] {
		return toJSONError("无效的 Issue 类型")
	}
	validPriorities := map[string]bool{"high": true, "medium": true, "low": true}
	if !validPriorities[priority] {
		return toJSONError("无效的优先级")
	}

	user := a.getCurrentUser()

	fs := a.feedbackStore
	if fs == nil {
		return toJSONError("反馈存储未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	issue := patchstore.Issue{
		ID:          shortUUID(),
		Type:        issueType,
		Priority:    priority,
		Title:       title,
		Description: description,
		Reporter:    user,
		Status:      "open",
		Timestamp:   time.Now(),
	}

	// 查找该用户已有的反馈（保留 rating）
	existing, err := fs.GetFeedback(ctx, patchID)
	if err != nil {
		slog.Error("ReportPatchIssue: failed to get existing feedback", "error", err, "patchID", patchID)
		return toJSONError("获取已有反馈失败")
	}
	var fb patchstore.UserFeedback
	for _, e := range existing {
		if e.User == user {
			fb = e
			break
		}
	}
	if fb.PatchID == "" {
		fb = patchstore.UserFeedback{
			PatchID: patchID,
			User:    user,
		}
	}
	fb.Issues = append(fb.Issues, issue)

	if err := fs.SaveFeedback(ctx, fb); err != nil {
		slog.Error("ReportPatchIssue failed", "error", err, "patchID", patchID)
		return toJSONError(fmt.Sprintf("Issue 提交失败: %v", err))
	}

	result, _ := json.Marshal(map[string]interface{}{
		"success": true,
		"issueId": issue.ID,
	})
	return string(result)
}

// UpdatePatchIssueStatus 更新 Issue 的状态
func (a *App) UpdatePatchIssueStatus(patchID string, issueID string, status string) string {
	validStatuses := map[string]bool{"open": true, "resolved": true, "wontfix": true}
	if !validStatuses[status] {
		return toJSONError("无效的状态")
	}

	fs := a.feedbackStore
	if fs == nil {
		return toJSONError("反馈存储未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := fs.UpdateIssueStatus(ctx, patchID, issueID, status); err != nil {
		return toJSONError(fmt.Sprintf("更新失败: %v", err))
	}

	result, _ := json.Marshal(map[string]interface{}{"success": true})
	return string(result)
}

// getCurrentUser 获取当前用户名（用于评分/Issue 的提交者标识）
func (a *App) getCurrentUser() string {
	if u, err := os.UserHomeDir(); err == nil {
		username := filepath.Base(u)
		if username != "" && username != "." {
			return username
		}
	}
	if u := os.Getenv("USERNAME"); u != "" {
		return u
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "unknown"
}

// shortUUID 生成 8 位短 UUID
func shortUUID() string {
	return strings.ReplaceAll(uuid.New().String()[:8], "-", "")
}

// initFeedbackStore 根据补丁存储配置初始化反馈存储
func (a *App) initFeedbackStore() {
	cfg := a.configMgr.Config.PatchStore
	if !cfg.Enabled || strings.TrimSpace(cfg.RemoteURL) == "" {
		a.feedbackStore = nil
		return
	}

	branch := cfg.Branch
	if branch == "" {
		branch = "main"
	}

	localDir := filepath.Join(filepath.Dir(a.configMgr.Config.Log.Dir), "patchstore")
	a.feedbackStore = patchstore.NewGitFeedbackStore(localDir, cfg.RemoteURL, branch)
	slog.Info("feedback store initialized", "dir", localDir, "remote", cfg.RemoteURL, "branch", branch)
}

func (a *App) SaveSettings(cfg config.AppConfig) string {
	cfg.Terminal = config.NormalizeTerminalConfig(cfg.Terminal)
	cfg.PatchStore.Type = "git"
	cfg.PatchStore.RemoteURL = strings.TrimSpace(cfg.PatchStore.RemoteURL)
	cfg.PatchStore.Branch = strings.TrimSpace(cfg.PatchStore.Branch)
	if cfg.PatchStore.Branch == "" {
		cfg.PatchStore.Branch = "main"
	}

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

	// 如果知识库目录发生变化，重建 Catalog
	newDir := a.resolveKnowledgeBase()
	if err := a.aiService.UpdateCatalog(newDir); err != nil {
		slog.Warn("app failed to rebuild knowledge catalog", "error", err)
	} else if a.aiService.GetCatalog() != nil {
		slog.Info("app knowledge catalog rebuilt", "scenarios", a.aiService.GetCatalog().TotalScenarios())
	}

	// 让补丁同步配置在保存后立即生效
	a.initPatchStore()
	a.initFeedbackStore()

	return ""
}

// SaveTerminalConfig persists terminal appearance and behavior settings without
// rebuilding unrelated services. It is used by the status bar and terminal
// keyboard/mouse shortcuts, which can update the font size frequently.
func (a *App) SaveTerminalConfig(cfg config.TerminalConfig) string {
	a.configMgr.Config.Terminal = config.NormalizeTerminalConfig(cfg)
	if err := a.configMgr.Save(); err != nil {
		return fmt.Sprintf("Failed to save terminal settings: %v", err)
	}
	return ""
}

// GetCommandWhitelist 获取命令白名单配置
func (a *App) GetCommandWhitelist() (*security.WhitelistConfig, error) {
	if a.whitelistMgr == nil {
		return nil, fmt.Errorf("白名单管理器未初始化")
	}
	return a.whitelistMgr.GetConfig(), nil
}

// SaveCommandWhitelist 保存白名单配置
func (a *App) SaveCommandWhitelist(config security.WhitelistConfig) error {
	if a.whitelistMgr == nil {
		return fmt.Errorf("白名单管理器未初始化")
	}
	return a.whitelistMgr.UpdateConfig(&config)
}

// GetPoliciesForIP 查询指定 IP 命中的所有白名单策略
// 用于 UI 反向查询:输入服务器 IP,看它适用哪些策略
func (a *App) GetPoliciesForIP(ip string) ([]security.Policy, error) {
	if a.whitelistMgr == nil {
		return nil, fmt.Errorf("白名单管理器未初始化")
	}
	return a.whitelistMgr.GetPoliciesForIP(ip), nil
}

// GetFileAccessConfig 获取文件访问控制配置
func (a *App) GetFileAccessConfig() (*security.FileAccessConfig, error) {
	if a.fileAccessMgr == nil {
		return nil, fmt.Errorf("文件访问控制管理器未初始化")
	}
	return a.fileAccessMgr.GetConfig(), nil
}

// SaveFileAccessConfig 保存文件访问控制配置
func (a *App) SaveFileAccessConfig(config security.FileAccessConfig) error {
	if a.fileAccessMgr == nil {
		return fmt.Errorf("文件访问控制管理器未初始化")
	}
	return a.fileAccessMgr.UpdateConfig(&config)
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
		slog.Error("failed to reload sessions after import", "error", err)
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

func (a *App) LocalCopy(src, dst string) string {
	srcP := filepath.Clean(strings.TrimSpace(src))
	dstP := filepath.Clean(strings.TrimSpace(dst))
	if srcP == "" || dstP == "" {
		return mustJSON(localFSResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotFound, Message: "路径为空"}})
	}

	srcFile, err := os.Open(srcP)
	if err != nil {
		return mustJSON(localFSResponse{OK: false, Error: toTransferErr(err)})
	}
	defer srcFile.Close()

	fi, err := srcFile.Stat()
	if err != nil {
		return mustJSON(localFSResponse{OK: false, Error: toTransferErr(err)})
	}
	if fi.IsDir() {
		return mustJSON(localFSResponse{OK: false, Error: &filetransfer.TransferError{Code: "IS_DIR", Message: "不支持复制目录"}})
	}

	dstFile, err := os.Create(dstP)
	if err != nil {
		return mustJSON(localFSResponse{OK: false, Error: toTransferErr(err)})
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		os.Remove(dstP)
		return mustJSON(localFSResponse{OK: false, Error: toTransferErr(err)})
	}

	return mustJSON(localFSResponse{OK: true})
}

func (a *App) FTRemoteMkdir(sessionID, remotePath string) string {
	info, err := a.getTransferClientWithRelay(sessionID)
	if err != nil {
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	defer info.closeFn()

	if info.identity == "root-relay" {
		relay := a.getRelayTransport(sessionID)
		if relay == nil {
			return mustJSON(remoteFSResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotFound, Message: "root-relay 传输未就绪"}})
		}
		if err := relay.Mkdir(context.Background(), remotePath); err != nil {
			return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
		}
		return mustJSON(remoteFSResponse{OK: true})
	}

	tr := filetransfer.NewSFTPTransport(info.client)
	if err := tr.Mkdir(context.Background(), remotePath); err != nil {
		// SFTP failed, try shell for root
		if info.identity == "root" {
			shell := a.getShellTransport(sessionID, info.client)
			if shell != nil {
				if shellErr := shell.Mkdir(context.Background(), remotePath); shellErr != nil {
					return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(shellErr)})
				}
				return mustJSON(remoteFSResponse{OK: true})
			}
		}
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	return mustJSON(remoteFSResponse{OK: true})
}

func (a *App) FTRemoteRename(sessionID, oldPath, newPath string) string {
	info, err := a.getTransferClientWithRelay(sessionID)
	if err != nil {
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	defer info.closeFn()

	if info.identity == "root-relay" {
		relay := a.getRelayTransport(sessionID)
		if relay == nil {
			return mustJSON(remoteFSResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotFound, Message: "root-relay 传输未就绪"}})
		}
		if err := relay.Rename(context.Background(), oldPath, newPath); err != nil {
			return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
		}
		return mustJSON(remoteFSResponse{OK: true})
	}

	tr := filetransfer.NewSFTPTransport(info.client)
	if err := tr.Rename(context.Background(), oldPath, newPath); err != nil {
		if info.identity == "root" {
			shell := a.getShellTransport(sessionID, info.client)
			if shell != nil {
				if shellErr := shell.Rename(context.Background(), oldPath, newPath); shellErr != nil {
					return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(shellErr)})
				}
				return mustJSON(remoteFSResponse{OK: true})
			}
		}
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	return mustJSON(remoteFSResponse{OK: true})
}

func (a *App) FTRemoteRemove(sessionID, remotePath string) string {
	info, err := a.getTransferClientWithRelay(sessionID)
	if err != nil {
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	defer info.closeFn()

	if info.identity == "root-relay" {
		relay := a.getRelayTransport(sessionID)
		if relay == nil {
			return mustJSON(remoteFSResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotFound, Message: "root-relay 传输未就绪"}})
		}
		if err := relay.Remove(context.Background(), remotePath, true); err != nil {
			return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
		}
		return mustJSON(remoteFSResponse{OK: true})
	}

	tr := filetransfer.NewSFTPTransport(info.client)
	if err := tr.Remove(context.Background(), remotePath, true); err != nil {
		if info.identity == "root" {
			shell := a.getShellTransport(sessionID, info.client)
			if shell != nil {
				if shellErr := shell.Remove(context.Background(), remotePath, true); shellErr != nil {
					return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(shellErr)})
				}
				return mustJSON(remoteFSResponse{OK: true})
			}
		}
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	return mustJSON(remoteFSResponse{OK: true})
}

func (a *App) FTRemoteReadFile(sessionID, remotePath string, maxBytes int64) string {
	info, err := a.getTransferClientWithRelay(sessionID)
	if err != nil {
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	defer info.closeFn()

	if info.identity == "root-relay" {
		relay := a.getRelayTransport(sessionID)
		if relay == nil {
			return mustJSON(remoteFSResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotFound, Message: "root-relay 传输未就绪"}})
		}
		b, err := relay.ReadFile(context.Background(), remotePath, maxBytes)
		if err != nil {
			return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
		}
		return mustJSON(remoteFSResponse{OK: true, Content: string(b)})
	}

	if strings.HasPrefix(a.getTransferMode(sessionID), "scp") {
		return mustJSON(remoteFSResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotSupported, Message: "SCP 模式不支持远端文件直读"}})
	}

	tr := filetransfer.NewSFTPTransport(info.client)
	b, err := tr.ReadFile(context.Background(), remotePath, maxBytes)
	if err != nil {
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	return mustJSON(remoteFSResponse{OK: true, Content: string(b)})
}

func (a *App) FTRemoteWriteFile(sessionID, remotePath string, content string) string {
	info, err := a.getTransferClientWithRelay(sessionID)
	if err != nil {
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	defer info.closeFn()

	if info.identity == "root-relay" {
		relay := a.getRelayTransport(sessionID)
		if relay == nil {
			return mustJSON(remoteFSResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotFound, Message: "root-relay 传输未就绪"}})
		}
		if err := relay.WriteFile(context.Background(), remotePath, []byte(content)); err != nil {
			return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
		}
		return mustJSON(remoteFSResponse{OK: true})
	}

	if strings.HasPrefix(a.getTransferMode(sessionID), "scp") {
		return mustJSON(remoteFSResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotSupported, Message: "SCP 模式不支持远端文件直写"}})
	}

	tr := filetransfer.NewSFTPTransport(info.client)
	if err := tr.WriteFile(context.Background(), remotePath, []byte(content)); err != nil {
		return mustJSON(remoteFSResponse{OK: false, Error: toTransferErr(err)})
	}
	return mustJSON(remoteFSResponse{OK: true})
}

func (a *App) getPreferredTransferSSHClient(sessionID string) (*ssh.Client, func(), string, error) {
	info, err := a.getTransferClientWithRelay(sessionID)
	if err != nil {
		return nil, nil, "", err
	}
	return info.client, info.closeFn, info.identity, nil
}

// transferClientInfo holds the SSH client and metadata for file transfer operations.
type transferClientInfo struct {
	client     *ssh.Client
	closeFn    func()
	identity   string // "login" | "root" | "root-relay"
	viaBastion bool   // true when connection goes through a bastion host
}

func (a *App) getTransferClientWithRelay(sessionID string) (transferClientInfo, error) {
	sess, ok := a.sessionMgr.Get(sessionID)
	if !ok || sess.Client == nil || sess.Client.SSHClient() == nil {
		return transferClientInfo{}, &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotFound, Message: "会话不存在"}
	}

	base := sess.Client.SSHClient()
	baseClose := func() {}
	identity := "login"

	cfg, ok := a.getConfig(sessionID)
	if !ok {
		slog.Debug("ft session has no config, using login identity", "session", sessionID[:8])
		return transferClientInfo{client: base, closeFn: baseClose, identity: identity}, nil
	}

	if cfg.RootPassword == "" {
		slog.Debug("ft session has no root password, using login identity", "session", sessionID[:8], "user", cfg.User)
		return transferClientInfo{client: base, closeFn: baseClose, identity: identity}, nil
	}
	if strings.EqualFold(cfg.User, "root") {
		slog.Debug("ft session already logged in as root", "session", sessionID[:8])
		return transferClientInfo{client: base, closeFn: baseClose, identity: "root"}, nil
	}

	// Through bastion: root direct SSH is typically blocked (bastion restricts root login).
	// Skip the failed attempt and go directly to root-relay mode.
	if cfg.Bastion != nil {
		slog.Debug("ft session via bastion, skipping root direct, using root-relay", "session", sessionID[:8])
		return transferClientInfo{
			client:     base,
			closeFn:    baseClose,
			identity:   "root-relay",
			viaBastion: true,
		}, nil
	}

	// Try root SSH direct connection
	slog.Debug("ft session trying root direct connection", "session", sessionID[:8], "host", cfg.Host)
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
	if err == nil && rootClient != nil && rootClient.SSHClient() != nil {
		slog.Debug("ft session root direct connection succeeded", "session", sessionID[:8])
		return transferClientInfo{
			client:   rootClient.SSHClient(),
			closeFn:  func() { _ = rootClient.Close() },
			identity: "root",
		}, nil
	}

	// Root SSH failed but we have root password → use relay mode
	slog.Debug("ft session root direct failed, falling back to root-relay", "session", sessionID[:8], "error", err)
	return transferClientInfo{
		client:     base,
		closeFn:    baseClose,
		identity:   "root-relay",
		viaBastion: cfg.Bastion != nil,
	}, nil
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
	info, err := a.getTransferClientWithRelay(sessionID)
	if err != nil {
		return mustJSON(ftResponse{OK: false, Error: toTransferErr(err)})
	}
	defer info.closeFn()

	slog.Debug("ftList session listing directory", "session", sessionID[:8], "path", remotePath, "identity", info.identity)

	var entries []filetransfer.Entry
	if info.identity == "root-relay" {
		relay := a.getRelayTransport(sessionID)
		if relay == nil {
			slog.Debug("ftList session root-relay transport not ready", "session", sessionID[:8])
			return mustJSON(remoteFSResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotFound, Message: "root-relay 传输未就绪"}})
		}
		entries, err = relay.List(context.Background(), remotePath)
	} else {
		tr := filetransfer.NewSFTPTransport(info.client)
		entries, err = tr.List(context.Background(), remotePath)
		// SFTP failed but we have root access — fall back to shell commands
		if err != nil && info.identity == "root" {
			slog.Debug("ftList session SFTP list failed, falling back to shell", "session", sessionID[:8], "error", err)
			shell := a.getShellTransport(sessionID, info.client)
			entries, err = shell.List(context.Background(), remotePath)
		}
	}
	if err != nil {
		slog.Error("ftList session list directory failed", "session", sessionID[:8], "error", err)
		te := toTransferErr(err)
		return mustJSON(ftResponse{OK: false, Error: te})
	}
	slog.Debug("ftList session list directory succeeded", "session", sessionID[:8], "entries", len(entries))
	return mustJSON(ftResponse{OK: true, Entries: entries})
}

func (a *App) FTStat(sessionID, remotePath string) string {
	info, err := a.getTransferClientWithRelay(sessionID)
	if err != nil {
		return mustJSON(ftResponse{OK: false, Error: toTransferErr(err)})
	}
	defer info.closeFn()

	var entry filetransfer.Entry
	if info.identity == "root-relay" {
		relay := a.getRelayTransport(sessionID)
		if relay == nil {
			return mustJSON(remoteFSResponse{OK: false, Error: &filetransfer.TransferError{Code: filetransfer.ErrorCodeNotFound, Message: "root-relay 传输未就绪"}})
		}
		entry, err = relay.Stat(context.Background(), remotePath)
	} else {
		tr := filetransfer.NewSFTPTransport(info.client)
		entry, err = tr.Stat(context.Background(), remotePath)
		// SFTP failed but we have root access — fall back to shell commands
		if err != nil && info.identity == "root" {
			shell := a.getShellTransport(sessionID, info.client)
			entry, err = shell.Stat(context.Background(), remotePath)
		}
	}
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
	info, err := a.getTransferClientWithRelay(sessionID)
	if err != nil {
		slog.Error("ftCheck session failed to get transfer client", "session", sessionID[:8], "error", err)
		return mustJSON(ftResponse{OK: false, Error: toTransferErr(err)})
	}
	defer info.closeFn()

	slog.Debug("ftCheck session identity", "session", sessionID[:8], "identity", info.identity)

	// Root relay mode: SFTP works for relay dir operations, so always available
	if info.identity == "root-relay" {
		// Through bastion: SFTP (not enabled on target) and SCP (hangs via bastion) are both unusable.
		// Skip them and go directly to base64 transfer via su session.
		if info.viaBastion {
			slog.Debug("ftCheck session detected bastion, skipping SFTP/SCP, using base64 direct", "session", sessionID[:8])
			return mustJSON(ftResponse{OK: true, Message: "su-relay(root-relay)"})
		}
		sftpTr := filetransfer.NewSFTPTransport(info.client)
		_, _, sftpErr := sftpTr.Check(context.Background())
		if sftpErr == nil {
			slog.Debug("ftCheck session resolved to sftp(root-relay)", "session", sessionID[:8])
			return mustJSON(ftResponse{OK: true, Message: "sftp(root-relay)"})
		}
		// SFTP not available, check SCP as fallback
		slog.Debug("ftCheck sftp unavailable, trying scp fallback", "session", sessionID[:8], "error", sftpErr)
		scpTr := filetransfer.NewSCPTransport(info.client)
		scpOk, _, scpErr := scpTr.Check(context.Background())
		if scpErr == nil && scpOk {
			slog.Debug("ftCheck resolved to scp(root-relay)", "session", sessionID[:8])
			return mustJSON(ftResponse{OK: true, Message: "scp(root-relay)"})
		}
		// Even SFTP and SCP not available, but relay can still work via su
		slog.Debug("ftCheck sftp/scp unavailable, using su-relay mode", "session", sessionID[:8])
		return mustJSON(ftResponse{OK: true, Message: "su-relay(root-relay)"})
	}

	c := info.client
	sftpTr := filetransfer.NewSFTPTransport(c)
	_, _, sftpErr := sftpTr.Check(context.Background())
	if sftpErr == nil {
		if info.identity == "root" {
			slog.Debug("ftCheck resolved to sftp(root)", "session", sessionID[:8])
			return mustJSON(ftResponse{OK: true, Message: "sftp(root)"})
		}
		slog.Debug("ftCheck resolved to sftp(login)", "session", sessionID[:8])
		return mustJSON(ftResponse{OK: true, Message: "sftp(login)"})
	}

	te := toTransferErr(sftpErr)
	slog.Debug("ftCheck sftp check failed", "session", sessionID[:8], "code", te.Code, "error", sftpErr)
	if te != nil && (te.Code == filetransfer.ErrorCodeSFTPNotSupported || te.Code == filetransfer.ErrorCodeUnknown || te.Code == filetransfer.ErrorCodeNetwork) {
		// Root identity can manage files via shell commands even without SFTP
		if info.identity == "root" {
			slog.Debug("ftCheck root identity, sftp unavailable but shell available", "session", sessionID[:8])
			return mustJSON(ftResponse{OK: true, Message: "sftp(root)"})
		}
		scpTr := filetransfer.NewSCPTransport(c)
		ok, _, err := scpTr.Check(context.Background())
		if err != nil {
			return mustJSON(ftResponse{OK: false, Error: toTransferErr(err)})
		}
		if ok {
			if info.identity == "root" {
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
	slog.Info("ft transfer started", "op", op, "session", sessionID[:8], "local", localPath, "remote", remotePath)
	info, err := a.getTransferClientWithRelay(sessionID)
	if err != nil {
		slog.Error("ft failed to get transfer client", "error", err)
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

		// Re-resolve client inside goroutine (session may have changed)
		taskInfo, err := a.getTransferClientWithRelay(sessionID)
		if err != nil {
			slog.Error("ft task failed to re-resolve transfer client", "task", taskID[:8], "error", err)
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
		defer taskInfo.closeFn()

		progressFn := func(p filetransfer.Progress) {
			if a.ctx == nil {
				return
			}
			payload := map[string]any{
				"taskId":    taskID,
				"sessionId": sessionID,
			}
			if p.Step != "" {
				// Step-only notification: don't overwrite byte progress
				payload["step"] = p.Step
			} else {
				payload["bytesDone"] = p.BytesDone
				payload["bytesTotal"] = p.BytesTotal
				payload["speedBps"] = p.SpeedBps
			}
			runtime.EventsEmit(a.ctx, "file-transfer-progress", payload)
		}

		var (
			res   filetransfer.TransferResult
			opErr error
		)

		// Root relay mode: use RootRelayTransport for actual transfer
		if taskInfo.identity == "root-relay" {
			slog.Debug("ft task using root-relay mode", "task", taskID[:8], "op", op)
			relay := a.getRelayTransport(sessionID)
			if relay == nil {
				slog.Warn("ft task root-relay transport not ready", "task", taskID[:8])
				if a.ctx != nil {
					runtime.EventsEmit(a.ctx, "file-transfer-done", map[string]any{
						"taskId":    taskID,
						"sessionId": sessionID,
						"ok":        false,
						"code":      filetransfer.ErrorCodeNotFound,
						"message":   "root-relay 传输未就绪",
					})
				}
				return
			}
			if op == "upload" {
				res, opErr = relay.Upload(ctx, localPath, remotePath, progressFn)
			} else {
				res, opErr = relay.Download(ctx, remotePath, localPath, progressFn)
			}

			usedTransport := "sftp(root-relay)"
			if a.ctx == nil {
				return
			}
			if opErr != nil {
				slog.Error("ft task root-relay transfer failed", "task", taskID[:8], "error", opErr)
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
			return
		}

		// Normal SFTP/SCP path
		c := taskInfo.client
		identity := taskInfo.identity

		slog.Debug("ft task using normal mode", "task", taskID[:8], "identity", identity, "op", op)
		sftpTr := filetransfer.NewSFTPTransport(c)
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
			slog.Debug("ft task sftp failed, trying scp fallback", "task", taskID[:8], "code", te.Code)
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
			slog.Error("ft task transfer failed", "task", taskID[:8], "transport", usedTransport, "error", opErr)
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
		slog.Info("ft task transfer completed", "task", taskID[:8], "transport", usedTransport, "bytes", res.Bytes)
	}()

	// Suppress unused warning
	_ = info
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
	return ""
}

// LoadQuickCommands returns the list of quick commands from config
func (a *App) LoadQuickCommands() []config.QuickCommand {
	return a.configMgr.Config.QuickCommands
}

// SaveQuickCommands updates and saves quick commands
func (a *App) SaveQuickCommands(commands []config.QuickCommand) string {
	a.configMgr.SetQuickCommands(commands)
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
		slog.Error("failed to generate conclusion", "error", err)
		return fmt.Sprintf("Error generating conclusion: %v", err)
	}
	return conclusion
}

// StreamConclusion 流式生成结论，通过 Wails 事件逐步推送给前端
func (a *App) StreamConclusion(contextStr string, rootCause string) string {
	ctx := context.Background()

	onToken := func(token string) {
		runtime.EventsEmit(a.ctx, "conclusion:token", map[string]string{
			"token": token,
		})
	}

	conclusion, err := a.aiService.GenerateConclusionStream(ctx, contextStr, rootCause, onToken)
	if err != nil {
		slog.Error("failed to stream conclusion", "error", err)
		runtime.EventsEmit(a.ctx, "conclusion:error", map[string]string{
			"error": err.Error(),
		})
		return fmt.Sprintf("Error generating conclusion: %v", err)
	}

	// Signal completion
	runtime.EventsEmit(a.ctx, "conclusion:done", map[string]string{
		"conclusion": conclusion,
	})

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
	config, ok := a.getConfig(sessionID)
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

func (a *App) CreateSavedFolder(name string) string {
	if err := a.savedSessionMgr.CreateFolder(name); err != nil {
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
		slog.Error("getCompletions error", "error", err)
		return "[]"
	}

	// Convert to JSON
	data, err := json.Marshal(resp)
	if err != nil {
		slog.Error("getCompletions json error", "error", err)
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
	config, ok := a.getConfig(sessionID)
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
		a.setConfig(sessionID, config)
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

// CreateScript 手动创建空脚本
func (a *App) CreateScript(name, description string) (*script.Script, error) {
	return a.scriptMgr.CreateScript(name, description)
}

// ReplayScript 回放脚本
func (a *App) ReplayScript(scriptID, sessionID string) error {
	return a.scriptMgr.ReplayScript(scriptID, sessionID)
}

// ReplayScriptWithVars 带变量值的回放脚本
func (a *App) ReplayScriptWithVars(scriptID, sessionID string, varValues map[string]string) error {
	return a.scriptMgr.ReplayScriptWithVars(scriptID, sessionID, varValues)
}

// ExportScript 导出脚本为Shell脚本（通过系统文件保存对话框）
func (a *App) ExportScript(scriptID string) error {
	content, err := a.scriptMgr.ExportScript(scriptID)
	if err != nil {
		return err
	}

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "script.sh",
		Filters: []runtime.FileFilter{
			{DisplayName: "Shell 脚本", Pattern: "*.sh"},
			{DisplayName: "所有文件", Pattern: "*.*"},
		},
	})
	if err != nil || path == "" {
		return err
	}

	return os.WriteFile(path, []byte(content), 0644)
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

// readTerminalChunk aggregates data from a terminal read loop.
// If the initial read (n bytes) filled buf completely, it continues reading
// until a short read, error, or the 256KB size limit. This reduces Wails event
// count during high-throughput scenarios (e.g. cat large file) while keeping
// interactive latency at zero for partial reads.
func readTerminalChunk(r io.Reader, buf []byte, n int) string {
	if n < len(buf) {
		return string(buf[:n])
	}
	// Buffer filled: high-throughput scenario, aggregate reads
	var agg bytes.Buffer
	agg.Write(buf[:n])
	for agg.Len() < 262144 {
		n2, err2 := r.Read(buf)
		if n2 > 0 {
			agg.Write(buf[:n2])
		}
		if n2 < len(buf) || err2 != nil {
			break
		}
	}
	return agg.String()
}

// initPatchStore 根据配置初始化补丁存储
func (a *App) initPatchStore() {
	cfg := a.configMgr.Config.PatchStore
	remoteURL := strings.TrimSpace(cfg.RemoteURL)
	a.refreshPatchSyncStatusConfig()
	if !cfg.Enabled || remoteURL == "" {
		a.setPatchStore(nil)
		a.updatePatchSyncStatus(func(status *PatchSyncStatus) {
			status.LastSyncMessage = "补丁同步未启用"
		})
		return
	}

	localDir := filepath.Join(filepath.Dir(a.configMgr.Config.Log.Dir), "patchstore")
	branch := strings.TrimSpace(cfg.Branch)
	if branch == "" {
		branch = "main"
	}

	store := patchstore.NewGitPatchStore(
		remoteURL,
		localDir,
		branch,
		"",
		"",
	)
	a.setPatchStore(store)

	go func(store patchstore.PatchStore) {
		if err := a.runPatchSync(store); err != nil {
			slog.Error("initial patch sync failed", "error", err)
		}
	}(store)
}

func (a *App) runPatchSync(store patchstore.PatchStore) error {
	if !a.patchSyncing.CompareAndSwap(false, true) {
		return fmt.Errorf("patch sync already running")
	}
	defer a.patchSyncing.Store(false)

	a.refreshPatchSyncStatusConfig()
	a.updatePatchSyncStatus(func(status *PatchSyncStatus) {
		status.Running = true
		status.LastSyncMessage = "正在同步..."
	})

	err := a.syncPatches(store)
	now := time.Now().Format("2006-01-02 15:04:05")
	if err != nil {
		a.refreshPatchSyncStatusConfig()
		a.updatePatchSyncStatus(func(status *PatchSyncStatus) {
			status.Running = false
			status.LastSyncAt = now
			status.LastSyncSuccess = false
			status.LastSyncMessage = err.Error()
		})
		return err
	}

	a.refreshPatchSyncStatusConfig()
	a.updatePatchSyncStatus(func(status *PatchSyncStatus) {
		status.Running = false
		status.LastSyncAt = now
		status.LastSyncSuccess = true
		if status.PendingCount > 0 {
			status.LastSyncMessage = fmt.Sprintf("同步完成，仍有 %d 条待重试补丁", status.PendingCount)
		} else {
			status.LastSyncMessage = "同步完成"
		}
	})
	return nil
}

// syncPatches 从共享存储同步补丁并重建本地知识文件
func (a *App) syncPatches(store patchstore.PatchStore) error {
	if store == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	remotePatches, err := store.DownloadAll(ctx)
	if err != nil {
		return fmt.Errorf("download patches: %w", err)
	}

	pendingStore := a.newPendingPatchStore()
	pendingPatches, err := pendingStore.List()
	if err != nil {
		return fmt.Errorf("list pending patches: %w", err)
	}

	patches := mergePatches(remotePatches, pendingPatches)
	if len(patches) == 0 {
		return nil
	}

	knowledgeDir := a.resolveKnowledgeBase()
	count, err := knowledge.RebuildFromPatches(knowledgeDir, patches)
	if err != nil {
		return fmt.Errorf("rebuild from patches: %w", err)
	}

	if count > 0 {
		if err := a.aiService.UpdateCatalog(knowledgeDir); err != nil {
			slog.Error("failed to update catalog after patch sync", "error", err)
		}
		slog.Info("patch sync completed", "patches", len(patches), "files_rebuilt", count)
	}

	if err := a.flushPendingPatches(store, pendingStore, remotePatches, pendingPatches); err != nil {
		slog.Warn("flush pending patches failed", "error", err)
	}

	return nil
}

// uploadPatch 异步上传补丁到共享存储
func (a *App) uploadPatch(store patchstore.PatchStore, pendingStore *patchstore.PendingStore, patch patchstore.Patch) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := store.Upload(ctx, patch); err != nil {
		slog.Error("failed to upload patch", "error", err, "service", patch.Service, "patch_id", patch.ID)
		return
	}

	if err := pendingStore.Delete(patch); err != nil {
		slog.Warn("failed to delete pending patch after upload", "error", err, "patch_id", patch.ID)
	}

	slog.Info("patch uploaded", "id", patch.ID, "service", patch.Service, "module", patch.Module)
}

func (a *App) flushPendingPatches(store patchstore.PatchStore, pendingStore *patchstore.PendingStore, remotePatches []patchstore.Patch, pendingPatches []patchstore.Patch) error {
	remoteByID := make(map[string]struct{}, len(remotePatches))
	for _, patch := range remotePatches {
		remoteByID[patch.ID] = struct{}{}
	}

	sort.Slice(pendingPatches, func(i, j int) bool {
		return pendingPatches[i].Timestamp.Before(pendingPatches[j].Timestamp)
	})

	for _, patch := range pendingPatches {
		if _, exists := remoteByID[patch.ID]; exists {
			if err := pendingStore.Delete(patch); err != nil {
				return err
			}
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := store.Upload(ctx, patch)
		cancel()
		if err != nil {
			return err
		}
		if err := pendingStore.Delete(patch); err != nil {
			return err
		}
	}

	return nil
}

func mergePatches(remotePatches []patchstore.Patch, pendingPatches []patchstore.Patch) []patchstore.Patch {
	merged := make([]patchstore.Patch, 0, len(remotePatches)+len(pendingPatches))
	seen := make(map[string]struct{}, len(remotePatches)+len(pendingPatches))

	appendUnique := func(patches []patchstore.Patch) {
		for _, patch := range patches {
			if _, exists := seen[patch.ID]; exists {
				continue
			}
			seen[patch.ID] = struct{}{}
			merged = append(merged, patch)
		}
	}

	appendUnique(remotePatches)
	appendUnique(pendingPatches)
	return merged
}

// migrateScriptsFromLegacyPath 将旧路径下的脚本数据迁移到新的独立目录
// 旧路径: {legacyBase}/script_*.json (脚本数据) + {legacyBase}/script/recording_*.json (冗余录制数据)
// 新路径: {newBase}/script_*.json
func migrateScriptsFromLegacyPath(legacyBase, newBase string) {
	migrateScriptExtFiles(legacyBase, newBase)
	cleanupLegacyRecordingFiles(legacyBase)
}

func migrateScriptExtFiles(srcDir, dstDir string) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "script_") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		src := filepath.Join(srcDir, entry.Name())
		dst := filepath.Join(dstDir, entry.Name())

		if _, err := os.Stat(dst); err == nil {
			continue
		}

		os.MkdirAll(dstDir, 0755)
		if err := os.Rename(src, dst); err != nil {
			data, err := os.ReadFile(src)
			if err != nil {
				continue
			}
			os.WriteFile(dst, data, 0644)
			os.Remove(src)
		}
	}
}

// cleanupRedundantRecordingFiles 清理脚本目录下冗余的 scripts/script/ 子目录
// 这些 recording_*.json 已不再需要，所有数据已在 script_*.json 中
func cleanupRedundantRecordingFiles(scriptsDir string) {
	recDir := filepath.Join(scriptsDir, "script")
	entries, err := os.ReadDir(recDir)
	if err != nil {
		return // 目录不存在，无需清理
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			os.Remove(filepath.Join(recDir, entry.Name()))
		}
	}
	os.Remove(recDir) // 目录为空则删除
}

// cleanupLegacyRecordingFiles 清理旧路径下冗余的 recording_*.json
// 旧路径: {legacyBase}/script/recording_*.json
func cleanupLegacyRecordingFiles(legacyBase string) {
	recDir := filepath.Join(legacyBase, "script")
	entries, err := os.ReadDir(recDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			os.Remove(filepath.Join(recDir, entry.Name()))
		}
	}
	os.Remove(recDir)
}
