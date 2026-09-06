/**
 * 终端搜索框 Esc 作用域业务用例（Issue #63 防护）。
 *
 * 修复前：每个终端实例都把 Escape 捕获监听挂到 window 上，按一次 Esc 会把
 * 所有窗口的搜索框一起关掉。修复后：只有按键发自本终端区域（搜索输入框、
 * 终端本体）才关闭自己的搜索框，其他窗口不受影响。
 */
import React from 'react';
import { render, fireEvent, act } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import '@testing-library/jest-dom';
import TerminalComponent from './Terminal';

let lastKeyHandler: ((e: any) => boolean) | null = null;

vi.mock('@xterm/xterm', () => {
    return {
        Terminal: class {
            constructor() {
                return {
                    open: vi.fn(),
                    write: vi.fn(),
                    dispose: vi.fn(),
                    onData: vi.fn(),
                    onSelectionChange: vi.fn(() => ({ dispose: vi.fn() })),
                    attachCustomKeyEventHandler: vi.fn((h: any) => { lastKeyHandler = h; }),
                    getSelection: vi.fn(() => ''),
                    clearSelection: vi.fn(),
                    paste: vi.fn(),
                    loadAddon: vi.fn(),
                    onScroll: vi.fn(() => ({ dispose: vi.fn() })),
                    onWriteParsed: vi.fn(() => ({ dispose: vi.fn() })),
                    onResize: vi.fn(() => ({ dispose: vi.fn() })),
                    focus: vi.fn(),
                    refresh: vi.fn(),
                    scrollToBottom: vi.fn(),
                    cols: 80,
                    rows: 24,
                    options: { scrollback: 5000 },
                    buffer: {
                        onBufferChange: vi.fn(() => ({ dispose: vi.fn() })),
                        active: {
                            viewportY: 0,
                            baseY: 0,
                            cursorY: 0,
                            length: 1,
                            getLine: vi.fn(() => null),
                        },
                    },
                    registerMarker: vi.fn(() => ({ dispose: vi.fn() })),
                    registerDecoration: vi.fn(() => ({ dispose: vi.fn() })),
                };
            }
        },
    };
});
vi.mock('@xterm/addon-fit', () => ({ FitAddon: class { fit = vi.fn(); proposeDimensions = vi.fn(() => ({ cols: 80, rows: 24 })); } }));
vi.mock('@xterm/addon-search', () => ({ SearchAddon: class {
    findNext = vi.fn();
    findPrevious = vi.fn();
    clearDecorations = vi.fn();
    dispose = vi.fn();
    onDidChangeResults = vi.fn(() => ({ dispose: vi.fn() }));
} }));

afterEach(() => {
    lastKeyHandler = null;
    vi.restoreAllMocks();
});

function renderTerminal(id: string) {
    const utils = render(<TerminalComponent id={id} runtime={{ resize: vi.fn() }} />);
    const handler = lastKeyHandler!;
    expect(handler).toBeTruthy();
    return { ...utils, handler };
}

function openSearch(handler: (e: any) => boolean) {
    act(() => {
        handler({ type: 'keydown', ctrlKey: true, code: 'KeyF', preventDefault: vi.fn() });
    });
}

function searchInputOf(utils: ReturnType<typeof renderTerminal>) {
    return utils.container.querySelector('input');
}

describe('搜索框 Esc 作用域（#63）', () => {
    it('在 A 窗口搜索框按 Esc：只关闭 A，B 保持打开', () => {
        const a = renderTerminal('a');
        const b = renderTerminal('b');
        openSearch(a.handler);
        openSearch(b.handler);
        expect(searchInputOf(a)).toBeInTheDocument();
        expect(searchInputOf(b)).toBeInTheDocument();

        fireEvent.keyDown(searchInputOf(a)!, { key: 'Escape' });

        expect(searchInputOf(a)).not.toBeInTheDocument();
        expect(searchInputOf(b)).toBeInTheDocument();
    });

    it('焦点在 A 终端本体时按 Esc：同样只关闭 A 的搜索框', () => {
        const a = renderTerminal('a');
        const b = renderTerminal('b');
        openSearch(a.handler);
        openSearch(b.handler);

        fireEvent.keyDown(a.container.querySelector('[data-testid="terminal-container-a"]')!, { key: 'Escape' });

        expect(searchInputOf(a)).not.toBeInTheDocument();
        expect(searchInputOf(b)).toBeInTheDocument();
    });

    it('不属于任何终端的 Esc（如直接派发到 window）：不关闭任何搜索框', () => {
        const a = renderTerminal('a');
        const b = renderTerminal('b');
        openSearch(a.handler);
        openSearch(b.handler);

        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));

        expect(searchInputOf(a)).toBeInTheDocument();
        expect(searchInputOf(b)).toBeInTheDocument();
    });

    it('B 窗口的 Esc 不影响 A：关闭动作按窗口互相隔离', () => {
        const a = renderTerminal('a');
        const b = renderTerminal('b');
        openSearch(a.handler);
        openSearch(b.handler);

        fireEvent.keyDown(b.container.querySelector('[data-testid="terminal-container-b"]')!, { key: 'Escape' });

        expect(searchInputOf(b)).not.toBeInTheDocument();
        expect(searchInputOf(a)).toBeInTheDocument();
    });
});
