# OpsCopilot 迭代计划与验收标准

**基准文档**：
- [PRD.md](./PRD.md)
- [System_Architecture_Design.md](./System_Architecture_Design.md)

---

## 迭代一：项目脚手架与基础终端 (v0.1)
**目标**：搭建 Go + Wails + React 基础框架，实现单节点直连 SSH 终端功能。

### 开发任务
1.  **项目初始化**：
    - 初始化 Wails 项目 (Go 1.21+, React, TypeScript)。
    - 集成 `xterm.js`, `xterm-addon-fit`, `xterm-addon-webgl` 到前端。
2.  **后端 SSH 基础封装**：
    - 实现 `SSHSession` 结构体。
    - 封装 `golang.org/x/crypto/ssh` 实现标准 TCP 连接。
    - 实现 PTY 请求与 Shell 启动。
3.  **前后端通信管道**：
    - 定义 `WriteToSession` (前端 -> 后端) 和 `EventsEmit` (后端 -> 前端) 数据流。
    - 实现 `ResizeTerminal` 以同步窗口大小。

### 验收标准 (Acceptance Criteria)
- [x] **启动测试**：应用能在 Windows/MacOS 正常启动，无白屏。
- [x] **连接测试**：输入 IP、端口、用户名、密码（或 Key），能成功连接至测试服务器。
- [x] **交互测试**：
    - 能在终端中执行基础命令 (`ls`, `top`, `vim`)。
    - `vim` 编辑器显示正常，无乱码。
    - 调整窗口大小时，`top` 命令的输出布局能自动适配（PTY Resize 生效）。
- [x] **性能指标**：打字延迟无肉眼可见卡顿。

---

## 迭代二：高级连接管理与安全存储 (v0.2)
**目标**：实现跳板机隧道连接、Root 自动提权及密码安全存储。

### 开发任务
1.  **跳板机隧道 (Bastion Tunneling)**：
    - 基于 `ssh.Client.Dial` 实现多跳连接逻辑。
    - 支持 "Local -> Bastion -> Target" 链路。
2.  **自动提权 (Auto-Sudo)**：
    - 实现 `Stdout` 流监听状态机。
    - 识别 `Password:` / `密码：` 关键字并自动注入 Root 密码。
3.  **安全存储**：
    - 集成 `github.com/zalando/go-keyring`。
    - 实现密码的加密写入与读取，移除所有明文配置文件。

### 验收标准 (Acceptance Criteria)
- [x] **跳板机测试**：配置跳板机 IP/Auth 后，能成功穿透连接到内网目标节点。
- [X] **提权测试**：
    - 连接配置中填写 Root 密码。
    - 连接后自动执行 `sudo -i` 或类似命令，自动完成密码输入，最终提示符变为 `#`。
- [X] **安全审计**：
    - 检查本地配置文件（如 json/yaml），确认无明文密码。
    - 检查应用日志，确认无密码泄露。

---

## 迭代三：多路复用与工作区 (v0.3)
**目标**：实现 2x2 分屏、会话并发管理及命令广播。

### 开发任务
1.  **前端分屏 UI**：
    - 实现 Grid 布局系统，支持动态添加/移除终端组件。
    - 状态栏显示每个终端的连接状态（Connecting, Connected, Disconnected）。
2.  **后端连接池**：
    - 升级 `App` 控制器，管理 `map[string]*SSHSession`。
    - 实现 `StartWorkspace` 接口，支持并发启动多个 Goroutine 进行连接。
3.  **命令广播**：
    - 实现前端广播输入框。
    - 后端实现 `Broadcast` 方法，遍历 Session 列表写入 Stdin。

### 验收标准 (Acceptance Criteria)
- [ ] **场景加载**：点击一个“场景”，能在 3 秒内同时拉起 4 个终端窗口。（已延后至后续迭代，涉及场景管理与导入导出功能）
- [x] **广播测试**：
    - 在广播框输入 `date`，4 个终端几乎同时显示当前时间。
    - 在广播框输入 `vi test.txt`，4 个终端同时进入编辑模式。
- [x] **独立操作**：点击任意一个终端，可以单独对其输入，互不影响。

---

## 迭代四：AI 智能业务助手 - 核心篇 (v0.4)
**目标**：接入 LLM，实现自然语言解析连接意图 (Smart Connect)。

### 开发任务
1.  **LLM 服务接入**：
    - 封装 OpenAI/DeepSeek API 客户端。
    - 设计系统提示词 (System Prompt) 用于结构化数据提取。
2.  **NLP 连接解析**：
    - 实现 `ParseConnectIntent` 接口。
    - 前端实现自然语言输入框与确认弹窗。

### 验收标准 (Acceptance Criteria)
- [ ] **语义解析测试**：
    - 输入：“连一下 192.168.1.100，用 jump-server 跳板，密码都是 123456”。
    - 输出：确认框中准确填入 Target IP, Jump IP, User, Password。
- [ ] **容错测试**：输入无关文本时，提示无法识别或请求补充信息。

---

## 迭代五：AI 智能业务助手 - 增强篇 (v0.5/v1.0)
**目标**：实现本地知识库 (RAG) 与故障排查辅助。

### 开发任务
1.  **向量数据库集成**：
    - 集成 `chromem-go` 或类似轻量级向量库。
    - 实现 Markdown 文档的解析、切片与 Embedding 存储。
2.  **RAG 问答链路**：
    - 实现“检索 -> 增强 -> 生成”流程。
    - 优化 Prompt 以支持运维场景的命令生成。
3.  **动态命令模板 UI**：
    - 识别 AI 返回的参数化命令（如 `{{id}}`），并在 UI 渲染为表单。

### 验收标准 (Acceptance Criteria)
- [ ] **知识库导入**：将《支付系统排查手册.md》放入指定目录，重启应用后生效。
- [ ] **问答测试**：
    - 提问：“支付超时怎么办？”
    - 回答：准确引用文档内容，并给出排查步骤。
- [ ] **命令生成**：
    - 回答中包含 `grep "Timeout" /var/log/pay.log`。
    - 用户点击“执行”或“填充”，命令自动填入当前终端或广播框。

---

## 总结
- **MVP (最小可行性产品)**：完成迭代一、二、三。此时已具备作为主力终端工具的能力。
- **完整版**：完成迭代四、五。此时具备“智能运维”的核心竞争力。
