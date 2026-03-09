# OpsCopilot 架构解耦文档

## 概述

本次重构实现了 Shell 软件和智能体（Agent）的解耦，为未来的独立部署和多智能体协作打下基础。

## 新增包结构

```
pkg/
├── shell/          # Shell 组件（新增）
│   ├── types.go    # Shell 接口定义
│   ├── manager.go  # Shell 管理器实现
│   └── mock.go     # Mock 实现（用于测试）
├── agent/          # Agent 组件（新增）
│   ├── types.go    # Agent 接口定义
│   ├── service.go  # Agent 服务实现
│   └── mock.go     # Mock 实现（用于测试）
└── bridge/         # 桥接层（新增）
    ├── types.go    # 事件类型和接口定义
    ├── bus.go      # 事件总线实现
    ├── command.go  # 命令桥接实现
    ├── state.go    # 状态桥接实现
    └── bridge.go   # 组合桥接器
```

## 核心接口

### Shell 接口 (pkg/shell/types.go)

```go
// Session 表示一个终端会话
type Session interface {
    ID() string
    Info() SessionInfo
    Send(data string) error
    Resize(cols, rows int) error
    Close() error
    Stdin() io.Writer
}

// Manager 会话管理器接口
type Manager interface {
    Connect(ctx context.Context, config ConnectConfig) (Session, error)
    Get(id string) (Session, bool)
    List() []Session
    Disconnect(id string) error
    DisconnectAll() error
    Broadcast(ids []string, data string) error
}
```

### Agent 接口 (pkg/agent/types.go)

```go
// Recorder 录制器接口
type Recorder interface {
    Start(recType RecordingType, sessionID, host, user string) (*RecordingSession, error)
    RecordInput(sessionID string, command string) error
    Stop() (*RecordingSession, error)
    GetStatus() RecorderStatus
    GetCurrentSession() *RecordingSession
}

// AgentService 智能体服务接口
type AgentService interface {
    ProcessUserInput(ctx context.Context, sessionID string, input string) (*AIResponse, error)
    GetStatus() ServiceStatus
    Start(ctx context.Context) error
    Stop() error
}
```

### 桥接接口 (pkg/bridge/types.go)

```go
// Bus 事件总线接口
type Bus interface {
    Publish(ctx context.Context, event Event) error
    Subscribe(eventType string, handler Handler) (string, error)
    Unsubscribe(subscriptionID string) error
    SubscribeAll(handler Handler) (string, error)
    Close() error
}

// CommandBridge 命令桥接接口
type CommandBridge interface {
    SendCommand(ctx context.Context, sessionID string, command string) error
    SendCommands(ctx context.Context, sessionID string, commands []string) error
    RegisterHandler(sessionID string, handler CommandHandler) error
    UnregisterHandler(sessionID string) error
}

// StateBridge 状态桥接接口
type StateBridge interface {
    GetSessionInfo(sessionID string) (SessionState, error)
    UpdateSessionInfo(sessionID string, state SessionState) error
    GetAllSessions() map[string]SessionState
    Watch(ctx context.Context) <-chan StateChange
}
```

## 事件类型

```go
const (
    // Shell 事件
    EventShellConnect      = "shell:connect"
    EventShellDisconnect   = "shell:disconnect"
    EventShellData         = "shell:data"
    EventShellInput        = "shell:input"
    EventShellOutput       = "shell:output"

    // Agent 事件
    EventAgentCommand      = "agent:command"
    EventAgentStatus       = "agent:status"
    EventAgentSuggestion   = "agent:suggestion"

    // 录制事件
    EventRecordingStart    = "recording:start"
    EventRecordingStop     = "recording:stop"
    EventRecordingInput    = "recording:input"
    EventRecordingOutput   = "recording:output"
)
```

## 使用示例

### 创建桥接器并连接 Shell 和 Agent

```go
// 创建桥接器
bus := bridge.NewBridge()

// 创建 Shell 管理器
shellMgr := shell.NewManager(
    shell.WithBus(bus),
)

// 创建 Agent 服务
agentSvc := agent.NewService(
    agent.WithBus(bus),
    agent.WithRecorder(recorder),
)

// 启动 Agent 服务
agentSvc.Start(context.Background())
```

### 订阅事件

```go
// 订阅 Shell 连接事件
bus.Subscribe(bridge.EventShellConnect, func(ctx context.Context, event bridge.Event) error {
    fmt.Printf("Session connected: %s\n", event.SessionID)
    return nil
})
```

### 执行命令

```go
// 注册命令处理器
bus.RegisterHandler("session-1", func(ctx context.Context, command string) error {
    // 处理命令
    return nil
})

// 发送命令
bus.SendCommand(ctx, "session-1", "ls -la")
```

## 测试覆盖

所有新组件都有完整的单元测试：

- `pkg/bridge/bus_test.go` - 事件总线测试
- `pkg/bridge/command_test.go` - 命令桥接测试
- `pkg/bridge/state_test.go` - 状态桥接测试
- `pkg/shell/mock_test.go` - Shell Mock 测试
- `pkg/agent/mock_test.go` - Agent Mock 测试

## 下一步

1. **阶段 6**: 统一录制器 - 合并 `session_recorder` 和 `recorder` 包
2. **阶段 7**: 拆分 App 结构体 - 创建 `shell_app.go` 和 `agent_app.go`
3. **阶段 8**: 前端适配 - 更新前端 API 调用
4. **阶段 9**: 文档和清理

## 收益

- **架构清晰**: Shell 和 Agent 完全解耦
- **易于测试**: 通过接口抽象，易于 mock
- **可扩展性强**: 支持智能体独立部署
- **代码质量**: 消除冗余，统一风格
