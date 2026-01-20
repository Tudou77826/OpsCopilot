import React from 'react';
import { describe, it, expect, vi } from 'vitest';
import { act, render } from '@testing-library/react';
import TerminalComponent, { TerminalRef } from './Terminal';
import { HighlightRule, TerminalConfig } from './highlightTypes';

let lastKeyHandler: ((e: any) => boolean) | null = null;
let termWrite: ((data: string) => void) | null = null;
let registerDecoration: any = null;
let registerMarker: any = null;
let selectionText = '';

vi.mock('xterm', () => {
    return {
        Terminal: class {
            constructor() {
                registerDecoration = vi.fn(() => ({ dispose: vi.fn() }));
                registerMarker = vi.fn(() => ({ dispose: vi.fn() }));
                return {
                    open: vi.fn(),
                    write: vi.fn((data: string) => termWrite?.(data)),
                    dispose: vi.fn(),
                    onData: vi.fn(),
                    attachCustomKeyEventHandler: vi.fn((h: any) => { lastKeyHandler = h; }),
                    getSelection: vi.fn(() => selectionText),
                    clearSelection: vi.fn(),
                    paste: vi.fn(),
                    loadAddon: vi.fn(),
                    onScroll: vi.fn(() => ({ dispose: vi.fn() })),
                    focus: vi.fn(),
                    cols: 80,
                    rows: 24,
                    buffer: {
                        active: {
                            viewportY: 0,
                            baseY: 0,
                            cursorY: 0,
                            length: 3,
                            getLine: vi.fn((i: number) => {
                                const lines = ['hello', 'error happened', 'world'];
                                return { translateToString: vi.fn(() => lines[i] || '') };
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

vi.mock('xterm-addon-fit', () => ({ FitAddon: class { fit = vi.fn(); } }));
vi.mock('xterm-addon-search', () => ({ SearchAddon: class { findNext = vi.fn(); findPrevious = vi.fn(); } }));

describe('Terminal search/highlight integration', () => {
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

    it('highlights all search hits in yellow when search is visible', async () => {
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
        const hasYellow = registerDecoration.mock.calls.some((c: any[]) => c[0] && c[0].backgroundColor === '#f6e05e');
        expect(hasYellow).toBe(true);

        const before = registerDecoration.mock.calls.length;
        await act(async () => {
            ref.current?.write('new output');
            await vi.runAllTimersAsync();
        });
        expect(registerDecoration.mock.calls.length).toBeGreaterThan(before);
        vi.useRealTimers();
    });
});
