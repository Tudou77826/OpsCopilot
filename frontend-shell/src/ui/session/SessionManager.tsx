import React, { useState, useEffect, useRef } from 'react';
import { TbFolderOpen, TbFolder, TbTerminal2 } from 'react-icons/tb';
import { ConnectionConfig, normalizeProtocol, PROTOCOL_LABEL } from '../types';
import { SessionManagerRuntime, SharedSessionRuntime, SessionNode } from '../ports';
import EditSavedSessionModal from './EditSavedSessionModal';
import SharedSessionPanel from './SharedSessionPanel';
import { confirmDialog } from '../feedback/ConfirmDialog';

interface SessionManagerProps {
    onConnect: (config: ConnectionConfig) => void;
    runtime: SessionManagerRuntime;
    /** 团队共享会话宿主能力（Wails 专有，可选）。Sidecar 不提供时不显示。 */
    sharedRuntime?: SharedSessionRuntime | null;
}

const SessionManager: React.FC<SessionManagerProps> = ({ onConnect, runtime, sharedRuntime }) => {
    const [sessions, setSessions] = useState<SessionNode[]>([]);
    const [searchTerm, setSearchTerm] = useState('');
    const [expandedFolders, setExpandedFolders] = useState<Set<string>>(new Set());
    const [contextMenu, setContextMenu] = useState<{ x: number; y: number; node: SessionNode | null } | null>(null);
    const [editingNodeId, setEditingNodeId] = useState<string | null>(null);
    const [editName, setEditName] = useState('');
    const [hoveredNodeId, setHoveredNodeId] = useState<string | null>(null);
    const [hoveredMenuItem, setHoveredMenuItem] = useState<string | null>(null);
    const [editingSession, setEditingSession] = useState<{ id: string; config: ConnectionConfig } | null>(null);
    const [dragOverFolderId, setDragOverFolderId] = useState<string | null>(null);
    const dragNodeIdRef = useRef<string | null>(null);
    const autoExpandTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

    useEffect(() => {
        loadSessions();
        const interval = setInterval(loadSessions, 5000);
        return () => clearInterval(interval);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [runtime]);

    // Close context menu on any outside pointer event (including xterm.js terminals)
    useEffect(() => {
        if (!contextMenu) return;
        const handler = (e: PointerEvent) => {
            const menuEl = document.querySelector('[data-session-context-menu]');
            if (menuEl && menuEl.contains(e.target as Node)) return;
            setContextMenu(null);
        };
        document.addEventListener('pointerdown', handler);
        return () => document.removeEventListener('pointerdown', handler);
    }, [contextMenu]);

    // Cleanup drag timer on unmount
    useEffect(() => {
        return () => {
            if (autoExpandTimerRef.current) clearTimeout(autoExpandTimerRef.current);
        };
    }, []);

    // Find a session by ID anywhere in the tree
    const findSessionById = (nodes: SessionNode[], id: string): SessionNode | undefined => {
        for (const n of nodes) {
            if (n.id === id) return n;
            if (n.children) { const f = findSessionById(n.children, id); if (f) return f; }
        }
        return undefined;
    };

    // Drag a session into a folder
    const handleDragStart = (e: React.DragEvent, node: SessionNode) => {
        if (node.type !== 'session') return;
        e.dataTransfer.setData('text/plain', node.id);
        e.dataTransfer.effectAllowed = 'move';
        dragNodeIdRef.current = node.id;
    };

    const handleDragOver = (e: React.DragEvent, node: SessionNode) => {
        if (node.type !== 'folder') return;
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        setDragOverFolderId(node.id);

        // Auto-expand after hovering 600ms
        if (autoExpandTimerRef.current) clearTimeout(autoExpandTimerRef.current);
        if (!expandedFolders.has(node.id)) {
            autoExpandTimerRef.current = setTimeout(() => {
                setExpandedFolders(prev => new Set(prev).add(node.id));
            }, 600);
        }
    };

    const handleDragLeave = (e: React.DragEvent) => {
        const related = e.relatedTarget as HTMLElement | null;
        if (related && e.currentTarget.contains(related)) return;
        if (autoExpandTimerRef.current) {
            clearTimeout(autoExpandTimerRef.current);
            autoExpandTimerRef.current = null;
        }
        setDragOverFolderId(null);
    };

    const handleDrop = async (e: React.DragEvent, folder: SessionNode) => {
        e.preventDefault();
        // 文件夹行已消化本次 drop,必须阻断冒泡:树容器兜底的 handleTreeDrop 语义是
        // "移出分组",若放行会在本处理器之后再次提交 updateSession(group=''),
        // 把刚移入文件夹的会话又弹回根目录(见 Issue #70)。
        e.stopPropagation();
        if (autoExpandTimerRef.current) {
            clearTimeout(autoExpandTimerRef.current);
            autoExpandTimerRef.current = null;
        }
        setDragOverFolderId(null);

        const sessionId = e.dataTransfer.getData('text/plain');
        if (!sessionId) return;

        const session = findSessionById(sessions, sessionId);
        if (!session?.config) return;
        if (session.config.group === folder.name) return;

        const updatedConfig = { ...session.config, group: folder.name };
        try {
            await runtime.updateSession(session.id, updatedConfig, folder.name);
        } catch (err) {
            console.error("Failed to move session:", err);
        }
        loadSessions();
    };

    const handleDragEnd = () => {
        dragNodeIdRef.current = null;
        setDragOverFolderId(null);
        if (autoExpandTimerRef.current) {
            clearTimeout(autoExpandTimerRef.current);
            autoExpandTimerRef.current = null;
        }
    };

    // Drop on tree container blank area = remove group (move to root)
    const handleTreeDrop = async (e: React.DragEvent) => {
        e.preventDefault();
        // 只响应真正落在容器空白处的 drop(按下目标即容器本身);
        // 落在会话行等其他子元素上的冒泡 drop 不做任何事,避免误触发"移出分组"。
        if (e.target !== e.currentTarget) return;
        const sessionId = e.dataTransfer.getData('text/plain');
        if (!sessionId) return;
        const session = findSessionById(sessions, sessionId);
        if (!session?.config || !session.config.group) return;
        const updatedConfig = { ...session.config };
        delete (updatedConfig as any).group;
        try {
            await runtime.updateSession(session.id, updatedConfig, '');
        } catch (err) {
            console.error("Failed to move session:", err);
        }
        loadSessions();
    };

    const loadSessions = async () => {
        try {
            const data = await runtime.listSessions();

            // Helper to normalize config keys (snake_case to camelCase)
            const normalizeConfig = (cfg: any): ConnectionConfig => {
                if (!cfg) return cfg;
                return {
                    ...cfg,
                    rootPassword: cfg.rootPassword || cfg.root_password, // Map root_password to rootPassword
                    bastion: cfg.bastion ? normalizeConfig(cfg.bastion) : undefined
                };
            };

            // Recursive helper to process nodes
            const processNode = (node: any): SessionNode => {
                return {
                    ...node,
                    config: node.config ? normalizeConfig(node.config) : undefined,
                    children: node.children ? node.children.map(processNode) : undefined
                };
            };

            // Helper to sort sessions (folders first, then sessions, both alphabetically)
            const sortSessions = (nodes: SessionNode[]): SessionNode[] => {
                // Separate folders and sessions
                const folders = nodes.filter(node => node.type === 'folder');
                const sessions = nodes.filter(node => node.type === 'session');

                // Sort folders alphabetically
                folders.sort((a, b) => a.name.localeCompare(b.name, 'zh-CN', { sensitivity: 'base' }));

                // Sort sessions alphabetically
                sessions.sort((a, b) => a.name.localeCompare(b.name, 'zh-CN', { sensitivity: 'base' }));

                // Recursively sort children in folders
                const sortedFolders = folders.map(folder => ({
                    ...folder,
                    children: folder.children ? sortSessions(folder.children) : undefined
                }));

                // Return folders first, then sessions
                return [...sortedFolders, ...sessions];
            };

            const processedData = data ? data.map(processNode) : [];
            const sortedData = sortSessions(processedData);
            setSessions(sortedData);
        } catch (e) {
            console.error("Failed to load sessions:", e);
        }
    };

    const handleToggleFolder = (id: string) => {
        const newSet = new Set(expandedFolders);
        if (newSet.has(id)) {
            newSet.delete(id);
        } else {
            newSet.add(id);
        }
        setExpandedFolders(newSet);
    };

    const handleContextMenu = (e: React.MouseEvent, node?: SessionNode | null) => {
        e.preventDefault();
        e.stopPropagation();
        setContextMenu({ x: e.clientX, y: e.clientY, node: node ?? null });
    };

    const handleRename = async (id: string, newName: string) => {
        if (!newName.trim()) return;
        await runtime.renameSession(id, newName);
        loadSessions();
        setEditingNodeId(null);
    };

    const handleDelete = async (id: string) => {
        const ok = await confirmDialog.show({ message: '确定要删除吗？', danger: true });
        if (ok) {
            await runtime.deleteSession(id);
            loadSessions();
        }
    };

    const handleUnGroup = async (node: SessionNode) => {
        if (!node.config) return;
        const next: ConnectionConfig = { ...node.config };
        delete (next as any).group;
        await runtime.updateSession(node.id, next, '');
        loadSessions();
    };

    const collectSessions = (node: SessionNode): SessionNode[] => {
        if (node.type === 'session' && node.config) return [node];
        if (node.type === 'folder' && node.children) {
            return node.children.flatMap(collectSessions);
        }
        return [];
    };

    const handleConnectAll = async (folder: SessionNode) => {
        const all = collectSessions(folder);
        const max = 5;
        if (all.length === 0) return;
        const toConnect = all.slice(0, max);
        if (all.length > max) {
            const ok = await confirmDialog.show({ message: `该文件夹共有 ${all.length} 个会话，最多同时连接 ${max} 个，是否继续？` });
            if (!ok) return;
        }
        toConnect.forEach(s => { if (s.config) onConnect(s.config); });
    };

    // Recursive render
    const renderTree = (nodes: SessionNode[], level: number = 0) => {
        if (!nodes) return null;

        return nodes.map(node => {
            const isFolder = node.type === 'folder';
            const isExpanded = expandedFolders.has(node.id);
            const isEditing = editingNodeId === node.id;
            const isHovered = hoveredNodeId === node.id;

            const paddingLeft = `${level * 20 + 10}px`;

            const isDragOver = isFolder && dragOverFolderId === node.id;

            return (
                <div key={node.id}>
                    <div
                        style={{
                            ...styles.nodeRow,
                            paddingLeft,
                            backgroundColor: isDragOver ? 'var(--bg-active)' : (isHovered ? 'var(--bg-elevated)' : 'transparent'),
                            outline: isDragOver ? '1px dashed var(--accent)' : 'none',
                            outlineOffset: '-1px',
                        }}
                        draggable={!isFolder}
                        onDragStart={(e) => handleDragStart(e, node)}
                        onDragOver={(e) => handleDragOver(e, node)}
                        onDragLeave={isFolder ? handleDragLeave : undefined}
                        onDrop={(e) => isFolder ? handleDrop(e, node) : undefined}
                        onDragEnd={handleDragEnd}
                        onMouseEnter={() => setHoveredNodeId(node.id)}
                        onMouseLeave={() => setHoveredNodeId(null)}
                        onContextMenu={(e) => handleContextMenu(e, node)}
                        onClick={() => isFolder ? handleToggleFolder(node.id) : null}
                        onDoubleClick={() => !isFolder && node.config && onConnect(node.config)}
                    >
                        <span style={{marginRight: '8px', userSelect: 'none', display: 'inline-flex', alignItems: 'center', color: isFolder ? 'var(--icon-folder-fg)' : 'var(--text-muted)'}}>{isFolder ? (isExpanded ? TbFolderOpen({size: 16}) : TbFolder({size: 16})) : TbTerminal2({size: 16})}</span>

                        {isEditing ? (
                            <input
                                autoFocus
                                value={editName}
                                onChange={e => setEditName(e.target.value)}
                                onBlur={() => handleRename(node.id, editName)}
                                onKeyDown={e => {
                                    if (e.key === 'Enter') handleRename(node.id, editName);
                                    if (e.key === 'Escape') setEditingNodeId(null);
                                }}
                                onClick={e => e.stopPropagation()}
                                style={styles.renameInput}
                            />
                        ) : (
                            <>
                                <span style={{
                                    ...styles.nodeName,
                                    fontWeight: isFolder ? 600 : 400,
                                    color: isFolder ? 'var(--text-primary)' : 'var(--text-secondary)',
                                }}>{node.name}</span>
                                {/* 协议 chip:仅非默认协议(telnet)显示,避免全 SSH 环境噪声 */}
                                {!isFolder && normalizeProtocol(node.config?.protocol) === 'telnet' && (
                                    <span style={protocolChipStyle}>{PROTOCOL_LABEL.telnet}</span>
                                )}
                            </>
                        )}
                    </div>
                    {isFolder && isExpanded && node.children && (
                        <div>{renderTree(node.children, level + 1)}</div>
                    )}
                </div>
            );
        });
    };

    // Filter Logic
    // 文件夹名命中搜索词时,展示该文件夹下全部子节点;
    // 文件夹名未命中但子节点命中时,仅保留命中的子节点。
    const filterNodes = (nodes: SessionNode[], term: string): SessionNode[] => {
        if (!term) return nodes;
        const lowerTerm = term.toLowerCase();

        return nodes.reduce<SessionNode[]>((acc, node) => {
            const matches = node.name.toLowerCase().includes(lowerTerm) ||
                           (node.config && node.config.host.includes(lowerTerm));

            if (node.type === 'folder') {
                if (matches) {
                    acc.push({ ...node });
                } else {
                    const filteredChildren = filterNodes(node.children || [], term);
                    if (filteredChildren.length > 0) {
                        acc.push({ ...node, children: filteredChildren });
                    }
                }
            } else {
                if (matches) acc.push(node);
            }
            return acc;
        }, []);
    };

    // Auto-expand effect when searching
    // 仅依赖 searchTerm:只在用户改动搜索词时展开一次匹配的文件夹。
    // 不依赖 sessions——loadSessions 每 5 秒轮询会替换 sessions,若把它放进依赖,
    // 会在每次定时刷新时重新展开所有匹配项,把用户刚刚手动折叠的文件夹再次撑开。
    useEffect(() => {
        if (searchTerm) {
            const expandRecursive = (nodes: SessionNode[]) => {
                nodes.forEach(node => {
                    if (node.type === 'folder') {
                        setExpandedFolders(prev => new Set(prev).add(node.id));
                        if (node.children) expandRecursive(node.children);
                    }
                });
            };
            expandRecursive(filterNodes(sessions, searchTerm));
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [searchTerm]);

    const displayedSessions = filterNodes(sessions, searchTerm);

    return (
        <div style={styles.container} onClick={() => setContextMenu(null)}>
            <div style={styles.searchBar}>
                <input
                    style={styles.searchInput}
                    placeholder="搜索会话 (IP/名称)..."
                    value={searchTerm}
                    onChange={e => setSearchTerm(e.target.value)}
                />
            </div>

            <div
                style={styles.treeContainer}
                onContextMenu={(e) => handleContextMenu(e)}
                onDragOver={(e) => { if (dragNodeIdRef.current) e.preventDefault(); }}
                onDrop={handleTreeDrop}
            >
                {renderTree(displayedSessions)}
                {displayedSessions.length === 0 && (
                    <div style={styles.empty}>无会话</div>
                )}
            </div>

            {/* 团队共享会话（下半 1/4 区域；功能未启用时组件自身返回 null，不占空间）。
                与上方会话树共用搜索词和统一连接流程 */}
            <SharedSessionPanel onConnect={onConnect} searchTerm={searchTerm} runtime={sharedRuntime} />

            {/* Context Menu */}
            {contextMenu && (
                <div style={{...styles.contextMenu, top: contextMenu.y, left: contextMenu.x}} data-session-context-menu>
                    <div
                        style={{
                            ...styles.menuItem,
                            backgroundColor: hoveredMenuItem === 'newfolder' ? 'var(--bg-active)' : 'transparent',
                            color: hoveredMenuItem === 'newfolder' ? 'var(--text-on-accent)' : 'var(--text-secondary)'
                        }}
                        onMouseEnter={() => setHoveredMenuItem('newfolder')}
                        onMouseLeave={() => setHoveredMenuItem(null)}
                        onClick={() => {
                            const name = prompt('文件夹名称:');
                            if (name?.trim()) {
                                runtime.createFolder(name.trim())
                                    .then(() => loadSessions())
                                    .catch((err) => {
                                        alert(err?.message || String(err));
                                        loadSessions();
                                    });
                            }
                            setContextMenu(null);
                        }}
                    >新建文件夹</div>
                    {contextMenu.node && contextMenu.node.type === 'session' && (
                        <div
                            style={{
                                ...styles.menuItem,
                                backgroundColor: hoveredMenuItem === 'connect' ? 'var(--bg-active)' : 'transparent',
                                color: hoveredMenuItem === 'connect' ? 'var(--text-on-accent)' : 'var(--text-secondary)'
                            }}
                            onMouseEnter={() => setHoveredMenuItem('connect')}
                            onMouseLeave={() => setHoveredMenuItem(null)}
                            onClick={() => {
                                if (contextMenu.node!.config) onConnect(contextMenu.node!.config);
                                setContextMenu(null);
                            }}
                        >打开连接</div>
                    )}
                    {contextMenu.node && contextMenu.node.type === 'session' && (
                        <div
                            style={{
                                ...styles.menuItem,
                                backgroundColor: hoveredMenuItem === 'edit' ? 'var(--bg-active)' : 'transparent',
                                color: hoveredMenuItem === 'edit' ? 'var(--text-on-accent)' : 'var(--text-secondary)'
                            }}
                            onMouseEnter={() => setHoveredMenuItem('edit')}
                            onMouseLeave={() => setHoveredMenuItem(null)}
                            onClick={() => {
                                if (contextMenu.node!.config) {
                                    setEditingSession({ id: contextMenu.node!.id, config: contextMenu.node!.config });
                                }
                                setContextMenu(null);
                            }}
                        >编辑连接</div>
                    )}
                    {contextMenu.node && contextMenu.node.type === 'session' && runtime.duplicateSession && (
                        <div
                            style={{
                                ...styles.menuItem,
                                backgroundColor: hoveredMenuItem === 'duplicate' ? 'var(--bg-active)' : 'transparent',
                                color: hoveredMenuItem === 'duplicate' ? 'var(--text-on-accent)' : 'var(--text-secondary)'
                            }}
                            onMouseEnter={() => setHoveredMenuItem('duplicate')}
                            onMouseLeave={() => setHoveredMenuItem(null)}
                            onClick={() => {
                                // 完整复制为新的连接条目（同配置副本，落同一文件夹），
                                // 用户随后编辑副本的主机/端口等信息。
                                runtime.duplicateSession!(contextMenu.node!.id)
                                    .then(() => loadSessions())
                                    .catch((err) => {
                                        alert(err?.message || String(err));
                                        loadSessions();
                                    });
                                setContextMenu(null);
                            }}
                        >复制连接信息</div>
                    )}
                    {contextMenu.node && contextMenu.node.type === 'session' && !!contextMenu.node.config?.group && (
                        <div
                            style={{
                                ...styles.menuItem,
                                backgroundColor: hoveredMenuItem === 'ungroup' ? 'var(--bg-active)' : 'transparent',
                                color: hoveredMenuItem === 'ungroup' ? 'var(--text-on-accent)' : 'var(--text-secondary)'
                            }}
                            onMouseEnter={() => setHoveredMenuItem('ungroup')}
                            onMouseLeave={() => setHoveredMenuItem(null)}
                            onClick={() => {
                                handleUnGroup(contextMenu.node!);
                                setContextMenu(null);
                            }}
                        >移出分组</div>
                    )}
                    {contextMenu.node && contextMenu.node.type === 'folder' && contextMenu.node.children && contextMenu.node.children.length > 0 && (
                        <div
                            style={{
                                ...styles.menuItem,
                                backgroundColor: hoveredMenuItem === 'connectall' ? 'var(--bg-active)' : 'transparent',
                                color: hoveredMenuItem === 'connectall' ? 'var(--text-on-accent)' : 'var(--text-secondary)'
                            }}
                            onMouseEnter={() => setHoveredMenuItem('connectall')}
                            onMouseLeave={() => setHoveredMenuItem(null)}
                            onClick={() => {
                                handleConnectAll(contextMenu.node!);
                                setContextMenu(null);
                            }}
                        >全部连接</div>
                    )}
                    {contextMenu.node && (
                    <div
                        style={{
                            ...styles.menuItem,
                            backgroundColor: hoveredMenuItem === 'rename' ? 'var(--bg-active)' : 'transparent',
                            color: hoveredMenuItem === 'rename' ? 'var(--text-on-accent)' : 'var(--text-secondary)'
                        }}
                        onMouseEnter={() => setHoveredMenuItem('rename')}
                        onMouseLeave={() => setHoveredMenuItem(null)}
                        onClick={() => {
                            setEditingNodeId(contextMenu.node!.id);
                            setEditName(contextMenu.node!.name);
                            setContextMenu(null);
                        }}
                    >重命名</div>
                    )}
                    {contextMenu.node && (
                    <div
                        style={{
                            ...styles.menuItem,
                            backgroundColor: hoveredMenuItem === 'delete' ? 'var(--bg-active)' : 'transparent',
                            color: hoveredMenuItem === 'delete' ? 'var(--text-on-accent)' : 'var(--text-secondary)'
                        }}
                        onMouseEnter={() => setHoveredMenuItem('delete')}
                        onMouseLeave={() => setHoveredMenuItem(null)}
                        onClick={() => {
                            handleDelete(contextMenu.node!.id);
                            setContextMenu(null);
                        }}
                    >删除</div>
                    )}
                </div>
            )}

            {editingSession && (
                <EditSavedSessionModal
                    isOpen={true}
                    sessionId={editingSession.id}
                    initialConfig={editingSession.config}
                    onClose={() => setEditingSession(null)}
                    onSaved={loadSessions}
                    runtime={runtime}
                />
            )}
        </div>
    );
};

// protocolChipStyle:telnet 协议标识 chip。
// 风格对齐 FilesPanel 的 infoChip(胶囊形 999px + 冷调深底),仅文字色
// 用低饱和橙(var(--stage-orange))区分协议,避免强对比暖色块在侧栏里突兀。
const protocolChipStyle: React.CSSProperties = {
    display: 'inline-block',
    marginLeft: '6px',
    padding: '1px 7px',
    fontSize: '10px',
    lineHeight: '1.5',
    color: 'var(--stage-orange)',
    backgroundColor: 'var(--bg-primary)',
    border: '1px solid var(--border)',
    borderRadius: '999px',
    userSelect: 'none',
    verticalAlign: 'middle',
};

const styles = {
    container: {
        display: 'flex',
        flexDirection: 'column' as const,
        height: '100%',
        color: 'var(--text-secondary)',
        backgroundColor: 'var(--bg-secondary)',
    },
    searchBar: {
        padding: '10px',
        borderBottom: '1px solid var(--border)',
    },
    searchInput: {
        width: '100%',
        padding: '6px',
        borderRadius: '4px',
        border: '1px solid var(--border)',
        backgroundColor: 'var(--bg-input)',
        color: 'var(--text-primary)',
        outline: 'none',
        boxSizing: 'border-box' as const,
    },
    treeContainer: {
        flex: 1,
        overflowY: 'auto' as const,
        padding: '10px 0',
        minHeight: 0, // Critical for nested flex scrolling
    },
    nodeRow: {
        display: 'flex',
        alignItems: 'center',
        padding: '4px 8px',
        cursor: 'pointer',
    },
    nodeName: {
        fontSize: '14px',
        userSelect: 'none' as const,
    },
    renameInput: {
        backgroundColor: 'var(--bg-input)',
        color: 'var(--text-primary)',
        border: '1px solid var(--accent)',
        outline: 'none',
        padding: '2px 4px',
        fontSize: '14px',
        width: '150px',
    },
    empty: {
        textAlign: 'center' as const,
        color: 'var(--text-disabled)',
        marginTop: '20px',
    },
    contextMenu: {
        position: 'fixed' as const,
        backgroundColor: 'var(--bg-secondary)',
        border: '1px solid var(--border)',
        boxShadow: '0 2px 8px rgba(0,0,0,0.5)',
        borderRadius: '4px',
        zIndex: 1000,
        minWidth: '120px',
        padding: '4px 0',
    },
    menuItem: {
        padding: '6px 12px',
        cursor: 'pointer',
        fontSize: '13px',
        transition: 'background-color 0.1s',
    }
};

export default SessionManager;
