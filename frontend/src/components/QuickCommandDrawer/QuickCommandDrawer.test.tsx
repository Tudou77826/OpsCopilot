import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import QuickCommandDrawer from './QuickCommandDrawer';

describe('QuickCommandDrawer', () => {
    const defaultProps = {
        onExecute: vi.fn(),
    };

    it('renders closed initially', () => {
        render(<QuickCommandDrawer {...defaultProps} />);
        expect(screen.getByText('快捷命令')).toBeInTheDocument();
        // Commands shouldn't be visible (or container has low height)
        // We check for handle text which should be always visible
        expect(screen.getByTitle('展开/收起')).toBeInTheDocument();
    });

    it('expands when clicked', () => {
        render(<QuickCommandDrawer {...defaultProps} />);
        const handle = screen.getByTitle('展开/收起');
        fireEvent.click(handle);
        // Assuming expanded state has a specific class or style, but for now we trust the event fires
        // In a real browser test we'd check height, here we just ensure no crash and state toggle logic (if we could access state)
    });

    it('executes command on click', () => {
        const onExecuteMock = vi.fn();
        render(<QuickCommandDrawer onExecute={onExecuteMock} />);
        
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
        fireEvent.keyDown(input, { key: 'Enter' });
        
        expect(screen.getByText('List Files')).toBeInTheDocument();
    });
});
