import type {
    QuickCommandHost,
    QuickCommandStorageAdapter,
    QuickCommand,
    QuickCommandImportAnalysis,
    QuickCommandImportReport,
    QuickCommandImportSelection,
    XshellQuickButtonDir,
} from '@opscopilot/shell-terminal/ui';

type WailsWindow = Window & {
    go?: { main?: { App?: {
        LoadQuickCommands?: () => Promise<QuickCommand[]>;
        AddQuickCommand?: (cmd: QuickCommand) => void;
        UpdateQuickCommand?: (id: string, updates: Partial<QuickCommand> & { id: string }) => void;
        DeleteQuickCommand?: (id: string) => void;
        ReorderQuickCommands?: (ids: string[]) => void;
        DetectXshellQuickButtonDirs?: () => Promise<XshellQuickButtonDir[]>;
        SelectQuickCommandImportFile?: () => Promise<string>;
        SelectQuickCommandImportDirectory?: () => Promise<string>;
        AnalyzeQuickCommandImport?: (
            path: string,
            options: { defaultGroup?: string },
        ) => Promise<QuickCommandImportAnalysis>;
        ApplyQuickCommandImport?: (
            path: string,
            selections: QuickCommandImportSelection[],
            options: { defaultGroup?: string },
        ) => Promise<QuickCommandImportReport>;
    } } };
    runtime?: { EventsOn?: (name: string, handler: (cmds: QuickCommand[]) => void) => () => void };
};

type WailsApp = NonNullable<NonNullable<NonNullable<WailsWindow['go']>['main']>['App']>;

function app(): WailsApp | undefined {
    return (window as WailsWindow).go?.main?.App;
}

/**
 * 调用生成的 Wails 绑定。
 *
 * 与存储适配器的可选链不同，导入相关的调用在方法缺失时要显式报错：静默返回
 * undefined 会让对话框显示成"分析成功但没有数据"，比直接报错更难排查。
 */
async function call<T>(name: keyof WailsApp, ...args: unknown[]): Promise<T> {
    const target = app();
    const fn = target?.[name] as ((...a: unknown[]) => Promise<T>) | undefined;
    if (typeof fn !== 'function') {
        throw new Error(`当前宿主未提供 ${String(name)}`);
    }
    // 用 apply 保留 this：Wails 生成的绑定依赖调用对象。
    return fn.apply(target, args) as Promise<T>;
}

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
        detectQuickButtonDirs() {
            return call<XshellQuickButtonDir[]>('DetectXshellQuickButtonDirs');
        },
        selectImportFile() {
            return call<string>('SelectQuickCommandImportFile');
        },
        selectImportDirectory() {
            return call<string>('SelectQuickCommandImportDirectory');
        },
        analyzeQuickCommandImport(path, defaultGroup) {
            return call<QuickCommandImportAnalysis>('AnalyzeQuickCommandImport', path, { defaultGroup });
        },
        applyQuickCommandImport(path, selections, defaultGroup) {
            return call<QuickCommandImportReport>('ApplyQuickCommandImport', path, selections, { defaultGroup });
        },
    };
}
