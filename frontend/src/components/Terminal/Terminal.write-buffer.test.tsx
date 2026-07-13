import React from 'react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { act, render } from '@testing-library/react';
import TerminalComponent, { TerminalRef } from './Terminal';
import { HighlightRule, TerminalConfig } from './highlightTypes';

let xtermWrite: ReturnType<typeof vi.fn>;
let registerDecoration: ReturnType<typeof vi.fn>;
let registerMarker: ReturnType<typeof vi.fn>;
let mockBufferLines: string[] = [];
let writeParsedCallback: (() => void) | null = null;

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

vi.mock('@xterm/xterm', () => {
    return {
        Terminal: class {
            constructor() {
                xtermWrite = vi.fn(() => writeParsedCallback?.());
                registerDecoration = vi.fn(() => ({ dispose: vi.fn() }));
                registerMarker = vi.fn((offset = 0) => ({ line: offset, dispose: vi.fn() }));
                return {
                    open: vi.fn(),
                    write: xtermWrite,
                    dispose: vi.fn(),
                    onData: vi.fn(() => ({ dispose: vi.fn() })),
                    onSelectionChange: vi.fn(() => ({ dispose: vi.fn() })),
                    attachCustomKeyEventHandler: vi.fn(),
                    getSelection: vi.fn(() => ''),
                    clearSelection: vi.fn(),
                    paste: vi.fn(),
                    loadAddon: vi.fn(),
                    onScroll: vi.fn(() => ({ dispose: vi.fn() })),
                    onWriteParsed: vi.fn((cb: () => void) => {
                        writeParsedCallback = cb;
                        return { dispose: vi.fn() };
                    }),
                    onResize: vi.fn(() => ({ dispose: vi.fn() })),
                    focus: vi.fn(),
                    refresh: vi.fn(),
                    scrollToBottom: vi.fn(),
                    scrollToLine: vi.fn(),
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
                            get length() { return mockBufferLines.length; },
                            getLine: vi.fn((i: number) => {
                                const text = mockBufferLines[i] || '';
                                return {
                                    isWrapped: false,
                                    length: 80,
                                    translateToString: vi.fn(() => text),
                                    getCell: vi.fn((col: number) => {
                                        const char = text[col] || '';
                                        return {
                                            getWidth: () => 1,
                                            getChars: () => char,
                                            getCode: () => char ? char.codePointAt(0)! : 0,
                                        };
                                    })
                                };
                            })
                        }
                    },
                    registerMarker,
                    registerDecoration,
                };
            }
        }
    };
});

vi.mock('@xterm/addon-fit', () => ({ FitAddon: class { fit = vi.fn(); proposeDimensions = vi.fn(() => ({ cols: 80, rows: 24 })); } }));
vi.mock('@xterm/addon-search', () => ({ SearchAddon: class { findNext = vi.fn(); findPrevious = vi.fn(); clearDecorations = vi.fn(); dispose = vi.fn(); onDidChangeResults = vi.fn(() => ({ dispose: vi.fn() })); } }));

describe('Terminal write buffering (Change 1)', () => {
    beforeEach(() => {
        mockBufferLines = [''];
        writeParsedCallback = null;
        rafCallbacks = [];
        vi.spyOn(window, 'requestAnimationFrame').mockImplementation(mockRAF);
    });

    afterEach(() => {
        vi.restoreAllMocks();
    });

    it('should buffer multiple writes and flush once per RAF', () => {
        const ref = React.createRef<TerminalRef>();
        render(<TerminalComponent id="buf-test" ref={ref} />);

        act(() => {
            ref.current?.write('hello');
            ref.current?.write(' ');
            ref.current?.write('world');
        });

        // xterm.write should NOT have been called yet (buffered)
        expect(xtermWrite).not.toHaveBeenCalled();

        // Flush the RAF
        act(() => {
            flushRAFs();
        });

        // xterm.write should be called once with merged data
        expect(xtermWrite).toHaveBeenCalledTimes(1);
        expect(xtermWrite).toHaveBeenCalledWith('hello world');
    });

    it('should flush only once when multiple writes are queued before RAF fires', () => {
        const ref = React.createRef<TerminalRef>();
        render(<TerminalComponent id="buf-test2" ref={ref} />);

        act(() => {
            ref.current?.write('a');
            ref.current?.write('b');
            ref.current?.write('c');
            ref.current?.write('d');
        });

        // Still no xterm.write
        expect(xtermWrite).not.toHaveBeenCalled();

        act(() => {
            flushRAFs();
        });

        expect(xtermWrite).toHaveBeenCalledTimes(1);
        expect(xtermWrite).toHaveBeenCalledWith('abcd');
    });

    it('should allow separate RAF cycles for sequential writes', () => {
        const ref = React.createRef<TerminalRef>();
        render(<TerminalComponent id="buf-test3" ref={ref} />);

        // First batch
        act(() => {
            ref.current?.write('batch1');
        });
        act(() => {
            flushRAFs();
        });
        expect(xtermWrite).toHaveBeenCalledTimes(1);
        expect(xtermWrite).toHaveBeenCalledWith('batch1');

        // Second batch
        act(() => {
            ref.current?.write('batch2');
        });
        act(() => {
            flushRAFs();
        });
        expect(xtermWrite).toHaveBeenCalledTimes(2);
        expect(xtermWrite).toHaveBeenLastCalledWith('batch2');
    });

    it('should handle empty write gracefully', () => {
        const ref = React.createRef<TerminalRef>();
        render(<TerminalComponent id="buf-test4" ref={ref} />);

        act(() => {
            ref.current?.write('');
        });
        act(() => {
            flushRAFs();
        });

        // Even empty string gets flushed
        expect(xtermWrite).toHaveBeenCalledTimes(1);
    });
});

describe('Terminal post-output highlight scan (Change 2)', () => {
    beforeEach(() => {
        // Buffer with matching content for highlight tests
        mockBufferLines = ['hello', 'error happened', 'world'];
        rafCallbacks = [];
        vi.spyOn(window, 'requestAnimationFrame').mockImplementation(mockRAF);
        vi.useFakeTimers();
    });

    afterEach(() => {
        vi.useRealTimers();
        vi.restoreAllMocks();
    });

    it('should NOT trigger highlight scan immediately on write', () => {
        const rules: HighlightRule[] = [{
            id: '1', name: 'err', pattern: '(?i)\\berror\\b',
            is_enabled: true, priority: 0,
            style: { background_color: '#5a1d1d', color: '#ffffff' }
        }];
        const cfg: TerminalConfig = { scrollback: 5000, search_enabled: true, highlight_enabled: true };

        const ref = React.createRef<TerminalRef>();
        render(<TerminalComponent id="scan-test1" ref={ref} terminalConfig={cfg} highlightRules={rules} />);

        // Clear any decoration calls from mount
        registerDecoration.mockClear();

        act(() => {
            ref.current?.write('error happened');
            flushRAFs(); // flush the write
        });

        // Immediately after write, no highlight scan should have run
        // (the 300ms post-output timer hasn't elapsed yet)
        expect(registerDecoration).not.toHaveBeenCalled();
    });

    it('should trigger highlight scan after parsed output', async () => {
        const rules: HighlightRule[] = [{
            id: '1', name: 'err', pattern: '(?i)\\berror\\b',
            is_enabled: true, priority: 0,
            style: { background_color: '#5a1d1d', color: '#ffffff' }
        }];
        const cfg: TerminalConfig = { scrollback: 5000, search_enabled: true, highlight_enabled: true };

        const ref = React.createRef<TerminalRef>();
        render(<TerminalComponent id="scan-test2" ref={ref} terminalConfig={cfg} highlightRules={rules} />);

        registerDecoration.mockClear();

        act(() => {
            ref.current?.write('error happened');
            flushRAFs(); // flush the write, sets lastOutputAt
        });

        // Advance past the 300ms post-output debounce + scheduleHighlightScan(0) timer
        await act(async () => {
            await vi.advanceTimersByTimeAsync(350);
        });

        // Now the highlight scan should have created decorations
        expect(registerDecoration).toHaveBeenCalled();
    });

    it('should update highlights while output is still flowing', async () => {
        const rules: HighlightRule[] = [{
            id: '1', name: 'err', pattern: '(?i)\\berror\\b',
            is_enabled: true, priority: 0,
            style: { background_color: '#5a1d1d', color: '#ffffff' }
        }];
        const cfg: TerminalConfig = { scrollback: 5000, search_enabled: true, highlight_enabled: true };

        const ref = React.createRef<TerminalRef>();
        render(<TerminalComponent id="scan-test3" ref={ref} terminalConfig={cfg} highlightRules={rules} />);

        // Let mount-time timers complete first
        await act(async () => {
            await vi.advanceTimersByTimeAsync(500);
        });
        registerDecoration.mockClear();
        mockBufferLines = ['hello', 'prefix error', 'world'];

        // Simulate continuous output: write at t=0, t=100, t=200
        act(() => {
            ref.current?.write('line1 error\n');
            flushRAFs();
        });

        await act(async () => {
            await vi.advanceTimersByTimeAsync(100);
            ref.current?.write('line2 error\n');
            flushRAFs();
        });

        await act(async () => {
            await vi.advanceTimersByTimeAsync(100);
            ref.current?.write('line3 error\n');
            flushRAFs();
        });

        // At t=200, the post-output scan timer keeps getting rescheduled
        // because each write resets the 300ms debounce. No scan has fired from write path.
        const callsDuringFlow = registerDecoration.mock.calls.length;
        expect(callsDuringFlow).toBeGreaterThan(0);

        // Now wait for output to stabilize — use runAllTimers to handle nested timers
        await act(async () => {
            await vi.runAllTimersAsync();
        });

        // The final pass reuses unchanged decorations instead of rebuilding them.
        expect(registerDecoration.mock.calls.length).toBe(callsDuringFlow);
    });

    it('should coalesce a rapid write batch into one parsed-output update', async () => {
        const rules: HighlightRule[] = [{
            id: '1', name: 'err', pattern: '(?i)\\berror\\b',
            is_enabled: true, priority: 0,
            style: { background_color: '#5a1d1d', color: '#ffffff' }
        }];
        const cfg: TerminalConfig = { scrollback: 5000, search_enabled: true, highlight_enabled: true };

        const ref = React.createRef<TerminalRef>();
        render(<TerminalComponent id="scan-test4" ref={ref} terminalConfig={cfg} highlightRules={rules} />);

        // Let mount-time timers complete
        await act(async () => {
            await vi.advanceTimersByTimeAsync(500);
        });
        registerDecoration.mockClear();
        mockBufferLines = ['prefix error', 'world'];

        // Rapid-fire 10 writes — each resets the 300ms debounce
        act(() => {
            for (let i = 0; i < 10; i++) {
                ref.current?.write(`error line ${i}\n`);
            }
            flushRAFs();
        });

        // Advance 200ms — debounce not yet elapsed, scan should NOT fire
        await act(async () => {
            await vi.advanceTimersByTimeAsync(31);
        });
        expect(registerDecoration).not.toHaveBeenCalled();

        // Advance past 300ms — now the scan fires (run all timers to handle nesting)
        await act(async () => {
            await vi.advanceTimersByTimeAsync(20);
        });

        // Scan should have fired — registerDecoration called at least once
        expect(registerDecoration.mock.calls.length).toBeGreaterThan(0);
    });
});
