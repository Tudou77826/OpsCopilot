import React, { useState, useEffect } from 'react';
import { ConnectionConfig } from '../../types';

// Wails bindings
declare global {
    interface Window {
        go: {
            main: {
                App: {
                    GetSavedSessions: () => Promise<SessionNode[]>;
                    DeleteSavedSession: (id: string) => Promise<string>;
                    RenameSavedSession: (id: string, newName: string) => Promise<string>;
                }
            }
        }
    }
}

export interface SessionNode {
    id: string;
    name: string;
    type: "folder" | "session";
    children?: SessionNode[];
    config?: ConnectionConfig;
}

interface SessionManagerProps {
    onConnect: (config: ConnectionConfig) => void;
}

const SessionManager: React.FC<SessionManagerProps> = ({ onConnect }) => {
    const [sessions, setSessions] = useState<SessionNode[]>([]);
    const [searchTerm, setSearchTerm] = useState('');
    const [expandedFolders, setExpandedFolders] = useState<Set<string>>(new Set());
    const [contextMenu, setContextMenu] = useState<{ x: number; y: number; node: SessionNode } | null>(null);
    const [editingNodeId, setEditingNodeId] = useState<string | null>(null);
    const [editName, setEditName] = useState('');
    const [hoveredNodeId, setHoveredNodeId] = useState<string | null>(null);
    const [hoveredMenuItem, setHoveredMenuItem] = useState<string | null>(null);

    useEffect(() => {
        loadSessions();
        const interval = setInterval(loadSessions, 5000);
        return () => clearInterval(interval);
    }, []);

    const loadSessions = async () => {
        try {
            const data = await window.go.main.App.GetSavedSessions();
            
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

            const processedData = data ? data.map(processNode) : [];
            setSessions(processedData);
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

    const handleContextMenu = (e: React.MouseEvent, node: SessionNode) => {
        e.preventDefault();
        e.stopPropagation();
        setContextMenu({ x: e.clientX, y: e.clientY, node });
    };

    const handleRename = async (id: string, newName: string) => {
        if (!newName.trim()) return;
        await window.go.main.App.RenameSavedSession(id, newName);
        loadSessions();
        setEditingNodeId(null);
    };

    const handleDelete = async (id: string) => {
        if (confirm('确定要删除吗？')) {
            await window.go.main.App.DeleteSavedSession(id);
            loadSessions();
        }
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

            return (
                <div key={node.id}>
                    <div 
                        style={{
                            ...styles.nodeRow, 
                            paddingLeft,
                            backgroundColor: isHovered ? '#2a2d2e' : 'transparent'
                        }}
                        onMouseEnter={() => setHoveredNodeId(node.id)}
                        onMouseLeave={() => setHoveredNodeId(null)}
                        onContextMenu={(e) => handleContextMenu(e, node)}
                        onClick={() => isFolder ? handleToggleFolder(node.id) : null}
                        onDoubleClick={() => !isFolder && node.config && onConnect(node.config)}
                    >
                        <span style={{marginRight: '8px', userSelect: 'none'}}>{isFolder ? (isExpanded ? '📂' : '📁') : '🖥️'}</span>
                        
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
                            <span style={styles.nodeName}>{node.name}</span>
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
    const filterNodes = (nodes: SessionNode[], term: string): SessionNode[] => {
        if (!term) return nodes;
        const lowerTerm = term.toLowerCase();
        
        return nodes.reduce<SessionNode[]>((acc, node) => {
            const matches = node.name.toLowerCase().includes(lowerTerm) || 
                           (node.config && node.config.host.includes(lowerTerm));
            
            if (node.type === 'folder') {
                const filteredChildren = filterNodes(node.children || [], term);
                if (matches || filteredChildren.length > 0) {
                    acc.push({
                        ...node,
                        children: filteredChildren
                    });
                }
            } else {
                if (matches) acc.push(node);
            }
            return acc;
        }, []);
    };
    
    // Auto-expand effect when searching
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
    }, [searchTerm, sessions]); // Added sessions dependency to re-expand if data loads while searching

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
            
            <div style={styles.treeContainer}>
                {renderTree(displayedSessions)}
                {displayedSessions.length === 0 && (
                    <div style={styles.empty}>无会话</div>
                )}
            </div>

            {/* Context Menu */}
            {contextMenu && (
                <div style={{...styles.contextMenu, top: contextMenu.y, left: contextMenu.x}}>
                    {contextMenu.node.type === 'session' && (
                        <div 
                            style={{
                                ...styles.menuItem,
                                backgroundColor: hoveredMenuItem === 'connect' ? '#094771' : 'transparent',
                                color: hoveredMenuItem === 'connect' ? '#fff' : '#ccc'
                            }} 
                            onMouseEnter={() => setHoveredMenuItem('connect')}
                            onMouseLeave={() => setHoveredMenuItem(null)}
                            onClick={() => {
                                if (contextMenu.node.config) onConnect(contextMenu.node.config);
                                setContextMenu(null);
                            }}
                        >打开连接</div>
                    )}
                    <div 
                        style={{
                            ...styles.menuItem,
                            backgroundColor: hoveredMenuItem === 'rename' ? '#094771' : 'transparent',
                            color: hoveredMenuItem === 'rename' ? '#fff' : '#ccc'
                        }}
                        onMouseEnter={() => setHoveredMenuItem('rename')}
                        onMouseLeave={() => setHoveredMenuItem(null)}
                        onClick={() => {
                            setEditingNodeId(contextMenu.node.id);
                            setEditName(contextMenu.node.name);
                            setContextMenu(null);
                        }}
                    >重命名</div>
                    <div 
                        style={{
                            ...styles.menuItem,
                            backgroundColor: hoveredMenuItem === 'delete' ? '#094771' : 'transparent',
                            color: hoveredMenuItem === 'delete' ? '#fff' : '#ccc'
                        }}
                        onMouseEnter={() => setHoveredMenuItem('delete')}
                        onMouseLeave={() => setHoveredMenuItem(null)}
                        onClick={() => {
                            handleDelete(contextMenu.node.id);
                            setContextMenu(null);
                        }}
                    >删除</div>
                </div>
            )}
        </div>
    );
};

const styles = {
    container: {
        display: 'flex',
        flexDirection: 'column' as const,
        height: '100%',
        color: '#ccc',
        backgroundColor: '#252526',
    },
    searchBar: {
        padding: '10px',
        borderBottom: '1px solid #333',
    },
    searchInput: {
        width: '100%',
        padding: '6px',
        borderRadius: '4px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#3c3c3c',
        color: '#fff',
        outline: 'none',
        boxSizing: 'border-box' as const,
    },
    treeContainer: {
        flex: 1,
        overflowY: 'auto' as const,
        padding: '10px 0',
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
        backgroundColor: '#3c3c3c',
        color: '#fff',
        border: '1px solid #007acc',
        outline: 'none',
        padding: '2px 4px',
        fontSize: '14px',
        width: '150px',
    },
    empty: {
        textAlign: 'center' as const,
        color: '#666',
        marginTop: '20px',
    },
    contextMenu: {
        position: 'fixed' as const,
        backgroundColor: '#252526',
        border: '1px solid #454545',
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
