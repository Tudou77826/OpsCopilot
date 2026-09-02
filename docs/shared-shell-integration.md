# Shared Shell：代码归属与开发入口

OpsCopilot 保持独立桌面产品和 CLI；Shell 产品组件由桌面端与后续平台插件共享。平台入口、任务管理、工作流管理和通用知识管理不由 OpsCopilot 接管。

## 代码归属

| 位置 | 职责 |
| --- | --- |
| `frontend-shell/src/ui` | 共享终端、分屏、连接、快捷命令、文件、脚本、设置与运维诊断 UI，通过 `ports.ts` 接收宿主能力 |
| `frontend/src/shell-adapter` | Wails 宿主适配器；桌面旧组件路径作为兼容入口，委托共享 UI |
| `frontend-shell/src/core`、`src/adapters/sidecar` | Sidecar 控制客户端、PTY 数据通道与宿主适配器 |
| `internal/shellsidecar`、`cmd/shellsidecar` | 独立本地 Shell 服务，复用 Ops 现有 Go 能力 |
| `frontend-shell/src/dev` | 无平台时的联调入口，不是 Teams 平台或最终插件 |
| `App.go`、`main.go`、`frontend/src/App.tsx` | 原有 Ops 桌面与 CLI 入口，本次不改造为工作台 |

共享 UI 不得直接调用 Wails、Sidecar RPC 或依赖实验工作台。新增宿主通过端口适配；同一产品交互不再复制到各宿主中。快捷命令保留 main 的分组选择、拖动排序和面板尺寸交互，Wails 与 Sidecar 均支持排序持久化。

Sidecar 使用 stdio JSON-RPC 控制面及本地 WebSocket PTY 数据面；`--dev` 才额外开放 WebSocket `/rpc`。平台宿主应管理进程生命周期、随机 token、能力门控及独立数据目录。此服务不是可直接公开部署的多租户后端，不应监听公网。Shell 诊断所需的现有知识能力保留，但不把通用知识管理平台并入 Shell。

## 本地开发

仓库根目录安装两端依赖：

```powershell
npm --prefix frontend-shell ci
npm --prefix frontend ci
```

桌面开发仍使用 `wails dev`，发布仍使用 `build_release.bat`。`wails.json` 的安装步骤已包含共享包依赖。

无平台联调分别运行以下命令：

```powershell
$shellToken = [guid]::NewGuid().ToString('N')
$shellToken
go run ./cmd/shellsidecar --dev --ws-addr 127.0.0.1:9777 --token $shellToken --data-dir ./frontend-shell/data
npm --prefix frontend-shell run dev
```

在第二个终端运行 Vite 命令，打开其输出的地址，将第一个终端显示的 `$shellToken` 填入控制面 URL：`ws://127.0.0.1:9777/rpc?token=<token>`。连接后通过 `initialize` 获得实际数据面地址和 token；宿主显式传入的端点配置优先。不要复用真实桌面配置目录进行联调。

## 质量检查

```powershell
npm --prefix frontend-shell run typecheck
npm --prefix frontend-shell test -- --maxWorkers=2
npm --prefix frontend test -- --run --maxWorkers=2
npm --prefix frontend-shell run build
npm --prefix frontend run build
go test ./...
go vet ./...
go build ./cmd/shellsidecar
```

`Shared Shell checks` 工作流对 PR 和 main 运行以上检查。桌面完整发布构建仍由原发布工作流负责。运行现有 Go 全套测试可能改变 `pkg/config/config.json`，建议在干净 worktree 验证，提交前检查配置与生成绑定差异，不覆盖开发者已有修改。

## 本次回收范围

2026-09-02 从 `codex/workbench-architecture` 的 `1cfc437f48a9743416db9057c3f203a03450e622` 按目录迁移，在 main 原有 `aef206d7139daf46da3f1ce09255ce07554e4026` 上适配；不是整分支合并。main 原有快捷命令和更新器演进继续保留。

- 已回收：共享 Shell UI、Wails 适配器、Sidecar 服务与适配器、隔离 SSH 测试工具和相关测试。
- 未回收：Workbench 导航和总入口、ProjectSession、任务/工作流领域、实验数据库及 DSH 专用插件。原分支保留用于追溯，不再作为第二套 Ops 主线推进。
- 未实现：Teams 插件、Teams 宿主集成验收，以及独立任务/工作流/知识插件。本次代码通过端口为这些后续工作提供基础。

本地回退锚点为 `checkpoint/ops-main-pre-shell-20260902`；源分支归档标签为 `archive/workbench-pre-teams-20260902`。运行数据、未提交文件及补丁另存于迁移时的本地 recovery 目录，不进入 Git。保留源 worktree，待后续插件迁移明确结束后再决定是否清理。

### 迁移验证记录（2026-09-02）

- 共享包：类型检查、16 个测试文件共 93 项测试、生产构建通过。
- 桌面前端：173 项测试及生产构建通过。
- Go：`go test ./...`、`go vet ./...` 通过。
- Windows：`wails build -f -m -nosyncgomod -nocolour` 完成依赖安装、绑定生成、前端编译和可执行文件构建；保留原有 go.mod，不同步或 tidy 模块。
- 浏览器联调：隔离 Sidecar 与本地测试 SSH 的连接、终端回显、快捷命令保存与执行、主题切换、断开连接通过；没有连接真实服务器或调用真实模型。
- 构建仍有现有 Wails 类型生成提示和前端包体积提示；未覆盖真实 Teams 宿主、发布签名及真实运维环境验收。
