import React, { useEffect, useMemo, useState } from 'react';
import { ConnectionConfig } from '../types';
import { SessionManagerRuntime, SharedSessionRuntime, SessionNode } from '../ports';
import { confirmDialog } from '../feedback/ConfirmDialog';
import { useToast } from '../feedback/Toast';
import ErrorBoundary from '../feedback/ErrorBoundary';
import NameDialog from '../filetransfer/NameDialog';
import SessionTreeView from './SessionTreeView';
import SessionContextMenu, { ContextMenuAction } from './SessionContextMenu';
import ConnectionPropertiesModal from './ConnectionPropertiesModal';
import XshellImportDialog from './XshellImportDialog';
import SharedSessionPanel from './SharedSessionPanel';
import { useSessionTree } from './useSessionTree';
import {
    breadcrumb,
    childrenOf,
    collectConnections,
    countConnections,
    filterTree,
    findNode,
    sortedChildIds,
} from './treeModel';

interface SessionManagerProps {
    onConnect: (config: ConnectionConfig) => void;
    runtime: SessionManagerRuntime;
    /** 团队共享会话宿主能力（Wails 专有，可选）。Sidecar 不提供时不显示。 */
    sharedRuntime?: SharedSessionRuntime | null;
}

/** 同一文件夹"全部连接"的并发上限，避免一次开出过多终端。 */
const CONNECT_ALL_LIMIT = 5;

interface NameDialogState {
    mode: 'folder' | 'rename';
    parentId?: string;
    node?: SessionNode;
}

interface PropertiesState {
    mode: 'create' | 'edit';
    parentId?: string;
    node?: SessionNode;
}

const emptyConnectionConfig = (): ConnectionConfig => ({
    name: '',
    protocol: 'ssh',
    host: '',
    port: 22,
    user: '',
});

/**
 * 会话管理面板容器。
 *
 * 只做编排：数据与展开态在 useSessionTree，树渲染与拖拽在 SessionTreeView，
 * 菜单项生成在 SessionContextMenu，纯逻辑在 treeModel。这里负责把用户动作翻译成
 * runtime 调用，并在每次变更后刷新。
 */
const SessionManager: React.FC<SessionManagerProps> = ({ onConnect, runtime, sharedRuntime }) => {
    const { nodes, expanded, toggle, expand, expandMany, refresh, loadError } = useSessionTree(runtime);
    const [searchTerm, setSearchTerm] = useState('');
    const [contextMenu, setContextMenu] = useState<{ x: number; y: number; node: SessionNode | null } | null>(null);
    const [nameDialog, setNameDialog] = useState<NameDialogState | null>(null);
    const [properties, setProperties] = useState<PropertiesState | null>(null);
    const [importOpen, setImportOpen] = useState(false);
    const toast = useToast();

    const canImport = typeof runtime.applyXshellImport === 'function';
    const canDuplicate = typeof runtime.duplicateConnection === 'function';

    const displayed = useMemo(() => filterTree(nodes, searchTerm), [nodes, searchTerm]);

    // 搜索时展开命中项的祖先链。刻意只依赖 searchTerm：把 nodes 放进依赖会让每次
    // 刷新都重新展开，把用户刚手动折叠的文件夹又撑开。
    useEffect(() => {
        if (!searchTerm.trim()) return;
        const ids: string[] = [];
        const walk = (list: SessionNode[]) => {
            for (const node of list) {
                if (node.type === 'folder') {
                    ids.push(node.id);
                    walk(node.children ?? []);
                }
            }
        };
        walk(filterTree(nodes, searchTerm));
        expandMany(ids);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [searchTerm]);

    const handleMove = async (id: string, newParentId: string, index: number) => {
        try {
            await runtime.moveNode(id, newParentId, index);
            if (newParentId) expand(newParentId);
        } catch (e: any) {
            toast.error(e?.toString?.() ?? '移动失败');
        }
        await refresh();
    };

    const handleRename = async (id: string, newName: string) => {
        try {
            await runtime.renameNode(id, newName);
        } catch (e: any) {
            toast.error(e?.toString?.() ?? '重命名失败');
        }
        await refresh();
    };

    const handleDelete = async (node: SessionNode) => {
        const count = countConnections(node);
        const message =
            node.type === 'folder'
                ? count > 0
                    ? `删除文件夹「${node.name}」会一并删除其中的 ${count} 个连接，且无法恢复。确定继续吗？`
                    : `确定删除文件夹「${node.name}」吗？`
                : `确定删除连接「${node.name}」吗？`;
        const ok = await confirmDialog.show({ message, danger: true });
        if (!ok) return;

        try {
            await runtime.deleteNode(node.id);
        } catch (e: any) {
            toast.error(e?.toString?.() ?? '删除失败');
        }
        await refresh();
    };

    const handleConnectAll = async (folder: SessionNode) => {
        const all = collectConnections(folder);
        if (all.length === 0) return;
        const targets = all.slice(0, CONNECT_ALL_LIMIT);
        if (all.length > CONNECT_ALL_LIMIT) {
            const ok = await confirmDialog.show({
                message: `该文件夹共有 ${all.length} 个会话，最多同时连接 ${CONNECT_ALL_LIMIT} 个，是否继续？`,
            });
            if (!ok) return;
        }
        targets.forEach((session) => {
            if (session.config) onConnect(session.config);
        });
    };

    const handleSortByName = async (parentId: string) => {
        const target = sortedChildIds(nodes, parentId);
        const current = childrenOf(nodes, parentId).map((node) => node.id);
        // 已经有序就不写盘，避免无意义的重排与磁盘写入。
        if (target.length === current.length && target.every((id, i) => id === current[i])) {
            toast.info('该层已经按名称排好序');
            return;
        }
        try {
            await runtime.reorderNodes(parentId, target);
        } catch (e: any) {
            toast.error(e?.toString?.() ?? '排序失败');
        }
        await refresh();
    };

    const handleDuplicate = async (node: SessionNode) => {
        try {
            await runtime.duplicateConnection?.(node.id);
        } catch (e: any) {
            toast.error(e?.toString?.() ?? '复制失败');
        }
        await refresh();
    };

    const handleAction = (action: ContextMenuAction) => {
        setContextMenu(null);
        switch (action.kind) {
            case 'newFolder':
                setNameDialog({ mode: 'folder', parentId: action.parentId });
                break;
            case 'newConnection':
                setProperties({ mode: 'create', parentId: action.parentId });
                break;
            case 'connect':
                if (action.node.config) onConnect(action.node.config);
                break;
            case 'connectAll':
                void handleConnectAll(action.node);
                break;
            case 'properties':
                setProperties({ mode: 'edit', node: action.node });
                break;
            case 'duplicate':
                void handleDuplicate(action.node);
                break;
            case 'sortByName':
                void handleSortByName(action.parentId);
                break;
            case 'moveToRoot':
                void handleMove(action.node.id, '', Number.MAX_SAFE_INTEGER);
                break;
            case 'rename':
                setNameDialog({ mode: 'rename', node: action.node });
                break;
            case 'delete':
                void handleDelete(action.node);
                break;
            case 'import':
                setImportOpen(true);
                break;
        }
    };

    const handleNameDialogConfirm = async (name: string) => {
        const dialog = nameDialog;
        setNameDialog(null);
        if (!dialog) return;

        try {
            if (dialog.mode === 'folder') {
                const parentId = dialog.parentId ?? '';
                const siblings = childrenOf(nodes, parentId);
                await runtime.createFolder(name, parentId);
                if (parentId) expand(parentId);
                void siblings;
            } else if (dialog.node) {
                await runtime.renameNode(dialog.node.id, name);
            }
        } catch (e: any) {
            toast.error(e?.toString?.() ?? '操作失败');
        }
        await refresh();
    };

    const propertiesParentId = properties?.parentId ?? '';
    const propertiesParentLabel = propertiesParentId
        ? breadcrumb(nodes, propertiesParentId)
        : undefined;
    const propertiesInitialConfig = properties?.node?.config ?? emptyConnectionConfig();

    return (
        <ErrorBoundary label="会话管理">
        <div style={styles.container}>
            <div style={styles.searchBar}>
                <input
                    style={styles.searchInput}
                    placeholder="搜索会话（IP / 名称）..."
                    value={searchTerm}
                    onChange={(e) => setSearchTerm(e.target.value)}
                />
                {canImport && (
                    <button
                        style={styles.importButton}
                        onClick={() => setImportOpen(true)}
                        title="从本机 Xshell 或导出文件导入会话"
                    >
                        导入
                    </button>
                )}
            </div>

            {loadError && <div style={styles.loadError}>{loadError}</div>}

            <SessionTreeView
                nodes={displayed}
                expanded={expanded}
                onToggle={toggle}
                onExpand={expand}
                onConnect={(node) => node.config && onConnect(node.config)}
                onContextMenu={(event, node) => {
                    event.preventDefault();
                    event.stopPropagation();
                    setContextMenu({ x: event.clientX, y: event.clientY, node });
                }}
                onRename={handleRename}
                onMove={handleMove}
            />

            {/* 团队共享会话（下半区；功能未启用时组件自身返回 null，不占空间）。
                与上方会话树共用搜索词和统一连接流程。 */}
            <SharedSessionPanel onConnect={onConnect} searchTerm={searchTerm} runtime={sharedRuntime} />

            {contextMenu && (
                <SessionContextMenu
                    x={contextMenu.x}
                    y={contextMenu.y}
                    node={contextMenu.node}
                    nodes={nodes}
                    canDuplicate={canDuplicate}
                    canImport={canImport}
                    onAction={handleAction}
                    onClose={() => setContextMenu(null)}
                />
            )}

            {nameDialog && (
                <NameDialog
                    title={nameDialog.mode === 'folder' ? '新建文件夹' : '重命名'}
                    defaultValue={nameDialog.node?.name ?? ''}
                    placeholder="名称"
                    onConfirm={(name) => void handleNameDialogConfirm(name)}
                    onCancel={() => setNameDialog(null)}
                />
            )}

            {properties && (
                <ConnectionPropertiesModal
                    isOpen={true}
                    mode={properties.mode}
                    sessionId={properties.node?.id}
                    initialConfig={propertiesInitialConfig}
                    parentId={propertiesParentId}
                    parentLabel={propertiesParentLabel}
                    onClose={() => setProperties(null)}
                    onSaved={refresh}
                    runtime={runtime}
                />
            )}

            <XshellImportDialog
                isOpen={importOpen}
                runtime={runtime}
                onClose={() => setImportOpen(false)}
                onImported={refresh}
            />
        </div>
        </ErrorBoundary>
    );
};

const styles: Record<string, React.CSSProperties> = {
    container: {
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        color: 'var(--text-secondary)',
        backgroundColor: 'var(--bg-secondary)',
    },
    searchBar: {
        display: 'flex',
        gap: 8,
        padding: '10px',
        borderBottom: '1px solid var(--border)',
    },
    searchInput: {
        flex: 1,
        padding: '6px',
        borderRadius: 4,
        border: '1px solid var(--border)',
        backgroundColor: 'var(--bg-input)',
        color: 'var(--text-primary)',
        outline: 'none',
        boxSizing: 'border-box',
        minWidth: 0,
    },
    importButton: {
        padding: '6px 12px',
        borderRadius: 4,
        border: '1px solid var(--border-strong)',
        backgroundColor: 'var(--bg-elevated)',
        color: 'var(--text-primary)',
        fontSize: 12,
        cursor: 'pointer',
        whiteSpace: 'nowrap',
    },
    loadError: {
        padding: '6px 10px',
        fontSize: 12,
        color: 'var(--danger)',
    },
};

export default SessionManager;
