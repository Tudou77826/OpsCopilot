# OpsCopilot MCP Server 设计文档

> 版本: v1.0
> 日期: 2024-03-13
> 状态: 待评审

## 1. 概述

### 1.1 目标

为 OpsCopilot 提供 MCP Server 能力，让 AI Agent（如 Claude Code）能够：
- 利用知识库获取排查思路
- 通过 SSH 执行只读诊断命令
- 自动进行操作审计和步骤录制
- 将排查经验沉淀为知识库

### 1.2 核心价值

```
AI 辅助定位 ──▶ 操作审计录制 ──▶ 经验归档化 ──▶ 知识复用
      ↑              ↑
   MCP Server      Recorder
```

### 1.3 约束条件

1. **只读操作**：所有命令必须是只读的，不能修改系统状态
2. **输出控制**：限制返回内容大小，防止撑爆 AI 上下文
3. **审计录制**：所有操作必须记录，用于复盘和知识沉淀
4. **连接管理**：支持多服务器，自动超时断开

---

## 2. 架构设计

### 2.1 整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                   AI Agent (Claude Code)                     │
│                      MCP Client                              │
└─────────────────────────┬───────────────────────────────────┘
                          │ MCP Protocol (stdio)
                          │ JSON-RPC 2.0
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                  OpsCopilot MCP Server                       │
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         │
│  │   Session   │  │   Command   │  │    Audit    │         │
│  │   Manager   │  │   Filter    │  │   Recorder  │         │
│  └──────┬──────┘  └─────────────┘  └──────┬──────┘         │
│         │                                   │                │
│  ┌──────┴───────────────────────────────────┴──────┐        │
│  │              Connection Pool                    │        │
│  └────────────────────────────────────────────────┘        │
│                                                             │
└─────────────────────────┬───────────────────────────────────┘
                          │
        ┌─────────────────┼─────────────────┐
        ▼                 ▼                 ▼
   ┌─────────┐       ┌─────────┐       ┌─────────┐
   │ Recorder│       │Knowledge│       │ Secret  │
   │ (现有)   │       │ (现有)   │       │ Store   │
   └─────────┘       └─────────┘       │ (现有)   │
                                       └─────────┘
```

### 2.2 模块职责

| 模块 | 职责 | 依赖 |
|------|------|------|
| MCP Server | 处理 MCP 协议，路由工具调用 | - |
| Session Manager | 管理排查会话生命周期 | pkg/recorder |
| Connection Pool | 管理 SSH 连接，自动清理 | pkg/sshclient, pkg/config |
| Command Filter | 只读命令白名单检查 | - |
| Audit Recorder | 记录操作步骤，对接录制系统 | pkg/recorder |

### 2.3 数据流

```
AI Agent 调用工具
      │
      ▼
┌─────────────────┐
│  MCP Server     │ 解析 JSON-RPC 请求
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Tool Router    │ 根据 tool name 分发
└────────┬────────┘
         │
    ┌────┴────┬────────────┐
    ▼         ▼            ▼
┌───────┐ ┌───────┐   ┌───────────┐
│Hints  │ │Session│   │ SSH Exec  │
│Tool   │ │Tools  │   │ Tool      │
└───────┘ └───────┘   └─────┬─────┘
                          │
                    ┌─────┴─────┐
                    ▼           ▼
              ┌──────────┐ ┌──────────┐
              │ Command  │ │Connection│
              │ Filter   │ │ Pool     │
              └──────────┘ └────┬─────┘
                               │
                               ▼
                         ┌──────────┐
                         │ SSH Exec │
                         └────┬─────┘
                              │
                    ┌─────────┴─────────┐
                    ▼                   ▼
              ┌──────────┐        ┌──────────┐
              │  Audit   │        │  Output  │
              │ Recorder │        │ Control  │
              └──────────┘        └──────────┘
```

---

## 3. MCP 工具定义

### 3.1 工具清单

| 工具名 | 描述 | 分类 |
|--------|------|------|
| `get_hints` | 获取排查思路提示 | 诊断辅助 |
| `session_start` | 开始排查会话 | 会话管理 |
| `session_status` | 查看会话状态 | 会话管理 |
| `session_end` | 结束排查会话 | 会话管理 |
| `server_list` | 列出服务器 | 服务器管理 |
| `server_connect` | 连接服务器 | 服务器管理 |
| `server_disconnect` | 断开连接 | 服务器管理 |
| `ssh_exec` | 执行只读命令 | 核心功能 |

### 3.2 工具详细定义

#### 3.2.1 get_hints

```json
{
    "name": "get_hints",
    "description": "基于知识库获取排查思路提示。\n\n输入问题描述，返回可能相关的排查方法、常用命令和注意事项。\n\n适用场景：\n- 开始排查前获取思路\n- 遇到问题时寻求指导\n- 了解相关知识点",
    "inputSchema": {
        "type": "object",
        "properties": {
            "problem": {
                "type": "string",
                "description": "问题描述或症状"
            },
            "context": {
                "type": "string",
                "description": "额外上下文（如服务器类型、应用名称、错误信息）"
            }
        },
        "required": ["problem"]
    }
}
```

**返回结构**：
```json
{
    "hints": [
        {
            "title": "CPU 使用率高排查",
            "description": "当 CPU 使用率异常时的一般排查思路",
            "commands": ["top -bn1 | head -20", "ps aux --sort=-%cpu | head"],
            "key_points": ["关注占用最高的进程", "检查是否有僵尸进程", "查看系统负载"]
        }
    ],
    "related_docs": ["troubleshooting/cpu-high.md"],
    "confidence": 0.85
}
```

#### 3.2.2 session_start

```json
{
    "name": "session_start",
    "description": "开始一个新的排查会话。\n\n所有后续操作都会关联到这个会话，用于审计录制和知识归档。\n\n建议在开始排查时调用此工具。",
    "inputSchema": {
        "type": "object",
        "properties": {
            "problem": {
                "type": "string",
                "description": "问题描述（用于标识和归档）"
            },
            "servers": {
                "type": "array",
                "items": {"type": "string"},
                "description": "可能涉及的服务器列表（可选，后续可添加）"
            }
        },
        "required": ["problem"]
    }
}
```

**返回结构**：
```json
{
    "session_id": "sess-20240313-001",
    "status": "active",
    "message": "会话已开始，所有操作将被记录"
}
```

#### 3.2.3 session_status

```json
{
    "name": "session_status",
    "description": "查看当前排查会话的状态。\n\n返回已连接的服务器、执行的操作数量等信息。",
    "inputSchema": {
        "type": "object",
        "properties": {}
    }
}
```

**返回结构**：
```json
{
    "session_id": "sess-20240313-001",
    "problem": "prod-api-01 响应慢",
    "status": "active",
    "started_at": "2024-03-13T10:00:00Z",
    "duration_seconds": 300,
    "connected_servers": ["prod-api-01", "prod-db-01"],
    "executed_commands": 12,
    "findings": []
}
```

#### 3.2.4 session_end

```json
{
    "name": "session_end",
    "description": "结束当前排查会话。\n\n会断开所有连接，生成排查报告，并将经验归档到知识库。",
    "inputSchema": {
        "type": "object",
        "properties": {
            "summary": {
                "type": "string",
                "description": "排查总结（问题原因、解决方案）"
            },
            "findings": {
                "type": "array",
                "items": {"type": "string"},
                "description": "关键发现列表"
            },
            "root_cause": {
                "type": "string",
                "description": "根本原因（如果已确定）"
            }
        },
        "required": ["summary"]
    }
}
```

**返回结构**：
```json
{
    "session_id": "sess-20240313-001",
    "status": "completed",
    "report": {
        "duration_seconds": 300,
        "servers_accessed": 2,
        "commands_executed": 12,
        "knowledge_file": "troubleshooting/db-connection-pool-exhausted.md"
    }
}
```

#### 3.2.5 server_list

```json
{
    "name": "server_list",
    "description": "列出所有可用服务器及其连接状态。\n\n返回两类服务器：\n- connected: 已建立连接，可直接使用\n- available: 已保存凭证，可以尝试连接（可能失败）",
    "inputSchema": {
        "type": "object",
        "properties": {
            "group": {
                "type": "string",
                "description": "按分组筛选"
            }
        }
    }
}
```

**返回结构**：
```json
{
    "connected": [
        {
            "name": "prod-api-01",
            "group": "production",
            "connected_at": "2024-03-13T10:00:00Z",
            "idle_seconds": 30
        }
    ],
    "available": [
        {
            "name": "prod-api-02",
            "group": "production",
            "host": "192.168.1.11",
            "user": "ops"
        },
        {
            "name": "prod-db-01",
            "group": "production",
            "host": "192.168.1.20",
            "user": "ops",
            "bastion": "bastion-prod"
        }
    ],
    "groups": ["production", "testing", "development"]
}
```

#### 3.2.6 server_connect

```json
{
    "name": "server_connect",
    "description": "连接到指定服务器。\n\n使用已保存的凭证进行连接。如果通过跳板机，会自动处理。",
    "inputSchema": {
        "type": "object",
        "properties": {
            "server": {
                "type": "string",
                "description": "服务器名称"
            }
        },
        "required": ["server"]
    }
}
```

**返回结构**：
```json
{
    "success": true,
    "server": "prod-api-01",
    "message": "连接成功"
}
```

**错误返回**：
```json
{
    "success": false,
    "server": "prod-api-01",
    "error": "连接失败: dial tcp 192.168.1.10:22: i/o timeout",
    "suggestion": "请检查网络连通性或联系管理员"
}
```

#### 3.2.7 server_disconnect

```json
{
    "name": "server_disconnect",
    "description": "断开指定服务器的连接。",
    "inputSchema": {
        "type": "object",
        "properties": {
            "server": {
                "type": "string",
                "description": "服务器名称"
            }
        },
        "required": ["server"]
    }
}
```

#### 3.2.8 ssh_exec（核心工具）

```json
{
    "name": "ssh_exec",
    "description": "在远程服务器上执行只读诊断命令。\n\n⚠️ 重要：仅支持只读命令！\n\n可用命令类别：\n- 文件查看：cat, head, tail, ls, find, grep, awk, sed -n\n- 进程管理：ps, top, pgrep, pstree\n- 系统资源：free, df, du, iostat, vmstat, mpstat\n- 网络诊断：netstat, ss, ip, ping, nslookup, dig\n- 服务状态：systemctl status/is-active, journalctl\n- Java诊断：jstat, jinfo, jstack, jmap -histo\n- 容器查看：docker ps/images/logs, kubectl get/describe/logs\n\n非只读命令（如 rm, mv, chmod, systemctl restart）会被拒绝。\n\n输出控制：\n- 总大小限制：10KB\n- 单行长度限制：500字（可配置）\n- 超长输出会截断，保留前5行和后N行",
    "inputSchema": {
        "type": "object",
        "properties": {
            "server": {
                "type": "string",
                "description": "服务器名称（必须是已连接的服务器）"
            },
            "command": {
                "type": "string",
                "description": "要执行的只读命令"
            },
            "max_line_length": {
                "type": "integer",
                "description": "单行最大长度，默认 500",
                "default": 500
            },
            "note": {
                "type": "string",
                "description": "命令说明（用于审计记录）"
            }
        },
        "required": ["server", "command"]
    }
}
```

**成功返回**：
```json
{
    "success": true,
    "output": "实际命令输出...",
    "meta": {
        "total_bytes": 15000,
        "returned_bytes": 10240,
        "total_lines": 500,
        "returned_lines": 150,
        "truncated_lines": 350,
        "long_lines_truncated": 2,
        "command": "top -bn1 | head -20",
        "server": "prod-api-01",
        "duration_ms": 234,
        "exit_code": 0
    }
}
```

**命令被拒绝**：
```json
{
    "success": false,
    "error": "命令被拒绝",
    "reason": "'rm -rf /tmp/test' 不在只读白名单中。只读命令包括：文件查看(cat/ls/grep)、进程管理(ps/top)、系统监控(free/df/netstat)、日志查看(tail/journalctl)、诊断工具(jstat/jstack)等。",
    "suggestion": "请使用只读命令，如需要查看文件请使用 cat/head/tail"
}
```

---

## 4. 只读命令白名单

### 4.1 白名单策略

采用**白名单机制**，只有匹配白名单的命令才允许执行。

### 4.2 白名单列表

```go
var allowedCommands = []string{
    // === 文件/目录查看 ===
    `^ls(\s|$)`,
    `^ll(\s|$)`,
    `^la(\s|$)`,
    `^dir(\s|$)`,
    `^tree(\s|$)`,
    `^find\s`,
    `^locate\s`,
    `^cat\s`,
    `^head\s`,
    `^tail\s`,
    `^less\s`,
    `^more\s`,
    `^stat\s`,
    `^file\s`,
    `^wc\s`,
    `^md5sum\s`,
    `^sha256sum\s`,
    `^sha1sum\s`,

    // === 文本处理（只读） ===
    `^grep\s`,
    `^egrep\s`,
    `^fgrep\s`,
    `^rg\s`,
    `^ag\s`,
    `^ack\s`,
    `^awk\s`,
    `^sed\s+(-n|--silent)`,  // 只允许静默模式的 sed
    `^sort\s`,
    `^uniq\s`,
    `^cut\s`,
    `^tr\s+[^\*]`,  // 不允许 tr 的删除操作
    `^column\s`,
    `^expand\s`,
    `^unexpand\s`,
    `^fold\s`,
    `^fmt\s`,
    `^pr\s`,
    `^tac\s`,
    `^rev\s`,
    `^basename\s`,
    `^dirname\s`,
    `^realpath\s`,
    `^readlink\s`,

    // === 系统信息 ===
    `^uname(\s|$)`,
    `^hostname(\s|$)`,
    `^hostnamectl\s`,
    `^date(\s|$)`,
    `^uptime(\s|$)`,
    `^whoami(\s|$)`,
    `^id(\s|$)`,
    `^who(\s|$)`,
    `^w(\s|$)`,
    `^users(\s|$)`,
    `^last(\s|$)`,
    `^lastb(\s|$)`,
    `^lastlog\s`,
    `^dmesg(\s|$)`,
    `^journalctl\s`,
    `^lsb_release\s`,
    `^cat\s+/etc/os-release`,
    `^cat\s+/etc/issue`,

    // === 进程管理（只读） ===
    `^ps\s`,
    `^top(\s|$)`,
    `^htop(\s|$)`,
    `^atop(\s|$)`,
    `^pgrep\s`,
    `^pidof\s`,
    `^pstree\s`,
    `^tasklist(\s|$)`,  // Windows
    `^wmic\s+process\s`,  // Windows

    // === 内存/磁盘/IO ===
    `^free\s`,
    `^df\s`,
    `^du\s`,
    `^lsblk(\s|$)`,
    `^blkid\s`,
    `^lsusb(\s|$)`,
    `^lspci(\s|$)`,
    `^lscpu(\s|$)`,
    `^dmidecode\s+-t`,  // 只允许查询特定类型
    `^hdparm\s+-[iI]`,
    `^fdisk\s+-l`,
    `^parted\s+.*print`,
    `^mount(\s|$)`,
    `^findmnt\s`,
    `^swapon\s+--show`,
    `^iostat\s`,
    `^vmstat\s`,
    `^mpstat\s`,
    `^sar\s`,
    `^pmap\s`,
    `^smem\s`,

    // === Java 诊断（只读） ===
    `^java\s+-version`,
    `^javac\s+-version`,
    `^jps(\s|$)`,
    `^jstat\s`,
    `^jinfo\s`,
    `^jmap\s+(-histo|--histo:live|-clstats)`,
    `^jstack\s`,
    `^jcmd\s+\d+\s+(VM\.|GC\.|Thread\.print|VM\.system_properties|VM\.flags|VM\.command_line)`,
    `^jhat\s`,  // 分析工具
    `^jrunscript\s`,

    // === 压缩文件查看 ===
    `^zcat\s`,
    `^zgrep\s`,
    `^zless\s`,
    `^zmore\s`,
    `^bzcat\s`,
    `^bzgrep\s`,
    `^xzcat\s`,
    `^xzgrep\s`,
    `^tar\s+(-t|--list)`,
    `^unzip\s+-l`,
    `^zipinfo\s`,
    `^7z\s+l`,
    `^rar\s+l`,

    // === 其他只读工具 ===
    `^echo\s`,
    `^printf\s+['\"]%`,
    `^test\s`,
    `^\[\s+`,
    `^expr\s`,
    `^bc\s+.*<<<`,
    `^awk\s+.*print`,
    `^jq\s`,
    `^yq\s+.*\.`,
    `^xmllint\s+.*--xpath`,
    `^xsltproc\s`,
    `^xmlstarlet\s+.*sel`,
    `^base64\s+(-d|--decode)`,
    `^xxd\s`,
    `^od\s`,
    `^hexdump\s`,
    `^strings\s`,
    `^time\s`,
    `^timeout\s+\d+\s+`,
    `^strace\s+.*-p`,  // 只允许 attach 模式
    `^ltrace\s+.*-p`,
    `^perf\s+(stat|record|top|list|show|script)`,
    `^bpftrace\s+-e`,
    `^opensnoop\s`,
    `^execsnoop\s`,
    `^statsnoop\s`,
    `^biosnoop\s`,
    `^filetop\s`,
    `^iolatency\s`,
}
```

### 4.3 检查逻辑

```go
type CommandChecker struct {
    allowedPatterns []*regexp.Regexp
}

func NewCommandChecker() *CommandChecker {
    c := &CommandChecker{}
    for _, pattern := range allowedCommands {
        c.allowedPatterns = append(c.allowedPatterns, regexp.MustCompile(pattern))
    }
    return c
}

func (c *CommandChecker) Check(command string) (allowed bool, reason string) {
    command = strings.TrimSpace(command)

    // 空命令
    if command == "" {
        return false, "命令不能为空"
    }

    // 检查白名单
    for _, pattern := range c.allowedPatterns {
        if pattern.MatchString(command) {
            return true, ""
        }
    }

    // 不在白名单
    return false, fmt.Sprintf(
        "命令 '%s' 不在只读白名单中。\n\n"+
        "可用的只读命令类别：\n"+
        "- 文件查看：cat, head, tail, ls, find, grep, awk\n"+
        "- 进程管理：ps, top, pgrep, pstree\n"+
        "- 系统资源：free, df, du, iostat, vmstat\n"+
        "- 网络诊断：netstat, ss, ip, ping, nslookup\n"+
        "- 服务状态：systemctl status, journalctl\n"+
        "- Java诊断：jstat, jinfo, jstack, jmap -histo\n"+
        "- 容器查看：docker ps/logs, kubectl get/logs\n\n"+
        "如需其他命令，请联系管理员更新白名单。",
        command,
    )
}
```

---

## 5. 输出控制策略

### 5.1 控制参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `max_total_bytes` | 10240 (10KB) | 返回内容的最大字节数 |
| `max_line_length` | 500 | 单行最大字符数 |
| `head_lines` | 5 | 截断时保留的头部行数 |

### 5.2 处理流程

```
原始输出
    │
    ▼
┌─────────────────────────┐
│  Step 1: 单行截断        │
│  超过 max_line_length    │
│  → 截断并添加标记        │
└────────────┬────────────┘
             │
             ▼
┌─────────────────────────┐
│  Step 2: 总大小检查      │
│  超过 max_total_bytes    │
│  → 保留前5行 + 后N行     │
└────────────┬────────────┘
             │
             ▼
┌─────────────────────────┐
│  Step 3: 生成元信息      │
│  → 计算统计数据          │
│  → 附加到返回结果        │
└─────────────────────────┘
```

### 5.3 实现代码

```go
type OutputController struct {
    MaxTotalBytes  int
    MaxLineLength  int
    HeadLines      int
}

type OutputResult struct {
    Output string
    Meta   OutputMeta
}

type OutputMeta struct {
    TotalBytes         int    `json:"total_bytes"`
    ReturnedBytes      int    `json:"returned_bytes"`
    TotalLines         int    `json:"total_lines"`
    ReturnedLines      int    `json:"returned_lines"`
    TruncatedLines     int    `json:"truncated_lines"`
    LongLinesTruncated int    `json:"long_lines_truncated"`
}

func (c *OutputController) Process(output string) *OutputResult {
    meta := OutputMeta{
        TotalBytes: len(output),
    }

    // Step 1: 单行截断
    lines := strings.Split(output, "\n")
    processedLines := make([]string, 0, len(lines))

    for _, line := range lines {
        if len(line) > c.MaxLineLength {
            // 截断长行：前200字 + 标记 + 后200字
            truncated := line[:200] +
                fmt.Sprintf("...[截断:原长度%d字]...", len(line)) +
                line[len(line)-200:]
            processedLines = append(processedLines, truncated)
            meta.LongLinesTruncated++
        } else {
            processedLines = append(processedLines, line)
        }
    }
    meta.TotalLines = len(processedLines)

    // Step 2: 总大小检查
    fullOutput := strings.Join(processedLines, "\n")
    if len(fullOutput) <= c.MaxTotalBytes {
        meta.ReturnedBytes = len(fullOutput)
        meta.ReturnedLines = len(processedLines)
        return &OutputResult{
            Output: fullOutput,
            Meta:   meta,
        }
    }

    // 需要截断
    var resultLines []string
    var usedBytes int

    // 保留头部
    for i := 0; i < c.HeadLines && i < len(processedLines); i++ {
        line := processedLines[i]
        if usedBytes + len(line) + 1 > c.MaxTotalBytes {
            break
        }
        resultLines = append(resultLines, line)
        usedBytes += len(line) + 1
    }

    headCount := len(resultLines)

    // 添加省略标记
    omitMarker := fmt.Sprintf("\n...[省略 %d 行]...\n", len(processedLines)-headCount)
    if usedBytes + len(omitMarker) < c.MaxTotalBytes {
        resultLines = append(resultLines, omitMarker)
        usedBytes += len(omitMarker)
    }

    // 从尾部添加行
    tailLines := make([]string, 0)
    remainingBytes := c.MaxTotalBytes - usedBytes

    for i := len(processedLines) - 1; i >= headCount && remainingBytes > 0; i-- {
        line := processedLines[i]
        if len(line)+1 > remainingBytes {
            break
        }
        tailLines = append([]string{line}, tailLines...)
        remainingBytes -= len(line) + 1
    }

    resultLines = append(resultLines, tailLines...)

    finalOutput := strings.Join(resultLines, "\n")
    meta.ReturnedBytes = len(finalOutput)
    meta.ReturnedLines = len(resultLines)
    meta.TruncatedLines = meta.TotalLines - meta.ReturnedLines

    return &OutputResult{
        Output: finalOutput,
        Meta:   meta,
    }
}
```

---

## 6. 核心数据结构

### 6.1 会话 (Session)

```go
type Session struct {
    ID          string            `json:"id"`
    Problem     string            `json:"problem"`
    StartTime   time.Time         `json:"start_time"`
    EndTime     *time.Time        `json:"end_time,omitempty"`
    Status      SessionStatus     `json:"status"` // active, completed, abandoned

    Connections map[string]*Connection `json:"connections"`
    Steps       []AuditRecord          `json:"steps"`
    Findings    []string               `json:"findings"`

    Recorder    *recorder.Recorder `json:"-"`
}

type SessionStatus string

const (
    SessionActive    SessionStatus = "active"
    SessionCompleted SessionStatus = "completed"
    SessionAbandoned SessionStatus = "abandoned"
)
```

### 6.2 连接 (Connection)

```go
type Connection struct {
    ServerName  string          `json:"server_name"`
    Client      *sshclient.Client `json:"-"`
    ConnectedAt time.Time       `json:"connected_at"`
    LastActive  time.Time       `json:"last_active"`
    Error       string          `json:"error,omitempty"`
}
```

### 6.3 审计记录 (AuditRecord)

```go
type AuditRecord struct {
    ID        string    `json:"id"`
    Timestamp time.Time `json:"timestamp"`
    Server    string    `json:"server"`
    Command   string    `json:"command"`
    Note      string    `json:"note,omitempty"`

    // 结果
    Success    bool   `json:"success"`
    Output     string `json:"output,omitempty"`
    Error      string `json:"error,omitempty"`
    ExitCode   int    `json:"exit_code"`
    DurationMs int64  `json:"duration_ms"`

    // 元信息
    OutputMeta OutputMeta `json:"output_meta,omitempty"`
}
```

### 6.4 服务器配置 (ServerConfig)

```go
// 复用现有结构，位于 pkg/sshclient/client.go
type ServerConfig struct {
    Name         string         `json:"name"`
    Host         string         `json:"host"`
    Port         int            `json:"port"`
    User         string         `json:"user"`
    Group        string         `json:"group,omitempty"`
    Bastion      string         `json:"bastion,omitempty"`
    Description  string         `json:"description,omitempty"`
}
```

---

## 7. 目录结构

```
cmd/
└── mcp-server/
    └── main.go                    # MCP Server 入口

pkg/
└── mcpserver/                     # MCP Server 实现
    ├── server.go                  # MCP 协议处理
    ├── tools.go                   # 工具注册与路由
    │
    ├── session.go                 # 会话管理
    ├── connection.go              # 连接池管理
    ├── checker.go                 # 只读命令检查
    ├── output.go                  # 输出控制
    │
    ├── tools/                     # 工具实现
    │   ├── hints.go               # get_hints
    │   ├── session.go             # session_start/status/end
    │   ├── server.go              # server_list/connect/disconnect
    │   └── ssh.go                 # ssh_exec
    │
    └── types.go                   # 数据结构定义

configs/
└── mcp-claude-code.json.example   # Claude Code 配置示例
```

---

## 8. 依赖关系

```
pkg/mcpserver/
    ├── pkg/sshclient/        # SSH 连接
    ├── pkg/config/           # 配置管理
    ├── pkg/recorder/         # 录制系统（仅复用文件存储逻辑）
    ├── pkg/knowledge/        # 知识库搜索
    ├── pkg/secretstore/      # 凭证存储
    └── pkg/mcp/protocol.go   # MCP 协议类型（复用）
```

---

## 9. 录制系统集成说明

> **重要**: 现有 `pkg/recorder/recorder.go` 是为 Wails 桌面应用设计的，它 它它它从终端输出流中实时提取命令。**MCP Server 场景不同**：我们直接拿到命令字符串和执行结果，不需要从终端流中提取。

MCP Server 需要的录制能力：
1. **复用 `Recorder` 结构体** 定义录制会话结构
2. **复用文件存储逻辑** 保存会话到 JSON 文件
3. **不复用** `CommandExtractor`（终端流提取器和 `StartShellWithSudo`（交互式 Shell）

MCP Server 的简化录制流程:
```go
type RecordingSession struct {
    ID           string
    Type         RecordingType
    Problem      string
    StartTime   time.Time
    EndTime     *time.Time
    RootCause   string
    Conclusion  string
    Commands    []RecordedCommand
    Suggestions []string
}

type RecordedCommand struct {
    ID        string
    Timestamp time.Time
    Command   string
    Server    string
    Output   string    // 截断后的输出
    ExitCode  int
    Duration  time.Duration
}
```

新增配置项:
```json
{
  "recording": {
    "enabled": true,
    "storage_path": "~/.opscopilot/recordings",
    "max_output_length": 10000
  }
}
```

---

### 9.2 MCP Server 录制实现

> MCP Server 的录制需求比 Wails 应用更简单：命令是直接传入的，不是从终端流中提取。
> 因此 MCP Server 使用独立的简化录制器，不直接复用 `pkg/recorder/recorder.go`。

#### 9.2.1 数据结构

```go
// pkg/mcpserver/recording.go

package mcpserver

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "sync"
    "time"

    "github.com/google/uuid"
)

// RecordingType 录制类型
type RecordingType string

const (
    RecordingTypeTroubleshoot RecordingType = "troubleshoot"
    RecordingTypeInspection  RecordingType = "inspection"
)

// RecordedCommand 记录的命令
type RecordedCommand struct {
    ID        string    `json:"id"`
    Timestamp time.Time `json:"timestamp"`
    Command   string    `json:"command"`
    Server    string    `json:"server"`
    Output   string    `json:"output"`
    ExitCode  int       `json:"exit_code"`
    Duration  int64     `json:"duration_ms"`
    Note      string    `json:"note,omitempty"`
}

// RecordingSession 录制会话
type RecordingSession struct {
    ID           string              `json:"id"`
    Type         RecordingType        `json:"type"`
    Problem      string              `json:"problem"`
    StartTime   time.Time            `json:"start_time"`
    EndTime     *time.Time           `json:"end_time,omitempty"`
    RootCause   string              `json:"root_cause,omitempty"`
    Conclusion string              `json:"conclusion,omitempty"`

    Commands    []RecordedCommand `json:"commands"`
    Servers     map[string]bool   `json:"servers"`
    Findings    []string            `json:"findings"`
    Suggestions []string            `json:"suggestions"`

    mu sync.RWMutex `json:"-"`
}

// MCPRecordingManager MCP Server 录制管理器
type MCPRecordingManager struct {
    sessions    map[string]*RecordingSession
    current     *RecordingSession
    storagePath string
    mu          sync.RWMutex
}
```

#### 9.2.2 MCPRecordingManager 实现

```go
func NewMCPRecordingManager(storagePath string) *MCPRecordingManager {
    return &MCPRecordingManager{
        sessions:    make(map[string]*RecordingSession),
        storagePath: storagePath,
    }
}

// StartSession 开始新的录制会话
func (m *MCPRecordingManager) StartSession(problem string, recType RecordingType) (*RecordingSession, error) {
    m.mu.Lock()
    defer m.mu.Unlock()

    session := &RecordingSession{
        ID:         uuid.NewString(),
        Type:       recType,
        Problem:    problem,
        StartTime: time.Now(),
        Commands:   make([]RecordedCommand, 0),
        Servers:    make(map[string]bool),
        Findings:    make([]string, 0),
        Suggestions: make([]string, 0),
    }

    m.sessions[session.ID] = session
    m.current = session

    // 确保目录存在
    if err := os.MkdirAll(m.storagePath, 0755); err != nil {
        return nil, fmt.Errorf("failed to create storage directory: %w", err)
    }

    return session, nil
}

// RecordCommand 记录命令执行
func (m *MCPRecordingManager) RecordCommand(server, command, output string, exitCode int, duration time.Duration, note string) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    if m.current == nil {
        return fmt.Errorf("no active recording session")
    }

    // 截断输出
    const maxOutputLen = 10000
    if len(output) > maxOutputLen {
        output = output[:maxOutputLen] + "...[truncated]"
    }

    m.current.Commands = append(m.current.Commands, RecordedCommand{
        ID:        uuid.NewString(),
        Timestamp: time.Now(),
        Command:   command,
        Server:    server,
        Output:   output,
        ExitCode:  exitCode,
        Duration:  duration.Milliseconds(),
        Note:      note,
    })

    m.current.Servers[server] = true

    return nil
}

// EndSession 结束录制会话
func (m *MCPRecordingManager) EndSession(rootCause, conclusion string, findings []string) (*RecordingSession, error) {
    m.mu.Lock()
    defer m.mu.Unlock()

    if m.current == nil {
        return nil, fmt.Errorf("no active recording session")
    }

    now := time.Now()
    m.current.EndTime = &now
    m.current.RootCause = rootCause
    m.current.Conclusion = conclusion
    m.current.Findings = append(m.current.Findings, findings...)

    // 保存到文件
    if err := m.save(m.current); err != nil {
        return nil, err
    }

    session := m.current
    m.current = nil

    return session, nil
}

// GetCurrentSession 获取当前会话
func (m *MCPRecordingManager) GetCurrentSession() *RecordingSession {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return m.current
}

// save 保存会话到文件
func (m *MCPRecordingManager) save(session *RecordingSession) error {
    // 按类型创建子目录
    typeDir := filepath.Join(m.storagePath, string(session.Type))
    if err := os.MkdirAll(typeDir, 0755); err != nil {
        return fmt.Errorf("failed to create type directory: %w", err)
    }

    filename := filepath.Join(typeDir, fmt.Sprintf("recording_%s.json", session.ID))

    data, err := json.MarshalIndent(session, "", "  ")
    if err != nil {
        return fmt.Errorf("failed to marshal session: %w", err)
    }

    if err := os.WriteFile(filename, data, 0644); err != nil {
        return fmt.Errorf("failed to write file: %w", err)
    }

    return nil
}
```

#### 9.2.3 与 Wails 录制器的对比

| 特性 | Wails 录制器 | MCP Server 录制器 |
|------|-------------|-------------------|
| 命令获取 | 从终端流实时提取 | 直接传入 |
| Shell 类型 | 交互式 PTY | 无 |
| 命令历史 | 支持上下键导航 | 无 |
| 录制触发 | 用户点击按钮 | session_start 工具 |
| 结束触发 | 用户点击结束 | session_end 工具 |
| 输出截断 | 无（完整录制） | 有（10KB 限制） |
| 文件格式 | JSON | JSON（兼容） |
| 存储路径 | 相同 | 相同 |

---

## 10. 迭代计划

### Phase 1: 基础框架 (MVP)

**目标**：可运行的 MCP Server，支持基本的 SSH 执行

**任务清单**：
- [ ] 创建 `cmd/mcp-server/main.go`
- [ ] 实现 MCP 协议处理（initialize, tools/list, tools/call）
- [ ] 实现 `server_list` 工具
- [ ] 实现 `server_connect` 工具
- [ ] 实现 `ssh_exec` 工具（基础版，无输出控制）
- [ ] 实现只读命令白名单检查
- [ ] 编写单元测试

**验收标准**：
- MCP Server 可以被 Claude Code 识别和调用
- 可以连接预配置的服务器
- 可以执行白名单内的命令

**预计工作量**：2-3 天

---

### Phase 2: 会话与审计

**目标**：支持会话管理和操作审计

**任务清单**：
- [ ] 实现 `session_start` 工具
- [ ] 实现 `session_status` 工具
- [ ] 实现 `session_end` 工具
- [ ] 实现审计记录功能
- [ ] 对接 `pkg/recorder` 进行步骤录制
- [ ] 实现连接超时自动断开

**验收标准**：
- 会话可以正常开始和结束
- 所有操作都被记录
- 空闲连接会自动断开

**预计工作量**：2 天

---

### Phase 3: 输出控制

**目标**：实现输出大小控制，防止撑爆 AI 上下文

**任务清单**：
- [ ] 实现 `OutputController`
- [ ] 实现单行截断逻辑
- [ ] 实现总大小截断逻辑
- [ ] 实现元信息计算
- [ ] 添加配置项支持

**验收标准**：
- 大输出被正确截断
- 元信息准确
- 截断不影响可读性

**预计工作量**：1 天

---

### Phase 4: 知识库集成

**目标**：实现 `get_hints` 工具，利用知识库提供排查思路

**任务清单**：
- [ ] 实现 `get_hints` 工具
- [ ] 对接 `pkg/knowledge` 搜索
- [ ] 设计 hints 返回格式
- [ ] 会话结束时生成知识库文档

**验收标准**：
- 可以根据问题返回相关排查思路
- 会话结束后可以生成知识库文档

**预计工作量**：2 天

---

### Phase 5: 完善与优化

**目标**：提升稳定性和用户体验

**任务清单**：
- [ ] 完善错误处理和错误提示
- [ ] 添加连接重试机制
- [ ] 优化白名单（根据使用反馈）
- [ ] 添加日志记录
- [ ] 编写集成测试
- [ ] 编写用户文档

**验收标准**：
- 错误信息清晰友好
- 网络波动时自动重试
- 有完整的使用文档

**预计工作量**：2 天

---

## 11. 风险与缓解措施

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 白名单不够完善，遗漏只读命令 | Agent 无法执行有效命令 | 提供清晰的错误提示，快速迭代更新白名单 |
| 输出截断丢失关键信息 | Agent 无法做出正确判断 | 保留头尾信息，提供元数据让 Agent 决定是否需要更多 |
| 连接超时设置不合理 | 影响排查体验 | 提供配置项，根据反馈调整默认值 |
| 知识库搜索质量不高 | hints 无效 | 复用现有搜索逻辑，持续优化 |
| MCP 协议变更 | 需要适配 | 跟进官方规范，保持兼容 |

---

## 12. 配置示例

### 12.1 Claude Code 配置

`~/.claude/settings.json`:
```json
{
  "mcpServers": {
    "opscopilot": {
      "command": "opscopilot-mcp-server",
      "args": [],
      "env": {
        "OPSCOPILOT_CONFIG_DIR": "${HOME}/.opscopilot"
      }
    }
  }
}
```

### 12.2 OpsCopilot 服务器配置

`~/.opscopilot/servers.json`:
```json
{
  "servers": [
    {
      "name": "prod-api-01",
      "host": "192.168.1.10",
      "port": 22,
      "user": "ops",
      "group": "production"
    },
    {
      "name": "prod-api-02",
      "host": "192.168.1.11",
      "port": 22,
      "user": "ops",
      "group": "production"
    },
    {
      "name": "bastion-prod",
      "host": "bastion.example.com",
      "port": 22,
      "user": "bastion"
    }
  ]
}
```

### 12.3 MCP Server 配置

`~/.opscopilot/mcp-config.json`:
```json
{
  "output": {
    "max_total_bytes": 10240,
    "max_line_length": 500,
    "head_lines": 5
  },
  "connection": {
    "idle_timeout_minutes": 30,
    "connect_timeout_seconds": 10
  },
  "audit": {
    "enabled": true,
    "record_output": true,
    "max_output_record_bytes": 10000
  }
}
```

---

## 13. 附录

### 12.1 MCP 协议交互示例

**初始化**：
```json
→ {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}
← {"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"opscopilot-mcp-server","version":"1.0.0"}}}
```

**获取工具列表**：
```json
→ {"jsonrpc":"2.0","id":2,"method":"tools/list"}
← {"jsonrpc":"2.0","id":2,"result":{"tools":[...]}}
```

**调用工具**：
```json
→ {"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ssh_exec","arguments":{"server":"prod-api-01","command":"uptime"}}}
← {"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"{...}"}]}}
```

### 12.2 参考资料

- [MCP Specification](https://modelcontextprotocol.io/)
- [Claude Code MCP Integration](https://docs.anthropic.com/claude-code/mcp)
- 现有代码: `pkg/mcp/`, `pkg/sshclient/`, `pkg/recorder/`

---

## 14. 使用指南

### 14.1 构建

```bash
# 方式1：完整构建（包含主程序和 MCP Server）
build_release.bat

# 方式2：仅构建 MCP Server
go build -o mcp-server.exe ./cmd/mcp-server/
```

构建产物位于 `build/bin/` 目录：
- `OpsCopilot.exe` - 主程序（桌面应用）
- `mcp-server.exe` - MCP Server（供 Claude Code 调用）

### 14.2 配置 Claude Code

1. **创建 MCP 配置文件**

编辑 `~/.claude/mcp.json`（Windows: `C:\Users\<用户名>\.claude\mcp.json`）：

```json
{
  "mcpServers": {
    "opscopilot": {
      "command": "D:/OpsCopilot/mcp-server.exe",
      "env": {
        "OPSCOPILOT_SESSIONS_FILE": "D:/OpsCopilot/sessions.json",
        "OPSCOPILOT_RECORDINGS_DIR": "D:/OpsCopilot/recordings",
        "OPSCOPILOT_KNOWLEDGE_DIR": "D:/OpsCopilot/docs"
      }
    }
  }
}
```

2. **环境变量说明**

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `OPSCOPILOT_SESSIONS_FILE` | sessions.json 路径 | `sessions.json` |
| `OPSCOPILOT_RECORDINGS_DIR` | 录制文件存储目录 | `recordings` |
| `OPSCOPILOT_KNOWLEDGE_DIR` | 知识库目录（归档排查经验） | `docs` |

3. **启用 MCP Server**

编辑 `~/.claude/settings.json`：

```json
{
  "enabledMcpjsonServers": ["opscopilot"]
}
```

### 14.3 使用示例

在 Claude Code 中，可以直接调用 MCP 工具：

```
帮我排查服务器 39.108.107.148 的磁盘空间使用情况
```

Claude Code 会自动：
1. 调用 `session_start` 开始排查会话
2. 调用 `server_connect` 连接服务器
3. 调用 `ssh_exec` 执行 `df -h`、`du -sh` 等命令
4. 调用 `session_end` 结束会话，归档到知识库

### 14.4 分发给其他用户

1. **打包发布**

```
build/bin/
├── OpsCopilot.exe          # 主程序
├── mcp-server.exe          # MCP Server
├── config.json             # 配置文件（可选）
├── prompts.json            # 提示词（可选）
└── sessions.json           # 服务器配置（用户自己创建）
```

2. **用户配置步骤**

   a. 解压到任意目录（如 `D:\OpsCopilot`）

   b. 在 OpsCopilot 主程序中配置服务器连接（或手动编辑 `sessions.json`）

   c. 配置 Claude Code 的 `mcp.json`（参考 14.2）

   d. 重启 Claude Code

3. **注意事项**

- MCP Server 依赖 OpsCopilot 的 `sessions.json` 和密码存储（OS Keyring）
- 用户需要先在 OpsCopilot 主程序中连接过服务器，密码才会存储到 Keyring
- `sessions.json` 存储服务器配置，Keyring 存储密码

### 14.5 可用工具列表

| 工具 | 功能 |
|------|------|
| `server_list` | 列出可用服务器 |
| `server_connect` | 连接服务器 |
| `server_disconnect` | 断开连接 |
| `ssh_exec` | 执行只读命令 |
| `session_start` | 开始排查会话 |
| `session_status` | 查看会话状态 |
| `session_end` | 结束会话并归档 |
| `get_hints` | 获取排查提示 |

---

*文档结束*
