import React, { useEffect, useMemo, useRef, useState } from 'react';

interface TerminalSessionLite {
    id: string;
    title: string;
}

interface FileEntry {
    path: string;
    name: string;
    isDir: boolean;
    size: number;
    modTime: string;
    mode: number;
}

interface TransferError {
    code: string;
    message: string;
}

interface FTResponse {
    ok: boolean;
    message?: string;
    error?: TransferError;
    taskId?: string;
    entries?: FileEntry[];
    entry?: FileEntry;
    result?: { bytes: number };
}

interface FilesPanelProps {
    activeTerminalId: string | null;
    terminals: TerminalSessionLite[];
}

type TaskState = {
    taskId: string;
    sessionId: string;
    bytesDone: number;
    bytesTotal: number;
    speedBps: number;
    status: 'running' | 'done' | 'error' | 'cancelled';
    message?: string;
};

type FilePaneProps = {
    title: string;
    badge?: string;
    path: string;
    pathInput: string;
    onPathInputChange: (p: string) => void;
    onGo: () => void;
    onUp: () => void;
    onRefresh: () => void;
    entries: FileEntry[];
    selected: string;
    onSelect: (p: string) => void;
    onOpenDir: (p: string) => void;
    onOpenFile?: (p: string) => void;
    disabled?: boolean;
    toolbar?: React.ReactNode;
};

function FilePane({ title, badge, path, pathInput, onPathInputChange, onGo, onUp, onRefresh, entries, selected, onSelect, onOpenDir, onOpenFile, disabled, toolbar }: FilePaneProps) {
    return (
        <div style={{ ...styles.pane, opacity: disabled ? 0.6 : 1 }}>
            <div style={styles.paneHeader}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    <div style={{ color: '#fff', fontSize: '12px', fontWeight: 600 }}>{title}</div>
                    {badge ? <div style={styles.badge}>{badge}</div> : null}
                </div>
                <div style={{ display: 'flex', gap: '8px' }}>
                    {toolbar}
                </div>
            </div>
            <div style={styles.pathBar}>
                <button style={styles.iconBtn} onClick={onUp} disabled={disabled}>↑</button>
                <button style={styles.iconBtn} onClick={onRefresh} disabled={disabled}>⟳</button>
                <input
                    style={styles.pathInput}
                    value={pathInput}
                    onChange={(e) => onPathInputChange(e.target.value)}
                    onKeyDown={(e) => {
                        if (e.key === 'Enter') onGo();
                    }}
                    disabled={disabled}
                />
                <button style={styles.btn} onClick={onGo} disabled={disabled}>进入</button>
            </div>
            <div style={styles.paneBody}>
                <div style={styles.fileTableWrap}>
                    <table style={styles.table}>
                        <thead>
                            <tr style={{ background: '#1e1e1e' }}>
                                <th style={styles.th}>名称</th>
                                <th style={styles.th}>类型</th>
                                <th style={styles.th}>大小</th>
                                <th style={styles.th}>更新时间</th>
                            </tr>
                        </thead>
                        <tbody>
                            {entries.map(e => (
                                <tr
                                    key={e.path}
                                    style={{ borderTop: '1px solid #333', background: selected === e.path ? '#2a2a2a' : 'transparent', cursor: disabled ? 'not-allowed' : 'pointer' }}
                                    onClick={() => !disabled && onSelect(e.path)}
                                    onDoubleClick={() => {
                                        if (disabled) return;
                                        if (e.isDir) {
                                            onOpenDir(e.path);
                                        } else if (onOpenFile) {
                                            onOpenFile(e.path);
                                        }
                                    }}
                                >
                                    <td style={{ ...styles.td, ...styles.cellName }} title={e.name}>{e.name}</td>
                                    <td style={{ ...styles.td, ...styles.cellType }}>{e.isDir ? '目录' : '文件'}</td>
                                    <td style={{ ...styles.td, ...styles.cellSize }}>{e.isDir ? '-' : e.size}</td>
                                    <td style={{ ...styles.td, ...styles.cellTime }}>{e.modTime ? new Date(e.modTime).toLocaleString() : ''}</td>
                                </tr>
                            ))}
                            {entries.length === 0 ? (
                                <tr>
                                    <td style={styles.td} colSpan={4}>暂无数据</td>
                                </tr>
                            ) : null}
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    );
}

const FilesPanel: React.FC<FilesPanelProps> = ({ activeTerminalId, terminals }) => {
    const defaultSessionId = useMemo(() => activeTerminalId || (terminals[0]?.id ?? ''), [activeTerminalId, terminals]);

    const [sessionId, setSessionId] = useState(defaultSessionId);
    const [protocol, setProtocol] = useState<string>('');
    const [loading, setLoading] = useState(false);
    const [msg, setMsg] = useState('');

    const [localPath, setLocalPath] = useState<string>('');
    const [localPathInput, setLocalPathInput] = useState<string>('');
    const [localEntries, setLocalEntries] = useState<FileEntry[]>([]);
    const [localSelected, setLocalSelected] = useState<string>('');

    const [remotePath, setRemotePath] = useState<string>('/root');
    const [remotePathInput, setRemotePathInput] = useState<string>('/root');
    const [remoteEntries, setRemoteEntries] = useState<FileEntry[]>([]);
    const [remoteSelected, setRemoteSelected] = useState<string>('');
    const [editOpen, setEditOpen] = useState(false);
    const [editPath, setEditPath] = useState('');
    const [editContent, setEditContent] = useState('');
    const [editSaving, setEditSaving] = useState(false);
    const [scpDownloadRemote, setScpDownloadRemote] = useState('');
    const [scpDownloadLocal, setScpDownloadLocal] = useState('');

    const [tasks, setTasks] = useState<Record<string, TaskState>>({});
    const [drawerOpen, setDrawerOpen] = useState(false);
    const refreshTimerRef = useRef<number | null>(null);
    const refreshRetryTimerRef = useRef<number | null>(null);
    const sessionIdRef = useRef<string>(sessionId);
    const protocolRef = useRef<string>(protocol);
    const localPathRef = useRef<string>(localPath);
    const remotePathRef = useRef<string>(remotePath);
    const localEntriesRef = useRef<FileEntry[]>(localEntries);
    const remoteEntriesRef = useRef<FileEntry[]>(remoteEntries);

    useEffect(() => {
        sessionIdRef.current = sessionId;
    }, [sessionId]);
    useEffect(() => {
        protocolRef.current = protocol;
    }, [protocol]);
    useEffect(() => {
        localPathRef.current = localPath;
    }, [localPath]);
    useEffect(() => {
        remotePathRef.current = remotePath;
    }, [remotePath]);
    useEffect(() => {
        localEntriesRef.current = localEntries;
    }, [localEntries]);
    useEffect(() => {
        remoteEntriesRef.current = remoteEntries;
    }, [remoteEntries]);

    useEffect(() => {
        const ids = new Set(terminals.map(t => t.id));
        if (!sessionId) {
            if (defaultSessionId) setSessionId(defaultSessionId);
            return;
        }
        if (ids.size > 0 && !ids.has(sessionId)) {
            setSessionId(defaultSessionId);
        }
    }, [defaultSessionId, sessionId, terminals]);

    const parseResp = (raw: any): FTResponse | null => {
        if (!raw) return null;
        try {
            return JSON.parse(raw) as FTResponse;
        } catch {
            return null;
        }
    };

    const formatError = (resp: FTResponse): string => {
        if (resp.error) return `${resp.error.message} (${resp.error.code})`;
        return resp.message || '失败';
    };

    const isSFTPSupported = () => {
        return protocol.startsWith('sftp');
    };

    const isTransferSupported = () => {
        return protocol.startsWith('sftp') || protocol.startsWith('scp');
    };

    const isSCPMode = () => {
        return protocol.startsWith('scp');
    };

    const sortEntries = (items: FileEntry[]) => {
        return items.slice().sort((a, b) => {
            if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
            return a.name.localeCompare(b.name);
        });
    };

    const localParent = (p: string) => {
        const s = (p || '').replace(/[\\/]+$/, '');
        const idx = Math.max(s.lastIndexOf('\\'), s.lastIndexOf('/'));
        if (idx <= 0) return s;
        return s.slice(0, idx);
    };

    const remoteParent = (p: string) => {
        const s = (p || '').replace(/\/+$/, '');
        const idx = s.lastIndexOf('/');
        if (idx <= 0) return '/';
        return s.slice(0, idx);
    };

    const remoteJoin = (dir: string, name: string) => {
        const d = (dir || '').trim();
        if (!d || d === '/') return '/' + name;
        if (d.endsWith('/')) return d + name;
        return d + '/' + name;
    };

    const refreshProtocol = async (sid?: string) => {
        const targetSessionId = sid || sessionIdRef.current;
        if (!targetSessionId) return;
        try {
            // @ts-ignore
            const raw = await window.go.main.App.FTCheck(targetSessionId);
            const resp = parseResp(raw);
            if (!resp) {
                setProtocol('');
                return;
            }
            if (!resp.ok) {
                setProtocol('');
                setMsg(formatError(resp));
                return;
            }
            setProtocol(resp.message || '');
        } catch {
            setProtocol('');
        }
    };

    const refreshLocal = async (path: string) => {
        setLoading(true);
        setMsg('');
        try {
            // @ts-ignore
            const raw = await window.go.main.App.LocalList(path);
            const resp = parseResp(raw);
            if (!resp) {
                setMsg('返回格式错误');
                return;
            }
            if (!resp.ok) {
                setMsg(formatError(resp));
                return;
            }
            const next = sortEntries(resp.entries || []);
            setLocalEntries(next);
            const nextDir = (path || '').trim() || localParent(resp.entries?.[0]?.path || '');
            if (nextDir) {
                setLocalPath(nextDir);
                setLocalPathInput(nextDir);
            }
        } catch (e: any) {
            setMsg('失败: ' + e.toString());
        } finally {
            setLoading(false);
        }
    };

    const refreshRemote = async (path: string) => {
        const targetSessionId = sessionIdRef.current;
        if (!targetSessionId) return;
        if (!protocolRef.current.startsWith('sftp')) {
            setRemoteEntries([]);
            return;
        }
        setLoading(true);
        setMsg('');
        try {
            // @ts-ignore
            const raw = await window.go.main.App.FTList(targetSessionId, path);
            const resp = parseResp(raw);
            if (!resp) {
                setMsg('返回格式错误');
                return;
            }
            if (!resp.ok) {
                setMsg(formatError(resp));
                return;
            }
            const next = sortEntries(resp.entries || []);
            setRemoteEntries(next);
            setRemotePath(path);
            setRemotePathInput(path);
        } catch (e: any) {
            setMsg('失败: ' + e.toString());
        } finally {
            setLoading(false);
        }
    };

    const refreshLocalAuto = async () => {
        const before = localEntriesRef.current.length;
        await refreshLocal(localPathRef.current);
        const after = localEntriesRef.current.length;
        if (before > 0 && after === 0) {
            if (refreshRetryTimerRef.current) window.clearTimeout(refreshRetryTimerRef.current);
            refreshRetryTimerRef.current = window.setTimeout(() => {
                refreshLocal(localPathRef.current);
            }, 600);
        }
    };

    const refreshRemoteAuto = async () => {
        if (!protocolRef.current.startsWith('sftp')) return;
        const before = remoteEntriesRef.current.length;
        await refreshRemote(remotePathRef.current);
        const after = remoteEntriesRef.current.length;
        if (before > 0 && after === 0) {
            if (refreshRetryTimerRef.current) window.clearTimeout(refreshRetryTimerRef.current);
            refreshRetryTimerRef.current = window.setTimeout(() => {
                refreshRemote(remotePathRef.current);
            }, 600);
        }
    };

    useEffect(() => {
        refreshLocal('');
    }, []);

    useEffect(() => {
        if (!sessionId) return;
        setRemoteSelected('');
        refreshProtocol(sessionId).then(() => {
            refreshRemote(remotePath);
        });
    }, [sessionId]);

    useEffect(() => {
        setLocalPathInput(localPath);
    }, [localPath]);

    useEffect(() => {
        setRemotePathInput(remotePath);
    }, [remotePath]);

    useEffect(() => {
        let offProgress: (() => void) | undefined;
        let offDone: (() => void) | undefined;

        // @ts-ignore
        if (window.runtime && window.runtime.EventsOn) {
            // @ts-ignore
            offProgress = window.runtime.EventsOn('file-transfer-progress', (data: any) => {
                const tid = data?.taskId as string;
                if (!tid) return;
                setTasks(prev => {
                    const cur = prev[tid] || {
                        taskId: tid,
                        sessionId: data?.sessionId || '',
                        bytesDone: 0,
                        bytesTotal: -1,
                        speedBps: 0,
                        status: 'running' as const
                    };
                    return {
                        ...prev,
                        [tid]: {
                            ...cur,
                            sessionId: data?.sessionId || cur.sessionId,
                            bytesDone: Number(data?.bytesDone ?? cur.bytesDone),
                            bytesTotal: Number(data?.bytesTotal ?? cur.bytesTotal),
                            speedBps: Number(data?.speedBps ?? cur.speedBps),
                            status: 'running'
                        }
                    };
                });
            });

            // @ts-ignore
            offDone = window.runtime.EventsOn('file-transfer-done', (data: any) => {
                const tid = data?.taskId as string;
                if (!tid) return;
                setTasks(prev => {
                    const cur = prev[tid] || {
                        taskId: tid,
                        sessionId: data?.sessionId || '',
                        bytesDone: 0,
                        bytesTotal: -1,
                        speedBps: 0,
                        status: 'running' as const
                    };
                    const ok = !!data?.ok;
                    const status = ok ? 'done' : (data?.message?.includes('取消') ? 'cancelled' : 'error');
                    return {
                        ...prev,
                        [tid]: {
                            ...cur,
                            status,
                            message: data?.message || (ok ? '完成' : '失败')
                        }
                    };
                });
                setDrawerOpen(true);

                const ok = !!data?.ok;
                const sid = (data?.sessionId as string) || '';
                if (!ok) return;
                if (sid && sid !== sessionIdRef.current) return;

                if (refreshTimerRef.current) {
                    window.clearTimeout(refreshTimerRef.current);
                }
                refreshTimerRef.current = window.setTimeout(() => {
                    refreshLocalAuto();
                    refreshRemoteAuto();
                }, 150);
            });
        }

        return () => {
            if (offProgress) offProgress();
            if (offDone) offDone();
            if (refreshTimerRef.current) {
                window.clearTimeout(refreshTimerRef.current);
                refreshTimerRef.current = null;
            }
            if (refreshRetryTimerRef.current) {
                window.clearTimeout(refreshRetryTimerRef.current);
                refreshRetryTimerRef.current = null;
            }
        };
    }, []);

    const cancelTask = async (taskId: string) => {
        setLoading(true);
        try {
            // @ts-ignore
            await window.go.main.App.FTCancel(taskId);
        } finally {
            setLoading(false);
        }
    };

    const startUploadSelected = async () => {
        if (!sessionId) {
            setMsg('请先选择会话');
            return;
        }
        if (!isTransferSupported()) {
            setMsg('对端不支持文件传输');
            return;
        }
        const src = localSelected;
        if (!src) {
            setMsg('请先选择本地文件');
            return;
        }
        const entry = localEntries.find(e => e.path === src);
        if (!entry || entry.isDir) {
            setMsg('仅支持上传文件');
            return;
        }
        await startUploadFile(entry);
    };

    const startUploadFile = async (entry: FileEntry) => {
        if (!sessionId) {
            setMsg('请先选择会话');
            return;
        }
        if (!isTransferSupported()) {
            setMsg('对端不支持文件传输');
            return;
        }
        if (!entry || entry.isDir) {
            setMsg('仅支持上传文件');
            return;
        }
        const baseDir = isSCPMode() ? (remotePathInput.trim() || remotePath) : remotePath;
        const dst = remoteJoin(baseDir, entry.name);

        if (protocolRef.current.startsWith('sftp')) {
            try {
                // @ts-ignore
                const raw = await window.go.main.App.FTStat(sessionIdRef.current, dst);
                const resp = parseResp(raw);
                if (resp && resp.ok) {
                    const ok = confirm(`远端已存在同名文件：\n${dst}\n\n是否覆盖？`);
                    if (!ok) return;
                }
            } catch {
            }
        } else if (protocolRef.current.startsWith('scp')) {
            const ok = confirm(`SCP 模式无法检测远端是否存在同名文件：\n${dst}\n\n是否继续上传（可能覆盖）？`);
            if (!ok) return;
        }

        setLoading(true);
        setMsg('');
        try {
            // @ts-ignore
            const raw = await window.go.main.App.FTUpload(sessionId, entry.path, dst);
            const resp = parseResp(raw);
            if (!resp) {
                setMsg('返回格式错误');
                return;
            }
            if (!resp.ok) {
                setMsg(formatError(resp));
                return;
            }
            if (resp.taskId) {
                setTasks(prev => ({
                    ...prev,
                    [resp.taskId as string]: {
                        taskId: resp.taskId as string,
                        sessionId,
                        bytesDone: 0,
                        bytesTotal: -1,
                        speedBps: 0,
                        status: 'running'
                    }
                }));
                setDrawerOpen(true);
            }
        } catch (e: any) {
            setMsg('失败: ' + e.toString());
        } finally {
            setLoading(false);
        }
    };

    const startDownloadSelected = async () => {
        if (!sessionId) {
            setMsg('请先选择会话');
            return;
        }
        if (!isTransferSupported()) {
            setMsg('对端不支持文件传输');
            return;
        }
        if (isSCPMode()) {
            setMsg('SCP 模式请使用右侧“下载”表单');
            return;
        }
        const src = remoteSelected;
        if (!src) {
            setMsg('请先选择远端文件');
            return;
        }
        const entry = remoteEntries.find(e => e.path === src);
        if (!entry || entry.isDir) {
            setMsg('仅支持下载文件');
            return;
        }
        await startDownloadFile(entry);
    };

    const startDownloadFile = async (entry: FileEntry) => {
        if (!sessionId) {
            setMsg('请先选择会话');
            return;
        }
        if (!isTransferSupported()) {
            setMsg('对端不支持文件传输');
            return;
        }
        if (isSCPMode()) {
            setMsg('SCP 模式请使用右侧“下载”表单');
            return;
        }
        if (!entry || entry.isDir) {
            setMsg('仅支持下载文件');
            return;
        }
        const dst = localPath ? `${localPath}${localPath.endsWith('\\') || localPath.endsWith('/') ? '' : '\\'}${entry.name}` : entry.name;
        setLoading(true);
        setMsg('');
        try {
            // @ts-ignore
            const raw = await window.go.main.App.FTDownload(sessionId, entry.path, dst);
            const resp = parseResp(raw);
            if (!resp) {
                setMsg('返回格式错误');
                return;
            }
            if (!resp.ok) {
                setMsg(formatError(resp));
                return;
            }
            if (resp.taskId) {
                setTasks(prev => ({
                    ...prev,
                    [resp.taskId as string]: {
                        taskId: resp.taskId as string,
                        sessionId,
                        bytesDone: 0,
                        bytesTotal: -1,
                        speedBps: 0,
                        status: 'running'
                    }
                }));
                setDrawerOpen(true);
            }
        } catch (e: any) {
            setMsg('失败: ' + e.toString());
        } finally {
            setLoading(false);
        }
    };

    const startDownloadByPath = async () => {
        if (!sessionId) {
            setMsg('请先选择会话');
            return;
        }
        if (!isTransferSupported()) {
            setMsg('对端不支持文件传输');
            return;
        }
        const rp = scpDownloadRemote.trim();
        const lp = scpDownloadLocal.trim();
        if (!rp || !lp) {
            setMsg('请填写远端路径与本地保存路径');
            return;
        }
        setLoading(true);
        setMsg('');
        try {
            // @ts-ignore
            const raw = await window.go.main.App.FTDownload(sessionId, rp, lp);
            const resp = parseResp(raw);
            if (!resp) {
                setMsg('返回格式错误');
                return;
            }
            if (!resp.ok) {
                setMsg(formatError(resp));
                return;
            }
            if (resp.taskId) {
                setTasks(prev => ({
                    ...prev,
                    [resp.taskId as string]: {
                        taskId: resp.taskId as string,
                        sessionId,
                        bytesDone: 0,
                        bytesTotal: -1,
                        speedBps: 0,
                        status: 'running'
                    }
                }));
                setDrawerOpen(true);
            }
        } catch (e: any) {
            setMsg('失败: ' + e.toString());
        } finally {
            setLoading(false);
        }
    };

    const createRemoteFolder = async () => {
        if (!sessionId) return;
        if (!isSFTPSupported()) {
            setMsg('当前模式不支持远端目录操作');
            return;
        }
        const name = prompt('新建文件夹名称');
        if (!name) return;
        const p = remoteJoin(remotePath, name);
        setLoading(true);
        setMsg('');
        try {
            // @ts-ignore
            const raw = await window.go.main.App.FTRemoteMkdir(sessionId, p);
            const resp = parseResp(raw);
            if (!resp) {
                setMsg('返回格式错误');
                return;
            }
            if (!resp.ok) {
                setMsg(formatError(resp));
                return;
            }
            await refreshRemote(remotePath);
        } catch (e: any) {
            setMsg('失败: ' + e.toString());
        } finally {
            setLoading(false);
        }
    };

    const deleteRemoteSelected = async () => {
        if (!sessionId) return;
        if (!isSFTPSupported()) {
            setMsg('当前模式不支持远端删除');
            return;
        }
        if (!remoteSelected) {
            setMsg('请先选择远端文件或目录');
            return;
        }
        const ok = confirm('确定要删除所选远端项吗？');
        if (!ok) return;
        setLoading(true);
        setMsg('');
        try {
            // @ts-ignore
            const raw = await window.go.main.App.FTRemoteRemove(sessionId, remoteSelected);
            const resp = parseResp(raw);
            if (!resp) {
                setMsg('返回格式错误');
                return;
            }
            if (!resp.ok) {
                setMsg(formatError(resp));
                return;
            }
            setRemoteSelected('');
            await refreshRemote(remotePath);
        } catch (e: any) {
            setMsg('失败: ' + e.toString());
        } finally {
            setLoading(false);
        }
    };

    const renameRemoteSelected = async () => {
        if (!sessionId) return;
        if (!isSFTPSupported()) {
            setMsg('当前模式不支持远端重命名');
            return;
        }
        if (!remoteSelected) {
            setMsg('请先选择远端文件或目录');
            return;
        }
        const entry = remoteEntries.find(e => e.path === remoteSelected);
        const next = prompt('重命名为', entry?.name || '');
        if (!next || !entry) return;
        const parent = remoteParent(entry.path);
        const newPath = remoteJoin(parent, next);
        setLoading(true);
        setMsg('');
        try {
            // @ts-ignore
            const raw = await window.go.main.App.FTRemoteRename(sessionId, entry.path, newPath);
            const resp = parseResp(raw);
            if (!resp) {
                setMsg('返回格式错误');
                return;
            }
            if (!resp.ok) {
                setMsg(formatError(resp));
                return;
            }
            setRemoteSelected('');
            await refreshRemote(remotePath);
        } catch (e: any) {
            setMsg('失败: ' + e.toString());
        } finally {
            setLoading(false);
        }
    };

    const openRemoteEditor = async () => {
        if (!sessionId) return;
        if (!isSFTPSupported()) {
            setMsg('当前模式不支持远端文件直读');
            return;
        }
        if (!remoteSelected) {
            setMsg('请先选择远端文件');
            return;
        }
        const entry = remoteEntries.find(e => e.path === remoteSelected);
        if (!entry || entry.isDir) {
            setMsg('仅支持编辑文件');
            return;
        }
        setLoading(true);
        setMsg('');
        try {
            // @ts-ignore
            const raw = await window.go.main.App.FTRemoteReadFile(sessionId, entry.path, 262144);
            const resp = parseResp(raw);
            if (!resp) {
                setMsg('返回格式错误');
                return;
            }
            if (!resp.ok) {
                setMsg(formatError(resp));
                return;
            }
            setEditPath(entry.path);
            setEditContent((resp as any).content || '');
            setEditOpen(true);
        } catch (e: any) {
            setMsg('失败: ' + e.toString());
        } finally {
            setLoading(false);
        }
    };

    const saveRemoteEditor = async () => {
        if (!sessionId) return;
        if (!editPath) return;
        setEditSaving(true);
        setMsg('');
        try {
            // @ts-ignore
            const raw = await window.go.main.App.FTRemoteWriteFile(sessionId, editPath, editContent);
            const resp = parseResp(raw);
            if (!resp) {
                setMsg('返回格式错误');
                return;
            }
            if (!resp.ok) {
                setMsg(formatError(resp));
                return;
            }
            setEditOpen(false);
            await refreshRemote(remotePath);
        } catch (e: any) {
            setMsg('失败: ' + e.toString());
        } finally {
            setEditSaving(false);
        }
    };

    const createLocalFolder = async () => {
        const name = prompt('新建文件夹名称');
        if (!name) return;
        const p = localPath ? `${localPath}${localPath.endsWith('\\') || localPath.endsWith('/') ? '' : '\\'}${name}` : name;
        setLoading(true);
        setMsg('');
        try {
            // @ts-ignore
            const raw = await window.go.main.App.LocalMkdir(p);
            const resp = parseResp(raw);
            if (!resp) {
                setMsg('返回格式错误');
                return;
            }
            if (!resp.ok) {
                setMsg(formatError(resp));
                return;
            }
            await refreshLocal(localPath);
        } catch (e: any) {
            setMsg('失败: ' + e.toString());
        } finally {
            setLoading(false);
        }
    };

    const deleteLocalSelected = async () => {
        if (!localSelected) {
            setMsg('请先选择本地文件或目录');
            return;
        }
        const ok = confirm('确定要删除所选项吗？');
        if (!ok) return;
        setLoading(true);
        setMsg('');
        try {
            // @ts-ignore
            const raw = await window.go.main.App.LocalRemove(localSelected);
            const resp = parseResp(raw);
            if (!resp) {
                setMsg('返回格式错误');
                return;
            }
            if (!resp.ok) {
                setMsg(formatError(resp));
                return;
            }
            setLocalSelected('');
            await refreshLocal(localPath);
        } catch (e: any) {
            setMsg('失败: ' + e.toString());
        } finally {
            setLoading(false);
        }
    };

    const renameLocalSelected = async () => {
        if (!localSelected) {
            setMsg('请先选择本地文件或目录');
            return;
        }
        const entry = localEntries.find(e => e.path === localSelected);
        const next = prompt('重命名为', entry?.name || '');
        if (!next || !entry) return;
        const parent = localParent(entry.path);
        const newPath = `${parent}${parent.endsWith('\\') || parent.endsWith('/') ? '' : '\\'}${next}`;
        setLoading(true);
        setMsg('');
        try {
            // @ts-ignore
            const raw = await window.go.main.App.LocalRename(entry.path, newPath);
            const resp = parseResp(raw);
            if (!resp) {
                setMsg('返回格式错误');
                return;
            }
            if (!resp.ok) {
                setMsg(formatError(resp));
                return;
            }
            setLocalSelected('');
            await refreshLocal(localPath);
        } catch (e: any) {
            setMsg('失败: ' + e.toString());
        } finally {
            setLoading(false);
        }
    };

    const taskList = Object.values(tasks)
        .filter(t => !sessionId || t.sessionId === sessionId)
        .slice()
        .sort((a, b) => a.taskId.localeCompare(b.taskId));

    return (
        <div style={{ padding: '12px', color: '#ddd', display: 'flex', flexDirection: 'column', gap: '10px', height: '100%', minHeight: 0, overflow: 'auto' }}>
            <div style={{ display: 'flex', gap: '10px', alignItems: 'center', flexWrap: 'wrap' as const }}>
                <div style={{ color: '#aaa' }}>会话</div>
                <select style={styles.select} value={sessionId} onChange={(e) => setSessionId(e.target.value)}>
                    {terminals.map(t => (
                        <option key={t.id} value={t.id}>
                            {t.title || t.id}
                        </option>
                    ))}
                </select>
                {protocol ? <div style={styles.badge}>{protocol}</div> : <div style={styles.badgeMuted}>未探测</div>}
                <div style={{ flex: 1 }} />
                <button style={styles.btnSecondary} onClick={() => setDrawerOpen(v => !v)}>
                    {drawerOpen ? '隐藏队列' : '显示队列'}
                </button>
            </div>

            {msg ? <div style={{ color: '#aaa', fontSize: '12px' }}>{msg}</div> : null}

            {!isSFTPSupported() && protocol.startsWith('scp') ? (
                <div style={{ color: '#aaa', fontSize: '12px' }}>
                    当前为 SCP 降级模式，仅支持上传/下载，不支持远端浏览与管理。
                </div>
            ) : null}

            <div style={styles.split}>
                <FilePane
                    title="本地"
                    badge={localPath ? localPath : ''}
                    path={localPath}
                    pathInput={localPathInput}
                    onPathInputChange={setLocalPathInput}
                    onGo={() => refreshLocal(localPathInput)}
                    onUp={() => {
                        const p = localParent(localPath);
                        refreshLocal(p);
                    }}
                    onRefresh={() => refreshLocal(localPath)}
                    entries={localEntries}
                    selected={localSelected}
                    onSelect={setLocalSelected}
                    onOpenDir={(p) => {
                        setLocalSelected('');
                        refreshLocal(p);
                    }}
                    onOpenFile={(p) => {
                        const entry = localEntries.find(e => e.path === p);
                        if (!entry || entry.isDir) return;
                        startUploadFile(entry);
                    }}
                    toolbar={
                        <>
                            <button style={styles.btnSecondary} onClick={createLocalFolder} disabled={loading}>新建</button>
                            <button style={styles.btnSecondary} onClick={renameLocalSelected} disabled={loading || !localSelected}>重命名</button>
                            <button style={styles.btnDanger} onClick={deleteLocalSelected} disabled={loading || !localSelected}>删除</button>
                        </>
                    }
                />

                {isSCPMode() ? (
                    <div style={styles.scpPane}>
                        <div style={styles.paneHeader}>
                            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                                <div style={{ color: '#fff', fontSize: '12px', fontWeight: 600 }}>远端（SCP）</div>
                                <div style={styles.badge}>{protocol}</div>
                            </div>
                        </div>
                        <div style={styles.scpBody}>
                            <div style={styles.scpGroup}>
                                <div style={styles.scpLabel}>上传目标目录</div>
                                <input style={styles.pathInput} value={remotePathInput} onChange={(e) => setRemotePathInput(e.target.value)} />
                                <div style={styles.scpHint}>双击左侧本地文件将上传到该目录。</div>
                            </div>
                            <div style={styles.scpGroup}>
                                <div style={styles.scpLabel}>下载远端文件</div>
                                <input style={styles.pathInput} value={scpDownloadRemote} onChange={(e) => setScpDownloadRemote(e.target.value)} placeholder="/path/to/file" />
                                <div style={styles.scpLabel}>本地保存路径</div>
                                <input style={styles.pathInput} value={scpDownloadLocal} onChange={(e) => setScpDownloadLocal(e.target.value)} placeholder="C:\\path\\to\\file" />
                                <button style={styles.btn} onClick={startDownloadByPath} disabled={loading || !scpDownloadRemote || !scpDownloadLocal}>开始下载</button>
                            </div>
                        </div>
                    </div>
                ) : (
                    <FilePane
                        title="远端"
                        badge={remotePath}
                        path={remotePath}
                        pathInput={remotePathInput}
                        onPathInputChange={setRemotePathInput}
                        onGo={() => refreshRemote(remotePathInput)}
                        onUp={() => refreshRemote(remoteParent(remotePath))}
                        onRefresh={() => refreshRemote(remotePath)}
                        entries={remoteEntries}
                        selected={remoteSelected}
                        onSelect={setRemoteSelected}
                        onOpenDir={(p) => {
                            const next = p;
                            setRemoteSelected('');
                            refreshRemote(next);
                        }}
                        onOpenFile={(p) => {
                            const entry = remoteEntries.find(e => e.path === p);
                            if (!entry || entry.isDir) return;
                            startDownloadFile(entry);
                        }}
                        disabled={!isSFTPSupported()}
                        toolbar={
                            <>
                                <button style={styles.btnSecondary} onClick={createRemoteFolder} disabled={loading || !isSFTPSupported()}>新建</button>
                                <button style={styles.btnSecondary} onClick={renameRemoteSelected} disabled={loading || !remoteSelected || !isSFTPSupported()}>重命名</button>
                                <button style={styles.btnDanger} onClick={deleteRemoteSelected} disabled={loading || !remoteSelected || !isSFTPSupported()}>删除</button>
                                <button style={styles.btnSecondary} onClick={openRemoteEditor} disabled={loading || !remoteSelected || !isSFTPSupported()}>编辑</button>
                            </>
                        }
                    />
                )}
            </div>

            {drawerOpen ? (
                <div style={styles.drawer}>
                    <div style={styles.drawerHeader}>
                        <div style={{ color: '#fff', fontSize: '13px', fontWeight: 600 }}>传输队列</div>
                        <div style={{ flex: 1 }} />
                        <button style={styles.btnSecondary} onClick={() => setDrawerOpen(false)}>收起</button>
                    </div>
                    <div style={styles.drawerBody}>
                        {taskList.length === 0 ? (
                            <div style={{ color: '#888', fontSize: '12px' }}>暂无任务</div>
                        ) : (
                            taskList.map(t => (
                                <div key={t.taskId} style={styles.taskRow}>
                                    <div style={styles.taskId}>{t.taskId.slice(0, 8)}</div>
                                    <div style={styles.taskStatus}>{t.status}</div>
                                    <div style={styles.taskProgress}>
                                        {t.bytesTotal > 0 ? `${t.bytesDone}/${t.bytesTotal}` : `${t.bytesDone}`}
                                    </div>
                                    <div style={styles.taskSpeed}>{t.speedBps > 0 ? `${t.speedBps} B/s` : ''}</div>
                                    <div style={styles.taskMsg}>{t.message || ''}</div>
                                    {t.status === 'running' ? (
                                        <button style={styles.btnSecondary} onClick={() => cancelTask(t.taskId)} disabled={loading}>
                                            取消
                                        </button>
                                    ) : null}
                                </div>
                            ))
                        )}
                    </div>
                </div>
            ) : null}

            {editOpen ? (
                <div style={styles.modalOverlay}>
                    <div style={styles.modal}>
                        <div style={styles.modalHeader}>
                            <div style={{ color: '#fff', fontSize: '13px', fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{editPath}</div>
                            <button style={styles.btnSecondary} onClick={() => !editSaving && setEditOpen(false)} disabled={editSaving}>×</button>
                        </div>
                        <div style={styles.modalBody}>
                            <textarea
                                style={styles.textarea}
                                value={editContent}
                                onChange={(e) => setEditContent(e.target.value)}
                            />
                        </div>
                        <div style={styles.modalFooter}>
                            <button style={styles.btnSecondary} onClick={() => setEditOpen(false)} disabled={editSaving}>取消</button>
                            <button style={styles.btn} onClick={saveRemoteEditor} disabled={editSaving}>
                                {editSaving ? '保存中...' : '保存'}
                            </button>
                        </div>
                    </div>
                </div>
            ) : null}
        </div>
    );
};

const styles: Record<string, React.CSSProperties> = {
    select: {
        padding: '6px 8px',
        borderRadius: '4px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#1e1e1e',
        color: '#fff',
        outline: 'none',
        minWidth: '220px'
    },
    badge: {
        padding: '4px 8px',
        borderRadius: '999px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#1e1e1e',
        color: '#ddd',
        fontSize: '12px'
    },
    badgeMuted: {
        padding: '4px 8px',
        borderRadius: '999px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#1e1e1e',
        color: '#888',
        fontSize: '12px'
    },
    btn: {
        padding: '6px 10px',
        borderRadius: '4px',
        border: 'none',
        backgroundColor: '#007acc',
        color: '#fff',
        cursor: 'pointer',
        fontSize: '12px',
        minWidth: '78px'
    },
    btnSecondary: {
        padding: '6px 10px',
        borderRadius: '4px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#1e1e1e',
        color: '#ddd',
        cursor: 'pointer',
        fontSize: '12px'
    },
    btnDanger: {
        padding: '6px 10px',
        borderRadius: '4px',
        border: '1px solid #7a2e2e',
        backgroundColor: '#2a1a1a',
        color: '#f2b8b5',
        cursor: 'pointer',
        fontSize: '12px'
    },
    iconBtn: {
        padding: '6px 8px',
        borderRadius: '4px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#1e1e1e',
        color: '#ddd',
        cursor: 'pointer',
        fontSize: '12px'
    },
    split: {
        flex: 1,
        display: 'flex',
        gap: '10px',
        overflow: 'hidden',
        minHeight: 0
    },
    pane: {
        flex: 1,
        border: '1px solid #333',
        borderRadius: '8px',
        overflow: 'hidden',
        display: 'flex',
        flexDirection: 'column',
        minHeight: 0
    },
    paneHeader: {
        padding: '10px 10px',
        borderBottom: '1px solid #333',
        backgroundColor: '#252526',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: '8px'
    },
    pathBar: {
        padding: '8px 10px',
        borderBottom: '1px solid #333',
        display: 'flex',
        gap: '8px',
        alignItems: 'center',
        backgroundColor: '#1e1e1e'
    },
    pathInput: {
        flex: 1,
        padding: '6px 8px',
        borderRadius: '4px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#1e1e1e',
        color: '#fff',
        outline: 'none',
        fontSize: '12px'
    },
    paneBody: {
        flex: 1,
        display: 'flex',
        minHeight: 0
    },
    fileTableWrap: {
        flex: 1,
        overflow: 'auto'
    },
    table: {
        width: '100%',
        borderCollapse: 'collapse',
        tableLayout: 'fixed'
    },
    th: {
        textAlign: 'left',
        fontWeight: 600,
        fontSize: '12px',
        color: '#bbb',
        padding: '8px 10px',
        position: 'sticky' as const,
        top: 0,
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis'
    },
    td: {
        fontSize: '12px',
        color: '#ddd',
        padding: '8px 10px',
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis'
    },
    cellName: {
        width: '50%'
    },
    cellType: {
        width: '70px',
        minWidth: '70px'
    },
    cellSize: {
        width: '90px',
        minWidth: '90px'
    },
    cellTime: {
        width: '180px',
        minWidth: '180px'
    },
    scpPane: {
        flex: 1,
        border: '1px solid #333',
        borderRadius: '8px',
        overflow: 'hidden',
        display: 'flex',
        flexDirection: 'column' as const,
        minHeight: 0
    },
    scpBody: {
        flex: 1,
        padding: '12px 12px',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '14px',
        overflow: 'auto'
    },
    scpGroup: {
        border: '1px solid #333',
        borderRadius: '8px',
        padding: '12px 12px',
        backgroundColor: '#1e1e1e',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '10px'
    },
    scpLabel: {
        color: '#aaa',
        fontSize: '12px'
    },
    scpHint: {
        color: '#888',
        fontSize: '12px'
    },
    drawer: {
        border: '1px solid #333',
        borderRadius: '8px',
        overflow: 'hidden'
    },
    drawerHeader: {
        padding: '10px 12px',
        backgroundColor: '#252526',
        borderBottom: '1px solid #333',
        display: 'flex',
        alignItems: 'center',
        gap: '10px'
    },
    drawerBody: {
        padding: '10px 12px',
        display: 'flex',
        flexDirection: 'column',
        gap: '6px',
        maxHeight: '220px',
        overflowY: 'auto'
    },
    taskRow: {
        display: 'flex',
        gap: '8px',
        alignItems: 'center'
    },
    taskId: {
        color: '#aaa',
        fontSize: '12px',
        minWidth: '80px'
    },
    taskStatus: {
        color: '#aaa',
        fontSize: '12px',
        minWidth: '70px'
    },
    taskProgress: {
        color: '#aaa',
        fontSize: '12px',
        minWidth: '140px'
    },
    taskSpeed: {
        color: '#aaa',
        fontSize: '12px',
        minWidth: '110px'
    },
    taskMsg: {
        color: '#aaa',
        fontSize: '12px',
        flex: 1
    },
    modalOverlay: {
        position: 'fixed' as const,
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0,0,0,0.65)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 3000
    },
    modal: {
        width: '720px',
        height: '520px',
        backgroundColor: '#252526',
        borderRadius: '8px',
        border: '1px solid #333',
        display: 'flex',
        flexDirection: 'column' as const,
        overflow: 'hidden'
    },
    modalHeader: {
        padding: '10px 12px',
        borderBottom: '1px solid #333',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: '10px',
        backgroundColor: '#1e1e1e'
    },
    modalBody: {
        flex: 1,
        padding: '10px 12px'
    },
    modalFooter: {
        padding: '10px 12px',
        borderTop: '1px solid #333',
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '10px',
        backgroundColor: '#1e1e1e'
    },
    textarea: {
        width: '100%',
        height: '100%',
        padding: '10px 12px',
        borderRadius: '6px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#1e1e1e',
        color: '#fff',
        outline: 'none',
        fontFamily: 'monospace',
        fontSize: '12px',
        resize: 'none' as const
    }
};

export default FilesPanel;
