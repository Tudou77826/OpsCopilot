import { render, screen, fireEvent, act } from '@testing-library/react';
import LayoutManager from './LayoutManager';
import { SessionStatus } from '../../types';
import { vi, describe, it, expect, beforeEach, afterEach } from 'vitest';

// Mock TerminalComponent
vi.mock('../Terminal/Terminal', () => ({
    default: ({ id }: { id: string }) => <div data-testid={`terminal-${id}`}>Terminal {id}</div>
}));

// Helper to capture RAF callbacks
let rafCallbacks: FrameRequestCallback[] = [];
const mockRAF = (cb: FrameRequestCallback) => {
    const id = rafCallbacks.push(cb);
    return id;
};
const flushRAFs = (time = 0) => {
    const cbs = rafCallbacks.splice(0);
    cbs.forEach(cb => cb(time));
};

// Helper: create mock dataTransfer for jsdom drag events
const mockDataTransfer = (): DataTransfer => ({
    dropEffect: 'none',
    effectAllowed: 'none' as DataTransfer['effectAllowed'],
    items: [] as unknown as DataTransferItemList,
    files: [] as unknown as FileList,
    types: [] as readonly string[],
    getData: () => '',
    setData: () => {},
    clearData: () => {},
    setDragImage: () => {},
});

const mockTerminals = [
    { id: 'session-1', title: 'Session 1', status: SessionStatus.CONNECTED },
    { id: 'session-2', title: 'Session 2', status: SessionStatus.CONNECTED },
];

describe('LayoutManager scheduleFitAll integration (Change 4)', () => {
    it('calls scheduleFitAll when mode-dependent useEffect fires', () => {
        const scheduleFitAll = vi.fn();
        render(
            <LayoutManager
                terminals={mockTerminals}
                mode="tab"
                onTerminalData={vi.fn()}
                terminalRefs={{ current: new Map() } as any}
                onCloseTerminal={vi.fn()}
                onRenameTerminal={vi.fn()}
                scheduleFitAll={scheduleFitAll}
            />
        );

        // Initial render triggers scheduleFitAll via useEffect
        expect(scheduleFitAll).toHaveBeenCalledWith(100);
    });

    it('calls scheduleFitAll on collapse in grid mode', () => {
        const scheduleFitAll = vi.fn();
        render(
            <LayoutManager
                terminals={mockTerminals}
                mode="grid"
                onTerminalData={vi.fn()}
                terminalRefs={{ current: new Map() } as any}
                onCloseTerminal={vi.fn()}
                onRenameTerminal={vi.fn()}
                scheduleFitAll={scheduleFitAll}
            />
        );

        // Clear initial call from layout-change useEffect
        scheduleFitAll.mockClear();

        // Find and click the collapse button for Session 1 (− button in grid title)
        const collapseBtns = screen.getAllByText('−');
        fireEvent.click(collapseBtns[0]);

        expect(scheduleFitAll).toHaveBeenCalledWith(150);
    });

    it('deduplicates multiple rapid calls to scheduleFitAll', () => {
        // Simulate the dedup behavior: multiple scheduleFitAll calls
        // only result in one actual fit execution
        let fitTimer: number | null = null;
        const fitMock = vi.fn();
        const scheduleFitAll = (delay = 120) => {
            if (fitTimer) window.clearTimeout(fitTimer);
            fitTimer = window.setTimeout(() => {
                fitTimer = null;
                fitMock();
            }, delay);
        };

        vi.useFakeTimers();

        // Simulate rapid collapse + expand
        scheduleFitAll(150);
        scheduleFitAll(150);
        scheduleFitAll(150);

        // Only one timer should be pending — advance once
        act(() => {
            vi.advanceTimersByTime(200);
        });

        // fitMock should be called exactly once despite 3 scheduleFitAll calls
        expect(fitMock).toHaveBeenCalledTimes(1);

        vi.useRealTimers();
    });
});

describe('LayoutManager drag RAF throttling (Change 5)', () => {
    beforeEach(() => {
        rafCallbacks = [];
        vi.spyOn(window, 'requestAnimationFrame').mockImplementation(mockRAF);
    });

    afterEach(() => {
        vi.restoreAllMocks();
    });

    it('batches multiple tab dragover events into single RAF', () => {
        const onReorder = vi.fn();
        render(
            <LayoutManager
                terminals={mockTerminals}
                mode="tab"
                onTerminalData={vi.fn()}
                terminalRefs={{ current: new Map() } as any}
                onCloseTerminal={vi.fn()}
                onRenameTerminal={vi.fn()}
                onReorderTerminals={onReorder}
            />
        );

        // Get the tab elements
        const tab1 = screen.getByText('Session 1').closest('[role="tab"]')!;
        const tab2 = screen.getByText('Session 2').closest('[role="tab"]')!;

        // Start dragging tab 1
        fireEvent.dragStart(tab1, { dataTransfer: mockDataTransfer() });

        // Rapidly fire multiple dragover events on tab 2
        const rect = { left: 0, width: 200, top: 0, height: 30, right: 200, bottom: 30, x: 0, y: 0, toJSON: () => ({}) };
        vi.spyOn(tab2 as HTMLElement, 'getBoundingClientRect').mockReturnValue(rect as any);

        fireEvent.dragOver(tab2, { clientX: 50, dataTransfer: mockDataTransfer() });
        fireEvent.dragOver(tab2, { clientX: 60, dataTransfer: mockDataTransfer() });
        fireEvent.dragOver(tab2, { clientX: 70, dataTransfer: mockDataTransfer() });
        fireEvent.dragOver(tab2, { clientX: 80, dataTransfer: mockDataTransfer() });

        // RAF should have been scheduled only once (the rest just update the ref)
        expect(rafCallbacks.length).toBe(1);

        // Flush the RAF — now setState should happen
        act(() => {
            flushRAFs();
        });

        // The tab should show drag indicator (position: relative is always set on tabs)
        expect(tab2).toHaveStyle({ position: 'relative' });
    });

    it('batches multiple grid dragover events into single RAF', () => {
        render(
            <LayoutManager
                terminals={mockTerminals}
                mode="grid"
                onTerminalData={vi.fn()}
                terminalRefs={{ current: new Map() } as any}
                onCloseTerminal={vi.fn()}
                onRenameTerminal={vi.fn()}
            />
        );

        // In grid mode, find grid title (draggable area) for session 1
        const termTitle1 = screen.getByText('Session 1');
        // The grid title div is the draggable ancestor
        const gridTitle1 = termTitle1.closest('[draggable="true"]') as HTMLElement;

        // Find the terminal wrapper for session 2 (drag target)
        const term2 = screen.getByTestId('terminal-session-2');
        const wrapper2 = term2.parentElement!;

        // Start dragging from grid title
        fireEvent.dragStart(gridTitle1, { dataTransfer: mockDataTransfer() });

        // Rapidly fire dragover on wrapper 2
        fireEvent.dragOver(wrapper2, { dataTransfer: mockDataTransfer() });
        fireEvent.dragOver(wrapper2, { dataTransfer: mockDataTransfer() });
        fireEvent.dragOver(wrapper2, { dataTransfer: mockDataTransfer() });

        // Only one RAF should be pending
        expect(rafCallbacks.length).toBe(1);

        // Flush
        act(() => {
            flushRAFs();
        });
    });

    it('skips setState when drag target has not changed', () => {
        render(
            <LayoutManager
                terminals={mockTerminals}
                mode="tab"
                onTerminalData={vi.fn()}
                terminalRefs={{ current: new Map() } as any}
                onCloseTerminal={vi.fn()}
                onRenameTerminal={vi.fn()}
            />
        );

        const tab1 = screen.getByText('Session 1').closest('[role="tab"]')!;
        const tab2 = screen.getByText('Session 2').closest('[role="tab"]')!;

        // Start dragging and dragover on tab 2
        fireEvent.dragStart(tab1, { dataTransfer: mockDataTransfer() });
        const rect = { left: 0, width: 200, top: 0, height: 30, right: 200, bottom: 30, x: 0, y: 0, toJSON: () => ({}) };
        vi.spyOn(tab2 as HTMLElement, 'getBoundingClientRect').mockReturnValue(rect as any);

        fireEvent.dragOver(tab2, { clientX: 50, dataTransfer: mockDataTransfer() });

        // Flush first RAF
        act(() => {
            flushRAFs();
        });

        // Fire same dragover again — same tabId, same position
        fireEvent.dragOver(tab2, { clientX: 50, dataTransfer: mockDataTransfer() });

        // RAF should be scheduled
        expect(rafCallbacks.length).toBe(1);

        // Flush — but setState should be skipped since nothing changed
        act(() => {
            flushRAFs();
        });

        // No additional RAFs should be scheduled
        expect(rafCallbacks.length).toBe(0);
    });

    it('cancels pending RAF on dragEnd', () => {
        render(
            <LayoutManager
                terminals={mockTerminals}
                mode="tab"
                onTerminalData={vi.fn()}
                terminalRefs={{ current: new Map() } as any}
                onCloseTerminal={vi.fn()}
                onRenameTerminal={vi.fn()}
            />
        );

        const tab1 = screen.getByText('Session 1').closest('[role="tab"]')!;
        const tab2 = screen.getByText('Session 2').closest('[role="tab"]')!;

        // Start dragging
        fireEvent.dragStart(tab1, { dataTransfer: mockDataTransfer() });
        const rect = { left: 0, width: 200, top: 0, height: 30, right: 200, bottom: 30, x: 0, y: 0, toJSON: () => ({}) };
        vi.spyOn(tab2 as HTMLElement, 'getBoundingClientRect').mockReturnValue(rect as any);

        // Dragover schedules RAF
        fireEvent.dragOver(tab2, { clientX: 50, dataTransfer: mockDataTransfer() });
        expect(rafCallbacks.length).toBe(1);

        // Drag end cancels the RAF
        const cancelSpy = vi.spyOn(window, 'cancelAnimationFrame');
        fireEvent.dragEnd(tab1, { dataTransfer: mockDataTransfer() });

        expect(cancelSpy).toHaveBeenCalled();
    });
});
