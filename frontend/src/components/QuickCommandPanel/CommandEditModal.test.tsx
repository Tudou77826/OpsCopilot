import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import '@testing-library/jest-dom';
import CommandEditModal from './CommandEditModal';

describe('CommandEditModal', () => {
    const defaultProps = {
        isOpen: true,
        command: { id: '1', name: 'Test Cmd', content: 'echo test', group: 'default' },
        isNew: false,
        availableGroups: ['default', 'nginx'],
        onSave: vi.fn(),
        onCancel: vi.fn(),
    };

    it('renders modal when open', () => {
        render(<CommandEditModal {...defaultProps} />);
        expect(screen.getByTestId('command-edit-modal')).toBeInTheDocument();
        expect(screen.getByText('编辑命令')).toBeInTheDocument();
    });

    it('shows "新建命令" for new command', () => {
        render(<CommandEditModal {...defaultProps} isNew={true} />);
        expect(screen.getByText('新建命令')).toBeInTheDocument();
    });

    it('renders form fields', () => {
        render(<CommandEditModal {...defaultProps} />);
        expect(screen.getByTestId('command-name-input')).toBeInTheDocument();
        expect(screen.getByTestId('command-group-select')).toBeInTheDocument();
        expect(screen.getByTestId('command-content-textarea')).toBeInTheDocument();
    });

    it('calls onCancel when cancel button is clicked', () => {
        const onCancel = vi.fn();
        render(<CommandEditModal {...defaultProps} onCancel={onCancel} />);

        fireEvent.click(screen.getByTestId('command-edit-cancel'));
        expect(onCancel).toHaveBeenCalled();
    });

    it('does not render when closed', () => {
        render(<CommandEditModal {...defaultProps} isOpen={false} />);
        expect(screen.queryByTestId('command-edit-modal')).not.toBeInTheDocument();
    });
});

describe('CommandEditModal new group flow', () => {
    const base = {
        isOpen: true,
        isNew: true,
        availableGroups: ['default', 'nginx'],
        onSave: vi.fn(),
        onCancel: vi.fn(),
    };

    it('enters group-name input mode directly when launched from the strip "+" (group=__new__)', () => {
        render(<CommandEditModal {...base} command={{ id: '9', name: '', content: '', group: '__new__' }} />);
        expect(screen.getByTestId('command-group-input')).toBeInTheDocument();
        expect(screen.getByText('新建分组')).toBeInTheDocument();
        expect(screen.queryByTestId('command-group-select')).not.toBeInTheDocument();
    });

    it('shows the select (not input) when opened for a normal new command', () => {
        render(<CommandEditModal {...base} command={{ id: '9', name: '', content: '', group: 'nginx' }} />);
        expect(screen.getByTestId('command-group-select')).toBeInTheDocument();
        expect(screen.queryByTestId('command-group-input')).not.toBeInTheDocument();
    });

    it('switches to input mode when selecting "+ 新建分组" in the select', () => {
        render(<CommandEditModal {...base} command={{ id: '9', name: '', content: '', group: 'nginx' }} />);
        fireEvent.change(screen.getByTestId('command-group-select'), { target: { value: '__new__' } });
        expect(screen.getByTestId('command-group-input')).toBeInTheDocument();
    });

    it('keeps input mode after blurring with an empty name (no silent revert)', () => {
        render(<CommandEditModal {...base} command={{ id: '9', name: '', content: '', group: '__new__' }} />);
        const input = screen.getByTestId('command-group-input');
        fireEvent.blur(input);
        expect(screen.getByTestId('command-group-input')).toBeInTheDocument();
    });

    it('blocks save when new group name is empty, allows after typing', () => {
        const onSave = vi.fn();
        render(<CommandEditModal {...base} onSave={onSave} command={{ id: '9', name: 'X', content: 'x', group: '__new__' }} />);
        fireEvent.click(screen.getByTestId('command-edit-save'));
        expect(onSave).not.toHaveBeenCalled();
        fireEvent.change(screen.getByTestId('command-group-input'), { target: { value: '数据库' } });
        fireEvent.click(screen.getByTestId('command-edit-save'));
        expect(onSave).toHaveBeenCalledTimes(1);
        expect(onSave.mock.calls[0][0].group).toBe('数据库');
    });

    it('reverts to select mode via the ✕ button', () => {
        render(<CommandEditModal {...base} command={{ id: '9', name: '', content: '', group: '__new__' }} />);
        fireEvent.click(screen.getByTestId('command-group-revert'));
        expect(screen.getByTestId('command-group-select')).toBeInTheDocument();
        expect(screen.queryByTestId('command-group-input')).not.toBeInTheDocument();
    });
});
