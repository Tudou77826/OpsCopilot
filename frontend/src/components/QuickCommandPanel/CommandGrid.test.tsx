import { render, screen, fireEvent, waitFor } from '@testing-library/react';
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
        searchQuery: '',
        onSearchChange: vi.fn(),
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

    it('calls onDelete from context menu after confirmation', async () => {
        const onDelete = vi.fn();
        // 无 React 宿主时 confirmDialog 降级为 window.confirm
        const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
        render(<CommandGrid {...defaultProps} onDelete={onDelete} />);

        fireEvent.contextMenu(screen.getByText('List Files'));
        fireEvent.click(screen.getByText('删除'));
        expect(confirmSpy).toHaveBeenCalled();
        await waitFor(() => expect(onDelete).toHaveBeenCalledWith('1'));
        confirmSpy.mockRestore();
    });

    it('does not delete when the confirmation is cancelled', () => {
        const onDelete = vi.fn();
        const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
        render(<CommandGrid {...defaultProps} onDelete={onDelete} />);

        fireEvent.contextMenu(screen.getByText('List Files'));
        fireEvent.click(screen.getByText('删除'));
        expect(onDelete).not.toHaveBeenCalled();
        confirmSpy.mockRestore();
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

    it('renders a search card that looks like other command cards', () => {
        // 搜索卡片与命令卡片同 flow，作为网格第一个元素（issue #56）
        render(<CommandGrid {...defaultProps} />);
        const search = screen.getByTestId('command-search');
        expect(search).toBeInTheDocument();
        // 含放大镜图标与可输入框
        expect(search.querySelector('input')).toBeInTheDocument();
    });

    it('calls onSearchChange when typing in the search card', () => {
        const onSearchChange = vi.fn();
        render(<CommandGrid {...defaultProps} onSearchChange={onSearchChange} />);

        const input = screen.getByTestId('command-search').querySelector('input')!;
        fireEvent.change(input, { target: { value: 'disk' } });
        expect(onSearchChange).toHaveBeenCalledWith('disk');
    });
});
