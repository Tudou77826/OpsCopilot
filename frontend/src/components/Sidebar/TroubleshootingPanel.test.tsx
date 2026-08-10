import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import TroubleshootingPanel from './TroubleshootingPanel';

vi.mock('./SessionReviewModal', () => ({
    default: ({ isOpen, onArchive }: { isOpen: boolean, onArchive: (params: any) => void }) => (
        isOpen ? <div data-testid="review-modal"><button onClick={() => onArchive({ conclusion: 'Conclusion', service: 'TestService', module: 'TestModule', targetFile: '' })}>Archive</button></div> : null
    )
}));

// Mock Wails runtime calls
const mockStartSession = vi.fn();
const mockStopSession = vi.fn();
const mockArchiveSession = vi.fn();
const mockAskAI = vi.fn();

window.go = {
    main: {
        App: {
            StartSession: mockStartSession,
            StopSession: mockStopSession,
            ArchiveSession: mockArchiveSession,
            AskAI: mockAskAI,
            PolishRootCause: vi.fn(),
        }
    }
} as any;

describe('TroubleshootingPanel', () => {
    beforeAll(() => {
        Element.prototype.scrollIntoView = vi.fn();
    });

    beforeEach(() => {
        mockStartSession.mockReset();
        mockStopSession.mockReset();
        mockArchiveSession.mockReset();
        mockAskAI.mockReset();
        mockAskAI.mockResolvedValue('');
        window.go = {
            main: { App: {
                StartSession: mockStartSession,
                StopSession: mockStopSession,
                ArchiveSession: mockArchiveSession,
                AskAI: mockAskAI,
                PolishRootCause: vi.fn(),
            } },
        } as any;
    });

    it('renders initial empty state correctly', () => {
        render(<TroubleshootingPanel />);
        expect(screen.getByTitle('发送')).toBeInTheDocument();
        expect(screen.getByPlaceholderText(/例如：/i)).toBeInTheDocument();
    });

    it('starts investigation when clicking start button', async () => {
        render(<TroubleshootingPanel />);

        const input = screen.getByPlaceholderText(/例如：/i);
        fireEvent.change(input, { target: { value: 'CPU high' } });

        const startBtn = screen.getByTitle('发送');
        fireEvent.click(startBtn);

        expect(mockStartSession).toHaveBeenCalledWith('CPU high');
        // Should show stop button
        expect(screen.getByText('整理排查结论')).toBeInTheDocument();
    });

    it('renders structured AI response correctly', async () => {
        // Mock AI response with structured data
        mockAskAI.mockResolvedValue(JSON.stringify({
            steps: ['Check CPU', 'Check Memory'],
            commands: [{ command: 'top', description: 'Show processes' }]
        }));

        render(<TroubleshootingPanel />);

        // Start
        fireEvent.change(screen.getByPlaceholderText(/例如：/i), { target: { value: 'Issue' } });
        fireEvent.click(screen.getByTitle('发送'));

        // Wait for async operations (using findBy which waits)
        expect(await screen.findByText('定位思路')).toBeInTheDocument();
        expect(screen.getByText('Check CPU')).toBeInTheDocument();
        expect(screen.getByText('Check Memory')).toBeInTheDocument();
        expect(screen.getByText('top')).toBeInTheDocument();
    });

    it('handles stop and archive flow', async () => {
        render(<TroubleshootingPanel />);

        // Start first
        fireEvent.change(screen.getByPlaceholderText(/例如：/i), { target: { value: 'Issue' } });
        fireEvent.click(screen.getByTitle('发送'));

        // Click stop
        fireEvent.click(screen.getByText('整理排查结论'));

        // Input root cause (simulating the stop UI flow)
        const rootCauseInput = screen.getByPlaceholderText(/简要补充根本原因/i);
        fireEvent.change(rootCauseInput, { target: { value: 'Bug in code' } });

        // Confirm stop
        fireEvent.click(screen.getByText('继续编辑结论'));

        // Should open review modal
        const archiveBtn = await screen.findByText('Archive'); // In mock modal
        fireEvent.click(archiveBtn);

        expect(mockArchiveSession).toHaveBeenCalled();
    });
});
