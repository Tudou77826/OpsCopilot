package main

import (
	"fmt"
	"log/slog"
	"os"

	"opscopilot/pkg/garden"
)

// 桌面壳的花园接入：与 Teams 插件共用同一份 pkg/garden 存储与前端呈现组件，
// 这里只补桌面特有的 Wails 绑定与业务结果处的事件钩子。
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
	return a.garden.ReadSnapshot()
}

// GardenSignal 返回入口轻提示所需的轻量 revision，不加载收藏清单或场景资源。
func (a *App) GardenSignal() (*garden.ChangeSignal, error) {
	if a.garden == nil {
		return nil, fmt.Errorf("花园能力未启用")
	}
	return a.garden.ReadSignal()
}

func (a *App) GardenPurchase(itemID string, price, initialLevel int) (*garden.PurchaseResult, error) {
	if a.garden == nil {
		return nil, fmt.Errorf("花园能力未启用")
	}
	return a.garden.Purchase(garden.ItemID(itemID), price, initialLevel)
}

func (a *App) GardenPlace(instanceID string, x, y, scale float64, flipX bool) (*garden.Snapshot, error) {
	if a.garden == nil {
		return nil, fmt.Errorf("花园能力未启用")
	}
	return a.garden.Place(instanceID, x, y, scale, flipX)
}

func (a *App) GardenStow(instanceID string) (*garden.Snapshot, error) {
	if a.garden == nil {
		return nil, fmt.Errorf("花园能力未启用")
	}
	return a.garden.Stow(instanceID)
}

// recordGarden 把一次合格业务事件交给花园结算。异步 + recover：花园是附属能力，
// 任何故障都不能影响连接/传输/回放主流程（与 recordSharedLogin 同一纪律）。
func (a *App) recordGarden(kind garden.EventKind, eventKey string) {
	if a.garden == nil || !a.garden.Enabled() {
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
