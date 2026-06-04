import React, { useRef, useState, useCallback, useEffect, useMemo } from 'react';
import { Layout, Model, TabNode, TabSetNode, BorderNode, Actions, DockLocation, ITabRenderValues } from 'flexlayout-react';
import TerminalComponent, { TerminalRef } from '../Terminal/Terminal';
import { HighlightRule, TerminalConfig } from '../Terminal/highlightTypes';
import { SessionStatus } from '../../types';
import { TimestampResult } from '../../utils/timestampParser';
import { createInitialLayout } from './layoutConfig';

interface TerminalSession {
    id: string;
    title: string;
    status: SessionStatus;
    config?: any;
    disconnectReason?: string;
}

interface FlexLayoutAdapterProps {
    terminals: TerminalSession[];
    onTerminalData: (id: string, data: string) => void;
    terminalRefs: React.MutableRefObject<Map<string, TerminalRef>>;
    onCloseTerminal: (id: string) => void;
    onRenameTerminal: (id: string, newTitle: string) => void;
    onDuplicateTerminal?: (id: string) => void;
    onReconnect?: (id: string) => void;
    onActiveTerminalChange?: (id: string | null) => void;
    isBroadcastMode?: boolean;
    broadcastIds?: string[];
    onToggleTerminalBroadcast?: (id: string) => void;
    completionDelay?: number;
    terminalConfig?: TerminalConfig;
    highlightRules?: HighlightRule[];
    onSelectionParsed?: (result: TimestampResult | null) => void;
    onOpenFileTransfer?: (terminalId: string) => void;
}

const FlexLayoutAdapter: React.FC<FlexLayoutAdapterProps> = ({
    terminals,
    onTerminalData,
    terminalRefs,
    onCloseTerminal,
    onRenameTerminal,
    onDuplicateTerminal,
    onReconnect,
    onActiveTerminalChange,
    isBroadcastMode,
    broadcastIds,
    onToggleTerminalBroadcast,
    completionDelay,
    terminalConfig,
    highlightRules,
    onSelectionParsed,
    onOpenFileTransfer,
}) => {
    const [contextMenu, setContextMenu] = useState<{ x: number; y: number; id: string } | null>(null);

    // Close context menu on any outside pointer event (including sidebar, other components)
    useEffect(() => {
        if (!contextMenu) return;
        const handler = (e: PointerEvent) => {
            const menuEl = document.querySelector('[data-tab-context-menu]');
            if (menuEl && menuEl.contains(e.target as Node)) return;
            setContextMenu(null);
        };
        document.addEventListener('pointerdown', handler);
        return () => document.removeEventListener('pointerdown', handler);
    }, [contextMenu]);

    // Fast-lookup map for terminals (updated synchronously, no re-render)
    const terminalsMapRef = useRef(new Map<string, TerminalSession>());
    terminalsMapRef.current = new Map(terminals.map(t => [t.id, t]));

    // Create model once
    const model = useMemo(() => Model.fromJson(createInitialLayout()), []);

    // Track previous terminal IDs to compute diffs
    const prevTerminalIdsRef = useRef<Set<string>>(new Set());

    // --- Sync terminals[] → flexlayout Model ---
    useEffect(() => {
        const currentIds = new Set(terminals.map(t => t.id));
        const prevIds = prevTerminalIdsRef.current;

        // Add new terminals
        for (const term of terminals) {
            if (!prevIds.has(term.id)) {
                let targetId: string | null = null;

                // Find any existing tabset to add to
                model.visitNodes((node: any) => {
                    if (node.getType() === 'tabset' && !targetId) {
                        targetId = node.getId();
                    }
                });

                // If no tabset exists, add one to root row
                if (!targetId) {
                    const rootRow = model.getRootRow();
                    // Add tab as a new tabset to root — flexlayout auto-creates tabset
                    model.doAction(Actions.addTab(
                        {
                            type: 'tab',
                            id: term.id,
                            name: term.title,
                            component: 'terminal',
                        },
                        rootRow.getId(),
                        DockLocation.CENTER,
                        0,
                        true,
                    ));
                    prevTerminalIdsRef.current = currentIds;
                    return; // skip the normal addTab below
                }

                if (targetId) {
                    model.doAction(Actions.addTab(
                        {
                            type: 'tab',
                            id: term.id,
                            name: term.title,
                            component: 'terminal',
                        },
                        targetId,
                        DockLocation.CENTER,
                        -1,
                        true, // select the new tab
                    ));
                }
            }
        }

        // Remove terminals no longer in state
        for (const prevId of prevIds) {
            if (!currentIds.has(prevId)) {
                try {
                    model.doAction(Actions.deleteTab(prevId));
                } catch {
                    // tab may already be gone
                }
            }
        }

        prevTerminalIdsRef.current = currentIds;
    }, [terminals, model]);

    // --- Intercept actions ---
    const handleAction = useCallback((action: any): any => {
        if (action.type === Actions.DELETE_TAB) {
            const tabId = action.data.node;
            onCloseTerminal(tabId);
            return undefined; // block flexlayout's own delete; our sync effect handles removal
        }
        if (action.type === Actions.RENAME_TAB) {
            const tabId = action.data.node;
            const newName = action.data.text;
            if (newName && newName.trim()) {
                onRenameTerminal(tabId, newName.trim());
            }
            return action;
        }
        return action;
    }, [onCloseTerminal, onRenameTerminal]);

    // --- Factory: render Terminal in each tab ---
    const factory = useCallback((node: TabNode) => {
        const terminalId = node.getId();

        return (
            <TerminalWrapper
                terminalId={terminalId}
                terminalsMapRef={terminalsMapRef}
                onTerminalData={onTerminalData}
                terminalRefs={terminalRefs}
                completionDelay={completionDelay}
                terminalConfig={terminalConfig}
                highlightRules={highlightRules}
                onSelectionParsed={onSelectionParsed}
                node={node}
                isBroadcastMode={isBroadcastMode}
                broadcastIds={broadcastIds}
                onToggleTerminalBroadcast={onToggleTerminalBroadcast}
            />
        );
    }, [onTerminalData, terminalRefs, completionDelay, terminalConfig, highlightRules, onSelectionParsed, isBroadcastMode, broadcastIds, onToggleTerminalBroadcast]);

    // --- Custom tab rendering ---
    const handleRenderTab = useCallback((node: TabNode, renderValues: ITabRenderValues) => {
        const terminalId = node.getId();
        const term = terminalsMapRef.current.get(terminalId);
        if (!term) return;

        const isConnected = term.status === SessionStatus.CONNECTED;

        // Status dot
        renderValues.leading = (
            <span style={{
                display: 'inline-block',
                width: '8px',
                height: '8px',
                borderRadius: '50%',
                backgroundColor: isConnected ? '#4caf50' : '#ff6b6b',
                marginRight: '6px',
                flexShrink: 0,
            }} />
        );

        // Content: title + broadcast icon + reconnect
        const parts: React.ReactNode[] = [];

        if (isBroadcastMode) {
            const isActive = broadcastIds?.includes(terminalId);
            parts.push(
                <span
                    key="broadcast"
                    style={{
                        cursor: 'pointer',
                        fontSize: '0.85rem',
                        marginRight: '4px',
                    }}
                    onClick={(e) => {
                        e.stopPropagation();
                        onToggleTerminalBroadcast?.(terminalId);
                    }}
                    title={isActive ? '广播已开启' : '广播已关闭'}
                >
                    {isActive ? '📡' : '🔇'}
                </span>
            );
        }

        parts.push(
            <span key="title" style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>
                {term.title}
            </span>
        );

        if (!isConnected) {
            parts.push(
                <button
                    key="reconnect"
                    style={{
                        backgroundColor: '#4caf50',
                        color: 'white',
                        border: 'none',
                        padding: '1px 6px',
                        borderRadius: '3px',
                        cursor: 'pointer',
                        fontSize: '10px',
                        marginLeft: '6px',
                    }}
                    onClick={(e) => {
                        e.stopPropagation();
                        onReconnect?.(terminalId);
                    }}
                >
                    重连
                </button>
            );
        }

        renderValues.content = <>{parts}</>;

        // Context menu button
        renderValues.buttons.push(
            <div
                key="menu"
                style={{
                    cursor: 'pointer',
                    padding: '0 4px',
                    fontSize: '14px',
                    color: '#999',
                    lineHeight: 1,
                }}
                onClick={(e) => {
                    e.stopPropagation();
                    setContextMenu({ x: e.nativeEvent.clientX, y: e.nativeEvent.clientY, id: terminalId });
                }}
            >
                ···
            </div>
        );

        // Disconnected tab styling
        if (!isConnected) {
            renderValues.leading = (
                <span style={{
                    display: 'inline-block',
                    width: '8px',
                    height: '8px',
                    borderRadius: '50%',
                    backgroundColor: '#ff6b6b',
                    marginRight: '6px',
                    flexShrink: 0,
                }} />
            );
        }
    }, [isBroadcastMode, broadcastIds, onToggleTerminalBroadcast, onReconnect]);

    // --- Track active terminal ---
    const handleModelChange = useCallback((_model: Model, action: any) => {
        const trackActions = [Actions.SELECT_TAB, Actions.SET_ACTIVE_TABSET, Actions.MOVE_NODE, Actions.ADD_TAB];
        if (trackActions.includes(action.type)) {
            let activeId: string | null = null;

            // getActiveTabset() returns the tabset the user last interacted with
            const activeTabset = _model.getActiveTabset();
            if (activeTabset) {
                const selected = activeTabset.getSelectedNode();
                if (selected) {
                    activeId = selected.getId();
                }
            }

            // Fallback: scan all tabsets (needed for MOVE_NODE / ADD_TAB
            // where getActiveTabset may not be updated yet)
            if (!activeId) {
                _model.visitNodes((node: any) => {
                    if (node.getType() === 'tabset') {
                        const selected = node.getSelectedNode();
                        if (selected) {
                            activeId = selected.getId();
                        }
                    }
                });
            }

            onActiveTerminalChange?.(activeId);
        }

        // Fit only visible (selected) terminals after layout changes
        setTimeout(() => {
            _model.visitNodes((node: any) => {
                if (node.getType() === 'tabset') {
                    const selected = node.getSelectedNode();
                    if (selected) {
                        const ref = terminalRefs.current.get(selected.getId());
                        try { ref?.fit(); } catch { /* ignore */ }
                    }
                }
            });
        }, 100);
    }, [onActiveTerminalChange, terminalRefs]);

    // --- Context menu handler from right-click on tabs ---
    const handleContextMenu = useCallback((node: TabNode | TabSetNode | BorderNode, event: React.MouseEvent<HTMLElement>) => {
        if (node instanceof TabNode) {
            event.preventDefault();
            setContextMenu({ x: event.clientX, y: event.clientY, id: node.getId() });
        }
    }, []);

    return (
        <div
            style={{ height: '100%', position: 'relative', overflow: 'hidden' }}
            onPointerDown={(e) => {
                const menuEl = document.querySelector('[data-tab-context-menu]');
                if (menuEl && menuEl.contains(e.target as Node)) return;
                setContextMenu(null);
            }}
        >
            {terminals.length === 0 ? (
                <div style={{
                    height: '100%',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    color: '#666',
                }}>
                    暂无活动连接。请点击右上角 "+ 新建连接" 开始使用。
                </div>
            ) : (
                <Layout
                    model={model}
                    factory={factory}
                    onAction={handleAction}
                    onRenderTab={handleRenderTab}
                    onModelChange={handleModelChange}
                    onContextMenu={handleContextMenu}
                    realtimeResize={true}
                />
            )}

            {/* Context Menu */}
            {contextMenu && (
                <ContextMenu
                    x={contextMenu.x}
                    y={contextMenu.y}
                    terminalId={contextMenu.id}
                    terminals={terminals}
                    onCloseTerminal={(id) => { onCloseTerminal(id); setContextMenu(null); }}
                    onRename={(id, name) => { onRenameTerminal(id, name); setContextMenu(null); }}
                    onDuplicate={onDuplicateTerminal ? (id) => { onDuplicateTerminal(id); setContextMenu(null); } : undefined}
                    onOpenFileTransfer={onOpenFileTransfer ? (id) => { onOpenFileTransfer(id); setContextMenu(null); } : undefined}
                    onClose={() => setContextMenu(null)}
                />
            )}
        </div>
    );
};

// --- Terminal wrapper with resize listener ---
interface TerminalWrapperProps {
    terminalId: string;
    terminalsMapRef: React.MutableRefObject<Map<string, TerminalSession>>;
    onTerminalData: (id: string, data: string) => void;
    terminalRefs: React.MutableRefObject<Map<string, TerminalRef>>;
    completionDelay?: number;
    terminalConfig?: TerminalConfig;
    highlightRules?: HighlightRule[];
    onSelectionParsed?: (result: TimestampResult | null) => void;
    node: TabNode;
    isBroadcastMode?: boolean;
    broadcastIds?: string[];
    onToggleTerminalBroadcast?: (id: string) => void;
}

const TerminalWrapper: React.FC<TerminalWrapperProps> = ({
    terminalId,
    onTerminalData,
    terminalRefs,
    completionDelay,
    terminalConfig,
    highlightRules,
    onSelectionParsed,
    node,
    isBroadcastMode,
    broadcastIds,
    onToggleTerminalBroadcast,
}) => {
    // Register resize listener on the tab node
    useEffect(() => {
        node.setEventListener('resize', () => {
            setTimeout(() => {
                const ref = terminalRefs.current.get(terminalId);
                try { ref?.fit(); } catch { /* ignore */ }
            }, 50);
        });
        return () => node.removeEventListener('resize');
    }, [node, terminalId, terminalRefs]);

    // Focus terminal when this tab becomes visible
    useEffect(() => {
        node.setEventListener('visibility', (event: { visible: boolean }) => {
            if (!event.visible) return;
            setTimeout(() => {
                const ref = terminalRefs.current.get(terminalId);
                try { ref?.fit(); ref?.focus(); } catch { /* ignore */ }
            }, 50);
        });
        return () => node.removeEventListener('visibility');
    }, [node, terminalId, terminalRefs]);

    const isActive = broadcastIds?.includes(terminalId);

    return (
        <div style={{ height: '100%', width: '100%', position: 'relative' }}>
            <TerminalComponent
                id={terminalId}
                sessionID={terminalId}
                onData={(data) => onTerminalData(terminalId, data)}
                completionDelay={completionDelay}
                terminalConfig={terminalConfig}
                highlightRules={highlightRules}
                onSelectionParsed={onSelectionParsed}
                ref={(el) => {
                    if (el) {
                        terminalRefs.current.set(terminalId, el);
                    } else {
                        terminalRefs.current.delete(terminalId);
                    }
                }}
            />
            {/* Broadcast overlay */}
            {isBroadcastMode && (
                <div
                    style={{
                        position: 'absolute',
                        top: '10px',
                        right: '20px',
                        zIndex: 10,
                        backgroundColor: isActive ? 'rgba(76, 175, 80, 0.9)' : 'rgba(60, 60, 60, 0.8)',
                        color: '#fff',
                        padding: '4px 8px',
                        borderRadius: '4px',
                        fontSize: '12px',
                        cursor: 'pointer',
                        display: 'flex',
                        alignItems: 'center',
                        gap: '4px',
                        boxShadow: '0 2px 4px rgba(0,0,0,0.3)',
                        border: isActive ? '1px solid #45a049' : '1px solid #555',
                        userSelect: 'none',
                    }}
                    onClick={(e) => {
                        e.stopPropagation();
                        onToggleTerminalBroadcast?.(terminalId);
                    }}
                    title={isActive ? '点击退出广播组' : '点击加入广播组'}
                >
                    {isActive ? '📡 广播中' : '🔇 已静音'}
                </div>
            )}
        </div>
    );
};

// --- Context Menu ---
interface ContextMenuProps {
    x: number;
    y: number;
    terminalId: string;
    terminals: TerminalSession[];
    onCloseTerminal: (id: string) => void;
    onRename: (id: string, name: string) => void;
    onDuplicate?: (id: string) => void;
    onOpenFileTransfer?: (id: string) => void;
    onClose: () => void;
}

const ContextMenu: React.FC<ContextMenuProps> = ({ x, y, terminalId, terminals, onCloseTerminal, onRename, onDuplicate, onOpenFileTransfer, onClose }) => {
    const term = terminals.find(t => t.id === terminalId);
    const idx = terminals.findIndex(t => t.id === terminalId);

    return (
        <div
            data-tab-context-menu
            style={{
                position: 'fixed',
                top: y,
                left: x,
                backgroundColor: '#252526',
                border: '1px solid #454545',
                boxShadow: '0 2px 8px rgba(0,0,0,0.5)',
                borderRadius: '4px',
                zIndex: 2000,
                minWidth: '140px',
                padding: '4px 0',
            }}
            onClick={(e) => e.stopPropagation()}
        >
            <div style={menuItemStyle} onClick={() => {
                const name = prompt('重命名:', term?.title || '');
                if (name?.trim()) onRename(terminalId, name.trim());
                onClose();
            }}>
                重命名
            </div>
            {onDuplicate && (
                <div style={menuItemStyle} onClick={() => { onDuplicate(terminalId); onClose(); }}>
                    复制标签
                </div>
            )}
            <div style={menuSeparatorStyle} />
            <div style={menuItemStyle} onClick={() => { onCloseTerminal(terminalId); onClose(); }}>
                关闭
            </div>
            <div style={{
                ...menuItemStyle,
                color: terminals.length <= 1 ? '#555' : '#ccc',
                cursor: terminals.length <= 1 ? 'default' : 'pointer',
            }} onClick={() => {
                if (terminals.length > 1) {
                    terminals.forEach(t => {
                        if (t.id !== terminalId) onCloseTerminal(t.id);
                    });
                }
                onClose();
            }}>
                关闭其他
            </div>
            <div style={{
                ...menuItemStyle,
                color: idx >= 0 && idx < terminals.length - 1 ? '#ccc' : '#555',
                cursor: idx >= 0 && idx < terminals.length - 1 ? 'pointer' : 'default',
            }} onClick={() => {
                if (idx >= 0 && idx < terminals.length - 1) {
                    terminals.slice(idx + 1).forEach(t => onCloseTerminal(t.id));
                }
                onClose();
            }}>
                关闭右侧全部
            </div>
            <div style={menuSeparatorStyle} />
            <div style={menuItemStyle} onClick={() => {
                if (term?.config) {
                    const cfg = term.config;
                    let text = `${cfg.user}@${cfg.host}:${cfg.port}`;
                    if (cfg.bastion?.host) {
                        text += ` (via ${cfg.bastion.user}@${cfg.bastion.host}:${cfg.bastion.port})`;
                    }
                    navigator.clipboard.writeText(text);
                }
                onClose();
            }}>
                复制连接信息
            </div>
            {onOpenFileTransfer && (
                <div style={menuItemStyle} onClick={() => {
                    onOpenFileTransfer(terminalId);
                    onClose();
                }}>
                    文件传输
                </div>
            )}
        </div>
    );
};

const menuItemStyle: React.CSSProperties = {
    padding: '6px 12px',
    cursor: 'pointer',
    fontSize: '13px',
    color: '#ccc',
};

const menuSeparatorStyle: React.CSSProperties = {
    height: '1px',
    backgroundColor: '#454545',
    margin: '4px 0',
};

export default FlexLayoutAdapter;
