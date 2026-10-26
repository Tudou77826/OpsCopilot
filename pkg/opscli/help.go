package opscli

import (
	"errors"
	"fmt"
	"strings"
)

type commandDoc struct {
	path, description, arguments, details string
}

var commandDocs = []commandDoc{
	{"session init", "创建任务 Session", "--session ID --environment ENV", "相同 ID 仅用于同一问题；环境不一致或 Session 已关闭时拒绝复用。"},
	{"session status", "查看 Session 状态", "--session ID", "恢复排查先查看状态，再用 connection list 获取已发现节点。"},
	{"session close", "最终结束任务并清理凭据和下载文件", "--session ID", "等待用户补充时不要关闭。返回 closing 时稍后再次关闭；关闭后 ID 不可复活。"},
	{"connection put", "登记或更新一个 SSH／数据库节点", "--session ID --input FILE|-", `从 stdin（--input -）或受保护 JSON 文件读取一个对象，最大 1 MiB。
SSH 示例：
  {"id":"app-01","kind":"ssh","ssh":{"host":"192.0.2.10","port":22,"user":"operator","password":"<由平台注入>"}}
数据库示例：
  {"id":"db-01","kind":"database","password":"<由平台注入>","database":{"engine":"jdbc","driverId":"<driver list 返回的 ID>","host":"192.0.2.20","port":3306,"user":"reader","database":"app"}}
数据库可用 gatewayId 引用同 Session 的 SSH 节点。相同 ID 更新同一身份时省略密码可保留凭据。
密码不放命令参数；登记节点不会立即建立连接。`},
	{"connection list", "列出本任务已登记的节点", "--session ID", "返回节点配置和凭据是否存在，不回显密码。"},
	{"exec", "执行一次远程只读排查命令", "--session ID --node NODE --command COMMAND [--directory /path] [--as-user USER --elevation su|sudo] [--timeout-sec 120] [--max-line-length 500]", `每次是独立 Shell，不继承目录或身份；--directory 必须为远端绝对路径。
提权时同时指定 --as-user 和 --elevation；超时 1–600 秒，长行限制 400–65536 字节。
示例：opscopilot-cli exec --session issue-123 --node app-01 --directory /srv/app/logs --command 'grep -n ERROR app.log | tail -50'
遵守部署白名单；返回 unknown 表示完成情况不确定，不盲目重跑。`},
	{"database query", "执行只读 SQL", "--session ID --node NODE (--sql SQL | --sql-file FILE|-) [--database DB] [--limit 1000] [--timeout-sec 120]", `--sql-file - 从 stdin 读取 SQL，适合多行内容；文件／stdin 最大 1 MiB，与 --sql 互斥。
示例：opscopilot-cli database query --session issue-123 --node db-01 --sql 'SELECT 1' --limit 20
返回行数 1–5000，超时 1–120 秒，输出最多约 32 KiB；截断后缩小查询范围。
依赖部署导入的 JDBC 驱动和 Java 21。当前服务端只读适配仅支持 MySQL 方言；先用 driver list 检查 serverCapabilities。`},
	{"database schemas", "列出数据库／Schema", "--session ID --node NODE [--timeout-sec 120]", "通过已导入的 JDBC 驱动读取元数据。当前服务端仅放行 MySQL 方言。"},
	{"database tables", "列出表和视图", "--session ID --node NODE [--database DB] [--timeout-sec 120]", "--database 指定目标库／Schema；省略时沿用连接配置和驱动默认行为。当前服务端仅放行 MySQL 方言。"},
	{"database structure", "查看字段、索引和已适配的 DDL", "--session ID --node NODE --table TABLE [--database DB] [--timeout-sec 120]", "通过驱动和数据库方言适配读取表结构。当前服务端仅放行 MySQL 方言。"},
	{"file download", "通过 SFTP 下载普通文件", "--session ID --node NODE --remote /path --local logs/file.log [--max-bytes 10485760] [--timeout-sec 120]", "--remote 必须为远端绝对路径；--local 是 Session 工作区内相对路径，不覆盖已有文件。使用 SSH 登录身份，受文件策略约束。大小范围 1–1073741824 字节，策略可进一步收紧；partial:true 表示文件不完整。"},
	{"knowledge list", "列出知识服务、模块或场景", "[--service SERVICE --module MODULE]", "不需要 Session；--service 和 --module 必须一起提供。依赖 OPSCOPILOT_KNOWLEDGE_DIR。"},
	{"knowledge search", "检索定位经验", "--query TEXT [--top 5]", "不需要 Session；依赖 OPSCOPILOT_KNOWLEDGE_DIR。示例：opscopilot-cli knowledge search --query '连接池 超时'"},
	{"knowledge read", "读取知识场景", "--id SCENARIO_ID", "ID 来自 knowledge list 或 search。内容最大约 32 KiB，检查 truncated 标记。"},
	{"driver list", "查看部署驱动和服务端兼容能力", "", "不需要 Session。available 仅表示运行组件文件就绪；serverCapabilities 说明各驱动的服务端只读适配状态，不代表连接或认证成功。"},
	{"driver import", "导入厂商 JDBC JAR（部署操作）", "--input FILE|-", `不属于自动定位任务操作；依赖 Java 21 和 OPSCOPILOT_JDBC_DRIVER_DIR。
JSON 示例：{"name":"MySQL","className":"com.mysql.cj.jdbc.Driver","urlTemplate":"jdbc:mysql://{host}:{port}/{database}","dialect":"mysql","paths":["/opt/drivers/mysql-connector.jar"]}
paths 可包含依赖 JAR；驱动类、URL 模板和 dialect 必须与厂商驱动匹配。`},
}

type commandRoute struct {
	topic string
	help  bool
}

func findCommand(path string) *commandDoc {
	for i := range commandDocs {
		if commandDocs[i].path == path {
			return &commandDocs[i]
		}
	}
	return nil
}

func commandGroup(name string) bool {
	for _, doc := range commandDocs {
		if strings.HasPrefix(doc.path, name+" ") {
			return true
		}
	}
	return false
}

func resolveCommand(args []string) (commandRoute, error) {
	route := commandRoute{}
	if len(args) == 0 {
		route.help = true
		return route, nil
	}
	if args[0] == "help" {
		route.help = true
		args = args[1:]
		if len(args) == 0 {
			return route, nil
		}
	}
	if args[0] == "--help" || args[0] == "-h" {
		route.help = true
		return route, nil
	}
	if commandGroup(args[0]) {
		route.topic = args[0]
		if len(args) == 1 || args[1] == "--help" || args[1] == "-h" {
			route.help = true
			return route, nil
		}
		route.topic += " " + args[1]
		if findCommand(route.topic) == nil {
			route.topic = args[0]
			return route, errors.New("未知子命令，请查看该组帮助")
		}
		args = args[2:]
	} else if findCommand(args[0]) != nil {
		route.topic = args[0]
		args = args[1:]
	} else {
		return route, errors.New("未知命令，请查看帮助")
	}
	if route.help && len(args) != 0 {
		return route, errors.New("help 后只接受命令名称")
	}
	// All current options take a value. Do not mistake a command/SQL value of
	// "--help" for a request for local help.
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			break
		}
		if args[i] == "--help" || args[i] == "-h" {
			route.help = true
			break
		}
		if strings.HasPrefix(args[i], "-") && !strings.Contains(args[i], "=") {
			i++
		}
	}
	return route, nil
}

func renderHelp(topic string) string {
	if doc := findCommand(topic); doc != nil {
		return fmt.Sprintf("%s\n\n用法：\n  opscopilot-cli %s %s\n\n%s\n", doc.description, doc.path, doc.arguments, doc.details)
	}
	var out strings.Builder
	if topic == "" {
		out.WriteString("OpsCopilot Server CLI — 服务端只读定位\n\n流程：session init → connection put → exec / database / file → session close\n暂停等待补充时保留 Session；最终结束才关闭。\n\n命令：\n")
	} else {
		fmt.Fprintf(&out, "%s 子命令：\n", topic)
	}
	for _, doc := range commandDocs {
		if topic == "" || strings.HasPrefix(doc.path, topic+" ") {
			fmt.Fprintf(&out, "  %-20s %s\n", doc.path, doc.description)
		}
	}
	out.WriteString("\n参数和示例：opscopilot-cli help <命令>，或在子命令后加 --help\n帮助不需要环境配置，也不会连接服务器。\n")
	if topic == "" {
		out.WriteString("\n环境：OPSCOPILOT_SERVER_ROOT、OPSCOPILOT_SERVER_KEY_FILE\n知识库：OPSCOPILOT_KNOWLEDGE_DIR\n数据库：OPSCOPILOT_JDBC_DRIVER_DIR、OPSCOPILOT_JDBC_RUNTIME_DIR（Java 21）\n策略：OPSCOPILOT_WHITELIST_PATH、OPSCOPILOT_FILE_ACCESS_PATH\n\n除帮助外 stdout 为 JSON；退出码非 0 时读取 status、error、hint。\n")
	}
	return out.String()
}

func helpHint(topic string) string {
	return strings.TrimSpace("查看参数和示例：opscopilot-cli help " + topic)
}
