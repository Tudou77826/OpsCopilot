import React, { useState, useEffect } from 'react';
import { QuickCommand } from './types';
import { confirmDialog } from '../ConfirmDialog/ConfirmDialog';

interface CommandGridProps {
    commands: QuickCommand[];
    onExecute: (content: string) => void;
    onEdit: (command: QuickCommand) => void;
    onDelete: (id: string) => void;
    onAdd: () => void;
    /** 当前搜索关键字（在当前分组内进一步过滤） */
    searchQuery: string;
    onSearchChange: (query: string) => void;
    /** 拖拽排序回调：给出当前分组命令的新顺序（搜索过滤时不允许拖拽） */
    onReorder: (ordered: QuickCommand[]) => void;
}

const CommandGrid: React.FC<CommandGridProps> = ({
    commands, onExecute, onEdit, onDelete, onAdd, searchQuery, onSearchChange, onReorder,
}) => {
    const [contextMenu, setContextMenu] = useState<{ x: number; y: number; cmdId: string } | null>(null);
    const [hoveredId, setHoveredId] = useState<string | null>(null);
    const [hoveredMenuItem, setHoveredMenuItem] = useState<number | null>(null);
    const searchInputRef = React.useRef<HTMLInputElement>(null);

    // 拖拽排序：搜索过滤时顺序意义不明确，禁用；只有一张卡时无意义，也禁用
    const canReorder = commands.length > 1 && !searchQuery.trim();
    const [dragId, setDragId] = useState<string | null>(null);
    const [dropHint, setDropHint] = useState<{ id: string; before: boolean } | null>(null);

    const commitReorder = (hint: { id: string; before: boolean }) => {
        if (!dragId || dragId === hint.id) return;
        const dragged = commands.find(c => c.id === dragId);
        if (!dragged) return;
        const rest = commands.filter(c => c.id !== dragId);
        const targetIdx = rest.findIndex(c => c.id === hint.id);
        if (targetIdx < 0) return;
        rest.splice(hint.before ? targetIdx : targetIdx + 1, 0, dragged);
        onReorder(rest);
    };

    // Close context menu on any outside pointer event (including xterm.js terminals)
    useEffect(() => {
        if (!contextMenu) return;
        const handler = (e: PointerEvent) => {
            const menuEl = document.querySelector('[data-testid="command-context-menu"]');
            if (menuEl && menuEl.contains(e.target as Node)) return;
            setContextMenu(null);
        };
        document.addEventListener('pointerdown', handler);
        return () => document.removeEventListener('pointerdown', handler);
    }, [contextMenu]);

    return (
        <>
            <div style={styles.grid} data-testid="command-grid">
                {/* 搜索卡片：与命令卡片同 flow 的第一个元素，外观接近普通卡片（不抢眼）。
                    在当前分组内按 name/content 过滤（issue #56）。 */}
                <div
                    style={styles.searchCard}
                    data-testid="command-search"
                    onClick={(e) => {
                        e.stopPropagation();
                        searchInputRef.current?.focus();
                    }}
                >
                    <span style={styles.searchIcon} aria-hidden>🔍</span>
                    <input
                        ref={searchInputRef}
                        style={styles.searchInput}
                        value={searchQuery}
                        onChange={(e) => onSearchChange(e.target.value)}
                        placeholder=""
                        aria-label="搜索快捷命令"
                        onClick={(e) => e.stopPropagation()}
                    />
                </div>
                {commands.map(cmd => {
                    const isHovered = hoveredId === cmd.id;
                    const isDragging = dragId === cmd.id;
                    const isDropTarget = dropHint?.id === cmd.id;
                    return (
                        <div
                            key={cmd.id}
                            style={{
                                ...styles.card,
                                backgroundColor: isHovered ? 'var(--bg-elevated)' : 'var(--bg-primary)',
                                borderColor: isHovered ? 'var(--border)' : 'var(--bg-elevated)',
                                color: isHovered ? 'var(--text-primary)' : 'var(--text-tertiary)',
                                // 光标保持普通指针：点击执行才是主操作，拖拽排序只是偶发能力，
                                // 不能用 grab 光标喧宾夺主（真正拖动时浏览器原生 DnD 会接管光标）
                                cursor: 'pointer',
                                opacity: isDragging ? 0.35 : undefined,
                                // 插入位置指示：目标卡左/右缘一条主题色竖线，不挤动布局
                                boxShadow: isDropTarget
                                    ? (dropHint!.before ? 'inset 3px 0 0 var(--accent)' : 'inset -3px 0 0 var(--accent)')
                                    : undefined,
                            }}
                            draggable={canReorder}
                            onDragStart={(e) => {
                                setDragId(cmd.id);
                                e.dataTransfer.effectAllowed = 'move';
                                // WebView2 要求 dataTransfer 有数据才允许 drop
                                e.dataTransfer.setData('text/plain', cmd.id);
                            }}
                            onDragEnd={() => { setDragId(null); setDropHint(null); }}
                            onDragOver={(e) => {
                                if (!dragId || dragId === cmd.id) return;
                                e.preventDefault();
                                e.dataTransfer.dropEffect = 'move';
                                const rect = e.currentTarget.getBoundingClientRect();
                                setDropHint({ id: cmd.id, before: e.clientX - rect.left < rect.width / 2 });
                            }}
                            onDragLeave={() => {
                                if (dropHint?.id === cmd.id) setDropHint(null);
                            }}
                            onDrop={(e) => {
                                e.preventDefault();
                                if (dropHint?.id === cmd.id) commitReorder(dropHint);
                                setDragId(null);
                                setDropHint(null);
                            }}
                            onClick={() => onExecute(cmd.content)}
                            onContextMenu={(e) => {
                                e.preventDefault();
                                setContextMenu({ x: e.clientX, y: e.clientY, cmdId: cmd.id });
                            }}
                            onMouseEnter={() => setHoveredId(cmd.id)}
                            onMouseLeave={() => setHoveredId(null)}
                            title={cmd.content}
                            data-testid={`command-card-${cmd.id}`}
                        >
                            {cmd.name}
                        </div>
                    );
                })}
                <div
                    style={styles.addCard}
                    onClick={onAdd}
                    data-testid="command-add-btn"
                >
                    + 添加
                </div>
            </div>

            {contextMenu && (
                <>
                    <div style={styles.backdrop} onClick={() => setContextMenu(null)} />
                    {(() => {
                        // 边界感知定位：菜单出现在指针处，但整体限制在视口内，
                        // 靠近屏幕底部/右缘时向上/向左收，避免弹出可视区外。
                        const MENU_W = 100;
                        const MENU_H = 70;
                        const top = Math.max(8, Math.min(contextMenu.y, window.innerHeight - MENU_H - 8));
                        const left = Math.max(8, Math.min(contextMenu.x, window.innerWidth - MENU_W - 8));
                        return (
                            <div style={{ ...styles.menu, top, left }} data-testid="command-context-menu">
                                <div
                                    style={hoveredMenuItem === 0 ? { ...styles.menuItem, ...styles.menuItemHover } : styles.menuItem}
                                    onMouseEnter={() => setHoveredMenuItem(0)}
                                    onMouseLeave={() => setHoveredMenuItem(null)}
                                    onClick={() => {
                                        const cmd = commands.find(c => c.id === contextMenu.cmdId);
                                        if (cmd) onEdit(cmd);
                                        setContextMenu(null);
                                    }}
                                >
                                    编辑
                                </div>
                                <div
                                    style={{
                                        ...(hoveredMenuItem === 1 ? { ...styles.menuItem, ...styles.menuItemHover } : styles.menuItem),
                                        color: 'var(--severity-danger)',
                                    }}
                                    onMouseEnter={() => setHoveredMenuItem(1)}
                                    onMouseLeave={() => setHoveredMenuItem(null)}
                                    onClick={async () => {
                                        const cmd = commands.find(c => c.id === contextMenu.cmdId);
                                        setContextMenu(null);
                                        if (!cmd) return;
                                        const ok = await confirmDialog.show({
                                            title: '删除快捷命令',
                                            message: `确定删除「${cmd.name}」？此操作立即生效且不可恢复。`,
                                            confirmText: '删除',
                                            danger: true,
                                        });
                                        if (ok) onDelete(cmd.id);
                                    }}
                                >
                                    删除
                                </div>
                            </div>
                        );
                    })()}
                </>
            )}
        </>
    );
};

const styles = {
    grid: {
        display: 'flex',
        flexWrap: 'wrap' as const,
        gap: '5px',
        padding: '10px 12px',
        overflowY: 'auto' as const,
        flexGrow: 1,
        flexShrink: 1,
        minWidth: 0,
        // 滚动边界由外层面板高度决定（面板可拖拽调高），
        // 这里不再写死 180px 上限，改为允许在 flex 布局中收缩
        minHeight: 0,
        alignContent: 'flex-start',
    },
    card: {
        padding: '4px 10px',
        borderRadius: '4px',
        cursor: 'pointer',
        border: '1px solid var(--bg-elevated)',
        fontSize: '12px',
        color: 'var(--text-tertiary)',
        backgroundColor: 'var(--bg-primary)',
        whiteSpace: 'nowrap' as const,
        overflow: 'hidden' as const,
        textOverflow: 'ellipsis',
        userSelect: 'none' as const,
        flex: '0 0 auto',
        maxWidth: '140px',
        transition: 'background-color 0.2s, border-color 0.2s, color 0.2s',
    },
    addCard: {
        padding: '4px 10px',
        borderRadius: '4px',
        cursor: 'pointer',
        border: '1px dashed var(--border)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        color: 'var(--text-disabled)',
        fontSize: '12px',
        backgroundColor: 'transparent',
        flex: '0 0 auto',
        transition: 'border-color 0.2s, color 0.2s',
    },
    // 搜索卡片：刻意做成与命令卡片近似的样式（同 padding/border/radius/字号），
    // 与命令流融为一体、不抢眼（issue #56）。内部是 🔍 + 透明输入框。
    // 输入框宽度收紧到与一个普通命令卡片相近，避免它比其它卡片长出一截而扎眼。
    searchCard: {
        padding: '4px 8px',
        borderRadius: '4px',
        cursor: 'text',
        border: '1px solid var(--bg-elevated)',
        display: 'flex',
        alignItems: 'center',
        gap: '4px',
        fontSize: '12px',
        backgroundColor: 'var(--bg-primary)',
        color: 'var(--text-tertiary)',
        userSelect: 'none' as const,
        flex: '0 0 auto',
        transition: 'border-color 0.2s, color 0.2s',
    },
    searchIcon: {
        fontSize: '11px',
        color: 'var(--text-disabled)',
        flexShrink: 0,
        lineHeight: 1,
    },
    searchInput: {
        border: 'none',
        outline: 'none',
        backgroundColor: 'transparent',
        color: 'var(--text-primary)',
        fontSize: '12px',
        padding: '0',
        margin: '0',
        width: '48px',
        fontFamily: 'inherit',
    },
    backdrop: {
        position: 'fixed' as const,
        top: 0, left: 0, right: 0, bottom: 0,
        zIndex: 998,
    },
    menu: {
        position: 'fixed' as const,
        backgroundColor: 'var(--bg-secondary)',
        border: '1px solid var(--border)',
        boxShadow: '0 4px 16px rgba(0,0,0,0.6)',
        borderRadius: '6px',
        zIndex: 999,
        minWidth: '90px',
        padding: '4px 0',
    },
    menuItem: {
        padding: '6px 12px',
        cursor: 'pointer',
        fontSize: '11px',
        color: 'var(--text-secondary)',
        transition: 'background-color 0.15s',
    },
    menuItemHover: {
        backgroundColor: 'var(--bg-elevated)',
    },
};

export default CommandGrid;
