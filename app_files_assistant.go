package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/assistanttools"
	"opscopilot/pkg/filecontrol"
	"opscopilot/pkg/filetransfer"
)

func (a *App) fileControlService() *filecontrol.Service {
	a.fileControlOnce.Do(func() {
		dir := "."
		if a.configMgr != nil {
			dir = a.configMgr.Directory()
		}
		a.fileControl = filecontrol.New(appFileBackend{a}, &filecontrol.FileJournal{Path: filepath.Join(dir, "audit", "files", "operations.jsonl")})
	})
	return a.fileControl
}

type FileWorkspaceUpdate struct {
	Revision uint64                  `json:"revision"`
	Items    []filecontrol.Workspace `json:"items"`
}

func (a *App) FileWorkspacePoll(client string, since uint64) (FileWorkspaceUpdate, error) {
	if err := a.workspaceService().Registered(client); err != nil {
		return FileWorkspaceUpdate{}, err
	}
	r, w := a.fileControlService().Poll(since)
	if a.workspaceService().Check(client) != nil {
		for i := range w {
			w[i].Pending = ""
		}
	}
	return FileWorkspaceUpdate{r, w}, nil
}
func (a *App) FileWorkspacePresented(client, id, nonce, connection string) error {
	if err := a.workspaceService().Check(client); err != nil {
		return err
	}
	return a.fileControlService().Acknowledge(id, nonce, connection)
}
func (a *App) FileWorkspaceTakeover(id string) error {
	w, err := a.fileControlService().Snapshot(id)
	if err != nil {
		return err
	}
	if w.OwnerTurn != "" {
		return a.assistantCoordinator().Stop(w.OwnerTurn)
	}
	return nil
}
func (a *App) FileWorkspaceDismiss(id string) error { return a.fileControlService().Dismiss(id) }
func (a *App) resolveFileEndpoint(ctx context.Context, id string) (filecontrol.Endpoint, error) {
	if ctx.Err() != nil {
		return filecontrol.Endpoint{}, ctx.Err()
	}
	inventory, err := a.workspaceInventory(ctx)
	if err != nil {
		return filecontrol.Endpoint{}, err
	}
	for _, tab := range inventory.Tabs {
		if tab.TabID != id || tab.Status != "connected" {
			continue
		}
		cfg, ok := a.getConfig(id)
		if !ok {
			return filecontrol.Endpoint{}, errors.New("SSH 连接配置已失效")
		}
		if cfg.Protocol != "" && cfg.Protocol != "ssh" {
			return filecontrol.Endpoint{}, errors.New("此连接不支持文件传输")
		}
		identity := cfg.User + "（登录身份）"
		if cfg.RootPassword != "" {
			identity = "root（使用已配置的文件提权通道）"
		}
		return filecontrol.Endpoint{SessionID: id, ConnectionID: tab.ConnectionID, Title: tab.Title, Identity: identity}, nil
	}
	return filecontrol.Endpoint{}, errors.New("请选择一个已连接的 SSH 终端；不能使用未连接或已替换的会话")
}
func (a *App) assistantFileTools(turn *assistantcontrol.Turn) *assistanttools.FileTools {
	return &assistanttools.FileTools{Turn: turn, Service: a.fileControlService(), Resolve: a.resolveFileEndpoint, Inventory: func(ctx context.Context) (any, error) {
		inventory, err := a.workspaceInventory(ctx)
		if err != nil {
			return nil, err
		}
		_, items := a.fileControlService().Poll(0)
		summaries := []map[string]any{}
		for _, w := range items {
			if !w.Dismissed {
				summaries = append(summaries, map[string]any{"id": w.ID, "endpoint": w.Endpoint, "ownerTurn": w.OwnerTurn, "taskCount": len(w.Tasks)})
			}
		}
		return map[string]any{"sessions": inventory.Tabs, "workspaces": summaries}, nil
	}}
}

type appFileBackend struct{ app *App }

func (b appFileBackend) Check(ctx context.Context, e filecontrol.Endpoint) error {
	actual, err := b.app.resolveFileEndpoint(ctx, e.SessionID)
	if err != nil {
		return err
	}
	if actual != e {
		return filecontrol.ErrStale
	}
	return nil
}
func (b appFileBackend) List(ctx context.Context, e filecontrol.Endpoint, p string, local bool) ([]filetransfer.Entry, error) {
	if err := b.Check(ctx, e); err != nil {
		return nil, err
	}
	if local {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		entries, err := f.ReadDir(4097)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		out := []filetransfer.Entry{}
		for _, entry := range entries {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			info, err := entry.Info()
			if err != nil {
				return nil, err
			}
			out = append(out, filetransfer.Entry{Path: filepath.Join(p, entry.Name()), Name: entry.Name(), IsDir: entry.IsDir(), Size: info.Size(), Mode: uint32(info.Mode()), ModTime: info.ModTime()})
		}
		return out, nil
	}
	var reply ftResponse
	if err := json.Unmarshal([]byte(b.app.ftListContext(ctx, e.SessionID, p)), &reply); err != nil {
		return nil, err
	}
	if !reply.OK {
		if reply.Error != nil {
			return nil, reply.Error
		}
		return nil, errors.New("目录读取失败")
	}
	return reply.Entries, nil
}
func (b appFileBackend) Transfer(ctx context.Context, e filecontrol.Endpoint, task filecontrol.Task, progress func(filetransfer.Progress)) (filetransfer.TransferResult, error) {
	if err := b.Check(ctx, e); err != nil {
		return filetransfer.TransferResult{}, err
	}
	// Exact file destinations only: directory merge/recursive transfer is not
	// implicitly granted by approving a software/log archive file.
	if task.Direction == "upload" {
		info, err := os.Stat(task.LocalPath)
		if err != nil {
			return filetransfer.TransferResult{}, err
		}
		if !info.Mode().IsRegular() {
			return filetransfer.TransferResult{}, errors.New("本轮仅支持普通文件传输，请先打包目录")
		}
	}
	var remote ftResponse
	if err := json.Unmarshal([]byte(b.app.ftStatContext(ctx, e.SessionID, task.RemotePath)), &remote); err != nil {
		return filetransfer.TransferResult{}, err
	}
	if remote.OK && remote.Entry != nil {
		if remote.Entry.IsDir {
			return filetransfer.TransferResult{}, errors.New("远端路径是目录，请提供完整文件路径")
		}
		if !os.FileMode(remote.Entry.Mode).IsRegular() {
			return filetransfer.TransferResult{}, errors.New("本轮仅支持普通文件")
		}
	} else if task.Direction == "download" || remote.Error == nil || remote.Error.Code != filetransfer.ErrorCodeNotFound {
		if remote.Error != nil {
			return filetransfer.TransferResult{}, remote.Error
		}
		return filetransfer.TransferResult{}, errors.New("无法核实远端文件")
	}
	finished := make(chan map[string]any, 1)
	observe := func(event string, data map[string]any) {
		if event == "file-transfer-done" {
			select {
			case finished <- data:
			default:
			}
		} else {
			p := filetransfer.Progress{}
			if n, ok := data["bytesDone"].(int64); ok {
				p.BytesDone = n
			}
			if n, ok := data["bytesTotal"].(int64); ok {
				p.BytesTotal = n
			}
			if v, ok := data["step"].(string); ok {
				p.Step = v
			}
			progress(p)
		}
	}
	var started ftResponse
	raw := b.app.startFileTransferTaskContext(ctx, e.SessionID, task.Direction, task.LocalPath, task.RemotePath, task.ID, func() error { return b.Check(ctx, e) }, observe)
	if err := json.Unmarshal([]byte(raw), &started); err != nil {
		return filetransfer.TransferResult{}, err
	}
	if !started.OK {
		if started.Error != nil {
			return filetransfer.TransferResult{}, started.Error
		}
		return filetransfer.TransferResult{}, errors.New("传输未启动")
	}
	// The transport must settle before releasing the tab. A cancelled caller
	// never detaches a still-writing job and reports it as completed.
	result := <-finished
	if ok, _ := result["ok"].(bool); !ok {
		return filetransfer.TransferResult{}, fmt.Errorf("文件传输未完成: %v", result["message"])
	}
	n, _ := result["bytes"].(int64)
	return filetransfer.TransferResult{Bytes: n}, nil
}
