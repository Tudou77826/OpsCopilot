/**
 * Tab 条拖拽边缘自动滚动业务用例（Issue #66 防护）。
 *
 * 标签超出一屏时，flexlayout 0.9 的拖拽不提供边缘滚动，靠边的标签拖不到。
 * 修复后：拖拽光标进入 Tab 条（.flexlayout__tabset_tabbar_inner）左右边缘带
 * 时按距离加速横向滚动，drop/dragend 后停止。
 */
import React from 'react';
import { render, fireEvent, createEvent, act } from '@testing-library/react';
import { describe, it, expect, vi, beforeAll, afterEach } from 'vitest';
import '@testing-library/jest-dom';
import { FlexLayoutAdapter } from '@opscopilot/shell-terminal/ui';

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
                    attachCustomKeyEventHandler: vi.fn(),
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
                        active: { viewportY: 0, baseY: 0, cursorY: 0, length: 1, getLine: vi.fn(() => null) },
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

beforeAll(() => {
    const RO = class { observe() {} unobserve() {} disconnect() {} };
    (global as any).ResizeObserver = (global as any).ResizeObserver ?? RO;
});

afterEach(() => {
    vi.restoreAllMocks();
});

function renderAdapter() {
    return render(
        <FlexLayoutAdapter
            terminals={[
                { id: 't1', title: 'conn-1', status: 'connected' },
                { id: 't2', title: 'conn-2', status: 'connected' },
            ]}
            onTerminalData={vi.fn()}
            terminalRefs={{ current: new Map() }}
            onCloseTerminal={vi.fn()}
            onRenameTerminal={vi.fn()}
            terminalRuntime={{} as any}
        />
    );
}

function tabStrip(): HTMLElement {
    const strip = document.querySelector('.flexlayout__tabset_tabbar_inner') as HTMLElement | null;
    expect(strip).toBeTruthy();
    return strip!;
}

// jsdom 无布局：注入溢出指标与条带矩形。
function mockStripMetrics(strip: HTMLElement) {
    Object.defineProperty(strip, 'scrollWidth', { configurable: true, value: 900 });
    Object.defineProperty(strip, 'clientWidth', { configurable: true, value: 300 });
    vi.spyOn(strip, 'getBoundingClientRect').mockReturnValue({
        left: 0, right: 300, top: 0, bottom: 30, width: 300, height: 30, x: 0, y: 0, toJSON: () => ({}),
    } as DOMRect);
}

// jsdom 的 DragEvent 不支持 clientX 初始化（真实浏览器无此问题），
// 用 createEvent + defineProperty 注入光标横坐标。
function dragOverAt(el: HTMLElement, clientX: number) {
    const ev = createEvent.dragOver(el, {});
    Object.defineProperty(ev, 'clientX', { value: clientX });
    fireEvent(el, ev);
}

// 用可控 rAF 手动驱动帧。
function frameDriver() {
    const pending = new Map<number, FrameRequestCallback>();
    let seed = 0;
    const rafSpy = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(((cb: FrameRequestCallback) => {
        seed += 1;
        pending.set(seed, cb);
        return seed;
    }) as typeof window.requestAnimationFrame);
    const cafSpy = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(((id: number) => {
        pending.delete(id);
    }) as typeof window.cancelAnimationFrame);
    const runFrames = async (n: number) => {
        await act(async () => {
            for (let i = 0; i < n; i++) {
                const cbs = [...pending.values()];
                pending.clear();
                cbs.forEach((cb) => cb(performance.now()));
            }
            await Promise.resolve();
        });
    };
    return {
        runFrames,
        restore: () => {
            rafSpy.mockRestore();
            cafSpy.mockRestore();
        },
    };
}

describe('Tab 条拖拽边缘自动滚动（#66）', () => {
    it('拖拽光标停在 Tab 条右侧边缘带时向右滚动，dragend 后停止', async () => {
        const driver = frameDriver();
        try {
            const { container } = renderAdapter();
            const strip = tabStrip();
            mockStripMetrics(strip);

            dragOverAt(strip, 280); // 距右边缘 20px < 60px 边缘带

            await driver.runFrames(2);
            expect(strip.scrollLeft).toBeGreaterThan(0);
            const scrolled = strip.scrollLeft;

            fireEvent.dragEnd(container.firstElementChild as HTMLElement);
            await driver.runFrames(2);
            expect(strip.scrollLeft).toBe(scrolled);
        } finally {
            driver.restore();
        }
    });

    it('拖拽光标停在 Tab 条左侧边缘带时向左滚动', async () => {
        const driver = frameDriver();
        try {
            renderAdapter();
            const strip = tabStrip();
            mockStripMetrics(strip);
            strip.scrollLeft = 100;

            dragOverAt(strip, 20); // 距左边缘 20px

            await driver.runFrames(2);
            expect(strip.scrollLeft).toBeLessThan(100);
        } finally {
            driver.restore();
        }
    });

    it('光标在 Tab 条中部时不滚动', async () => {
        const driver = frameDriver();
        try {
            renderAdapter();
            const strip = tabStrip();
            mockStripMetrics(strip);

            dragOverAt(strip, 150);

            await driver.runFrames(2);
            expect(strip.scrollLeft).toBe(0);
        } finally {
            driver.restore();
        }
    });

    it('拖拽光标不在 Tab 条上时不启动滚动', async () => {
        const driver = frameDriver();
        try {
            const { container } = renderAdapter();
            const strip = tabStrip();
            mockStripMetrics(strip);

            // 派发到布局容器（Tab 条外）
            dragOverAt(container.firstElementChild!, 280);

            await driver.runFrames(2);
            expect(strip.scrollLeft).toBe(0);
        } finally {
            driver.restore();
        }
    });
});
