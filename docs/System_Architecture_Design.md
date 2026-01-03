# 开发方案说明书 (System Architecture Design)

**技术架构**：Go (Backend) + Wails (Bridge) + React/xterm.js (Frontend)
**适用阶段**：系统设计与编码实施

## 1. 总体架构图

```mermaid
graph TD
    User[用户] --> UI[Frontend (React + xterm.js)]
    
    subgraph "Frontend Layer (Webview)"
        UI -- Render --> Terminal[xterm.js 实例]
        UI -- Events --> Bridge[Wails JS Runtime]
    end

    subgraph "Backend Layer (Go)"
        Bridge -- Binding --> App[App Controller]
        
        App --> AI[AI Service]
        App --> SSH[SSH Manager]
        App --> Store[Config Store]
        
        AI -- API --> LLM[OpenAI/DeepSeek]
        AI -- Search --> VectorDB[Local Vector Memory]
        
        SSH -- TCP --> Bastion[跳板机]
        Bastion -- Tunnel --> Target[目标服务器]
    end
```

## 2. 关键模块详细设计

### 2.1 SSH 核心连接模块 (Backend)
**技术选型**：`golang.org/x/crypto/ssh`
**核心职责**：实现诉求 3 的复杂跳板与提权。

*   **数据结构设计**：
    ```go
    type SSHSession struct {
        ID       string
        Client   *ssh.Client
        Session  *ssh.Session
        Stdin    io.WriteCloser
        // 用于广播的通道
        CmdChan  chan string
    }
    ```

*   **关键流程：自动提权 (Auto-Sudo)**
    1.  建立标准 SSH 连接。
    2.  申请 PTY (Pseudo Terminal)。
    3.  启动一个 Goroutine 实时读取 `Stdout`。
    4.  **状态机匹配**：使用 `strings.Contains` 或正则匹配流数据。
        *   若检测到 `Password:` 或 `密码：` 关键字，且当前处于“提权等待状态”，则向 `Stdin` 写入 Root 密码 + `\n`。
    5.  所有读取到的数据通过 `wails.Runtime.EventsEmit` 推送给前端。

### 2.2 终端渲染模块 (Frontend)
**技术选型**：React + `xterm.js` + `xterm-addon-fit` + `xterm-addon-webgl`
**核心职责**：实现诉求 4 的 UI 展示。

*   **多屏实现**：
    *   使用 CSS Grid (`grid-template-columns: 1fr 1fr`) 实现 2x2 布局。
    *   每个格子是一个独立的 React 组件 `<TerminalInstance id="uuid" />`。
*   **广播实现**：
    *   前端维护一个 `activeSessionIDs` 数组。
    *   用户在广播框输入时，调用 Go 方法 `Broadcast(ids, command)`。
    *   Go 后端遍历 ID，并发写入对应的 Stdin。

### 2.3 AI 智能体模块 (Backend)
**技术选型**：`langchain-go` 或原生 API 调用 + `chromem-go` (嵌入式向量库)
**核心职责**：实现诉求 1, 2, 5。

*   **NLP 连接解析 (Prompt Engineering)**
    *   **Prompt**: `提取用户输入中的连接信息，返回严格的 JSON 格式：{"target_ip":"", "jump_ip":"", "user":"", "pwd":"", "root_pwd":""}`
    *   Go 接收 JSON 并反序列化为 `ConnectConfig` 对象。

*   **RAG 知识库实现**
    *   **预处理**：应用启动时，扫描 `./knowledge` 目录下的 Markdown 文件。
    *   **索引**：将文档按段落切分，调用 Embedding 接口（或本地模型），存入内存中的向量索引。
    *   **查询**：用户提问 -> Embedding -> 向量相似度搜索 -> 组装 Prompt -> LLM 生成。

## 3. 接口定义 (Wails Bridge)

Go 暴露给前端的方法 (`App.go`)：

1.  `ParseConnectIntent(naturalLang string) ConnectConfig`
    *   功能：解析自然语言连接信息。
2.  `StartWorkspace(nodeConfigs []NodeConfig) []string`
    *   功能：并发启动连接，返回 Session IDs。
3.  `ResizeTerminal(id string, rows int, cols int)`
    *   功能：同步窗口大小变化到 PTY。
4.  `WriteToSession(id string, data string)`
    *   功能：前端 xterm 输入数据写入后端。
5.  `AskAI(question string, context string) AIResponse`
    *   功能：RAG 问答。

## 4. 安全设计
1.  **敏感信息脱敏**：日志中严禁打印密码字段。
2.  **密钥存储**：使用 `github.com/zalando/go-keyring` 将密码存储在操作系统的安全容器中（MacOS Keychain, Windows Credential Manager），仅在连接建立瞬间读取内存。

## 5. 实施路线图
*   **Phase 1 (基础)**: 跑通 Go+Wails+xterm.js，实现单节点 SSH 连接（无跳板机）。
*   **Phase 2 (连接)**: 实现跳板机 Tunneling 和 Root 自动提权逻辑。
*   **Phase 3 (多开)**: 完成前端分屏 UI 和后端并发连接池，实现命令广播。
*   **Phase 4 (AI)**: 接入 LLM API，完成 NLP 解析和 RAG 文档检索功能。
