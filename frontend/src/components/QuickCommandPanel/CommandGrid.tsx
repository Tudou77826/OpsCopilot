import React, { useState } from 'react';
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
                                backgroundColor: isHovered ? '#2a2a2a' : '#222',
                                borderColor: isHovered ? '#444' : '#2a2a2a',
                                color: isHovered ? '#ddd' : '#aaa',
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
        flexShrink: 0,
        maxHeight: '180px',
        alignContent: 'flex-start',
    },
    card: {
        padding: '4px 10px',
        borderRadius: '4px',
        cursor: 'pointer',
        border: '1px solid #2a2a2a',
        fontSize: '11px',
        color: '#aaa',
        backgroundColor: '#222',
        whiteSpace: 'nowrap' as const,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        userSelect: 'none' as const,
        transition: 'background-color 0.2s, border-color 0.2s, color 0.2s',
    },
    addCard: {
        padding: '4px 10px',
        borderRadius: '4px',
        cursor: 'pointer',
        border: '1px dashed #333',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        color: '#444',
        fontSize: '11px',
        backgroundColor: 'transparent',
        transition: 'border-color 0.2s, color 0.2s',
    },
    backdrop: {
        position: 'fixed' as const,
        top: 0, left: 0, right: 0, bottom: 0,
        zIndex: 998,
    },
    menu: {
        position: 'fixed' as const,
        backgroundColor: '#252526',
        border: '1px solid #404040',
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
        color: '#bbb',
        transition: 'background-color 0.15s',
    },
};

export default CommandGrid;
