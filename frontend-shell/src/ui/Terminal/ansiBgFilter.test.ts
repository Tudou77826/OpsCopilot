import { describe, it, expect } from 'vitest';
import { AnsiBackgroundFilter } from './ansiBgFilter';

describe('AnsiBackgroundFilter', () => {
    it('透传普通文本（无转义）', () => {
        const f = new AnsiBackgroundFilter();
        expect(f.feed('hello world\r\n')).toBe('hello world\r\n');
    });

    it('剔除标准黑色背景 SGR (40m)', () => {
        const f = new AnsiBackgroundFilter();
        // \x1b[40m 应被整条剔除
        expect(f.feed('\x1b[40mtext')).toBe('text');
    });

    it('保留前景色，剔除背景色（1;31;40m → 1;31m）', () => {
        const f = new AnsiBackgroundFilter();
        expect(f.feed('\x1b[1;31;40mhi')).toBe('\x1b[1;31mhi');
    });

    it('保留 49m（默认背景，用于清除远端背景色）', () => {
        const f = new AnsiBackgroundFilter();
        expect(f.feed('\x1b[49m')).toBe('\x1b[49m');
    });

    it('剔除 256 色背景 (48;5;0m)', () => {
        const f = new AnsiBackgroundFilter();
        expect(f.feed('\x1b[48;5;0m')).toBe('');
    });

    it('剔除真彩色背景，保留前景 (38;2;1;2;3;48;2;0;0;0m → 38;2;1;2;3m)', () => {
        const f = new AnsiBackgroundFilter();
        expect(f.feed('\x1b[38;2;1;2;3;48;2;0;0;0m')).toBe('\x1b[38;2;1;2;3m');
    });

    it('剔除 bright 背景 (100m)', () => {
        const f = new AnsiBackgroundFilter();
        expect(f.feed('\x1b[100m')).toBe('');
    });

    it('保留全局 reset（\\x1b[m 与 \\x1b[0m）', () => {
        const f = new AnsiBackgroundFilter();
        expect(f.feed('\x1b[m')).toBe('\x1b[m');
        expect(f.feed('\x1b[0m')).toBe('\x1b[0m');
    });

    it('不误伤非背景 SGR（前景、粗体、清屏、光标移动）', () => {
        const f = new AnsiBackgroundFilter();
        expect(f.feed('\x1b[31m')).toBe('\x1b[31m');   // 红色前景
        expect(f.feed('\x1b[1m')).toBe('\x1b[1m');     // 粗体
        expect(f.feed('\x1b[2J')).toBe('\x1b[2J');     // 清屏（非 SGR）
        expect(f.feed('\x1b[H')).toBe('\x1b[H');       // 光标归位
        expect(f.feed('\x1b[?25h')).toBe('\x1b[?25h'); // 显示光标（私有序列）
    });

    it('OSC 序列原样透传', () => {
        const f = new AnsiBackgroundFilter();
        expect(f.feed('\x1b]0;title\x07')).toBe('\x1b]0;title\x07');
        expect(f.feed('\x1b]0;title\x1b\\')).toBe('\x1b]0;title\x1b\\');
    });

    it('跨 chunk 分片的转义序列正确处理', () => {
        // \x1b[1;40m 被拆成三次 feed：1(粗体)保留，40(黑背景)剔除
        const f = new AnsiBackgroundFilter();
        expect(f.feed('\x1b[')).toBe('');           // 暂存
        expect(f.feed('1;')).toBe('');              // 暂存
        expect(f.feed('40m')).toBe('\x1b[1m');      // 40 剔除，1 保留
        expect(f.feed('done')).toBe('done');
    });

    it('分片但包含前景+背景，保留前景', () => {
        const f = new AnsiBackgroundFilter();
        expect(f.feed('\x1b[31;4')).toBe('');
        expect(f.feed('0mtext')).toBe('\x1b[31mtext'); // 40 剔除，31 保留
    });

    it('末尾不完整转义暂存，下次拼接', () => {
        const f = new AnsiBackgroundFilter();
        const r1 = f.feed('before\x1b[31');
        expect(r1).toBe('before');
        const r2 = f.feed('mafter');
        expect(r2).toBe('\x1b[31mafter');
    });

    it('reset() 清空内部状态', () => {
        const f = new AnsiBackgroundFilter();
        f.feed('\x1b['); // 暂存未完成转义
        f.reset();
        expect(f.feed('40m')).toBe('40m'); // 不再被当作转义（前缀已清）
    });

    it('混合场景：文本 + 多种 SGR + OSC 一起处理', () => {
        const f = new AnsiBackgroundFilter();
        const input = 'start \x1b[1;31;40mred-bold-on-dark\x1b[0m \x1b]0;tab\x07 end';
        const expected = 'start \x1b[1;31mred-bold-on-dark\x1b[0m \x1b]0;tab\x07 end';
        expect(f.feed(input)).toBe(expected);
    });

    it('单字节 CSI (0x9b) 也正确剔除背景（重写时统一输出标准 ESC [ 形式）', () => {
        const f = new AnsiBackgroundFilter();
        // 0x9b = 单字节 CSI，等价于 ESC [；剔除背景后用规范的 ESC [ 重建
        expect(f.feed('\x9b40mtext')).toBe('text');
        expect(f.feed('\x9b31mred')).toBe('\x1b[31mred');
    });
});
