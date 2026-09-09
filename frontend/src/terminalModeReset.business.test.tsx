/**
 * 终端模式复位业务用例。
 *
 * 用户操作流程：SSH 会话断线 → 点击重连，复用同一个 xterm 实例。
 * 复位序列必须同时满足两组诉求：
 *
 * 1)（#65 / #67 防护）上一会话在 vim/less 等全屏程序运行中断线时，残留的
 *    备用屏（?1049h）与应用光标键（DECCKM, ?1h）必须被清除，否则滚动条消失、
 *    滚轮被翻译成 ^[OA/^[OB 发给远端；
 * 2)（重连输出落点）普通屏场景（屏幕上有历史输出、无备用屏）复位序列不得
 *    移动光标：xterm 的 ?1049l 会无条件 restoreCursor()，保存位置滚出视口后
 *    被 Math.max(savedY - ybase, 0) 钳到"视口顶部"，重连输出从此覆写历史——
 *    表现为"新输出出现在屏幕最上方、与历史混在一起，完全无法使用"。
 *
 * 本用例使用真实 xterm（不 mock），在 jsdom 中驱动 buffer/解析器状态。
 * jsdom 无布局，渲染相关断言不在此覆盖（已由真实浏览器实测）。
 */
import React from 'react';
import { describe, it, expect, beforeAll, beforeEach, afterEach } from 'vitest';
import { act, cleanup } from '@testing-library/react';
import { Terminal } from '@xterm/xterm';
import { terminalModeResetSequence, writeTerminalModeReset } from './terminalModeReset';

// 可控地等待 xterm 写缓冲解析完成（write callback）
const writeP = (term: Terminal, data: string) => new Promise<void>(r => term.write(data, r));

// 在缓冲区中查找包含指定前缀文本的行号（绝对行号，含滚动历史）
function findLine(term: Terminal, snippet: string): number {
    const buffer = term.buffer.active;
    for (let i = 0; i < buffer.length; i++) {
        if (buffer.getLine(i)!.translateToString(true).includes(snippet)) {
            return i;
        }
    }
    return -1;
}

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

describe('重连后终端模式复位（#65/#67 + 重连输出落点）', () => {
    let container: HTMLDivElement;

    beforeEach(() => {
        container = document.createElement('div');
        document.body.appendChild(container);
    });
    afterEach(() => {
        cleanup();
        container.remove();
    });

    it('序列契约：普通屏不发 ?1049l；备用屏以 ?1049l 开头；均以 DECSC 收尾且不残留开启序列', () => {
        const normal = terminalModeResetSequence(false);
        const alternate = terminalModeResetSequence(true);
        expect(normal).not.toContain('\x1b[?1049l');
        expect(alternate.indexOf('\x1b[?1049l')).toBe(0); // 必须最先切回普通屏
        expect(alternate.indexOf('\x1b[!p')).toBeGreaterThan(0);
        for (const seq of [normal, alternate]) {
            for (const mode of ['1000', '1002', '1003', '1005', '1006', '1007', '2004']) {
                expect(seq).toContain(`\x1b[?${mode}l`);
            }
            expect(seq.endsWith('\x1b7')).toBe(true); // 保存光标重新锚定收尾
            expect(seq).not.toMatch(/\x1b\[\?\d+h/); // 不允许残留任何"开启"类私有模式
        }
    });

    it('writeTerminalModeReset 按缓冲区类型选序列，无 buffer 信息时按普通屏处理；目标缺失安全跳过', () => {
        const written: string[] = [];
        writeTerminalModeReset({ write: d => written.push(d) });
        expect(written).toEqual([terminalModeResetSequence(false)]);
        written.length = 0;
        writeTerminalModeReset({ write: d => written.push(d), buffer: { active: { type: 'normal' } } });
        expect(written).toEqual([terminalModeResetSequence(false)]);
        written.length = 0;
        writeTerminalModeReset({ write: d => written.push(d), buffer: { active: { type: 'alternate' } } });
        expect(written).toEqual([terminalModeResetSequence(true)]);
        expect(() => writeTerminalModeReset(undefined)).not.toThrow();
        expect(() => writeTerminalModeReset(null)).not.toThrow();
    });

    it('普通屏满历史断线重连：复位不移动光标，新输出接续在历史底部（不覆写历史）', async () => {
        const term = new Terminal({ rows: 30, cols: 80, scrollback: 5000 });
        term.open(container);

        // —— 模拟上一会话：大量输出灌满屏幕与滚动历史，然后断线
        let out = '\r\n';
        for (let i = 1; i <= 120; i++) {
            out += `HIST-${String(i).padStart(3, '0')} ===== historical output line =====\r\n`;
        }
        out += 'root@repro:~# ';
        await act(async () => {
            await writeP(term, out);
        });
        await act(async () => {
            await writeP(term, '\r\n[断开] 连接已关闭\r\n');
        });

        const buffer = term.buffer.active;
        const cursorBefore = buffer.baseY + buffer.cursorY; // 绝对行号
        const histLast = findLine(term, 'HIST-120');
        expect(histLast).toBeGreaterThan(0);

        // —— 重连成功：写入复位序列（App.handleReconnect 的行为）
        await act(async () => {
            await writeP(term, terminalModeResetSequence(buffer.type === 'alternate'));
        });

        // 光标必须原地不动（修复前：被 ?1049l 的钳位 restoreCursor 挪到视口顶部）
        expect(term.buffer.active.baseY + term.buffer.active.cursorY).toBe(cursorBefore);

        // —— 新会话输出
        let next = '\r\n';
        for (let i = 1; i <= 15; i++) {
            next += `NEW-${String(i).padStart(3, '0')} >>>>> reconnect output line <<<<<\r\n`;
        }
        next += 'root@repro:~# ';
        await act(async () => {
            await writeP(term, next);
        });

        // NEW 输出必须整体落在 HIST-120 之后（修复前：落进历史中间覆写 HIST-092~）
        const newFirst = findLine(term, 'NEW-001');
        expect(newFirst).toBeGreaterThan(histLast);
        // 历史本身未被覆写
        expect(findLine(term, 'HIST-001')).toBeGreaterThan(0);
        expect(findLine(term, 'HIST-119')).toBeGreaterThan(histLast - 2);
        term.dispose();
    });

    it('全屏程序中断线后重连：残留的备用屏与应用光标键被复位，新输出落回历史底部', async () => {
        const term = new Terminal({ rows: 30, cols: 80, scrollback: 5000 });
        term.open(container);
        const received: string[] = [];
        term.onData(d => received.push(d));

        // —— 模拟上一会话：先有历史输出，用户进入 vim（备用屏 + DECCKM），随后断线
        let out = '\r\n';
        for (let i = 1; i <= 120; i++) {
            out += `HIST-${String(i).padStart(3, '0')} ===== historical output line =====\r\n`;
        }
        out += 'root@repro:~# ';
        await act(async () => {
            await writeP(term, out);
        });
        const histLast = findLine(term, 'HIST-120');
        await act(async () => {
            await writeP(term, '\x1b[?1049h\x1b[?1h');
        });
        expect(term.buffer.active.type).toBe('alternate');

        // DECCKM 开启时，方向键以 SS3 形式（ESC O A）发往远端 —— #65 的字节来源
        keydownArrowUp(term);
        expect(received).toContain('\x1bOA');

        // —— 重连成功：写入复位序列（App.handleReconnect 的行为）
        await act(async () => {
            await writeP(term, terminalModeResetSequence(term.buffer.active.type === 'alternate'));
        });

        // 备用屏退出，回到普通屏 —— 滚动条恢复、新 shell 不再画进残留屏（#67）
        expect(term.buffer.active.type).toBe('normal');
        // DECCKM 复位：方向键恢复 CSI 形式（ESC [ A），不再产生 SS3 —— #65 消失
        const idxOA = received.indexOf('\x1bOA');
        keydownArrowUp(term);
        const after = received.slice(idxOA + 1);
        expect(after).toContain('\x1b[A');
        expect(after).not.toContain('\x1bOA');

        // 新会话输出落在历史底部（进入 vim 前的光标位置之下），不落进历史中间
        await act(async () => {
            await writeP(term, '\r\nNEW-001 after vim crash\r\nroot@repro:~# ');
        });
        expect(findLine(term, 'NEW-001 after vim crash')).toBeGreaterThan(histLast);
        term.dispose();
    });

    it('复位后保存光标被重新锚定：远端发 ESC 8（restoreCursor）或重复复位都不会把光标拽走', async () => {
        const term = new Terminal({ rows: 30, cols: 80, scrollback: 5000 });
        term.open(container);

        let out = '\r\n';
        for (let i = 1; i <= 120; i++) {
            out += `HIST-${String(i).padStart(3, '0')} ===== line =====\r\n`;
        }
        out += 'root@repro:~# ';
        await act(async () => {
            await writeP(term, out);
        });
        const cursorBefore = term.buffer.active.baseY + term.buffer.active.cursorY;

        await act(async () => {
            await writeP(term, terminalModeResetSequence(false));
        });
        // 模拟远端 shell 初始化脚本发来 ESC 8（恢复光标）
        await act(async () => {
            await writeP(term, '\x1b8');
        });
        expect(term.buffer.active.baseY + term.buffer.active.cursorY).toBe(cursorBefore);

        // 复位序列被重复写入（重试/竞态路径）同样安全
        await act(async () => {
            await writeP(term, terminalModeResetSequence(false));
        });
        expect(term.buffer.active.baseY + term.buffer.active.cursorY).toBe(cursorBefore);
        term.dispose();
    });
});
