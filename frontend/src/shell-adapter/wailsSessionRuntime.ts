import type {
    SessionManagerRuntime,
    SessionNode,
    ConnectionConfig,
    XshellSessionDir,
    XshellCredentialStatus,
    XshellImportOptions,
    XshellImportAnalysis,
    XshellImportReport,
} from '@opscopilot/shell-terminal/ui';

/** Wails 注入的 App 方法集（只声明本适配器用到的部分）。 */
type WailsApp = {
    GetConnectionTree: () => Promise<SessionNode[]>;
    CreateSavedFolder: (name: string, parentId: string) => Promise<SessionNode>;
    CreateSavedConnection: (config: ConnectionConfig, parentId: string) => Promise<SessionNode>;
    RenameTreeNode: (id: string, newName: string) => Promise<void>;
    UpdateSavedConnection: (id: string, config: ConnectionConfig) => Promise<void>;
    MoveTreeNode: (id: string, newParentId: string, index: number) => Promise<void>;
    DeleteTreeNode: (id: string) => Promise<void>;
    ReorderTreeChildren: (parentId: string, orderedIds: string[]) => Promise<void>;
    DuplicateSavedConnection: (id: string) => Promise<SessionNode>;

    DetectXshellSessionDirs: () => Promise<XshellSessionDir[]>;
    GetXshellImportStatus: () => Promise<XshellCredentialStatus>;
    SelectSessionImportFile: () => Promise<string>;
    SelectSessionImportDirectory: () => Promise<string>;
    AnalyzeXshellImport: (path: string, options: XshellImportOptions) => Promise<XshellImportAnalysis>;
    ApplyXshellImport: (path: string, options: XshellImportOptions) => Promise<XshellImportReport>;
};

type WailsWindow = Window & { go?: { main?: { App?: Partial<WailsApp> } } };

function app(): Partial<WailsApp> | undefined {
    return (window as WailsWindow).go?.main?.App;
}

/**
 * 调用 Wails 绑定的一个方法。
 *
 * 用 apply 而不是取出函数再直接调用，是为了保住 this 绑定——Wails 注入的方法
 * 依赖其宿主对象。返回类型是 Promise：Go 侧改为 (T, error) 约定后，错误会直接让
 * Promise reject，因此这里不再有"检查返回串里是否含错误"的转换。
 */
async function call<T>(name: keyof WailsApp, ...args: unknown[]): Promise<T> {
    const target = app() as Record<string, ((...a: unknown[]) => Promise<T>)> | undefined;
    const fn = target?.[name as string];
    if (!target || typeof fn !== 'function') {
        throw new Error(`当前宿主未提供 ${String(name)}`);
    }
    return fn.apply(target, args);
}

/** Wails 对共享 SessionManager 的唯一运行时适配。 */
export const wailsSessionRuntime: SessionManagerRuntime = {
    async listTree() {
        return (await call<SessionNode[]>('GetConnectionTree')) ?? [];
    },
    async createFolder(name, parentId) {
        await call('CreateSavedFolder', name, parentId);
    },
    async createConnection(config, parentId) {
        await call('CreateSavedConnection', config, parentId);
    },
    async renameNode(id, newName) {
        await call('RenameTreeNode', id, newName);
    },
    async updateConnection(id, config) {
        await call('UpdateSavedConnection', id, config);
    },
    async moveNode(id, newParentId, index) {
        await call('MoveTreeNode', id, newParentId, index);
    },
    async deleteNode(id) {
        await call('DeleteTreeNode', id);
    },
    async reorderNodes(parentId, orderedIds) {
        await call('ReorderTreeChildren', parentId, orderedIds);
    },
    async duplicateConnection(id) {
        await call('DuplicateSavedConnection', id);
    },

    // ── Xshell 导入 ──────────────────────────────────────────
    detectXshellDirs() {
        return call<XshellSessionDir[]>('DetectXshellSessionDirs');
    },
    xshellImportStatus() {
        return call<XshellCredentialStatus>('GetXshellImportStatus');
    },
    selectImportFile() {
        return call<string>('SelectSessionImportFile');
    },
    selectImportDirectory() {
        return call<string>('SelectSessionImportDirectory');
    },
    analyzeXshellImport(path, options) {
        return call<XshellImportAnalysis>('AnalyzeXshellImport', path, options);
    },
    applyXshellImport(path, options) {
        return call<XshellImportReport>('ApplyXshellImport', path, options);
    },
};
