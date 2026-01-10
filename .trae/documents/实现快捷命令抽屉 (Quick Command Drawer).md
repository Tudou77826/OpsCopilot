# 添加快捷命令抽屉 (TDD) - 修正版

根据您的反馈，我更新了计划，明确了命令对象结构和激活终端的逻辑。

## 1. 组件设计
- **位置**: 底部滑出，位于 `BroadcastBar` 之下（视觉上）。
- **数据结构**: `interface QuickCommand { id: string; name: string; content: string; }`。
- **展示**: 仅展示 `name`，悬停可显示 `content` 作为 tooltip。
- **交互**: 点击命令 -> 发送到**最近交互的终端** (`lastActiveTerminalId`)。

## 2. TDD 步骤

### 阶段一：状态管理与激活追踪 (`App.tsx` & `LayoutManager`)
1.  **追踪激活终端**:
    - 在 `App` 中新增状态 `activeTerminalId`。
    - 更新 `LayoutManager`，当终端被点击或聚焦时，回调 `onActiveTerminalChange`。
    - 既然现在的 `LayoutManager` 已经处理了 `onTerminalData` 等，我们只需确保它能捕获点击事件并通知父组件。

### 阶段二：开发 `QuickCommandDrawer` 组件
1.  **编写测试 (`QuickCommandDrawer.test.tsx`)**:
    - **渲染**: 验证只显示 Command Name。
    - **交互**: 验证点击 Handle 展开/收起。
    - **执行**: 点击命令按钮，验证 `onExecute(content)` 被调用。
    - **编辑**: 
        - 模拟右键点击 -> 弹出菜单。
        - 模拟点击“编辑” -> 弹出 Dialog/Input。
        - 验证修改 Name/Content 后保存，列表更新。
2.  **实现组件 (`QuickCommandDrawer.tsx`)**:
    - 实现 UI：底部固定栏 + 滑出动画。
    - 实现右键菜单逻辑。
    - 实现简单的编辑模态框（或行内编辑）。

### 阶段三：集成
1.  **集成到 `App.tsx`**:
    - 引入 `QuickCommandDrawer`。
    - 实现 `handleQuickCommand`：
        - 检查 `activeTerminalId`。
        - 如果存在，调用 `window.go.main.App.Write(activeTerminalId, command + '\n')`。
        - 如果不存在，提示用户“请先选择一个终端”。

## 3. 验证计划
- 运行新组件的单元测试。
- 手动操作：
    1. 打开两个终端。
    2. 点击终端 A，点击快捷命令 -> 确认 A 收到。
    3. 点击终端 B，点击快捷命令 -> 确认 B 收到。
    4. 右键编辑命令名字和内容，确认生效。
