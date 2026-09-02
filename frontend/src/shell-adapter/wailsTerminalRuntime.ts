import type { TerminalRuntime } from '@opscopilot/shell-terminal/ui';

type WailsWindow = Window & {
    go?: { main?: { App?: {
        ResizeTerminal?: (sessionId: string, cols: number, rows: number) => void | Promise<void>;
        GetCompletions?: (input: string, cursor: number) => string | Promise<string>;
    } } };
};

/** Wails 对共享 Terminal UI 的唯一运行时适配。 */
export const wailsTerminalRuntime: TerminalRuntime = {
    resize(sessionId, cols, rows) {
        return (window as WailsWindow).go?.main?.App?.ResizeTerminal?.(sessionId, cols, rows);
    },
    async getCompletions(input, cursor) {
        const raw = await (window as WailsWindow).go?.main?.App?.GetCompletions?.(input, cursor);
        if (!raw) return null;
        return JSON.parse(raw);
    },
};
