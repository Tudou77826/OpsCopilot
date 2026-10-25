import { useEffect, useMemo, useRef, useState } from 'react';
import { FilesPanel } from '@opscopilot/shell-terminal/ui';
import type { TerminalWorkspaceController } from '../../../../frontend-shell/src/workspace/controller';
import { wailsFileTransferHost } from '../../shell-adapter/wailsFileTransferHost';
import * as api from '../../../wailsjs/go/main/App';
import '../TerminalHandoff/terminalHandoff.css';
import './assistantDatabase.css';

import type { FileWorkspace } from '../../../../frontend-shell/src/workspace/files';

export function useAssistantFileWorkspaces(client: string, controller?: TerminalWorkspaceController) {
    const [items, setItems] = useState<FileWorkspace[]>([]);
    const ready = useRef(new Map<string, string>());
    const latest = useRef(controller); latest.current = controller;
    useEffect(() => {
        if (!client) return;
        let live = true, revision = 0;
        let current: FileWorkspace[] = [];
        const seen = new Set<string>();
        void (async () => {
            while (live) {
                try {
                    const update = await api.FileWorkspacePoll(client, revision);
                    if (!live) return;
                    if (update.revision !== revision) {
                        revision = update.revision; current = update.items.filter(w => !w.dismissed); setItems(current);
                    }
                    for (const w of current) {
                        if (!w.pending || seen.has(w.pending)) continue;
                        latest.current?.selectWorkspace?.(w.id);
                        if (latest.current?.presentedWorkspace?.() !== w.id || ready.current.get(w.id) !== w.pending) continue;
                        await api.FileWorkspacePresented(client, w.id, w.pending, w.endpoint.connectionId);
                        seen.add(w.pending);
                    }
                } catch (error) { if (String(error).includes('工作台请求已失效')) break; }
                await new Promise(resolve => setTimeout(resolve, 200));
            }
        })();
        return () => { live = false; ready.current.clear(); };
    }, [client]);
    return { items, ready };
}

export function AssistantFileWorkspaceView({ workspace, ready }: { workspace: FileWorkspace; ready: React.MutableRefObject<Map<string, string>> }) {
    const content = useRef<HTMLDivElement>(null);
    const current = useRef(workspace); current.current = workspace;
    const [error, setError] = useState('');
    const owned = !!workspace.ownerTurn;
    useEffect(() => {
        if (content.current) (content.current as HTMLElement & { inert: boolean }).inert = owned;
    }, [owned]);
    useEffect(() => () => { ready.current.delete(workspace.id); }, [workspace.id, ready]);
    const host = useMemo(() => {
        const guarded = { ...wailsFileTransferHost, initialRemotePath: '/' };
        // DOM inert also blocks keyboard/focus; this guard handles OS file drops
        // and manual asynchronous operations already in flight at takeover.
        for (const key of ['FTList', 'FTStat', 'FTCancel', 'FTUpload', 'FTDownload', 'FTUploadDir', 'FTDownloadDir', 'FTRemoteMkdir', 'FTRemoteRemove', 'FTRemoteRename', 'FTRemoteReadFile', 'FTRemoteWriteFile', 'LocalList', 'LocalMkdir', 'LocalRemove', 'LocalRename', 'LocalCopy', 'LocalStat', 'SelectSavePath'] as const) {
            const original = guarded[key];
            if (original) Object.assign(guarded, { [key]: (...args: unknown[]) => {
                if (current.current.ownerTurn) return Promise.reject(Error('AI 正在使用此文件页签，请先手动接管'));
                return (original as (...values: unknown[]) => Promise<string>)(...args);
            } });
        }
        return guarded;
    }, [workspace.id]);
    const managed = useMemo(() => ({
        nonce: workspace.pending, localPath: workspace.localPath, remotePath: workspace.remotePath,
        localEntries: workspace.localEntries, remoteEntries: workspace.remoteEntries,
        tasks: workspace.tasks, blocked: owned,
        revealTask: workspace.revealTask, revealNonce: workspace.revealNonce,
        onPresented: (nonce: string) => ready.current.set(workspace.id, nonce),
    }), [workspace, owned, ready]);
    return <section className="oc-ai-db" aria-label={`文件工作区 · ${workspace.endpoint.title}`}>
        {(error || workspace.error) && <div role="alert" className="oc-db-banner">{error || workspace.error}</div>}
        <div className="oc-handoff-viewport">
            <div ref={content} className="oc-handoff-content" aria-disabled={owned}>
                <FilesPanel activeTerminalId={workspace.endpoint.sessionId} terminals={[{ id: workspace.endpoint.sessionId, title: workspace.endpoint.title }]} host={host} managed={managed} />
            </div>
            {owned && <div className="oc-handoff-shield" aria-label="AI 正在使用此文件页签">
                <div className="oc-handoff-control"><span className="oc-handoff-orbit" aria-hidden="true" /><div><strong>AI 正在使用此文件页签</strong><span>传输状态会保留，随时可以接回</span></div>
                    <button onClick={() => void api.FileWorkspaceTakeover(workspace.id).catch(reason => setError(String(reason)))}>手动接管</button>
                </div>
            </div>}
        </div>
    </section>;
}
