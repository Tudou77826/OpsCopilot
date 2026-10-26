// Package opscli adapts headless CLI requests to shared operation services.
package opscli

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/core/ops"
	"opscopilot/pkg/core/security"
	"opscopilot/pkg/filetxn"
	"opscopilot/pkg/opssession"
	"opscopilot/pkg/sshclient"
)

type response struct {
	Success bool   `json:"success"`
	Status  string `json:"status"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
	Hint    string `json:"hint,omitempty"`
}

type redactedError struct {
	message string
	cause   error
}

func (e redactedError) Error() string { return e.message }
func (e redactedError) Unwrap() error { return e.cause }

func Run(ctx context.Context, args []string, input io.Reader, output, diagnostic io.Writer) int {
	route, routeErr := resolveCommand(args)
	if routeErr == nil && route.help {
		fmt.Fprint(output, renderHelp(route.topic))
		return 0
	}
	var value any
	var status string
	err := routeErr
	if err == nil {
		value, status, err = dispatch(ctx, args, input)
	}
	result := response{Success: err == nil, Status: status, Data: value}
	if result.Status == "" {
		result.Status = "success"
	}
	if err != nil {
		result.Success = false
		if status == "" {
			result.Status = errorStatus(err)
		}
		result.Error = assistantcontrol.DisplayExcerpt(err.Error(), 4096)
		result.Hint = helpHint(route.topic)
		if errors.Is(err, opssession.ErrNotFound) {
			result.Hint = "请由平台先执行 session init；恢复任务时核对 --session 和 OPSCOPILOT_SERVER_ROOT。"
		} else if errors.Is(err, opssession.ErrClosed) {
			result.Hint = "该 Session 已最终关闭。新任务使用新的 Session ID；等待人工补充时不要执行 session close。"
		}
	}
	if err = json.NewEncoder(output).Encode(result); err != nil {
		fmt.Fprintln(diagnostic, "结果写入失败")
		return 1
	}
	if !result.Success {
		return 1
	}
	return 0
}
func errorStatus(err error) string {
	switch {
	case errors.Is(err, opssession.ErrClosed):
		return "session_closed"
	case errors.Is(err, opssession.ErrBusy):
		return "closing"
	case errors.Is(err, opssession.ErrNotFound):
		return "not_found"
	case errors.Is(err, ops.ErrPolicy):
		return "denied"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	if strings.Contains(err.Error(), "unable to authenticate") || strings.Contains(strings.ToUpper(err.Error()), "SQLSTATE=28000") {
		return "authentication_failed"
	}
	return "failed"
}
func flags(name string, args []string, bind func(*flag.FlagSet)) error {
	f := flag.NewFlagSet(name, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	bind(f)
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return errors.New("不支持未命名参数，请查看 help")
	}
	return nil
}
func storeFromEnvironment() (*opssession.Store, error) {
	path := os.Getenv("OPSCOPILOT_SERVER_KEY_FILE")
	if path == "" {
		return nil, errors.New("请配置 OPSCOPILOT_SERVER_KEY_FILE，由部署提供 32 字节加密密钥文件")
	}
	if info, e := os.Stat(path); e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 && os.PathSeparator == '/' {
		return nil, errors.New("部署密钥必须为仅属主可读写的普通文件（chmod 600）")
	}
	b, err := filetxn.ReadBounded(path, 256)
	if err != nil {
		return nil, errors.New("无法读取部署加密密钥文件，或文件超过大小限制")
	}
	if len(b) != 32 {
		value := strings.TrimSpace(string(b))
		if decoded, e := hex.DecodeString(value); e == nil && len(decoded) == 32 {
			b = decoded
		} else if decoded, e := base64.StdEncoding.DecodeString(value); e == nil && len(decoded) == 32 {
			b = decoded
		}
	}
	return opssession.New(os.Getenv("OPSCOPILOT_SERVER_ROOT"), b)
}
func decodeInput(path string, input io.Reader, target any) error {
	if path == "" {
		return errors.New("请指定 --input - 从标准输入读取，或指定受保护的 JSON 文件")
	}
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return errors.New("无法读取输入文件")
		}
		defer f.Close()
		input = f
	}
	limited := io.LimitReader(input, (1<<20)+1)
	b, err := io.ReadAll(limited)
	if err != nil || len(b) > 1<<20 {
		return errors.New("输入读取失败或超过 1 MiB")
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err = d.Decode(target); err != nil {
		return errors.New("输入 JSON 格式或字段无效")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("输入只能包含一个 JSON 对象")
	}
	return nil
}
func dispatch(ctx context.Context, args []string, input io.Reader) (any, string, error) {
	if args[0] == "knowledge" {
		dir := os.Getenv("OPSCOPILOT_KNOWLEDGE_DIR")
		v, e := knowledge(args[1:], dir)
		return v, "", e
	}
	if args[0] == "driver" {
		v, e := driverCommand(ctx, args[1:], input)
		return v, "", e
	}
	if args[0] == "session" {
		if len(args) < 2 {
			return nil, "", errors.New("session 需要 init/status/close")
		}
		var id, environment string
		err := flags("session", args[2:], func(f *flag.FlagSet) {
			f.StringVar(&id, "session", "", "平台 Session 标识")
			if args[1] == "init" {
				f.StringVar(&environment, "environment", "", "业务环境")
			}
		})
		if err != nil {
			return nil, "", err
		}
		if err := requireOptions("--session", id); err != nil {
			return nil, "", err
		}
		if args[1] == "init" {
			if err := requireOptions("--environment", environment); err != nil {
				return nil, "", err
			}
		}
		store, err := storeFromEnvironment()
		if err != nil {
			return nil, "", err
		}
		switch args[1] {
		case "init":
			state, e := store.InitContext(ctx, id, environment)
			return opssession.Public(state), "", e
		case "status":
			state, e := store.InspectContext(ctx, id)
			return opssession.Public(state), "", e
		case "close":
			closeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			e := store.Close(closeCtx, id)
			return map[string]any{"closed": e == nil}, "", e
		default:
			return nil, "", errors.New("未知 Session 操作")
		}
	}
	if args[0] == "connection" {
		if len(args) < 2 {
			return nil, "", errors.New("connection 需要 put/list")
		}
		var id, path string
		err := flags("connection", args[2:], func(f *flag.FlagSet) {
			f.StringVar(&id, "session", "", "平台 Session 标识")
			if args[1] == "put" {
				f.StringVar(&path, "input", "", "JSON 输入")
			}
		})
		if err != nil {
			return nil, "", err
		}
		if err := requireOptions("--session", id); err != nil {
			return nil, "", err
		}
		if args[1] == "put" {
			if err := requireOptions("--input", path); err != nil {
				return nil, "", err
			}
		}
		store, err := storeFromEnvironment()
		if err != nil {
			return nil, "", err
		}
		switch args[1] {
		case "list":
			state, e := store.InspectContext(ctx, id)
			if e == nil && state.Closed {
				e = opssession.ErrClosed
			}
			return opssession.Public(state), "", e
		case "put":
			var node opssession.Node
			if e := decodeInput(path, input, &node); e != nil {
				return nil, "", e
			}
			saved, e := store.PutContext(ctx, id, node)
			if e != nil {
				return nil, "", e
			}
			public := opssession.Public(opssession.State{Nodes: []opssession.Node{saved}})
			return public["nodes"], "", nil
		default:
			return nil, "", errors.New("未知连接操作")
		}
	}
	var session, node, command, directory, user, elevation, remotePath, localPath, sqlText, sqlFile, database, table string
	seconds, line, limit, maxBytes := 120, 500, 1000, 10<<20
	sub := ""
	rest := args[1:]
	if args[0] == "file" || args[0] == "database" {
		if len(rest) == 0 {
			return nil, "", errors.New("缺少子命令")
		}
		sub, rest = rest[0], rest[1:]
	}
	err := flags(args[0], rest, func(f *flag.FlagSet) {
		f.StringVar(&session, "session", "", "平台 Session 标识")
		f.StringVar(&node, "node", "", "连接节点标识")
		f.IntVar(&seconds, "timeout-sec", 120, "超时秒数（1–600）")
		switch args[0] {
		case "exec":
			f.StringVar(&command, "command", "", "远端命令")
			f.StringVar(&directory, "directory", "", "远端绝对目录")
			f.StringVar(&user, "as-user", "", "执行账号")
			f.StringVar(&elevation, "elevation", "", "su 或 sudo")
			f.IntVar(&line, "max-line-length", 500, "长行裁剪字节数")
		case "file":
			f.StringVar(&remotePath, "remote", "", "远端文件")
			f.StringVar(&localPath, "local", "", "Session 工作区内相对文件名")
			f.IntVar(&maxBytes, "max-bytes", 10<<20, "下载大小限制")
		case "database":
			f.StringVar(&sqlText, "sql", "", "只读 SQL")
			if sub == "query" {
				f.StringVar(&sqlFile, "sql-file", "", "SQL 文件，- 表示标准输入")
			}
			f.StringVar(&database, "database", "", "目标库")
			f.StringVar(&table, "table", "", "目标表")
			f.IntVar(&limit, "limit", 1000, "最大返回行数")
		}
	})
	if err != nil {
		return nil, "", err
	}
	if seconds < 1 || seconds > 600 || line < 400 || line > 65536 {
		return nil, "", errors.New("超时必须为 1–600 秒，长行限制为 400–65536 字节")
	}
	if args[0] != "exec" && args[0] != "file" && args[0] != "database" {
		return nil, "", errors.New("未知命令，请查看 help")
	}
	if err := requireOptions("--session", session, "--node", node); err != nil {
		return nil, "", err
	}
	switch args[0] {
	case "exec":
		if err := requireOptions("--command", command); err != nil {
			return nil, "", err
		}
	case "file":
		if err := validateDownloadOptions(remotePath, localPath, maxBytes); err != nil {
			return nil, "", err
		}
	case "database":
		if seconds > 120 || limit < 1 || limit > 5000 {
			return nil, "", errors.New("数据库超时必须为 1–120 秒，返回行数必须为 1–5000")
		}
		if sub == "structure" {
			if err := requireOptions("--table", table); err != nil {
				return nil, "", err
			}
		}
		if sub == "query" {
			// Parse only option names; values may themselves start with '-'.
			if optionPresent(rest, "sql") && optionPresent(rest, "sql-file") {
				return nil, "", errors.New("--sql 与 --sql-file 不能同时提供")
			}
			if sqlFile != "" {
				sqlText, err = readSQL(sqlFile, input)
				if err != nil {
					return nil, "", err
				}
			}
			if strings.TrimSpace(sqlText) == "" {
				return nil, "", errors.New("query 需要 --sql 或 --sql-file（- 表示从 stdin 读取）")
			}
		}
	}
	store, err := storeFromEnvironment()
	if err != nil {
		return nil, "", err
	}
	ctx, finish, err := store.Begin(ctx, session)
	if err != nil {
		return nil, "", err
	}
	defer finish()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancel()
	target, err := store.NodeContext(ctx, session, node)
	if err != nil {
		return nil, "", err
	}
	secrets := nodeSecrets(target)
	if target.GatewayID != "" {
		if gateway, e := store.NodeContext(ctx, session, target.GatewayID); e == nil {
			secrets = append(secrets, nodeSecrets(gateway)...)
		}
	}
	redact := func(err error) error {
		if err == nil {
			return nil
		}
		message := hideCredentials(err.Error(), secrets)
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return redactedError{message: message, cause: err}
	}
	switch args[0] {
	case "exec":
		if target.Kind != "ssh" {
			return nil, "", errors.New("执行命令需要 SSH 节点")
		}
		policy, err := security.LoadWhitelistManager(os.Getenv("OPSCOPILOT_WHITELIST_PATH"))
		if err != nil {
			return nil, "", err
		}
		options := ops.OnceOptions{Directory: directory, User: user, Elevation: elevation, Timeout: time.Duration(seconds) * time.Second, OutputBytes: 10240, LineBytes: line}
		request, err := ops.PrepareOnce(target.SSH, command, options, policy)
		if err != nil {
			return nil, "", redact(err)
		}
		conn, err := sshclient.NewClientWithTrace(ctx, target.SSH)
		if err != nil {
			return nil, connectionErrorStatus(err), redact(err)
		}
		defer conn.Close()
		stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
		defer stop()
		value, err := ops.ExecuteOnce(ctx, conn, target.SSH, request, options)
		value.Stdout = redactText(value.Stdout, nodeSecrets(target))
		value.Stderr = redactText(value.Stderr, nodeSecrets(target))
		return value, value.Status, redact(err)
	case "file":
		if sub != "download" {
			return nil, "", errors.New("服务端文件操作仅支持 download")
		}
		value, err := download(ctx, store, session, target, remotePath, localPath, maxBytes)
		return value, "", redact(err)
	case "database":
		value, err := queryDatabase(ctx, store, session, target, sub, database, table, sqlText, limit, seconds)
		return value, "", redact(err)
	}
	return nil, "", errors.New("未知操作")
}
func connectionErrorStatus(err error) string {
	if strings.Contains(err.Error(), "unable to authenticate") {
		return "authentication_failed"
	}
	return errorStatus(err)
}
func nodeSecrets(n opssession.Node) []string {
	var out []string
	if n.Password != nil {
		out = append(out, *n.Password)
	}
	if n.SSH != nil {
		for c := n.SSH; c != nil; c = c.Bastion {
			out = append(out, c.Password, c.RootPassword)
		}
	}
	return out
}
func redactText(value string, secrets []string) string {
	value = hideCredentials(value, secrets)
	return assistantcontrol.DisplayExcerpt(value, 16<<10)
}
