---
name: opscopilot-ops
version: '{{OPSCOPILOT_VERSION}}'
description: 通过 OpsCopilot CLI 使用用户预先配置的远程 Linux 服务器与团队运维知识库。knowledge 检索知识库中的排障场景（问题定位、线上故障排查、查团队历史经验时先用它拿方向、根因与处理步骤）；exec 在目标服务器上执行白名单内的 shell 命令，用于取证与验证；file 在本地与服务器之间双向传输文件。适用于服务器故障定位、日志排查、状态检查等运维场景；纯本地操作或未在 OpsCopilot 中登记服务器的场景不适用。
---

<!-- OpsCopilot Skill 文件
     由 OpsCopilot 安装生成，命令路径已替换为本机 opscopilot.exe 的绝对路径。
     请勿手动编辑；如需更新，请在 OpsCopilot GUI 设置 → AI 接入 中重新安装。 -->

# OpsCopilot 运维助手

通过 `"{{OPSCOPILOT_BIN}}"` 命令行连接用户配置的远程服务器和运维知识库，执行运维操作或进行故障定位。所有操作受命令白名单和文件访问控制约束（用户在 OpsCopilot GUI 中配置）。

## 前置条件

- `knowledge` 子命令：无需任何配置，开箱可用。
- `exec` / `file` 子命令：用户必须已在 OpsCopilot GUI 中登记目标服务器（sessions.json，以 IP 标识，含连接信息和凭据）并输入过服务器密码（已存入系统凭据库）。
- `diagnose` 子命令：还需配置好 LLM。

如未配置，命令会返回明确的错误提示，引导用户先在 GUI 中完成配置。

## 子命令

### 1. knowledge —— 检索团队知识库（问题定位的首选入口）

知识库收录了团队沉淀的排障场景（SOP 与历史事件归档），按 服务 → 模块 → 场景 组织。每个场景包含现象、关键词、常见根因、排查步骤和建议命令。**定位任何服务器问题，先用它查有没有现成经验，不要凭空推理。**

```
# 查看知识库覆盖哪些服务→模块
"{{OPSCOPILOT_BIN}}" knowledge list

# 查看某模块下所有场景（返回短 ID + 标题）
"{{OPSCOPILOT_BIN}}" knowledge list --service <服务名> --module <模块名>

# 按症状/关键词检索（错误码是强标识，直接放进检索词）
"{{OPSCOPILOT_BIN}}" knowledge search --query "日志源 开启失败 E215" [--top 5]

# 读取场景全文（拿根因、排查步骤和建议命令）
"{{OPSCOPILOT_BIN}}" knowledge read --id <场景短ID>
```

**典型定位流程**：
1. `knowledge search --query "<症状 + 错误码>"` 拿命中场景（含短 ID 和摘要）。
2. `knowledge read --id <ID>` 读场景全文，获得根因判断和排查命令。
3. 场景中的建议命令需要上机验证时，转 `exec`（先向用户确认目标服务器 IP）。

**检索零命中时**：返回值会标注是"检索词不匹配"（知识库覆盖该领域，换个词或用 `list` 浏览场景标题）还是"领域未覆盖"（直接走自行排查：先 `exec` 收集信息）。弱命中（weak_matches）往往是词面不同但实质相同的历史场景，值得看一眼。

**输出说明**：
- search 命中：`{id, service, module, title, snippet, score}`；零命中：`{miss_kind, covered_services, weak_matches, hint}`，退出码仍为 0，零命中不是错误。
- read 返回场景全文（超长会截断并标注）。
- 场景以短 ID 寻址，没有文件路径概念。

### 2. exec —— 在服务器上执行命令

在已连接的服务器上执行命令。命令受白名单约束，非法命令会被拒绝并在错误信息中给出该服务器允许的命令规则（正则）。

```
"{{OPSCOPILOT_BIN}}" exec --server <服务器IP> --command "<命令>" [--intent "<执行意图>"] [--max-line-length <N>] [--timeout-sec <N>]
```

**命令作用**：
- 用来在指定服务器上执行一条非交互式 shell 命令，并返回标准输出、退出码、耗时、截断信息等结构化结果。
- 适合收集运行状态、日志片段、进程/端口/磁盘/容器信息，也可执行用户明确授权且白名单允许的修复命令。
- 不适合运行需要持续交互的程序；需要交互时应改成有明确边界的一次性命令。

**参数说明**：
- `--server <服务器IP>`：目标服务器标识，必须与 OpsCopilot GUI 中登记的服务器 IP 匹配；命令内部会按需自动连接。不确定 IP 时先问用户，不要猜。
- `--command "<命令>"`：要在远程服务器执行的完整命令。命令应尽量具体、可重复、非交互；包含空格、管道或重定向时必须整体加引号。
- `--intent "<执行意图>"`：可选但建议每次提供，用一句话说明为什么执行这条命令，例如"检查磁盘是否打满导致服务异常"。不要只重复命令本身，也不要写入密码、令牌等敏感信息。
- `--max-line-length <N>`：单行输出最大长度，默认 500。用于避免超长日志行、压缩 JSON、堆栈行把上下文撑爆；需要看完整长行时再适当调大。
- `--timeout-sec <N>`：单条命令最大运行秒数，必须大于 0。慢命令可调大；不确定耗时时先保持默认或缩小命令范围。

**超时参数使用规则**：
- `--timeout-sec` 是单条远程命令的执行超时；不传时读取 `config.json` 里的 `cli.exec_timeout_sec`，未配置则默认 120 秒。
- 快速状态查询（如 `df -h`、`ps aux`、`docker ps`）通常不需要覆盖默认值；已知会慢的日志检索、包管理查询、数据库诊断可显式设置为 180-600 秒。
- 不要直接执行无界交互/持续输出命令，例如 `tail -f`、裸 `top`、`watch`。改用有边界的命令，例如 `tail -n 200`、`top -b -n 1`，或在命令内部加 `timeout 10s ...`。
- 如果命令因超时失败，优先向用户说明命令已被超时保护终止；需要继续排查时，缩小查询范围或提高 `--timeout-sec` 后重试。

**输出**（JSON）：
```json
{
  "success": true,
  "output": "命令的标准输出（超长行已截断）",
  "meta": {
    "command": "...", "server": "...", "intent": "...", "exit_code": 0,
    "duration_ms": 123, "total_bytes": 1024, "returned_bytes": 1024,
    "truncated_lines": 0, "long_lines_truncated": 0
  }
}
```

**何时用**：用户需要查看服务器状态（如 `ps aux`、`df -h`、`docker ps`、`journalctl -u xxx --no-pager`、`tail -n 200 /var/log/xxx`）。优先用 exec 执行具体的只读命令收集信息，而不是泛泛描述。

**示例**：
```
"{{OPSCOPILOT_BIN}}" exec --server 10.0.0.12 --command "df -h" --intent "检查磁盘使用率是否异常"
"{{OPSCOPILOT_BIN}}" exec --server 10.0.0.12 --command "journalctl -u nginx --since '30 min ago' --no-pager | tail -n 200" --intent "查看 nginx 近期错误以定位 5xx 原因" --timeout-sec 180
```

**白名单机制**：命令按**服务器 IP 段**匹配白名单策略，只有命中策略里登记的命令（正则）才放行。默认策略（`IP段: *`）内置了一批只读命令（ls/cat/ps/df/journalctl/docker ps 等），覆盖常见排查。如需执行写入/重启等操作，需用户在 GUI 中添加对应策略。被拒的错误信息会列出每条规则的描述和真实正则，按正则调整命令写法即可，不必反复试错。

### 3. file —— 文件传输

```
# 下载（远程 → 本地）
"{{OPSCOPILOT_BIN}}" file download --server <服务器IP> --remote <远程路径> --local <本地路径> [--max-bytes <N>]

# 上传（本地 → 远程）
"{{OPSCOPILOT_BIN}}" file upload --server <服务器IP> --local <本地路径> --remote <远程路径> [--backup=<true|false>] [--mkdir]
```

**命令作用**：
- `file download`：把远程服务器上的单个文件下载到本地，适合拉取日志、配置、dump、诊断样本后做本地分析。
- `file upload`：把本地单个文件上传到远程服务器，适合投放配置、脚本、补丁或临时诊断工具。上传属于写操作，应更谨慎。

**download 参数说明**：
- `--server <服务器IP>`：目标服务器标识，必须已在 GUI 中登记。
- `--remote <远程路径>`：远程文件路径，只支持文件，不用于下载目录；路径必须通过文件访问控制。
- `--local <本地路径>`：本地保存路径。父目录不存在时会自动创建。
- `--max-bytes <N>`：最大下载字节数，默认 10485760（10 MiB）。实际允许值还会受 GUI 文件访问策略限制；策略更小时以策略为准。

**upload 参数说明**：
- `--server <服务器IP>`：目标服务器标识，必须已在 GUI 中登记。
- `--local <本地路径>`：本地源文件路径，只支持文件，不支持目录。
- `--remote <远程路径>`：远程目标文件路径，必须通过文件访问控制；不要上传到未确认用途的系统关键路径。
- `--backup=<true|false>`：覆盖远程已有文件前是否自动备份，默认 `true`；备份路径会在结果 `meta.backup_path` 中返回。明确不需要备份时传 `--backup=false`。
- `--mkdir`：远程目标目录不存在时自动创建，默认关闭。仅在确认目标目录应被创建时使用。

**示例**：
```
"{{OPSCOPILOT_BIN}}" file download --server 10.0.0.12 --remote /var/log/nginx/error.log --local ./tmp/error.log --max-bytes 5242880
"{{OPSCOPILOT_BIN}}" file upload --server 10.0.0.12 --local ./fix.sh --remote /tmp/fix.sh --backup --mkdir
```

受文件访问控制约束：远程可读/可写路径和大小上限均需在 GUI 配置中放行。默认写入路径为空（禁止上传），需用户显式配置。

### 4. diagnose —— 知识库 AI 诊断（仅无人代办场景）

`knowledge` 子命令已覆盖 agent 场景下的知识库检索。`diagnose` 内部会跑多轮 LLM 推理循环，**单次需要三到五分钟且无客户端超时**，只适合没有 agent 代办的场景（如用户在终端直接调用、脚本集成）；有 agent 时不要用它，用 `knowledge search/read` 自己完成检索与推理。

```
"{{OPSCOPILOT_BIN}}" diagnose --problem "<故障现象描述>"
```

输出 JSON 的 `diagnosis` 字段是 JSON 字符串，解析后含 `summary`（诊断结论）、`steps`（排查步骤）、`commands`（建议命令，每条带 `source` 出处）。

## 工作流程建议

1. **任何服务器故障/异常** → 先 `knowledge search` 查团队经验（症状 + 错误码作检索词）。
2. **命中场景** → `knowledge read` 拿根因与排查命令 → 需上机验证时问用户服务器 IP → `exec` 执行。
3. **零命中** → 按返回的 hint 降级：换词重试或 `knowledge list` 浏览场景标题 → 仍无则自行用 `exec` 收集信息排查（只读命令优先）。
4. **需要拉取文件分析**（日志、配置） → `file download` 到本地后分析。
5. **修复后验证** → 回到 exec 验证症状是否消失。

每次诊断或批量操作后，简明地向用户汇报：发现了什么、做了什么、结果如何。引用知识库结论时注明场景标题或 ID。不要把原始 JSON 直接甩给用户。
