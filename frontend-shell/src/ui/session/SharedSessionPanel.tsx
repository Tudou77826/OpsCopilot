import React, { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { TbUsers, TbRefresh, TbTerminal2, TbLock, TbSearch, TbChevronDown, TbChevronUp } from 'react-icons/tb';
import { useToast } from '../feedback/Toast';
import { confirmDialog } from '../feedback/ConfirmDialog';
import { ConnectionConfig } from '../types';
import { SharedSessionEntry, SharedSessionRuntime } from '../ports';

// 经由注入的 SharedSessionRuntime 访问宿主（Wails 提供；Sidecar 不提供时组件返回 null）

interface SharedSessionPanelProps {
    // 统一连接流程（宿主 handleConnect）：开终端 tab、状态栏、错误弹窗
    onConnect: (config: ConnectionConfig) => void;
    // 受会话管理顶部的搜索框控制（与上方会话树一致）
    searchTerm?: string;
    // 团队共享会话宿主能力（Wails 专有）。Sidecar 不提供时组件不渲染。
    runtime?: SharedSessionRuntime | null;
}

// 面板高度/折叠偏好持久化（与 Sidebar 宽度同类的本地 UI 偏好）
const PANEL_HEIGHT_KEY = 'opscopilot:sharedPanelHeight';
const PANEL_COLLAPSED_KEY = 'opscopilot:sharedPanelCollapsed';
const DEFAULT_PANEL_HEIGHT = 220;
const MIN_PANEL_HEIGHT = 110;
const MIN_TREE_HEIGHT = 140; // 拖到最大时为上方会话树保留的最小高度

function loadPanelHeight(): number {
    const raw = Number(localStorage.getItem(PANEL_HEIGHT_KEY));
    return raw >= MIN_PANEL_HEIGHT ? raw : DEFAULT_PANEL_HEIGHT;
}

function loadPanelCollapsed(): boolean {
    return localStorage.getItem(PANEL_COLLAPSED_KEY) === '1';
}

// relativeTime 将后端的 "2006-01-02 15:04:05" 本地时间转为相对时间描述。
function relativeTime(lastLoginAt: string): string {
    if (!lastLoginAt) return '';
    // 后端按本地时区格式化，此处按本地时区解析
    const t = new Date(lastLoginAt.replace(' ', 'T'));
    if (isNaN(t.getTime())) return lastLoginAt;
    const diffMs = Date.now() - t.getTime();
    const diffMin = Math.floor(diffMs / 60000);
    if (diffMin < 1) return '刚刚';
    if (diffMin < 60) return `${diffMin} 分钟前`;
    const diffHour = Math.floor(diffMin / 60);
    if (diffHour < 24) return `${diffHour} 小时前`;
    const diffDay = Math.floor(diffHour / 24);
    if (diffDay < 30) return `${diffDay} 天前`;
    return t.toLocaleDateString();
}

const SharedSessionPanel: React.FC<SharedSessionPanelProps> = ({ onConnect, searchTerm, runtime }) => {
    const toast = useToast();
    const [enabled, setEnabled] = useState(false);
    const [entries, setEntries] = useState<SharedSessionEntry[]>([]);
    const [loading, setLoading] = useState(false);
    const [contextMenu, setContextMenu] = useState<{ x: number; y: number; entry: SharedSessionEntry } | null>(null);
    const [hoveredKey, setHoveredKey] = useState<string | null>(null);
    // 面板高度（px）与折叠状态：本地持久化，重启保持
    const [height, setHeight] = useState<number>(loadPanelHeight);
    const [collapsed, setCollapsed] = useState<boolean>(loadPanelCollapsed);
    const panelRef = useRef<HTMLDivElement>(null);

    // 拖拽上边缘调整高度：面板底部锚定，向上拖增大。
    // 与 Sidebar 宽度拖拽同模式（document 级 mousemove/mouseup + body 光标）。
    // heightRef 每次渲染同步最新高度，供拖拽结束时持久化。
    const heightRef = useRef(height);
    heightRef.current = height;

    const startPanelResize = (mouseDownEvent: React.MouseEvent) => {
        mouseDownEvent.preventDefault();
        const container = panelRef.current?.parentElement;
        if (!container) return;
        const containerRect = container.getBoundingClientRect();
        const maxHeight = Math.max(MIN_PANEL_HEIGHT, containerRect.height - MIN_TREE_HEIGHT);

        const doDrag = (mouseMoveEvent: MouseEvent) => {
            const newHeight = containerRect.bottom - mouseMoveEvent.clientY;
            // 钳制到 [min, max]：越界时贴边跟随，而非忽略
            setHeight(Math.min(Math.max(newHeight, MIN_PANEL_HEIGHT), maxHeight));
        };
        const stopDrag = () => {
            document.removeEventListener('mousemove', doDrag);
            document.removeEventListener('mouseup', stopDrag);
            document.body.style.cursor = 'default';
            try {
                localStorage.setItem(PANEL_HEIGHT_KEY, String(heightRef.current));
            } catch { /* ignore */ }
        };
        document.addEventListener('mousemove', doDrag);
        document.addEventListener('mouseup', stopDrag);
        document.body.style.cursor = 'ns-resize';
    };

    const toggleCollapsed = () => {
        setCollapsed(prev => {
            const next = !prev;
            try {
                localStorage.setItem(PANEL_COLLAPSED_KEY, next ? '1' : '0');
            } catch { /* ignore */ }
            return next;
        });
    };

    const load = useCallback(async () => {
        const list = runtime?.list;
        if (!list) return;
        try {
            const parsed = await list();
            setEnabled(true);
            setEntries(Array.isArray(parsed) ? parsed : []);
        } catch (e) {
            // 宿主未启用/无此能力时保持 enabled=false，组件隐藏
            console.error('Failed to load shared sessions:', e);
        }
    }, [runtime]);

    // 轮询 + 事件双通道刷新（对齐 SessionManager 的 5 秒轮询）
    useEffect(() => {
        if (!runtime?.list) return;
        load();
        const interval = setInterval(load, 5000);
        return () => clearInterval(interval);
    }, [load, runtime]);

    // 宿主推送的同步通知（Wails: session-share:synced）。无该能力时跳过。
    useEffect(() => {
        if (!runtime?.onSynced) return;
        return runtime.onSynced(() => { void load(); });
    }, [runtime, load]);

    // 点击面板其他区域关闭右键菜单
    useEffect(() => {
        if (!contextMenu) return;
        const handler = () => setContextMenu(null);
        document.addEventListener('pointerdown', handler);
        return () => document.removeEventListener('pointerdown', handler);
    }, [contextMenu]);

    // 与上方会话树共用同一个搜索词：匹配名称/主机/用户/共享者
    const term = (searchTerm || '').trim().toLowerCase();
    const displayed = useMemo(() => {
        if (!term) return entries;
        return entries.filter(e =>
            (e.name || '').toLowerCase().includes(term) ||
            e.host.toLowerCase().includes(term) ||
            (e.user || '').toLowerCase().includes(term) ||
            (e.owner || '').toLowerCase().includes(term)
        );
    }, [entries, term]);

    const handleConnect = async (entry: SharedSessionEntry) => {
        const connect = runtime?.connect;
        if (!connect) return;
        try {
            const result = await connect(entry.entryKey);
            if (!result || !result.success || !result.config) {
                toast.error(`共享会话连接失败: ${result?.message || '未知错误'}`);
                return;
            }
            // 走统一连接流程：开 tab、状态栏、失败弹窗都由宿主处理
            onConnect(result.config);
        } catch (e: any) {
            toast.error(`共享会话连接失败: ${e?.message || e}`);
        }
    };

    const handleSaveToLocal = async (entry: SharedSessionEntry) => {
        const save = runtime?.saveToLocal;
        if (!save) return;
        try {
            await save(entry.entryKey);
            toast.success(`已保存「${entry.name || entry.host}」到我的会话`);
        } catch (e: any) {
            toast.error(e?.message || e);
        }
    };

    const handleRemove = async (entry: SharedSessionEntry) => {
        const remove = runtime?.remove;
        if (!remove) return;
        const ok = await confirmDialog.show({ message: `确定删除共享的「${entry.name || entry.host}」吗？其他成员将不再看到该条目`, danger: true });
        if (!ok) return;
        try {
            await remove(entry.entryKey);
            toast.success('已删除共享条目');
        } catch (e: any) {
            toast.error(e?.message || e);
        }
        load();
    };

    // Sidecar 未提供共享会话能力时不渲染（宿主未提供则不显示入口）
    if (!runtime?.list || !enabled) return null;

    return (
        <div
            ref={panelRef}
            data-testid="shared-panel"
            style={{
                ...styles.container,
                height: collapsed ? 'auto' : `${height}px`,
                flexShrink: 0,
            }}
            onContextMenu={(e) => e.preventDefault()}
        >
            {/* 拖拽调高手柄（折叠时隐藏） */}
            {!collapsed && <div style={styles.resizeHandle} onMouseDown={startPanelResize} />}

            <div style={styles.header}>
                <span style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', color: 'var(--text-secondary)', minWidth: 0 }}>
                    {TbUsers({ size: 14 })}
                    <span style={styles.title}>团队共享</span>
                    {!collapsed && entries.length > 0 && (
                        <span style={styles.countBadge}>{entries.length}</span>
                    )}
                </span>
                <span style={{ display: 'inline-flex', alignItems: 'center', gap: '2px' }}>
                    {!collapsed && (
                        <button
                            style={styles.headerBtn}
                            title="刷新共享会话"
                            onClick={() => { setLoading(true); load().finally(() => setLoading(false)); }}
                            disabled={loading}
                        >
                            {TbRefresh({ size: 14 })}
                        </button>
                    )}
                    <button
                        style={styles.headerBtn}
                        title={collapsed ? '展开团队共享' : '折叠团队共享'}
                        onClick={toggleCollapsed}
                    >
                        {collapsed ? TbChevronUp({ size: 14 }) : TbChevronDown({ size: 14 })}
                    </button>
                </span>
            </div>

            {!collapsed && (
            <div style={styles.list}>
                {displayed.length === 0 && (
                    <div style={styles.empty}>
                        {term ? (
                            <span style={{ display: 'inline-flex', alignItems: 'center', gap: '4px' }}>
                                {TbSearch({ size: 12 })} 无匹配的共享会话
                            </span>
                        ) : '暂无共享会话'}
                    </div>
                )}
                {displayed.map(entry => (
                    <div
                        key={entry.entryKey}
                        style={{
                            ...styles.row,
                            backgroundColor: hoveredKey === entry.entryKey ? 'var(--bg-elevated)' : 'transparent',
                        }}
                        onMouseEnter={() => setHoveredKey(entry.entryKey)}
                        onMouseLeave={() => setHoveredKey(null)}
                        onDoubleClick={() => handleConnect(entry)}
                        onContextMenu={(e) => {
                            e.preventDefault();
                            setContextMenu({ x: e.clientX, y: e.clientY, entry });
                        }}
                        title={`${entry.user}@${entry.host}:${entry.port} · 共享者 ${entry.owner} · 最近登录 ${entry.lastLoginAt}${entry.hasSecrets && !entry.decryptable ? ' · 密钥不匹配，无法解密密码' : ''}`}
                    >
                        <span style={{ ...styles.icon, color: entry.hasSecrets && !entry.decryptable ? 'var(--warning)' : 'var(--text-muted)' }}>
                            {entry.hasSecrets && !entry.decryptable ? TbLock({ size: 14 }) : TbTerminal2({ size: 14 })}
                        </span>
                        <div style={styles.info}>
                            <div style={styles.nameLine}>
                                <span style={styles.name}>{entry.name || entry.host}</span>
                                {entry.protocol === 'telnet' && (
                                    <span style={protocolChipStyle}>Telnet</span>
                                )}
                            </div>
                            <div style={styles.meta}>
                                <span>{entry.user}@{entry.host}</span>
                                <span style={styles.owner}>· {entry.owner}</span>
                                <span style={styles.time}>· {relativeTime(entry.lastLoginAt)}</span>
                            </div>
                        </div>
                    </div>
                ))}
            </div>
            )}

            {contextMenu && (
                <div style={{ ...menuStyles.menu, top: contextMenu.y, left: contextMenu.x }} onPointerDown={(e) => e.stopPropagation()}>
                    <div style={menuStyles.item} onClick={() => { handleConnect(contextMenu.entry); setContextMenu(null); }}>
                        打开连接
                    </div>
                    <div style={menuStyles.item} onClick={() => { handleSaveToLocal(contextMenu.entry); setContextMenu(null); }}>
                        保存到我的会话
                    </div>
                    {contextMenu.entry.own && (
                        <div style={menuStyles.item} onClick={() => { handleRemove(contextMenu.entry); setContextMenu(null); }}>
                            删除共享
                        </div>
                    )}
                </div>
            )}
        </div>
    );
};

const styles = {
    container: {
        minHeight: 0,
        display: 'flex' as const,
        flexDirection: 'column' as const,
        borderTop: '1px solid var(--border)',
        backgroundColor: 'var(--bg-secondary)',
        overflow: 'hidden',
        position: 'relative' as const,
    },
    // 顶部拖拽调高手柄：热区略高于可见线条，hover 高亮
    resizeHandle: {
        height: '5px',
        margin: '-2px 0 -3px 0',
        cursor: 'ns-resize',
        flexShrink: 0,
        position: 'relative' as const,
    },
    header: {
        display: 'flex' as const,
        alignItems: 'center' as const,
        justifyContent: 'space-between' as const,
        padding: '6px 10px',
        borderBottom: '1px solid var(--border)',
        flexShrink: 0,
    },
    title: {
        fontSize: '12px',
        fontWeight: 600,
        userSelect: 'none' as const,
    },
    countBadge: {
        fontSize: '10px',
        lineHeight: '1.4',
        padding: '0 6px',
        color: 'var(--text-muted)',
        backgroundColor: 'var(--bg-primary)',
        border: '1px solid var(--border)',
        borderRadius: '999px',
        userSelect: 'none' as const,
        flexShrink: 0,
    },
    headerBtn: {
        display: 'inline-flex' as const,
        alignItems: 'center' as const,
        justifyContent: 'center' as const,
        padding: '2px',
        border: 'none',
        backgroundColor: 'transparent',
        color: 'var(--text-muted)',
        cursor: 'pointer',
        borderRadius: '4px',
    },
    list: {
        flex: 1,
        overflowY: 'auto' as const,
        minHeight: 0,
        padding: '4px 0',
    },
    row: {
        display: 'flex' as const,
        alignItems: 'center' as const,
        padding: '4px 10px',
        cursor: 'pointer',
    },
    icon: {
        marginRight: '8px',
        display: 'inline-flex' as const,
        flexShrink: 0,
    },
    info: {
        display: 'flex' as const,
        flexDirection: 'column' as const,
        minWidth: 0,
    },
    nameLine: {
        display: 'flex' as const,
        alignItems: 'center' as const,
        gap: '6px',
    },
    name: {
        fontSize: '13px',
        color: 'var(--text-secondary)',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap' as const,
        userSelect: 'none' as const,
    },
    meta: {
        fontSize: '11px',
        color: 'var(--text-disabled)',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap' as const,
        userSelect: 'none' as const,
    },
    owner: {
        color: 'var(--text-muted)',
    },
    time: {
        color: 'var(--accent)',
    },
    empty: {
        textAlign: 'center' as const,
        color: 'var(--text-disabled)',
        fontSize: '12px',
        padding: '10px 0',
    },
};

const menuStyles = {
    menu: {
        position: 'fixed' as const,
        backgroundColor: 'var(--bg-secondary)',
        border: '1px solid var(--border)',
        boxShadow: '0 2px 8px rgba(0,0,0,0.5)',
        borderRadius: '4px',
        zIndex: 1000,
        minWidth: '130px',
        padding: '4px 0',
    },
    item: {
        padding: '6px 12px',
        cursor: 'pointer',
        fontSize: '13px',
        color: 'var(--text-secondary)',
    },
};

// 协议 chip：与 SessionManager 的 protocolChipStyle 保持一致
const protocolChipStyle: React.CSSProperties = {
    display: 'inline-block',
    padding: '0 6px',
    fontSize: '10px',
    lineHeight: '1.5',
    color: 'var(--stage-orange)',
    backgroundColor: 'var(--bg-primary)',
    border: '1px solid var(--border)',
    borderRadius: '999px',
    userSelect: 'none',
    flexShrink: 0,
};

export default SharedSessionPanel;
