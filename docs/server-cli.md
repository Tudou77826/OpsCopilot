# 服务端定位工具部署与调用

独立程序 `opscopilot-cli` 供问题平台启动的 Chrys／iCode 调用，支持 Linux amd64、arm64。它不启动桌面、AI 会话、MCP 服务或任务队列。问题平台管理 Agent、人工提问、模型历史、并发及结论回传；工具管理连接事实和单次远程操作。

## 查看用法

`opscopilot-cli help` 列出完整命令；`opscopilot-cli database` 列出该组子命令。每个操作均支持 `--help` 和 `help <命令>`，无需配置环境，不会读取凭据或建立连接：

```sh
opscopilot-cli help connection put
opscopilot-cli database query --help
opscopilot-cli help driver import
```

帮助包含必填参数、JSON 输入示例及限制。失败时 stdout JSON 的可选 `hint` 提供对应帮助入口或 Session 恢复／关闭指引。

## 部署

在仓库根目录执行 `pwsh scripts/build-server-cli.ps1`，产物位于 `build/server-cli/`。Linux 也可用 Go 1.25 及以上版本构建：

```sh
CGO_ENABLED=0 go build -trimpath -o opscopilot-cli ./cmd/opscopilot-cli
chmod +x opscopilot-cli
```

ZIP 包中的二进制解压到 Linux 后需要补执行权限。包不包含部署密钥、连接数据、iCode、JDBC 驱动或 Java。将 `skills/opscopilot-server/` 安装到该 iCode 支持的 Skill 目录，并在运行环境将程序目录加入 PATH。

Python Skill 脚本需兼容部署机器的解释器；iCode 可能优先使用 PATH 中的系统 Python，而非自身打包的解释器。JDBC helper 需要 Java 21，不能据系统安装了 Java 就认定版本满足要求；使用独立运行目录可避免更改系统默认 Java。

由部署账号准备以下变量，再由平台传给每次工具调用：

| 变量 | 用途 |
|---|---|
| `OPSCOPILOT_SERVER_ROOT` | 必填，Session 数据根；使用本机文件系统 |
| `OPSCOPILOT_SERVER_KEY_FILE` | 必填，根目录之外的加密密钥文件，32 字节随机值，也支持 hex/base64 编码；Linux 权限 0600 |
| `OPSCOPILOT_KNOWLEDGE_DIR` | 知识查询必填，共享知识目录，沿用现有目录结构 |
| `OPSCOPILOT_JDBC_DRIVER_DIR` | 数据库必填，部署导入的驱动登记目录 |
| `OPSCOPILOT_JDBC_RUNTIME_DIR` | 数据库运行组件，含 `jdbc-helper.jar` 和 `runtime/bin/java`；省略时使用程序旁 `jdbc-runtime/` |
| `OPSCOPILOT_WHITELIST_PATH` | 可选，现有命令白名单 JSON；省略使用内置规则，显式路径无效则失败，不替换配置 |
| `OPSCOPILOT_FILE_ACCESS_PATH` | 可选，现有文件访问策略 JSON；省略使用内置规则，显式路径无效则失败 |

示例（首次部署执行，不要每次启动重新生成密钥）：

```sh
umask 077
mkdir -p /var/lib/opscopilot /etc/opscopilot
# 仅首次部署、确认文件不存在时生成；备份密钥，否则无法恢复旧 Session。
test -e /etc/opscopilot/session.key || head -c 32 /dev/urandom > /etc/opscopilot/session.key
export OPSCOPILOT_SERVER_ROOT=/var/lib/opscopilot
export OPSCOPILOT_SERVER_KEY_FILE=/etc/opscopilot/session.key
export OPSCOPILOT_KNOWLEDGE_DIR=/opt/opscopilot/knowledge
```

Session 使用 AES-256-GCM 加密，标识绑定为认证数据；文件写入采用跨进程锁与原子替换。使用属主权限运行，避免共享可写数据根。相同服务账号下的 Session 隔离用于防止串用，不是恶意本地程序的系统沙箱。平台和 iCode 的工具输入日志也必须按其凭据策略处理。

JDBC 使用 Java 21 运行组件；可将已有 Linux Java 21 目录链接为 `jdbc-runtime/runtime`，将本仓库 `jdbc-helper` 构建的 `jdbc-helper.jar` 放入同目录。使用与目标 Linux libc、架构匹配的 Java，不能复用 Windows Java。部署人员通过 `driver import --input -` 导入厂商 JAR，工具不会下载驱动。

**只读兼容范围：**当前经过服务端事务只读适配的是 MySQL 方言。通用 JDBC（包括尚未做方言适配的 Zenith）会明确拒绝程序只读连接；不会取消保护或将 `Connection.setReadOnly` 提示当作服务端强制保护。新增数据库方言需要适配并验证只读事务。建议同时使用数据库只读账号。

`driver list` 保留 `available`、`message`、`drivers`，并提供 `serverCapabilities`：按 `driverId` 标明 `readOnlyAdapted` 及原因。`available` 只表示运行组件文件存在；`readOnlyAdapted` 只表示方言已适配，二者都不能替代真实驱动加载、Java 版本、连接与权限验证。

## Session 和节点

平台生成稳定 Session 标识（字母、数字、点、下划线、连字符，最长 128），恢复同一问题时沿用标识。不同问题使用不同标识。

```sh
opscopilot-cli session init --session issue-123 --environment business-a
opscopilot-cli session status --session issue-123
opscopilot-cli connection put --session issue-123 --input -
opscopilot-cli connection list --session issue-123
```

`connection put` 从标准输入读取一个 JSON 对象。示例中的占位凭据由平台或发现脚本通过管道填充，禁止放到命令参数中：

```json
{
  "id": "service-a",
  "name": "业务节点 A",
  "kind": "ssh",
  "ssh": {
    "host": "192.0.2.10", "port": 22, "user": "operator",
    "password": "<平台提供>", "root_password": "<需要提权时提供>",
    "bastion": {"host": "192.0.2.1", "port": 22, "user": "gateway", "password": "<跳板凭据>"}
  }
}
```

SSH 节点支持密码／keyboard-interactive、多级跳板和现有可选主机指纹。不要求预先登记所有主机指纹。节点 ID 区分用途、端口和账号，不使用 IP 作为唯一身份。同 ID 更新需提供节点配置；目标不变时省略密码保留原值，SSH 省略跳板保留原路线。目标或账号变化不会继承该身份的密码。需要重置整条路线时使用新节点 ID。

数据库节点示例，`driverId` 取部署 `driver list` 输出，`gatewayId` 可省略以直连：

```json
{
  "id": "database-a", "kind": "database", "password": "<数据库凭据>",
  "gatewayId": "service-a",
  "database": {
    "engine": "jdbc", "driverId": "<已部署驱动ID>",
    "host": "192.0.2.20", "port": 3306, "user": "readonly", "database": "business"
  }
}
```

数据库密码省略时只在主机、端口、账号和驱动都不变的更新中保留；提供空字符串显式清除。节点列表只返回配置摘要和凭据是否存在。每个 Session 最多 256 节点，输入 JSON 最大 1 MiB。

## 执行、下载与知识

```sh
opscopilot-cli exec --session issue-123 --node service-a --command 'grep -n ERROR app.log | tail -50' --directory /srv/app/logs --timeout-sec 30
opscopilot-cli exec --session issue-123 --node service-a --command 'systemctl status app' --as-user root --elevation sudo
opscopilot-cli file download --session issue-123 --node service-a --remote /var/log/app.log --local logs/app.log --max-bytes 10485760
opscopilot-cli database query --session issue-123 --node database-a --database business --sql 'SELECT id,status FROM incidents LIMIT 20' --limit 20 --timeout-sec 30
opscopilot-cli database schemas --session issue-123 --node database-a
opscopilot-cli database tables --session issue-123 --node database-a --database business
opscopilot-cli database structure --session issue-123 --node database-a --database business --table incidents
opscopilot-cli knowledge list
opscopilot-cli knowledge search --query '连接池 超时' --top 5
opscopilot-cli knowledge read --id '<场景短ID>'
```

多行 SQL 可用 `database query --sql-file /path/query.sql`，也可用 `--sql-file -` 从标准输入读取。文件位于运行 Ops 的服务器上，须为 UTF-8 普通文件，最大 1 MiB；`--sql` 与 `--sql-file` 互斥。两种输入使用相同的只读检查、超时和结果限制：

```sh
opscopilot-cli database query --session issue-123 --node database-a --database business --sql-file - <<'SQL'
SELECT id, status
FROM incidents
WHERE status = 'open'
LIMIT 20
SQL
```

每次调用重新连接，无跨调用 Shell、工作目录或事务。提权明确指定 `su` 或 `sudo`，密码只经 SSH stdin 交给远端程序，失败不切换身份、不重跑。需要 tty 的特殊 PAM／sudo 配置可能拒绝非交互提权，届时返回真实错误。

首版下载使用 SSH 登录账号的 SFTP 通道，不继承命令里的 `su` 身份，不使用隐式 SCP 降级。远端路径须绝对、通过部署文件策略；本地路径相对 Session 工作目录，禁止逃逸及覆盖已有文件。文件默认最大 10 MiB，策略可进一步收紧。失败若已产生文件返回 `partial:true` 和实际字节数，供 Agent 核对，不把部分文件当成功结果。

命令只允许匹配现有策略的只读类别；白名单不能证明所有任意 Shell 组合或程序无副作用，Chrys 仍须遵守任务授权。命令总返回预算约 10 KiB，收集缓冲每个流 64 KiB，长行默认裁剪为头尾片段；`truncated` 标识涵盖两阶段裁剪。SQL 和元数据输出限制 32 KiB，查询最多 5000 行、超时 1–120 秒；普通命令超时 1–600 秒。截断后由 Agent 缩小查询，不保存完整输出归档。

## 结果和生命周期

stdout 为单个 JSON（帮助除外）：`{success,status,data,error,hint}`，`data`、`error`、`hint` 按需出现。退出码 0 表示成功，非 0 必须读取结果，不能当作远端命令退出码。SSH 的真实退出码在 `data.exitCode`；无确定退出状态时为 null。`startAttempted` 表示已尝试提交执行请求，`started` 表示已收到服务端确认；确认回包丢失时不能因为 `started:false` 就断定未执行。结合 `completionKnown` 和 `status` 判断，不猜测成功或自动重试。

`status` 包括 `success`、`failed`、`denied`、`authentication_failed`、`timeout`、`cancelled`、`unknown`、`session_closed`、`not_found`、`closing`。数据库和文件返回各自结果结构；远端认证错误包含脱敏原因。工具不自动重连重跑、不自动刷新凭据。超时／断线／取消后，即使关闭 SSH 通道也不保证远端子进程全部停止；Agent 需要按结果核对现场。

平台等待人工答复时，停止 Agent／CLI 即释放活动连接，但保留 Session 的节点、凭据及下载文件。恢复后新进程读取同一 Session；凭据失效时仅更新对应节点，不重算其他拓扑。模型上下文和历史由平台／iCode 恢复。

问题结束且平台决定删除工具状态时：

```sh
opscopilot-cli session close --session issue-123
```

关闭先禁止新操作，再通知活动调用取消并等待最多 10 秒，成功后清除节点凭据及下载目录，保留不可复活的关闭标记。返回 `closing` 表示尚未收尾，平台应稍后重复关闭；重复关闭幂等。停止一轮或退出进程不等于关闭 Session。

## 自测入口

```sh
go test ./pkg/opscli ./pkg/opssession ./pkg/core/ops ./pkg/core/security ./pkg/sshclient ./pkg/filetxn -count=1
```

Linux 默认测试包含真实 loopback SSH/SFTP、独立 CLI 进程与 Session 恢复／关闭。`OPSCOPILOT_TEST_CLI_BINARY` 可指定预编译测试二进制。MySQL/JDBC 联测另需 `OPSCOPILOT_TEST_JDBC_RUNTIME`、`OPSCOPILOT_TEST_JDBC_JAR`、`OPSCOPILOT_TEST_MYSQL_PORT`、`OPSCOPILOT_TEST_MYSQL_PASSWORD`，可选 `OPSCOPILOT_TEST_MYSQL_HOST`；测试库沿用 `e2e/harness/database-local.ps1` 的隔离数据，不指向业务库。

目标内网的实际数据库方言、跳板策略，以及问题平台的提问和 iCode 原生会话恢复需要联合验收，独立工具测试不替代这些集成验证。
