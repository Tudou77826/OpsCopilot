import React from 'react';
import { act, fireEvent, render, screen } from '@testing-library/react';
import TerminalComponent, { TerminalRef } from './Terminal';
import { vi, describe, it, expect, afterEach } from 'vitest';

const { TerminalMock, terminalInstances, scrollCallbacks, fitMock, proposeDimensionsMock, refreshMock, scrollToBottomMock, scrollToLineMock } = vi.hoisted(() => {
    return {
        TerminalMock: vi.fn(),
        terminalInstances: [] as any[],
        scrollCallbacks: [] as Array<() => void>,
        fitMock: vi.fn(),
        proposeDimensionsMock: vi.fn(() => ({ cols: 80, rows: 24 })),
        refreshMock: vi.fn(),
        scrollToBottomMock: vi.fn(),
        scrollToLineMock: vi.fn()
    }
});

// Mock xterm
vi.mock('@xterm/xterm', () => {
  return {
    Terminal: class {
        constructor() {
            TerminalMock();
            const terminal = {
                open: vi.fn((container: HTMLElement) => {
                    const xtermEl = document.createElement('div');
                    xtermEl.className = 'xterm';
                    const viewportEl = document.createElement('div');
                    viewportEl.className = 'xterm-viewport';
                    xtermEl.appendChild(viewportEl);
                    container.appendChild(xtermEl);
                }),
                write: vi.fn(),
                dispose: vi.fn(),
                onData: vi.fn(),
                onSelectionChange: vi.fn(() => ({ dispose: vi.fn() })),
                attachCustomKeyEventHandler: vi.fn(),
                getSelection: vi.fn(() => ''),
                clearSelection: vi.fn(),
                paste: vi.fn(),
                loadAddon: vi.fn(),
                onScroll: vi.fn((cb: () => void) => {
                    scrollCallbacks.push(cb);
                    return { dispose: vi.fn() };
                }),
                onWriteParsed: vi.fn(() => ({ dispose: vi.fn() })),
                onResize: vi.fn(() => ({ dispose: vi.fn() })),
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
                    onBufferChange: vi.fn(() => ({ dispose: vi.fn() })),
                    active: {
                        viewportY: 0,
                        baseY: 0,
                        cursorY: 0,
                        length: 0,
                        getLine: vi.fn(() => ({ translateToString: vi.fn(() => '') }))
                    }
                },
                registerMarker: vi.fn((offset = 0) => ({ line: offset, dispose: vi.fn() })),
                registerDecoration: vi.fn(() => ({ dispose: vi.fn() })),
            };
            terminalInstances.push(terminal);
            return terminal;
        }
    },
  };
});

// Mock xterm-addon-fit
vi.mock('@xterm/addon-fit', () => {
  return {
    FitAddon: class {
        fit = fitMock;
        proposeDimensions = proposeDimensionsMock;
    }
  }
})

vi.mock('@xterm/addon-search', () => {
    return {
        SearchAddon: class {
            findNext = vi.fn();
            findPrevious = vi.fn();
            clearDecorations = vi.fn();
            dispose = vi.fn();
            onDidChangeResults = vi.fn(() => ({ dispose: vi.fn() }));
        }
    };
});

describe('TerminalComponent', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
    TerminalMock.mockClear();
    terminalInstances.length = 0;
    scrollCallbacks.length = 0;
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

  it('changes only the terminal font size on Ctrl + wheel', () => {
    const onFontSizeChange = vi.fn();
    render(
      <TerminalComponent
        id="test-term"
        terminalConfig={{ scrollback: 5000, search_enabled: true, highlight_enabled: true, font_size: 14 }}
        onFontSizeChange={onFontSizeChange}
      />
    );

    fireEvent.wheel(screen.getByTestId('terminal-container-test-term'), { ctrlKey: true, deltaY: -100 });
    expect(onFontSizeChange).toHaveBeenCalledWith(15);
    expect(screen.getByText('字号 15px')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '恢复默认字号 14px' }));
    expect(onFontSizeChange).toHaveBeenLastCalledWith(14);
    expect(screen.queryByText('字号 15px')).not.toBeInTheDocument();
  });

  it('restores the default terminal font size with Ctrl + 0', () => {
    const onFontSizeChange = vi.fn();
    render(
      <TerminalComponent
        id="test-term"
        terminalConfig={{ scrollback: 5000, search_enabled: true, highlight_enabled: true, font_size: 18 }}
        onFontSizeChange={onFontSizeChange}
      />
    );

    const keyHandler = terminalInstances[0].attachCustomKeyEventHandler.mock.calls[0][0];
    const preventDefault = vi.fn();
    let handled: boolean | undefined;
    act(() => {
      handled = keyHandler({
        type: 'keydown',
        ctrlKey: true,
        altKey: false,
        metaKey: false,
        code: 'Digit0',
        preventDefault,
      });
    });

    expect(handled).toBe(false);
    expect(preventDefault).toHaveBeenCalled();
    expect(onFontSizeChange).toHaveBeenCalledWith(14);
    expect(screen.getByText('默认')).toBeInTheDocument();
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

  it('preserves the scrollback offset when the terminal is not at the bottom', async () => {
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

    await act(async () => {
        await new Promise(resolve => setTimeout(resolve, 0));
    });

    act(() => {
        scrollCallbacks.forEach(cb => cb());
    });

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

  it('keeps sticky-bottom intent when hidden output leaves viewport stale', () => {
    let rect = {
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
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(() => rect);
    vi.spyOn(HTMLElement.prototype, 'getClientRects').mockImplementation(() => (
        rect.width > 0 && rect.height > 0 ? [rect] as any : [] as any
    ));

    const ref = React.createRef<TerminalRef>();
    render(<TerminalComponent id="hidden-output-term" ref={ref} />);
    const viewport = screen.getByTestId('terminal-container-hidden-output-term')
        .querySelector<HTMLElement>('.xterm-viewport');
    expect(viewport).not.toBeNull();
    Object.defineProperty(viewport, 'scrollHeight', { configurable: true, value: 9000 });
    Object.defineProperty(viewport, 'clientHeight', { configurable: true, value: 600 });
    viewport!.scrollTop = 1200;

    const term = terminalInstances[0];
    term.buffer.active.baseY = 400;
    term.buffer.active.viewportY = 400;

    rect = {
        ...rect,
        width: 0,
        height: 0,
        bottom: 0,
        right: 0,
    } as DOMRect;
    term.buffer.active.baseY = 520;
    term.buffer.active.viewportY = 320;

    act(() => {
        scrollCallbacks.forEach(cb => cb());
    });

    rect = {
        ...rect,
        width: 800,
        height: 480,
        bottom: 480,
        right: 800,
    } as DOMRect;

    fitMock.mockClear();
    refreshMock.mockClear();
    scrollToBottomMock.mockClear();
    scrollToLineMock.mockClear();

    act(() => {
        ref.current?.fit();
    });

    expect(scrollToBottomMock).toHaveBeenCalled();
    expect(scrollToLineMock).not.toHaveBeenCalled();
    expect(refreshMock).toHaveBeenCalledWith(0, 23);
    expect(viewport!.scrollTop).toBe(8400);
  });
});
