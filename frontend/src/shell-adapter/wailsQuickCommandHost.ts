import type { QuickCommandHost, QuickCommandStorageAdapter, QuickCommand } from '@opscopilot/shell-terminal/ui';

type WailsWindow = Window & {
    go?: { main?: { App?: {
        LoadQuickCommands?: () => Promise<QuickCommand[]>;
        AddQuickCommand?: (cmd: QuickCommand) => void;
        UpdateQuickCommand?: (id: string, updates: Partial<QuickCommand> & { id: string }) => void;
        DeleteQuickCommand?: (id: string) => void;
        ReorderQuickCommands?: (ids: string[]) => void;
    } } };
    runtime?: { EventsOn?: (name: string, handler: (cmds: QuickCommand[]) => void) => () => void };
};

// 存储适配器：单条意图化操作，配合后端文件变化热加载保持多窗口一致。
class WailsQuickCommandStorage implements QuickCommandStorageAdapter {
    async load(): Promise<QuickCommand[]> {
        const cmds = await (window as WailsWindow).go?.main?.App?.LoadQuickCommands?.();
        return cmds || [];
    }
    add(cmd: QuickCommand): void {
        (window as WailsWindow).go?.main?.App?.AddQuickCommand?.(cmd);
    }
    update(id: string, updates: Partial<QuickCommand>): void {
        (window as WailsWindow).go?.main?.App?.UpdateQuickCommand?.(id, { ...updates, id });
    }
    remove(id: string): void {
        (window as WailsWindow).go?.main?.App?.DeleteQuickCommand?.(id);
    }
    reorder(ids: string[]): void {
        (window as WailsWindow).go?.main?.App?.ReorderQuickCommands?.(ids);
    }
}

/**
 * 构造 Wails 快捷命令宿主适配器。
 * @param execute 发送命令到激活终端（由 App.tsx 注入 handleQuickCommand）。
 */
export function makeWailsQuickCommandHost(execute: (content: string) => void): QuickCommandHost {
    return {
        execute,
        storage: new WailsQuickCommandStorage(),
        onExternalChange(handler) {
            const runtime = (window as WailsWindow).runtime;
            if (!runtime?.EventsOn) return () => {};
            const off = runtime.EventsOn('quick-commands-updated', (cmds: QuickCommand[]) => handler(cmds));
            return () => {
                if (typeof off === 'function') off();
            };
        },
    };
}
