import React, { useState } from 'react';
import SessionManager from './SessionManager';
import TroubleshootingPanel from './TroubleshootingPanel';
import AIChatPanel from './AIChatPanel';
import MonitoringPanel from './MonitoringPanel';
import { ConnectionConfig } from '../../types';

interface TerminalSessionLite {
    id: string;
    title: string;
}

interface SidebarProps {
    isOpen: boolean;
    activeTab: 'sessions' | 'troubleshoot' | 'chat' | 'monitoring';
    onToggle: () => void;
    onStart?: () => void;
    onStop?: () => void;
    onConnect: (config: ConnectionConfig) => void;
    activeTerminalId: string | null;
    terminals: TerminalSessionLite[];
    experimentalMonitoringEnabled: boolean;
}

const Sidebar: React.FC<SidebarProps> = ({ isOpen, activeTab, onToggle, onStart, onStop, onConnect, activeTerminalId, terminals, experimentalMonitoringEnabled }) => {
    const [width, setWidth] = useState(350);

    const startResizing = (mouseDownEvent: React.MouseEvent) => {
        mouseDownEvent.preventDefault();

        const doDrag = (mouseMoveEvent: MouseEvent) => {
             // Calculate width from right edge: Window Width - Mouse X
             const newWidth = document.body.clientWidth - mouseMoveEvent.clientX;
             if (newWidth > 250 && newWidth < 800) {
                 setWidth(newWidth);
             }
        };

        const stopDrag = () => {
            document.removeEventListener('mousemove', doDrag);
            document.removeEventListener('mouseup', stopDrag);
            document.body.style.cursor = 'default';
        };

        document.addEventListener('mousemove', doDrag);
        document.addEventListener('mouseup', stopDrag);
        document.body.style.cursor = 'col-resize';
    };

    const getTitle = () => {
        switch (activeTab) {
            case 'sessions': return '会话管理';
            case 'troubleshoot': return '定位助手';
            case 'chat': return 'AI 问答';
            case 'monitoring': return '监控';
            default: return '侧边栏';
        }
    };

    return (
        <div style={{
            ...styles.container, 
            width: isOpen ? width : 0, 
            position: 'relative',
            // When closed, hide border and content but keep mounted
            borderLeft: isOpen ? '1px solid #333' : 'none',
        }}>
            <style>{`
                .hide-scrollbar::-webkit-scrollbar {
                    display: none;
                }
                .hide-scrollbar {
                    -ms-overflow-style: none;
                    scrollbar-width: none;
                }
            `}</style>
            
            {/* Only show resize handle when open */}
            {isOpen && (
                <div
                    style={styles.resizeHandle}
                    onMouseDown={startResizing}
                />
            )}

            {/* Content Container - Hide when closed to avoid layout issues */}
            <div style={{ 
                display: isOpen ? 'flex' : 'none', 
                flexDirection: 'column', 
                height: '100%',
                flex: 1 
            }}>
                {/* Header */}
                <div style={styles.header}>
                    <h3 style={styles.title}>{getTitle()}</h3>
                    <button onClick={onToggle} style={styles.closeButton} aria-label="Toggle Sidebar">×</button>
                </div>

                <div style={styles.mainArea}>
                    {/* Content Area */}
                    <div style={styles.content}>
                        {/* SessionManager might not need persistence, but we can keep it consistent */}
                        <div style={{ display: activeTab === 'sessions' ? 'flex' : 'none', flex: 1, flexDirection: 'column', height: '100%', overflow: 'hidden' }}>
                             <SessionManager onConnect={onConnect} />
                        </div>
                        
                        {/* Always mounted, toggled visibility */}
                        <div style={{ display: activeTab === 'troubleshoot' ? 'flex' : 'none', flex: 1, flexDirection: 'column', height: '100%', overflow: 'hidden' }}>
                            <TroubleshootingPanel onStart={onStart} onStop={onStop} />
                        </div>

                        <div style={{ display: activeTab === 'chat' ? 'flex' : 'none', flex: 1, flexDirection: 'column', height: '100%', overflow: 'hidden' }}>
                            <AIChatPanel />
                        </div>

                        <div style={{ display: activeTab === 'monitoring' ? 'flex' : 'none', flex: 1, flexDirection: 'column', height: '100%', minHeight: 0, overflowY: 'auto', overflowX: 'hidden' }} className="hide-scrollbar">
                            {experimentalMonitoringEnabled ? (
                                <MonitoringPanel activeTerminalId={activeTerminalId} terminals={terminals} />
                            ) : (
                                <div style={{ padding: '12px', color: '#aaa' }}>
                                    该功能为实验功能，默认关闭。请在“设置 → 应用选项”中开启后使用。
                                </div>
                            )}
                        </div>
                    </div>
                </div>
            </div>
        </div>
    );
};

const styles = {
    container: {
        backgroundColor: '#252526',
        borderLeft: '1px solid #333',
        display: 'flex',
        flexDirection: 'column' as const,
        height: '100%',
    },
    resizeHandle: {
        position: 'absolute' as const,
        left: -3,
        top: 0,
        bottom: 0,
        width: '6px',
        cursor: 'col-resize',
        zIndex: 100,
        backgroundColor: 'transparent',
    },
    header: {
        padding: '10px 16px',
        backgroundColor: '#252526',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        borderBottom: '1px solid #333',
    },
    mainArea: {
        display: 'flex',
        flex: 1,
        overflow: 'hidden',
        minHeight: 0,
    },
    title: {
        margin: 0,
        fontSize: '14px',
        color: '#fff',
    },
    closeButton: {
        background: 'none',
        border: 'none',
        color: '#ccc',
        cursor: 'pointer',
        fontSize: '18px',
    },
    content: {
        flex: 1,
        overflow: 'hidden',
        display: 'flex',
        flexDirection: 'column' as const,
        minHeight: 0,
    },
};

export default Sidebar;
