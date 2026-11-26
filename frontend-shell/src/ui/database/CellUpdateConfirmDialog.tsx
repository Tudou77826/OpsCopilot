import { useState } from 'react';
import { TbAlertTriangle, TbCheck, TbCopy } from 'react-icons/tb';
import { DatabaseDialog } from './DatabaseDialog';
import { DatabaseError } from './DatabaseError';

export interface PendingBatchUpdate {
  rowCount: number;
  cellCount: number;
  sql: string;
  usedPrimaryKey: boolean;
  isProduction: boolean;
}

export function CellUpdateConfirmDialog({
  update,
  busy,
  error,
  onConfirm,
  onClose,
}: {
  update: PendingBatchUpdate;
  busy: boolean;
  error: string;
  onConfirm(sql: string): void;
  onClose(): void;
}) {
  const [sql, setSql] = useState(update.sql);
  const [copied, setCopied] = useState(false);

  async function copySQL() {
    try {
      await navigator.clipboard.writeText(sql);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      // ignore clipboard errors
    }
  }

  return (
    <DatabaseDialog
      title="提交数据变更"
      subtitle={`${update.rowCount} 条 UPDATE · 共修改 ${update.cellCount} 个单元格`}
      className="oc-db-update-dialog"
      busy={busy}
      onClose={onClose}
      onSubmit={() => {
        if (sql.trim()) onConfirm(sql);
      }}
      feedback={error ? <DatabaseError message={error} /> : undefined}
      actions={
        <>
          <button
            type="button"
            disabled={busy || !sql.trim()}
            onClick={() => void copySQL()}
          >
            {copied ? <TbCheck size={14} /> : <TbCopy size={14} />}
            {copied ? '已复制 SQL' : '复制 SQL'}
          </button>
          <button type="button" disabled={busy} onClick={onClose}>
            取消
          </button>
          <button
            type="submit"
            className="oc-db-primary"
            disabled={busy || !sql.trim()}
          >
            {busy ? '正在执行…' : '执行更新'}
          </button>
        </>
      }
    >
      {update.isProduction && (
        <div className="oc-db-update-warning is-danger" role="alert">
          <TbAlertTriangle size={15} />
          <span>当前为生产环境数据库，请仔细核对 WHERE 条件后再执行。</span>
        </div>
      )}
      {!update.usedPrimaryKey && (
        <div className="oc-db-update-warning" role="alert">
          <TbAlertTriangle size={15} />
          <span>
            未检测到表主键或唯一索引，已使用整行条件匹配 WHERE，若存在重复行可能会更新多条记录。
          </span>
        </div>
      )}
      <textarea
        data-db-initial-focus=""
        className="oc-db-cell-editor"
        aria-label="UPDATE SQL 预览"
        rows={Math.min(14, Math.max(5, sql.split('\n').length + 1))}
        disabled={busy}
        value={sql}
        onChange={(e) => setSql(e.target.value)}
      />
    </DatabaseDialog>
  );
}
