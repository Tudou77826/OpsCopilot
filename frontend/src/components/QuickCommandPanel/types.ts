export interface QuickCommand {
    id: string;
    name: string;
    content: string;
    group?: string;
}

/**
 * 存储适配器：意图化单条操作。
 * 旧的全量 save(commands) 会让多窗口互相用旧快照覆盖，已废弃；
 * 增删改各走单条接口，配合后端文件变化热加载保持多窗口一致。
 */
export interface QuickCommandStorageAdapter {
    load(): Promise<QuickCommand[]>;
    add(cmd: QuickCommand): void;
    update(id: string, updates: Partial<QuickCommand>): void;
    remove(id: string): void;
}
