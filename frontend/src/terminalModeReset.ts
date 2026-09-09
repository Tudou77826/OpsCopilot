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
 * 把实例恢复到与新 shell 匹配的干净模式状态。
 *
 * 光标安全（防止重连输出冲进滚动历史）：
 *   - xterm 的 ?1049l 处理器无条件执行 restoreCursor()——即使从未进入过备用屏。
 *     若保存的光标（savedY，绝对行号）已被滚动历史甩出视口，restoreCursor 的
 *     Math.max(savedY - ybase, 0) 钳位会把光标钉在"视口顶部"，重连输出从此处
 *     向下覆写历史（表现为新输出出现在屏幕最上方、与历史混排）。因此仅在
 *     buffer.active.type === 'alternate'（残留备用屏）时才发 ?1049l——此时恢复的
 *     是进入备用屏时保存的光标，即用户在历史底部的真实位置；
 *   - DECSTR（\x1b[!p）会把保存光标改写为 (0, ybase)（视口顶），若之后有任何
 *     restoreCursor（远端发 ESC 8、重复写复位序列等）同样会跳到视口顶。序列
 *     末尾的 DECSC（\x1b7）把保存光标重新锚定为当前光标，消除这一隐患。
 */
export function terminalModeResetSequence(fromAlternateScreen: boolean): string {
    return (fromAlternateScreen ? '\x1b[?1049l' : '') // 退出残留备用屏，恢复进入前的光标
        + '\x1b[!p' // DECSTR 软复位（DECCKM/IRM/DECOM/DECAWM/SGR/字符集/滚动边界）
        + '\x1b[?1004l' // 焦点事件上报
        + '\x1b[?1000l' // 鼠标单击上报
        + '\x1b[?1002l' // 鼠标拖动上报
        + '\x1b[?1003l' // 鼠标任意事件上报
        + '\x1b[?1005l' // UTF-8 鼠标编码
        + '\x1b[?1006l' // SGR 鼠标编码
        + '\x1b[?1007l' // 备用屏滚轮转方向键（alternate scroll）
        + '\x1b[?2004l' // 括号粘贴
        + '\x1b7'; // DECSC：保存光标重新锚定为当前光标
}

export interface TerminalModeResetTarget {
    write(data: string): void;
    /** xterm Terminal 实例提供 buffer.active.type，用于判断是否残留备用屏 */
    buffer?: { active: { type: string } };
}

/**
 * 向终端写入模式复位序列。target 允许为空（重连时终端可能已被关闭）。
 */
export function writeTerminalModeReset(target: TerminalModeResetTarget | undefined | null): void {
    if (!target) {
        return;
    }
    target.write(terminalModeResetSequence(target.buffer?.active?.type === 'alternate'));
}
