import React from 'react';
import { act, render, screen } from '@testing-library/react';
import TerminalComponent, { TerminalRef } from './Terminal';
import { vi, describe, it, expect, afterEach } from 'vitest';

const { TerminalMock, terminalInstances, fitMock, proposeDimensionsMock, refreshMock, scrollToBottomMock, scrollToLineMock } = vi.hoisted(() => {
    return {
        TerminalMock: vi.fn(),
        terminalInstances: [] as any[],
        fitMock: vi.fn(),
        proposeDimensionsMock: vi.fn(() => ({ cols: 80, rows: 24 })),
        refreshMock: vi.fn(),
        scrollToBottomMock: vi.fn(),
        scrollToLineMock: vi.fn()
    }
});

// Mock xterm
vi.mock('xterm', () => {
  return {
    Terminal: class {
        constructor() {
            TerminalMock();
            const terminal = {
                open: vi.fn(),
                write: vi.fn(),
                dispose: vi.fn(),
                onData: vi.fn(),
                onSelectionChange: vi.fn(() => ({ dispose: vi.fn() })),
                attachCustomKeyEventHandler: vi.fn(),
                getSelection: vi.fn(() => ''),
                clearSelection: vi.fn(),
                paste: vi.fn(),
                loadAddon: vi.fn(),
                onScroll: vi.fn(() => ({ dispose: vi.fn() })),
                focus: vi.fn(),
                refresh: refreshMock,
                scrollToBottom: scrollToBottomMock,
                scrollToLine: scrollToLineMock,
                cols: 80,
                rows: 24,
                options: {
                    scrollback: 5000,
                },
                buffer: {
                    active: {
                        viewportY: 0,
                        baseY: 0,
                        cursorY: 0,
                        length: 0,
                        getLine: vi.fn(() => ({ translateToString: vi.fn(() => '') }))
                    }
                },
                registerMarker: vi.fn(() => ({ dispose: vi.fn() })),
                registerDecoration: vi.fn(() => ({ dispose: vi.fn() })),
            };
            terminalInstances.push(terminal);
            return terminal;
        }
    },
  };
});

// Mock xterm-addon-fit
vi.mock('xterm-addon-fit', () => {
  return {
    FitAddon: class {
        fit = fitMock;
        proposeDimensions = proposeDimensionsMock;
    }
  }
})

vi.mock('xterm-addon-search', () => {
    return {
        SearchAddon: class {
            findNext = vi.fn();
            findPrevious = vi.fn();
        }
    };
});

describe('TerminalComponent', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
    TerminalMock.mockClear();
    terminalInstances.length = 0;
    fitMock.mockClear();
    proposeDimensionsMock.mockClear();
    proposeDimensionsMock.mockReturnValue({ cols: 80, rows: 24 });
    refreshMock.mockClear();
    scrollToBottomMock.mockClear();
    scrollToLineMock.mockClear();
  });

  it('renders terminal container', () => {
    render(<TerminalComponent id="test-term" />);
    const element = screen.getByTestId('terminal-container-test-term');
    expect(element).toBeInTheDocument();
  });

  it('initializes xterm on mount', () => {
    render(<TerminalComponent id="test-term" />);
    expect(TerminalMock).toHaveBeenCalled();
  });

  it('refits and refreshes the visible terminal over multiple frames', () => {
    vi.useFakeTimers();
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb: FrameRequestCallback) => {
        return window.setTimeout(() => cb(performance.now()), 0);
    });
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation((id: number) => {
        window.clearTimeout(id);
    });

    const rect = {
        width: 800,
        height: 480,
        top: 0,
        left: 0,
        bottom: 480,
        right: 800,
        x: 0,
        y: 0,
        toJSON: () => ({}),
    } as DOMRect;
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue(rect);
    vi.spyOn(HTMLElement.prototype, 'getClientRects').mockReturnValue([rect] as any);

    const ref = React.createRef<TerminalRef>();
    render(<TerminalComponent id="visible-term" ref={ref} />);

    fitMock.mockClear();
    refreshMock.mockClear();
    scrollToBottomMock.mockClear();

    act(() => {
        ref.current?.fit();
    });

    expect(fitMock).toHaveBeenCalledTimes(1);
    expect(refreshMock).toHaveBeenCalledWith(0, 23);
    expect(scrollToBottomMock).toHaveBeenCalled();

    act(() => {
        vi.advanceTimersByTime(200);
    });

    expect(fitMock.mock.calls.length).toBeGreaterThan(1);
    expect(refreshMock.mock.calls.length).toBeGreaterThan(1);
  });

  it('preserves the scrollback offset when the terminal is not at the bottom', () => {
    const rect = {
        width: 800,
        height: 480,
        top: 0,
        left: 0,
        bottom: 480,
        right: 800,
        x: 0,
        y: 0,
        toJSON: () => ({}),
    } as DOMRect;
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue(rect);
    vi.spyOn(HTMLElement.prototype, 'getClientRects').mockReturnValue([rect] as any);

    const ref = React.createRef<TerminalRef>();
    render(<TerminalComponent id="scrolled-term" ref={ref} />);

    const term = terminalInstances[0];
    term.buffer.active.baseY = 120;
    term.buffer.active.viewportY = 90;

    fitMock.mockClear();
    refreshMock.mockClear();
    scrollToBottomMock.mockClear();
    scrollToLineMock.mockClear();

    fitMock.mockImplementationOnce(() => {
        term.buffer.active.baseY = 140;
        term.buffer.active.viewportY = 100;
    });

    act(() => {
        ref.current?.fit();
    });

    expect(scrollToBottomMock).not.toHaveBeenCalled();
    expect(scrollToLineMock).toHaveBeenCalledWith(110);
    expect(refreshMock).toHaveBeenCalledWith(0, 23);
  });
});
