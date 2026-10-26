---
name: opscopilot-server
description: 服务端只读故障定位工具。问题平台提供 Session 和入口节点后，使用独立 OpsCopilot CLI 登记业务节点、读取远程日志和状态、执行只读 SQL、下载文件与检索团队知识；等待人工补充后恢复同一 Session。
---

# 服务端故障定位

程序为 PATH 中的 `opscopilot-cli`。平台预设数据根、加密密钥、知识库、驱动和运行环境；不调用桌面 `opscopilot.exe`，不创建 GUI 页签。

不清楚参数时用 `opscopilot-cli help <命令>`（如 `help connection put`）或子命令的 `--help` 获取用法和 JSON 示例；帮助不要求环境配置、不连接服务器。操作失败解析 JSON `error`，并参考可选 `hint`，不要猜测参数或重跑结果未知的命令。

## 操作边界

只在本次任务业务环境内查看日志、状态和只读 SQL。禁止修改配置、重启、删除、上传安装；需要修改时给出建议并交回平台。允许任务授权下的提权和发现更多业务节点。发现／解密／凭据刷新使用既有 Skill 和 iCode 原生脚本能力，工具不限制发现算法。

## 开始和恢复

1. 从平台获取稳定 Session ID 和业务环境。首次用 `session init --session ID --environment ENV`；恢复先 `session status --session ID`、`connection list --session ID`，直接使用已登记节点，不重复发现。
2. 平台或脚本提供节点 JSON，通过 stdin 调 `connection put --session ID --input -`。SSH 字段为 `id,kind:"ssh",ssh:{host,port,user,password,root_password,bastion}`；`bastion` 可递归。数据库为 `id,kind:"database",password,gatewayId,database:{engine:"jdbc",driverId,host,port,user,database}`。`gatewayId` 引用同 Session SSH 节点，可省略直连。密码不放命令行、不写入结论。
3. 节点以 ID 寻址，不能仅按 IP 合并。凭据过期只刷新对应节点，同目标省略密码会保留原值；更新包含主机、端口、账号等完整配置。数据库驱动通过部署好的 `driver list` 获取；检查 `serverCapabilities` 中对应 `driverId` 的 `readOnlyAdapted` 和原因，未适配时报告限制。`available` 只表示运行组件文件存在，不保证连接成功。任务不导入或修改驱动。

## 工具调用

```sh
opscopilot-cli exec --session ID --node NODE --command 'grep -n ERROR app.log | tail -50' --directory /srv/app/logs --timeout-sec 30
opscopilot-cli exec --session ID --node NODE --command 'systemctl status app' --as-user root --elevation sudo
opscopilot-cli file download --session ID --node NODE --remote /var/log/app.log --local logs/app.log --max-bytes 10485760
opscopilot-cli database query --session ID --node DBNODE --database app --sql 'SELECT id,status FROM incidents LIMIT 20' --limit 20 --timeout-sec 30
opscopilot-cli database schemas --session ID --node DBNODE
opscopilot-cli database tables --session ID --node DBNODE --database app
opscopilot-cli database structure --session ID --node DBNODE --database app --table incidents
opscopilot-cli knowledge list
opscopilot-cli knowledge search --query '症状 关键词' --top 5
opscopilot-cli knowledge read --id SCENARIO_ID
```

每次是独立 Shell，不继承目录、身份或事务。`--directory` 为绝对路径；提权必须指定目标账号和 `su|sudo`。下载经 SSH 登录身份的 SFTP，和终端提权分开。`--local` 为 Session 内相对路径，不覆盖已有文件；失败返回 `partial:true` 时不能把文件当作完整结果。

长 SQL 可改用 `database query --sql-file /path/query.sql` 或 `--sql-file -` 从 stdin 读取；与 `--sql` 互斥，最大 1 MiB，须为 UTF-8。文件位于运行 CLI 的本机。两种输入都受相同只读保护，不用于绕过 SQL 拒绝。

stdout 为 `{success,status,data,error,hint}`，后三项按需出现；退出码非 0 必须解析失败结果。SSH 真正退出码为 `data.exitCode`，`completionKnown:false` 或 `status:unknown` 表示无法确认，不猜测成功、不盲目重跑。`startAttempted:true` 但 `started:false` 可能是确认回包丢失，不能断定命令未执行。`truncated:true` 或结果集 `truncated:true` 表示输出有裁剪，改用更窄的时间范围、grep 条件、head/tail、SQL LIMIT；没有隐藏的完整输出读取入口。

命令受部署白名单和文件策略约束，不能以 `sh -c`、改写脚本或其他通道绕过拒绝。数据库程序强制只读，目前支持 MySQL 方言；通用 JDBC 未适配时应报告能力限制，不能建议取消保护。

## 提问、异常和结束

- 信息不足：使用问题平台提供的提问工具，保存已知证据和待补充信息，退出当前 Agent 进程；不要关闭工具 Session。
- 用户补充：恢复平台／iCode 对话上下文和同一工具 Session，复用节点与文件。
- 认证失败：按发现 Skill 更新过期凭据；缺资料交给平台提问，不自动换身份或重算整个拓扑。
- 超时、取消、断线：记录已取得的输出和是否确认完成。关闭客户端不证明远端进程已停止，按风险核对现场。
- 结论反馈到平台，由平台决定最终 `session close --session ID`。`closing` 表示尚在收尾，应稍后重复关闭。关闭清除凭据和下载文件且标识不可复活，不用于暂停。

工具不另存完整命令输出、模型历史或定位报告；沿用 iCode 工具轨迹和平台会话记录。部署及完整 JSON 示例见随包 `server-cli.md`。
