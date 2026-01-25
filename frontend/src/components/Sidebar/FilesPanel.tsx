import React, { useEffect, useMemo, useState } from 'react';

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

const FilesPanel: React.FC<FilesPanelProps> = ({ activeTerminalId, terminals }) => {
    const defaultSessionId = useMemo(() => {
        return activeTerminalId || (terminals[0]?.id ?? '');
    }, [activeTerminalId, terminals]);

    const [sessionId, setSessionId] = useState(defaultSessionId);
    const [remotePath, setRemotePath] = useState('.');
    const [entries, setEntries] = useState<FileEntry[]>([]);
    const [loading, setLoading] = useState(false);
    const [msg, setMsg] = useState('');

    const [uploadLocalPath, setUploadLocalPath] = useState('');
    const [uploadRemotePath, setUploadRemotePath] = useState('');
    const [downloadRemotePath, setDownloadRemotePath] = useState('');
    const [downloadLocalPath, setDownloadLocalPath] = useState('');

    const [tasks, setTasks] = useState<Record<string, TaskState>>({});

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
            });
        }

        return () => {
            if (offProgress) offProgress();
            if (offDone) offDone();
        };
    }, []);

    const parseResp = (raw: any): FTResponse | null => {
        if (!raw) return null;
        try {
            return JSON.parse(raw) as FTResponse;
        } catch {
            return null;
        }
    };

    const listDir = async () => {
        if (!sessionId) {
            setMsg('请先选择会话');
            return;
        }
        setLoading(true);
        setMsg('');
        try {
            // @ts-ignore
            const raw = await window.go.main.App.FTList(sessionId, remotePath);
            const resp = parseResp(raw);
            if (!resp) {
                setMsg('返回格式错误');
                return;
            }
            if (!resp.ok) {
                if (resp.error) {
                    setMsg(`${resp.error.message} (${resp.error.code})`);
                } else {
                    setMsg(resp.message || '失败');
                }
                return;
            }
            setEntries(resp.entries || []);
        } catch (e: any) {
            setMsg('失败: ' + e.toString());
        } finally {
            setLoading(false);
        }
    };

    const startUpload = async () => {
        if (!sessionId) {
            setMsg('请先选择会话');
            return;
        }
        const lp = uploadLocalPath.trim();
        const rp = uploadRemotePath.trim();
        if (!lp || !rp) {
            setMsg('请填写本地路径与远端路径');
            return;
        }
        setLoading(true);
        setMsg('');
        try {
            // @ts-ignore
            const raw = await window.go.main.App.FTUpload(sessionId, lp, rp);
            const resp = parseResp(raw);
            if (!resp) {
                setMsg('返回格式错误');
                return;
            }
            if (!resp.ok) {
                if (resp.error) {
                    setMsg(`${resp.error.message} (${resp.error.code})`);
                } else {
                    setMsg(resp.message || '失败');
                }
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
            }
            setMsg('已开始上传');
        } catch (e: any) {
            setMsg('失败: ' + e.toString());
        } finally {
            setLoading(false);
        }
    };

    const startDownload = async () => {
        if (!sessionId) {
            setMsg('请先选择会话');
            return;
        }
        const rp = downloadRemotePath.trim();
        const lp = downloadLocalPath.trim();
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
                if (resp.error) {
                    setMsg(`${resp.error.message} (${resp.error.code})`);
                } else {
                    setMsg(resp.message || '失败');
                }
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
            }
            setMsg('已开始下载');
        } catch (e: any) {
            setMsg('失败: ' + e.toString());
        } finally {
            setLoading(false);
        }
    };

    const cancelTask = async (taskId: string) => {
        setLoading(true);
        try {
            // @ts-ignore
            await window.go.main.App.FTCancel(taskId);
        } finally {
            setLoading(false);
        }
    };

    const taskList = Object.values(tasks)
        .filter(t => !sessionId || t.sessionId === sessionId)
        .slice()
        .sort((a, b) => a.taskId.localeCompare(b.taskId));

    return (
        <div style={{ padding: '12px', color: '#ddd', display: 'flex', flexDirection: 'column', gap: '12px', height: '100%' }}>
            <div style={{ display: 'flex', gap: '8px', alignItems: 'center', flexWrap: 'wrap' as const }}>
                <div style={{ color: '#aaa' }}>会话</div>
                <select
                    style={styles.select}
                    value={sessionId}
                    onChange={(e) => setSessionId(e.target.value)}
                >
                    {terminals.map(t => (
                        <option key={t.id} value={t.id}>
                            {t.title || t.id}
                        </option>
                    ))}
                </select>
            </div>

            <div style={{ display: 'flex', gap: '8px', alignItems: 'center', flexWrap: 'wrap' as const }}>
                <div style={{ color: '#aaa' }}>远端目录</div>
                <input
                    style={styles.input}
                    value={remotePath}
                    onChange={(e) => setRemotePath(e.target.value)}
                    placeholder="例如: /var/log"
                />
                <button style={styles.btn} onClick={listDir} disabled={loading}>
                    {loading ? '处理中...' : '列出'}
                </button>
            </div>

            {msg ? <div style={{ color: '#aaa', fontSize: '12px' }}>{msg}</div> : null}

            <div style={{ flex: 1, overflow: 'auto', border: '1px solid #333', borderRadius: '6px' }}>
                {entries.length === 0 ? (
                    <div style={{ padding: '10px', color: '#888' }}>暂无数据</div>
                ) : (
                    <table style={{ width: '100%', borderCollapse: 'collapse' }}>
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
                                <tr key={e.path} style={{ borderTop: '1px solid #333' }}>
                                    <td style={styles.td}>{e.name}</td>
                                    <td style={styles.td}>{e.isDir ? '目录' : '文件'}</td>
                                    <td style={styles.td}>{e.isDir ? '-' : e.size}</td>
                                    <td style={styles.td}>{e.modTime ? new Date(e.modTime).toLocaleString() : ''}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                )}
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                <div style={{ color: '#fff', fontSize: '13px' }}>上传</div>
                <div style={{ display: 'flex', gap: '8px', alignItems: 'center', flexWrap: 'wrap' as const }}>
                    <input
                        style={styles.input}
                        value={uploadLocalPath}
                        onChange={(e) => setUploadLocalPath(e.target.value)}
                        placeholder="本地文件路径（例如: C:\\\\tmp\\\\a.txt）"
                    />
                    <input
                        style={styles.input}
                        value={uploadRemotePath}
                        onChange={(e) => setUploadRemotePath(e.target.value)}
                        placeholder="远端目标路径（例如: /tmp/a.txt）"
                    />
                    <button style={styles.btn} onClick={startUpload} disabled={loading}>
                        开始上传
                    </button>
                </div>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                <div style={{ color: '#fff', fontSize: '13px' }}>下载</div>
                <div style={{ display: 'flex', gap: '8px', alignItems: 'center', flexWrap: 'wrap' as const }}>
                    <input
                        style={styles.input}
                        value={downloadRemotePath}
                        onChange={(e) => setDownloadRemotePath(e.target.value)}
                        placeholder="远端文件路径（例如: /tmp/a.txt）"
                    />
                    <input
                        style={styles.input}
                        value={downloadLocalPath}
                        onChange={(e) => setDownloadLocalPath(e.target.value)}
                        placeholder="本地保存路径（例如: C:\\\\tmp\\\\a.txt）"
                    />
                    <button style={styles.btn} onClick={startDownload} disabled={loading}>
                        开始下载
                    </button>
                </div>
            </div>

            {taskList.length > 0 ? (
                <div style={{ borderTop: '1px solid #333', paddingTop: '10px', display: 'flex', flexDirection: 'column', gap: '6px' }}>
                    <div style={{ color: '#fff', fontSize: '13px' }}>任务</div>
                    {taskList.map(t => (
                        <div key={t.taskId} style={{ display: 'flex', gap: '8px', alignItems: 'center', flexWrap: 'wrap' as const }}>
                            <div style={{ color: '#aaa', fontSize: '12px', minWidth: '160px' }}>{t.taskId.slice(0, 8)}</div>
                            <div style={{ color: '#aaa', fontSize: '12px', minWidth: '80px' }}>{t.status}</div>
                            <div style={{ color: '#aaa', fontSize: '12px', minWidth: '120px' }}>
                                {t.bytesTotal > 0 ? `${t.bytesDone}/${t.bytesTotal}` : `${t.bytesDone}`}
                            </div>
                            <div style={{ color: '#aaa', fontSize: '12px', minWidth: '100px' }}>{t.speedBps > 0 ? `${t.speedBps} B/s` : ''}</div>
                            <div style={{ color: '#aaa', fontSize: '12px', flex: 1 }}>{t.message || ''}</div>
                            {t.status === 'running' ? (
                                <button style={styles.btn} onClick={() => cancelTask(t.taskId)} disabled={loading}>
                                    取消
                                </button>
                            ) : null}
                        </div>
                    ))}
                </div>
            ) : null}
        </div>
    );
};

const styles: Record<string, React.CSSProperties> = {
    input: {
        padding: '6px 8px',
        borderRadius: '4px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#1e1e1e',
        color: '#fff',
        outline: 'none',
        minWidth: '220px'
    },
    select: {
        padding: '6px 8px',
        borderRadius: '4px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#1e1e1e',
        color: '#fff',
        outline: 'none',
        minWidth: '220px'
    },
    btn: {
        padding: '6px 10px',
        borderRadius: '4px',
        border: 'none',
        backgroundColor: '#007acc',
        color: '#fff',
        cursor: 'pointer'
    },
    th: {
        textAlign: 'left',
        fontWeight: 600,
        fontSize: '12px',
        color: '#bbb',
        padding: '8px 10px'
    },
    td: {
        fontSize: '12px',
        color: '#ddd',
        padding: '8px 10px'
    }
};

export default FilesPanel;

