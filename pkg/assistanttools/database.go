package assistanttools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"opscopilot/pkg/agenttransport/mcp"
	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/dbcontrol"
	"opscopilot/pkg/dbworkspace"
)

type DatabaseTools struct {
	deniedSQL map[string]bool
	Turn      *assistantcontrol.Turn
	Service   *dbcontrol.Service
	List      func(context.Context) (any, error)
	Create    func(context.Context, mcp.DatabaseArgs) (dbworkspace.Profile, error)
	current   string
	lease     context.Context
	consented map[string]bool
	denied    map[string]bool
}

func (d *DatabaseTools) Close() {
	if d.current != "" {
		d.Service.Release(d.current, d.Turn.ID())
	}
	d.lease = nil
}
func (d *DatabaseTools) Call(ctx context.Context, name, argument string) (any, error) {
	d.Turn.SetActivity("database")
	defer d.Turn.SetActivity("")
	var in mcp.DatabaseArgs
	if json.Unmarshal([]byte(argument), &in) != nil {
		return nil, errors.New("数据库参数无效")
	}
	if ctx.Err() != nil || d.Turn.Context().Err() != nil {
		return nil, assistantcontrol.ErrStale
	}
	if d.consented == nil {
		d.consented = map[string]bool{}
		d.denied = map[string]bool{}
		d.deniedSQL = map[string]bool{}
	}
	switch name {
	case "database_list":
		return d.List(ctx)
	case "database_create":
		return d.Create(ctx, in)
	case "database_open":
		d.Close()
		profiles, err := d.Service.DB.List()
		if err != nil {
			return nil, err
		}
		for _, p := range profiles {
			if p.ID == in.ProfileID {
				if err = d.confirm(ctx, "打开独立数据库连接？", "将使用已保存配置建立新会话，不继承其他页签的事务。", p.Name+" · "+p.Host, ""); err != nil {
					return nil, err
				}
				w, err := d.Service.Open(ctx, p)
				if err == nil {
					d.current = w.ID
					err = d.Turn.ChangeTarget(assistantcontrol.Target{TabID: w.ID, ConnectionID: w.Session.ID})
				}
				return w, err
			}
		}
		return nil, errors.New("数据库配置不存在")
	case "database_take":
		d.Close()
		w, err := d.Service.Snapshot(in.WorkspaceID)
		if err != nil {
			return nil, err
		}
		if d.denied[w.ID] {
			return nil, errors.New("本轮已拒绝这个数据库页签的授权")
		}
		if !d.consented[w.ID] {
			if err = d.confirm(ctx, "允许助手使用这个数据库页签？", "允许读取该会话的表结构、查询区与结果，并执行常规查询；数据修改和未知 SQL 仍逐次确认。", w.Profile.Name, ""); err != nil {
				d.denied[w.ID] = true
				return nil, err
			}
			d.consented[w.ID] = true
		}
		d.current = w.ID
		d.lease, err = d.Service.Take(d.Turn.Context(), w.ID, d.Turn.ID())
		if err != nil {
			return nil, err
		}
		if err = d.Turn.ChangeTarget(assistantcontrol.Target{TabID: w.ID, ConnectionID: w.Session.ID}); err != nil {
			d.Close()
			return nil, err
		}
		return d.Service.Snapshot(w.ID)
	case "database_release":
		d.Close()
		return map[string]bool{"owned": false}, nil
	}
	if d.lease == nil || d.lease.Err() != nil {
		return nil, errors.New("请先使用 database_take 取得数据库页签控制权")
	}
	w, err := d.Service.Snapshot(d.current)
	if err != nil {
		return nil, err
	}
	lease, cancel := context.WithCancel(d.lease)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	if name == "database_context" || name == "database_tables" || name == "database_structure" {
		if err = d.Service.Record(lease, w.ID, d.Turn.ID(), name); err != nil {
			d.Close()
			d.Turn.Finish("failed", err.Error())
			return nil, err
		}
	}
	switch name {
	case "database_context":
		return w, nil
	case "database_tables":
		return d.Service.DB.TablesContext(lease, w.Session.ID, in.Database)
	case "database_structure":
		return d.Service.DB.StructureContext(lease, dbworkspace.TableRequest{SessionID: w.Session.ID, Database: in.Database, Table: in.Table})
	case "database_execute":
		refusalKey := fmt.Sprintf("%s:%x", w.ID, sha256.Sum256([]byte(in.SQL)))
		if d.deniedSQL[refusalKey] {
			return nil, errors.New("本轮已拒绝这条 SQL，不再重复申请")
		}
		dialect, err := d.Service.DB.Dialect(w.Session.ID)
		if err != nil {
			return nil, err
		}
		read, reason, err := dbworkspace.AssistantSQL(in.SQL, dialect)
		if err != nil {
			return nil, err
		}
		b, err := d.Service.Append(lease, w.ID, d.Turn.ID(), in.Purpose, in.SQL)
		if err != nil {
			return nil, err
		}
		if !read {
			d.Service.SetBlock(w.ID, b.ID, "awaiting_approval", dbworkspace.QueryResult{}, "")
			if err = d.confirm(lease, "确认这一次数据库操作", reason, w.Profile.Name+" / "+in.Database, in.SQL); err != nil {
				d.Service.SetBlock(w.ID, b.ID, "not_executed", dbworkspace.QueryResult{}, "未获得本次批准")
				d.deniedSQL[refusalKey] = true
				d.Close()
				return nil, err
			}
		}
		result, err := d.Service.Execute(lease, w.ID, d.Turn.ID(), b, in.Database)
		if err != nil {
			status := "failed"
			if lease.Err() != nil || result.Cancelled {
				status = "cancelled"
			}
			d.Service.SetBlock(w.ID, b.ID, status, result, err.Error())
			if errors.Is(err, dbcontrol.ErrAudit) {
				d.Close()
				d.Turn.Finish("failed", err.Error())
			}
		}
		return map[string]any{"blockId": b.ID, "result": result}, err
	case "database_close":
		if in.WorkspaceID != w.ID {
			return nil, errors.New("只能关闭当前明确接管的独立数据库连接")
		}
		if err = d.confirm(lease, "关闭这个数据库连接？", "未提交事务可能被回滚。关闭后保留连接配置、SQL 原文和结果；不会自动提交事务。", w.Profile.Name, ""); err != nil {
			return nil, err
		}
		if err = d.Service.Record(lease, w.ID, d.Turn.ID(), "close_intent"); err != nil {
			d.Close()
			return nil, err
		}
		err = d.Service.Close(lease, w.ID, d.Turn.ID())
		return map[string]bool{"closed": err == nil}, err
	}
	return nil, errors.New("未知数据库工具")
}
func (d *DatabaseTools) confirm(ctx context.Context, title, description, location, sql string) error {
	answer, err := d.Turn.Request(ctx, assistantcontrol.Prompt{Kind: "approval", Title: title, Description: description, Command: sql, Details: []assistantcontrol.Detail{{Label: "执行位置", Value: location}}, Choices: []assistantcontrol.Choice{{ID: "reject", Label: "取消"}, {ID: "allow", Label: "允许本次"}}, Expires: time.Now().Add(5 * time.Minute)})
	if err != nil {
		return err
	}
	if len(answer.Selected) != 1 || answer.Selected[0] != "allow" {
		return errors.New("用户未批准本次数据库操作")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return d.Turn.Context().Err()
}
