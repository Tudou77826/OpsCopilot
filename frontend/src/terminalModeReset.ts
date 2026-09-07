/**
 * 终端模式复位序列。
 *
 * 背景（Issue #65 / #67）：会话重连复用同一个 xterm 实例。若上一条 SSH 会话
 * 在全屏程序（vim/less/top 等）运行期间断线，远端来不及发送退出序列
 * （?1049l / ?1l），xterm 实例上就会残留：
 *   - 备用屏（?1049h）：无回滚 → 没有滚动条，重连后新 shell 被画进残留屏，
 *     提示符停在屏中部且无法"滚到底"（#67）；
 *   - 应用光标键（DECCKM, ?1h）+ 无回滚 → xterm 把滚轮翻译成 ^[OA/^[OB 发给
 *     远端（#65 的控制符来源）；
 *   - 以及鼠标上报、括号粘贴等其它私有模式残留。
 *
 * 本序列在重连成功后、新会话数据到达前写入本地终端（不经过 PTY），
 * 把实例恢复到与新 shell 匹配的干净模式状态。顺序要求：先 ?1049l 切回普通屏，
 * 再做 DECSTR 软复位与各模式显式清除。
 */
export const TERMINAL_MODE_RESET_SEQUENCE =
    '\x1b[?1049l' // 退出备用屏，恢复普通屏与光标位置
    + '\x1b[!p' // DECSTR 软复位（DECCKM/IRM/DECOM/DECAWM/SGR 等）
    + '\x1b[?1004l' // 焦点事件上报
    + '\x1b[?1000l' // 鼠标单击上报
    + '\x1b[?1002l' // 鼠标拖动上报
    + '\x1b[?1003l' // 鼠标任意事件上报
    + '\x1b[?1005l' // UTF-8 鼠标编码
    + '\x1b[?1006l' // SGR 鼠标编码
    + '\x1b[?1007l' // 备用屏滚轮转方向键（alternate scroll）
    + '\x1b[?2004l'; // 括号粘贴

export interface TerminalModeResetTarget {
    write(data: string): void;
}

/**
 * 向终端写入模式复位序列。target 允许为空（重连时终端可能已被关闭）。
 */
export function writeTerminalModeReset(target: TerminalModeResetTarget | undefined | null): void {
    target?.write(TERMINAL_MODE_RESET_SEQUENCE);
}
