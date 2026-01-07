# 重构 Sidebar 以支持多功能助手 (TDD & UI 测试优先)

根据您的要求，我将把现有的“AI 助手”拆分为“定位助手”和“AI 问答”，并严格遵循 TDD 流程。

## 1. 准备工作 & 架构调整
- **目标**: 将 `Sidebar.tsx` 瘦身为一个纯粹的容器，负责根据 `activeTab` 渲染不同的子组件。
- **新增组件**:
    - `TroubleshootingPanel`: 承载原有的排查逻辑。
    - `AIChatPanel`: 新的自由问答组件。
- **状态管理**: `App.tsx` 将管理三个 Tab 状态: `'sessions'` (会话), `'troubleshoot'` (定位), `'chat'` (问答)。

## 2. 详细 TDD 步骤

### 阶段一：提取“定位助手” (TroubleshootingPanel)
1.  **编写测试 (`TroubleshootingPanel.test.tsx`)**:
    - 测试用例：
        - 渲染初始状态（输入框、开始按钮）。
        - 点击“开始排查”进入排查模式。
        - 渲染结构化消息（步骤、命令）。
        - 点击“结束排查”触发归档弹窗。
    - **可测试性设计**: 为关键元素添加 `data-testid` 或 `aria-label`。
2.  **实现组件 (`TroubleshootingPanel.tsx`)**:
    - 从 `Sidebar.tsx` 中迁移原有的排查逻辑、状态 (`isInvestigating`, `messages` 等) 和 UI。
    - 确保名称修改为“定位助手”，图标更新。

### 阶段二：开发“AI 问答” (AIChatPanel)
1.  **编写测试 (`AIChatPanel.test.tsx`)**:
    - 测试用例：
        - 渲染消息列表和输入框。
        - 发送消息：验证用户消息上屏，并调用后端 API。
        - 接收回复：验证 AI 消息渲染（普通文本格式）。
        - **新建对话**: 点击按钮清空当前消息列表。
2.  **实现组件 (`AIChatPanel.tsx`)**:
    - 实现简单的对话流。
    - 顶部添加“新建对话”按钮。
    - 消息渲染组件不同于定位助手，仅需支持 Markdown 或纯文本，不需要复杂的步骤卡片。

### 阶段三：集成与侧边栏更新
1.  **更新 `Sidebar` 测试 (`Sidebar.test.tsx`)**:
    - 验证根据 `activeTab` 属性正确渲染对应的 Panel。
2.  **更新 `App.tsx`**:
    - 更新右侧图标栏：
        - 🖥️ **会话管理**
        - 🩺 **定位助手** (原 AI 助手，图标变更)
        - 💬 **AI 问答** (新功能)
    - 更新状态管理逻辑以支持 3 个 Tab 的切换。

## 3. 验证计划
- 运行所有单元测试：`npm test` (或 `go test` + 前端测试命令)。
- 手动验证：
    1. 确认“定位助手”保留了原有的排查流（Root Cause 分析等）。
    2. 确认“AI 问答”可以进行普通对话，且“新建对话”能清屏。
    3. 确认侧边栏切换流畅。
