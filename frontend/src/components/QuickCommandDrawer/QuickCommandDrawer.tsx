import React, { useState, useEffect, useRef } from 'react';

export interface QuickCommand {
    id: string;
    name: string;
    content: string;
    group?: string; // 所属分组
}

interface QuickCommandDrawerProps {
    onExecute: (content: string) => void;
    isOpen: boolean;
    onToggle: () => void;
}

const QuickCommandDrawer: React.FC<QuickCommandDrawerProps> = ({ onExecute, isOpen, onToggle }) => {
    const [commands, setCommands] = useState<QuickCommand[]>([]);
    const [selectedGroup, setSelectedGroup] = useState<string>('default');
    const [availableGroups, setAvailableGroups] = useState<string[]>(['default']);
    const [showGroupList, setShowGroupList] = useState(false);
    const groupListRef = useRef<HTMLDivElement>(null);
    const [contextMenu, setContextMenu] = useState<{ x: number, y: number, cmdId: string } | null>(null);
    const [editingCmd, setEditingCmd] = useState<QuickCommand | null>(null);
    const [loaded, setLoaded] = useState(false);

    // Close group list when clicking outside
    useEffect(() => {
        const handleClickOutside = (event: MouseEvent) => {
            if (groupListRef.current && !groupListRef.current.contains(event.target as Node)) {
                setShowGroupList(false);
            }
        };

        if (showGroupList) {
            document.addEventListener('mousedown', handleClickOutside);
        }

        return () => {
            document.removeEventListener('mousedown', handleClickOutside);
        };
    }, [showGroupList]);

    // Load from backend on mount
    useEffect(() => {
        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.LoadQuickCommands) {
            // @ts-ignore
            window.go.main.App.LoadQuickCommands().then((cmds: QuickCommand[]) => {
                if (cmds) {
                    setCommands(cmds);
                } else {
                    setCommands([]);
                }
                setLoaded(true);
            }).catch((err: any) => {
                console.error("Failed to load quick commands", err);
                setLoaded(true);
            });
        } else {
            setLoaded(true);
        }
    }, []);

    // Load groups when commands change
    useEffect(() => {
        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.GetQuickCommandGroups) {
            // @ts-ignore
            window.go.main.App.GetQuickCommandGroups().then((groups: string[]) => {
                if (groups && groups.length > 0) {
                    setAvailableGroups(groups);
                    // If current selected group doesn't exist, select the first available group
                    if (!groups.includes(selectedGroup)) {
                        setSelectedGroup(groups[0]);
                    }
                }
            }).catch((err: any) => {
                console.error("Failed to load groups", err);
            });
        }
    }, [commands]);

    // Save to backend on change
    useEffect(() => {
        if (!loaded) return;

        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.SaveQuickCommands) {
            // @ts-ignore
            window.go.main.App.SaveQuickCommands(commands);
        }
    }, [commands, loaded]);

    const handleAdd = () => {
        const newCmd: QuickCommand = {
            id: Date.now().toString(),
            name: 'New Command',
            content: 'echo "Hello"',
            group: selectedGroup // New command belongs to current group
        };
        setCommands(prev => [...prev, newCmd]);
        setEditingCmd(newCmd); // Immediately edit
    };

    const handleDelete = (id: string) => {
        setCommands(prev => prev.filter(c => c.id !== id));
        setContextMenu(null);
    };

    const handleSaveEdit = () => {
        if (!editingCmd) return;
        setCommands(prev => prev.map(c => c.id === editingCmd.id ? editingCmd : c));
        setEditingCmd(null);
    };

    // Filter commands by selected group
    const filteredCommands = commands.filter(cmd => {
        const group = cmd.group || 'default';
        return group === selectedGroup;
    });

    return (
        <>
            <div
                style={{
                    ...styles.container,
                    maxHeight: isOpen ? '300px' : '24px',
                }}
            >
                {/* Handle with Group Selector */}
                <div
                    style={styles.handle}
                >
                    {/* Group Selector on the left */}
                    <div style={styles.groupSelectorContainer} ref={groupListRef}>
                        <div
                            style={styles.groupTrigger}
                            onClick={(e) => {
                                e.stopPropagation();
                                setShowGroupList(!showGroupList);
                            }}
                        >
                            <span style={styles.groupTriggerText}>{selectedGroup}</span>
                            <span style={styles.groupTriggerArrow}>{showGroupList ? '▲' : '▼'}</span>
                        </div>

                        {showGroupList && (
                            <div
                                style={{
                                    ...styles.groupListPopup,
                                    left: groupListRef.current ? groupListRef.current.getBoundingClientRect().left : 0,
                                    bottom: groupListRef.current ? window.innerHeight - groupListRef.current.getBoundingClientRect().top + 4 : 0,
                                } as any}
                            >
                                {availableGroups.map(group => (
                                    <div
                                        key={group}
                                        style={{
                                            ...styles.groupListItem,
                                            ...(selectedGroup === group ? styles.groupListItemActive : {})
                                        }}
                                        onClick={(e) => {
                                            e.stopPropagation();
                                            setSelectedGroup(group);
                                            setShowGroupList(false);
                                        }}
                                    >
                                        {group}
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>

                    {/* Spacer */}
                    <div style={{ flex: 1 }} />

                    {/* Toggle button centered */}
                    <div
                        style={styles.toggleButton}
                        onClick={onToggle}
                    >
                        {isOpen ? '▼ 快捷命令' : '▲ 快捷命令'}
                    </div>

                    {/* Spacer */}
                    <div style={{ flex: 1 }} />
                </div>

                {/* Content */}
                <div style={styles.content}>
                    {/* Commands Grid */}
                    <div style={styles.grid}>
                        {filteredCommands.map(cmd => (
                            <div
                                key={cmd.id}
                                style={styles.card}
                                onClick={() => onExecute(cmd.content)}
                                onContextMenu={(e) => {
                                    e.preventDefault();
                                    setContextMenu({ x: e.clientX, y: e.clientY, cmdId: cmd.id });
                                }}
                                title={cmd.content}
                            >
                                {cmd.name}
                            </div>
                        ))}
                        <div style={styles.addCard} onClick={handleAdd}>+</div>
                    </div>
                </div>
            </div>

            {/* Context Menu */}
            {contextMenu && (
                <>
                    <div style={styles.backdrop} onClick={() => setContextMenu(null)} />
                    <div style={{ ...styles.menu, top: contextMenu.y - 80, left: contextMenu.x }}>
                        <div style={styles.menuItem} onClick={() => {
                            const cmd = commands.find(c => c.id === contextMenu.cmdId);
                            if (cmd) setEditingCmd(cmd);
                            setContextMenu(null);
                        }}>编辑</div>
                        <div style={styles.menuItem} onClick={() => handleDelete(contextMenu.cmdId)}>删除</div>
                    </div>
                </>
            )}

            {/* Edit Modal */}
            {editingCmd && (
                <div style={styles.modalOverlay}>
                    <div style={styles.modal}>
                        <h3 style={styles.modalTitle}>编辑命令</h3>
                        <div style={styles.formGroup}>
                            <label style={styles.label}>名称</label>
                            <input
                                style={styles.input}
                                value={editingCmd.name}
                                onChange={e => setEditingCmd({ ...editingCmd, name: e.target.value })}
                            />
                        </div>
                        <div style={styles.formGroup}>
                            <label style={styles.label}>分组</label>
                            <input
                                style={styles.input}
                                value={editingCmd.group || 'default'}
                                onChange={e => setEditingCmd({ ...editingCmd, group: e.target.value })}
                                placeholder="default"
                                list="availableGroups"
                            />
                            <datalist id="availableGroups">
                                {availableGroups.map(g => <option key={g} value={g} />)}
                            </datalist>
                        </div>
                        <div style={styles.formGroup}>
                            <label style={styles.label}>命令内容</label>
                            <textarea
                                style={styles.textarea}
                                value={editingCmd.content}
                                onChange={e => setEditingCmd({ ...editingCmd, content: e.target.value })}
                            />
                        </div>
                        <div style={styles.modalActions}>
                            <button onClick={() => setEditingCmd(null)} style={styles.secondaryBtn}>取消</button>
                            <button onClick={handleSaveEdit} style={styles.primaryBtn}>保存</button>
                        </div>
                    </div>
                </div>
            )}
        </>
    );
};

const styles = {
    container: {
        display: 'flex',
        flexDirection: 'column' as const,
        width: '100%',
        backgroundColor: '#252526',
        borderTop: '1px solid #333',
        flexShrink: 0,
        transition: 'max-height 0.3s ease',
        overflow: 'hidden',
    },
    handle: {
        height: '24px',
        width: '100%',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        backgroundColor: '#2e2e2e',
        color: '#ccc',
        fontSize: '12px',
        userSelect: 'none' as const,
        borderBottom: '1px solid #333',
        padding: '0 6px',
    },
    centerControls: {
        display: 'flex' as const,
        alignItems: 'center' as const,
        gap: '8px',
    },
    groupSelectorContainer: {
        position: 'relative' as const,
    },
    groupTrigger: {
        display: 'flex' as const,
        alignItems: 'center' as const,
        gap: '6px',
        padding: '2px 8px',
        backgroundColor: '#3c3c3c',
        border: '1px solid #555',
        borderRadius: '3px',
        cursor: 'pointer',
        fontSize: '12px',
        color: '#ccc',
        userSelect: 'none' as const,
        ':hover': {
            backgroundColor: '#444',
            borderColor: '#007acc',
        }
    },
    groupTriggerText: {
        fontWeight: 500,
    },
    groupTriggerArrow: {
        fontSize: '10px',
    },
    groupListPopup: {
        position: 'fixed' as const,
        backgroundColor: '#252526',
        border: '1px solid #454545',
        borderRadius: '4px',
        boxShadow: '0 4px 12px rgba(0,0,0,0.5)',
        minWidth: '100px',
        maxHeight: '200px',
        overflowY: 'auto' as const,
        zIndex: 9999,
    },
    groupListItem: {
        padding: '6px 12px',
        fontSize: '12px',
        color: '#ccc',
        cursor: 'pointer',
        whiteSpace: 'nowrap' as const,
        ':hover': {
            backgroundColor: '#094771',
            color: '#fff',
        }
    },
    groupListItemActive: {
        backgroundColor: '#007acc',
        color: '#fff',
    },
    toggleButton: {
        cursor: 'pointer',
        fontSize: '10px',
        padding: '2px 8px',
        ':hover': {
            color: '#fff',
        }
    },
    content: {
        flex: 1,
        padding: '8px 10px',
        overflowY: 'auto' as const,
    },
    grid: {
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fill, minmax(80px, 1fr))',
        gap: '6px',
    },
    card: {
        backgroundColor: '#333',
        color: '#fff',
        padding: '4px 6px',
        borderRadius: '3px',
        cursor: 'pointer',
        fontSize: '11px',
        textAlign: 'center' as const,
        border: '1px solid #444',
        whiteSpace: 'nowrap' as const,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        userSelect: 'none' as const,
        ':hover': {
            backgroundColor: '#444',
            borderColor: '#007acc',
        }
    },
    addCard: {
        backgroundColor: 'transparent',
        color: '#888',
        padding: '4px 6px',
        borderRadius: '3px',
        cursor: 'pointer',
        fontSize: '12px',
        textAlign: 'center' as const,
        border: '1px dashed #555',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
    },
    backdrop: {
        position: 'fixed' as const,
        top: 0, left: 0, right: 0, bottom: 0,
        zIndex: 998,
    },
    menu: {
        position: 'fixed' as const,
        backgroundColor: '#252526',
        border: '1px solid #454545',
        boxShadow: '0 2px 8px rgba(0,0,0,0.5)',
        borderRadius: '4px',
        zIndex: 999,
        minWidth: '100px',
        padding: '4px 0',
    },
    menuItem: {
        padding: '6px 12px',
        cursor: 'pointer',
        fontSize: '13px',
        color: '#ccc',
        ':hover': {
            backgroundColor: '#094771',
            color: '#fff',
        }
    },
    modalOverlay: {
        position: 'fixed' as const,
        top: 0, left: 0, right: 0, bottom: 0,
        backgroundColor: 'rgba(0,0,0,0.5)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 1000,
    },
    modal: {
        backgroundColor: '#252526',
        padding: '20px',
        borderRadius: '8px',
        width: '300px',
        border: '1px solid #454545',
    },
    modalTitle: {
        color: '#fff',
        marginTop: 0,
        marginBottom: '16px',
        fontSize: '16px',
    },
    formGroup: {
        marginBottom: '12px',
    },
    label: {
        display: 'block',
        color: '#ccc',
        marginBottom: '4px',
        fontSize: '12px',
    },
    input: {
        width: '100%',
        padding: '6px',
        backgroundColor: '#3c3c3c',
        border: '1px solid #555',
        color: '#fff',
        borderRadius: '4px',
        boxSizing: 'border-box' as const,
    },
    textarea: {
        width: '100%',
        height: '80px',
        padding: '6px',
        backgroundColor: '#3c3c3c',
        border: '1px solid #555',
        color: '#fff',
        borderRadius: '4px',
        resize: 'none' as const,
        boxSizing: 'border-box' as const,
        fontFamily: 'monospace',
    },
    modalActions: {
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '8px',
        marginTop: '20px',
    },
    primaryBtn: {
        padding: '6px 12px',
        backgroundColor: '#007acc',
        color: 'white',
        border: 'none',
        borderRadius: '4px',
        cursor: 'pointer',
    },
    secondaryBtn: {
        padding: '6px 12px',
        backgroundColor: '#333',
        color: '#ccc',
        border: '1px solid #555',
        borderRadius: '4px',
        cursor: 'pointer',
    }
};

export default QuickCommandDrawer;
