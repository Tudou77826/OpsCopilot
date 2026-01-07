import React, { useState } from 'react';
import SessionManager from './SessionManager';
import TroubleshootingPanel from './TroubleshootingPanel';
import AIChatPanel from './AIChatPanel';
import { ConnectionConfig } from '../../types';

interface SidebarProps {
    isOpen: boolean;
    activeTab: 'sessions' | 'troubleshoot' | 'chat';
    onToggle: () => void;
    onStart?: () => void;
    onStop?: () => void;
    onConnect: (config: ConnectionConfig) => void;
}

const Sidebar: React.FC<SidebarProps> = ({ isOpen, activeTab, onToggle, onStart, onStop, onConnect }) => {
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

    if (!isOpen) {
        return null;
    }

    const getTitle = () => {
        switch (activeTab) {
            case 'sessions': return '会话管理';
            case 'troubleshoot': return '定位助手';
            case 'chat': return 'AI 问答';
            default: return '侧边栏';
        }
    };

    return (
        <div style={{...styles.container, width: width, position: 'relative'}}>
            <style>{`
                .hide-scrollbar::-webkit-scrollbar {
                    display: none;
                }
                .hide-scrollbar {
                    -ms-overflow-style: none;
                    scrollbar-width: none;
                }
            `}</style>
            <div
                style={styles.resizeHandle}
                onMouseDown={startResizing}
            />

            {/* Header */}
            <div style={styles.header}>
                <h3 style={styles.title}>{getTitle()}</h3>
                <button onClick={onToggle} style={styles.closeButton} aria-label="Toggle Sidebar">×</button>
            </div>

            <div style={styles.mainArea}>
                {/* Content Area */}
                <div style={styles.content}>
                    {activeTab === 'sessions' && (
                        <SessionManager onConnect={onConnect} />
                    )}
                    {activeTab === 'troubleshoot' && (
                        <TroubleshootingPanel onStart={onStart} onStop={onStop} />
                    )}
                    {activeTab === 'chat' && (
                        <AIChatPanel />
                    )}
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
    },
};

export default Sidebar;
