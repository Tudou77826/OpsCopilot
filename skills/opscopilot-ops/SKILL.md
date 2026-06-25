---
name: opscopilot-ops
version: '{{OPSCOPILOT_VERSION}}'
description: 通过 OpsCopilot CLI 连接用户预先配置的远程 Linux 服务器，为 AI 提供远程执行命令、传输文件和基于知识库的故障诊断三项能力。exec 在目标服务器上执行 shell 命令，适用于查看运行状态、运行脚本、调试环境等任意需要登录服务器执行操作的场景；file 在本地与服务器之间双向传输文件，可下载服务器上的日志、配置、数据等文件至本地分析，或将本地文件上传至服务器；diagnose 接收用户描述的故障现象，基于团队沉淀的运维知识库输出结构化的诊断结论、排查步骤与建议命令。
---

<!-- OpsCopilot Skill 文件
     由 OpsCopilot 安装生成，命令路径已替换为本机 opscopilot.exe 的绝对路径。
     请勿手动编辑；如需更新，请在 OpsCopilot GUI 设置 → AI 接入 中重新安装。 -->

# OpsCopilot 运维助手

通过 `"{{OPSCOPILOT_BIN}}"` 命令行连接用户配置的远程服务器，执行运维操作或进行 AI 故障诊断。所有操作受命令白名单和文件访问控制约束（用户在 OpsCopilot GUI 中配置），非交互式访问一律过安全闸门。

## 前置条件

用户必须已在 OpsCopilot GUI 中：
1. 登记好目标服务器（sessions.json，以 IP 标识，含连接信息和凭据）
2. 输入过服务器密码（已存入系统凭据库）
3. 配置好 LLM（diagnose 子命令需要）

如未配置，命令会返回明确的错误提示，引导用户先在 GUI 中完成配置。

## 子命令

### 1. exec —— 执行单条命令

在已连接的服务器上执行命令。命令受白名单约束，非法命令会被拒绝并返回该服务器允许的命令列表。

```
"{{OPSCOPILOT_BIN}}" exec --server <服务器IP> --command "<命令>" [--max-line-length <N>] [--timeout-sec <N>]
```

**输出**（JSON）：
```json
{
  "success": true,
  "output": "命令的标准输出（已限流）",
  "meta": {
    "command": "...", "server": "...", "exit_code": 0,
    "duration_ms": 123, "total_bytes": 1024, "returned_bytes": 1024,
    "truncated_lines": 0, "long_lines_truncated": 0
  }
}
```

**何时用**：用户需要查看服务器状态（如 `ps aux`、`df -h`、`docker ps`、`journalctl -u xxx`、`tail -f /var/log/xxx`）。优先用 exec 执行具体的只读命令收集信息，而不是泛泛描述。

**注意**：白名单默认只允许只读命令。如需执行写入操作，提示用户在 GUI 白名单配置中添加对应策略。

### 2. diagnose —— AI 故障诊断

基于 OpsCopilot 知识库对故障问题进行 AI 诊断。纯知识检索 + 推理，**不连接任何服务器**。输出建议的排查命令和步骤，由你（AI）决定是否调 exec 去验证。

```
"{{OPSCOPILOT_BIN}}" diagnose --problem "<故障现象描述>"
```

**输出**（JSON）：
```json
{
  "diagnosis": "{\"summary\":\"...\",\"steps\":[...],\"commands\":[{\"command\":\"...\",\"source\":\"文档.md#L行号\"}]}"
}
```

`diagnosis` 字段是 JSON 字符串，解析后含：
- `summary`：诊断结论
- `steps`：排查步骤
- `commands`：建议命令，每条带 `source` 指明出自哪份知识库文档的哪一行

**何时用**：用户描述了一个**症状/问题**而非具体命令时（如"支付服务 504 了"、"mysql 连不上"、"内存占用高"）。diagnose 会综合知识库给出完整诊断路径，比逐条 exec 更高效。

**重要**：如果诊断返回空的 commands/steps，说明知识库中没有相关经验——此时如实告诉用户"知识库未覆盖此类问题"，不要编造命令。改用 exec 逐项排查基础信息。

### 3. file —— 文件传输

```
# 下载（远程 → 本地）
"{{OPSCOPILOT_BIN}}" file download --server <服务器IP> --remote <远程路径> --local <本地路径> [--max-bytes <N>]

# 上传（本地 → 远程）
"{{OPSCOPILOT_BIN}}" file upload --server <服务器IP> --local <本地路径> --remote <远程路径> [--backup] [--mkdir]
```

受文件访问控制约束：远程可读/可写路径和大小上限均需在 GUI 配置中放行。默认写入路径为空（禁止上传），需用户显式配置。

## 工作流程建议

1. **症状明确、需要具体信息** → 直接 `exec` 收集（如日志、进程、端口、资源占用）
2. **症状模糊、需要诊断方向** → 先 `diagnose` 查知识库建议，拿到建议命令后，自己判断哪些值得执行，再逐条调 `exec` 验证（diagnose 只给建议，不替你执行）
3. **需要拉取文件分析**（日志、配置） → `file download` 到本地后分析
4. **修复后验证** → 回到 exec 验证症状是否消失

每次诊断或批量操作后，简明地向用户汇报：发现了什么、做了什么、结果如何。不要把原始 JSON 直接甩给用户。
