# OpsCopilot 架构术语表

> 关联文档：[OpsCopilot 综合工程工作台架构演进计划](architecture-evolution-plan.md)

## 1. 用途

本术语表用于评审 OpsCopilot 架构演进中的概念边界和中英文命名。评审通过后，产品文案、设计文档、Go 类型、TypeScript 类型、数据库字段和事件名称应遵循同一套含义。

术语分为三层：

| 层级 | 使用范围 | 规则 |
|---|---|---|
| 产品核心术语 | 产品、设计、研发、用户均可能看到 | 数量必须少，含义稳定，优先使用中文名称 |
| 架构与实现术语 | 设计文档、代码、日志和接口 | 可以更精确，不应强迫用户理解 |
| 禁用或待替换术语 | 历史设计或容易产生歧义的名称 | 不进入新接口和产品文案 |

状态说明：

- **建议确认**：建议作为正式术语冻结。
- **仅实现使用**：可以进入代码，不作为产品核心概念宣传。
- **建议改名**：含义有价值，但当前名称容易误解。
- **停止使用**：不应成为新架构中的独立概念。

## 2. 一句话架构定义

> 人、Agent 和 Workflow 在同一个 Workspace 中，共享工程资源、工程资产和标准能力；需要持续跟踪的工作可以组织为 WorkItem。

这句话只依赖六个产品核心术语：Workspace、Resource、Asset、Capability、WorkItem、Artifact。

## 3. 产品核心术语

| 英文名称 | 建议中文名称 | 精确定义 | 不表示什么 | 状态 |
|---|---|---|---|---|
| Workspace | 工作空间 | 一个长期存在的工程环境边界，聚合可用的资源、资产、权限和运行配置 | 不等同于本地目录、代码仓库、任务或 Agent 临时环境 | 建议确认 |
| Resource | 工程资源 | 可以被查看、连接或操作的目标，例如服务器、集群、数据库、代码仓库和外部系统 | 不是知识文档、脚本或运行产物 | 建议确认 |
| Asset | 工程资产 | 可以反复检索、维护和复用的内容或定义，例如知识、经验、Runbook、命令模板和策略 | 不是被操作的服务器，也不是一次运行产生的日志或 diff | 建议确认 |
| Capability | 能力 | 对一种工程操作的标准化契约，可被人、Agent 和 Workflow 共同使用 | 不是一次具体执行，也不等同于插件或页面按钮 | 建议确认 |
| WorkItem | 工作项 | 可选的持续工作容器，用于描述目标、状态和责任，并关联运行、产物、审批与结论 | 不是打开工作台或终端的前置条件，也不负责实际执行 | 建议确认 |
| Artifact | 产物 | 某次操作或运行生成、修改或收集的结果，例如日志、diff、测试报告、导出文件和诊断报告 | 不是可复用资产；经过审核后可以转化为 Asset | 建议确认 |

### 3.1 核心对象关系

```text
Workspace
├── 包含 Resource
├── 包含 Asset
├── 提供 Capability
├── 产生 Run 和 Artifact
└── 包含可选 WorkItem
       └── 关联 Run、Artifact、审批和结论
```

### 3.2 三组最容易混淆的概念

#### Workspace、工作目录和工作副本

| 名称 | 回答的问题 | 示例 |
|---|---|---|
| Workspace | 当前处于哪个长期工程环境 | 支付平台工作空间 |
| WorkingDirectory | 这一次 Agent 从哪个目录执行命令 | `D:\dev\payment-api` |
| WorkingCopy | Agent 实际读取和修改的那份代码副本 | 为 AgentRun 创建的 Git Worktree |

Workspace 可以没有本地目录；一个 Workspace 也可以包含多个代码仓库。WorkingDirectory 和 WorkingCopy 只描述一次执行的现场。

#### Resource、Asset 和 Artifact

| 名称 | 判断方法 | 示例 |
|---|---|---|
| Resource | 它是否是要连接、查看或操作的目标 | 服务器、Kubernetes 集群、Git 仓库 |
| Asset | 它是否是经过维护、准备反复复用的内容或定义 | 知识、Runbook、命令模板 |
| Artifact | 它是否是某次工作生成或收集的结果 | 日志包、代码 diff、测试报告 |

Artifact 经过人工确认和整理后，可以转化为 Knowledge、Experience 或 Runbook Asset，但不能自动视为资产。

#### Capability、Invocation 和 Run

| 名称 | 判断方法 | 示例 |
|---|---|---|
| Capability | 系统长期具备什么能力 | SSH 命令执行、文件下载、知识检索 |
| Invocation | 某个能力被调用了一次 | 在服务器 A 执行 `systemctl status` |
| Run | Agent 或 Workflow 的一次完整运行 | Agent 完成一次代码修复 |

Capability 是定义，Invocation 和 Run 是运行时实例。

## 4. 执行与运行术语

| 英文名称 | 建议中文名称 | 定义 | 使用层级 | 状态 |
|---|---|---|---|---|
| Human | 人工执行者 | 直接使用终端、文件、知识等能力的用户 | 产品与实现 | 建议确认 |
| Agent | Agent / 智能体 | 根据目标和上下文自主选择能力并迭代执行的执行者 | 产品与实现 | 建议确认 |
| AgentRun | Agent 运行 | Agent 针对一个目标的一次运行实例 | 产品可见、实现精确 | 建议确认 |
| Workflow | 工作流 | 由固定或可配置步骤组成的可重复执行定义 | 产品与实现 | 建议确认 |
| WorkflowRun | 工作流运行 | 某个 Workflow 的一次执行实例 | 产品可见、实现精确 | 建议确认 |
| StepRun | 步骤运行 | Workflow 中某个步骤的一次执行记录 | 实现 | 仅实现使用 |
| Invocation | 能力调用 | 某个调用主体对 Capability 发起的一次具体调用 | 实现 | 建议确认 |
| TerminalSession | 终端会话 | 一次交互式终端连接，包含双向流、窗口状态和连接生命周期 | 产品与实现 | 建议确认 |
| ExecutionContext | 执行上下文 | 一次 AgentRun 或 WorkflowRun 的工作目录、环境、权限、进程和关联标识集合 | 实现 | 仅实现使用 |
| WorkingDirectory | 工作目录 | 本次执行命令和解析相对路径所使用的主目录 | 产品可配置、实现精确 | 建议确认 |
| WorkingCopy | 工作副本 | Agent 实际读取和修改的文件集合，可以是已有目录或隔离副本 | 实现 | 仅实现使用 |
| ManagedWorktree | 受管工作树 | 由系统创建、登记并负责安全清理的 Git Worktree | 实现 | 仅实现使用 |
| Artifact | 产物 | Run 或 Invocation 生成、修改或收集的结果 | 产品与实现 | 建议确认 |

### 4.1 Agent 与 AgentRun

`Agent` 表示执行机制或配置，`AgentRun` 表示一次真实执行。不能用 Agent 同时表示二者。

```text
Agent：代码修复 Agent
AgentRun #1：修复支付接口超时，失败
AgentRun #2：基于新提示重新执行，成功
```

### 4.2 Workflow 与 WorkflowRun

`Workflow` 是可重复使用的流程定义，`WorkflowRun` 是一次实际运行。流程编辑器修改的是 Workflow，进度页面展示的是 WorkflowRun。

## 5. 能力体系术语

| 英文名称 | 建议中文名称 | 定义 | 状态 |
|---|---|---|---|
| Capability Gateway | 能力网关 | 统一完成能力发现、授权、调用、进度、取消、错误和审计的应用层入口 | 仅实现使用 |
| Invoke | 调用 | 执行一次有明确输入和结果的 Capability | 仅作为接口方法 |
| OpenSession | 打开会话 | 打开一次双向流式 Capability，例如 SSH/Telnet 终端 | 仅作为接口方法 |
| Provider | 能力实现 | Capability 的底层实现，例如 SSH Provider、Git Provider | 仅实现使用 |
| Tool Adapter | 工具适配器 | 将 Capability 的描述和调用协议转换为 LLM Tool 协议 | 仅实现使用 |
| Connector | 连接器 | 对接外部系统事件和 API 的集成组件，例如 Issue、告警和工单连接器 | 产品与实现 |
| Policy | 策略 | 对路径、命令、目标、凭据和审批要求进行判断的规则 | 产品与实现 |
| Approval | 审批 | 高风险或受控操作执行前需要满足的人为授权 | 产品与实现 |

### 5.1 Capability 与 Tool

- Capability 是产品内部唯一的能力实现契约。
- Tool 是提供给 LLM 的协议视图。
- Tool Adapter 可以把合适的 Capability 暴露给 Agent。
- 交互式终端不能为了适配 Tool 而退化成一次性调用。

### 5.2 Provider 与 Connector

- Provider 向内实现能力，例如用 SSH 协议实现命令执行。
- Connector 向外连接系统，例如从告警平台接收事件并回写结果。
- 一个 Connector 可以调用多个 Capability；一个 Capability 也可以选择不同 Provider。

## 6. 工程资产术语

| 英文名称 | 建议中文名称 | 定义 | 当前能力映射 | 状态 |
|---|---|---|---|---|
| Knowledge | 知识条目 | 经过维护的事实、说明、架构、配置或故障资料 | Markdown 知识库 | 建议确认 |
| Experience | 经验条目 | 从历史工作中提炼的症状、假设、检查、结论和处理经验 | 故障结论与定位经验 | 建议确认，但需确认结构范围 |
| Runbook | Runbook / 操作手册 | 可重复使用的操作步骤，可包含说明、变量、人工步骤和可执行命令 | Script 及排障 SOP | 建议确认 UI 中文名 |
| CommandTemplate | 命令模板 | 带说明和可选参数的单条命令或短命令组合 | 快捷命令 | 建议替换 Command Asset |
| Policy | 策略 | 可复用的权限、安全和审批规则 | 命令白名单、文件访问规则 | 建议确认 |
| AssetCatalog | 资产目录 | 记录资产元数据、位置、版本、作用域和索引的目录服务 | Knowledge Catalog 等 | 仅实现使用 |
| ContextResolver | 上下文匹配器 | 根据当前 Workspace、Resource、输入和可选 WorkItem 匹配相关资产 | 现有知识检索的扩展 | 仅实现使用 |

### 6.1 Knowledge、Experience 与 Runbook

```text
Knowledge：说明“是什么、为什么”
Experience：说明“过去遇到过什么、如何判断”
Runbook：说明“按照什么步骤执行”
```

三者可以引用彼此，但不应合并为一个笼统的“知识”类型。是否需要将 Experience 独立建模，应以检索、推荐和沉淀流程能否体现差异为判断标准。

## 7. 记录与审计术语

| 英文名称 | 建议中文名称 | 定义 | 是否依赖 WorkItem | 状态 |
|---|---|---|---|---|
| RuntimeState | 运行状态 | 支持 AgentRun、WorkflowRun 恢复、取消和故障解释的内部状态 | 否 | 仅实现使用 |
| WorkRecord | 工作记录 | 面向用户展示的处理过程、备注和关键结果 | 可选关联 | 建议确认 |
| SecurityAudit | 安全审计 | 强制记录高风险调用的主体、目标、时间、结果和审批 | 否 | 建议确认 |
| Recording | 录制 | 用户显式开启的终端输入输出采集过程 | 可选关联 | 建议沿用现有名称 |

运行状态、工作记录和安全审计的保留周期与访问权限不同，不能共用一个模糊的 Activity 表或录制文件解决全部需求。

## 8. 建议改名或停止使用的术语

| 当前名称 | 问题 | 建议处理 |
|---|---|---|
| Action | 可能表示 Capability、一次调用、工作流节点或 UI 按钮 | 架构中用 Capability 表示能力，用 Invocation 表示一次调用 |
| Action Gateway | 容易暗示只支持一次性动作，不能自然表达交互式终端 | 改为 Capability Gateway |
| Execution Workspace | 与产品核心 Workspace 冲突 | 用 ExecutionContext 表示执行环境，用 WorkingCopy 表示代码现场 |
| Execution Workspace Manager | 同时混入 Workspace 和执行环境两个概念 | 建议改为 WorkingCopyManager；进程和环境由 ExecutionManager 管理 |
| Command Asset | 容易误解为已经执行的命令 | 改为 CommandTemplate |
| Capture Service | 与现有 Recorder/Recording 重复造词 | 改为 RecordingService 或沿用 Recorder |
| Activity | 含义过宽，可能混合运行、工作记录和安全审计 | 不作为核心实体；分别使用 RunRecord、WorkRecord、AuditEvent |
| ActivitySession | 为兼容无 WorkItem 操作而增加的隐藏业务容器 | 停止使用；普通操作直接发生在 Workspace 中 |
| Session | 单独使用时无法判断是终端、登录、录制还是应用会话 | 必须使用限定名称，例如 TerminalSession |
| Task | 容易同时指 WorkItem、AgentRun、Workflow Step 或后台任务 | 产品层使用 WorkItem；实现层使用明确的 Run/Job/Step 名称 |

## 9. 建议冻结的中英文命名

以下命名适合作为第一轮评审结论：

| 代码与文档名称 | 中文产品名称 | 建议 |
|---|---|---|
| Workspace | 工作空间 | 保留；必须反复强调不等同于本地目录 |
| Resource | 工程资源 | 保留；UI 上下文明确时可简称“资源” |
| Asset | 工程资产 | 保留；避免单独使用过于宽泛的“资产” |
| Capability | 能力 | 保留 |
| WorkItem | 工作项 | 保留；不建议简化为“任务” |
| Artifact | 产物 | 保留 |
| Agent | Agent | 产品优先使用 Agent，解释性文案可写“智能体” |
| AgentRun | Agent 运行 | 保留 |
| Workflow | 工作流 | 保留 |
| WorkflowRun | 工作流运行 | 保留 |
| Runbook | Runbook / 操作手册 | 需要产品确认最终 UI 中文名 |
| Experience | 经验条目 | 需要确认是否作为独立资产类型 |
| CommandTemplate | 命令模板 | 建议采用 |
| TerminalSession | 终端会话 | 保留 |
| WorkingDirectory | 工作目录 | 保留 |

## 10. 需要评审确认的问题

1. 中文产品名称使用“工作空间”还是“工程空间”？建议使用行业更通用的“工作空间”。
2. WorkItem 是否统一显示为“工作项”？建议不使用容易与后台任务混淆的“任务”。
3. Runbook 的 UI 名称使用“Runbook”“操作手册”还是“运行手册”？建议首次出现使用“Runbook（操作手册）”，熟悉后简称 Runbook。
4. Experience 是否值得成为独立资产类型？如果只是一篇 Markdown，应先作为 Knowledge 的标签；如果具有症状、假设、检查、结论结构，则独立建模。
5. 产品界面是否直接展示“能力”？建议只在能力管理和 Workflow 编辑场景展示，日常终端操作不暴露该术语。
6. “工程资源”和“工程资产”的中文是否足够容易区分？如果用户测试中仍混淆，UI 应使用具体类型名称，例如“服务器”“知识”“Runbook”，不强迫用户理解抽象父类。

## 11. 命名约束

1. 一个英文名称只对应一个架构含义；不能用 Workspace 同时表示产品空间和本地目录。
2. 用户界面优先显示具体对象名称，抽象父类主要用于导航、筛选和架构设计。
3. 类型名称使用单数，集合与列表通过字段名表达，例如 `[]WorkItem`、`workItems`。
4. 一次运行使用 `Run` 后缀，一次交互连接使用 `Session` 后缀，一次能力调用使用 `Invocation`。
5. `ID` 字段使用完整限定名，例如 `workspaceId`、`workItemId`、`agentRunId`，不使用含义不明的 `contextId`。
6. WorkItem 关联在运行和调用中保持可选，不通过命名暗示其为必需父对象。
7. 新术语进入代码前，必须能回答：它负责什么、不负责什么、由谁创建、何时结束、与现有术语有何不同。
