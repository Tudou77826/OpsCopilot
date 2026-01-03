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
}

const LayoutManager: React.FC<LayoutManagerProps> = ({ terminals, mode, onTerminalData, terminalRefs, onCloseTerminal, onRenameTerminal }) => {
    const [activeTab, setActiveTab] = useState<string>(terminals[0]?.id || '');
    const [editingTab, setEditingTab] = useState<string | null>(null);
    const [editValue, setEditValue] = useState('');

    // Ensure active tab is valid
    React.useEffect(() => {
        if (terminals.length > 0 && !terminals.find(t => t.id === activeTab)) {
            setActiveTab(terminals[terminals.length - 1].id);
        }
    }, [terminals, activeTab]);

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
        return <div style={styles.emptyState}>No active connections. Click "New Connection" to start.</div>;
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

    return (
        <div style={styles.container}>
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
                                flexDirection: 'column'
                            }}
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
    }
};

export default LayoutManager;
