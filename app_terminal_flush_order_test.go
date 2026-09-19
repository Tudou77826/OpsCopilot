package main

import (
	"strings"
	"sync"
	"testing"
)

// 业务场景（Issue #74）：断链后重连，闸门里暂存的旧输出与新会话的实时输出
// 可能在同一时刻分别走「冲刷」与「直通」两条路。若二者不互斥，冲刷循环在
// 锁外逐段 emit 期间，新输出直通会插队——带光标控制码的旧暂存段落到新输出
// 之后，把新内容覆写掉（表现为重连后新输出被旧输出记录覆盖）。
// deliver / markReadyAndEmit 共用闸门锁，保证全路径严格到达序。

func TestTerminalOutputGate_冲刷与新输出直通严格有序(t *testing.T) {
	// 重连前暂存两段旧输出（如断链瞬间的残余 banner）
	g := newTerminalOutputGate()
	g.feed("A")
	g.feed("B")

	var emitted []string
	// 就绪冲刷与新输出直通并发：无论谁先拿到锁，输出都必须是 A、B、NEW
	g.markReadyAndEmit(func(s string) { emitted = append(emitted, s) })
	g.deliver("NEW", func(s string) { emitted = append(emitted, s) })

	if got := strings.Join(emitted, ","); got != "A,B,NEW" {
		t.Fatalf("顺序应为 A,B,NEW，实际 %q", got)
	}
}

func TestTerminalOutputGate_冲刷期间新输出不插队(t *testing.T) {
	// 并发压力版：暂存 A、B 后，同时启动冲刷与 NEW 直通。
	// 旧实现（调用方锁外逐段 emit + 读循环无锁直通）下 NEW 可插到 A/B 之前；
	// 新实现下无论调度如何，输出恒为 A,B,NEW。
	for i := 0; i < 500; i++ {
		g := newTerminalOutputGate()
		g.feed("A")
		g.feed("B")

		var mu sync.Mutex
		var emitted []string
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			g.markReadyAndEmit(func(s string) {
				mu.Lock()
				emitted = append(emitted, s)
				mu.Unlock()
			})
		}()
		go func() {
			defer wg.Done()
			g.deliver("NEW", func(s string) {
				mu.Lock()
				emitted = append(emitted, s)
				mu.Unlock()
			})
		}()
		wg.Wait()

		if got := strings.Join(emitted, ","); got != "A,B,NEW" {
			t.Fatalf("第 %d 轮顺序错乱: %q（新输出被旧暂存段覆盖的乱序源）", i, got)
		}
	}
}

func TestTerminalOutputGate_未就绪时断开消息跟随暂存段(t *testing.T) {
	// 断链发生在首段输出仍未冲刷时：[断开] 消息经闸门统一出口暂存，
	// 冲刷时必须排在既有暂存段之后（不能插队盖到旧输出上）。
	g := newTerminalOutputGate()
	g.feed("banner-1")
	g.deliver("\r\n[断开] 连接已关闭\r\n", func(string) {})

	var emitted []string
	g.markReadyAndEmit(func(s string) { emitted = append(emitted, s) })
	want := "banner-1\r\n[断开] 连接已关闭\r\n"
	if got := strings.Join(emitted, ""); got != want {
		t.Fatalf("断开消息应排在暂存段之后，实际 %q", got)
	}
}
