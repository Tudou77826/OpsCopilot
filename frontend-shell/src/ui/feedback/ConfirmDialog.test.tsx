import { describe, expect, it } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { ConfirmDialogInternal, confirmDialog } from './ConfirmDialog';

describe('ConfirmDialog.prompt', () => {
  it('renders in-app prompt dialog, validates required fields, and resolves submitted values', async () => {
    render(<ConfirmDialogInternal />);

    const promise = confirmDialog.prompt({
      title: '重命名标签',
      fields: [
        { key: 'name', label: '标签名称', defaultValue: '旧名称', required: true },
        { key: 'note', label: '备注', defaultValue: '' },
      ],
      confirmText: '保存',
    });

    const nameInput = await screen.findByDisplayValue('旧名称');
    const saveBtn = screen.getByRole('button', { name: '保存' });

    fireEvent.change(nameInput, { target: { value: '   ' } });
    expect(saveBtn).toBeDisabled();

    fireEvent.change(nameInput, { target: { value: '新标签' } });
    expect(saveBtn).not.toBeDisabled();

    const noteInput = screen.getByLabelText('备注');
    fireEvent.change(noteInput, { target: { value: '测试备注' } });
    fireEvent.click(saveBtn);

    await expect(promise).resolves.toEqual({ name: '新标签', note: '测试备注' });
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });

  it('resolves null when cancelled via Escape', async () => {
    render(<ConfirmDialogInternal />);

    const promise = confirmDialog.prompt({
      title: '输入名称',
      fields: [{ key: 'name', label: '名称', defaultValue: 'abc' }],
    });

    const dialog = await screen.findByRole('dialog');
    fireEvent.keyDown(dialog, { key: 'Escape' });

    await expect(promise).resolves.toBeNull();
  });
});
