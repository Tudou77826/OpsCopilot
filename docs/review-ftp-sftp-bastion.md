---
service: OpsCopilot
module: filetransfer
type: sop
---

# FTP/SFTP 文件传输 & 跳板机 Root 提权获取文件 — Code Review

> 审查范围：`pkg/filetransfer/`、`pkg/sshclient/`、`pkg/mcpserver/tools.go`、`pkg/secretstore/`、`app.go` 中文件传输相关逻辑

---

## 关键词

Root 提权, Keyring, 密码存储, 安全风险, 密码明文, FTP, su -, ssh, bastion, 静默降级, SFTP, SCP, 跳板机

## 一、整体架构

```
┌──────────────────────────────────────────────────────────────┐
│                      前端 (FilesPanel.tsx)                    │
│   FTList / FTStat / FTUpload / FTDownload / FTRemoteReadFile │
└──────────────┬───────────────────────────┬───────────────────┘
               │ Wails Bindings            │
               ▼                           ▼
┌──────────────────────────┐  ┌────────────────────────────────┐
│     app.go               │  │  mcpserver/tools.go            │
│  FT* 系列方法            │  │  toolServerConnect / toolSSHExec│
│  getPreferredTransferSSH │  │  (MCP Server 入口)             │
│  Client()                │  │                                │
└──────────┬───────────────┘  └───────────────┬────────────────┘
           │                                  │
           ▼                                  ▼
┌──────────────────────────┐  ┌────────────────────────────────┐
│  sshclient/client.go     │  │  sshclient/client.go           │
│  NewClient() — 建立连接  │  │  StartShellWithSudo()          │
│  (直连 or 跳板机)        │  │  SudoHandler — 自动 su -       │
└──────────┬───────────────┘  └────────────────────────────────┘
           │
           ▼
┌──────────────────────────────────────────────────────────────┐
│                    filetransfer/                              │
│  ┌─────────────────┐    ┌─────────────────┐                  │
│  │ SFTPTransport   │    │ SCPTransport    │  (自动降级)      │
│  │ (优先)          │    │ (SFTP不可用时)  │                  │
│  └─────────────────┘    └─────────────────┘                  │
│  types.go — Entry/Progress/TransferResult/TransferError      │
│  copy.go  — copyWithProgress (进度回调)                       │
└──────────────────────────────────────────────────────────────┘
```

---

## 二、核心数据结构

### 2.1 连接配置（两层，分属不同包）

**app 层 — `app.go:326`**

```go
type ConnectConfig struct {
    Name         string         `json:"name"`
    Host         string         `json:"host"`
    Port         int            `json:"port"`
    User         string         `json:"user"`
    Password     string         `json:"password"`
    RootPassword string         `json:"rootPassword"` // ← root 账密
    Bastion      *ConnectConfig `json:"bastion"`      // ← 跳板机（递归结构）
    Group        string         `json:"group"`
}
```

**SSH 层 — `pkg/sshclient/client.go:14`**

```go
type ConnectConfig struct {
    Name         string
    Host         string
    Port         int
    User         string
    Password     string
    RootPassword string         // 用于 StartShellWithSudo
    Bastion      *ConnectConfig // 跳板机
    Group        string
}
```

> 两层结构字段完全一致，`app.Connect` 时手动逐字段拷贝到 `sshclient.ConnectConfig`。

### 2.2 文件传输类型 — `pkg/filetransfer/types.go`

```go
type TransferError struct {
    Code    ErrorCode  // UNKNOWN / SFTP_NOT_SUPPORTED / PERMISSION_DENIED / NOT_FOUND / AUTH_FAILED / NETWORK
    Message string
}
type Entry struct { Path, Name, IsDir, Size, Mode, ModTime }
type Progress struct { BytesDone, BytesTotal, SpeedBps int64 }
type TransferResult struct { Bytes int64 }
```

### 2.3 MCP Connection — `pkg/mcpserver/server.go:50`

```go
type Connection struct {
    Name         string
    Host         string            // IP，用于白名单匹配
    Client       *sshclient.Client
    RootPassword string            // 用于 sudo 提权
    ConnectedAt  time.Time
    LastActive   atomic.Int64
}
```

---

## 三、跳板机 + Root 提权文件获取流程（重点）

### 3.1 完整流程图

```
用户操作                        系统行为
───────                        ────────
1. 填写连接配置
   ├─ 普通用户 user/password
   ├─ Root 密码 rootPassword
   └─ 跳板机 bastion.{host,port,user,password}

2. Connect(config)
   │
   ├─ 2a. 保存密码到 Keyring
   │      ├─ 主机密码: service="OpsCopilot-SSH", key=host:user
   │      └─ 跳板机密码: service="OpsCopilot-SSH", key=bastionHost:bastionUser
   │
   ├─ 2b. 递归建立 SSH 连接
   │      sshclient.NewClient(config)
   │      ├─ 如果有 Bastion:
   │      │   ├─ NewClient(config.Bastion)         ← 先连跳板机
   │      │   │   └─ ssh.Dial("tcp", bastionAddr)  ← 跳板机直连
   │      │   ├─ bastionClient.Dial("tcp", target)  ← TCP 转发
   │      │   │   └─ 失败? dialViaConsole()         ← Netcat/Bash/Python 降级
   │      │   └─ ssh.NewClientConn(conn)            ← 在隧道上建 SSH
   │      └─ 无 Bastion: ssh.Dial("tcp", addr)     ← 直连
   │
   └─ 2c. 启动 Shell（自动 su -）
          ├─ 有 RootPassword → StartShellWithSudo()
          │   ├─ StartShell() — 开 PTY shell
          │   ├─ 创建 SudoHandler{RootPassword, Stdin}
          │   ├─ AutoSudoReader 包装 stdout
          │   └─ go func() { sleep 500ms; stdin.Write("su -\n") }()
          └─ 无 RootPassword → StartShell()

3. 文件操作（FTList / FTDownload / FTUpload 等）
   │
   └─ getPreferredTransferSSHClient(sessionID)   ← 关键决策
      │
      ├─ 3a. 获取当前会话的 SSH Client (普通用户身份)
      │
      ├─ 3b. 检查 RootPassword
      │      ├─ RootPassword == "" → 直接用 login 用户
      │      └─ User == "root"     → 直接用 login (已是 root)
      │
      ├─ 3c. 如果有 RootPassword 且 User ≠ root:
      │      构造新的 root SSH 连接:
      │      rootCfg = {
      │          Host:     cfg.Host,      ← 同一台目标机
      │          User:     "root",
      │          Password: cfg.RootPassword,
      │          Bastion:  cfg.Bastion,   ← 同一个跳板机
      │      }
      │      rootClient = sshclient.NewClient(rootCfg)
      │      └─ 返回 rootClient.SSHClient() + closeFn
      │
      └─ 3d. 用获得的 *ssh.Client 创建传输层
             ├─ SFTPTransport(client) — 优先
             └─ SCPTransport(client)  — SFTP 不可用时降级
```

### 3.2 `getPreferredTransferSSHClient` 详解 (`app.go:1065`)

这是文件传输的核心决策函数：

```go
func (a *App) getPreferredTransferSSHClient(sessionID string) (*ssh.Client, func(), string, error) {
    // 1. 获取当前会话的 SSH 连接（普通用户通过跳板机建立的）
    sess, _ := a.sessionMgr.Get(sessionID)
    base := sess.Client.SSHClient()
    baseClose := func() {}
    identity := "login"

    // 2. 读取该会话的连接配置
    cfg, ok := a.activeConfigs[sessionID]
    if !ok {
        return base, baseClose, identity, nil  // 无配置，用普通用户
    }

    // 3. 没有 root 密码，直接用普通用户
    if cfg.RootPassword == "" {
        return base, baseClose, identity, nil
    }
    // 已经是 root 用户，直接用
    if strings.EqualFold(cfg.User, "root") {
        return base, baseClose, "root", nil
    }

    // 4. ★ 关键：用 root 账号 + root 密码 + 同一跳板机，新建一条 SSH 连接
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
    // 失败则降级回普通用户
    if err != nil || rootClient == nil || rootClient.SSHClient() == nil {
        return base, baseClose, identity, nil
    }
    return rootClient.SSHClient(), func() { _ = rootClient.Close() }, "root", nil
}
```

**要点总结**：
- 文件传输**不走 `su -` 提权**，而是**新建一条 `root@目标机` 的 SSH 连接**
- 新连接经过**同一个跳板机**（Bastion 配置被原样复制）
- 如果 root 连接失败，**静默降级**为普通用户身份（无日志、无告警）

### 3.3 MCP Server 的 Root 提权方式（不同路径）

MCP Server（AI Agent 调用路径）的提权方式与 UI 文件传输不同：

**连接时** (`pkg/mcpserver/tools.go:158`)：
```
root 密码获取优先级：
1. sessions.json 中的 serverConfig.RootPassword（可能明文）
2. Keyring: service="OpsCopilot-SSH", key=host:root
3. （注释提到但不实现：用普通用户密码当 root 密码）
```

**执行命令时** (`pkg/mcpserver/tools.go:309`)：
```go
// 不是新建 SSH 连接，而是通过 su -c 在当前 session 执行
if conn.RootPassword != "" {
    escapedCmd := strings.ReplaceAll(command, "'", "'\\''")
    fullCmd := fmt.Sprintf("echo '%s' | su -c '%s' -", conn.RootPassword, escapedCmd)
    output, err = conn.Client.Run(fullCmd)
    // 失败则回退到普通执行
}
```

> **安全风险**：`echo 'password' | su -c 'cmd' -` 格式中，root 密码会以命令行参数形式出现在进程列表中（`/proc/*/cmdline`），同一机器的其他用户可通过 `ps aux` 看到。

---

## 四、两种传输协议实现

### 4.1 SFTP Transport (`pkg/filetransfer/sftp_transport.go`)

| 方法 | 功能 | 说明 |
|------|------|------|
| `Check()` | 检查 SFTP 可用性 | `sftp.NewClient()` 测试 |
| `List()` | 列目录 | `c.ReadDir()` |
| `Stat()` | 文件信息 | `c.Stat()` |
| `Upload()` | 上传文件 | `c.Create()` + `copyWithProgress()` |
| `Download()` | 下载文件 | `c.Open()` + `copyWithProgress()` |
| `Mkdir()` | 创建目录 | `c.MkdirAll()` |
| `Rename()` | 重命名 | `c.Rename()` |
| `Remove()` | 删除 | 单文件或递归删除 |
| `ReadFile()` | 读取远端文件内容 | `io.LimitReader`，默认 256KB 上限 |
| `WriteFile()` | 写入远端文件内容 | `O_WRONLY|O_CREATE|O_TRUNC` |

- 每个 API 调用都会 `newClient()` 创建新的 sftp.Client session，用完 `defer Close()`
- 所有错误都经过 `toTransferError()` 转为结构化的 `TransferError`

### 4.2 SCP Transport (`pkg/filetransfer/scp_transport.go`)

| 方法 | 功能 | 说明 |
|------|------|------|
| `Check()` | 检查 SCP 可用性 | `command -v scp` |
| `Upload()` | 上传 | 原生 SCP 协议：`C0644 size name\n` + 数据 + ACK |
| `Download()` | 下载 | `scp -f` 命令 + 协议解析 |

- 仅支持 Upload/Download，不支持 ReadFile/WriteFile（UI 层做了限制）
- 实现了完整的 SCP 协议握手（ACK/NACK）

### 4.3 自动降级策略 (`app.go:1269-1307`)

```
尝试 SFTP → 失败？
  ├─ 错误码是 SFTP_NOT_SUPPORTED / UNKNOWN / NETWORK
  │   └─ 尝试 SCP → Check 通过？
  │       ├─ 是 → 使用 SCP
  │       └─ 否 → 报错 "对端未开启 SFTP，且未安装 scp"
  └─ 其他错误 → 直接报错
```

---

## 五、跳板机连接机制 (`pkg/sshclient/client.go`)

### 5.1 连接优先级

```
1. TCP Forwarding (bastionClient.Dial)
   └─ 使用 SSH 内置的端口转发
2. Netcat 降级 (dialViaConsole)
   ├─ nc host port
   ├─ ncat host port
   ├─ netcat host port
   ├─ bash -c 'exec 3<>/dev/tcp/host/port; cat <&3 & cat >&3'
   └─ python3 socket proxy
```

### 5.2 认证方式

```go
authMethods := []ssh.AuthMethod{
    ssh.Password(config.Password),
    ssh.KeyboardInteractive(func(...) {
        // 对所有问题都回复同一密码
        answers[i] = config.Password
    }),
}
```

- 同时注册 Password + KeyboardInteractive，兼容更多服务器
- **注意**：`HostKeyCallback: ssh.InsecureIgnoreHostKey()` — 不验证主机密钥（存在 MITM 风险）

---

## 六、密码存储

### 6.1 Keyring 存储 (`pkg/secretstore/store.go`)

使用 `go-keyring` 库，映射到操作系统原生 Keyring：
- **Windows**: Windows Credential Manager
- **macOS**: Keychain
- **Linux**: Secret Service / gnome-keyring

### 6.2 存储键值

| 场景 | service | user (key) | 密码 |
|------|---------|------------|------|
| UI 连接主机密码 | `OpsCopilot-SSH` | `host:user` | user 的密码 |
| UI 连接跳板机密码 | `OpsCopilot-SSH` | `bastionHost:bastionUser` | 跳板机用户密码 |
| MCP 获取主机密码 | `opscopilot` | `host_user` | user 的密码 |
| MCP 获取 root 密码 | `OpsCopilot-SSH` | `host:root` | root 密码 |
| MCP 获取跳板机密码 | `opscopilot` | `bastionHost_bastionUser` | 跳板机密码 |

> **不一致**：UI 用 `host:user`（冒号），MCP 获取主机密码用 `host_user`（下划线）。这导致 MCP Server 可能无法获取到 UI 保存的密码。

### 6.3 明文回退

MCP Server 中存在明文密码回退：
```go
// tools.go:134-142
password, err := s.secretStore.Get("opscopilot", serverConfig.Host+"_"+serverConfig.User)
if password == "" && serverConfig.Password != "" {
    password = serverConfig.Password  // ← sessions.json 中的明文密码
}
```

---

## 七、安全问题清单

### CRITICAL

| # | 问题 | 位置 | 说明 |
|---|------|------|------|
| C1 | **root 密码出现在命令行** | `tools.go:314` | `echo 'password' \| su -c 'cmd' -` 会暴露在 `/proc/*/cmdline`，同一机器其他用户 `ps aux` 可见 |
| C2 | **HostKey 不验证** | `client.go:61` | `ssh.InsecureIgnoreHostKey()` 使连接易受 MITM 攻击，跳板机场景下风险更高 |
| C3 | **Root 密码明文存储** | sessions.json | `serverConfig.RootPassword` 可能为明文，MCP 回退读取 |

### HIGH

| # | 问题 | 位置 | 说明 |
|---|------|------|------|
| H1 | **root 连接失败静默降级** | `app.go:1103` | root SSH 建连失败时静默回退到普通用户，用户可能以为在用 root 操作实际权限不足 |
| H2 | **Keyring key 不一致** | `app.go:369` vs `tools.go:132` | UI 存 `host:user`，MCP 读 `host_user`，导致 MCP 可能无法获取密码 |
| H3 | **SCP 路径未做安全校验** | `scp_transport.go:57` | `shellSingleQuote` 防注入，但对 `..` 路径穿越未做限制 |
| H4 | **无文件完整性校验** | 全局 | 上传/下载无 checksum 验证 |

### MEDIUM

| # | 问题 | 位置 | 说明 |
|---|------|------|------|
| M1 | **每次操作新建 sftp client** | `sftp_transport.go:298` | 每次 List/Stat/Upload 等都 `sftp.NewClient()`，频繁操作时效率低 |
| M2 | **root SSH 连接不复用** | `app.go:1102` | 每次 `getPreferredTransferSSHClient` 都新建 root 连接，连续操作时浪费资源 |
| M3 | **su - 自动发送的 race condition** | `client.go:239` | `time.Sleep(500ms)` 后发 `su -`，若 shell 初始化慢则可能丢包 |
| M4 | **无传输速率限制** | `copy.go` | 大文件传输可能占满带宽 |

---

## 八、两条路径对比

| 维度 | UI 文件管理器 | MCP Server (AI Agent) |
|------|--------------|----------------------|
| **入口** | `app.go` FT* 方法 | `tools.go` toolServerConnect/Exec |
| **root 提权方式** | 新建 `root@target` SSH 连接（经跳板机） | `echo 'pwd' \| su -c 'cmd' -` 在现有 session 执行 |
| **传输协议** | SFTP（优先）→ SCP 降级 | 无文件传输，仅命令执行 |
| **root 密码来源** | `activeConfigs[sessionID].RootPassword` | sessions.json → Keyring → 明文回退 |
| **失败策略** | 静默降级到普通用户 | su 失败则回退到普通执行 |
| **连接复用** | 每次 getPreferred 都新建 root 连接 | 复用现有 connection |
| **安全等级** | 较好（root 密码不在命令行暴露） | 较差（密码在命令行可见） |

---

## 九、改进建议

### 短期修复

1. **统一 Keyring key 格式**：`app.go` 和 `tools.go` 使用相同的 `service:key` 格式
2. **root 连接失败时通知用户**：不要静默降级，至少在 UI 上显示 warning
3. **MCP 中避免 `echo 'password'`**：改用 `sudo -S` 从 stdin 传入密码，或改用与 UI 相同的新建 root SSH 连接方式
4. **SSH HostKey 持久化**：首次连接时保存 host key，后续验证

### 中期优化

5. **复用 root SSH 连接**：缓存 root client，避免每次操作都新建
6. **复用 sftp.Client**：在 SFTPTransport 内缓存 client，减少连接开销
7. **添加文件传输完整性校验**：传输后比对 checksum
8. **Root 密码不落盘**：移除 sessions.json 中的 RootPassword 字段，仅从 Keyring 获取

### 长期规划

9. **基于 sudoers 的权限控制**：替代直接 root 密码，使用 `NOPASSWD` sudo 规则
10. **SSH Certificate 认证**：替代密码认证，更安全
11. **传输审计日志**：记录所有文件操作的完整审计链路