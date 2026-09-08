/**
 * 会话首段输出冲刷（连接首帧丢失问题的前端侧）。
 *
 * 背景：连接建立瞬间远端立刻吐出首帧输出（motd/banner/设备菜单），而后端事件要在
 * 前端挂上 `terminal-data:<id>` 监听后才会真正送达。后端已实现终端输出闸门
 * （terminalOutputGate）：监听就绪前的输出先暂存，等前端调用 App.TerminalOutputReady
 * 才冲刷。
 *
 * 本模块负责在正确的时机触发冲刷：终端组件真正挂载（ref 就绪）之后——太早调用会
 * 把输出冲刷给一个还不存在的终端，太晚调用会让首帧一直压在闸门里。
 */

export interface FlushOptions {
    /** 轮询间隔（毫秒），测试可注入 */
    tickMs?: number;
    /** 前端兜底超时（毫秒）：即使终端一直没挂载也要触发冲刷，避免后端闸门永久积压 */
    timeoutMs?: number;
    /** 测试注入轮询定时器（默认 window.setTimeout） */
    scheduleTick?: (fn: () => void, ms: number) => void;
    /** 测试注入当前时间（默认 Date.now） */
    now?: () => number;
}

/**
 * 等待终端 ref 就绪后调用 callReady；达到 timeoutMs 仍未就绪也强制调用（兜底）。
 * 幂等由后端保证（TerminalOutputReady 可重复调用）。
 */
export function flushTerminalInitialOutput(
    sessionId: string,
    hasRef: () => boolean,
    callReady: (sessionId: string) => void,
    options: FlushOptions = {},
): void {
    const {
        tickMs = 20,
        timeoutMs = 5000,
        scheduleTick = (fn, ms) => setTimeout(fn, ms),
        now = () => Date.now(),
    } = options;

    const startedAt = now();

    const tick = () => {
        if (hasRef() || now() - startedAt > timeoutMs) {
            callReady(sessionId);
            return;
        }
        scheduleTick(tick, tickMs);
    };

    tick();
}