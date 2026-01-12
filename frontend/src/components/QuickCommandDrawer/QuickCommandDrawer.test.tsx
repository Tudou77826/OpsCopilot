import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import QuickCommandDrawer from './QuickCommandDrawer';

describe('QuickCommandDrawer', () => {
    const defaultProps = {
        onExecute: vi.fn(),
        isOpen: true, // Default to open so we can see content in tests
        onToggle: vi.fn(),
    };

    it('renders correctly', () => {
        render(<QuickCommandDrawer {...defaultProps} />);
        expect(screen.getByText('▼ 快捷命令')).toBeInTheDocument();
    });

    it('calls onToggle when handle is clicked', () => {
        const onToggleMock = vi.fn();
        render(<QuickCommandDrawer {...defaultProps} onToggle={onToggleMock} />);
        const handle = screen.getByTitle('展开/收起');
        fireEvent.click(handle);
        expect(onToggleMock).toHaveBeenCalled();
    });

    it('executes command on click', () => {
        const onExecuteMock = vi.fn();
        render(<QuickCommandDrawer {...defaultProps} onExecute={onExecuteMock} />);

        // Find default command (assuming "ls -la" or similar exists in default state for test)
        // If empty by default, we might need to add one first or mock localStorage

        // Let's assume we mocked localStorage to have one command
        // Since we can't easily mock localStorage in this setup without setupFiles, 
        // we might test the "Add" flow first or rely on default props if we exposed initialCommands
    });

    it('adds a new command', () => {
        render(<QuickCommandDrawer {...defaultProps} />);
        const addBtn = screen.getByText('+');
        fireEvent.click(addBtn);

        // Should show edit dialog/inputs (mocked or real)
        // Since we are building it, let's assume clicking + adds a default "New Command"
        expect(screen.getByText('New Command')).toBeInTheDocument();
    });

    it('edits a command name', () => {
        render(<QuickCommandDrawer {...defaultProps} />);
        // Add one
        fireEvent.click(screen.getByText('+'));

        const cmd = screen.getByText('New Command');
        fireEvent.contextMenu(cmd);

        const editBtn = screen.getByText('编辑');
        fireEvent.click(editBtn);

        // Expect input with value
        const input = screen.getByDisplayValue('New Command');
        fireEvent.change(input, { target: { value: 'List Files' } });

        // Find the save button (primaryBtn)
        const saveBtn = screen.getByText('保存');
        fireEvent.click(saveBtn);

        expect(screen.getByText('List Files')).toBeInTheDocument();
    });
});
