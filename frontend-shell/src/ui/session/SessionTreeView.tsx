import React, { useRef, useState } from 'react';
import { TbFolder, TbFolderOpen, TbTerminal2 } from 'react-icons/tb';
import { normalizeProtocol, PROTOCOL_LABEL } from '../types';
import type { SessionNode } from '../ports';
import {
    breadcrumb,
    canMove,
    dropPositionFrom,
    findLocation,
    indentForLevel,
    insertionIndex,
    isNoopMove,
    MAX_INDENT_LEVEL,
    type DropPosition,
} from './treeModel';

export type MoveHandler = (id: string, newParentId: string, index: number) => void | Promise<void>;

type Props = {
    nodes: SessionNode[];
    expanded: Set<string>;
    onToggle: (id: string) => void;
    onExpand: (id: string) => void;
    onConnect: (node: SessionNode) => void;
    onContextMenu: (event: React.MouseEvent, node: SessionNode | null) => void;
    onRename: (id: string, newName: string) => void | Promise<void>;
    onMove: MoveHandler;
};

type DropTarget = { id: string; position: DropPosition };

/**
 * 会话树视图：递归渲染 + 拖拽编排。
 *
 * 拖拽采用行高三分区：上 25% 插到该行之前、下 25% 之后（同级排序），
 * 中间 50% 且目标是文件夹则移入（Xshell 的手感）。文件夹自身也可拖，
 * 拖入自身或后代会被 canMove 拦下并显示禁止光标，后端另有独立校验兜底。
 */
const SessionTreeView: React.FC<Props> = ({
    nodes,
    expanded,
    onToggle,
    onExpand,
    onConnect,
    onContextMenu,
    onRename,
    onMove,
}) => {
    const [dropTarget, setDropTarget] = useState<DropTarget | null>(null);
    const [rejectedId, setRejectedId] = useState<string | null>(null);
    const [hoveredId, setHoveredId] = useState<string | null>(null);
    const [editingId, setEditingId] = useState<string | null>(null);
    const [editValue, setEditValue] = useState('');

    const dragIdRef = useRef<string | null>(null);
    const autoExpandTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const autoScrollRafRef = useRef<number | null>(null);
    const containerRef = useRef<HTMLDivElement | null>(null);
    const lastDragPointRef = useRef({ x: 0, y: 0 });

    const stopAutoScroll = () => {
        if (autoScrollRafRef.current !== null) {
            cancelAnimationFrame(autoScrollRafRef.current);
            autoScrollRafRef.current = null;
        }
    };

    // 边缘自动滚动：列表超出一屏时，底部节点需要能拖到顶部的文件夹。
    const startAutoScroll = () => {
        if (autoScrollRafRef.current !== null) return;
        const step = () => {
            const container = containerRef.current;
            if (!container) return;
            const rect = container.getBoundingClientRect();
            const { y } = lastDragPointRef.current;
            const edge = 40;
            if (y >= rect.top && y <= rect.top + edge) {
                container.scrollTop -= Math.ceil((1 - (y - rect.top) / edge) * 14) + 2;
            } else if (y <= rect.bottom && y > rect.bottom - edge) {
                container.scrollTop += Math.ceil((1 - (rect.bottom - y) / edge) * 14) + 2;
            }
            autoScrollRafRef.current = requestAnimationFrame(step);
        };
        autoScrollRafRef.current = requestAnimationFrame(step);
    };

    const clearAutoExpand = () => {
        if (autoExpandTimerRef.current) {
            clearTimeout(autoExpandTimerRef.current);
            autoExpandTimerRef.current = null;
        }
    };

    const resetDrag = () => {
        dragIdRef.current = null;
        setDropTarget(null);
        setRejectedId(null);
        clearAutoExpand();
        stopAutoScroll();
    };

    const handleDragStart = (event: React.DragEvent, node: SessionNode) => {
        event.dataTransfer.setData('text/plain', node.id);
        event.dataTransfer.effectAllowed = 'move';
        dragIdRef.current = node.id;
    };

    const handleDragOver = (event: React.DragEvent, node: SessionNode) => {
        const dragId = dragIdRef.current;
        if (!dragId) return;
        event.preventDefault();

        const rect = event.currentTarget.getBoundingClientRect();
        const ratio = rect.height > 0 ? (event.clientY - rect.top) / rect.height : 0.5;
        const position = dropPositionFrom(ratio, node.type === 'folder');

        // 落点的"父"：移入时就是目标文件夹本身，排序时是目标所在的那一层。
        const parentId = position === 'inside' ? node.id : (findLocation(nodes, node.id)?.parent?.id ?? '');

        if (!canMove(nodes, dragId, parentId)) {
            event.dataTransfer.dropEffect = 'none';
            setRejectedId(node.id);
            setDropTarget(null);
            return;
        }

        event.dataTransfer.dropEffect = 'move';
        setRejectedId(null);
        setDropTarget({ id: node.id, position });

        // 悬停文件夹 600ms 后自动展开，方便把节点拖进深层分组。
        if (node.type === 'folder' && !expanded.has(node.id)) {
            clearAutoExpand();
            autoExpandTimerRef.current = setTimeout(() => onExpand(node.id), 600);
        }
    };

    const handleDrop = async (event: React.DragEvent, node: SessionNode) => {
        // 文件夹行已消化本次 drop，必须阻断冒泡：容器兜底的语义是"移到根"，
        // 放行会在本处理器之后再次提交移动，把刚移入的节点又弹回根（Issue #70）。
        event.stopPropagation();
        event.preventDefault();

        const target = dropTarget;
        const dragId = dragIdRef.current;
        resetDrag();
        if (!dragId || !target || target.id !== node.id) return;

        const parentId = target.position === 'inside' ? node.id : (findLocation(nodes, node.id)?.parent?.id ?? '');
        if (!canMove(nodes, dragId, parentId)) return;

        const index = insertionIndex(nodes, dragId, parentId, node.id, target.position);
        if (isNoopMove(nodes, dragId, parentId, index)) return;
        await onMove(dragId, parentId, index);
    };

    const handleContainerDragOver = (event: React.DragEvent) => {
        if (!dragIdRef.current) return;
        event.preventDefault();
        lastDragPointRef.current = { x: event.clientX, y: event.clientY };
        startAutoScroll();
    };

    const handleContainerDrop = async (event: React.DragEvent) => {
        event.preventDefault();
        stopAutoScroll();
        // 只响应真正落在容器空白处的 drop；落在行上的冒泡由行内处理器负责。
        if (event.target !== event.currentTarget) return;
        const dragId = dragIdRef.current;
        resetDrag();
        if (!dragId) return;
        if (!canMove(nodes, dragId, '')) return;

        const index = insertionIndex(nodes, dragId, '', null, 'inside');
        if (isNoopMove(nodes, dragId, '', index)) return;
        await onMove(dragId, '', index);
    };

    const startRename = (node: SessionNode) => {
        setEditingId(node.id);
        setEditValue(node.name);
    };

    const commitRename = async (node: SessionNode) => {
        const next = editValue.trim();
        setEditingId(null);
        if (next && next !== node.name) await onRename(node.id, next);
    };

    const renderTree = (list: SessionNode[], level: number): React.ReactNode =>
        list.map((node) => {
            const isFolder = node.type === 'folder';
            const isExpanded = expanded.has(node.id);
            const isEditing = editingId === node.id;
            const isRejected = rejectedId === node.id;
            const isDropInside = dropTarget?.id === node.id && dropTarget.position === 'inside';
            const isDropBefore = dropTarget?.id === node.id && dropTarget.position === 'before';
            const isDropAfter = dropTarget?.id === node.id && dropTarget.position === 'after';
            const isHovered = hoveredId === node.id;
            // 深层节点不再继续缩进，改为完整路径 tooltip，避免被推出视野。
            const paddingLeft = indentForLevel(level);
            const tooltip = level >= MAX_INDENT_LEVEL ? breadcrumb(nodes, node.id) : undefined;

            return (
                <div key={node.id}>
                    <div
                        data-testid={`tree-row-${node.id}`}
                        data-drop-position={dropTarget?.id === node.id ? dropTarget.position : undefined}
                        style={{
                            ...styles.row,
                            paddingLeft,
                            backgroundColor: isDropInside || isHovered ? 'var(--bg-elevated)' : 'transparent',
                            outline: isDropInside ? '1px dashed var(--accent)' : 'none',
                            outlineOffset: '-1px',
                            borderTop: isDropBefore ? '2px solid var(--accent)' : '2px solid transparent',
                            borderBottom: isDropAfter ? '2px solid var(--accent)' : '2px solid transparent',
                            cursor: isRejected ? 'not-allowed' : 'pointer',
                            opacity: isRejected ? 0.5 : 1,
                        }}
                        title={tooltip}
                        draggable={!isEditing}
                        onDragStart={(e) => handleDragStart(e, node)}
                        onDragOver={(e) => handleDragOver(e, node)}
                        onDrop={(e) => void handleDrop(e, node)}
                        onDragEnd={resetDrag}
                        onMouseEnter={() => setHoveredId(node.id)}
                        onMouseLeave={() => setHoveredId(null)}
                        onContextMenu={(e) => onContextMenu(e, node)}
                        onClick={() => (isFolder ? onToggle(node.id) : undefined)}
                        onDoubleClick={() => (!isFolder && node.config ? onConnect(node) : startRename(node))}
                    >
                        <span style={{ ...styles.icon, color: isFolder ? 'var(--icon-folder-fg)' : 'var(--text-muted)' }}>
                            {isFolder
                                ? (isExpanded ? TbFolderOpen({ size: 16 }) : TbFolder({ size: 16 }))
                                : TbTerminal2({ size: 16 })}
                        </span>

                        {isEditing ? (
                            <input
                                autoFocus
                                value={editValue}
                                onChange={(e) => setEditValue(e.target.value)}
                                onBlur={() => void commitRename(node)}
                                onKeyDown={(e) => {
                                    if (e.key === 'Enter') void commitRename(node);
                                    if (e.key === 'Escape') setEditingId(null);
                                }}
                                onClick={(e) => e.stopPropagation()}
                                onDoubleClick={(e) => e.stopPropagation()}
                                style={styles.renameInput}
                            />
                        ) : (
                            <>
                                <span
                                    style={{
                                        ...styles.name,
                                        fontWeight: isFolder ? 600 : 400,
                                        color: isFolder ? 'var(--text-primary)' : 'var(--text-secondary)',
                                    }}
                                >
                                    {node.name}
                                </span>
                                {/* 协议 chip：仅非默认协议显示，避免全 SSH 环境噪声 */}
                                {!isFolder && normalizeProtocol(node.config?.protocol) === 'telnet' && (
                                    <span style={protocolChipStyle}>{PROTOCOL_LABEL.telnet}</span>
                                )}
                            </>
                        )}
                    </div>
                    {isFolder && isExpanded && node.children && renderTree(node.children, level + 1)}
                </div>
            );
        });

    return (
        <div
            ref={containerRef}
            style={styles.container}
            data-testid="session-tree"
            onContextMenu={(e) => onContextMenu(e, null)}
            onDragOver={handleContainerDragOver}
            onDrop={(e) => void handleContainerDrop(e)}
        >
            {renderTree(nodes, 0)}
            {nodes.length === 0 && <div style={styles.empty}>无会话</div>}
        </div>
    );
};

// protocolChipStyle 与 FilesPanel 的 infoChip 风格对齐（胶囊形 + 冷调深底），
// 仅文字色用低饱和橙区分协议，避免强对比暖色块在侧栏里突兀。
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
        flex: 1,
        overflowY: 'auto' as const,
        padding: '10px 0',
        minHeight: 0,
    },
    row: {
        display: 'flex',
        alignItems: 'center',
        padding: '4px 8px',
        userSelect: 'none' as const,
    },
    icon: {
        marginRight: '8px',
        display: 'inline-flex',
        alignItems: 'center',
    },
    name: {
        fontSize: '14px',
        whiteSpace: 'nowrap' as const,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
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
};

export default SessionTreeView;
