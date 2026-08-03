import React, { useState, useEffect } from 'react';
import { QuickCommand } from './types';

interface CommandGridProps {
    commands: QuickCommand[];
    onExecute: (content: string) => void;
    onEdit: (command: QuickCommand) => void;
    onDelete: (id: string) => void;
    onAdd: () => void;
}

const CommandGrid: React.FC<CommandGridProps> = ({ commands, onExecute, onEdit, onDelete, onAdd }) => {
    const [contextMenu, setContextMenu] = useState<{ x: number; y: number; cmdId: string } | null>(null);
    const [hoveredId, setHoveredId] = useState<string | null>(null);

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
                {commands.map(cmd => {
                    const isHovered = hoveredId === cmd.id;
                    return (
                        <div
                            key={cmd.id}
                            style={{
                                ...styles.card,
                                backgroundColor: isHovered ? 'var(--bg-elevated)' : 'var(--bg-primary)',
                                borderColor: isHovered ? 'var(--border)' : 'var(--bg-elevated)',
                                color: isHovered ? 'var(--text-primary)' : 'var(--text-tertiary)',
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
                    <div style={{ ...styles.menu, top: contextMenu.y - 80, left: contextMenu.x }} data-testid="command-context-menu">
                        <div style={styles.menuItem} onClick={() => {
                            const cmd = commands.find(c => c.id === contextMenu.cmdId);
                            if (cmd) onEdit(cmd);
                            setContextMenu(null);
                        }}>
                            编辑
                        </div>
                        <div style={styles.menuItem} onClick={() => {
                            onDelete(contextMenu.cmdId);
                            setContextMenu(null);
                        }}>
                            删除
                        </div>
                    </div>
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
        maxHeight: '180px',
        alignContent: 'flex-start',
    },
    card: {
        padding: '4px 10px',
        borderRadius: '4px',
        cursor: 'pointer',
        border: '1px solid var(--bg-elevated)',
        fontSize: '11px',
        color: 'var(--text-tertiary)',
        backgroundColor: 'var(--bg-primary)',
        whiteSpace: 'nowrap' as const,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        userSelect: 'none' as const,
        flex: '0 0 auto',
        maxWidth: '120px',
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
        fontSize: '11px',
        backgroundColor: 'transparent',
        flex: '0 0 auto',
        transition: 'border-color 0.2s, color 0.2s',
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
};

export default CommandGrid;
