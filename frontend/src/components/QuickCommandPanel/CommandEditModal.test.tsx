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
