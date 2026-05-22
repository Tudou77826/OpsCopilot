import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import '@testing-library/jest-dom';
import CommandGrid from './CommandGrid';

describe('CommandGrid', () => {
    const commands = [
        { id: '1', name: 'List Files', content: 'ls -la', group: 'default' },
        { id: '2', name: 'Check Disk', content: 'df -h', group: 'default' },
    ];

    const defaultProps = {
        commands,
        onExecute: vi.fn(),
        onEdit: vi.fn(),
        onDelete: vi.fn(),
        onAdd: vi.fn(),
    };

    it('renders command cards', () => {
        render(<CommandGrid {...defaultProps} />);
        expect(screen.getByTestId('command-grid')).toBeInTheDocument();
        expect(screen.getByText('List Files')).toBeInTheDocument();
        expect(screen.getByText('Check Disk')).toBeInTheDocument();
    });

    it('calls onExecute when card is clicked', () => {
        const onExecute = vi.fn();
        render(<CommandGrid {...defaultProps} onExecute={onExecute} />);

        fireEvent.click(screen.getByText('List Files'));
        expect(onExecute).toHaveBeenCalledWith('ls -la');
    });

    it('shows context menu on right-click', () => {
        render(<CommandGrid {...defaultProps} />);

        fireEvent.contextMenu(screen.getByText('List Files'));
        expect(screen.getByTestId('command-context-menu')).toBeInTheDocument();
        expect(screen.getByText('编辑')).toBeInTheDocument();
        expect(screen.getByText('删除')).toBeInTheDocument();
    });

    it('calls onEdit from context menu', () => {
        const onEdit = vi.fn();
        render(<CommandGrid {...defaultProps} onEdit={onEdit} />);

        fireEvent.contextMenu(screen.getByText('List Files'));
        fireEvent.click(screen.getByText('编辑'));
        expect(onEdit).toHaveBeenCalledWith(commands[0]);
    });

    it('calls onDelete from context menu', () => {
        const onDelete = vi.fn();
        render(<CommandGrid {...defaultProps} onDelete={onDelete} />);

        fireEvent.contextMenu(screen.getByText('List Files'));
        fireEvent.click(screen.getByText('删除'));
        expect(onDelete).toHaveBeenCalledWith('1');
    });

    it('calls onAdd when add button is clicked', () => {
        const onAdd = vi.fn();
        render(<CommandGrid {...defaultProps} onAdd={onAdd} />);

        fireEvent.click(screen.getByTestId('command-add-btn'));
        expect(onAdd).toHaveBeenCalled();
    });

    it('renders empty grid with only add button', () => {
        render(<CommandGrid {...defaultProps} commands={[]} />);
        expect(screen.getByTestId('command-add-btn')).toBeInTheDocument();
        expect(screen.queryByText('编辑')).not.toBeInTheDocument();
    });
});
