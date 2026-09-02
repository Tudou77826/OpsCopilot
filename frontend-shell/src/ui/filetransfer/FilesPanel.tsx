import React, { useEffect, useMemo, useRef, useState } from 'react';
import { FiArchive, FiCode, FiFile, FiFileText, FiFolder } from 'react-icons/fi';
import { confirmDialog } from '../feedback/ConfirmDialog';
import type { ConfirmChoice } from '../feedback/ConfirmDialog';
import FileContextMenu, { ContextMenuItem } from './FileContextMenu';
import NameDialog from './NameDialog';

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
    content?: string;
}

/** 传输进度事件载荷（宿主推送）。step 为空串表示清除排队提示。 */
export interface FileTransferProgress {
    taskId: string;
    sessionId?: string;
    bytesDone?: number;
    bytesTotal?: number;
    speedBps?: number;
    step?: string;
}

/** 传输完成事件载荷（宿主推送）。 */
export interface FileTransferDone {
    taskId: string;
    sessionId?: string;
    ok: boolean;
    cancelled?: boolean;
    message?: string;
    bytes?: number;
}

/** OS 级文件拖放（Wails 专有，可选）：给出屏幕坐标与真实本地路径。 */
export type FileDropHandler = (x: number, y: number, paths: string[]) => void;

/**
 * 文件传输宿主端口：Wails 由 window.go/window.runtime 适配，
 * Sidecar 由 shell.ft.* / shell.fs.* RPC 适配。
 * 所有方法返回 JSON 信封字符串（FTResponse/localFSResponse），
 * 与 Wails 侧历史线格式一致；宿主缺失的可选能力直接不实现，
 * 组件据此隐藏对应入口（保存另存为、OS 拖放等）。
 */
export interface FileTransferHost {
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
    LocalStat: (localPath: string) => Promise<string>;
    /** OS 拖放进本地面板的落地拷贝（无 OS 拖放能力的宿主可不实现）。 */
    LocalCopy?: (src: string, dst: string) => Promise<string>;
    /** 保存路径选择对话框（无原生对话框的宿主可不实现；缺失时隐藏"另存为"选项）。 */
    SelectSavePath?: (defaultName: string) => Promise<string>;
    /** 传输进度事件订阅；返回取消函数。 */
    onProgress: (handler: (p: FileTransferProgress) => void) => () => void;
    /** 传输完成事件订阅；返回取消函数。 */
    onDone: (handler: (d: FileTransferDone) => void) => () => void;
    /** OS 文件拖放订阅（Wails 专有）；返回取消函数。缺失表示宿主读不到 OS 路径。 */
    onFileDrop?: (handler: FileDropHandler) => () => void;
}

interface FilesPanelProps {
    activeTerminalId: string | null;
    terminals: TerminalSessionLite[];
    host: FileTransferHost;
}

type TaskState = {
    taskId: string;
    sessionId: string;
    name?: string; // 传输文件名，批量场景下替代无意义的 taskId 前缀
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
// 传输队列面板：默认/最小内容高度与拖拽调整的持久化键。
const QUEUE_DEFAULT_HEIGHT = 220;
const QUEUE_MIN_HEIGHT = 120;
const QUEUE_HEIGHT_STORAGE_KEY = 'opscopilot-ft-queue-height';

// 各列最小宽度（用于列宽自适应与拖拽下限，三处共用）
const COLUMN_MIN_WIDTH: Record<FileColumnKey, number> = {
    name: 160,
    owner: 48,
    size: 72,
    time: 116,
};

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
    owner: string;
    path: string;
    pathInput: string;
    onPathInputChange: (p: string) => void;
    onGo: () => void;
    onUp: () => void;
    onRefresh: () => void;
    entries: FileEntry[];
    selectedSet: Set<string>;
    onSelect: (p: string, event: React.MouseEvent) => void;
    onOpenDir: (p: string) => void;
    onOpenFile?: (p: string) => void;
    disabled?: boolean;
    layoutMode: LayoutMode;
    draggableEntries?: boolean;
    onEntryDragStart?: (entry: FileEntry, event: React.DragEvent) => void;
    dropOverlay?: DropOverlayState | null;
    onPaneDragOver?: (event: React.DragEvent<HTMLDivElement>) => void;
    onPaneDragLeave?: (event: React.DragEvent<HTMLDivElement>) => void;
    onPaneDrop?: (event: React.DragEvent<HTMLDivElement>) => void;
    paneRef?: React.Ref<HTMLDivElement>;
    onRowContextMenu?: (entry: FileEntry, event: React.MouseEvent) => void;
    onBlankContextMenu?: (event: React.MouseEvent) => void;
    onNavigate?: (p: string) => void;
    onToggleCheck?: (p: string) => void;
    onToggleCheckAll?: (paths: string[], checked: boolean) => void;
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

const formatSpeed = (bps: number) => {
    if (!Number.isFinite(bps) || bps < 0) return '-';
    if (bps < 1024) return `${bps} B/s`;
    const units = ['KB/s', 'MB/s', 'GB/s'];
    let value = bps / 1024;
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
    owner,
    path,
    pathInput,
    onPathInputChange,
    onGo,
    onUp,
    onRefresh,
    entries,
    selectedSet,
    onSelect,
    onOpenDir,
    onOpenFile,
    disabled,
    layoutMode,
    draggableEntries,
    onEntryDragStart,
    dropOverlay,
    onPaneDragOver,
    onPaneDragLeave,
    onPaneDrop,
    paneRef,
    onRowContextMenu,
    onBlankContextMenu,
    onNavigate,
    onToggleCheck,
    onToggleCheckAll,
}: FilePaneProps) {
    const [columnWidths, setColumnWidths] = useState<Record<FileColumnKey, number>>(COLUMN_MIN_WIDTH);
    const resizeRef = useRef<{ key: FileColumnKey; startX: number; startWidth: number; previousCursor: string } | null>(null);
    // 用户手动拖过的列不再被内容自适应覆盖
    const userResizedRef = useRef<Record<FileColumnKey, boolean>>({ name: false, owner: false, size: false, time: false });
    const [sortKey, setSortKey] = useState<'name' | 'size' | 'time'>('name');
    const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc');
    const [filterText, setFilterText] = useState('');
    const [showHidden, setShowHidden] = useState(false);

    // 近似测量文本渲染宽度（11px 字号：ASCII ~6.5px、宽字符 ~12px）
    const measureText = (s: string) => {
        let w = 0;
        for (const ch of s) {
            w += ch.charCodeAt(0) > 255 ? 12 : 6.5;
        }
        return w;
    };
    // 建议宽度：该列所有显示项按长度降序，去掉最长的前 20% 后取剩余最大值，再加内边距
    const suggestWidth = (items: string[], min: number, padding: number) => {
        if (items.length === 0) return min;
        const lens = items.map(measureText).sort((a, b) => b - a);
        const drop = Math.floor(lens.length * 0.2);
        const kept = lens.slice(drop);
        const maxLen = kept.length > 0 ? kept[0] : lens[0];
        return Math.max(min, Math.ceil(maxLen + padding));
    };
    // 内容自适应的初始列宽（名称列需额外容纳复选框+图标+间距）
    const autoWidths = useMemo(() => ({
        name: suggestWidth(entries.map(e => e.name), COLUMN_MIN_WIDTH.name, 92),
        owner: suggestWidth(entries.map(e => getEntryOwnerLabel(e, owner)), COLUMN_MIN_WIDTH.owner, 22),
        size: suggestWidth(entries.map(e => (e.isDir ? '-' : formatFileSize(e.size))), COLUMN_MIN_WIDTH.size, 26),
        time: suggestWidth(entries.map(e => (e.modTime ? new Date(e.modTime).toLocaleString() : '')), COLUMN_MIN_WIDTH.time, 26),
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }), [entries, owner]);
    // entries 变化时，未手动拖过的列跟随内容自适应
    useEffect(() => {
        setColumnWidths(prev => {
            const next = { ...prev };
            (Object.keys(autoWidths) as FileColumnKey[]).forEach(key => {
                if (!userResizedRef.current[key]) next[key] = autoWidths[key];
            });
            return next;
        });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [autoWidths]);

    useEffect(() => {
        const onMove = (event: MouseEvent) => {
            const state = resizeRef.current;
            if (!state) return;
            const nextWidth = Math.max(COLUMN_MIN_WIDTH[state.key], state.startWidth + event.clientX - state.startX);
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

    // 过滤 + 排序后的展示列表：目录始终排在文件前，其余按所选列排序。
    const visibleEntries = useMemo(() => {
        let list = entries;
        if (!showHidden) list = list.filter(e => !e.name.startsWith('.'));
        if (filterText) {
            const f = filterText.toLowerCase();
            list = list.filter(e => e.name.toLowerCase().includes(f));
        }
        if (list.length === 0) return [];
        const sorted = [...list].sort((a, b) => {
            if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
            let cmp = 0;
            if (sortKey === 'size') {
                cmp = (a.size - b.size) || a.name.localeCompare(b.name);
            } else if (sortKey === 'time') {
                const ta = a.modTime ? new Date(a.modTime).getTime() : 0;
                const tb = b.modTime ? new Date(b.modTime).getTime() : 0;
                cmp = (ta - tb) || a.name.localeCompare(b.name);
            } else {
                cmp = a.name.localeCompare(b.name);
            }
            return sortDir === 'asc' ? cmp : -cmp;
        });
        return sorted;
    }, [entries, showHidden, filterText, sortKey, sortDir]);

    const toggleSort = (key: 'name' | 'size' | 'time') => {
        if (sortKey === key) {
            setSortDir(d => (d === 'asc' ? 'desc' : 'asc'));
        } else {
            setSortKey(key);
            setSortDir('asc');
        }
    };

    // 复选框多选：visibleEntries 中已被勾选的数量，用于表头全选/半选状态
    const checkedCount = useMemo(
        () => visibleEntries.filter(e => selectedSet.has(e.path)).length,
        [visibleEntries, selectedSet]
    );
    const allChecked = checkedCount > 0 && checkedCount === visibleEntries.length;
    const someChecked = checkedCount > 0 && checkedCount < visibleEntries.length;
    const checkAllPaths = useMemo(() => visibleEntries.map(e => e.path), [visibleEntries]);
    const handleCheckAllClick = (e: React.ChangeEvent<HTMLInputElement>) => {
        e.stopPropagation();
        onToggleCheckAll?.(checkAllPaths, e.target.checked);
    };

    const sortArrow = (key: 'name' | 'size' | 'time'): string => {
        if (sortKey !== key) return '';
        return sortDir === 'asc' ? ' ▲' : ' ▼';
    };

    // 面包屑：将当前路径拆分为可点击的分段（支持 Windows 盘符与 POSIX 路径）
    const breadcrumbSegments = useMemo(() => {
        const segs: { label: string; path: string }[] = [];
        if (!path) return segs;
        const isPosix = path.startsWith('/') && !/^[A-Za-z]:/.test(path);
        if (isPosix) {
            const parts = path.split('/').filter(Boolean);
            let acc = '';
            segs.push({ label: '/', path: '/' });
            for (const p of parts) {
                acc += '/' + p;
                segs.push({ label: p, path: acc });
            }
            return segs;
        }
        const driveMatch = path.match(/^([A-Za-z]:)(.*)$/);
        if (driveMatch) {
            let acc = driveMatch[1] + '\\';
            segs.push({ label: driveMatch[1], path: acc });
            const rest = driveMatch[2].split(/[\\/]+/).filter(Boolean);
            for (const p of rest) {
                acc += p + '\\';
                segs.push({ label: p, path: acc });
            }
            return segs;
        }
        let acc = '';
        for (const p of path.split(/[\\/]+/).filter(Boolean)) {
            acc = acc ? acc + '\\' + p : p;
            segs.push({ label: p, path: acc });
        }
        return segs;
    }, [path]);

    const renderHeaderCell = (label: string, key: FileColumnKey, extraStyle?: React.CSSProperties) => (
        <th
            style={{ ...styles.th, ...extraStyle }}
            onClick={() => key !== 'owner' && toggleSort(key)}
        >
            <span style={styles.thLabel}>{label}{key !== 'owner' ? sortArrow(key) : ''}</span>
            {layoutMode !== 'narrow' ? (
                <span
                    style={styles.colResizeHandle}
                    data-testid={`column-resize-${key}`}
                    onMouseDown={(event) => {
                        event.preventDefault();
                        userResizedRef.current[key] = true;
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

    // 行内复选框：勾选切换选中集合，onClick 阻止冒泡避免触发行点击（行点击保持单选）
    const renderRowCheckbox = (entry: FileEntry, testId?: string) => (
        <input
            type="checkbox"
            checked={selectedSet.has(entry.path)}
            onChange={() => onToggleCheck?.(entry.path)}
            onClick={(ev) => ev.stopPropagation()}
            disabled={disabled}
            aria-label={`勾选-${entry.name}`}
            data-testid={testId}
        />
    );

    return (
        <div
            ref={paneRef}
            style={{ ...styles.pane, opacity: disabled ? 0.6 : 1 }}
            data-testid={`file-pane-${title}`}
            data-layout-mode={layoutMode}
            onDragOver={onPaneDragOver}
            onDragLeave={onPaneDragLeave}
            onDrop={onPaneDrop}
            onContextMenu={(e) => {
                if (!onBlankContextMenu) return;
                e.preventDefault();
                onBlankContextMenu(e);
            }}
        >
            <div style={styles.paneHeaderRow}>
                <div style={styles.paneTitle}>{title}</div>
                {onNavigate && breadcrumbSegments.length > 0 ? (
                    <>
                        <span style={styles.breadcrumbSep}>/</span>
                        {breadcrumbSegments.map((seg, i) => (
                            <React.Fragment key={seg.path}>
                                <button style={styles.breadcrumbItem} onClick={() => onNavigate(seg.path)} disabled={disabled}>{seg.label}</button>
                                {i < breadcrumbSegments.length - 1 ? <span style={styles.breadcrumbSep}>/</span> : null}
                            </React.Fragment>
                        ))}
                    </>
                ) : null}
                <div style={{ flex: 1 }} />
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
            <div style={styles.filterBar}>
                <input
                    style={styles.filterInput}
                    value={filterText}
                    onChange={(e) => setFilterText(e.target.value)}
                    placeholder="过滤文件名…"
                    disabled={disabled}
                    data-testid={`file-filter-${title}`}
                />
                <label style={styles.hiddenToggle}>
                    <input type="checkbox" checked={showHidden} onChange={(e) => setShowHidden(e.target.checked)} disabled={disabled} />
                    <span>显示隐藏</span>
                </label>
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
                        {visibleEntries.map((e, index) => {
                            const size = e.isDir ? '-' : formatFileSize(e.size);
                            const time = e.modTime ? new Date(e.modTime).toLocaleString() : '';
                            const entryKey = e.path || `${title}:${e.name}:${index}`;
                            const ownerText = getEntryOwnerLabel(e, owner);
                            return (
                                <div
                                    key={entryKey}
                                    style={{
                                        ...styles.compactRow,
                                        ...(selectedSet.has(e.path) ? styles.compactRowSelected : null),
                                        cursor: disabled ? 'not-allowed' : 'pointer',
                                    }}
                                    onClick={(ev) => !disabled && onSelect(e.path, ev)}
                                    onDoubleClick={() => openEntry(e)}
                                    onContextMenu={(ev) => {
                                        if (!onRowContextMenu) return;
                                        ev.preventDefault();
                                        ev.stopPropagation();
                                        onRowContextMenu(e, ev);
                                    }}
                                    draggable={!!draggableEntries && !e.isDir && !disabled}
                                    onDragStart={(event) => onEntryDragStart?.(e, event)}
                                    title={e.name}
                                >
                                    <div style={styles.compactMain}>
                                        {onToggleCheck ? (
                                            <span style={styles.compactCheck}>{renderRowCheckbox(e)}</span>
                                        ) : null}
                                        <FileKindIcon entry={e} />
                                        <span style={styles.compactName}>{e.name}</span>
                                    </div>
                                    <div style={styles.compactMeta}>{ownerText} · {size}{time ? ` · ${time}` : ''}</div>
                                </div>
                            );
                        })}
                        {visibleEntries.length === 0 ? <div style={styles.emptyState}>暂无数据</div> : null}
                    </div>
                ) : (
                <div style={styles.fileTableWrap}>
                    <table style={styles.table}>
                        <colgroup>
                            <col style={{ width: 32 }} />
                            {isColumnVisible('name') ? <col style={{ width: columnWidths.name }} /> : null}
                            {isColumnVisible('owner') ? <col style={{ width: columnWidths.owner }} /> : null}
                            {isColumnVisible('size') ? <col style={{ width: columnWidths.size }} /> : null}
                            {isColumnVisible('time') ? <col style={{ width: columnWidths.time }} /> : null}
                        </colgroup>
                        <thead>
                            <tr>
                                {onToggleCheckAll ? (
                                    <th style={{ ...styles.th, width: 32, padding: '0 4px' }}>
                                        <input
                                            type="checkbox"
                                            checked={allChecked}
                                            ref={el => { if (el) el.indeterminate = someChecked; }}
                                            onChange={handleCheckAllClick}
                                            disabled={disabled || visibleEntries.length === 0}
                                            aria-label={`全选-${title}`}
                                            data-testid={`file-checkall-${title}`}
                                        />
                                    </th>
                                ) : null}
                                {isColumnVisible('name') ? renderHeaderCell('名称', 'name') : null}
                                {isColumnVisible('owner') ? renderHeaderCell('所属', 'owner', styles.cellOwner) : null}
                                {isColumnVisible('size') ? renderHeaderCell('大小', 'size', styles.cellSize) : null}
                                {isColumnVisible('time') ? renderHeaderCell('更新时间', 'time', styles.cellTime) : null}
                            </tr>
                        </thead>
                        <tbody>
                            {visibleEntries.map((e, index) => (
                                <tr
                                    key={e.path || `${title}:${e.name}:${index}`}
                                    style={{
                                        ...styles.fileRow,
                                        ...(selectedSet.has(e.path) ? styles.fileRowSelected : null),
                                        cursor: disabled ? 'not-allowed' : 'pointer',
                                    }}
                                    onClick={(ev) => !disabled && onSelect(e.path, ev)}
                                    onDoubleClick={() => {
                                        openEntry(e);
                                    }}
                                    onContextMenu={(ev) => {
                                        if (!onRowContextMenu) return;
                                        ev.preventDefault();
                                        ev.stopPropagation();
                                        onRowContextMenu(e, ev);
                                    }}
                                    draggable={!!draggableEntries && !e.isDir && !disabled}
                                    onDragStart={(event) => onEntryDragStart?.(e, event)}
                                >
                                    {onToggleCheck ? (
                                        <td style={{ ...styles.td, ...styles.cellCheck }}>
                                            {renderRowCheckbox(e, `file-check-${title}-${index}`)}
                                        </td>
                                    ) : null}
                                    <td style={{ ...styles.td, ...styles.cellName }} title={e.name}>
                                        <FileKindIcon entry={e} />
                                        <span>{e.name}</span>
                                    </td>
                                    {isColumnVisible('owner') ? <td style={{ ...styles.td, ...styles.cellOwner }}>{getEntryOwnerLabel(e, owner)}</td> : null}
                                    {isColumnVisible('size') ? <td style={{ ...styles.td, ...styles.cellSize }}>{e.isDir ? '-' : formatFileSize(e.size)}</td> : null}
                                    {isColumnVisible('time') ? <td style={{ ...styles.td, ...styles.cellTime }}>{e.modTime ? new Date(e.modTime).toLocaleString() : ''}</td> : null}
                                </tr>
                            ))}
                            {visibleEntries.length === 0 ? (
                                <tr>
                                    <td style={styles.td} colSpan={visibleColumns.length + (onToggleCheck ? 1 : 0)}>暂无数据</td>
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

const FilesPanel: React.FC<FilesPanelProps> = ({ activeTerminalId, terminals, host }) => {
    const api = host;
    const defaultSessionId = useMemo(() => activeTerminalId || (terminals[0]?.id ?? ''), [activeTerminalId, terminals]);

    const [sessionId, setSessionId] = useState(defaultSessionId);
    const [protocol, setProtocol] = useState<string>('');
    const [loading, setLoading] = useState(false);
    const [msg, setMsg] = useState('');

    const [localPath, setLocalPath] = useState<string>('');
    const [localPathInput, setLocalPathInput] = useState<string>('');
    const [localEntries, setLocalEntries] = useState<FileEntry[]>([]);
    const [localSelected, setLocalSelected] = useState<Set<string>>(new Set());
    const [localAnchor, setLocalAnchor] = useState<string>('');

    const [remotePath, setRemotePath] = useState<string>('/root');
    const [remotePathInput, setRemotePathInput] = useState<string>('/root');
    const [remoteEntries, setRemoteEntries] = useState<FileEntry[]>([]);
    const [remoteSelected, setRemoteSelected] = useState<Set<string>>(new Set());
    const [remoteAnchor, setRemoteAnchor] = useState<string>('');
    const [editOpen, setEditOpen] = useState(false);
    const [editPath, setEditPath] = useState('');
    const [editContent, setEditContent] = useState('');
    const [editSaving, setEditSaving] = useState(false);
    const [scpDownloadRemote, setScpDownloadRemote] = useState('');
    const [scpDownloadLocal, setScpDownloadLocal] = useState('');

    const [tasks, setTasks] = useState<Record<string, TaskState>>({});
    const [drawerOpen, setDrawerOpen] = useState(false);
    // 用户手动隐藏队列后，抑制 done/注册事件的自动弹出，直到用户主动再打开。
    const queueHiddenByUserRef = useRef(false);
    // 队列面板内容高度（可拖拽调整），持久化到 localStorage。
    const [queueBodyHeight, setQueueBodyHeight] = useState<number>(() => {
        try {
            const v = Number(window.localStorage.getItem(QUEUE_HEIGHT_STORAGE_KEY));
            if (Number.isFinite(v) && v >= QUEUE_MIN_HEIGHT && v <= 2000) return Math.floor(v);
        } catch { /* localStorage 不可用时用默认值 */ }
        return QUEUE_DEFAULT_HEIGHT;
    });
    const [ctxMenu, setCtxMenu] = useState<{ x: number; y: number; items: ContextMenuItem[] } | null>(null);
    const [nameDialog, setNameDialog] = useState<{ title: string; defaultValue: string; onConfirm: (name: string) => void; onCancel: () => void } | null>(null);
    // 弹出应用内名称输入框，返回输入值；取消返回 null。
    const askName = (title: string, defaultValue = ''): Promise<string | null> =>
        new Promise(resolve => {
            setNameDialog({
                title,
                defaultValue,
                onConfirm: (name) => { setNameDialog(null); resolve(name); },
                onCancel: () => { setNameDialog(null); resolve(null); },
            });
        });
    const copyPaths = (paths: string[]) => {
        if (paths.length === 0) return;
        try {
            navigator.clipboard?.writeText(paths.join('\n'));
        } catch {
            // 剪贴板不可用时静默忽略
        }
    };
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
    const [localDropOverlay, setLocalDropOverlay] = useState<DropOverlayState | null>(null);
    const draggedLocalEntryRef = useRef<FileEntry | null>(null);
    const remotePaneRef = useRef<HTMLDivElement>(null);
    const localPaneRef = useRef<HTMLDivElement>(null);
    const startUploadFileRef = useRef<(entry: FileEntry) => Promise<void>>();

    // OS 级文件拖放（Wails 宿主经 host.onFileDrop 提供真实路径；其余宿主不订阅）。
    // 订阅对任意 drop 都触发 —— 组件自己做命中测试。
    useEffect(() => {
        if (!api.onFileDrop) return;

        const handleFileDrop = async (x: number, y: number, paths: string[]) => {
            if (!paths || paths.length === 0) return;

            // Hit-test: remote pane
            const remotePane = remotePaneRef.current;
            if (remotePane) {
                const rect = remotePane.getBoundingClientRect();
                if (x >= rect.left && x <= rect.right && y >= rect.top && y <= rect.bottom) {
                    setRemoteDropOverlay(null);
                    if (!sessionIdRef.current) {
                        setMsg('请先选择一个已连接的会话');
                        return;
                    }
                    const proto = protocolRef.current;
                    const supported = proto.startsWith('sftp') || proto.startsWith('scp') || proto.includes('root-relay');
                    if (!supported) {
                        setMsg('当前连接不支持文件上传');
                        return;
                    }
                    for (const filePath of paths) {
                        const name = filePath.replace(/^.*[\\/]/, '');
                        const entry: FileEntry = {
                            path: filePath, name, isDir: false,
                            size: 0, modTime: '', mode: 0,
                        };
                        if (startUploadFileRef.current) {
                            await startUploadFileRef.current(entry);
                        }
                    }
                    return;
                }
            }

            // Hit-test: local pane
            const localPane = localPaneRef.current;
            if (localPane) {
                const rect = localPane.getBoundingClientRect();
                if (x >= rect.left && x <= rect.right && y >= rect.top && y <= rect.bottom) {
                    setLocalDropOverlay(null);
                    const dir = localPathRef.current;
                    if (!dir) {
                        setMsg('请先进入一个本地目录');
                        return;
                    }
                    setLoading(true);
                    try {
                        for (const filePath of paths) {
                            const name = filePath.replace(/^.*[\\/]/, '');
                            const dst = dir + (dir.endsWith('\\') || dir.endsWith('/') ? '' : '\\') + name;
                            if (!api.LocalCopy) {
                                setMsg('当前环境不支持复制系统文件');
                                return;
                            }
                            await api.LocalCopy(filePath, dst);
                        }
                        refreshLocalAuto();
                    } catch (e: any) {
                        setMsg('复制失败: ' + e.toString());
                    } finally {
                        setLoading(false);
                    }
                    return;
                }
            }
        };

        return api.onFileDrop(handleFileDrop);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    // Global drag/drop interception: prevent browser defaults for managed file drags.
    useEffect(() => {
        const hasManagedFileDrag = (e: DragEvent) => {
            const types = e.dataTransfer?.types ? Array.from(e.dataTransfer.types) : [];
            return types.includes('Files') || types.includes('application/x-opscopilot-local-file') || !!draggedLocalEntryRef.current;
        };
        const onDragOver = (e: DragEvent) => {
            if (hasManagedFileDrag(e)) e.preventDefault();
        };
        const onDrop = (e: DragEvent) => {
            if (!hasManagedFileDrag(e)) return;
            e.preventDefault();
            // Allow to bubble if it lands on a managed pane (React handler or Wails OnFileDrop)
            const remote = remotePaneRef.current;
            const local = localPaneRef.current;
            const target = e.target;
            if (target instanceof Node) {
                if (remote && remote.contains(target)) return;
                if (local && local.contains(target)) return;
            }
            e.stopPropagation();
        };
        document.addEventListener('dragover', onDragOver, true);
        document.addEventListener('drop', onDrop, true);
        return () => {
            document.removeEventListener('dragover', onDragOver, true);
            document.removeEventListener('drop', onDrop, true);
        };
    }, []);

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

    // 统一的多选逻辑：单击单选；Ctrl/Cmd 点击增减；Shift 点击范围选择。
    // anchor 为当前 pane 的锚点（上次点击项），用于 Shift 范围选择的起点。
    const handleSelect = (p: string, event: React.MouseEvent, entries: FileEntry[], setSel: React.Dispatch<React.SetStateAction<Set<string>>>, setAnchor: React.Dispatch<React.SetStateAction<string>>, anchor: string) => {
        const multi = event.metaKey || event.ctrlKey;
        const range = event.shiftKey;
        setSel(prev => {
            const next = new Set(prev);
            if (range && entries.length > 0) {
                const anchorPath = anchor || entries[0].path;
                const idxA = entries.findIndex(e => e.path === anchorPath);
                const idxB = entries.findIndex(e => e.path === p);
                if (idxA >= 0 && idxB >= 0) {
                    const [lo, hi] = idxA <= idxB ? [idxA, idxB] : [idxB, idxA];
                    next.clear();
                    for (let i = lo; i <= hi; i += 1) next.add(entries[i].path);
                } else {
                    next.clear();
                    next.add(p);
                }
                return next;
            }
            if (multi) {
                if (next.has(p)) next.delete(p);
                else next.add(p);
                return next;
            }
            return new Set([p]);
        });
        setAnchor(p);
    };

    // 复选框勾选（单行）：切换选中集合中该路径的存在性
    const toggleCheck = (setSel: React.Dispatch<React.SetStateAction<Set<string>>>, p: string) => {
        setSel(prev => {
            const next = new Set(prev);
            if (next.has(p)) next.delete(p);
            else next.add(p);
            return next;
        });
    };
    // 表头全选/取消全选：作用于当前可见（过滤/排序后）的条目
    const toggleCheckAll = (setSel: React.Dispatch<React.SetStateAction<Set<string>>>, paths: string[], checked: boolean) => {
        setSel(prev => {
            const next = new Set(prev);
            for (const p of paths) {
                if (checked) next.add(p);
                else next.delete(p);
            }
            return next;
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
        setRemoteSelected(new Set());
        setRemoteAnchor('');
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
        // 宿主事件经端口订阅（Wails: window.runtime.EventsOn；Sidecar: RPC 通知）
        offProgress = api.onProgress((data) => {
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
                        // 后端字节进度会显式下发空 step 清除排队等提示；
                        // 仅当事件未携带 step 字段时才保留旧值
                        step: data?.step !== undefined ? String(data.step) : cur.step
                    }
                };
            });
        });

        offDone = api.onDone((data) => {
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
                // 优先用后端显式 cancelled 标志判定（不依赖错误消息文本）
                const cancelled = !!data?.cancelled || data?.message?.includes('取消');
                const status = ok ? 'done' : (cancelled ? 'cancelled' : 'error');
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
            setDrawerOpen(open => (queueHiddenByUserRef.current ? open : true));

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

    // 注册传输任务到队列。IPC 委派任务（主程序执行）收不到 done 事件，直接标记完成。
    const registerTask = (taskId: string, message?: string, name?: string) => {
        const isDelegated = !!message?.includes?.('主程序');
        setTasks(prev => ({
            ...prev,
            [taskId]: {
                taskId,
                sessionId,
                name,
                bytesDone: 0,
                bytesTotal: -1,
                speedBps: 0,
                status: isDelegated ? 'done' : 'running',
                message: isDelegated ? (message || '任务已提交到主程序执行') : undefined,
            },
        }));
        setDrawerOpen(open => (queueHiddenByUserRef.current ? open : true));
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
        const src = [...localSelected][0];
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
                registerTask(resp.taskId, (resp as any).message, entry.name);
            }
        } catch (e: any) {
            setMsg('失败: ' + e.toString());
        } finally {
            setLoading(false);
        }
    };
    startUploadFileRef.current = startUploadFile;

    const handleLocalEntryDragStart = (entry: FileEntry, event: React.DragEvent) => {
        if (entry.isDir) return;
        draggedLocalEntryRef.current = entry;
        event.dataTransfer.effectAllowed = 'copy';
        event.dataTransfer.setData('application/x-opscopilot-local-file', JSON.stringify(entry));
        event.dataTransfer.setData('text/plain', entry.path);
    };

    const getRemoteDropOverlay = (event: React.DragEvent): DropOverlayState => {
        const hasLocalEntry = event.dataTransfer.types.includes('application/x-opscopilot-local-file');
        const hasExternalFile = event.dataTransfer.types.includes('Files');
        // OS 文件拖入需要宿主提供真实路径（Wails OnFileDrop；经端口注入）
        const hasHostDrop = !!api.onFileDrop;

        if (!hasLocalEntry && !hasExternalFile) {
            return {
                visible: true,
                blocked: true,
                title: '无法识别拖入内容',
                detail: '请从左侧本地文件列表拖入文件',
            };
        }
        // External files from OS file manager: only host drop can resolve real paths
        if (!hasLocalEntry && hasExternalFile && !hasHostDrop) {
            return {
                visible: true,
                blocked: true,
                title: '当前环境无法读取系统文件路径',
                detail: '请从左侧本地文件列表拖入文件',
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
                if (entry.path && entry.name) {
                    draggedLocalEntryRef.current = null;
                    return entry;
                }
            } catch {
            }
        }

        // Fallback to ref when dataTransfer payload is lost (Wails WebView)
        const refEntry = draggedLocalEntryRef.current;
        if (refEntry) {
            draggedLocalEntryRef.current = null;
            return refEntry;
        }

        return null;
    };

    const handleRemoteDrop = async (event: React.DragEvent<HTMLDivElement>) => {
        event.preventDefault();
        const overlay = getRemoteDropOverlay(event);
        setRemoteDropOverlay(null);
        if (overlay.blocked) {
            setMsg(overlay.detail);
            return;
        }

        // Internal drags: handle here via dataTransfer / ref
        const hasLocalEntry = event.dataTransfer.types.includes('application/x-opscopilot-local-file');
        if (hasLocalEntry) {
            const entry = extractDroppedLocalEntry(event);
            if (!entry) {
                setMsg('未能读取拖入文件路径，请从左侧本地文件列表拖拽文件上传');
                return;
            }
            await startUploadFile(entry);
        }
        // External OS file drops: Wails OnFileDrop callback handles the upload.
        // Do NOT stopPropagation — the event must bubble for Wails to detect it.
    };

    // ── Local pane drop: copy external files into the current local directory ──
    const getLocalDropOverlay = (event: React.DragEvent): DropOverlayState => {
        const hasFiles = event.dataTransfer.types.includes('Files');
        if (!hasFiles) {
            return {
                visible: true,
                blocked: true,
                title: '无法识别拖入内容',
                detail: '请拖入系统文件',
            };
        }
        if (!localPath) {
            return {
                visible: true,
                blocked: true,
                title: '无法复制',
                detail: '请先进入一个本地目录',
            };
        }
        return {
            visible: true,
            blocked: false,
            title: '复制到本地目录',
            detail: localPath,
        };
    };

    const handleLocalDragOver = (event: React.DragEvent<HTMLDivElement>) => {
        const overlay = getLocalDropOverlay(event);
        event.preventDefault();
        event.dataTransfer.dropEffect = overlay.blocked ? 'none' : 'copy';
        setLocalDropOverlay(overlay);
    };

    const handleLocalDragLeave = (event: React.DragEvent<HTMLDivElement>) => {
        if (event.currentTarget.contains(event.relatedTarget as Node | null)) return;
        setLocalDropOverlay(null);
    };

    const handleLocalDrop = async (event: React.DragEvent<HTMLDivElement>) => {
        event.preventDefault();
        setLocalDropOverlay(null);
        // External OS file copies are handled by Wails OnFileDrop callback.
        // Do NOT stopPropagation — the event must bubble for Wails to detect it.
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
        const src = [...remoteSelected][0];
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

    // 批量上传：遍历所有选中的本地文件逐个上传（跳过目录）。
    const startUploadSelectedAll = async () => {
        if (!sessionId) {
            setMsg('请先选择会话');
            return;
        }
        if (!isTransferSupported()) {
            setMsg('对端不支持文件传输');
            return;
        }
        const targets = localEntries.filter(e => localSelected.has(e.path) && !e.isDir);
        if (targets.length === 0) {
            setMsg('请先选择本地文件');
            return;
        }
        for (const entry of targets) {
            await startUploadFile(entry);
        }
    };

    // 批量下载：遍历所有选中的远端文件逐个下载（跳过目录）。
    const startDownloadSelectedAll = async () => {
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
        const targets = remoteEntries.filter(e => remoteSelected.has(e.path) && !e.isDir);
        if (targets.length === 0) {
            setMsg('请先选择远端文件');
            return;
        }
        // 后端按会话限流排队，这里逐个启动；启动失败的文件汇总提示，
        // 传输进度与成败在任务列表中逐项展示。
        const failures: string[] = [];
        for (const entry of targets) {
            await startDownloadFile(entry, (m) => failures.push(`${entry.name}: ${m}`));
        }
        if (failures.length > 0) {
            setMsg(`批量下载：${targets.length - failures.length} 个任务已开始，${failures.length} 个启动失败\n` + failures.join('\n'));
        }
    };

    // 右键菜单：右键未选中的条目时先单选该条目，已选中时保留多选。
    // 用"右键目标是否在选中集合中"来决定操作是单文件还是批量。
    const showLocalRowMenu = (entry: FileEntry, event: React.MouseEvent) => {
        if (!localSelected.has(entry.path)) {
            handleSelect(entry.path, event, localEntries, setLocalSelected, setLocalAnchor, localAnchor);
        }
        const multi = localSelected.has(entry.path) && localSelected.size > 1;
        const items: ContextMenuItem[] = [
            { label: multi ? `上传所选 ${localSelected.size} 项` : '上传', onClick: () => startUploadSelectedAll() },
            { label: '新建文件夹', onClick: () => createLocalFolder() },
            { label: '重命名', disabled: multi, onClick: () => renameLocalSelected() },
            { label: '删除', danger: true, onClick: () => deleteLocalSelected() },
            { label: multi ? `复制路径 (${localSelected.size})` : '复制路径', onClick: () => copyPaths([...localSelected]) },
            { label: '刷新', onClick: () => refreshLocal(localPath) },
        ];
        setCtxMenu({ x: event.clientX, y: event.clientY, items });
    };

    const showLocalBlankMenu = (event: React.MouseEvent) => {
        setCtxMenu({
            x: event.clientX,
            y: event.clientY,
            items: [
                { label: '新建文件夹', onClick: () => createLocalFolder() },
                { label: '刷新', onClick: () => refreshLocal(localPath) },
            ],
        });
    };

    const showRemoteRowMenu = (entry: FileEntry, event: React.MouseEvent) => {
        if (!remoteSelected.has(entry.path)) {
            handleSelect(entry.path, event, remoteEntries, setRemoteSelected, setRemoteAnchor, remoteAnchor);
        }
        const multi = remoteSelected.has(entry.path) && remoteSelected.size > 1;
        const singleTarget = remoteSelected.has(entry.path) && remoteSelected.size === 1;
        const items: ContextMenuItem[] = [
            { label: multi ? `下载所选 ${remoteSelected.size} 项` : '下载', onClick: () => startDownloadSelectedAll() },
            { label: '编辑', disabled: !singleTarget || entry.isDir || !isSFTPSupported(), onClick: () => openRemoteEditor() },
            { label: '新建文件夹', onClick: () => createRemoteFolder() },
            { label: '重命名', disabled: multi || !isSFTPSupported(), onClick: () => renameRemoteSelected() },
            { label: '删除', danger: true, onClick: () => deleteRemoteSelected() },
            { label: multi ? `复制路径 (${remoteSelected.size})` : '复制路径', onClick: () => copyPaths([...remoteSelected]) },
            { label: '刷新', onClick: () => refreshRemote(remotePath) },
        ];
        setCtxMenu({ x: event.clientX, y: event.clientY, items });
    };

    const showRemoteBlankMenu = (event: React.MouseEvent) => {
        setCtxMenu({
            x: event.clientX,
            y: event.clientY,
            items: [
                { label: '新建文件夹', onClick: () => createRemoteFolder() },
                { label: '刷新', onClick: () => refreshRemote(remotePath) },
            ],
        });
    };

    // 下载前冲突处理：本地已存在同名文件时，让用户选择覆盖/另存为/取消。
    // 返回最终保存路径；null 表示用户取消本次下载。
    // 另存为依赖宿主的保存对话框能力（SelectSavePath），缺失时不提供该选项。
    const resolveLocalConflict = async (name: string, dst: string): Promise<string | null> => {
        try {
            const raw = await api.LocalStat(dst);
            const resp = parseResp(raw);
            if (resp && resp.ok) {
                const choices: ConfirmChoice[] = [
                    { label: '覆盖', value: 'overwrite', danger: true, primary: true },
                ];
                if (api.SelectSavePath) {
                    choices.push({ label: '另存为…', value: 'save-as' });
                }
                const choice = await confirmDialog.show({
                    title: '本地文件已存在',
                    message: `本地已存在同名文件：\n${dst}\n\n请选择处理方式：`,
                    cancelText: '取消',
                    choices,
                });
                if (choice === 'save-as' && api.SelectSavePath) {
                    const saved = await api.SelectSavePath(name);
                    if (!saved) return null;
                    return saved;
                }
                if (choice !== 'overwrite') return null;
            }
        } catch {
            // 无法查询（视为目标不存在），继续使用默认路径
        }
        return dst;
    };

    const startDownloadFile = async (entry: FileEntry, onError?: (msg: string) => void) => {
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
        const defaultDst = localPath ? `${localPath}${localPath.endsWith('\\') || localPath.endsWith('/') ? '' : '\\'}${entry.name}` : entry.name;
        const dst = await resolveLocalConflict(entry.name, defaultDst);
        if (!dst) return; // 用户取消
        setLoading(true);
        setMsg('');
        try {
            const raw = await api.FTDownload(sessionId, entry.path, dst);
            const resp = parseResp(raw);
            if (!resp) {
                const m = '返回格式错误';
                if (onError) onError(m); else setMsg(m);
                return;
            }
            if (!resp.ok) {
                const m = formatError(resp);
                if (onError) onError(m); else setMsg(m);
                return;
            }
            if (resp.taskId) {
                registerTask(resp.taskId, (resp as any).message, entry.name);
            }
        } catch (e: any) {
            const m = '失败: ' + e.toString();
            if (onError) onError(m); else setMsg(m);
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
        const lp0 = scpDownloadLocal.trim();
        if (!rp || !lp0) {
            setMsg('请填写远端路径与本地保存路径');
            return;
        }
        const lp = await resolveLocalConflict(lp0.replace(/^.*[\\/]/, '') || 'download', lp0);
        if (!lp) return; // 用户取消
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
                registerTask(resp.taskId, (resp as any).message, rp.replace(/^.*[\\/]/, '') || 'download');
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
        const name = await askName('新建文件夹');
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
        const targets = [...remoteSelected];
        if (targets.length === 0) {
            setMsg('请先选择远端文件或目录');
            return;
        }
        const targetSet = new Set(targets);
        const hasDir = remoteEntries.some(e => targetSet.has(e.path) && e.isDir);
        const targetNames = targets.map(p => p.split('/').pop() || p);
        const summary = targetNames.length <= 3 ? targetNames.join('\n') : targetNames.slice(0, 3).join('\n') + `\n…等 ${targetNames.length} 项`;
        const ok = await confirmDialog.show({
            message: hasDir
                ? `确定要删除以下 ${targets.length} 项吗？\n${summary}\n\n其中包含目录，将递归删除目录及其全部内容，此操作不可恢复。`
                : `确定要删除以下 ${targets.length} 项吗？\n${summary}\n\n此操作不可恢复。`,
            danger: true,
        });
        if (!ok) return;
        setLoading(true);
        setMsg('');
        let firstError = '';
        try {
            for (const p of targets) {
                const raw = await api.FTRemoteRemove(sessionId, p);
                const resp = parseResp(raw);
                if (!resp) {
                    firstError = '返回格式错误';
                    break;
                }
                if (!resp.ok) {
                    firstError = formatError(resp);
                    break;
                }
            }
            if (firstError) {
                setMsg(firstError);
                return;
            }
            setRemoteSelected(new Set());
            setRemoteAnchor('');
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
        if (remoteSelected.size === 0) {
            setMsg('请先选择远端文件或目录');
            return;
        }
        const entry = remoteEntries.find(e => e.path === [...remoteSelected][0]);
        const next = await askName('重命名为', entry?.name || '');
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
            setRemoteSelected(new Set());
            setRemoteAnchor('');
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
        if (remoteSelected.size === 0) {
            setMsg('请先选择远端文件');
            return;
        }
        const entry = remoteEntries.find(e => e.path === [...remoteSelected][0]);
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
            setEditContent(resp.content || '');
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
        const name = await askName('新建文件夹');
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
        const targets = [...localSelected];
        if (targets.length === 0) {
            setMsg('请先选择本地文件或目录');
            return;
        }
        const targetSet = new Set(targets);
        const hasDir = localEntries.some(e => targetSet.has(e.path) && e.isDir);
        const targetNames = targets.map(p => p.replace(/^.*[\\/]/, '') || p);
        const summary = targetNames.length <= 3 ? targetNames.join('\n') : targetNames.slice(0, 3).join('\n') + `\n…等 ${targetNames.length} 项`;
        const ok = await confirmDialog.show({
            message: hasDir
                ? `确定要删除以下 ${targets.length} 项吗？\n${summary}\n\n其中包含目录，将递归删除目录及其全部内容，此操作不可恢复。`
                : `确定要删除以下 ${targets.length} 项吗？\n${summary}\n\n此操作不可恢复。`,
            danger: true,
        });
        if (!ok) return;
        setLoading(true);
        setMsg('');
        let firstError = '';
        try {
            for (const p of targets) {
                const raw = await api.LocalRemove(p);
                const resp = parseResp(raw);
                if (!resp) {
                    firstError = '返回格式错误';
                    break;
                }
                if (!resp.ok) {
                    firstError = formatError(resp);
                    break;
                }
            }
            if (firstError) {
                setMsg(firstError);
                return;
            }
            setLocalSelected(new Set());
            setLocalAnchor('');
            await refreshLocal(localPath);
        } catch (e: any) {
            setMsg('失败: ' + e.toString());
        } finally {
            setLoading(false);
        }
    };

    const renameLocalSelected = async () => {
        if (localSelected.size === 0) {
            setMsg('请先选择本地文件或目录');
            return;
        }
        const entry = localEntries.find(e => e.path === [...localSelected][0]);
        const next = await askName('重命名为', entry?.name || '');
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
            setLocalSelected(new Set());
            setLocalAnchor('');
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
    // 队列聚合统计：批量传输时提供全量进度。排队任务以后端 step 提示的前缀识别。
    const queueStats = (() => {
        let done = 0, error = 0, cancelled = 0, running = 0, queued = 0;
        for (const t of taskList) {
            if (t.status === 'done') done++;
            else if (t.status === 'error') error++;
            else if (t.status === 'cancelled') cancelled++;
            else if ((t.step || '').startsWith('排队')) queued++;
            else running++;
        }
        const total = taskList.length;
        const finished = done + error + cancelled;
        return { total, done, error, cancelled, running, queued, finished, pct: total > 0 ? Math.floor(finished / total * 100) : 0 };
    })();
    const hiddenErrorCount = queueStats.error;
    // 清理已结束且无待处理信息的任务（失败任务保留，便于查看与重试）。
    const clearFinishedTasks = () => {
        setTasks(prev => {
            const next: Record<string, TaskState> = {};
            for (const [id, t] of Object.entries(prev)) {
                if (t.status === 'done' || t.status === 'cancelled') continue;
                next[id] = t;
            }
            return next;
        });
    };
    // 拖拽队列顶部手柄调整高度：向上拖增大、向下拖减小，双击恢复默认。
    const startQueueResize = (e: React.MouseEvent) => {
        e.preventDefault();
        const startY = e.clientY;
        const startHeight = queueBodyHeight;
        const maxHeight = Math.max(QUEUE_MIN_HEIGHT, Math.floor((containerRef.current?.clientHeight || 400) * 0.6));
        let latest = startHeight;
        const onMove = (ev: MouseEvent) => {
            latest = Math.min(maxHeight, Math.max(QUEUE_MIN_HEIGHT, startHeight + (startY - ev.clientY)));
            setQueueBodyHeight(latest);
        };
        const onUp = () => {
            window.removeEventListener('mousemove', onMove);
            window.removeEventListener('mouseup', onUp);
            try { window.localStorage.setItem(QUEUE_HEIGHT_STORAGE_KEY, String(latest)); } catch { /* 忽略持久化失败 */ }
        };
        window.addEventListener('mousemove', onMove);
        window.addEventListener('mouseup', onUp);
    };
    const resetQueueHeight = () => {
        setQueueBodyHeight(QUEUE_DEFAULT_HEIGHT);
        try { window.localStorage.setItem(QUEUE_HEIGHT_STORAGE_KEY, String(QUEUE_DEFAULT_HEIGHT)); } catch { /* 忽略持久化失败 */ }
    };
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
                <select style={styles.select} value={sessionId} onChange={(e) => setSessionId(e.target.value)} aria-label="当前会话">
                    {terminals.map(t => (
                        <option key={t.id} value={t.id}>
                            {t.title || t.id}
                        </option>
                    ))}
                </select>
                {protocol ? (
                    <span style={styles.protocolChip} title={`${getProtocolLabel(protocol)} · ${getWorkModeLabel(protocol)}`}>
                        {getProtocolLabel(protocol)}
                    </span>
                ) : (
                    <span style={styles.protocolChipMuted}>连接方式未探测</span>
                )}
                <div style={{ flex: 1 }} />
                <button style={styles.btnSecondary} onClick={() => {
                    if (isNarrow) {
                        setNarrowPane(v => v === 'queue' ? 'local' : 'queue');
                        return;
                    }
                    setDrawerOpen(v => {
                        queueHiddenByUserRef.current = !v ? false : true;
                        return !v;
                    });
                }}>
                    {showQueue ? '隐藏队列' : `显示队列${hiddenErrorCount > 0 ? ` (${hiddenErrorCount} 失败)` : ''}`}
                </button>
            </div>

            {isNarrow ? (
                <div style={styles.segmented} role="tablist" aria-label="文件传输视图">
                    <button style={narrowPane === 'local' ? styles.segmentedActive : styles.segmentedButton} onClick={() => { queueHiddenByUserRef.current = true; setNarrowPane('local'); }}>本地</button>
                    <button style={narrowPane === 'remote' ? styles.segmentedActive : styles.segmentedButton} onClick={() => { queueHiddenByUserRef.current = true; setNarrowPane('remote'); }}>远端</button>
                    <button style={narrowPane === 'queue' ? styles.segmentedActive : styles.segmentedButton} onClick={() => { queueHiddenByUserRef.current = false; setNarrowPane('queue'); }}>队列</button>
                </div>
            ) : null}

            {msg ? <div style={{ color: 'var(--text-tertiary)', fontSize: '12px' }}>{msg}</div> : null}

            {isRootRelay() ? (
                <div style={styles.relayBanner}>
                    当前无法 Root 直连，已切换为 Root 中转模式。通过 Base64 直传传输文件，单文件上限 300 KB，传输后自动校验文件完整性。
                </div>
            ) : null}

            {!isSFTPSupported() && protocol.startsWith('scp') && !isRootRelay() ? (
                <div style={{ color: 'var(--text-tertiary)', fontSize: '12px' }}>
                    当前为 SCP 降级模式，仅支持上传/下载，不支持远端浏览与管理。
                </div>
            ) : null}

            {(!isNarrow || narrowPane !== 'queue') ? (
            <div style={splitStyle} data-testid="file-transfer-split">
                {(!isNarrow || narrowPane === 'local') ? (
                    <FilePane
                        title="本地"
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
                        selectedSet={localSelected}
                        onSelect={(p, ev) => handleSelect(p, ev, localEntries, setLocalSelected, setLocalAnchor, localAnchor)}
                        onOpenDir={(p) => {
                            setLocalSelected(new Set());
                            setLocalAnchor('');
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
                        dropOverlay={localDropOverlay}
                        onPaneDragOver={handleLocalDragOver}
                        onPaneDragLeave={handleLocalDragLeave}
                        onPaneDrop={handleLocalDrop}
                        paneRef={localPaneRef}
                        onRowContextMenu={showLocalRowMenu}
                        onBlankContextMenu={showLocalBlankMenu}
                        onNavigate={(p) => {
                            setLocalSelected(new Set());
                            setLocalAnchor('');
                            refreshLocal(p);
                        }}
                        onToggleCheck={(p) => toggleCheck(setLocalSelected, p)}
                        onToggleCheckAll={(paths, checked) => toggleCheckAll(setLocalSelected, paths, checked)}
                    />
                ) : null}

                {(!isNarrow || narrowPane === 'remote') ? (isSCPMode() ? (
                    <div
                        style={styles.scpPane}
                        ref={remotePaneRef}
                        onDragOver={handleRemoteDragOver}
                        onDragLeave={handleRemoteDragLeave}
                        onDrop={handleRemoteDrop}
                    >
                        {remoteDropOverlay?.visible ? (
                            <div
                                style={{
                                    ...styles.dropOverlay,
                                    ...(remoteDropOverlay.blocked ? styles.dropOverlayBlocked : styles.dropOverlayReady),
                                }}
                                data-testid="file-drop-overlay"
                            >
                                <div style={styles.dropOverlayTitle}>{remoteDropOverlay.title}</div>
                                <div style={styles.dropOverlayDetail}>{remoteDropOverlay.detail}</div>
                            </div>
                        ) : null}
                        <div style={styles.paneHeader}>
                            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                                <div style={{ color: 'var(--text-primary)', fontSize: '12px', fontWeight: 600 }}>远端（SCP）</div>
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
                        owner="-"
                        path={remotePath}
                        pathInput={remotePathInput}
                        onPathInputChange={setRemotePathInput}
                        onGo={() => refreshRemote(remotePathInput)}
                        onUp={() => refreshRemote(remoteParent(remotePath))}
                        onRefresh={() => refreshRemote(remotePath)}
                        entries={remoteEntries}
                        selectedSet={remoteSelected}
                        onSelect={(p, ev) => handleSelect(p, ev, remoteEntries, setRemoteSelected, setRemoteAnchor, remoteAnchor)}
                        onOpenDir={(p) => {
                            const next = p;
                            setRemoteSelected(new Set());
                            setRemoteAnchor('');
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
                        paneRef={remotePaneRef}
                        onRowContextMenu={showRemoteRowMenu}
                        onBlankContextMenu={showRemoteBlankMenu}
                        onNavigate={(p) => {
                            setRemoteSelected(new Set());
                            setRemoteAnchor('');
                            refreshRemote(p);
                        }}
                        onToggleCheck={(p) => toggleCheck(setRemoteSelected, p)}
                        onToggleCheckAll={(paths, checked) => toggleCheckAll(setRemoteSelected, paths, checked)}
                    />
                )) : null}
            </div>
            ) : null}

            {showQueue ? (
                <div style={styles.drawer}>
                    <div
                        style={styles.queueResizeHandle}
                        title="拖拽调整队列高度，双击恢复默认"
                        onMouseDown={startQueueResize}
                        onDoubleClick={resetQueueHeight}
                    />
                    <div style={styles.drawerHeader}>
                        <div style={{ color: 'var(--text-primary)', fontSize: '13px', fontWeight: 600, whiteSpace: 'nowrap' }}>传输队列</div>
                        <div style={styles.queueSummaryText} data-testid="queue-summary">
                            <span>共 {queueStats.total}</span>
                            {queueStats.total > 0 ? (
                                <>
                                    <span style={{ color: 'var(--success)' }}>✓ {queueStats.done}</span>
                                    {queueStats.running > 0 ? <span style={{ color: 'var(--severity-info)' }}>传输 {queueStats.running}</span> : null}
                                    {queueStats.queued > 0 ? <span style={{ color: 'var(--text-tertiary)' }}>排队 {queueStats.queued}</span> : null}
                                    {queueStats.error > 0 ? <span style={{ color: 'var(--severity-danger)' }}>✗ {queueStats.error}</span> : null}
                                    {queueStats.cancelled > 0 ? <span style={{ color: 'var(--text-tertiary)' }}>取消 {queueStats.cancelled}</span> : null}
                                </>
                            ) : null}
                        </div>
                        <div style={{ flex: 1 }} />
                        <button
                            style={styles.btnSecondary}
                            onClick={clearFinishedTasks}
                            disabled={queueStats.done + queueStats.cancelled === 0}
                        >清空已完成</button>
                        <button style={styles.btnSecondary} onClick={() => {
                            queueHiddenByUserRef.current = true;
                            setDrawerOpen(false);
                            if (isNarrow) setNarrowPane('local');
                        }}>收起</button>
                    </div>
                    <div style={styles.queueProgressRow}>
                        <div style={styles.queueProgressBar} title={`总进度 ${queueStats.pct}%（完成 ${queueStats.done}/${queueStats.total}）`}>
                            <div style={{ ...styles.queueProgressFill, width: `${queueStats.pct}%` }} />
                        </div>
                        <span style={styles.queueProgressLabel}>{queueStats.total > 0 ? `${queueStats.done}/${queueStats.total} · ${queueStats.pct}%` : ''}</span>
                    </div>
                    <div style={{ ...styles.drawerBody, maxHeight: queueBodyHeight }}>
                        {taskList.length === 0 ? (
                            <div style={{ color: 'var(--text-muted)', fontSize: '12px' }}>暂无任务</div>
                        ) : (
                            taskList.map(t => {
                                const queued = t.status === 'running' && (t.step || '').startsWith('排队');
                                let stateText: string;
                                let stateColor: string;
                                if (t.status === 'done') {
                                    stateText = '✓ 完成';
                                    stateColor = 'var(--success)';
                                } else if (t.status === 'error') {
                                    stateText = '✗ 失败';
                                    stateColor = 'var(--severity-danger)';
                                } else if (t.status === 'cancelled') {
                                    stateText = '已取消';
                                    stateColor = 'var(--text-tertiary)';
                                } else if (queued) {
                                    stateText = '排队';
                                    stateColor = 'var(--text-tertiary)';
                                } else {
                                    stateText = '传输中';
                                    stateColor = 'var(--severity-info)';
                                }
                                // 第三列统一聚合：完成/失败显示消息，排队显示等待提示，传输中显示字节与速度
                                let detail = '';
                                if (t.status === 'done' || t.status === 'error' || t.status === 'cancelled') {
                                    detail = t.message || '';
                                } else if (queued) {
                                    detail = t.step || '';
                                } else if (t.bytesTotal > 0) {
                                    detail = `${formatFileSize(t.bytesDone)} / ${formatFileSize(t.bytesTotal)}`;
                                    if (t.speedBps > 0) detail += ` · ${formatSpeed(t.speedBps)}`;
                                }
                                return (
                                    <div key={t.taskId} style={styles.taskRow}>
                                        <div style={styles.taskId} title={t.name || t.taskId}>{t.name || t.taskId.slice(0, 8)}</div>
                                        <span style={{ ...styles.taskState, color: stateColor }}>{stateText}</span>
                                        <span style={styles.taskDetail} title={detail || undefined}>{detail}</span>
                                        {t.status === 'running' ? (
                                            <button style={{ ...styles.btnSecondary, padding: '2px 8px', fontSize: '11px' }} onClick={() => cancelTask(t.taskId)} disabled={loading}>
                                                取消
                                            </button>
                                        ) : <span style={styles.taskOpsPlaceholder} />}
                                    </div>
                                );
                            })
                        )}
                    </div>
                </div>
            ) : null}

            {nameDialog ? (
                <NameDialog
                    title={nameDialog.title}
                    defaultValue={nameDialog.defaultValue}
                    onConfirm={nameDialog.onConfirm}
                    onCancel={nameDialog.onCancel}
                />
            ) : null}

            {ctxMenu ? (
                <FileContextMenu
                    x={ctxMenu.x}
                    y={ctxMenu.y}
                    items={ctxMenu.items}
                    onClose={() => setCtxMenu(null)}
                />
            ) : null}

            {editOpen ? (
                <div style={styles.modalOverlay}>
                    <div style={styles.modal}>
                        <div style={styles.modalHeader}>
                            <div style={{ color: 'var(--text-primary)', fontSize: '13px', fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{editPath}</div>
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
        color: 'var(--text-secondary)',
        display: 'flex',
        flexDirection: 'column',
        gap: '8px',
        height: '100%',
        minHeight: 0,
        overflow: 'hidden',
        backgroundColor: 'var(--bg-secondary)'
    },
    rootNarrow: {
        padding: '8px',
        gap: '8px'
    },
    topBar: {
        display: 'flex',
        gap: '8px',
        alignItems: 'center',
        flexShrink: 0,
        padding: '7px 8px',
        border: '1px solid var(--border-subtle)',
        borderRadius: '6px',
        backgroundColor: 'var(--bg-tertiary)',
        boxShadow: 'inset 0 1px 0 rgba(255,255,255,0.03)'
    },
    segmented: {
        display: 'flex',
        border: '1px solid var(--border)',
        borderRadius: '6px',
        overflow: 'hidden',
        flexShrink: 0
    },
    segmentedButton: {
        flex: 1,
        padding: '6px 8px',
        border: 'none',
        borderRight: '1px solid var(--border)',
        backgroundColor: 'var(--bg-primary)',
        color: 'var(--text-tertiary)',
        cursor: 'pointer',
        fontSize: '12px'
    },
    segmentedActive: {
        flex: 1,
        padding: '6px 8px',
        border: 'none',
        borderRight: '1px solid var(--border)',
        backgroundColor: 'var(--bg-active)',
        color: 'var(--text-on-accent)',
        cursor: 'pointer',
        fontSize: '12px',
        fontWeight: 600
    },
    select: {
        padding: '5px 8px',
        borderRadius: '4px',
        border: '1px solid var(--border)',
        backgroundColor: 'var(--bg-primary)',
        color: 'var(--text-primary)',
        outline: 'none',
        minWidth: '180px',
        maxWidth: '100%',
        height: '28px',
        fontSize: '12px'
    },
    badge: {
        padding: '2px 7px',
        borderRadius: '999px',
        border: '1px solid var(--border-subtle)',
        backgroundColor: 'var(--bg-secondary)',
        color: 'var(--text-secondary)',
        fontSize: '11px',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
        maxWidth: '260px'
    },
    badgeMuted: {
        padding: '4px 8px',
        borderRadius: '999px',
        border: '1px solid var(--border)',
        backgroundColor: 'var(--bg-primary)',
        color: 'var(--text-muted)',
        fontSize: '12px'
    },
    btn: {
        padding: '5px 10px',
        borderRadius: '4px',
        border: '1px solid var(--accent-hover)',
        backgroundColor: 'var(--accent)',
        color: 'var(--text-on-accent)',
        cursor: 'pointer',
        fontSize: '11px',
        minWidth: '58px',
        height: '28px',
        fontWeight: 600
    },
    btnSecondary: {
        padding: '5px 9px',
        borderRadius: '4px',
        border: '1px solid var(--border-subtle)',
        backgroundColor: 'var(--bg-primary)',
        color: 'var(--text-secondary)',
        cursor: 'pointer',
        fontSize: '11px',
        height: '28px'
    },
    btnDanger: {
        padding: '5px 9px',
        borderRadius: '4px',
        border: '1px solid var(--danger-border)',
        backgroundColor: 'var(--danger-bg-subtle)',
        color: 'var(--severity-danger)',
        cursor: 'pointer',
        fontSize: '11px',
        height: '28px'
    },
    iconBtn: {
        padding: '4px 7px',
        borderRadius: '4px',
        border: '1px solid var(--border-subtle)',
        backgroundColor: 'var(--bg-primary)',
        color: 'var(--text-secondary)',
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
        border: '1px solid var(--border-subtle)',
        borderRadius: '6px',
        overflow: 'hidden',
        display: 'flex',
        flexDirection: 'column',
        minHeight: 0,
        minWidth: 0,
        backgroundColor: 'var(--bg-primary)',
        boxShadow: '0 1px 0 rgba(255,255,255,0.03)'
    },
    paneTitle: {
        color: 'var(--text-primary)',
        fontSize: '12px',
        fontWeight: 700,
        letterSpacing: 0,
        flexShrink: 0,
        whiteSpace: 'nowrap' as const
    },
    paneHeaderRow: {
        padding: '5px 8px',
        borderBottom: '1px solid var(--border-subtle)',
        backgroundColor: 'var(--bg-tertiary)',
        display: 'flex',
        alignItems: 'center',
        gap: '4px',
        minWidth: 0,
        overflow: 'hidden'
    },
    pathBar: {
        padding: '6px 8px',
        borderBottom: '1px solid var(--border-subtle)',
        display: 'flex',
        gap: '6px',
        alignItems: 'center',
        backgroundColor: 'var(--bg-secondary)',
        flexWrap: 'wrap' as const
    },
    pathInput: {
        flex: 1,
        padding: '4px 8px',
        borderRadius: '4px',
        border: '1px solid var(--border)',
        backgroundColor: 'var(--bg-primary)',
        color: 'var(--text-primary)',
        outline: 'none',
        fontSize: '11px',
        height: '28px',
        boxSizing: 'border-box' as const
    },
    breadcrumbItem: {
        padding: '2px 6px',
        borderRadius: '4px',
        border: 'none',
        backgroundColor: 'transparent',
        color: 'var(--text-secondary)',
        cursor: 'pointer',
        fontSize: '11px',
        whiteSpace: 'nowrap' as const,
        flexShrink: 0,
    },
    breadcrumbSep: {
        color: 'var(--text-muted)',
        fontSize: '11px',
        userSelect: 'none' as const,
        flexShrink: 0,
    },
    filterBar: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        padding: '5px 8px',
        borderBottom: '1px solid var(--border-subtle)',
        backgroundColor: 'var(--bg-secondary)',
    },
    filterInput: {
        flex: 1,
        padding: '4px 8px',
        borderRadius: '4px',
        border: '1px solid var(--border-subtle)',
        backgroundColor: 'var(--bg-primary)',
        color: 'var(--text-primary)',
        outline: 'none',
        fontSize: '11px',
        height: '24px',
        boxSizing: 'border-box' as const,
    },
    hiddenToggle: {
        display: 'flex',
        alignItems: 'center',
        gap: '4px',
        color: 'var(--text-muted)',
        fontSize: '11px',
        whiteSpace: 'nowrap' as const,
        cursor: 'pointer',
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
        // 预留滚动条空间，避免 sticky 表头覆盖右侧垂直滚动条
        scrollbarGutter: 'stable' as const,
        backgroundColor: 'var(--bg-primary)'
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
        color: 'var(--text-muted)',
        padding: '0 8px',
        position: 'sticky' as const,
        top: 0,
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        userSelect: 'none' as const,
        backgroundColor: 'var(--bg-tertiary)',
        borderBottom: '1px solid var(--border-subtle)',
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
        color: 'var(--text-secondary)',
        padding: '5px 8px',
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        height: '28px',
        borderBottom: '1px solid var(--border-subtle)',
        boxSizing: 'border-box' as const
    },
    fileRow: {
        backgroundColor: 'transparent'
    },
    fileRowSelected: {
        backgroundColor: 'var(--bg-active-soft)',
        boxShadow: 'inset 3px 0 0 var(--accent)'
    },
    cellName: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        minWidth: 0
    },
    cellCheck: {
        textAlign: 'center' as const,
        padding: '0 4px',
        width: 32,
    },
    cellOwner: {
        color: 'var(--text-secondary)'
    },
    cellSize: {
        textAlign: 'right' as const,
        color: 'var(--text-secondary)'
    },
    cellTime: {
        color: 'var(--text-tertiary)'
    },
    colResizeHandle: {
        position: 'absolute' as const,
        top: 0,
        right: 0,
        width: '10px',
        height: '100%',
        cursor: 'col-resize',
        borderRight: '1px solid var(--border-subtle)',
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
        borderBottom: '1px solid var(--border-subtle)',
        padding: '7px 8px',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '3px',
        backgroundColor: 'transparent'
    },
    compactRowSelected: {
        backgroundColor: 'var(--bg-active-soft)',
        boxShadow: 'inset 3px 0 0 var(--accent)'
    },
    compactMain: {
        display: 'flex',
        alignItems: 'center',
        gap: '7px',
        minWidth: 0
    },
    compactCheck: {
        display: 'inline-flex',
        alignItems: 'center',
        flexShrink: 0,
        cursor: 'pointer',
    },
    compactName: {
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
        minWidth: 0,
        color: 'var(--text-secondary)',
        fontSize: '12px'
    },
    compactMeta: {
        color: 'var(--text-tertiary)',
        fontSize: '10px',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
        paddingLeft: '30px'
    },
    emptyState: {
        color: 'var(--text-muted)',
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
        border: '1px solid var(--border-subtle)',
        backgroundColor: 'var(--bg-primary)'
    },
    fileIconFolder: {
        color: 'var(--icon-folder-fg)',
        backgroundColor: 'var(--icon-folder-bg)',
        borderColor: 'var(--icon-folder-border)'
    },
    fileIconFile: {
        color: 'var(--text-tertiary)'
    },
    fileIconCode: {
        color: 'var(--icon-code-fg)',
        backgroundColor: 'var(--icon-code-bg)',
        borderColor: 'var(--icon-code-border)'
    },
    fileIconArchive: {
        color: 'var(--icon-archive-fg)',
        backgroundColor: 'var(--icon-archive-bg)',
        borderColor: 'var(--icon-archive-border)'
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
        border: '1px dashed var(--drop-ready-border)',
        backgroundColor: 'var(--drop-ready-bg)'
    },
    dropOverlayBlocked: {
        border: '1px dashed var(--drop-blocked-border)',
        backgroundColor: 'var(--drop-blocked-bg)'
    },
    dropOverlayTitle: {
        color: 'var(--text-primary)',
        fontSize: '14px',
        fontWeight: 700
    },
    dropOverlayDetail: {
        color: 'var(--text-secondary)',
        fontSize: '12px',
        maxWidth: '80%',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap'
    },
    scpPane: {
        flex: 1,
        border: '1px solid var(--border)',
        borderRadius: '8px',
        overflow: 'hidden',
        display: 'flex',
        flexDirection: 'column' as const,
        minHeight: 0,
        position: 'relative' as const
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
        border: '1px solid var(--border)',
        borderRadius: '8px',
        padding: '12px 12px',
        backgroundColor: 'var(--bg-primary)',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '10px'
    },
    scpLabel: {
        color: 'var(--text-tertiary)',
        fontSize: '12px'
    },
    scpHint: {
        color: 'var(--text-muted)',
        fontSize: '12px'
    },
    drawer: {
        border: '1px solid var(--border)',
        borderRadius: '8px',
        overflow: 'hidden'
    },
    queueResizeHandle: {
        height: '6px',
        cursor: 'ns-resize',
        backgroundColor: 'var(--bg-secondary)',
        borderBottom: '1px solid var(--border)'
    },
    queueSummaryText: {
        display: 'flex',
        gap: '10px',
        alignItems: 'center',
        color: 'var(--text-tertiary)',
        fontSize: '11px',
        whiteSpace: 'nowrap',
        overflow: 'hidden'
    },
    queueProgressRow: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        padding: '4px 12px 6px',
        backgroundColor: 'var(--bg-secondary)',
        borderBottom: '1px solid var(--border)'
    },
    queueProgressLabel: {
        color: 'var(--text-tertiary)',
        fontSize: '11px',
        whiteSpace: 'nowrap'
    },
    queueProgressBar: {
        flex: '1 1 auto',
        height: '6px',
        borderRadius: '3px',
        backgroundColor: 'var(--bg-primary)',
        border: '1px solid var(--border)',
        overflow: 'hidden'
    },
    queueProgressFill: {
        height: '100%',
        borderRadius: '2px',
        backgroundColor: 'var(--success)',
        transition: 'width 0.2s ease'
    },
    drawerHeader: {
        padding: '10px 12px',
        backgroundColor: 'var(--bg-secondary)',
        borderBottom: '1px solid var(--border)',
        display: 'flex',
        alignItems: 'center',
        gap: '10px'
    },
    drawerBody: {
        padding: '10px 12px',
        display: 'flex',
        flexDirection: 'column',
        gap: '6px',
        // 内容高度由拖拽手柄控制（内联 maxHeight 覆盖）
        overflowY: 'auto'
    },
    taskRow: {
        // 四列网格保证多行之间的列对齐：文件名 | 状态 | 详情 | 操作
        display: 'grid',
        gridTemplateColumns: 'minmax(100px, 1.1fr) 56px minmax(120px, 1fr) 48px',
        gap: '8px',
        alignItems: 'center'
    },
    taskId: {
        color: 'var(--text-secondary)',
        fontSize: '12px',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap'
    },
    taskState: {
        fontSize: '11px',
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis'
    },
    taskDetail: {
        color: 'var(--text-tertiary)',
        fontSize: '11px',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap'
    },
    taskOpsPlaceholder: {
        width: '48px'
    },
    modalOverlay: {
        position: 'fixed' as const,
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'var(--overlay)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 3000
    },
    modal: {
        width: '720px',
        height: '520px',
        backgroundColor: 'var(--bg-secondary)',
        borderRadius: '8px',
        border: '1px solid var(--border)',
        display: 'flex',
        flexDirection: 'column' as const,
        overflow: 'hidden'
    },
    modalHeader: {
        padding: '10px 12px',
        borderBottom: '1px solid var(--border)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: '10px',
        backgroundColor: 'var(--bg-primary)'
    },
    modalBody: {
        flex: 1,
        padding: '10px 12px'
    },
    modalFooter: {
        padding: '10px 12px',
        borderTop: '1px solid var(--border)',
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '10px',
        backgroundColor: 'var(--bg-primary)'
    },
    textarea: {
        width: '100%',
        height: '100%',
        padding: '10px 12px',
        borderRadius: '6px',
        border: '1px solid var(--border)',
        backgroundColor: 'var(--bg-primary)',
        color: 'var(--text-primary)',
        outline: 'none',
        fontFamily: 'var(--font-mono)',
        fontSize: '12px',
        resize: 'none' as const
    },
    protocolChip: {
        display: 'inline-flex',
        alignItems: 'center',
        padding: '3px 10px',
        border: '1px solid var(--accent)',
        borderRadius: '999px',
        backgroundColor: 'var(--bg-active-soft)',
        color: 'var(--accent)',
        fontSize: '11px',
        whiteSpace: 'nowrap' as const,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        maxWidth: '260px',
    },
    protocolChipMuted: {
        padding: '3px 10px',
        border: '1px solid var(--border-subtle)',
        borderRadius: '999px',
        backgroundColor: 'var(--bg-secondary)',
        color: 'var(--text-disabled)',
        fontSize: '11px',
    },
    relayBanner: {
        padding: '8px 10px',
        borderRadius: '6px',
        border: '1px solid var(--warning-tint-border)',
        backgroundColor: 'var(--warning-bg-subtle)',
        color: 'var(--severity-warning)',
        fontSize: '11px',
        lineHeight: '1.6'
    }
};

export default FilesPanel;
