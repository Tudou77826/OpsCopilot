import { useEffect, useId, useMemo, useRef, useState } from 'react';
import {
  TbBraces,
  TbCheck,
  TbCopy,
  TbPencil,
  TbTextWrap,
  TbX,
} from 'react-icons/tb';
import { tryFormatJSON } from './resultGridUtils';

export interface CellDetailTarget {
  rowNumber: number;
  origRowIndex: number;
  colIndex: number;
  columnName: string;
  columnType: string;
  value: string | null;
}

export function CellDetailDialog({
  cell,
  canEdit = false,
  initialEditing = false,
  onEditSubmit,
  onClose,
}: {
  cell: CellDetailTarget;
  canEdit?: boolean;
  initialEditing?: boolean;
  onEditSubmit?: (newValue: string | null) => void;
  onClose(): void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  const formattedJSON = useMemo(() => tryFormatJSON(cell.value), [cell.value]);
  const [wrap, setWrap] = useState(true);
  const [prettyJSON, setPrettyJSON] = useState(() => formattedJSON !== null);
  const [copied, setCopied] = useState(false);
  const [copyError, setCopyError] = useState('');
  const [editing, setEditing] = useState(
    () => Boolean(initialEditing && canEdit && onEditSubmit),
  );
  const [draft, setDraft] = useState(cell.value ?? '');
  const [draftNull, setDraftNull] = useState(cell.value === null);

  useEffect(() => {
    const el = dialog.current;
    const previous = document.activeElement;
    el?.showModal();
    return () => {
      el?.close();
      if (previous instanceof HTMLElement && previous.isConnected) {
        previous.focus();
      }
    };
  }, []);

  const displayValue =
    cell.value === null
      ? 'NULL'
      : prettyJSON && formattedJSON !== null
        ? formattedJSON
        : cell.value;

  const meta = [
    `#${cell.rowNumber}`,
    cell.columnType || undefined,
    cell.value === null
      ? 'NULL'
      : cell.value === ''
        ? '空串'
        : `${cell.value.length} 字符`,
  ]
    .filter(Boolean)
    .join(' · ');

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(displayValue);
      setCopyError('');
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      setCopyError('复制失败，请手动选中复制');
    }
  }

  function handleSaveEdit(e: React.FormEvent) {
    e.preventDefault();
    const nextValue = draftNull ? null : draft;
    if (nextValue === cell.value) {
      setEditing(false);
      return;
    }
    onEditSubmit?.(nextValue);
  }

  return (
    <dialog
      ref={dialog}
      className="oc-db-dialog oc-db-cell-dialog"
      aria-labelledby={titleId}
      onClick={(e) => {
        if (!editing && e.target === dialog.current) onClose();
      }}
      onCancel={(e) => {
        e.preventDefault();
        onClose();
      }}
      onKeyDown={(e) => e.stopPropagation()}
    >
      <header className="oc-db-cell-head">
        <div className="oc-db-cell-title">
          <strong id={titleId} title={cell.columnName}>
            {cell.columnName}
          </strong>
          <span className="oc-db-cell-meta">{meta}</span>
        </div>
        <div className="oc-db-cell-actions">
          {!editing && (
            <>
              <button
                type="button"
                className={wrap ? 'is-active' : ''}
                aria-label="自动换行"
                aria-pressed={wrap}
                title={wrap ? '关闭自动换行' : '开启自动换行'}
                onClick={() => setWrap((v) => !v)}
              >
                <TbTextWrap size={15} />
              </button>
              <button
                type="button"
                className={prettyJSON && formattedJSON !== null ? 'is-active' : ''}
                disabled={formattedJSON === null}
                aria-label="JSON 格式化"
                aria-pressed={prettyJSON && formattedJSON !== null}
                title={
                  formattedJSON !== null
                    ? prettyJSON
                      ? '切换原始文本'
                      : '格式化 JSON'
                    : '非 JSON 内容'
                }
                onClick={() => setPrettyJSON((v) => !v)}
              >
                <TbBraces size={15} />
              </button>
              {canEdit && onEditSubmit && (
                <button
                  type="button"
                  aria-label="编辑此值"
                  title="编辑此值"
                  onClick={() => {
                    setDraft(cell.value ?? '');
                    setDraftNull(cell.value === null);
                    setEditing(true);
                  }}
                >
                  <TbPencil size={15} />
                </button>
              )}
              <button
                type="button"
                className={copied ? 'is-copied' : ''}
                aria-label="复制内容"
                title={copied ? '已复制' : '复制内容'}
                onClick={() => void handleCopy()}
              >
                {copied ? <TbCheck size={15} /> : <TbCopy size={15} />}
              </button>
              <span className="oc-db-cell-sep" aria-hidden />
            </>
          )}
          <button
            type="button"
            aria-label="关闭弹窗"
            title="关闭 (Esc)"
            onClick={onClose}
          >
            <TbX size={16} />
          </button>
        </div>
      </header>

      {editing ? (
        <form className="oc-db-cell-edit-form" onSubmit={handleSaveEdit}>
          <textarea
            autoFocus
            className="oc-db-cell-editor"
            aria-label="编辑单元格内容"
            disabled={draftNull}
            placeholder={draftNull ? '当前将写入 NULL' : '输入新值…'}
            value={draftNull ? '' : draft}
            onChange={(e) => setDraft(e.target.value)}
          />
          <footer className="oc-db-cell-edit-foot">
            <label className="oc-db-cell-null-toggle">
              <input
                type="checkbox"
                checked={draftNull}
                onChange={(e) => setDraftNull(e.target.checked)}
              />
              NULL
            </label>
            <span className="oc-db-toolbar-spacer" />
            <button type="button" onClick={() => setEditing(false)}>
              取消
            </button>
            <button type="submit" className="oc-db-primary">
              暂存修改
            </button>
          </footer>
        </form>
      ) : (
        <div className="oc-db-cell-body">
          <pre
            className={`oc-db-cell-preview${wrap ? ' is-wrap' : ''}${cell.value === null ? ' is-null' : ''}`}
            tabIndex={0}
            aria-label="单元格完整内容"
          >
            {displayValue}
          </pre>
          {copyError && <div className="oc-db-cell-err">{copyError}</div>}
        </div>
      )}
    </dialog>
  );
}
