# 优化 AI 解析失败的用户体验

您希望在 AI 解析失败时，能通过弹窗展示详细的错误信息，以便于排查（例如看到原始的 AI 响应）。

## 1. 后端增强 (Go)
**目标**：在解析失败时，不仅返回错误信息，还尽可能保留 AI 的原始响应内容，以便前端展示。
- [ ] **Refactor**: 修改 `ParseIntent` 逻辑，当 `json.Unmarshal` 失败时，返回的 error message 应包含更丰富的上下文（虽然现在的 log 已经有了，但 error 最好也带上，或者定义专门的 Error 类型）。
- [ ] **Current State**: 目前代码中 `fmt.Errorf("failed to parse AI response as JSON: %v. Raw: %s", err, resp)` 已经包含了 Raw response，但前端可能只展示了部分信息。

## 2. 前端增强 (React)
**目标**：改进 `SmartConnectModal` 的错误展示 UI。
- [ ] **Component**: 引入一个新的 `ErrorModal` 或在当前 Modal 中增加一个详情展开区。
- [ ] **Logic**: 当 `handleParse` 捕获到错误时：
    -   解析错误信息字符串。
    -   如果错误信息很长（包含 Raw JSON），不要直接塞在那个小红字里。
    -   提供一个 "Show Details" 链接/按钮。
    -   点击后弹出一个对话框（或展开），用 `<pre>` 标签展示完整的错误堆栈和 AI 原始响应。
- [ ] **UI**: 优化错误提示区域，使用醒目的警告色，并提供 "Retry" 建议。

## 计划步骤
1.  **前端**: 修改 `SmartConnectModal.tsx`，增加 `showErrorDetails` 状态。
2.  **前端**: 优化错误显示区域，增加“查看详情”交互。
3.  **前端**: 实现详情展示视图，支持复制原始响应内容。

这是一个纯前端的体验优化，不需要修改后端逻辑（因为后端已经在 error message 里返回了 Raw 数据）。
