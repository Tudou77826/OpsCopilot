import type { SharedSessionRuntime, SharedSessionEntry, SharedConnectResult } from '@opscopilot/shell-terminal/ui';
import { EventsOn } from '../../wailsjs/runtime/runtime';

type WailsWindow = Window & {
    go?: { main?: { App?: {
        GetSharedSessions?: () => Promise<string>;
        ConnectSharedSession?: (entryKey: string) => Promise<SharedConnectResult>;
        SaveSharedSessionToLocal?: (entryKey: string) => Promise<string>;
        RemoveSharedSession?: (entryKey: string) => Promise<string>;
    } } };
};

/**
 * Wails 对共享团队会话面板的运行时适配。
 * 后端 GetSharedSessions 返回 JSON 字符串 { enabled, sessions }；
 * 未启用时抛错，让 SharedSessionPanel 保持隐藏（与"宿主未提供则不显示"一致）。
 */
export const wailsSessionSharedRuntime: SharedSessionRuntime = {
    async list() {
        const raw = await (window as WailsWindow).go?.main?.App?.GetSharedSessions?.();
        const parsed = raw ? JSON.parse(raw) : {};
        if (!parsed.enabled) {
            throw new Error('共享会话未启用');
        }
        return (Array.isArray(parsed.sessions) ? parsed.sessions : []) as SharedSessionEntry[];
    },
    async connect(entryKey) {
        const result = await (window as WailsWindow).go?.main?.App?.ConnectSharedSession?.(entryKey);
        return result || { success: false, message: '未知错误' };
    },
    async saveToLocal(entryKey) {
        const err = await (window as WailsWindow).go?.main?.App?.SaveSharedSessionToLocal?.(entryKey);
        if (err) throw new Error(err);
    },
    async remove(entryKey) {
        const err = await (window as WailsWindow).go?.main?.App?.RemoveSharedSession?.(entryKey);
        if (err) throw new Error(err);
    },
    onSynced(handler) {
        try {
            return EventsOn('session-share:synced', () => handler());
        } catch {
            // 测试/无 Wails runtime 环境不订阅
            return () => {};
        }
    },
};
