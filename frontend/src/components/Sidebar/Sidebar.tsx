import React, { useState, useRef, useEffect } from 'react';
import SessionManager from './SessionManager';
import TroubleshootingPanel from './TroubleshootingPanel';
import AIChatPanel from './AIChatPanel';
import KnowledgeBrowser from './KnowledgeBrowser';
import ScriptRecordingPanel from '../ScriptPanel/ScriptRecordingPanel';
import ScriptListPanel from '../ScriptPanel/ScriptListPanel';
import ScriptEditorModal from '../ScriptPanel/ScriptEditorModal';
import { ConnectionConfig } from '../../types';
import { useToast } from '../Toast/Toast';
import { confirmDialog } from '../ConfirmDialog/ConfirmDialog';
import { KnowledgeTarget } from '../AI';

type SidebarTab = 'sessions' | 'troubleshoot' | 'chat' | 'script' | 'knowledge';

interface TerminalSessionLite {
    id: string;
    title: string;
}

interface SidebarProps {
    isOpen: boolean;
    activeTab: SidebarTab;
    onToggle: () => void;
    onConnect: (config: ConnectionConfig) => void;
    activeTerminalId: string | null;
    terminals: TerminalSessionLite[];
    onTypeCommand?: (command: string) => void;
    onOpenKnowledgeSource?: (target: Omit<KnowledgeTarget, 'requestId'>) => void;
    knowledgeTarget?: KnowledgeTarget | null;
}

const getDefaultWidth = (tab: SidebarTab): number => {
    if (tab === 'knowledge') return Math.max(Math.floor(document.body.clientWidth * 0.6), 500);
    if (tab === 'troubleshoot' || tab === 'chat') {
        return Math.min(520, Math.max(420, Math.floor(document.body.clientWidth * 0.34)));
    }
    return 300;
};

const Sidebar: React.FC<SidebarProps> = ({
    isOpen, activeTab, onToggle, onConnect, activeTerminalId, terminals,
    onTypeCommand, onOpenKnowledgeSource, knowledgeTarget,
}) => {
    const [widths, setWidths] = useState<Record<SidebarTab, number>>(() => ({
        sessions: 300,
        troubleshoot: getDefaultWidth('troubleshoot'),
        chat: getDefaultWidth('chat'),
        script: 300,
        knowledge: getDefaultWidth('knowledge'),
    }));
    const width = widths[activeTab];
    const [editingScriptId, setEditingScriptId] = useState<string | null>(null);
    const scriptListRef = useRef<any>(null);
    const toast = useToast();

    useEffect(() => {
        const handleResize = () => setWidths(previous => ({
            ...previous,
            [activeTab]: Math.min(previous[activeTab], Math.max(300, document.body.clientWidth - 120)),
        }));
        window.addEventListener('resize', handleResize);
        return () => window.removeEventListener('resize', handleResize);
    }, [activeTab]);

    const handleEditScript = (scriptId: string) => {
        setEditingScriptId(scriptId);
    };

    const handleCloseEditor = () => {
        setEditingScriptId(null);
    };

    const handleSaveScript = () => {
        // 刷新脚本列表
        if (scriptListRef.current) {
            scriptListRef.current.loadScripts();
        }
    };

    const handleReplayScript = async (scriptId: string) => {
        if (!activeTerminalId) {
            toast.warning('请先连接到SSH会话');
            return;
        }

        const ok = await confirmDialog.show({
            title: '回放脚本',
            message: '确定要在当前会话中回放此脚本吗？',
            confirmText: '回放',
            danger: false,
        });
        if (!ok) return;

        try {
            // @ts-ignore
            await window.go.main.App.ReplayScript(scriptId, activeTerminalId);
            toast.success('脚本回放完成');
        } catch (err: any) {
            toast.error('回放失败: ' + err.message);
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
                 setWidths(previous => ({ ...previous, [activeTab]: newWidth }));
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
            case 'knowledge': return '知识库';
            case 'script': return '脚本录制';
            default: return '侧边栏';
        }
    };

    return (
        <>
        <div className={`sidebar-shell sidebar-${activeTab}`} style={{
            ...styles.container,
            width: isOpen ? width : 0,
            position: 'relative',
            // When closed, hide border and content but keep mounted
            borderLeft: isOpen ? '1px solid var(--border)' : 'none',
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
                <div className="sidebar-header" style={styles.header}>
                    <div className="sidebar-title-group">
                        <h3 style={styles.title}>{getTitle()}</h3>
                        {(activeTab === 'troubleshoot' || activeTab === 'chat') && <span className="sidebar-ai-badge">AI</span>}
                    </div>
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
                            <TroubleshootingPanel
                                activeTerminalTitle={terminals.find(terminal => terminal.id === activeTerminalId)?.title}
                                onTypeCommand={onTypeCommand}
                                onOpenSource={onOpenKnowledgeSource}
                            />
                        </div>

                        <div style={{ display: activeTab === 'chat' ? 'flex' : 'none', flex: 1, flexDirection: 'column', height: '100%', overflow: 'hidden' }}>
                            <AIChatPanel
                                activeTerminalTitle={terminals.find(terminal => terminal.id === activeTerminalId)?.title}
                                onOpenSource={onOpenKnowledgeSource}
                            />
                        </div>

                        {/* Knowledge Browser */}
                        <div style={{ display: activeTab === 'knowledge' ? 'flex' : 'none', flex: 1, flexDirection: 'column', height: '100%', overflow: 'hidden' }}>
                            <KnowledgeBrowser target={knowledgeTarget} />
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

        {/* Script Editor Modal */}
        <ScriptEditorModal
            isOpen={editingScriptId !== null}
            scriptId={editingScriptId}
            onClose={handleCloseEditor}
            onSave={handleSaveScript}
        />
        </>
    );
};

const styles = {
    container: {
        backgroundColor: 'var(--bg-secondary)',
        borderLeft: '1px solid var(--border)',
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
        backgroundColor: 'var(--bg-secondary)',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        borderBottom: '1px solid var(--border)',
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
        color: 'var(--text-primary)',
    },
    closeButton: {
        background: 'none',
        border: 'none',
        color: 'var(--text-secondary)',
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
