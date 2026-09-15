package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"opscopilot/pkg/garden"
)

// 桌面壳的花园接入：与 Teams 插件共用同一份 pkg/garden 存储与前端呈现组件，
// 这里只补两件桌面特有的事——Wails 绑定（快照/已读）与三个业务结果处的事件钩子。
// 事件只由后端业务结果产生，前端永不上报（与 sidecar 的能力边界一致）。

// openGarden 打开工作目录下的 garden.json（与 config.json / quick_commands.json 同目录）。
func openGarden() *garden.Store {
	store, err := garden.Open("garden.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Failed to initialize garden: %v\n", err)
		return nil
	}
	return store
}

// GardenSnapshot 返回花园只读快照（形状与 sidecar 的 shell.garden.snapshot 一致）。
func (a *App) GardenSnapshot() (*garden.Snapshot, error) {
	if a.garden == nil {
		return nil, fmt.Errorf("花园能力未启用")
	}
	return a.garden.Snapshot(), nil
}

// GardenDismiss 把一条反馈标记为已读（at 为快照里 pending 的时间戳，RFC3339）。
func (a *App) GardenDismiss(at string) error {
	if a.garden == nil {
		return fmt.Errorf("花园能力未启用")
	}
	parsed, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return fmt.Errorf("时间戳格式无效: %w", err)
	}
	a.garden.DismissAt(parsed.UnixNano())
	return nil
}

// recordGarden 把一次合格业务事件交给花园结算。异步 + recover：花园是附属能力，
// 任何故障都不能影响连接/传输/回放主流程（与 recordSharedLogin 同一纪律）。
func (a *App) recordGarden(kind garden.EventKind, eventKey string) {
	if a.garden == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("garden: 结算 panic", "kind", kind, "recover", r)
			}
		}()
		if _, err := a.garden.Record(kind, eventKey); err != nil {
			slog.Warn("garden: 事件结算失败", "kind", kind, "error", err)
		}
	}()
}
