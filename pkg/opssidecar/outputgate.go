package opssidecar

import "sync"

// outputGate 会话首段输出闸门。
//
// 背景：连接建立瞬间远端往往立刻吐出一批输出（SSH motd/登录 shell 欢迎语/
// 网络设备初始菜单），而前端要等 open 响应、创建终端组件、挂上数据监听后
// 才能真正收到。这段窗口内事件因无监听者而直接丢弃——首屏信息就此丢失。
// 闸门在"前端监听就绪"之前暂存输出，前端挂好终端后发 terminal/ready 一次性
// 冲刷，保证首段输出零丢失。
//
// 安全阀：暂存超过 outputGateMax 后不再累积、直接透传（防止前端迟迟不就绪
// 时无限积压或永久卡住首段输出）；markReady 幂等。
//
// 语义自 OpsCopilot app.go 的 terminalOutputGate 原样上移（Issue #74 的
// 乱序修正在内）：deliver 与 markReady 共用同一把锁，保证「旧暂存段冲刷」
// 与「新输出直通」绝不交错。
type outputGate struct {
	mu     sync.Mutex
	closed bool
	ready  bool
	buf    [][]byte
	bufLen int
}

const outputGateMax = 512 << 10 // 512KB

func newOutputGate() *outputGate {
	return &outputGate{}
}

// close 会话关闭或被替换时丢弃旧暂存段。幂等。
func (g *outputGate) close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.closed = true
	g.buf = nil
	g.bufLen = 0
}

// deliver 生产路径的统一出口：未就绪暂存；就绪后在锁内经 emit 直通。
// 与 markReady 共用同一把锁，保证冲刷段与新输出严格按到达顺序到达前端。
func (g *outputGate) deliver(data []byte, emit func([]byte)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return
	}
	if !g.ready && g.bufLen+len(data) <= outputGateMax {
		g.buf = append(g.buf, data)
		g.bufLen += len(data)
		return
	}
	emit(data)
}

// markReady 标记前端监听已就绪，返回需冲刷的暂存输出（按到达顺序）。
// 幂等：第二次调用返回 nil。调用方负责按序 emit 返回段。
func (g *outputGate) markReady() [][]byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed || g.ready {
		return nil
	}
	g.ready = true
	out := g.buf
	g.buf = nil
	g.bufLen = 0
	return out
}
