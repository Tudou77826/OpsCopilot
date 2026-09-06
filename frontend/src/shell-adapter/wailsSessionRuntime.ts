import type { SessionManagerRuntime, SessionNode, ConnectionConfig } from '@opscopilot/shell-terminal/ui';

type WailsWindow = Window & {
    go?: { main?: { App?: {
        GetSavedSessions?: () => Promise<SessionNode[]>;
        DeleteSavedSession?: (id: string) => Promise<string>;
        RenameSavedSession?: (id: string, newName: string) => Promise<string>;
        UpdateSavedSession?: (id: string, config: ConnectionConfig) => Promise<string>;
        CreateSavedFolder?: (name: string) => Promise<string>;
        DuplicateSavedSession?: (id: string) => Promise<string>;
    } } };
};

/** Wails 对共享 SessionManager 的唯一运行时适配。 */
export const wailsSessionRuntime: SessionManagerRuntime = {
    async listSessions() {
        return (await (window as WailsWindow).go?.main?.App?.GetSavedSessions?.()) || [];
    },
    async deleteSession(id) {
        const err = await (window as WailsWindow).go?.main?.App?.DeleteSavedSession?.(id);
        if (err) throw new Error(err);
    },
    async renameSession(id, newName) {
        const err = await (window as WailsWindow).go?.main?.App?.RenameSavedSession?.(id, newName);
        if (err) throw new Error(err);
    },
    async updateSession(id, config, group) {
        const err = await (window as WailsWindow).go?.main?.App?.UpdateSavedSession?.(id, { ...config, group });
        if (err) throw new Error(err);
    },
    async createFolder(name) {
        const err = await (window as WailsWindow).go?.main?.App?.CreateSavedFolder?.(name);
        if (err) throw new Error(err);
    },
    async duplicateSession(id) {
        const err = await (window as WailsWindow).go?.main?.App?.DuplicateSavedSession?.(id);
        if (err) throw new Error(err);
    },
};
