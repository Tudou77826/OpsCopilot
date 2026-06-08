import React, { useEffect, useMemo, useRef, useState } from 'react';
import { FiArchive, FiCode, FiFile, FiFileText, FiFolder } from 'react-icons/fi';
import { confirmDialog } from '../ConfirmDialog/ConfirmDialog';

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
    owner?: string;
    group?: string;
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
    backend?: FileTransferBackend;
}

interface FileTransferBackend {
    FTCheck: (sessionId: string) => Promise<string>;
    FTList: (sessionId: string, remotePath: string) => Promise<string>;
    FTStat: (sessionId: string, remotePath: string) => Promise<string>;
    FTUpload: (sessionId: string, localPath: string, remotePath: string) => Promise<string>;
    FTDownload: (sessionId: string, remotePath: string, localPath: string) => Promise<string>;
    FTCancel: (taskId: string) => Promise<string>;
    FTRemoteMkdir: (sessionId: string, remotePath: string) => Promise<string>;
    FTRemoteRemove: (sessionId: string, remotePath: string) => Promise<string>;
    FTRemoteRename: (sessionId: string, oldPath: string, newPath: string) => Promise<string>;
    FTRemoteReadFile: (sessionId: string, remotePath: string, maxBytes: number) => Promise<string>;
    FTRemoteWriteFile: (sessionId: string, remotePath: string, content: string) => Promise<string>;
    LocalList: (localPath: string) => Promise<string>;
    LocalMkdir: (localPath: string) => Promise<string>;
    LocalRemove: (localPath: string) => Promise<string>;
    LocalRename: (oldPath: string, newPath: string) => Promise<string>;
}

const appBackend = window.go?.main?.App as any;
const defaultBackend: FileTransferBackend = {
    FTCheck: (sessionId: string) => appBackend.FTCheck(sessionId),
    FTList: (sessionId: string, remotePath: string) => appBackend.FTList(sessionId, remotePath),
    FTStat: (sessionId: string, remotePath: string) => appBackend.FTStat(sessionId, remotePath),
    FTUpload: (sessionId: string, localPath: string, remotePath: string) => appBackend.FTUpload(sessionId, localPath, remotePath),
    FTDownload: (sessionId: string, remotePath: string, localPath: string) => appBackend.FTDownload(sessionId, remotePath, localPath),
    FTCancel: (taskId: string) => appBackend.FTCancel(taskId),
    FTRemoteMkdir: (sessionId: string, remotePath: string) => appBackend.FTRemoteMkdir(sessionId, remotePath),
    FTRemoteRemove: (sessionId: string, remotePath: string) => appBackend.FTRemoteRemove(sessionId, remotePath),
    FTRemoteRename: (sessionId: string, oldPath: string, newPath: string) => appBackend.FTRemoteRename(sessionId, oldPath, newPath),
    FTRemoteReadFile: (sessionId: string, remotePath: string, maxBytes: number) => appBackend.FTRemoteReadFile(sessionId, remotePath, maxBytes),
    FTRemoteWriteFile: (sessionId: string, remotePath: string, content: string) => appBackend.FTRemoteWriteFile(sessionId, remotePath, content),
    LocalList: (localPath: string) => appBackend.LocalList(localPath),
    LocalMkdir: (localPath: string) => appBackend.LocalMkdir(localPath),
    LocalRemove: (localPath: string) => appBackend.LocalRemove(localPath),
    LocalRename: (oldPath: string, newPath: string) => appBackend.LocalRename(oldPath, newPath),
};

type TaskState = {
    taskId: string;
    sessionId: string;
    bytesDone: number;
    bytesTotal: number;
    speedBps: number;
    status: 'running' | 'done' | 'error' | 'cancelled';
    message?: string;
    step?: string;
};

type LayoutMode = 'wide' | 'medium' | 'narrow';
type NarrowPane = 'local' | 'remote' | 'queue';
type DropOverlayState = {
    visible: boolean;
    blocked: boolean;
    title: string;
    detail: string;
};

const NARROW_LAYOUT_WIDTH = 640;
const WIDE_LAYOUT_WIDTH = 920;
const LAYOUT_HYSTERESIS = 16;

export const getFileTransferLayoutMode = (width: number): LayoutMode => {
    if (width < NARROW_LAYOUT_WIDTH) return 'narrow';
    if (width < WIDE_LAYOUT_WIDTH) return 'medium';
    return 'wide';
};

export const getStableFileTransferLayoutMode = (width: number, currentMode: LayoutMode): LayoutMode => {
    if (currentMode === 'narrow') {
        if (width < NARROW_LAYOUT_WIDTH + LAYOUT_HYSTERESIS) return 'narrow';
        return getFileTransferLayoutMode(width);
    }
    if (currentMode === 'wide') {
        if (width >= WIDE_LAYOUT_WIDTH - LAYOUT_HYSTERESIS) return 'wide';
        return getFileTransferLayoutMode(width);
    }

    if (width < NARROW_LAYOUT_WIDTH - LAYOUT_HYSTERESIS) return 'narrow';
    if (width >= WIDE_LAYOUT_WIDTH + LAYOUT_HYSTERESIS) return 'wide';
    return 'medium';
};

type FilePaneProps = {
    title: string;
    badge?: string;
    owner: string;
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
    layoutMode: LayoutMode;
    draggableEntries?: boolean;
    onEntryDragStart?: (entry: FileEntry, event: React.DragEvent) => void;
    dropOverlay?: DropOverlayState | null;
    onPaneDragOver?: (event: React.DragEvent<HTMLDivElement>) => void;
    onPaneDragLeave?: (event: React.DragEvent<HTMLDivElement>) => void;
    onPaneDrop?: (event: React.DragEvent<HTMLDivElement>) => void;
};

type FileColumnKey = 'name' | 'owner' | 'size' | 'time';

const formatFileSize = (size: number) => {
    if (!Number.isFinite(size) || size < 0) return '-';
    if (size < 1024) return `${size} B`;
    const units = ['KB', 'MB', 'GB', 'TB'];
    let value = size / 1024;
    let unit = units[0];
    for (let i = 1; i < units.length && value >= 1024; i += 1) {
        value /= 1024;
        unit = units[i];
    }
    return `${value >= 10 ? value.toFixed(0) : value.toFixed(1)} ${unit}`;
};

const getFileKind = (entry: FileEntry) => {
    if (entry.isDir) return '目录';
    const ext = entry.name.toLowerCase().split('.').pop() || '';
    if (['log', 'txt', 'md', 'conf', 'ini', 'yaml', 'yml'].includes(ext)) return '文本文件';
    if (['sh', 'bash', 'ps1', 'bat', 'go', 'ts', 'tsx', 'js', 'jsx', 'json', 'py'].includes(ext)) return '脚本文件';
    if (['zip', 'tar', 'gz', 'tgz', 'rar', '7z'].includes(ext)) return '压缩包';
    return '文件';
};

const getEntryOwnerLabel = (entry: FileEntry, fallback: string) => {
    if (entry.owner && entry.group) return `${entry.owner}:${entry.group}`;
    if (entry.owner) return entry.owner;
    if (entry.group) return entry.group;
    return fallback || '-';
};

const FileKindIcon = ({ entry }: { entry: FileEntry }) => {
    const kind = getFileKind(entry);
    const style = entry.isDir
        ? styles.fileIconFolder
        : kind === '脚本文件'
            ? styles.fileIconCode
            : kind === '压缩包'
                ? styles.fileIconArchive
                : styles.fileIconFile;
    const Icon = entry.isDir
        ? FiFolder
        : kind === '脚本文件'
            ? FiCode
            : kind === '压缩包'
                ? FiArchive
                : kind === '文本文件'
                    ? FiFileText
                    : FiFile;

    return (
        <span style={{ ...styles.fileIcon, ...style }} title={kind} aria-label={kind} data-testid={entry.isDir ? 'file-kind-directory' : 'file-kind-file'}>
            {React.createElement(Icon as React.ComponentType<{ size?: number; 'aria-hidden'?: string }>, { size: 15, 'aria-hidden': 'true' })}
        </span>
    );
};

function FilePane({
    title,
    badge,
    owner,
    path,
    pathInput,
    onPathInputChange,
    onGo,
    onUp,
    onRefresh,
    entries,
    selected,
    onSelect,
    onOpenDir,
    onOpenFile,
    disabled,
    toolbar,
    layoutMode,
    draggableEntries,
    onEntryDragStart,
    dropOverlay,
    onPaneDragOver,
    onPaneDragLeave,
    onPaneDrop,
}: FilePaneProps) {
    const [columnWidths, setColumnWidths] = useState<Record<FileColumnKey, number>>({
        name: 280,
        owner: 64,
        size: 92,
        time: 152,
    });
    const resizeRef = useRef<{ key: FileColumnKey; startX: number; startWidth: number; previousCursor: string } | null>(null);

    useEffect(() => {
        const onMove = (event: MouseEvent) => {
            const state = resizeRef.current;
            if (!state) return;
            const minWidth: Record<FileColumnKey, number> = {
                name: 160,
                owner: 48,
                size: 72,
                time: 116,
            };
            const nextWidth = Math.max(minWidth[state.key], state.startWidth + event.clientX - state.startX);
            setColumnWidths(prev => ({ ...prev, [state.key]: nextWidth }));
        };
        const onUp = () => {
            if (resizeRef.current) {
                document.body.style.cursor = resizeRef.current.previousCursor;
                resizeRef.current = null;
            }
        };
        window.addEventListener('mousemove', onMove);
        window.addEventListener('mouseup', onUp);
        return () => {
            window.removeEventListener('mousemove', onMove);
            window.removeEventListener('mouseup', onUp);
        };
    }, []);

    const visibleColumns: FileColumnKey[] = layoutMode === 'wide'
        ? ['name', 'owner', 'size', 'time']
        : layoutMode === 'medium'
            ? ['name', 'size', 'time']
            : ['name'];
    const isColumnVisible = (key: FileColumnKey) => visibleColumns.includes(key);

    const renderHeaderCell = (label: string, key: FileColumnKey, extraStyle?: React.CSSProperties) => (
        <th style={{ ...styles.th, ...extraStyle }}>
            <span style={styles.thLabel}>{label}</span>
            {layoutMode !== 'narrow' ? (
                <span
                    style={styles.colResizeHandle}
                    data-testid={`column-resize-${key}`}
                    onMouseDown={(event) => {
                        event.preventDefault();
                        resizeRef.current = { key, startX: event.clientX, startWidth: columnWidths[key], previousCursor: document.body.style.cursor };
                        document.body.style.cursor = 'col-resize';
                    }}
                />
            ) : null}
        </th>
    );

    const openEntry = (entry: FileEntry) => {
        if (disabled) return;
        if (entry.isDir) {
            onOpenDir(entry.path);
        } else if (onOpenFile) {
            onOpenFile(entry.path);
        }
    };

    return (
        <div
            style={{ ...styles.pane, opacity: disabled ? 0.6 : 1 }}
            data-testid={`file-pane-${title}`}
            data-layout-mode={layoutMode}
            onDragOver={onPaneDragOver}
            onDragLeave={onPaneDragLeave}
            onDrop={onPaneDrop}
        >
            <div style={styles.paneHeader}>
                <div style={styles.paneTitleGroup}>
                    <div style={styles.paneTitle}>{title}</div>
                    {badge ? <div style={styles.badge}>{badge}</div> : null}
                </div>
                <div style={styles.paneToolbar}>
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
                {dropOverlay?.visible ? (
                    <div
                        style={{
                            ...styles.dropOverlay,
                            ...(dropOverlay.blocked ? styles.dropOverlayBlocked : styles.dropOverlayReady),
                        }}
                        data-testid="file-drop-overlay"
                    >
                        <div style={styles.dropOverlayTitle}>{dropOverlay.title}</div>
                        <div style={styles.dropOverlayDetail}>{dropOverlay.detail}</div>
                    </div>
                ) : null}
                {layoutMode === 'narrow' ? (
                    <div style={styles.compactList} data-testid={`file-list-${title}`}>
                        {entries.map((e, index) => {
                            const size = e.isDir ? '-' : formatFileSize(e.size);
                            const time = e.modTime ? new Date(e.modTime).toLocaleString() : '';
                            const entryKey = e.path || `${title}:${e.name}:${index}`;
                            const ownerText = getEntryOwnerLabel(e, owner);
                            return (
                                <div
                                    key={entryKey}
                                    style={{
                                        ...styles.compactRow,
                                        ...(selected && selected === e.path ? styles.compactRowSelected : null),
                                        cursor: disabled ? 'not-allowed' : 'pointer',
                                    }}
                                    onClick={() => !disabled && onSelect(e.path)}
                                    onDoubleClick={() => openEntry(e)}
                                    draggable={!!draggableEntries && !e.isDir && !disabled}
                                    onDragStart={(event) => onEntryDragStart?.(e, event)}
                                    title={e.name}
                                >
                                    <div style={styles.compactMain}>
                                        <FileKindIcon entry={e} />
                                        <span style={styles.compactName}>{e.name}</span>
                                    </div>
                                    <div style={styles.compactMeta}>{ownerText} · {size}{time ? ` · ${time}` : ''}</div>
                                </div>
                            );
                        })}
                        {entries.length === 0 ? <div style={styles.emptyState}>暂无数据</div> : null}
                    </div>
                ) : (
                <div style={styles.fileTableWrap}>
                    <table style={styles.table}>
                        <colgroup>
                            {isColumnVisible('name') ? <col style={{ width: columnWidths.name }} /> : null}
                            {isColumnVisible('owner') ? <col style={{ width: columnWidths.owner }} /> : null}
                            {isColumnVisible('size') ? <col style={{ width: columnWidths.size }} /> : null}
                            {isColumnVisible('time') ? <col style={{ width: columnWidths.time }} /> : null}
                        </colgroup>
                        <thead>
                            <tr>
                                {isColumnVisible('name') ? renderHeaderCell('名称', 'name') : null}
                                {isColumnVisible('owner') ? renderHeaderCell('所属', 'owner', styles.cellOwner) : null}
                                {isColumnVisible('size') ? renderHeaderCell('大小', 'size', styles.cellSize) : null}
                                {isColumnVisible('time') ? renderHeaderCell('更新时间', 'time', styles.cellTime) : null}
                            </tr>
                        </thead>
                        <tbody>
                            {entries.map((e, index) => (
                                <tr
                                    key={e.path || `${title}:${e.name}:${index}`}
                                    style={{
                                        ...styles.fileRow,
                                        ...(selected && selected === e.path ? styles.fileRowSelected : null),
                                        cursor: disabled ? 'not-allowed' : 'pointer',
                                    }}
                                    onClick={() => !disabled && onSelect(e.path)}
                                    onDoubleClick={() => {
                                        openEntry(e);
                                    }}
                                    draggable={!!draggableEntries && !e.isDir && !disabled}
                                    onDragStart={(event) => onEntryDragStart?.(e, event)}
                                >
                                    <td style={{ ...styles.td, ...styles.cellName }} title={e.name}>
                                        <FileKindIcon entry={e} />
                                        <span>{e.name}</span>
                                    </td>
                                    {isColumnVisible('owner') ? <td style={{ ...styles.td, ...styles.cellOwner }}>{getEntryOwnerLabel(e, owner)}</td> : null}
                                    {isColumnVisible('size') ? <td style={{ ...styles.td, ...styles.cellSize }}>{e.isDir ? '-' : formatFileSize(e.size)}</td> : null}
                                    {isColumnVisible('time') ? <td style={{ ...styles.td, ...styles.cellTime }}>{e.modTime ? new Date(e.modTime).toLocaleString() : ''}</td> : null}
                                </tr>
                            ))}
                            {entries.length === 0 ? (
                                <tr>
                                    <td style={styles.td} colSpan={visibleColumns.length}>暂无数据</td>
                                </tr>
                            ) : null}
                        </tbody>
                    </table>
                </div>
                )}
            </div>
        </div>
    );
}

const FilesPanel: React.FC<FilesPanelProps> = ({ activeTerminalId, terminals, backend }) => {
    const api = backend || defaultBackend;
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
    const containerRef = useRef<HTMLDivElement | null>(null);
    const sessionIdRef = useRef<string>(sessionId);
    const protocolRef = useRef<string>(protocol);
    const localPathRef = useRef<string>(localPath);
    const remotePathRef = useRef<string>(remotePath);
    const localEntriesRef = useRef<FileEntry[]>(localEntries);
    const remoteEntriesRef = useRef<FileEntry[]>(remoteEntries);
    const [layoutMode, setLayoutMode] = useState<LayoutMode>('wide');
    const [narrowPane, setNarrowPane] = useState<NarrowPane>('local');
    const [remoteDropOverlay, setRemoteDropOverlay] = useState<DropOverlayState | null>(null);

    useEffect(() => {
        const getObservedWidth = (entry?: ResizeObserverEntry) => {
            const borderBoxSize = entry?.borderBoxSize;
            const borderBox = Array.isArray(borderBoxSize) ? borderBoxSize[0] : borderBoxSize;
            if (borderBox?.inlineSize) return Math.round(borderBox.inlineSize);
            return Math.round(containerRef.current?.offsetWidth || containerRef.current?.clientWidth || window.innerWidth);
        };

        const updateLayoutMode = (width = getObservedWidth()) => {
            setLayoutMode(prev => {
                const next = getStableFileTransferLayoutMode(width, prev);
                return next === prev ? prev : next;
            });
        };

        updateLayoutMode();

        if (typeof ResizeObserver !== 'undefined' && containerRef.current) {
            const observer = new ResizeObserver((entries) => {
                updateLayoutMode(getObservedWidth(entries[0]));
            });
            observer.observe(containerRef.current);
            return () => observer.disconnect();
        }

        const onWindowResize = () => updateLayoutMode();
        window.addEventListener('resize', onWindowResize);
        return () => window.removeEventListener('resize', onWindowResize);
    }, []);

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
        if (resp.error) {
            const code = resp.error.code;
            if (code === 'FILE_SIZE_EXCEEDED') return '文件过大，Base64 直传模式最大支持 300 KB';
            if (code === 'CHECKSUM_MISMATCH') return '文件校验失败，传输数据可能不完整。请重试';
            return `${resp.error.message} (${code})`;
        }
        return resp.message || '失败';
    };

    const isSFTPSupported = () => {
        // root-relay has full file management capabilities via su + shell commands
        return protocol.startsWith('sftp') || protocol.includes('root-relay');
    };

    const isTransferSupported = () => {
        return protocol.startsWith('sftp') || protocol.startsWith('scp') || protocol.includes('root-relay');
    };

    const isSCPMode = () => {
        return protocol.startsWith('scp') && !protocol.includes('root-relay');
    };

    const isRootRelay = () => {
        return protocol.includes('root-relay');
    };

    const getProtocolLabel = (p: string): string => {
        const map: Record<string, string> = {
            'sftp(login)': 'SFTP（密码登录）',
            'sftp(key)': 'SFTP（密钥登录）',
            'sftp(root)': 'SFTP（Root 直连）',
            'sftp(root-relay)': 'SFTP（Root 中转模式）',
            'scp(root-relay)': 'SCP 中转（Root 中转模式）',
            'su-relay(root-relay)': 'Base64 直传（Root 中转模式）',
            'scp(login)': 'SCP（兼容模式）',
            'scp(fallback)': 'SCP（兼容模式）',
            'scp(root)': 'SCP（Root 兼容模式）',
        };
        return map[p] || (p ? p : '连接方式未探测');
    };

    const getWorkModeLabel = (p: string): string => {
        if (p === 'su-relay(root-relay)') return 'Base64 直传';
        if (p.includes('root-relay')) return 'Root 中转';
        if (p.startsWith('sftp') || p.startsWith('scp')) return '常规直连';
        return '—';
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
        if (!targetSessionId) return '';
        try {
            const raw = await api.FTCheck(targetSessionId);
            const resp = parseResp(raw);
            if (!resp) {
                protocolRef.current = '';
                setProtocol('');
                return '';
            }
            if (!resp.ok) {
                protocolRef.current = '';
                setProtocol('');
                setMsg(formatError(resp));
                return '';
            }
            const nextProtocol = resp.message || '';
            protocolRef.current = nextProtocol;
            setProtocol(nextProtocol);
            return nextProtocol;
        } catch {
            protocolRef.current = '';
            setProtocol('');
            return '';
        }
    };

    const refreshLocal = async (path: string) => {
        setLoading(true);
        setMsg('');
        try {
            const raw = await api.LocalList(path);
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
        if (!protocolRef.current.startsWith('sftp') && !protocolRef.current.includes('root-relay')) {
            setRemoteEntries([]);
            return;
        }
        setLoading(true);
        setMsg('');
        try {
            const raw = await api.FTList(targetSessionId, path);
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
        if (!protocolRef.current.startsWith('sftp') && !protocolRef.current.includes('root-relay')) return;
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
                        status: 'running' as const,
                        step: undefined as string | undefined
                    };
                    return {
                        ...prev,
                        [tid]: {
                            ...cur,
                            sessionId: data?.sessionId || cur.sessionId,
                            bytesDone: Number(data?.bytesDone ?? cur.bytesDone),
                            bytesTotal: Number(data?.bytesTotal ?? cur.bytesTotal),
                            speedBps: Number(data?.speedBps ?? cur.speedBps),
                            status: 'running',
                            step: (data?.step as string) || cur.step
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
                        status: 'running' as const,
                        step: undefined as string | undefined
                    };
                    const ok = !!data?.ok;
                    const status = ok ? 'done' : (data?.message?.includes('取消') ? 'cancelled' : 'error');
                    return {
                        ...prev,
                        [tid]: {
                            ...cur,
                            status,
                            message: data?.message || (ok ? '完成' : '失败'),
                            step: undefined
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
            await api.FTCancel(taskId);
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

        if (protocolRef.current.startsWith('sftp') || protocolRef.current.includes('root-relay')) {
            try {
                const raw = await api.FTStat(sessionIdRef.current, dst);
                const resp = parseResp(raw);
                if (resp && resp.ok) {
                    const ok = await confirmDialog.show({ message: `远端已存在同名文件：\n${dst}\n\n是否覆盖？`, confirmText: '覆盖', danger: true });
                    if (!ok) return;
                }
            } catch {
            }
        } else if (protocolRef.current.startsWith('scp')) {
            const ok = await confirmDialog.show({ message: `SCP 模式无法检测远端是否存在同名文件：\n${dst}\n\n是否继续上传（可能覆盖）？`, confirmText: '继续上传', danger: true });
            if (!ok) return;
        }

        setLoading(true);
        setMsg('');
        try {
            const raw = await api.FTUpload(sessionId, entry.path, dst);
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
                // Detect IPC delegated task: done events won't reach this frontend
                const isDelegated = !!(resp as any).message?.includes?.('主程序');
                setTasks(prev => ({
                    ...prev,
                    [resp.taskId as string]: {
                        taskId: resp.taskId as string,
                        sessionId,
                        bytesDone: 0,
                        bytesTotal: -1,
                        speedBps: 0,
                        status: isDelegated ? 'done' : 'running',
                        message: isDelegated ? ((resp as any).message || '任务已提交到主程序执行') : undefined
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

    const handleLocalEntryDragStart = (entry: FileEntry, event: React.DragEvent) => {
        if (entry.isDir) return;
        event.dataTransfer.effectAllowed = 'copy';
        event.dataTransfer.setData('application/x-opscopilot-local-file', JSON.stringify(entry));
        event.dataTransfer.setData('text/plain', entry.path);
    };

    const getRemoteDropOverlay = (event: React.DragEvent): DropOverlayState => {
        const hasLocalEntry = event.dataTransfer.types.includes('application/x-opscopilot-local-file');
        const hasExternalFile = event.dataTransfer.types.includes('Files');
        if (!hasLocalEntry && !hasExternalFile) {
            return {
                visible: true,
                blocked: true,
                title: '无法识别拖入内容',
                detail: '请从左侧本地文件列表拖入单个文件',
            };
        }
        if (!sessionId) {
            return {
                visible: true,
                blocked: true,
                title: '无法上传',
                detail: '请先选择一个已连接的会话',
            };
        }
        if (!isTransferSupported()) {
            return {
                visible: true,
                blocked: true,
                title: '当前连接不支持文件上传',
                detail: getProtocolLabel(protocol),
            };
        }
        return {
            visible: true,
            blocked: false,
            title: '上传到远端目录',
            detail: remotePath || remotePathInput || '/',
        };
    };

    const handleRemoteDragOver = (event: React.DragEvent<HTMLDivElement>) => {
        const overlay = getRemoteDropOverlay(event);
        event.preventDefault();
        event.dataTransfer.dropEffect = overlay.blocked ? 'none' : 'copy';
        setRemoteDropOverlay(overlay);
    };

    const handleRemoteDragLeave = (event: React.DragEvent<HTMLDivElement>) => {
        if (event.currentTarget.contains(event.relatedTarget as Node | null)) return;
        setRemoteDropOverlay(null);
    };

    const extractDroppedLocalEntry = (event: React.DragEvent): FileEntry | null => {
        const payload = event.dataTransfer.getData('application/x-opscopilot-local-file');
        if (payload) {
            try {
                const entry = JSON.parse(payload) as FileEntry;
                return entry.path && entry.name ? entry : null;
            } catch {
                return null;
            }
        }

        const file = event.dataTransfer.files?.[0] as (File & { path?: string; webkitRelativePath?: string }) | undefined;
        const path = file?.path || file?.webkitRelativePath || '';
        if (!file || !path) return null;
        return {
            path,
            name: file.name,
            isDir: false,
            size: file.size,
            modTime: '',
            mode: 0,
        };
    };

    const handleRemoteDrop = async (event: React.DragEvent<HTMLDivElement>) => {
        event.preventDefault();
        const overlay = getRemoteDropOverlay(event);
        setRemoteDropOverlay(null);
        if (overlay.blocked) {
            setMsg(overlay.detail);
            return;
        }

        const entry = extractDroppedLocalEntry(event);
        if (!entry) {
            setMsg('未能读取拖入文件路径，请从左侧本地文件列表拖拽文件上传');
            return;
        }
        await startUploadFile(entry);
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
            const raw = await api.FTDownload(sessionId, entry.path, dst);
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
                const isDelegated = !!(resp as any).message?.includes?.('主程序');
                setTasks(prev => ({
                    ...prev,
                    [resp.taskId as string]: {
                        taskId: resp.taskId as string,
                        sessionId,
                        bytesDone: 0,
                        bytesTotal: -1,
                        speedBps: 0,
                        status: isDelegated ? 'done' : 'running',
                        message: isDelegated ? ((resp as any).message || '任务已提交到主程序执行') : undefined
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
            const raw = await api.FTDownload(sessionId, rp, lp);
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
                const isDelegated = !!(resp as any).message?.includes?.('主程序');
                setTasks(prev => ({
                    ...prev,
                    [resp.taskId as string]: {
                        taskId: resp.taskId as string,
                        sessionId,
                        bytesDone: 0,
                        bytesTotal: -1,
                        speedBps: 0,
                        status: isDelegated ? 'done' : 'running',
                        message: isDelegated ? ((resp as any).message || '任务已提交到主程序执行') : undefined
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
            const raw = await api.FTRemoteMkdir(sessionId, p);
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
        const ok = await confirmDialog.show({ message: '确定要删除所选远端项吗？', danger: true });
        if (!ok) return;
        setLoading(true);
        setMsg('');
        try {
            const raw = await api.FTRemoteRemove(sessionId, remoteSelected);
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
            const raw = await api.FTRemoteRename(sessionId, entry.path, newPath);
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
            const raw = await api.FTRemoteReadFile(sessionId, entry.path, 262144);
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
            const raw = await api.FTRemoteWriteFile(sessionId, editPath, editContent);
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
            const raw = await api.LocalMkdir(p);
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
        const ok = await confirmDialog.show({ message: '确定要删除所选项吗？', danger: true });
        if (!ok) return;
        setLoading(true);
        setMsg('');
        try {
            const raw = await api.LocalRemove(localSelected);
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
            const raw = await api.LocalRename(entry.path, newPath);
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
    const isNarrow = layoutMode === 'narrow';
    const panelRootStyle = isNarrow ? { ...styles.root, ...styles.rootNarrow } : styles.root;
    const splitStyle = layoutMode === 'narrow'
        ? { ...styles.split, ...styles.splitNarrow }
        : layoutMode === 'medium'
            ? { ...styles.split, ...styles.splitMedium }
            : styles.split;
    const showQueue = drawerOpen || (isNarrow && narrowPane === 'queue');

    return (
        <div ref={containerRef} style={panelRootStyle} data-testid="files-panel" data-layout-mode={layoutMode}>
            <div style={styles.topBar}>
                <div style={styles.infoGrid}>
                    <div style={{ ...styles.infoField, ...styles.infoFieldPrimary }}>
                        <span style={styles.infoLabel}>当前会话</span>
                        <select style={styles.select} value={sessionId} onChange={(e) => setSessionId(e.target.value)}>
                            {terminals.map(t => (
                                <option key={t.id} value={t.id}>
                                    {t.title || t.id}
                                </option>
                            ))}
                        </select>
                    </div>
                    <div style={styles.infoChip}>
                        <span style={styles.infoLabel}>连接方式</span>
                        <span style={protocol ? styles.infoValue : styles.infoValueMuted}>
                            {getProtocolLabel(protocol)}
                        </span>
                    </div>
                    <div style={styles.infoChip}>
                        <span style={styles.infoLabel}>工作方式</span>
                        <span style={protocol ? styles.infoValue : styles.infoValueMuted}>
                            {getWorkModeLabel(protocol)}
                        </span>
                    </div>
                </div>
                <div style={{ flex: 1 }} />
                <button style={styles.btnSecondary} onClick={() => {
                    if (isNarrow) {
                        setNarrowPane(v => v === 'queue' ? 'local' : 'queue');
                        return;
                    }
                    setDrawerOpen(v => !v);
                }}>
                    {showQueue ? '隐藏队列' : '显示队列'}
                </button>
            </div>

            {isNarrow ? (
                <div style={styles.segmented} role="tablist" aria-label="文件传输视图">
                    <button style={narrowPane === 'local' ? styles.segmentedActive : styles.segmentedButton} onClick={() => setNarrowPane('local')}>本地</button>
                    <button style={narrowPane === 'remote' ? styles.segmentedActive : styles.segmentedButton} onClick={() => setNarrowPane('remote')}>远端</button>
                    <button style={narrowPane === 'queue' ? styles.segmentedActive : styles.segmentedButton} onClick={() => setNarrowPane('queue')}>队列</button>
                </div>
            ) : null}

            {msg ? <div style={{ color: '#aaa', fontSize: '12px' }}>{msg}</div> : null}

            {isRootRelay() ? (
                <div style={styles.relayBanner}>
                    当前无法 Root 直连，已切换为 Root 中转模式。通过 Base64 直传传输文件，单文件上限 300 KB，传输后自动校验文件完整性。
                </div>
            ) : null}

            {!isSFTPSupported() && protocol.startsWith('scp') && !isRootRelay() ? (
                <div style={{ color: '#aaa', fontSize: '12px' }}>
                    当前为 SCP 降级模式，仅支持上传/下载，不支持远端浏览与管理。
                </div>
            ) : null}

            {(!isNarrow || narrowPane !== 'queue') ? (
            <div style={splitStyle} data-testid="file-transfer-split">
                {(!isNarrow || narrowPane === 'local') ? (
                    <FilePane
                        title="本地"
                        badge={localPath ? localPath : ''}
                        owner="本地"
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
                        layoutMode={layoutMode}
                        draggableEntries
                        onEntryDragStart={handleLocalEntryDragStart}
                        toolbar={
                            <>
                                <button style={styles.btnSecondary} onClick={createLocalFolder} disabled={loading}>新建</button>
                                <button style={styles.btnSecondary} onClick={renameLocalSelected} disabled={loading || !localSelected}>重命名</button>
                                <button style={styles.btnDanger} onClick={deleteLocalSelected} disabled={loading || !localSelected}>删除</button>
                            </>
                        }
                    />
                ) : null}

                {(!isNarrow || narrowPane === 'remote') ? (isSCPMode() ? (
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
                        owner="-"
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
                        layoutMode={layoutMode}
                        dropOverlay={remoteDropOverlay}
                        onPaneDragOver={handleRemoteDragOver}
                        onPaneDragLeave={handleRemoteDragLeave}
                        onPaneDrop={handleRemoteDrop}
                        toolbar={
                            <>
                                <button style={styles.btnSecondary} onClick={createRemoteFolder} disabled={loading || !isSFTPSupported()}>新建</button>
                                <button style={styles.btnSecondary} onClick={renameRemoteSelected} disabled={loading || !remoteSelected || !isSFTPSupported()}>重命名</button>
                                <button style={styles.btnDanger} onClick={deleteRemoteSelected} disabled={loading || !remoteSelected || !isSFTPSupported()}>删除</button>
                                <button style={styles.btnSecondary} onClick={openRemoteEditor} disabled={loading || !remoteSelected || !isSFTPSupported()}>编辑</button>
                            </>
                        }
                    />
                )) : null}
            </div>
            ) : null}

            {showQueue ? (
                <div style={styles.drawer}>
                    <div style={styles.drawerHeader}>
                        <div style={{ color: '#fff', fontSize: '13px', fontWeight: 600 }}>传输队列</div>
                        <div style={{ flex: 1 }} />
                        <button style={styles.btnSecondary} onClick={() => {
                            setDrawerOpen(false);
                            if (isNarrow) setNarrowPane('local');
                        }}>收起</button>
                    </div>
                    <div style={styles.drawerBody}>
                        {taskList.length === 0 ? (
                            <div style={{ color: '#888', fontSize: '12px' }}>暂无任务</div>
                        ) : (
                            taskList.map(t => (
                                <div key={t.taskId} style={styles.taskRow}>
                                    <div style={styles.taskId} title={t.taskId}>{t.taskId.slice(0, 8)}</div>
                                    {t.status === 'done' ? (
                                        <span style={{ color: '#4ade80', fontSize: '11px' }}>✓ 完成</span>
                                    ) : t.status === 'error' || t.status === 'cancelled' ? (
                                        <span style={{ color: '#f87171', fontSize: '11px' }}>✗ {t.status}</span>
                                    ) : (
                                        <span style={styles.taskStatus}>{t.status}</span>
                                    )}
                                    {t.status === 'done' && t.message ? (
                                        <div style={styles.taskMsg}>{t.message}</div>
                                    ) : null}
                                    {t.status === 'running' && t.bytesTotal > 0 ? (
                                        <div style={styles.taskProgress}>{t.bytesDone}/{t.bytesTotal}</div>
                                    ) : null}
                                    {t.status === 'running' && t.speedBps > 0 ? (
                                        <div style={styles.taskSpeed}>{t.speedBps} B/s</div>
                                    ) : null}
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
    root: {
        padding: '10px',
        color: '#ddd',
        display: 'flex',
        flexDirection: 'column',
        gap: '8px',
        height: '100%',
        minHeight: 0,
        overflow: 'hidden',
        backgroundColor: '#15181c'
    },
    rootNarrow: {
        padding: '8px',
        gap: '8px'
    },
    topBar: {
        display: 'flex',
        gap: '8px',
        alignItems: 'center',
        flexWrap: 'wrap' as const,
        flexShrink: 0,
        padding: '7px 8px',
        border: '1px solid #303844',
        borderRadius: '6px',
        backgroundColor: '#1a1e23',
        boxShadow: 'inset 0 1px 0 rgba(255,255,255,0.03)'
    },
    segmented: {
        display: 'flex',
        border: '1px solid #333',
        borderRadius: '6px',
        overflow: 'hidden',
        flexShrink: 0
    },
    segmentedButton: {
        flex: 1,
        padding: '6px 8px',
        border: 'none',
        borderRight: '1px solid #333',
        backgroundColor: '#1e1e1e',
        color: '#aaa',
        cursor: 'pointer',
        fontSize: '12px'
    },
    segmentedActive: {
        flex: 1,
        padding: '6px 8px',
        border: 'none',
        borderRight: '1px solid #333',
        backgroundColor: '#094771',
        color: '#fff',
        cursor: 'pointer',
        fontSize: '12px',
        fontWeight: 600
    },
    select: {
        padding: '5px 8px',
        borderRadius: '4px',
        border: '1px solid #3b4652',
        backgroundColor: '#111419',
        color: '#fff',
        outline: 'none',
        minWidth: '180px',
        maxWidth: '100%',
        height: '28px',
        fontSize: '12px'
    },
    badge: {
        padding: '2px 7px',
        borderRadius: '999px',
        border: '1px solid #35404c',
        backgroundColor: '#161b20',
        color: '#b9c6d3',
        fontSize: '11px',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
        maxWidth: '260px'
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
        padding: '5px 10px',
        borderRadius: '4px',
        border: '1px solid #1f6ea5',
        backgroundColor: '#0b74b8',
        color: '#fff',
        cursor: 'pointer',
        fontSize: '11px',
        minWidth: '58px',
        height: '28px',
        fontWeight: 600
    },
    btnSecondary: {
        padding: '5px 9px',
        borderRadius: '4px',
        border: '1px solid #343d47',
        backgroundColor: '#171c21',
        color: '#cbd5df',
        cursor: 'pointer',
        fontSize: '11px',
        height: '28px'
    },
    btnDanger: {
        padding: '5px 9px',
        borderRadius: '4px',
        border: '1px solid #663337',
        backgroundColor: '#2a171a',
        color: '#f4b8bd',
        cursor: 'pointer',
        fontSize: '11px',
        height: '28px'
    },
    iconBtn: {
        padding: '4px 7px',
        borderRadius: '4px',
        border: '1px solid #343d47',
        backgroundColor: '#151a1f',
        color: '#cbd5df',
        cursor: 'pointer',
        fontSize: '11px',
        minWidth: '30px',
        height: '28px'
    },
    split: {
        flex: 1,
        display: 'flex',
        gap: '10px',
        overflow: 'hidden',
        minHeight: 0
    },
    splitMedium: {
        gap: '6px'
    },
    splitNarrow: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '8px'
    },
    pane: {
        flex: 1,
        border: '1px solid #2f3842',
        borderRadius: '6px',
        overflow: 'hidden',
        display: 'flex',
        flexDirection: 'column',
        minHeight: 0,
        minWidth: 0,
        backgroundColor: '#11161c',
        boxShadow: '0 1px 0 rgba(255,255,255,0.03)'
    },
    paneTitleGroup: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        minWidth: 0,
        overflow: 'hidden'
    },
    paneTitle: {
        color: '#f4f7fb',
        fontSize: '12px',
        fontWeight: 700,
        letterSpacing: 0
    },
    paneToolbar: {
        display: 'flex',
        gap: '5px',
        flexWrap: 'wrap' as const,
        justifyContent: 'flex-end',
        flexShrink: 0
    },
    paneHeader: {
        padding: '7px 8px',
        borderBottom: '1px solid #2d3540',
        backgroundColor: '#1d232a',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: '6px'
    },
    pathBar: {
        padding: '6px 8px',
        borderBottom: '1px solid #2b333d',
        display: 'flex',
        gap: '6px',
        alignItems: 'center',
        backgroundColor: '#161b21',
        flexWrap: 'wrap' as const
    },
    pathInput: {
        flex: 1,
        padding: '4px 8px',
        borderRadius: '4px',
        border: '1px solid #323c47',
        backgroundColor: '#0f1318',
        color: '#f4f7fb',
        outline: 'none',
        fontSize: '11px',
        height: '28px',
        boxSizing: 'border-box' as const
    },
    paneBody: {
        flex: 1,
        display: 'flex',
        minHeight: 0,
        position: 'relative' as const
    },
    fileTableWrap: {
        flex: 1,
        overflow: 'auto',
        backgroundColor: '#11161c'
    },
    table: {
        width: '100%',
        borderCollapse: 'collapse',
        tableLayout: 'fixed',
        minWidth: '598px'
    },
    th: {
        textAlign: 'left',
        fontWeight: 700,
        fontSize: '10px',
        color: '#8794a3',
        padding: '0 8px',
        position: 'sticky' as const,
        top: 0,
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        userSelect: 'none' as const,
        backgroundColor: '#1b222a',
        borderBottom: '1px solid #2d3540',
        boxSizing: 'border-box' as const,
        height: '28px',
        zIndex: 2
    },
    thLabel: {
        display: 'block',
        paddingRight: '14px',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap'
    },
    td: {
        fontSize: '11px',
        color: '#d8dee6',
        padding: '5px 8px',
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        height: '28px',
        borderBottom: '1px solid #202831',
        boxSizing: 'border-box' as const
    },
    fileRow: {
        backgroundColor: 'transparent'
    },
    fileRowSelected: {
        backgroundColor: '#213244',
        boxShadow: 'inset 3px 0 0 #2f9bf4'
    },
    cellName: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        minWidth: 0
    },
    cellOwner: {
        color: '#9fb3c8'
    },
    cellSize: {
        textAlign: 'right' as const,
        color: '#c8d1dc'
    },
    cellTime: {
        color: '#9aa4af'
    },
    colResizeHandle: {
        position: 'absolute' as const,
        top: 0,
        right: 0,
        width: '10px',
        height: '100%',
        cursor: 'col-resize',
        borderRight: '1px solid #344150',
        opacity: 0.75,
        boxSizing: 'border-box' as const
    },
    compactList: {
        flex: 1,
        overflowY: 'auto',
        overflowX: 'hidden',
        minHeight: 0
    },
    compactRow: {
        borderBottom: '1px solid #202831',
        padding: '7px 8px',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '3px',
        backgroundColor: 'transparent'
    },
    compactRowSelected: {
        backgroundColor: '#213244',
        boxShadow: 'inset 3px 0 0 #2f9bf4'
    },
    compactMain: {
        display: 'flex',
        alignItems: 'center',
        gap: '7px',
        minWidth: 0
    },
    compactName: {
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
        minWidth: 0,
        color: '#ddd',
        fontSize: '12px'
    },
    compactMeta: {
        color: '#8f9aaa',
        fontSize: '10px',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
        paddingLeft: '30px'
    },
    emptyState: {
        color: '#888',
        fontSize: '12px',
        padding: '10px 8px'
    },
    fileIcon: {
        flexShrink: 0,
        width: '21px',
        height: '21px',
        borderRadius: '4px',
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        border: '1px solid #313b45',
        backgroundColor: '#171d24'
    },
    fileIconFolder: {
        color: '#f0c04f',
        backgroundColor: '#282315',
        borderColor: '#594820'
    },
    fileIconFile: {
        color: '#a6b3c2'
    },
    fileIconCode: {
        color: '#7dd3fc',
        backgroundColor: '#122633',
        borderColor: '#21475f'
    },
    fileIconArchive: {
        color: '#c4b5fd',
        backgroundColor: '#251f36',
        borderColor: '#40345e'
    },
    dropOverlay: {
        position: 'absolute' as const,
        zIndex: 5,
        inset: '10px',
        borderRadius: '8px',
        display: 'flex',
        flexDirection: 'column' as const,
        alignItems: 'center',
        justifyContent: 'center',
        gap: '8px',
        pointerEvents: 'none' as const,
        backdropFilter: 'blur(2px)',
        textAlign: 'center' as const,
        padding: '18px'
    },
    dropOverlayReady: {
        border: '1px dashed #5fb3ff',
        backgroundColor: 'rgba(17, 48, 73, 0.86)'
    },
    dropOverlayBlocked: {
        border: '1px dashed #ef8c8c',
        backgroundColor: 'rgba(73, 24, 24, 0.88)'
    },
    dropOverlayTitle: {
        color: '#fff',
        fontSize: '14px',
        fontWeight: 700
    },
    dropOverlayDetail: {
        color: '#c8d1dc',
        fontSize: '12px',
        maxWidth: '80%',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap'
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
    taskStep: {
        color: '#58a6ff',
        fontSize: '11px',
        flex: '1 1 80px',
        maxWidth: '200px',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
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
        fontFamily: 'var(--font-mono)',
        fontSize: '12px',
        resize: 'none' as const
    },
    infoGrid: {
        display: 'flex',
        flexWrap: 'wrap' as const,
        gap: '8px',
        alignItems: 'center',
        minWidth: 0
    },
    infoField: {
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
        fontSize: '12px'
    },
    infoFieldPrimary: {
        minWidth: 0
    },
    infoChip: {
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
        padding: '4px 8px',
        border: '1px solid #303844',
        borderRadius: '999px',
        backgroundColor: '#15181c',
        fontSize: '11px',
        maxWidth: '260px'
    },
    infoLabel: {
        color: '#7d8794',
        whiteSpace: 'nowrap' as const
    },
    infoValue: {
        color: '#ddd',
        whiteSpace: 'nowrap' as const,
        overflow: 'hidden',
        textOverflow: 'ellipsis'
    },
    infoValueMuted: {
        color: '#666',
        whiteSpace: 'nowrap' as const
    },
    relayBanner: {
        padding: '8px 10px',
        borderRadius: '6px',
        border: '1px solid #5a4a1a',
        backgroundColor: '#2a2510',
        color: '#d4c87a',
        fontSize: '11px',
        lineHeight: '1.6'
    }
};

export default FilesPanel;
