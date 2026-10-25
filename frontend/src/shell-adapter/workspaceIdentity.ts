// A tab identity survives reload, while each mounted page receives a fresh
// server registration. Native WebViews and browser tabs have separate storage.
let key: string;
try {
    key = sessionStorage.getItem('opscopilot-workspace-page') || crypto.randomUUID();
    sessionStorage.setItem('opscopilot-workspace-page', key);
} catch { key = crypto.randomUUID(); }
export const workspacePageKey = key;
let registered = '';
let ready = '';
let snapshot: (() => import('../../../frontend-shell/src/workspace/controller').WorkspaceSnapshot) | undefined;
export function provideWorkspaceSnapshot(provider: NonNullable<typeof snapshot>) {
    snapshot = provider;
    return () => { if (snapshot === provider) snapshot = undefined; };
}
export function currentWorkspaceSnapshot() {
    if (!snapshot) throw Error('当前页面尚未准备好工作台');
    return snapshot();
}
export function registerWorkspaceIdentity(client: string) { registered = client; ready = ''; }
export function markWorkspaceReady(client: string) { if (registered === client) ready = client; }
export function clearWorkspaceIdentity(client: string) { if (registered === client) { registered = ''; ready = ''; } }
export async function currentWorkspaceIdentity(): Promise<string> {
    const deadline = Date.now() + 15000;
    while (Date.now() < deadline) {
        if (registered && ready === registered) return registered;
        await new Promise(resolve => setTimeout(resolve, 50));
    }
    throw Error('当前页面尚未连接工作台，请刷新页面后重试');
}
