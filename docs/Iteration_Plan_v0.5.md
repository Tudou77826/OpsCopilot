# OpsCopilot 迭代五：AI 智能业务助手 (增强篇) - 需求分析与功能设计

## 1. 迭代目标
本迭代旨在构建一个具备**多源知识融合**与**自我进化能力**的智能运维辅助系统。
1.  **多源知识融合**：结合“Linux 基础知识”（通用）、“本地业务文档”（私有核心）、“外部业务文档”（实时扩展）以及“历史排查沉淀”四层上下文。
2.  **实战闭环**：不仅仅提供建议，更要记录“问题 -> 决策 -> 执行”的完整排查会话，沉淀为新的知识库资产。

---

## 2. 核心需求分析 (Requirements Analysis)

### 2.1 分层知识上下文 (Layered Context)
鉴于内部 AI 服务已部署在安全内网，我们采用**直接上下文注入 (Direct Context Injection)** 模式，利用大模型的长上下文窗口能力。

*   **L1: Linux 基础命令 (Base Knowledge)**
    *   **定义**：运维通用的 Linux 命令知识（如 `grep`, `netstat`, `lsof`, `awk` 等）。
    *   **集成方式**：作为 System Prompt 的一部分或预置的静态知识包。

*   **L2: 本地业务文档 (Local Business Docs)**
    *   **业务概念与流程 (Business Concept/Process)**: 帮助 AI 理解业务名词、架构和基础流程（如《支付系统架构图》、《订单状态机流转》）。
    *   **排查手册 (Troubleshooting Manual/SOP)**: 具体的故障处理步骤（如《支付超时排查SOP》）。
    *   **集成方式**：**Full Context**。启动时读取 `documents/` 下（业务分类相关的）所有文档，直接拼接放入 Prompt 的 Context 区域。
    *   **优势**：架构极简，无向量库依赖，利用大模型对全文逻辑的理解，避免切片导致的信息破碎。

*   **L3: 外部业务文档 (External Business Docs)**
    *   **定义**：存储在公司 Wiki、Confluence 或其他知识库系统中的文档。
    *   **集成方式**：**在线 API (Online API)**。通过 HTTP 接口实时检索或按需拉取。

*   **L4: 历史排查沉淀 (Historical Archives)**
    *   **定义**: 过去已归档的排查会话记录，包含现象、操作过程与根因结论。
    *   **集成方式**: **RAG / Few-Shot**。作为历史参考案例，供 AI 在类似问题出现时参考。

### 2.2 排查工作流 (Investigation Workflow)
*   **流程定义**:
    1.  **开始排查 (Start Investigation)**: 用户点击侧边栏“开始排查”按钮，系统标记会话开始。
    2.  **AI 建议 (AI Suggestion)**: AI 分析用户输入的**问题现象 (Phenomena)**，结合上下文给出排查思路与命令建议。
    3.  **自动记录 (Auto-record)**: 系统自动记录用户在终端的所有操作（输入命令与关键输出），无需用户手动复制粘贴。
    4.  **结束与根因 (Finish & Root Cause)**: 用户点击“结束排查”，并**手动输入根因 (User Input Root Cause)**。
    5.  **AI 总结 (AI Conclusion)**: AI 结合**会话全过程 (Phenomena/Actions)** 与 **用户输入的根因**，自动草拟排查总结报告。
    6.  **归档 (Archive)**: 用户确认报告后，系统将其**归档 (Archive)**，作为未来 AI 分析的历史参考数据。

### 2.3 交互设计 (Interaction Design)
*   **侧边栏插件 (Sidebar Plugin)**: 
    *   AI 助手以 **侧边栏** 形式常驻界面右侧。
    *   支持 **并排显示 (Split View)**，确保用户在阅读 AI 建议时，无需切换窗口即可在左侧终端执行命令。
    *   侧边栏应包含：对话流、常用命令推荐卡片、会话控制按钮（开始/结束）。

---

## 3. 功能详细设计 (Functional Design)

### 3.1 极简 RAG 架构与工作流

```mermaid
graph TD
    User[用户] --> |1. 开始排查| UI[侧边栏助手]
    UI --> |2. 提问| AI_Service
    
    subgraph "Knowledge Engine (知识引擎)"
        L1[Linux Base Prompt]
        L2[Markdown Loader (本地业务文档)]
        L3[External API Client (业务文档)]
        L4[Historical Archives (历史沉淀)]
        Recorder[Session Recorder (实录)]
    end
    
    AI_Service --> |3. 加载上下文| L1 & L2 & L3 & L4
    AI_Service --> |4. 请求建议| RemoteLLM[内部安全 AI API]
    RemoteLLM --> |5. 返回建议| UI
    
    User --> |6. 执行命令| Terminal
    Terminal --> |7. 自动记录| Recorder
    
    User --> |8. 结束排查| UI
    UI --> |9. 请求总结| AI_Service
    AI_Service --> |10. 生成报告| Recorder
    Recorder --> |11. 归档| Storage
```

### 3.2 模块设计更新

#### A. 知识管理模块 (`pkg/knowledge`)
*   **`KnowledgeLoader`**：
    *   `LoadAll(dir string) (string, error)`：读取目录下所有 MD 文件，拼接为单个字符串。
*   **`ExternalDocProvider` 接口**：
    *   `Fetch(query string)`：对接外部 API。

#### B. 会话实录模块 (`pkg/session_recorder`)
*   **数据结构**：
    ```go
    type TroubleshootingSession struct {
        ID        string
        StartTime time.Time
        EndTime   time.Time
        Problem   string
        Context   []string // 引用文档列表
        Timeline  []TimelineEvent // 包含 UserQuery, AISuggestion, TerminalAction
        RootCause string          // 用户输入的根因
        Conclusion string         // AI 生成的总结
    }
    ```
*   **持久化**：JSON 文件存储。

#### C. AI 交互模块 (`pkg/ai`)
*   **Prompt 模板升级**：
    ```text
    [Role]
    你是一个精通 Linux 和业务排查的运维专家。

    [Context - Local Business Docs]
    {{local_docs_content}}

    [Context - Historical Cases]
    {{historical_cases}}

    [Instruction]
    1. 分析用户问题。
    2. 优先基于 Context 内容给出排查步骤。
    3. 输出可执行的 JSON 命令块。
    ```

---

## 4. 任务拆解与子迭代计划

为了确保开发过程逻辑清晰且能逐步交付验收，我们将 Iteration 5 拆分为 5 个详细的子迭代。

### **v0.5.1: 知识引擎核心 (Backend - Knowledge Engine)**
**目标**: 构建“多源知识融合”的基础能力，使系统能理解本地文档。
*   **任务**:
    1.  **实现 Markdown Loader**:
        *   创建 `pkg/knowledge` 包。
        *   开发 `LoadAll(dir string)` 函数，读取 `documents/` 目录下所有 `.md` 文件并拼接。
    2.  **实现 Prompt Assembler**:
        *   在 `pkg/ai` 中升级 Prompt 构建逻辑。
        *   实现 `L1 (Linux Base)` + `L2 (Local Docs)` 的上下文拼接逻辑。
*   **验收标准**: 后端单元测试通过，能正确输出包含本地文档内容的完整 Prompt 字符串。

### **v0.5.2: 会话实录底层 (Backend - Session Recorder)**
**目标**: 构建“实战闭环”的数据基础设施，支持会话的记录与持久化。
*   **任务**:
    1.  **定义数据结构**:
        *   实现 `TroubleshootingSession` 结构体（包含 ID, Timeline, RootCause 等）。
    2.  **实现生命周期管理**:
        *   开发 `StartSession()`, `StopSession()`, `AddEvent()` 接口。
        *   开发 `SaveSession()` 接口，支持将会话序列化为 JSON 文件存储。
*   **验收标准**: 后端单元测试通过，模拟一次会话流程（开始->添加事件->结束），能生成符合规范的 JSON 文件。

### **v0.5.3: 侧边栏 UI 框架 (Frontend - Sidebar UI)**
**目标**: 完成交互界面的重构，支持“并排显示”模式。
*   **任务**:
    1.  **布局管理器升级**:
        *   修改 `LayoutManager`，支持 `Split View` (左侧终端 + 右侧侧边栏)。
        *   增加侧边栏的显示/隐藏控制逻辑。
    2.  **开发 Sidebar 组件**:
        *   实现基础容器 UI。
        *   实现“对话流”区域（Message List）和“输入框”区域。
*   **验收标准**: 启动应用后，侧边栏能正常展开/折叠，不遮挡终端，且界面布局美观。

### **v0.5.4: 智能问答集成 (Full Stack - Chat Integration)**
**目标**: 联调 v0.5.1 和 v0.5.3，实现带业务上下文的智能问答。
*   **任务**:
    1.  **前后端联调**:
        *   前端 Sidebar 发送用户提问给后端 `AI Service`。
        *   后端调用 LLM API 并返回结果。
    2.  **Markdown 渲染**:
        *   前端 Sidebar 支持渲染 AI 返回的 Markdown 格式建议。
*   **验收标准**: 在 `documents/` 放入测试文档，在 Sidebar 提问相关内容，AI 能准确回答（证明上下文注入成功）。

### **v0.5.5: 排查工作流闭环 (Full Stack - Workflow Loop)**
**目标**: 联调 v0.5.2，实现完整的“排查 -> 记录 -> 总结 -> 归档”流程。
*   **任务**:
    1.  **工作流控制集成**:
        *   前端“开始排查”按钮绑定后端 `session_recorder.StartSession`。
        *   前端“结束排查”按钮触发“根因输入弹窗”，提交后调用 `StopSession`。
    2.  **终端数据捕获**:
        *   将 Terminal 的输入输出流实时 Hook 到 `session_recorder.AddEvent`。
    3.  **AI 总结与归档**:
        *   在 `StopSession` 流程中，自动触发 AI 生成总结报告。
*   **验收标准**: 完成一次完整排查操作，检查本地生成的归档文件，内容包含：用户提问、AI 建议、终端执行的命令、用户输入的根因、AI 生成的总结。

---

## 5. 验收标准 (Acceptance Criteria)

*   [ ] **上下文注入**：
    *   在 `documents/` 下放入“业务概念”和“排查手册”文档。
    *   提问相关问题，AI 能准确结合两者回答。
*   [ ] **侧边栏体验**：
    *   打开侧边栏不影响终端输入。
    *   能流畅进行多轮对话，且 UI 不遮挡。
*   [ ] **排查实录**：
    *   点击“开始”，执行一系列操作，点击“结束”。
    *   弹出框输入根因结论。
    *   系统生成包含“AI 建议”、“用户操作”、“根因”、“AI 总结”的完整报告并归档。
