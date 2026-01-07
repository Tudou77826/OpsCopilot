# 实施计划：连接会话管理器

我将实现一个持久化的会话管理器，用于存储、管理和复用 SSH 连接信息，并将其集成到侧边栏中。

## 1. 后端 (Go) - `pkg/sessionmanager` & `app.go`

### 数据结构与存储

* 创建 `pkg/sessionmanager` 包。

* 定义 `Session` 结构体：

  * `ID`: UUID

  * `Name`: 字符串 (IP)

  * `Type`: "session" (会话) | "folder" (文件夹)

  * `Children`: `[]*Session` (用于文件夹)

  * `Config`: `sshclient.ConnectConfig`

* 实现 `Manager` 结构体以处理 `sessions.json` 的持久化存储。

* **自动保存逻辑**:

  * 实现 `Upsert(config ConnectConfig, group string)` 方法：

    * 根据 Host (IP) 查找现有会话。

    * 如果存在，更新信息。

    * 如果不存在，新建会话。

    * 如果提供了 `group` (组名)，将会话放入/移动到该文件夹中。

### Wails API (`app.go`)

* 将 `SessionManager` 注入到 `App` 中。

* 暴露以下方法供前端调用：

  * `GetSessions() []Session`

  * `DeleteSession(id string)`

  * `RenameSession(id string, newName string)`

* 更新 `Connect` 方法：

  * 在发起连接时自动调用 `SessionManager.Upsert` 保存/更新会话。

## 2. 前端 (React) - 侧边栏 & 会话管理

### 类型定义

* 更新 `ConnectionConfig` 接口，增加可选字段 `group?: string`。

### 侧边栏重构 (`Sidebar.tsx`)

* 修改 `Sidebar.tsx` 以支持标签页 (Tab) 界面：

  * **Tab 1: 会话管理** (新的会话管理器)****&#x20;

  * **Tab 2: AI 助手** (现有的对话功能)

### 会话管理器组件 (`SessionManager.tsx`)

* **树形视图 (Tree View)**:

  * 渲染文件夹和会话节点。

  * 显示 IP 作为会话名称。

* **搜索功能**:

  * 根据名称/IP 过滤树节点。

* **右键菜单 (Context Menu)**:

  * **会话节点**: \[打开连接], \[编辑], \[重命名], \[删除]

  * **文件夹节点**: \[重命名], \[删除]

* **编辑模态框**:

  * 提供简单表单以编辑 用户名、端口、密钥路径 等信息。

### 智能连接集成 (`SmartConnectModal.tsx`)

* 更新 `SmartConnectModal.tsx`：

  * 当解析/选中多个目标时，“分组名称”使用跳板机的IP，没有跳板机的情况下就是用默认名称即可（重名时加后缀）

  * 将此 `group` 名称传递给后端的 `Connect` 调用，以实现自动分组。

## 3. 验证计划

* **测试自动保存**: 使用智能连接连接到一个目标，验证其是否出现在会话管理器中。

* **测试分组**: 使用智能连接同时连接多个目标，验证它们是否被归类到文件夹中。

* **测试持久化**: 重启应用，验证会话信息是否保留。

* **测试右键菜单**: 验证 右键 -> 打开连接 是否正常工作。

