## 目标

* 为 AI 问答/问题排查新增“基于内容”的知识库检索能力，减少只靠文件名选文档的误差。

* 保持改动隔离：仅影响 Agent（AskWithContext/AskTroubleshoot），其他 LLM 调用不变。

## 方案概述

* 在 knowledge 层新增 `search_knowledge` 工具：按 query 在所有 md 内容中召回 topK 候选（返回 path + score + snippets）。

* 在 Agent 工具集里加入 `search_knowledge`，并调整 system prompt：先 search，再 read 1–3 篇精读，再回答。

* 继续复用现有 `read_knowledge_file` 做精读，前端“本次参考文档”仍以实际 read 的文档为准。

## 具体改动

* pkg/knowledge

  * 新增内容检索实现（遍历 md → 分块 → 打分 → topK + snippet 截断）。

  * 扩展工具定义：新增 `search_knowledge` 的 JSON schema。

* pkg/ai/agent.go

  * 工具列表新增 `search_knowledge`。

  * tool call 处理新增 case：执行检索并把 JSON 结果回灌给模型。

  * 更新 agentToolPrompt：明确优先用 search，再 read。

  * 保持现有 maxSteps 与重试逻辑不变。

* pkg/ai/intent.go

  * 不改现有非 Agent 路径；仅维持 AskWithContext/AskTroubleshoot 继续走 Agent。

## 体验与可观测性

* 通过 `agent:status` 增加一个新阶段（例如 searching）用于内容检索提示（沿用现有展示组件即可）。

* 日志记录：输出 search的关键字和 命中 topK 文件与分数（截断），便于排查效果。

## 约束与默认参数

* 默认 topK=5，snippet 每条最多 300–500 字符，总输出长度上限（例如 8k–12k chars）。

* 默认精读 1–3 篇（由模型决定，prompt 引导）。

## 验证

* go test ./...

* frontend npm run build

* 手工验证：

  * “后端定位手册”大杂烩文档场景能从内容命中相关段落；

  * 自动记录文档（无语义文件名）也能通过内容召回到正确文件。

## 后续（暂不做）

* 增加文档整理/格式化工具（结构化标题、摘要、标签）以提升检索精度与可解释性。

