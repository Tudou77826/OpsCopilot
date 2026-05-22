export interface QuickCommand {
    id: string;
    name: string;
    content: string;
    group?: string;
}

export interface QuickCommandStorageAdapter {
    load(): Promise<QuickCommand[]>;
    save(commands: QuickCommand[]): void;
}
