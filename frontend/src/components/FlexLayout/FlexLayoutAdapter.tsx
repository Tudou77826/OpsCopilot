import React, { useRef, useState, useCallback, useEffect, useMemo } from 'react';
import { Layout, Model, TabNode, TabSetNode, BorderNode, Actions, DockLocation, ITabRenderValues } from 'flexlayout-react';
import TerminalComponent, { TerminalRef } from '../Terminal/Terminal';
import FilesPanel from '../Sidebar/FilesPanel';
import { HighlightRule, TerminalConfig } from '../Terminal/highlightTypes';
import { SessionStatus, normalizeProtocol, PROTOCOL_LABEL } from '../../types';
import { TimestampResult } from '../../utils/timestampParser';
import { createInitialLayout } from './layoutConfig';

const FILE_TRANSFER_TAB_ID = 'file-transfer';

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
    activeTerminalId?: string | null;
    onActiveTerminalChange?: (id: string | null) => void;
    isBroadcastMode?: boolean;
    broadcastIds?: string[];
    onToggleTerminalBroadcast?: (id: string) => void;
    onStartBroadcastFrom?: (id: string) => void;
    completionDelay?: number;
    terminalConfig?: TerminalConfig;
    onTerminalFontSizeChange?: (fontSize: number) => void;
    highlightRules?: HighlightRule[];
    onSelectionParsed?: (result: TimestampResult | null) => void;
}

const FlexLayoutAdapter: React.FC<FlexLayoutAdapterProps> = ({
    terminals,
    onTerminalData,
    terminalRefs,
    onCloseTerminal,
    onRenameTerminal,
    onDuplicateTerminal,
    onReconnect,
    activeTerminalId,
    onActiveTerminalChange,
    isBroadcastMode,
    broadcastIds,
    onToggleTerminalBroadcast,
    onStartBroadcastFrom,
    completionDelay,
    terminalConfig,
    onTerminalFontSizeChange,
    highlightRules,
    onSelectionParsed,
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

    const findFirstTabsetId = useCallback((): string | null => {
        let targetId: string | null = null;
        model.visitNodes((node: any) => {
            if (node.getType() === 'tabset' && !targetId) {
                targetId = node.getId();
            }
        });
        return targetId;
    }, [model]);

    const openFileTransferTab = useCallback((terminalId?: string | null) => {
        if (terminalId) {
            onActiveTerminalChange?.(terminalId);
        }

        if (model.getNodeById(FILE_TRANSFER_TAB_ID)) {
            model.doAction(Actions.selectTab(FILE_TRANSFER_TAB_ID));
            return;
        }

        const targetId = findFirstTabsetId();
        model.doAction(Actions.addTab(
            {
                type: 'tab',
                id: FILE_TRANSFER_TAB_ID,
                name: '文件传输',
                component: 'fileTransfer',
                enableRename: false,
            },
            targetId || model.getRootRow().getId(),
            DockLocation.CENTER,
            targetId ? -1 : 0,
            true,
        ));
    }, [findFirstTabsetId, model, onActiveTerminalChange]);

    // --- Sync terminals[] → flexlayout Model ---
    useEffect(() => {
        const currentIds = new Set(terminals.map(t => t.id));
        const prevIds = prevTerminalIdsRef.current;

        // Add missing terminals. A tab can disappear from the layout model while
        // the session is still present, for example after layout recovery/HMR.
        for (const term of terminals) {
            if (!prevIds.has(term.id) || !model.getNodeById(term.id)) {
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
                    continue; // skip the normal addTab below; later sessions can use the new tabset
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
            if (tabId === FILE_TRANSFER_TAB_ID) {
                return action;
            }
            onCloseTerminal(tabId);
            return undefined; // block flexlayout's own delete; our sync effect handles removal
        }
        if (action.type === Actions.RENAME_TAB) {
            const tabId = action.data.node;
            if (tabId === FILE_TRANSFER_TAB_ID) {
                return undefined;
            }
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
        if (node.getComponent() === 'fileTransfer') {
            return (
                <FilesPanel
                    activeTerminalId={activeTerminalId || null}
                    terminals={terminals}
                />
            );
        }

        const terminalId = node.getId();

        return (
            <TerminalWrapper
                terminalId={terminalId}
                terminalsMapRef={terminalsMapRef}
                onTerminalData={onTerminalData}
                terminalRefs={terminalRefs}
                onActiveTerminalChange={onActiveTerminalChange}
                completionDelay={completionDelay}
                terminalConfig={terminalConfig}
                onTerminalFontSizeChange={onTerminalFontSizeChange}
                highlightRules={highlightRules}
                onSelectionParsed={onSelectionParsed}
                node={node}
                isBroadcastMode={isBroadcastMode}
                broadcastIds={broadcastIds}
                onToggleTerminalBroadcast={onToggleTerminalBroadcast}
            />
        );
    }, [activeTerminalId, terminals, onTerminalData, terminalRefs, completionDelay, terminalConfig, onTerminalFontSizeChange, highlightRules, onSelectionParsed, isBroadcastMode, broadcastIds, onToggleTerminalBroadcast]);

    // --- Custom tab rendering ---
    const handleRenderTab = useCallback((node: TabNode, renderValues: ITabRenderValues) => {
        if (node.getComponent() === 'fileTransfer') {
            renderValues.leading = (
                <span style={{
                    color: '#58a6ff',
                    fontSize: '12px',
                    marginRight: '6px',
                    flexShrink: 0,
                }}>
                    ⇄
                </span>
            );
            renderValues.content = (
                <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>
                    文件传输
                </span>
            );
            return;
        }

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

        // 协议 chip:仅非默认协议(telnet)显示,与侧栏会话树风格一致
        // (胶囊形 + 低饱和橙文字 + 冷调深底)。
        if (normalizeProtocol(term.config?.protocol) === 'telnet') {
            parts.push(
                <span
                    key="protocol"
                    style={{
                        display: 'inline-block',
                        marginLeft: '6px',
                        padding: '1px 7px',
                        fontSize: '10px',
                        lineHeight: '1.5',
                        color: '#d08a3e',
                        backgroundColor: '#1a1a1a',
                        border: '1px solid #3a3a3a',
                        borderRadius: '999px',
                        flexShrink: 0,
                        userSelect: 'none',
                    }}
                    title="Telnet 协议"
                >
                    {PROTOCOL_LABEL.telnet}
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

            if (activeId && terminalsMapRef.current.has(activeId)) {
                onActiveTerminalChange?.(activeId);
            } else if (!activeId) {
                onActiveTerminalChange?.(null);
            }
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
        if (node instanceof TabNode && node.getId() !== FILE_TRANSFER_TAB_ID) {
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
                    onOpenFileTransfer={(id) => { openFileTransferTab(id); setContextMenu(null); }}
                    isBroadcastMode={isBroadcastMode}
                    broadcastIds={broadcastIds}
                    onToggleTerminalBroadcast={onToggleTerminalBroadcast ? (id) => { onToggleTerminalBroadcast(id); setContextMenu(null); } : undefined}
                    onStartBroadcastFrom={onStartBroadcastFrom ? (id) => { onStartBroadcastFrom(id); setContextMenu(null); } : undefined}
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
    onActiveTerminalChange?: (id: string | null) => void;
    completionDelay?: number;
    terminalConfig?: TerminalConfig;
    onTerminalFontSizeChange?: (fontSize: number) => void;
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
    onActiveTerminalChange,
    completionDelay,
    terminalConfig,
    onTerminalFontSizeChange,
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
        <div
            style={{ height: '100%', width: '100%', minWidth: 0, minHeight: 0, position: 'relative', overflow: 'hidden' }}
            onPointerDownCapture={() => onActiveTerminalChange?.(terminalId)}
            onFocusCapture={() => onActiveTerminalChange?.(terminalId)}
        >
            <TerminalComponent
                id={terminalId}
                sessionID={terminalId}
                onData={(data) => onTerminalData(terminalId, data)}
                completionDelay={completionDelay}
                terminalConfig={terminalConfig}
                onFontSizeChange={onTerminalFontSizeChange}
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
    isBroadcastMode?: boolean;
    broadcastIds?: string[];
    onToggleTerminalBroadcast?: (id: string) => void;
    onStartBroadcastFrom?: (id: string) => void;
    onClose: () => void;
}

const ContextMenu: React.FC<ContextMenuProps> = ({ x, y, terminalId, terminals, onCloseTerminal, onRename, onDuplicate, onOpenFileTransfer, isBroadcastMode, broadcastIds, onToggleTerminalBroadcast, onStartBroadcastFrom, onClose }) => {
    const term = terminals.find(t => t.id === terminalId);
    const idx = terminals.findIndex(t => t.id === terminalId);

    // 广播菜单项文案与行为随状态变化:
    //   未开启        → 开启广播(仅本标签)
    //   已开 + 在组内  → 退出广播组
    //   已开 + 不在组  → 加入广播组
    // 断开的终端置灰,不可操作。
    const broadcastProps = useMemo(() => {
        if (!onStartBroadcastFrom && !onToggleTerminalBroadcast) return null;
        const isConnected = term?.status === SessionStatus.CONNECTED;
        const inGroup = isBroadcastMode && (broadcastIds?.includes(terminalId) ?? false);
        let label: string;
        let onClick: (() => void) | null;
        if (!isBroadcastMode) {
            label = '📡 开启广播(仅本标签)';
            onClick = onStartBroadcastFrom ? () => onStartBroadcastFrom(terminalId) : null;
        } else if (inGroup) {
            label = '🔇 退出广播组';
            onClick = onToggleTerminalBroadcast ? () => onToggleTerminalBroadcast(terminalId) : null;
        } else {
            label = '📡 加入广播组';
            onClick = onToggleTerminalBroadcast ? () => onToggleTerminalBroadcast(terminalId) : null;
        }
        const disabled = !isConnected || !onClick;
        return { label, onClick: disabled ? null : onClick, disabled };
    }, [term, isBroadcastMode, broadcastIds, terminalId, onStartBroadcastFrom, onToggleTerminalBroadcast]);

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
            {broadcastProps && (
                <div
                    style={{
                        ...menuItemStyle,
                        color: broadcastProps.disabled ? '#555' : '#ccc',
                        cursor: broadcastProps.disabled ? 'default' : 'pointer',
                    }}
                    onClick={() => {
                        if (broadcastProps.disabled || !broadcastProps.onClick) return;
                        broadcastProps.onClick();
                        onClose();
                    }}
                >
                    {broadcastProps.label}
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
