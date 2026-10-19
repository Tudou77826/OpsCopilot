import { useState } from 'react';
import { DatabaseDialog } from './DatabaseDialog';
import { errorText } from './workspaceModel';

export type FolderAction =
  { kind: 'create' } | { kind: 'rename' | 'delete'; name: string };
export function DatabaseFolderDialog({
  action,
  count,
  onSave,
  onClose,
}: {
  action: FolderAction;
  count: number;
  onSave(name: string): Promise<void>;
  onClose(): void;
}) {
  const [name, setName] = useState(action.kind === 'create' ? '' : action.name);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const deleting = action.kind === 'delete';
  const title = deleting
    ? '删除连接分组'
    : action.kind === 'create'
      ? '新建连接分组'
      : '重命名连接分组';
  return (
    <DatabaseDialog
      compact
      title={title}
      busy={busy}
      onClose={onClose}
      onSubmit={() => {
        setBusy(true);
        setError('');
        void onSave(name.trim())
          .then(onClose)
          .catch((e) => {
            setError(errorText(e));
            setBusy(false);
          });
      }}
      feedback={
        error && (
          <p role="alert" className="oc-db-error">
            {error}
          </p>
        )
      }
      actions={
        <>
          <button
            type="button"
            autoFocus={deleting}
            data-db-initial-focus={deleting || undefined}
            disabled={busy}
            onClick={onClose}
          >
            取消
          </button>
          <button
            type="submit"
            className={deleting ? 'oc-db-danger' : 'oc-db-primary'}
            disabled={busy || (!deleting && !name.trim())}
          >
            {busy
              ? '正在保存…'
              : deleting
                ? '删除分组'
                : action.kind === 'create'
                  ? '创建'
                  : '保存'}
          </button>
        </>
      }
    >
      {deleting ? (
        <p>
          删除分组「{name}」？
          {count > 0
            ? `其中的 ${count} 个连接将移回顶层，连接配置与会话都会保留。`
            : '这是一个空分组。'}
        </p>
      ) : (
        <label>
          分组名称
          <input
            autoFocus
            data-db-initial-focus
            required
            maxLength={100}
            value={name}
            disabled={busy}
            placeholder="例如：生产环境"
            onChange={(e) => {
              setName(e.target.value);
              setError('');
            }}
          />
        </label>
      )}
    </DatabaseDialog>
  );
}
