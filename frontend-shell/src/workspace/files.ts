export interface FileEntry {
    path: string; name: string; isDir: boolean; size: number; modTime: string; mode: number;
}
export interface FileWorkspace {
    id: string;
    endpoint: { sessionId: string; connectionId: string; title: string; identity: string };
    localPath: string; remotePath: string;
    localEntries: FileEntry[]; remoteEntries: FileEntry[];
    tasks: Array<{ id: string; direction: string; localPath: string; remotePath: string; status: string; bytes: number; total: number; message: string }>;
    ownerTurn: string; pending: string; revision: number; dismissed: boolean; error?: string;
    revealTask?: string; revealNonce?: string;
}
export interface FileWorkspaceUpdate { revision: number; items: FileWorkspace[]; }
