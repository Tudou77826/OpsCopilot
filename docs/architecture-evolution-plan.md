# OpsCopilot 综合工程工作台架构演进计划

> 关联文档：[OpsCopilot 架构术语表](architecture-glossary.md)

## 1. 执行摘要

OpsCopilot 将从“以终端为主要入口、能力集中在 App 中的智能运维助手”，演进为“以 Workspace 为内核的综合工程工作台”。

新架构只保留一条核心主线：

> 人、Agent 和 Workflow 在同一个 Workspace 中，共享终端、代码、文件、知识、脚本和外部系统等工程能力。

WorkItem 不是进入工作台的前置条件，而是可选的工作组织能力。用户可以直接打开终端、浏览文件或查询知识；当工作需要跨时段跟踪、协作、审批或沉淀时，再创建 WorkItem 并关联已有运行记录和产物。

本次演进采用**模块化单体、增量迁移、兼容旧接口**的策略，不重写成熟的 SSH、终端、文件传输、知识库和脚本实现。建议以 4～6 人团队投入 28～36 周完成主线建设；前 14～17 周交付第一个 Workspace 能力闭环，后续再完善 Agent、WorkItem 和 Workflow。

## 2. 产品与架构内核

### 2.1 四个核心概念

```text
Workspace
├── 工作现场：本地目录、代码仓库、远程服务器
├── 工程资产：知识、经验、脚本、Runbook、策略
├── 工程能力：终端、SSH、文件、Git、Shell、外部 API
├── 执行方式：Human、Agent、Workflow
└── WorkItem：可选的目标、协作和追踪容器
```

| 概念 | 定义 | 是否必须 |
|---|---|---|
| Workspace | 当前工程环境及其可用资源、资产和权限 | 必须，应用始终有一个当前 Workspace |
| Capability | 可被人、Agent、Workflow 共同使用的标准能力 | 必须 |
| Human / Agent / Workflow | 使用能力完成工作的三种执行方式 | 按需 |
| WorkItem | 将目标、运行记录、产物和结论组织为可持续管理的工作 | 可选 |

### 2.2 三种自然工作方式

#### 快速操作

```text
打开 Workspace → 打开 SSH 终端 → 操作服务器
```

不创建 WorkItem，不要求填写任务信息，也不引入隐藏的业务会话对象。

#### Agent 工程任务

```text
打开 Workspace → 选择本地工作目录 → 启动 Agent → 修改与验证
```

AgentRun 必须绑定明确的工作目录；是否创建 WorkItem 由用户和场景决定。

#### 持续跟踪的工作

```text
创建或接收 WorkItem
→ 关联终端、AgentRun、WorkflowRun 和产物
→ 协作、审批、交付与沉淀
```

外部工单、告警和 Issue 通常直接创建 WorkItem；临时操作可在需要时将已有记录关联到新 WorkItem。

### 2.3 必须避免的复杂化

- 不为每次打开终端自动创建 WorkItem。
- 不增加用户不可见的 ActivitySession 作为平行业务主线。
- 不要求所有操作都进入任务时间线；安全审计与工作记录分别处理。
- 不让终端拥有知识和脚本；它们属于 Workspace，并按当前上下文呈现。
- 不让 WorkItem 拥有执行能力；它只引用运行记录和产物。
- 不把本机绝对路径作为可跨机器恢复的业务身份。

## 3. 目标架构

```text
┌───────────────────────────────────────────────────────────────┐
│ Workspace                                                     │
│                                                               │
│  Resources      Assets             Runs                       │
│  本地目录        Knowledge          TerminalSession            │
│  代码仓库        Experience         AgentRun                    │
│  远程服务器      Runbook / Script   WorkflowRun                 │
│  外部系统        Policy             Artifact                    │
│       │              │                 │                        │
│       └──────────────┴──── Context ────┘                        │
│                              │                                 │
│  Human ──────────────────────┤                                 │
│  Agent ──────────────────────┼── Capability Gateway            │
│  Workflow ───────────────────┤      ├── Terminal / SSH         │
│                              │      ├── File                   │
│                              │      ├── Local Shell / Git      │
│                              │      ├── Knowledge / Runbook    │
│                              │      └── Connector              │
│                              │                                 │
│  WorkItem（可选）◀──关联目标、Runs、Artifacts、结论与审批       │
└───────────────────────────────────────────────────────────────┘
```

### 3.1 Workspace

Workspace 是产品内核，表示当前工程环境，而不等同于一个本地目录。它可以同时包含：

- 零到多个本地目录或代码仓库；
- 零到多个远程服务器和保存的连接；
- 知识库、经验、脚本、Runbook 和策略；
- 当前可用的 Agent、Workflow 和连接器配置。

应用首次运行时可以自动打开默认 Workspace，用户无需先完成配置。纯运维用户即使没有本地代码目录，也可以直接使用远程终端。

### 3.2 Capability Gateway

Capability 表示可复用能力，Invocation 表示一次具体调用。Gateway 统一负责发现、授权、调用、进度、取消、结果和审计，但不能牺牲交互式终端体验。

首期支持两种调用形态：

| 形态 | 适用能力 | 特征 |
|---|---|---|
| Invoke | 执行命令、文件操作、知识检索、脚本、Git | 有明确输入和结果，可超时、取消和审计 |
| OpenSession | SSH/Telnet 交互终端、本地 Shell | 长连接、双向流、窗口尺寸和原生控制序列 |

每次调用都携带 Workspace、调用主体、目标资源和权限上下文；WorkItemID、AgentRunID、WorkflowRunID 均为可选关联。

### 3.3 工程资产

知识、脚本和定位经验作为独立工程资产存在：

| 现有能力 | 新架构定位 | 使用者 |
|---|---|---|
| Markdown 知识库 | Knowledge Asset | 人、Agent、Workflow |
| 历史定位经验 | Experience Asset | 人、Agent、Workflow |
| 录制脚本 | Runbook Asset + 可执行 Capability | 人、Agent、Workflow |
| 快捷命令 | CommandTemplate | 人，必要时 Agent |
| 命令白名单与文件规则 | Policy Asset | Gateway 统一执行 |

Context Resolver 根据当前 Workspace、选中资源、服务、环境、用户输入以及可选 WorkItem，筛选相关资产。终端侧栏可以显示匹配结果，但终端不是资产的所有者。

### 3.4 Agent Runtime 与本地工作目录

AgentRun 直接属于 Workspace，并绑定 ExecutionContext：

```text
AgentRun
└── ExecutionContext
    ├── workingDirectory：本次运行的主工作目录
    ├── repository + baseRevision：可重建身份
    ├── mode：existing-directory / managed-worktree
    ├── environment：工具链与环境变量引用
    ├── allowedPaths：允许访问的路径
    └── terminal/process：本次运行启动的进程
```

简单任务可以使用用户选择的现有目录；需要并行、隔离或安全回退时，默认创建受管 Git Worktree。Agent 的修改、diff、测试结果和日志属于 AgentRun 的 Artifact，可选关联到 WorkItem。

### 3.5 WorkItem

WorkItem 只承担三项职责：

1. 描述目标、状态、负责人和来源；
2. 关联 TerminalSession、AgentRun、WorkflowRun、Artifact 和审批；
3. 汇总过程、结论和可沉淀资产。

WorkItem 不承载终端连接、Agent 执行或能力注册。其时间线由已关联记录投影生成，不要求全部 Workspace 操作先经过 WorkItem。

### 3.6 运行记录、工作记录与安全审计

三类记录必须分开：

| 记录 | 用途 | 保存策略 |
|---|---|---|
| Runtime State | 支持 Agent/Workflow 恢复和取消 | 按运行生命周期保存 |
| Work Record | 向用户展示处理过程，可关联 WorkItem | 由用户或产品规则决定 |
| Security Audit | 记录高风险调用、目标、主体和审批 | 按安全策略强制保存 |

不能为了生成 WorkItem 时间线而默认永久保存全部终端原始输入输出。终端录制应保持显式开启，并遵守脱敏和保留策略。

## 4. 当前架构基线

现有代码已经提供了可迁移的能力基础：

- [`app.go`](../app.go) 集成终端、AI、录制、脚本、安全和文件传输，同时承担编排、状态和 Wails API 职责，是首要解耦对象。
- [`pkg/tools`](../pkg/tools) 已有 Tool 接口和注册器，可复用于 Capability 描述与 AI 工具转换，但不能直接覆盖交互式终端契约。
- [`pkg/ai/agent.go`](../pkg/ai/agent.go) 已实现知识检索型 ReAct 循环，但缺少持久化 AgentRun、工作目录和执行权限上下文。
- [`pkg/recorder`](../pkg/recorder) 已能记录命令与时间线，但只有一个全局 `current` 录制对象，应演进为按运行独立的显式 Capture 能力。
- [`pkg/script`](../pkg/script) 已支持录制、变量和回放，可兼容演进为 Runbook Asset 与可执行 Capability。
- [`pkg/core/ops`](../pkg/core/ops) 已抽象远程执行和文件传输，适合作为 SSH/File Provider。
- [`frontend/src/App.tsx`](../frontend/src/App.tsx) 和多个组件直接调用 `window.go.main.App`，需要稳定客户端层隔离 Wails 门面。

演进重点是重新组织现有能力，补齐 Workspace、统一能力调用、Agent 执行现场和可选工作管理，不重写底层协议。

## 5. 分阶段实施路线

| 阶段 | 周期 | 目标 | 阶段出口 |
|---|---:|---|---|
| 0. 决策基线与迁移护栏 | 2 周 | 冻结核心模型和回归基线 | 核心 ADR 通过，旧功能可回归和回退 |
| 1. Workspace 内核与应用边界 | 3～4 周 | 建立唯一常驻上下文 | 终端可在默认 Workspace 中直接使用 |
| 2. Capability Gateway | 4～5 周 | 统一现有能力调用 | UI 与测试 Agent 共享首批 Capability |
| 3. 资产与上下文复用 | 3～4 周 | 解开知识、经验、脚本与终端绑定 | 同一资产可被人和 Agent 复用 |
| 4. Agent Runtime 与执行现场 | 5～6 周 | 支持绑定工作目录的工程 Agent | Agent 可在受控目录中修改并验证代码 |
| 5. 可选 WorkItem 与工作记录 | 3～4 周 | 支持复杂工作的持续管理 | 无任务终端与任务化排障均可自然完成 |
| 6. Workflow、审批与连接器 | 5～6 周 | 支持可恢复的多步自动化 | 一个真实外部诉求闭环跑通 |
| 7. 工作台体验收敛与旧架构退场 | 3～5 周 | 迁移默认入口并清理重复实现 | `App` 退化为生命周期与 API 门面 |

### 阶段 0：决策基线与迁移护栏

**交付物**

- 领域术语与关系：Workspace、Resource、Asset、Capability、Run、WorkItem。
- 架构决策记录：模块化单体、Capability 两种调用形态、SQLite 使用边界、Agent 工作目录策略、兼容迁移策略。
- 六条关键回归链路：终端连接与交互、命令执行、文件传输、知识检索、故障录制、脚本录制与回放。
- 代码治理规则：新领域逻辑不再加入 `app.go`；前端新增业务调用不直接散落使用 `window.go.main.App`。
- 功能开关、数据备份和失败回退机制。

**验收门槛**

- 产品、前端和后端能够用同一张模型图解释新架构。
- 明确确认“打开终端不创建 WorkItem”和“AgentRun 必须绑定工作目录”。
- 当前发布版本的关键链路具备自动或半自动回归能力。

### 阶段 1：Workspace 内核与应用边界

**交付物**

```text
internal/workbench/
├── workspace/      Workspace、Resource、Asset 引用
├── store/          SQLite 运行元数据与迁移
├── application/    GUI/CLI 共用应用服务
└── api/            Wails 兼容门面
```

- 自动创建和打开默认 Workspace，不增加用户进入成本。
- Workspace 管理本地根目录、代码仓库、远程资源、资产目录和策略引用。
- SQLite 保存 Workspace 元数据和后续运行状态；现有知识正文与脚本文件不搬入数据库。
- 建立应用服务层，现有 Wails `App` 逐步改为参数转换和调用委托。
- 前端建立 `workbenchClient`，集中封装 Wails 方法和事件订阅。

**验收门槛**

- 用户升级后仍可一键打开原生终端，不需要理解或创建 WorkItem。
- 无本地代码目录的 Workspace 仍可正常使用 SSH 和知识能力。
- 数据库异常时可以降级到传统终端模式。
- GUI 与 CLI 可以调用同一应用服务。

### 阶段 2：Capability Gateway

**交付物**

- Capability 注册、描述、版本、输入输出 Schema 和调用主体模型。
- Invoke 与 OpenSession 两种调用协议。
- 统一权限检查、凭据引用、进度、取消、超时、结构化错误和安全审计。
- 首批 Provider：Knowledge、SSH Exec、Interactive Terminal、File、Script。
- 兼容适配器：旧 `App` 方法调用 Gateway，旧前端行为保持不变。
- Tool Adapter：将合适的 Capability 暴露给 LLM，避免 Tool 和 Capability 双份实现。

**验收门槛**

- xterm.js 的交互、控制序列、Resize 和长连接不因 Gateway 退化。
- 同一个 SSH Exec Capability 可被 UI 和测试 Agent 调用，结果一致。
- Script 回放、文件操作和知识检索不再依赖调用方自行拼接底层组件。
- 高风险调用统一经过策略判断并产生安全审计。

### 阶段 3：资产与上下文复用

**交付物**

- Workspace Asset Catalog：统一管理 Knowledge、Experience、Runbook、Command 和 Policy 元数据。
- Context Resolver：根据 Workspace、选中资源、服务、环境和问题描述推荐资产。
- 终端上下文面板：显示相关知识、经验、脚本和命令，但不改变资产归属。
- Agent Context Provider：向 Agent 提供同一套资产检索能力和引用信息。
- 旧 Markdown、Script JSON 和快捷命令保持原地读取，增加兼容元数据索引。
- 使用反馈：记录推荐、打开、执行和采纳结果，为后续排序提供依据。

**验收场景**

用户直接打开某台服务器终端，无需 WorkItem；系统依据当前 Workspace 和服务器上下文推荐相关知识和 Runbook。用户可以手动执行，测试 Agent 也可以检索和调用同一资产。

**验收门槛**

- 同一份 Knowledge/Runbook 至少可被终端侧栏和 Agent 使用。
- 旧知识和脚本无需人工搬迁即可继续工作。
- 资产离开终端页面后仍可在 Workspace 资产中心被搜索、维护和版本化。
- 推荐结果能够追溯匹配依据和来源。

### 阶段 4：Agent Runtime 与执行现场

**交付物**

```text
internal/workbench/
├── execution/      WorkingDirectory、Worktree、进程与权限
├── agent/          AgentRun、步骤、检查点、取消与恢复
└── capability/providers/
    ├── localshell/
    └── git/
```

- AgentRun 持久化：目标、模型、工作目录、步骤、工具调用、状态、费用和结果。
- WorkingCopyManager：选择已有目录，或创建、锁定、恢复和安全清理受管 Worktree。
- 本地 Shell、文件编辑、代码搜索、Git diff 和测试 Capability。
- 路径允许列表、命令策略、敏感信息遮蔽和危险动作确认。
- Agent 产生的 diff、测试报告和日志保存为 Artifact；WorkItem 关联保持可选。

**验收场景**

用户在 Workspace 中选择代码仓库和目标，启动 Agent；系统创建独立 Worktree，Agent 检索知识、修改代码、执行测试并展示 diff。全过程不要求先创建 WorkItem。

**验收门槛**

- 每个 AgentRun 都有明确 workingDirectory，运行日志能够解释实际操作位置。
- 两个并行 AgentRun 不修改同一工作目录。
- Agent 无法访问允许范围外的路径，失败运行不污染用户原始目录。
- 应用重启后运行记录可恢复，WorkingCopy 可重新绑定或重建。

### 阶段 5：可选 WorkItem 与工作记录

**交付物**

- WorkItem 的创建、状态、负责人、来源、结论和归档。
- 将已有 TerminalSession、AgentRun、Artifact 关联到 WorkItem。
- WorkItem 时间线由关联记录投影生成，不建立第二套执行记录。
- 显式终端录制和 RecordingService，取代全局唯一 Recorder `current`。
- 从外部 Issue/告警创建 WorkItem；从当前 Workspace 选择已有记录创建 WorkItem。
- 从处理结论生成 Knowledge/Experience/Runbook 候选，经人工确认后沉淀。

**验收场景 A：快速操作**

用户打开终端执行简单命令后关闭，全程没有 WorkItem，也没有额外步骤。

**验收场景 B：持续排障**

用户创建“支付接口响应慢”WorkItem，关联两台终端、一次 AgentRun 和下载日志，形成结论并沉淀经验。

**验收门槛**

- 两种场景都使用同一 Workspace 和 Capability，不存在两套产品链路。
- 创建 WorkItem 不复制终端、运行和产物数据，只建立稳定关联。
- 未关联 WorkItem 的操作不会污染 WorkItem 列表和时间线。
- 安全审计不依赖是否创建 WorkItem。

### 阶段 6：Workflow、审批与连接器

**首期节点类型**

- Capability Step：调用标准能力。
- Agent Step：在明确工作目录中运行 Agent。
- Human Approval：审批、补充信息或选择分支。
- Condition：根据结构化结果选择分支。
- Wait/Callback：等待定时条件或外部系统回调。

**交付物**

- WorkflowDefinition、WorkflowRun、StepRun 和持久化状态机。
- 重试、超时、取消、检查点、幂等和人工审批。
- 轻量流程编辑器，只允许受控节点，不建设通用 BPMN 平台。
- Connector 接口，并接入一个真实诉求来源，例如 Webhook、Issue 或告警平台。
- WorkflowRun 直接属于 Workspace，可选关联 WorkItem；外部正式诉求默认创建 WorkItem。

**验收门槛**

- Workflow 在应用重启后可以从安全检查点继续。
- 相同步骤重复触发不会产生不可控副作用。
- 高风险步骤在审批前不会执行，审批进入安全审计和可选工作记录。
- 一个外部诉求能够创建 WorkItem、触发 Workflow 并回写处理结果。

### 阶段 7：工作台体验收敛与旧架构退场

**交付物**

- 默认首页以 Workspace 为中心，直接提供终端、文件、知识、Agent 和资源入口。
- WorkItems、Workflows 和 Assets 是 Workspace 内的功能区，不阻断快速操作。
- 终端保持可独立全屏和多标签使用；上下文面板按需展开。
- 前端散落的 Wails 调用迁移到客户端层和领域 hooks。
- `App` 缩减为生命周期与 API 门面，领域逻辑迁入应用服务。
- 完成旧录制、脚本、会话数据的兼容读取、迁移验证和退役公告。

**验收门槛**

- 新用户无需理解 WorkItem 即可完成连接服务器和执行命令。
- 核心用户可以在同一个 Workspace 中完成排障和代码任务。
- 连续两个稳定版本没有严重迁移或恢复问题后，才关闭旧入口。
- `app.go` 不再新增领域逻辑，核心能力具备独立契约测试。

## 6. 前 90～120 天交付范围

第一个里程碑验证 Workspace 内核，而不是 WorkItem 管理：

1. 默认 Workspace 能够承接现有保存的服务器、知识目录和脚本目录。
2. 用户无需创建任务即可打开交互式终端。
3. Knowledge、SSH Exec、Terminal、File 和 Script 通过 Capability Gateway 提供。
4. 终端侧栏与测试 Agent 可以复用同一套知识、经验和 Runbook。
5. 前端新增调用通过 `workbenchClient`，旧 `App` 方法通过适配器继续工作。
6. 完成一个 Agent 最小纵切：选择本地目录，读取代码，调用知识或 Shell，生成可审查结果。

这个里程碑回答三个架构问题：终端体验是否仍然直接；现有资产是否真正脱离终端复用；Agent 是否能在明确工作目录中安全运行。WorkItem 和 Workflow 在此基础上建设，不再反向决定工作台内核。

## 7. 数据与兼容策略

| 现有数据/能力 | 过渡方式 | 目标形态 |
|---|---|---|
| 保存的 SSH 会话 | 保持原格式，建立 Resource 引用 | Workspace Resource |
| Markdown 知识库 | 不搬迁正文，增加 Catalog 元数据 | Knowledge Asset |
| Script JSON | 原地读取，补充作用域和风险元数据 | Runbook Asset + Capability |
| 快捷命令 | 兼容现有配置，增加个人/Workspace 作用域 | CommandTemplate |
| 故障录制 | 保留读取，改为独立 Capture 实例 | Work Record，可选关联 WorkItem |
| AI 检索记录 | 写入 AgentRun，并保留知识引用 | AgentRun Record |
| 终端会话 | 保持协议与交互，增加 Workspace 归属 | TerminalSession |
| 白名单与文件规则 | 先适配现有配置，再统一策略契约 | Policy Asset |

迁移期间至少跨两个稳定版本保留旧数据读取能力。破坏性迁移必须自动备份、校验结果并提供版本化回退工具。SQLite 用于 Workspace 元数据、Run 和 WorkItem 等运行状态，不替代 Markdown、脚本和代码仓库本身。

## 8. 质量、安全与可观测性

### 8.1 测试重点

- Workspace、AgentRun、WorkItem、WorkflowRun 状态测试。
- Capability 契约测试：授权、输入输出、超时、取消、会话关闭和错误语义。
- 交互终端回归：控制序列、Resize、断线重连、广播和文件传输。
- WorkingDirectory/Worktree 并发隔离、路径越界和安全清理测试。
- 旧数据升级、降级和损坏恢复测试。
- 三条产品旅程：直接终端、Agent 代码任务、WorkItem 工作流。

### 8.2 运行指标

- 打开终端的步骤数、连接成功率和首屏耗时。
- Capability 成功率、耗时、取消率和策略拦截率。
- Knowledge/Runbook 推荐率、使用率和采纳率。
- AgentRun 成功率、人工接管率、测试通过率和成本。
- WorkItem 使用率与完成时长，但不追求所有操作任务化。
- Workflow 失败率、重试率和审批等待时长。
- 数据迁移、恢复和事件积压状态。

### 8.3 安全底线

- 凭据只通过 Keyring/SecretStore 引用，不进入 Prompt、日志和数据库正文。
- Agent、文件 Capability 和本地 Shell 使用同一套路径策略。
- 远程高风险调用记录目标、主体、风险级别和审批，与 WorkItem 是否存在无关。
- Artifact 外部回写前执行敏感信息检查和脱敏。
- 仅清理由 WorkingCopyManager 创建且位于受管根目录的 WorkingCopy。
- 终端原始输入输出默认不永久录制，显式录制时展示状态和保留策略。

## 9. 推进机制

### 9.1 团队建议

- 1 名架构/后端负责人：核心模型、应用边界、迁移和阶段出口。
- 2 名 Go 工程师：Capability、存储、Execution、Agent/Workflow Runtime。
- 1～2 名前端工程师：客户端层、Workspace 壳层和工作台交互。
- 1 名产品/设计负责人：快速操作、Agent 工作和持续任务三类旅程。
- 测试与安全能力从阶段 0 介入。

### 9.2 决策与发布节奏

- 每周一次架构迁移评审，只处理跨模块契约、数据模型和阶段风险。
- 每两周提供可运行增量，所有新链路受功能开关控制。
- 每个阶段达到出口指标后再扩大范围，不以代码合并代替阶段完成。
- 固定验证三类场景：临时终端操作、绑定本地目录的 Agent、外部诉求驱动的 WorkItem。

### 9.3 代码治理

| 区域 | 规则 |
|---|---|
| `app.go` | 只保留生命周期、兼容 API 和依赖组装，不新增领域逻辑 |
| `pkg/tools` | 作为 LLM Tool 适配层，不独立复制 Capability 实现 |
| `internal/workbench` | 承载 Workspace、Capability、Run 和 WorkItem 应用逻辑 |
| 前端组件 | 通过客户端层调用，不直接扩散 Wails 绑定 |
| 数据迁移 | 必须提供样本、校验、备份和回退测试 |

## 10. 主要风险与控制措施

| 风险 | 早期信号 | 控制措施 |
|---|---|---|
| Workspace 变成新的万能对象 | 所有逻辑都直接写入 Workspace 包 | Workspace 只保存边界和引用，能力由独立模块实现 |
| Capability Gateway 破坏终端体验 | 将终端输入建模为一次性 Invocation | 明确 OpenSession 流式契约并建立终端回归基线 |
| WorkItem 再次变成强制入口 | 打开终端时自动出现任务 | 产品验收固定包含“无 WorkItem 快速操作” |
| Tool 与 Capability 重复建设 | 同一个 SSH 能力出现两套实现 | Tool 只做 Schema/协议适配，执行统一委托 Gateway |
| Agent 污染用户目录 | 多个 Agent 共享目录或路径不明确 | 每个 AgentRun 显式 workingDirectory，并默认隔离 Worktree |
| 知识仍然绑定终端 | 资产只能从终端侧栏访问 | Workspace Asset Catalog 和 Agent Context 共用索引 |
| Workflow 范围膨胀 | 开始实现完整 BPMN | 首期限制五类节点和一个真实流程 |
| SQLite 阻断基础功能 | 数据库错误导致无法打开终端 | 传统终端降级模式、备份和恢复演练 |
| 新旧数据长期不一致 | 两个入口展示不同结果 | 双读/双写校验指标和明确退役版本 |

## 11. 项目级成功标准

1. 用户打开 Workspace 后，可以不创建 WorkItem 直接进入终端。
2. 80% 以上核心能力通过 Capability Gateway 提供，且交互终端体验不退化。
3. 同一 Knowledge/Runbook 可以被人、Agent 和 Workflow 中至少两类执行者复用。
4. 每个 AgentRun 都绑定明确 workingDirectory；隔离运行不会污染原始目录。
5. WorkItem 可以关联已有运行和产物，但未任务化操作不会生成任务噪声。
6. AgentRun 和 WorkflowRun 在异常退出后能够恢复到可解释状态。
7. 新领域逻辑不再直接堆入 `app.go`，前端新增调用统一经过客户端层。
8. 旧数据迁移经过真实样本验证，连续两个稳定版本没有严重回退需求。

## 12. 立即启动的第一批事项

1. 评审并冻结“Workspace 是内核、WorkItem 可选”的架构决策。
2. 为直接终端、Agent 本地目录、WorkItem 排障三条旅程建立体验基线。
3. 创建默认 Workspace 和应用服务的最小实现，不改变当前终端入口。
4. 定义 Capability 的 Invoke/OpenSession 契约，并用现有 SSH 实现完成原型。
5. 完成第一个技术纵切：

```text
打开默认 Workspace
→ 直接进入 SSH 终端
→ 从 Workspace 获取相关知识或脚本
→ 不创建 WorkItem
```

该纵切通过后，再引入 Agent workingDirectory。两者稳定后，才建设 WorkItem 和 Workflow；这样能够确保任务管理不会反向绑架工作台的基础体验。
