/**
 * 会话首段输出冲刷的业务用例（连接首帧丢失修复的前端侧）。
 *
 * 用户操作流程：新建/复制会话或重连的瞬间，远端立刻吐出首段输出
 * （motd/banner/设备菜单）。后端把它暂存在输出闸门里，前端必须等终端组件
 * 真正挂载（ref 就绪）后再触发冲刷——太早触发会把输出冲刷给还不存在的终端，
 * 太晚（或从不触发）会让首段输出永远压在闸门里。
 */
import { describe, it, expect, vi, afterEach } from 'vitest';
import { flushTerminalInitialOutput } from './terminalOutputFlush';

afterEach(() => {
    vi.useRealTimers();
});

describe('会话首段输出冲刷', () => {
    it('终端已挂载（ref 立即可用）时在同一 tick 内立即触发冲刷', () => {
        vi.useFakeTimers();
        const ready = vi.fn();
        flushTerminalInitialOutput('s1', () => true, ready);
        expect(ready).toHaveBeenCalledTimes(1);
        expect(ready).toHaveBeenCalledWith('s1');
    });

    it('终端尚未挂载时轮询等待，ref 就绪后下一 tick 触发冲刷', () => {
        vi.useFakeTimers();
        let refReady = false;
        const ready = vi.fn();
        flushTerminalInitialOutput('s1', () => refReady, ready, { tickMs: 20, timeoutMs: 5000 });

        // 未就绪期间绝不提前触发
        vi.advanceTimersByTime(20);
        vi.advanceTimersByTime(20);
        expect(ready).not.toHaveBeenCalled();

        // 终端挂载完成 → 下一 tick 触发
        refReady = true;
        vi.advanceTimersByTime(20);
        expect(ready).toHaveBeenCalledTimes(1);
        expect(ready).toHaveBeenCalledWith('s1');
    });

    it('终端一直未挂载时，超时兜底仍触发冲刷（防首段输出永久压在闸门里）', () => {
        vi.useFakeTimers();
        const ready = vi.fn();
        flushTerminalInitialOutput('s1', () => false, ready, { tickMs: 20, timeoutMs: 100 });
        vi.advanceTimersByTime(500);
        expect(ready).toHaveBeenCalledTimes(1);
        expect(ready).toHaveBeenCalledWith('s1');
    });

    it('多次触发冲刷是安全的（后端 TerminalOutputReady 幂等）', () => {
        vi.useFakeTimers();
        let refReady = false;
        const ready = vi.fn();
        flushTerminalInitialOutput('s1', () => refReady, ready, { tickMs: 10, timeoutMs: 100 });
        refReady = true;
        vi.advanceTimersByTime(10);
        vi.advanceTimersByTime(10);
        expect(ready).toHaveBeenCalledTimes(1); // 触发一次后不再轮询重复调用
    });
});