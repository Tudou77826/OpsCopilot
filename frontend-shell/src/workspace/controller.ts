export interface WorkspaceTarget { tabId: string; connectionId: string; }
export interface WorkspaceTab extends WorkspaceTarget { title: string; status: string; ready: boolean; }
export interface WorkspaceSnapshot { tabs: WorkspaceTab[]; active: WorkspaceTarget; }
export interface WorkspacePresentation { id: string; action: 'open' | 'select'; tab: WorkspaceTab; }
export interface WorkspaceHost {
    attach(): Promise<string>;
    detach(client: string): Promise<void>;
    pull(client: string, snapshot: WorkspaceSnapshot): Promise<WorkspacePresentation[]>;
    acknowledge(client: string, id: string, success: boolean): Promise<void>;
}
export interface TerminalWorkspaceController {
    selectWorkspace?(id: string): boolean;
    presentedWorkspace?(): string | null;
    selectTerminal(id: string): boolean;
    presentedTerminal(): string | null;
}
export interface WorkspaceView {
    snapshot(): WorkspaceSnapshot;
    open(tab: WorkspaceTab): void;
    select(id: string): boolean;
    discard(tab: WorkspaceTab): void;
}

const wait = () => new Promise<void>(resolve => setTimeout(resolve, 200));
const sameTarget = (a: WorkspaceTarget, b: WorkspaceTarget) => a.tabId === b.tabId && a.connectionId === b.connectionId;

/** A presentation receipt requires the actual selected layout node and a mounted
 * terminal, not merely a successful dial or an update scheduled in React. */
export function startWorkspaceController(host: WorkspaceHost, view: () => WorkspaceView, onClient?: (client: string) => void): () => void {
    let live = true;
    let client = '';
    async function present(command: WorkspacePresentation) {
        let created = false;
        let acknowledged = false;
        try {
            if (!live) return;
            const existing = view().snapshot().tabs.find(tab => tab.tabId === command.tab.tabId);
            if (command.action === 'open' && !existing) {
                view().open(command.tab);
                created = true;
            }
            if (existing && !sameTarget(existing, command.tab)) throw Error('连接已更换');
            const deadline = Date.now() + 9000;
            while (live && Date.now() < deadline) {
                view().select(command.tab.tabId);
                await wait();
                if (!live) return;
                const snapshot = view().snapshot();
                const pending = await host.pull(client, snapshot);
                if (!pending.some(item => item.id === command.id)) return;
                if (sameTarget(snapshot.active, command.tab) && snapshot.tabs.some(tab => sameTarget(tab, command.tab) && tab.ready && tab.status === 'connected')) {
                    await host.acknowledge(client, command.id, true);
                    acknowledged = true;
                    return;
                }
            }
            if (live) await host.acknowledge(client, command.id, false);
        } finally {
            // Unknown acknowledgement outcomes also close the exact new resource;
            // automation must never continue in a tab that the UI discarded.
            if (created && !acknowledged) view().discard(command.tab);
        }
    }
    void (async () => {
        while (live && !client) {
            try { client = await host.attach(); }
            catch { await wait(); }
        }
        if (!live) { if (client) await host.detach(client).catch(() => undefined); return; }
        onClient?.(client);
        while (live) {
            try {
                const commands = await host.pull(client, view().snapshot());
                for (const command of commands || []) { if (live) await present(command); }
            } catch (error) {
                // A newer page owns presentation. Never reattach and steal it back.
                if (String(error).includes('工作台请求已失效')) break;
            }
            await wait();
        }
    })().catch(() => undefined);
    return () => { live = false; if (client) void host.detach(client).catch(() => undefined); };
}
