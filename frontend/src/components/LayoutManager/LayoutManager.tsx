import React, { useState } from 'react';
import TerminalComponent, { TerminalRef } from '../Terminal/Terminal';

interface TerminalSession {
    id: string;
    title: string;
}

interface LayoutManagerProps {
    terminals: TerminalSession[];
    mode: 'tab' | 'grid';
    onTerminalData: (id: string, data: string) => void;
    // Ref map to expose terminal instances to parent
    terminalRefs: React.MutableRefObject<Map<string, TerminalRef>>;
    onCloseTerminal: (id: string) => void;
    onRenameTerminal: (id: string, newTitle: string) => void;
    onDuplicateTerminal?: (id: string) => void;
    onClose?: () => void; // Optional onClose prop
    onActiveTerminalChange?: (id: string | null) => void;
}

const LayoutManager: React.FC<LayoutManagerProps> = ({ terminals, mode, onTerminalData, terminalRefs, onCloseTerminal, onRenameTerminal, onDuplicateTerminal, onActiveTerminalChange }) => {
    const [activeTab, setActiveTab] = useState<string>(terminals[0]?.id || '');
    const [editingTab, setEditingTab] = useState<string | null>(null);
    const [editValue, setEditValue] = useState('');
    const [contextMenu, setContextMenu] = useState<{ x: number, y: number, id: string } | null>(null);

    // Ensure active tab is valid
    React.useEffect(() => {
        if (terminals.length > 0 && !terminals.find(t => t.id === activeTab)) {
            const nextActive = terminals[terminals.length - 1].id;
            setActiveTab(nextActive);
        } else if (terminals.length === 0) {
            setActiveTab('');
        }
    }, [terminals, activeTab]);

    // Notify active terminal change
    React.useEffect(() => {
        if (onActiveTerminalChange) {
            onActiveTerminalChange(activeTab || null);
        }
    }, [activeTab, onActiveTerminalChange]);

    const handleTabClick = (id: string) => {
        setActiveTab(id);
        // Focus the terminal when tab is clicked
        setTimeout(() => {
            terminalRefs.current.get(id)?.fit();
        }, 50);
    };

    // Trigger fit on layout change
    React.useEffect(() => {
        // Wait for layout transition/render
        const timer = setTimeout(() => {
            terminalRefs.current.forEach(term => term.fit());
        }, 100);
        return () => clearTimeout(timer);
    }, [terminals.length, mode, terminalRefs]);

    if (terminals.length === 0) {
        return <div style={styles.emptyState}>暂无活动连接。请点击右上角 “+ 新建连接” 开始使用。</div>;
    }

    const getGridStyle = () => {
        const count = terminals.length;
        let cols = 1;
        let rows = 1;

        if (count >= 2 && count <= 4) {
            cols = 2;
            rows = 2;
        } else if (count > 4) {
            cols = 3;
            rows = Math.ceil(count / 3);
        }
        
        if (count === 2) {
             // 2 windows side-by-side (1 row, 2 columns)
             cols = 2;
             rows = 1;
        }

        return {
            ...styles.gridContent,
            gridTemplateColumns: `repeat(${cols}, 1fr)`,
            gridTemplateRows: `repeat(${rows}, 1fr)`,
        };
    };

    const handleTabDoubleClick = (id: string, currentTitle: string) => {
        setEditingTab(id);
        setEditValue(currentTitle);
    };

    const handleRenameSubmit = (id: string) => {
        if (editValue.trim()) {
            onRenameTerminal(id, editValue.trim());
        }
        setEditingTab(null);
    };

    const handleKeyDown = (e: React.KeyboardEvent, id: string) => {
        if (e.key === 'Enter') {
            handleRenameSubmit(id);
        } else if (e.key === 'Escape') {
            setEditingTab(null);
        }
    };

    const handleContextMenu = (e: React.MouseEvent, id: string) => {
        e.preventDefault();
        setContextMenu({ x: e.clientX, y: e.clientY, id });
    };

    return (
        <div style={styles.container} onClick={() => setContextMenu(null)}>
            {mode === 'tab' && (
                <div style={styles.tabHeader} role="tablist">
                    {terminals.map(term => (
                        <div
                            key={term.id}
                            role="tab"
                            aria-selected={activeTab === term.id}
                            style={{
                                ...styles.tab,
                                ...(activeTab === term.id ? styles.activeTab : {})
                            }}
                            onClick={() => handleTabClick(term.id)}
                            onDoubleClick={() => handleTabDoubleClick(term.id, term.title)}
                            onContextMenu={(e) => handleContextMenu(e, term.id)}
                        >
                            {editingTab === term.id ? (
                                <input
                                    autoFocus
                                    value={editValue}
                                    onChange={(e) => setEditValue(e.target.value)}
                                    onBlur={() => handleRenameSubmit(term.id)}
                                    onKeyDown={(e) => handleKeyDown(e, term.id)}
                                    style={styles.renameInput}
                                    onClick={(e) => e.stopPropagation()}
                                />
                            ) : (
                                <div style={styles.tabContentInner}>
                                    <span style={styles.tabTitle}>{term.title}</span>
                                    <span 
                                        style={styles.closeBtn}
                                        onClick={(e) => {
                                            e.stopPropagation();
                                            onCloseTerminal(term.id);
                                        }}
                                    >
                                        ×
                                    </span>
                                </div>
                            )}
                        </div>
                    ))}
                </div>
            )}

            <div style={mode === 'grid' ? getGridStyle() : styles.tabContent}>
                {terminals.map(term => {
                    // In tab mode, hide inactive terminals instead of unmounting to preserve state
                    const isVisible = mode === 'grid' || activeTab === term.id;
                    
                    return (
                        <div 
                            key={term.id} 
                            style={{
                                ...styles.terminalWrapper,
                                display: isVisible ? 'flex' : 'none',
                                flexDirection: 'column',
                                border: (mode === 'grid' && activeTab === term.id) ? '1px solid #007acc' : '1px solid transparent'
                            }}
                            onClick={() => handleTabClick(term.id)}
                        >
                            {mode === 'grid' && (
                                <div style={styles.gridTitle}>
                                    {term.title}
                                </div>
                            )}
                            <div style={{flex: 1, position: 'relative', overflow: 'hidden'}}>
                                <TerminalComponent 
                                    id={term.id} 
                                    onData={(data) => onTerminalData(term.id, data)}
                                    ref={(el) => {
                                        if (el) {
                                            terminalRefs.current.set(term.id, el);
                                        } else {
                                            terminalRefs.current.delete(term.id);
                                        }
                                    }}
                                />
                            </div>
                        </div>
                    );
                })}
            </div>

            {/* Context Menu */}
            {contextMenu && (
                <div 
                    style={{
                        ...styles.contextMenu, 
                        top: contextMenu.y, 
                        left: contextMenu.x
                    }}
                    onClick={(e) => e.stopPropagation()}
                >
                    <div 
                        style={styles.menuItem} 
                        onClick={() => {
                            const term = terminals.find(t => t.id === contextMenu.id);
                            if (term) {
                                setEditingTab(contextMenu.id);
                                setEditValue(term.title);
                            }
                            setContextMenu(null);
                        }}
                    >
                        重命名
                    </div>
                    <div 
                        style={styles.menuItem} 
                        onClick={() => {
                            if (onDuplicateTerminal) onDuplicateTerminal(contextMenu.id);
                            setContextMenu(null);
                        }}
                    >
                        复制标签
                    </div>
                </div>
            )}
        </div>
    );
};

const styles = {
    container: {
        height: '100%',
        display: 'flex',
        flexDirection: 'column' as const,
        backgroundColor: '#1e1e1e',
    },
    emptyState: {
        height: '100%',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        color: '#666',
    },
    tabHeader: {
        display: 'flex',
        backgroundColor: '#252526',
        borderBottom: '1px solid #1e1e1e',
    },
    tab: {
        padding: '0', // Reset padding for inner content
        color: '#969696',
        cursor: 'pointer',
        borderRight: '1px solid #1e1e1e',
        backgroundColor: '#2d2d2d',
        maxWidth: '200px',
        minWidth: '120px',
        overflow: 'hidden',
        whiteSpace: 'nowrap' as const,
        textOverflow: 'ellipsis',
        display: 'flex',
        alignItems: 'center',
    },
    tabContentInner: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        width: '100%',
        padding: '8px 12px',
        boxSizing: 'border-box' as const,
    },
    tabTitle: {
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap' as const,
        marginRight: '8px',
    },
    closeBtn: {
        borderRadius: '50%',
        width: '16px',
        height: '16px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontSize: '12px',
        color: '#ccc',
        cursor: 'pointer',
        flexShrink: 0,
        ':hover': {
            backgroundColor: 'rgba(255, 255, 255, 0.2)',
            color: '#fff',
        }
    },
    renameInput: {
        width: '100%',
        padding: '4px',
        border: 'none',
        outline: 'none',
        backgroundColor: '#3c3c3c',
        color: '#fff',
        margin: '4px',
    },
    activeTab: {
        color: '#fff',
        backgroundColor: '#1e1e1e',
        borderTop: '1px solid #007acc',
    },
    tabContent: {
        flex: 1,
        position: 'relative' as const,
        overflow: 'hidden',
    },
    gridContent: {
        flex: 1,
        display: 'grid',
        // gridTemplateColumns and rows are set dynamically
        gap: '4px',
        padding: '4px',
        backgroundColor: '#000',
        height: '100%',
        minHeight: 0, // Important for nested flex containers
        overflow: 'hidden',
    },
    terminalWrapper: {
        height: '100%',
        width: '100%',
        position: 'relative' as const,
        overflow: 'hidden',
        backgroundColor: '#1e1e1e', // Ensure background for grid items
    },
    gridTitle: {
        backgroundColor: '#2d2d2d',
        color: '#ccc',
        padding: '4px 8px',
        fontSize: '0.8rem',
        borderBottom: '1px solid #3c3c3c',
        whiteSpace: 'nowrap' as const,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
    },
    contextMenu: {
        position: 'fixed' as const,
        backgroundColor: '#252526',
        border: '1px solid #454545',
        boxShadow: '0 2px 8px rgba(0,0,0,0.5)',
        borderRadius: '4px',
        zIndex: 2000,
        minWidth: '120px',
        padding: '4px 0',
    },
    menuItem: {
        padding: '6px 12px',
        cursor: 'pointer',
        fontSize: '13px',
        color: '#ccc',
        transition: 'background-color 0.1s',
        ':hover': {
            backgroundColor: '#094771',
            color: '#fff',
        }
    }
};

export default LayoutManager;
