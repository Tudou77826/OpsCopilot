import { render, screen, fireEvent } from '@testing-library/react';
import LayoutManager from './LayoutManager';
import { vi, describe, it, expect } from 'vitest';

// Mock TerminalComponent
vi.mock('../Terminal/Terminal', () => ({
    default: ({ id }: { id: string }) => <div data-testid={`terminal-${id}`}>Terminal {id}</div>
}));

describe('LayoutManager', () => {
    const mockTerminals = [
        { id: 'session-1', title: 'Session 1' },
        { id: 'session-2', title: 'Session 2' },
    ];
    const mockOnTerminalData = vi.fn();
    const mockTerminalRefs = { current: new Map() };
    const mockOnCloseTerminal = vi.fn();
    const mockOnRenameTerminal = vi.fn();

    it('renders tab mode correctly', () => {
        render(
            <LayoutManager 
                terminals={mockTerminals} 
                mode="tab" 
                onTerminalData={mockOnTerminalData}
                terminalRefs={mockTerminalRefs}
                onCloseTerminal={mockOnCloseTerminal}
                onRenameTerminal={mockOnRenameTerminal}
            />
        );

        // Should show tab headers
        expect(screen.getByText('Session 1')).toBeInTheDocument();
        expect(screen.getByText('Session 2')).toBeInTheDocument();

        // Should show active terminal (session-1 default)
        const t1 = screen.getByTestId('terminal-session-1');
        expect(t1).toBeVisible();

        // Should hide inactive terminal (but still in DOM)
        const t2 = screen.getByTestId('terminal-session-2');
        expect(t2).not.toBeVisible();
    });

    it('switches tabs correctly', () => {
        render(
            <LayoutManager 
                terminals={mockTerminals} 
                mode="tab" 
                onTerminalData={mockOnTerminalData}
                terminalRefs={mockTerminalRefs}
                onCloseTerminal={mockOnCloseTerminal}
                onRenameTerminal={mockOnRenameTerminal}
            />
        );

        // Click Session 2 tab
        fireEvent.click(screen.getByText('Session 2'));

        const t1 = screen.getByTestId('terminal-session-1');
        const t2 = screen.getByTestId('terminal-session-2');

        expect(t1).not.toBeVisible();
        expect(t2).toBeVisible();
    });

    it('renders grid mode correctly with titles', () => {
        render(
            <LayoutManager 
                terminals={mockTerminals} 
                mode="grid" 
                onTerminalData={mockOnTerminalData}
                terminalRefs={mockTerminalRefs}
                onCloseTerminal={mockOnCloseTerminal}
                onRenameTerminal={mockOnRenameTerminal}
            />
        );

        // Should NOT show main tab headers
        expect(screen.queryByRole('tablist')).not.toBeInTheDocument();

        // Should show titles in grid items
        expect(screen.getByText('Session 1')).toBeInTheDocument();
        expect(screen.getByText('Session 2')).toBeInTheDocument();

        // Should show ALL terminals
        const t1 = screen.getByTestId('terminal-session-1');
        const t2 = screen.getByTestId('terminal-session-2');

        expect(t1).toBeVisible();
        expect(t2).toBeVisible();
    });

    it('closes terminal when close button is clicked', () => {
        render(
            <LayoutManager 
                terminals={mockTerminals} 
                mode="tab" 
                onTerminalData={mockOnTerminalData}
                terminalRefs={mockTerminalRefs}
                onCloseTerminal={mockOnCloseTerminal}
                onRenameTerminal={mockOnRenameTerminal}
            />
        );

        // Find close button for Session 1 (assuming it has aria-label or text '×')
        const closeBtns = screen.getAllByText('×');
        fireEvent.click(closeBtns[0]);

        expect(mockOnCloseTerminal).toHaveBeenCalledWith('session-1');
    });

    it('renames terminal on double click', () => {
        render(
            <LayoutManager 
                terminals={mockTerminals} 
                mode="tab" 
                onTerminalData={mockOnTerminalData}
                terminalRefs={mockTerminalRefs}
                onCloseTerminal={mockOnCloseTerminal}
                onRenameTerminal={mockOnRenameTerminal}
            />
        );

        // Double click the tab
        const tab = screen.getByText('Session 1');
        fireEvent.doubleClick(tab);

        // Input should appear
        const input = screen.getByDisplayValue('Session 1');
        fireEvent.change(input, { target: { value: 'New Name' } });
        
        // Submit on blur or enter
        fireEvent.blur(input);

        expect(mockOnRenameTerminal).toHaveBeenCalledWith('session-1', 'New Name');
    });
});
