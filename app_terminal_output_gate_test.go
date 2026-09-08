package main

import (
	"strings"
	"testing"
)

// 业务场景：SSH 连接建立的瞬间，远端立刻吐出首帧输出（motd/登录横幅/网络设备初始菜单）。
// 此时前端还在 await Connect()、尚未挂数据监听，这些输出若直接 EventsEmit 会因无监听者
// 而丢失（历史缺陷：首帧信息丢失）。terminalOutputGate 在"监听就绪"前暂存、就绪后冲刷，
// 保证首段输出零丢失且顺序不变。

func TestTerminalOutputGate_连接即发首帧输出零丢失(t *testing.T) {
	g := newTerminalOutputGate()

	// 模拟会话建立瞬间、前端监听未就绪时的三段连续输出（如同一屏 motd）
	if g.feed("*** Welcome ***\r\n") {
		t.Fatal("监听未就绪时 feed 应暂存（返回 false），而不是直接透传")
	}
	if g.feed("[system ready]\r\n") {
		t.Fatal("监听未就绪时 feed 应暂存（返回 false），而不是直接透传")
	}
	if g.feed("root@core-switch:~# ") {
		t.Fatal("监听未就绪时 feed 应暂存（返回 false），而不是直接透传")
	}

	// 前端挂好终端后冲刷：三段必须按到达顺序、完整地交还
	flush := g.markReady()
	if got := strings.Join(flush, ""); got != "*** Welcome ***\r\n[system ready]\r\nroot@core-switch:~# " {
		t.Fatalf("冲刷内容与顺序不符: %q", got)
	}

	// 就绪后新输出直接透传，不再积压
	if !g.feed("ls") {
		t.Fatal("就绪后 feed 应直接透传（返回 true）")
	}
	if !g.feed("\r\n") {
		t.Fatal("就绪后 feed 应直接透传（返回 true）")
	}
}

func TestTerminalOutputGate_冲刷幂等(t *testing.T) {
	g := newTerminalOutputGate()
	g.feed("banner-1")
	g.feed("banner-2")
	if got := strings.Join(g.markReady(), ""); got != "banner-1banner-2" {
		t.Fatalf("首次冲刷内容不对: %q", got)
	}
	// 第二次调用不应再吐出任何数据（幂等，防止重复冲刷造成终端重复显示）
	if again := g.markReady(); len(again) != 0 {
		t.Fatalf("markReady 应幂等, 第二次返回 %v", again)
	}
	// 冲刷后紧接着的 feed 已就绪直通
	if !g.feed("after") {
		t.Fatal("冲刷后 feed 应直通")
	}
}

func TestTerminalOutputGate_容量兜底不无限积压(t *testing.T) {
	g := newTerminalOutputGate()
	// 单个 chunk 就超过上限：必须透传，不能积压
	oversize := strings.Repeat("x", terminalOutputGateMax+1)
	if !g.feed(oversize) {
		t.Fatal("首个 chunk 超过上限时应透传（返回 true）")
	}
	// 先暂存一部分，剩余可用容量放不下下一段时也要透传（不截断、不无限积压）
	g2 := newTerminalOutputGate()
	head := strings.Repeat("y", terminalOutputGateMax-1)
	if g2.feed(head) {
		t.Fatal("未超上限应暂存")
	}
	if !g2.feed("zz") {
		t.Fatal("累积将超上限时应透传，而不是把 zz 丢弃")
	}
}

func TestTerminalOutputGate_从无输出到就绪不产生脏数据(t *testing.T) {
	g := newTerminalOutputGate()
	// 远端一直沉默（如仍在握手/慢启动），前端就绪时不应冲刷出任何东西
	if flush := g.markReady(); len(flush) != 0 {
		t.Fatalf("无输出时就绪不应有冲刷数据: %v", flush)
	}
	if !g.feed("later") {
		t.Fatal("就绪后新输出直通")
	}
}
