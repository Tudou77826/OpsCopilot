package main

import (
	"sync"
	"testing"
	"time"
)

func newTestAppWithLimiter() *App {
	return &App{
		ftLimiters: make(map[string]*ftLimiter),
	}
}

// 并发获取同一会话的限流器时，同时持有槽位的任务数不得超过 maxConcurrentTransfersPerSession。
func TestFTLimiterCapsConcurrency(t *testing.T) {
	a := newTestAppWithLimiter()
	const tasks = 20

	var mu sync.Mutex
	running, peak := 0, 0
	var wg sync.WaitGroup
	for i := 0; i < tasks; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lim := a.acquireFTLimiter("sess-1")
			lim.sem <- struct{}{}
			defer func() { a.releaseFTLimiter("sess-1", lim, true) }()

			mu.Lock()
			running++
			if running > peak {
				peak = running
			}
			mu.Unlock()
			time.Sleep(20 * time.Millisecond)
			mu.Lock()
			running--
			mu.Unlock()
		}()
	}
	wg.Wait()

	if peak > maxConcurrentTransfersPerSession {
		t.Fatalf("peak concurrency %d exceeds limit %d", peak, maxConcurrentTransfersPerSession)
	}
	if peak < maxConcurrentTransfersPerSession {
		t.Fatalf("peak concurrency %d never reached limit %d, limiter may be serializing", peak, maxConcurrentTransfersPerSession)
	}
}

// 所有任务退出后，会话的限流器条目应被清理，避免 map 泄漏。
func TestFTLimiterCleanupWhenIdle(t *testing.T) {
	a := newTestAppWithLimiter()

	lim := a.acquireFTLimiter("sess-1")
	lim.sem <- struct{}{}
	a.releaseFTLimiter("sess-1", lim, true)

	if len(a.ftLimiters) != 0 {
		t.Fatalf("expected ftLimiters to be cleaned up, got %d entries", len(a.ftLimiters))
	}
}

// 排队中的任务也算引用：持有者退出时不得误删仍被排队任务引用的限流器。
func TestFTLimiterKeptWhileQueued(t *testing.T) {
	a := newTestAppWithLimiter()

	// 任务 A：占满全部槽位
	holder := a.acquireFTLimiter("sess-1")
	holder.sem <- struct{}{}
	var extras []*ftLimiter
	for i := 1; i < maxConcurrentTransfersPerSession; i++ {
		extra := a.acquireFTLimiter("sess-1")
		extra.sem <- struct{}{}
		extras = append(extras, extra)
	}

	// 任务 B：登记引用但仍在排队（未获取槽位）
	queued := a.acquireFTLimiter("sess-1")

	a.releaseFTLimiter("sess-1", holder, true)

	a.ftMu.Lock()
	_, exists := a.ftLimiters["sess-1"]
	a.ftMu.Unlock()
	if !exists {
		t.Fatal("limiter entry removed while a queued task still references it")
	}

	// 排队任务取消退出后（未获取过槽位），槽位持有者再全部退出，条目最终被清理
	a.releaseFTLimiter("sess-1", queued, false)
	for _, extra := range extras {
		a.releaseFTLimiter("sess-1", extra, true)
	}
	a.ftMu.Lock()
	_, exists = a.ftLimiters["sess-1"]
	a.ftMu.Unlock()
	if exists {
		t.Fatal("expected limiter entry removed after all holders exited")
	}
}
