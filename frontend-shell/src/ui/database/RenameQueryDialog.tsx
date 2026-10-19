import { useState } from 'react';
import { DatabaseDialog } from './DatabaseDialog';

export function RenameQueryDialog({
  title,
  onSave,
  onClose,
}: {
  title: string;
  onSave(title: string): void;
  onClose(): void;
}) {
  const [name, setName] = useState(title);
  return (
    <DatabaseDialog
      compact
      title="重命名查询"
      onClose={onClose}
      onSubmit={() => {
        if (name.trim()) {
          onSave(name.trim());
          onClose();
        }
      }}
      actions={
        <>
          <button type="button" onClick={onClose}>
            取消
          </button>
          <button
            className="oc-db-primary"
            type="submit"
            disabled={!name.trim()}
          >
            保存
          </button>
        </>
      }
    >
      <label>
        查询名称
        <input
          autoFocus
          data-db-initial-focus
          required
          value={name}
          maxLength={60}
          onFocus={(e) => e.target.select()}
          onChange={(e) => setName(e.target.value)}
        />
      </label>
    </DatabaseDialog>
  );
}
