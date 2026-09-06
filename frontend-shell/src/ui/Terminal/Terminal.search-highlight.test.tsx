import React from 'react';
import { afterEach, describe, it, expect, vi } from 'vitest';
import { act, render } from '@testing-library/react';
import TerminalComponent, { TerminalRef } from './Terminal';
import { HighlightRule, TerminalConfig } from './highlightTypes';

let lastKeyHandler: ((e: any) => boolean) | null = null;
let termWrite: ((data: string) => void) | null = null;
let registerDecoration: any = null;
let registerMarker: any = null;
let selectionText = '';
let searchFindNext: any = null;
let searchAddonOptions: any = null;

vi.mock('@xterm/xterm', () => {
    return {
        Terminal: class {
            constructor() {
                registerDecoration = vi.fn(() => ({ dispose: vi.fn() }));
                registerMarker = vi.fn((offset = 0) => ({ line: offset, dispose: vi.fn() }));
                return {
                    open: vi.fn(),
                    write: vi.fn((data: string) => termWrite?.(data)),
                    dispose: vi.fn(),
                    onData: vi.fn(),
                    onSelectionChange: vi.fn(() => ({ dispose: vi.fn() })),
                    attachCustomKeyEventHandler: vi.fn((h: any) => { lastKeyHandler = h; }),
                    getSelection: vi.fn(() => selectionText),
                    clearSelection: vi.fn(),
                    paste: vi.fn(),
                    loadAddon: vi.fn(),
                    onScroll: vi.fn(() => ({ dispose: vi.fn() })),
                    onWriteParsed: vi.fn(() => ({ dispose: vi.fn() })),
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
                            length: 3,
                            getLine: vi.fn((i: number) => {
                                const lines = ['hello', 'error happened', 'world'];
                                const text = lines[i] || '';
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
vi.mock('@xterm/addon-search', () => ({ SearchAddon: class {
    constructor(options: any) {
        searchAddonOptions = options;
        searchFindNext = vi.fn();
    }
    findNext = (...args: any[]) => searchFindNext(...args);
    findPrevious = vi.fn();
    clearDecorations = vi.fn();
    dispose = vi.fn();
    onDidChangeResults = vi.fn(() => ({ dispose: vi.fn() }));
} }));

describe('Terminal search/highlight integration', () => {
    afterEach(() => {
        selectionText = '';
        lastKeyHandler = null;
        termWrite = null;
        searchFindNext = null;
        searchAddonOptions = null;
        vi.useRealTimers();
        vi.restoreAllMocks();
    });

    it('opens search panel via Ctrl+F handler', async () => {
        const ref = React.createRef<TerminalRef>();
        render(<TerminalComponent id="t1" ref={ref} />);
        expect(lastKeyHandler).toBeTruthy();

        const res = lastKeyHandler!({ type: 'keydown', ctrlKey: true, code: 'KeyF', preventDefault: vi.fn() });
        expect(res).toBe(false);
    });

    it('creates decorations when highlight rules enabled and buffer contains match', async () => {
        vi.useFakeTimers();

        const rules: HighlightRule[] = [{
            id: '1',
            name: 'err',
            pattern: '(?i)\\berror\\b',
            is_enabled: true,
            priority: 0,
            style: { background_color: '#5a1d1d', color: '#ffffff' }
        }];
        const cfg: TerminalConfig = { scrollback: 5000, search_enabled: true, highlight_enabled: true };

        const ref = React.createRef<TerminalRef>();
        render(<TerminalComponent id="t2" ref={ref} terminalConfig={cfg} highlightRules={rules} />);

        await vi.runOnlyPendingTimersAsync();
        expect(registerDecoration).toHaveBeenCalled();
        vi.useRealTimers();
    });

    it('delegates all-match highlighting to SearchAddon when search is visible', async () => {
        vi.useFakeTimers();
        const ref = React.createRef<TerminalRef>();
        render(<TerminalComponent id="t3" ref={ref} />);

        selectionText = 'error';
        await act(async () => {
            lastKeyHandler!({ type: 'keydown', ctrlKey: true, code: 'KeyF', preventDefault: vi.fn() });
        });
        await act(async () => {
            await vi.runAllTimersAsync();
        });
        expect(searchFindNext).toHaveBeenCalled();
        expect(searchAddonOptions).toEqual({ highlightLimit: 10_000 });
        const options = searchFindNext.mock.calls.at(-1)?.[1];
        expect(options).toMatchObject({
            incremental: true,
            decorations: {
                matchBackground: '#665c00',
                activeMatchBackground: '#f59e0b'
            }
        });
        vi.useRealTimers();
    });

    it('does not recreate search highlights when output arrives immediately after closing search', async () => {
        vi.useFakeTimers();
        const ref = React.createRef<TerminalRef>();
        const { container } = render(<TerminalComponent id="t4" ref={ref} />);

        selectionText = 'error';
        await act(async () => {
            lastKeyHandler!({ type: 'keydown', ctrlKey: true, code: 'KeyF', preventDefault: vi.fn() });
        });
        await act(async () => {
            await vi.runAllTimersAsync();
        });

        registerDecoration.mockClear();

        await act(async () => {
            // Esc 必须发自本终端区域（如终端宿主内）才会关闭搜索框；
            // 直接派发到 window 的按键不属于任何终端，不应触发关闭。
            const host = container.querySelector('.terminal-host')!;
            host.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
            ref.current?.write('error after close');
            await vi.runAllTimersAsync();
        });

        const hasYellow = registerDecoration.mock.calls.some((c: any[]) => c[0] && c[0].backgroundColor === '#f6e05e');
        expect(hasYellow).toBe(false);
    });
});
