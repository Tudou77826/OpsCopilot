# 优化 AI 解析校验与错误展示 (Revised)

您提出了两点需求：
1.  **完善错误展示**：虽然之前已经实现了部分“查看详情”功能，但您引用了之前的计划文档，似乎希望进一步确认或增强这部分体验（特别是针对“光明正大展示”的需求）。我将复查前端实现，确保错误展示足够友好和完整。
2.  **校验缺失关键信息的回复**：您指出的 Case（Terminal#968-976）中，AI 返回了 `user: null, password: null`，这种配置是无法连接的。后端应该在解析成功 JSON 后，进一步校验字段完整性，如果缺少关键信息（User/Host），应直接报错，而不是返回给前端一个不可用的配置。

## 1. 后端增强：结果校验 (Go)
**目标**：在 `ParseConnectIntent` 中增加逻辑校验，拦截无效配置。
- [ ] **Validation Logic**: 遍历解析出的 `configs`。
    -   如果 `Host` 为空，报错。
    -   如果 `User` 为空，报错提示 "Missing username"。
    -   注意：Password 可以为空（可能是 Key 登录），但 User 通常必须有。
- [ ] **Error Handling**: 如果发现无效配置，返回 `fmt.Errorf`，并在错误信息中包含 "Incomplete configuration received from AI: missing user/host"。

## 2. 前端复查与微调 (React)
**目标**：确保 `SmartConnectModal` 的错误展示符合预期。
- [ ] **Review**: 检查 `SmartConnectModal.tsx` 中 `showErrorDetails` 的实现（上一轮已添加）。
- [ ] **Improvement**: 既然您提到“光明正大展示”，我将调整 UI，如果是校验错误（如 Missing User），直接在结果列表上方显示醒目的 Alert，或者直接进入错误状态，提示用户补充信息。

## 计划步骤
1.  **后端**: 修改 `pkg/ai/intent.go`，在 JSON Unmarshal 后增加字段校验循环。
2.  **验证**: 再次输入 "帮我登录 39.108.107.148"（故意不给账号），验证是否会抛出错误并在前端显示详情。
