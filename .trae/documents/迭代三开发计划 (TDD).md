# 迭代三：多路复用与多布局终端开发计划 (v0.3)

本计划旨在完成多路复用（Multiplexing）功能，支持同时管理多个 SSH 会话，并提供 **Tab 页** 和 **2x2 网格** 两种布局模式。开发将严格遵循 TDD 模式。

## 1. 后端开发：会话管理 (SessionManager)
**目标**：管理多个并发 SSH 会话，支持按 ID 寻址和广播。
- [ ] **Define**: 定义 `SessionManager` 接口，支持 `Add`, `Get`, `Remove`, `List`。
- [ ] **Refactor**: 将 `App` 结构体中的单一 `client/stdin` 替换为 `SessionManager`。
- [ ] **Test**: 编写 `pkg/session/manager_test.go`，验证多会话的增删查改及并发安全性。
- [ ] **Implement**: 实现 `SessionManager`，每个 Session 包含 `ID`, `Client`, `Stdin`, `Stdout` 等信息。

## 2. 后端开发：广播功能 (Broadcasting)
**目标**：将单一指令并发分发到选定的多个会话。
- [ ] **Test**: 编写 `pkg/session/broadcast_test.go`。
    -   创建多个 Mock Session。
    -   调用 `Broadcast(ids, command)`。
    -   验证所有 Session 的 Stdin 是否都收到了相同的指令。
- [ ] **Implement**: 在 `SessionManager` 中实现 `Broadcast` 方法，使用 Goroutine 并发写入。

## 3. 前端开发：布局管理器 (LayoutManager)
**目标**：实现 Tab 模式和 Grid 模式的切换与渲染。
- [ ] **Component**: 创建 `LayoutManager` 组件，接受 `terminals` 数组和 `mode` ('tab' | 'grid') 作为 props。
- [ ] **Test**: 编写 `frontend/src/components/LayoutManager/LayoutManager.test.tsx`。
    -   验证 'tab' 模式下显示 Tabs 头部和当前激活的 Terminal。
    -   验证 'grid' 模式下同时显示所有 Terminal（或最多4个）。
    -   验证模式切换是否保持 Terminal 状态（不应销毁重建组件，避免连接断开）。

## 4. 前端开发：广播控制条
**目标**：提供输入框和发送按钮，控制向当前所有激活终端广播命令。
- [ ] **Component**: 创建 `BroadcastBar` 组件。
- [ ] **Test**: 编写 `frontend/src/components/BroadcastBar/BroadcastBar.test.tsx`。
    -   验证输入框和发送按钮。
    -   验证提交时调用回调函数。

## 5. 集成与 API 升级
- [ ] **API Update**: 更新 `App.go` 暴露的方法：
    -   `Connect(config) -> sessionID` (支持多开，返回 ID)
    -   `Write(sessionID, data)` (指定会话写入)
    -   `Broadcast(sessionIDs[], data)` (广播)
    -   `Close(sessionID)`
- [ ] **Frontend Integration**: 更新 `App.tsx`：
    -   维护全局 `terminals` 状态。
    -   集成 `LayoutManager` 和 `BroadcastBar`。
    -   处理后端事件路由（根据 `sessionID` 分发数据到对应 Terminal）。

## 验收标准
- [ ] 能同时打开多个 SSH 会话。
- [ ] 点击 "Tab Mode" 按钮，终端以标签页形式显示，一次只显示一个。
- [ ] 点击 "Grid Mode" 按钮，终端以 2x2 网格形式显示，同时可见。
- [ ] 在广播条输入 `date`，所有连接中的终端同时执行并显示结果。
