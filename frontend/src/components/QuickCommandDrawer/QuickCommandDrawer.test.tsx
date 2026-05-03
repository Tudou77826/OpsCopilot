import { render, screen, fireEvent, act } from '@testing-library/react';
import { describe, it, expect, vi, beforeAll } from 'vitest';
import QuickCommandDrawer from './QuickCommandDrawer';

describe('QuickCommandDrawer', () => {
    const defaultProps = {
        onExecute: vi.fn(),
        isOpen: true,
        onToggle: vi.fn(),
    };

    beforeAll(() => {
        // Mock Wails backend calls used by the component
        (window as any).go = {
            main: {
                App: {
                    LoadQuickCommands: vi.fn().mockResolvedValue([]),
                    SaveQuickCommands: vi.fn(),
                }
            }
        };
    });

    it('renders correctly', () => {
        render(<QuickCommandDrawer {...defaultProps} />);
        expect(screen.getByText('▼ 快捷命令')).toBeInTheDocument();
    });

    it('calls onToggle when toggle button is clicked', () => {
        const onToggleMock = vi.fn();
        render(<QuickCommandDrawer {...defaultProps} onToggle={onToggleMock} />);
        // Toggle button shows "▼ 快捷命令" when open
        const toggleBtn = screen.getByText('▼ 快捷命令');
        fireEvent.click(toggleBtn);
        expect(onToggleMock).toHaveBeenCalled();
    });

    it('executes command on click', async () => {
        const onExecuteMock = vi.fn();
        (window as any).go.main.App.LoadQuickCommands = vi.fn().mockResolvedValue([
            { id: '1', name: 'List Files', content: 'ls -la', group: 'default' }
        ]);
        render(<QuickCommandDrawer {...defaultProps} onExecute={onExecuteMock} />);

        // Wait for async load to complete and command to appear
        const cmd = await screen.findByTitle('ls -la');
        fireEvent.click(cmd);
        expect(onExecuteMock).toHaveBeenCalledWith('ls -la');
    });

    it('opens edit modal when + is clicked with default values', () => {
        render(<QuickCommandDrawer {...defaultProps} />);
        const addBtn = screen.getByText('+');
        fireEvent.click(addBtn);

        // Modal opens — "New Command" is in the input value
        expect(screen.getByDisplayValue('New Command')).toBeInTheDocument();
        expect(screen.getByText('编辑命令')).toBeInTheDocument();
    });

    it('edits a command name via modal', async () => {
        (window as any).go.main.App.LoadQuickCommands = vi.fn().mockResolvedValue([
            { id: '1', name: 'List Files', content: 'ls -la', group: 'default' }
        ]);
        render(<QuickCommandDrawer {...defaultProps} />);

        // Wait for async load
        const cmd = await screen.findByTitle('ls -la');

        // Right-click to open context menu
        fireEvent.contextMenu(cmd);

        // Click "编辑" in context menu
        const editBtn = screen.getByText('编辑');
        fireEvent.click(editBtn);

        // Modal opens with current name in input
        const nameInput = screen.getByDisplayValue('List Files') as HTMLInputElement;
        fireEvent.change(nameInput, { target: { value: 'List All Files' } });

        // Save
        const saveBtn = screen.getByText('保存');
        fireEvent.click(saveBtn);

        // After save, the card should show the new name
        expect(screen.getByText('List All Files')).toBeInTheDocument();
    });
});
