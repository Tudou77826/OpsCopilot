# 目标
在保持“实验功能：文件传输（SFTP/SCP）默认关闭”的前提下，参考 Xftp 的核心交互模型，把当前 FilesPanel 从“路径输入+表格+任务列表”升级为更完整的文件管理体验，并补齐对应后端能力（可用时走 SFTP，不支持时清晰降级到 SCP/只传输模式）。

# 参考 Xftp 的关键 UX 元素（我们将对齐的点）
- 双面板（本地/远端）+ 地址栏/面包屑 + 上级/刷新/新建/删除/重命名等工具条
- 传输队列（独立区域/底部抽屉），展示进度/速度/剩余时间/状态，可取消、可重试
- 会话/站点管理（已存在会话下拉，增强为“收藏/常用目录/最近”）
- 不支持/受限场景时明确提示并降级（SFTP 不支持→SCP 仅上传/下载；chroot/权限不足→显示原因）

（Xftp 具备标签环境、传输队列、拖拽等特性描述可参考其产品介绍摘要）【WebSearch:3†https://www.dayanzai.me/xftp.html】

# 现状差距盘点（基于当前仓库实现）
- 远端：仅有 List/Stat/Upload/Download/Cancel；mkdir/rm/rename/递归/直编等缺失；SFTP 不支持时 list/stat 不可用。
- 本地：仅支持“手输本地路径”，缺少目录浏览能力。
- 任务：有进度与取消，但缺少统一队列视图（筛选/重试/清理/持久化）。

# 总体设计

## 1) 信息架构与交互（UI/UX）

### 1.1 入口与布局
- 仍挂在右侧 Sidebar 的“文件”Tab（实验开关控制可见）。
- 内部采用“类 Xftp”布局：
  - 顶部：会话选择 + 传输方式徽标（sftp(root)/sftp(login)/scp(login)）+ 快捷动作（刷新/设置）。
  - 中间：左右分栏
    - 左：本地（目录树 + 文件表格）
    - 右：远端（目录树 + 文件表格）
  - 底部：可折叠的“传输队列/历史”抽屉（默认折叠，任务开始自动展开一小段）。

### 1.2 地址栏/导航
- 本地与远端各自有地址栏：支持输入路径、回车跳转、上级、刷新、历史（后退/前进可选）。
- 面包屑：点击可快速进入父目录。

### 1.3 文件表格
- 列：名称（图标+类型）、大小、修改时间、权限（可选）、所有者（可选）。
- 默认排序：目录在前 + 名称排序；支持点击排序。
- 双击目录进入；双击文件默认下载到本地“默认目录”（可配置）或打开“直编”模式（后续）。

### 1.4 传输操作
- 拖拽：本地→远端上传；远端→本地下载（首版可先用“选择+按钮”，后续再做拖拽）。
- 工具条：上传、下载、新建文件夹、删除、重命名。
- 传输队列：展示任务、进度、速度、状态；支持取消、重试、清理已完成。

### 1.5 降级与异常态
- 打开面板时自动调用 `FTCheck`：
  - `sftp(*)`：启用完整远端浏览与操作。
  - `scp(*)`：显示“仅传输模式”（隐藏远端目录树/列表或只显示提示），只提供“输入远端路径下载/输入远端目标路径上传”。
  - `none`：提示原因与运维建议。
- 权限不足：在远端面板顶部显示提示条（例如访问 /root 权限不足，建议切换 root 会话或使用 sudo 导出）。

## 2) 后端能力补齐（对应功能）

### 2.1 本地文件系统 API（Wails 导出）
新增一组本地文件系统方法（避免依赖浏览器文件选择 API）：
- `LocalList(path) -> entries`、`LocalStat(path)`
- `LocalMkdir(path)`、`LocalRemove(path)`、`LocalRename(old,new)`
- 可选：`LocalOpenInExplorer(path)`（便于定位）

### 2.2 远端文件管理（SFTP 优先）
新增远端管理 API（SFTP 可用时生效）：
- `FTRemoteMkdir(sessionId, path)`
- `FTRemoteRemove(sessionId, path, recursive?)`
- `FTRemoteRename(sessionId, old, new)`
- `FTRemoteReadFile(sessionId, path, limit)` / `FTRemoteWriteFile(sessionId, path, content)`（直编能力：先按小文件限定实现）

### 2.3 SCP 降级策略
- 当 `FTCheck` 返回 scp：仅保留上传/下载；远端管理（list/mkdir/rm/rename/直编）在 UI 中置灰并解释。
- 上传/下载仍可复用现有“自动降级”逻辑（SFTP_NOT_SUPPORTED → SCP）。

### 2.4 任务队列增强（事件协议不破坏）
- 沿用现有 `file-transfer-progress/done` 事件，但补充字段：
  - `op`（upload/download）、`src`、`dst`、`transport`、`errorCode`
- UI 侧可据此更好地展示与重试。

## 3) 安全与可用性
- 路径规范化：本地路径使用 `filepath.Clean`；远端路径使用 `path.Clean`。
- 防误操作：删除/覆盖前二次确认；大文件直编禁止。
- 仍保持实验开关默认关闭。

# TDD 测试策略（后端为主）

## 1) 复用现有 test SSH server
- 现有 `pkg/filetransfer/test_ssh_server_test.go` 已支持 SFTP subsystem 与最小 scp handler。
- 扩展测试覆盖：
  - SFTP 模式下 mkdir/rename/remove/read/write 的行为
  - 权限不足/路径不存在映射为结构化错误码

## 2) 本地文件系统 API 单测
- 使用 `t.TempDir()` 验证 LocalList/LocalMkdir/LocalRemove/LocalRename。

# 迭代拆分（建议）

## Phase A：UI 重构 + 本地浏览（最快让 UX 像 Xftp）
- 新建组件：`FileManagerPanel`、`FilePane(Local/Remote)`、`PathBar`、`FileTable`、`TransferDrawer`。
- 接入 LocalList/LocalMkdir/LocalRename/LocalRemove。
- 验收：本地像资源管理器一样可浏览、可选择文件。

## Phase B：远端 SFTP 完整文件管理
- 新增远端 mkdir/rm/rename。
- 选择/按钮触发上传下载；任务进入队列。
- 验收：sftp 可用时实现“浏览+操作+传输队列”。

## Phase C：SCP 降级下的“仅传输模式”
- 面板识别 scp 模式，切换到“输入路径+上传下载队列”，并在 UI 清晰提示限制。
- 验收：sftp 不支持时仍能稳定上传/下载，用户不再误以为是 bug。

# 需要改动的关键文件（预览）
- 前端：`FilesPanel.tsx`（重构或替换）、`Sidebar.tsx`、`App.tsx`（状态/入口）、新增多个组件文件。
- 后端：`app.go`（新增 Local*/Remote* 导出）、`pkg/filetransfer`（补齐远端操作）、新增对应 *_test.go。

# 完成标准（交付验收）
- 体验：在 Sidebar 内实现“类 Xftp”双面板 + 队列。
- 功能：本地可浏览；远端 SFTP 可浏览/新建/重命名/删除；SFTP 不支持时有清晰降级与可用的上传/下载。
- 稳定性：`go test ./...` 与 `npm -C frontend run build` 均通过。

确认后我会按 Phase A→B→C 落地，并保持实验开关默认关闭。