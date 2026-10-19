import { useState } from 'react';
import { DatabaseDialog } from './DatabaseDialog';
import type { DatabaseProfile } from './types';
import { errorText } from './workspaceModel';

export function DatabaseGroupDialog({
  profile,
  groups,
  onSave,
  onClose,
}: {
  profile: DatabaseProfile;
  groups: string[];
  onSave(group: string): Promise<void>;
  onClose(): void;
}) {
  const [group, setGroup] = useState(profile.group ?? '');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  return (
    <DatabaseDialog
      compact
      title={`移动「${profile.name}」到分组`}
      busy={busy}
      onClose={onClose}
      onSubmit={() => {
        setBusy(true);
        setError('');
        void onSave(group.trim())
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
          <button type="button" disabled={busy} onClick={onClose}>
            取消
          </button>
          <button type="submit" className="oc-db-primary" disabled={busy}>
            {busy ? '正在保存…' : '移动'}
          </button>
        </>
      }
    >
      <label>
        分组名称
        <input
          autoFocus
          data-db-initial-focus
          disabled={busy}
          value={group}
          maxLength={100}
          list="oc-db-groups"
          placeholder="输入新分组，或留空移出分组"
          onChange={(e) => {
            setGroup(e.target.value);
            setError('');
          }}
        />
      </label>
      <datalist id="oc-db-groups">
        {groups.map((name) => (
          <option key={name} value={name} />
        ))}
      </datalist>
      <p className="oc-db-help">同名分组会自动合并，清空后连接放回顶层。</p>
    </DatabaseDialog>
  );
}
