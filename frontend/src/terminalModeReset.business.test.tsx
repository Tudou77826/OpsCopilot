/**
 * 终端模式复位业务用例（Issue #65 / #67 防护）。
 *
 * 用户操作流程：SSH 会话在 vim/less 等全屏程序运行中断线 → 重连。
 * 重连复用同一个 xterm 实例，上一会话残留的备用屏（?1049h）与应用光标键
 * （DECCKM, ?1h）必须被重连时写入的复位序列清除，否则：
 *   - 滚动条消失、新会话被画进残留屏（#67：提示符卡中部、无法滚到底）；
 *   - 滚轮被 xterm 翻译成 ^[OA/^[OB 发给远端（#65）。
 *
 * 本用例使用真实 xterm（不 mock），在 jsdom 中驱动 buffer/解析器状态。
 * jsdom 无布局，渲染相关断言不在此覆盖（已由真实浏览器实测）。
 */
import React from 'react';
import { describe, it, expect, beforeAll, beforeEach, afterEach } from 'vitest';
import { act, cleanup } from '@testing-library/react';
import { Terminal } from '@xterm/xterm';
import { TERMINAL_MODE_RESET_SEQUENCE, writeTerminalModeReset } from './terminalModeReset';

// 可控地等待 xterm 写缓冲解析完成（write callback）
const writeP = (term: Terminal, data: string) => new Promise<void>(r => term.write(data, r));

beforeAll(() => {
    // jsdom 未实现 matchMedia，xterm open() 需要
    if (!window.matchMedia) {
        window.matchMedia = ((q: string) => ({
            matches: false, media: q, onchange: null,
            addListener: () => undefined, removeListener: () => undefined,
            addEventListener: () => undefined, removeEventListener: () => undefined,
            dispatchEvent: () => false,
        })) as unknown as typeof window.matchMedia;
    }
});

function keydownArrowUp(term: Terminal) {
    const ev = new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true, cancelable: true });
    Object.defineProperty(ev, 'keyCode', { value: 38 });
    Object.defineProperty(ev, 'which', { value: 38 });
    act(() => {
        term.textarea!.dispatchEvent(ev);
    });
}

describe('重连后终端模式复位（#65/#67）', () => {
    let container: HTMLDivElement;

    beforeEach(() => {
        container = document.createElement('div');
        document.body.appendChild(container);
    });
    afterEach(() => {
        cleanup();
        container.remove();
    });

    it('序列契约：先退出备用屏，再软复位，最后显式清除鼠标/括号粘贴等模式', () => {
        const i1049l = TERMINAL_MODE_RESET_SEQUENCE.indexOf('\x1b[?1049l');
        const decstr = TERMINAL_MODE_RESET_SEQUENCE.indexOf('\x1b[!p');
        expect(i1049l).toBe(0); // 必须最先切回普通屏
        expect(decstr).toBeGreaterThan(i1049l);
        for (const mode of ['1000', '1002', '1003', '1005', '1006', '1007', '2004']) {
            expect(TERMINAL_MODE_RESET_SEQUENCE).toContain(`\x1b[?${mode}l`);
        }
        // 不允许残留任何"开启"类私有模式序列（h 结尾）
        expect(TERMINAL_MODE_RESET_SEQUENCE).not.toMatch(/\x1b\[\?\d+h/);
    });

    it('writeTerminalModeReset 把序列写入目标，目标缺失时安全跳过', () => {
        const written: string[] = [];
        writeTerminalModeReset({ write: d => written.push(d) });
        expect(written).toEqual([TERMINAL_MODE_RESET_SEQUENCE]);
        expect(() => writeTerminalModeReset(undefined)).not.toThrow();
        expect(() => writeTerminalModeReset(null)).not.toThrow();
    });

    it('全屏程序中断线后重连：残留的备用屏与应用光标键被复位序列清除', async () => {
        const term = new Terminal({ scrollback: 5000 });
        term.open(container);
        const received: string[] = [];
        term.onData(d => received.push(d));

        // —— 模拟上一会话：vim 运行中（进入备用屏 + DECCKM），随后断线，退出序列丢失
        await act(async () => {
            await writeP(term, '\x1b[?1049h\x1b[?1h');
        });
        expect(term.buffer.active.type).toBe('alternate');

        // DECCKM 开启时，方向键以 SS3 形式（ESC O A）发往远端 —— #65 的字节来源
        keydownArrowUp(term);
        expect(received).toContain('\x1bOA');

        // —— 重连成功：写入复位序列（App.handleReconnect 的行为）
        await act(async () => {
            await writeP(term, TERMINAL_MODE_RESET_SEQUENCE);
        });

        // 备用屏退出，回到普通屏 —— 滚动条恢复、新 shell 不再画进残留屏（#67）
        expect(term.buffer.active.type).toBe('normal');
        // DECCKM 复位：方向键恢复 CSI 形式（ESC [ A），不再产生 SS3 —— #65 消失
        const idxOA = received.indexOf('\x1bOA');
        keydownArrowUp(term);
        const after = received.slice(idxOA + 1);
        expect(after).toContain('\x1b[A');
        expect(after).not.toContain('\x1bOA');
        term.dispose();
    });
});
