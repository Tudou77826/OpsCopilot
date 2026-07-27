package telnetclient

import (
	"io"
	"strings"
	"sync"
)

// loginState 状态机阶段。
//
// telnet 登录是严格有序的两步:先 Login 提示(填用户名),后 Password
// 提示(填密码)。无状态机会误触发(如 Password 出现在 MOTD/banner,
// 或登录后用户敲 cat password.txt)。本状态机保证:
//   - state 0:只响应 login/username 提示 → 填用户名 → 进入 state 1
//   - state 1:只响应 password 提示 → 填密码 → 进入 state 2(终态)
//   - state 2:停止匹配,避免登录后的命令误触发回填
type loginState int

const (
	stateWaitLogin    loginState = iota // 等待 login:/username: 提示
	stateWaitPassword                   // 等待 password: 提示
	stateDone                           // 登录流程完成,停止匹配
)

// loginKeywords / passwordKeywords 为登录提示的关键字。
// 全部小写存储;匹配时对数据 ToLower 后 Contains,大小写不敏感,
// 覆盖 "login:"/"Login:"/"LOGIN:" 等变体。
var (
	loginKeywords = []string{"login:", "username:", "user:"}
	// password 提示除了英文还覆盖中文全角冒号(部分国产设备)。
	// 注意:不匹配裸 "password"(无冒号),避免 "password.txt" 误触发;
	// 但 "password：" 全角在正常文本里几乎不会出现,可宽松匹配。
	passwordKeywords = []string{"password:", "password："}
)

// LoginHandler 监控 stdout 数据流,在看到登录提示时自动向 stdin 回填凭据。
//
// 设计照搬 sshclient.AutoSudoReader 的 io.Reader 装饰器 + 异步 Handle 模式:
//   - 数据原样透传给调用方(不消费,Login:/Password: 提示仍会显示在前端);
//   - 异步 go Handle() 避免阻塞读流;
//   - Handle 入口立即 string(data) 拷贝,不跨 goroutine 持有调用方 buffer。
//
// 相对 AutoSudoReader 的增强:状态机 + stateDone 终态,防止登录后误触发。
type LoginHandler struct {
	mu       sync.Mutex
	state    loginState
	username string
	password string
	stdin    io.Writer
}

// newLoginHandler 构造一个登录回填处理器。username/password 为空时,
// 对应阶段直接跳过(不回填)。
func newLoginHandler(stdin io.Writer, username, password string) *LoginHandler {
	return &LoginHandler{
		state:    stateWaitLogin,
		username: username,
		password: password,
		stdin:    stdin,
	}
}

// Handle 处理一批刚读到的数据。并发安全(异步 go 调用)。
// 数据原样透传由调用方负责(本方法只做匹配与回填)。
func (h *LoginHandler) Handle(data []byte) {
	// 立即拷贝,避免持有调用方 buffer 引用
	s := strings.ToLower(string(data))

	h.mu.Lock()
	defer h.mu.Unlock()

	switch h.state {
	case stateWaitLogin:
		if h.username == "" {
			// 无用户名,跳过 login 阶段,直接等 password
			h.state = stateWaitPassword
			// fallthrough 不安全(锁已持),手动重试一次 password 匹配
			if h.password != "" && containsAny(s, passwordKeywords) {
				h.writePassword()
			}
			return
		}
		if containsAny(s, loginKeywords) {
			h.stdin.Write([]byte(h.username + "\n"))
			h.state = stateWaitPassword
		}
	case stateWaitPassword:
		if h.password == "" {
			h.state = stateDone
			return
		}
		if containsAny(s, passwordKeywords) {
			h.writePassword()
		}
	case stateDone:
		// 已完成登录流程,不再匹配(防止用户日后敲 password.txt 误触发)
	}
}

// writePassword 向 stdin 写入密码 + 换行。调用前需持有 h.mu。
func (h *LoginHandler) writePassword() {
	h.stdin.Write([]byte(h.password + "\n"))
	h.state = stateDone
}

// containsAny 判断 s 是否包含 keywords 中任一子串。
func containsAny(s string, keywords []string) bool {
	for _, k := range keywords {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// autoLoginReader 是套在真实 stdout 上的 io.Reader 装饰器:
// 透传数据的同时,异步触发 LoginHandler.Handle 做关键字匹配。
type autoLoginReader struct {
	reader  io.Reader
	handler *LoginHandler
}

func (r *autoLoginReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		// 异步处理,避免阻塞读流(照搬 sshclient.AutoSudoReader 的并发模式)
		go r.handler.Handle(append([]byte(nil), p[:n]...))
	}
	return n, err
}
