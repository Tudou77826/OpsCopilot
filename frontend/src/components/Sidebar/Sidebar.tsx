import React, { useState, useRef } from 'react';
import SessionManager from './SessionManager';
import TroubleshootingPanel from './TroubleshootingPanel';
import AIChatPanel from './AIChatPanel';
import ScriptRecordingPanel from '../ScriptPanel/ScriptRecordingPanel';
import ScriptListPanel from '../ScriptPanel/ScriptListPanel';
import { ConnectionConfig } from '../../types';

interface TerminalSessionLite {
    id: string;
    title: string;
}

interface SidebarProps {
    isOpen: boolean;
    activeTab: 'sessions' | 'troubleshoot' | 'chat' | 'script';
    onToggle: () => void;
    onStart?: () => void;
    onStop?: () => void;
    onConnect: (config: ConnectionConfig) => void;
    activeTerminalId: string | null;
    terminals: TerminalSessionLite[];
}

const Sidebar: React.FC<SidebarProps> = ({ isOpen, activeTab, onToggle, onStart, onStop, onConnect, activeTerminalId, terminals }) => {
    const [width, setWidth] = useState(350);
    const [editingScriptId, setEditingScriptId] = useState<string | null>(null);
    const scriptListRef = useRef<any>(null);

    const handleEditScript = (scriptId: string) => {
        setEditingScriptId(scriptId);
        // TODO: Open script editor modal
    };

    const handleReplayScript = async (scriptId: string) => {
        if (!activeTerminalId) {
            alert('请先连接到SSH会话');
            return;
        }

        if (!confirm(`确定要在当前会话中回放此脚本吗？`)) {
            return;
        }

        try {
            // @ts-ignore
            await window.go.main.App.ReplayScript(scriptId, activeTerminalId);
            alert('脚本回放完成');
        } catch (err: any) {
            alert('回放失败: ' + err.message);
        }
    };

    const handleRecordingComplete = () => {
        // 录制完成后刷新脚本列表
        if (scriptListRef.current) {
            scriptListRef.current.loadScripts();
        }
    };

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
            case 'script': return '脚本录制';
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

                        {/* Script Recording Panel */}
                        <div style={{ display: activeTab === 'script' ? 'flex' : 'none', flex: 1, flexDirection: 'column', height: '100%', overflow: 'hidden' }}>
                            <ScriptRecordingPanel
                                activeSessionId={activeTerminalId}
                                onRecordingComplete={handleRecordingComplete}
                            />
                            <ScriptListPanel
                                ref={scriptListRef}
                                activeSessionId={activeTerminalId}
                                onEditScript={handleEditScript}
                                onReplayScript={handleReplayScript}
                            />
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
