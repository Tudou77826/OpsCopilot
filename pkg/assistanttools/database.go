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
	Review    assistantcontrol.Reviewer
}

func (d *DatabaseTools) Close() {
	if d.current != "" {
		d.Service.Release(d.current, d.Turn.ID())
	}
	d.lease = nil
}
func (d *DatabaseTools) Call(ctx context.Context, name, argument string) (value any, callErr error) {
	recorded := false
	defer func() {
		if callErr != nil && !recorded && !hasCallReceipt(ctx) {
			if errors.Is(callErr, assistantcontrol.ErrActionDenied) {
				d.Turn.ObserveOperation("database-error-"+fmt.Sprint(len(d.Turn.Snapshot().Messages)), "数据库操作", assistantcontrol.Operation{Kind: "database", Status: "denied"})
				return
			}
			d.Turn.ObserveOperation("database-error-"+fmt.Sprint(len(d.Turn.Snapshot().Messages)), "数据库操作未完成", assistantcontrol.Operation{Kind: "database", Status: "failed", Stage: databaseStage(name), Outcome: "此步骤未完成；此前已完成的操作仍保留。", Next: "展开诊断详情核对原因，再决定下一步。", Detail: callErr.Error()})
		}
	}()
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
					d.Turn.ObserveOperation("database-"+w.ID, "数据库页签已打开 · "+w.Profile.Name, assistantcontrol.Operation{Kind: "database", WorkspaceID: w.ID, Status: "completed"})
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
			return nil, fmt.Errorf("%w：本轮已拒绝这个数据库页签的授权", assistantcontrol.ErrActionDenied)
		}
		if !d.consented[w.ID] {
			if err = d.confirm(ctx, "允许助手使用这个数据库页签？", "允许读取该会话的表结构、查询区与结果，并执行常规查询；数据修改和未知 SQL 仍逐次确认。", w.Profile.Name, ""); err != nil {
				if errors.Is(err, assistantcontrol.ErrActionDenied) {
					d.denied[w.ID] = true
				}
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
		if d.Review != nil {
			location := fmt.Sprintf("%s · %s:%d · 会话 %s / %s", w.Profile.Name, w.Profile.Host, w.Profile.Port, w.Session.ID, in.Database)
			if err = d.confirm(lease, "读取数据库信息", "只读操作："+name+"；目标表："+in.Table, location, ""); err != nil {
				return nil, err
			}
		}
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
			return nil, fmt.Errorf("%w：本轮已拒绝这条 SQL，不再重复申请", assistantcontrol.ErrActionDenied)
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
		recorded = true
		receipt := assistantcontrol.Operation{Kind: "database", WorkspaceID: w.ID, ItemID: b.ID, Status: "not_executed", Stage: "SQL 执行", Outcome: "SQL 原文已保留，尚未提交数据库。"}
		receipt.Action, receipt.Location = in.SQL, w.Profile.Name
		d.Turn.ObserveOperation(b.ID, "SQL 尚未执行", receipt)
		if !read || d.Review != nil {
			d.Service.SetBlock(w.ID, b.ID, "awaiting_approval", dbworkspace.QueryResult{}, "")
			if err = d.confirm(lease, "确认这一次数据库操作", reason, w.Profile.Name+" / "+in.Database, in.SQL); err != nil {
				d.Service.SetBlock(w.ID, b.ID, "not_executed", dbworkspace.QueryResult{}, "未获得本次批准")
				if errors.Is(err, assistantcontrol.ErrActionDenied) {
					d.deniedSQL[refusalKey] = true
					receipt.Status = "denied"
				}
				d.Turn.ObserveOperation(b.ID, "执行 SQL", receipt)
				d.Close()
				return nil, err
			}
		}
		receipt.Status, receipt.Outcome = "running", "正在准备或执行 SQL，尚未确认最终结果。"
		d.Turn.ObserveOperation(b.ID, "SQL 处理中", receipt)
		result, err := d.Service.Execute(lease, w.ID, d.Turn.ID(), b, in.Database)
		// Preserve the database's observed outcome even when final audit fails.
		observed, _ := d.Service.Snapshot(w.ID)
		for _, block := range observed.Blocks {
			if block.ID == b.ID {
				receipt.Status = block.Status
				receipt.Detail = block.Error
				break
			}
		}
		title := "SQL 执行完成"
		switch receipt.Status {
		case "completed":
			receipt.Outcome = "已收到数据库执行结果，SQL 原文和结果保留在页签中。"
		case "preparing", "not_executed", "awaiting_approval":
			title, receipt.Status, receipt.Outcome = "SQL 未提交", "not_executed", "准备阶段未完成，SQL 未提交；原文已保留。"
			receipt.Next = "查看诊断详情后再决定是否重新执行。"
		default:
			title = "SQL 执行未完成"
			receipt.Outcome = "已进入数据库执行阶段；未确认整条语句成功，也不能推定所有操作均未生效。"
			receipt.Next = "先核对 SQL 结果与事务状态，再决定是否重试。"
		}
		if err != nil {
			receipt.Detail = err.Error()
			if receipt.Status == "completed" {
				title = "SQL 已执行，结果记录异常"
				receipt.Status = "warning"
				receipt.Next = "先核对页签中的结果，不要直接重复执行。"
			}
		}
		if result.Error != "" {
			if receipt.Detail == "" {
				receipt.Detail = result.Error
			} else if receipt.Detail != result.Error {
				receipt.Detail += "\n" + result.Error
			}
		}
		receipt.Result = databaseResultPreview(result)
		if in.Purpose != "" {
			title = in.Purpose
		}
		d.Turn.ObserveOperation(b.ID, title, receipt)
		if err != nil {
			status := "failed"
			if receipt.Status == "not_executed" {
				status = "not_executed"
			}
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
func databaseStage(name string) string {
	switch name {
	case "database_open":
		return "建立数据库连接 / 呈现页签"
	case "database_create":
		return "保存连接配置"
	case "database_take":
		return "接管数据库页签"
	case "database_execute":
		return "准备 SQL"
	case "database_close":
		return "关闭数据库连接"
	}
	return "读取数据库信息"
}
func (d *DatabaseTools) confirm(ctx context.Context, title, description, location, sql string) error {
	humanReason := ""
	if sql != "" && dbworkspace.AssistantSQLRequiresHuman(sql) {
		humanReason = "SQL 涉及业务数据或结构修改、敏感访问或其他明确风险，需要人工确认"
	}
	if title == "关闭这个数据库连接？" {
		humanReason = "关闭连接可能回滚未提交事务，需要人工确认"
	}
	return d.Turn.Authorize(ctx, d.Review, assistantcontrol.ReviewInput{Tool: "database", Location: location, Action: title + "\n" + description + "\n" + sql}, assistantcontrol.Prompt{Kind: "approval", Title: title, Description: description, Command: sql, Details: []assistantcontrol.Detail{{Label: "执行位置", Value: location}}, Choices: []assistantcontrol.Choice{{ID: "reject", Label: "取消"}, {ID: "allow", Label: "允许本次"}}, Expires: time.Now().Add(5 * time.Minute)}, humanReason)
}
