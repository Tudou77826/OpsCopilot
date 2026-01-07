import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import TroubleshootingPanel from './TroubleshootingPanel';

// Mock child components to simplify testing
vi.mock('./TroubleshootingStep', () => ({
    default: ({ step }: { step: string }) => <div data-testid="step">{step}</div>
}));
vi.mock('./CommandCard', () => ({
    default: ({ command }: { command: string }) => <div data-testid="command">{command}</div>
}));
vi.mock('./SessionReviewModal', () => ({
    default: ({ isOpen, onArchive }: { isOpen: boolean, onArchive: (c: string) => void }) => (
        isOpen ? <div data-testid="review-modal"><button onClick={() => onArchive('Conclusion')}>Archive</button></div> : null
    )
}));

// Mock Wails runtime calls
const mockStartSession = vi.fn();
const mockStopSession = vi.fn();
const mockAskAI = vi.fn();

window.go = {
    main: {
        App: {
            StartSession: mockStartSession,
            StopSession: mockStopSession,
            AskAI: mockAskAI,
            PolishRootCause: vi.fn(),
        }
    }
} as any;

describe('TroubleshootingPanel', () => {
    it('renders initial empty state correctly', () => {
        render(<TroubleshootingPanel onStart={vi.fn()} onStop={vi.fn()} />);
        expect(screen.getByText('开始排查')).toBeInTheDocument();
        expect(screen.getByPlaceholderText(/输入问题/i)).toBeInTheDocument();
    });

    it('starts investigation when clicking start button', async () => {
        const onStartMock = vi.fn();
        render(<TroubleshootingPanel onStart={onStartMock} onStop={vi.fn()} />);

        const input = screen.getByPlaceholderText(/输入问题/i);
        fireEvent.change(input, { target: { value: 'CPU high' } });
        
        const startBtn = screen.getByText('开始排查');
        fireEvent.click(startBtn);

        expect(onStartMock).toHaveBeenCalled();
        expect(mockStartSession).toHaveBeenCalledWith('CPU high');
        // Should show stop button
        expect(screen.getByText('结束排查')).toBeInTheDocument();
    });

    it('renders structured AI response correctly', async () => {
        // Mock AI response with structured data
        mockAskAI.mockResolvedValue(JSON.stringify({
            steps: ['Check CPU', 'Check Memory'],
            commands: [{ command: 'top', description: 'Show processes' }]
        }));

        render(<TroubleshootingPanel onStart={vi.fn()} onStop={vi.fn()} />);
        
        // Start
        fireEvent.change(screen.getByPlaceholderText(/输入问题/i), { target: { value: 'Issue' } });
        fireEvent.click(screen.getByText('开始排查'));

        // Wait for async operations (using findBy which waits)
        expect(await screen.findByText('排查思路')).toBeInTheDocument();
        expect(screen.getAllByTestId('step')).toHaveLength(2);
        expect(screen.getByTestId('command')).toHaveTextContent('top');
    });

    it('handles stop and archive flow', async () => {
        const onStopMock = vi.fn();
        render(<TroubleshootingPanel onStart={vi.fn()} onStop={onStopMock} />);
        
        // Start first
        fireEvent.change(screen.getByPlaceholderText(/输入问题/i), { target: { value: 'Issue' } });
        fireEvent.click(screen.getByText('开始排查'));

        // Click stop
        fireEvent.click(screen.getByText('结束排查'));
        
        // Input root cause (simulating the stop UI flow)
        const rootCauseInput = screen.getByPlaceholderText(/根本原因/i);
        fireEvent.change(rootCauseInput, { target: { value: 'Bug in code' } });
        
        // Confirm stop
        fireEvent.click(screen.getByText('确认结束'));

        // Should open review modal
        const archiveBtn = await screen.findByText('Archive'); // In mock modal
        fireEvent.click(archiveBtn);

        expect(mockStopSession).toHaveBeenCalled();
        expect(onStopMock).toHaveBeenCalled();
    });
});
