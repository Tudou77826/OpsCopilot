import React, { useState, useRef } from 'react';
import { ProductSidebar } from '../../../../frontend-shell/src/ui/product/ProductSidebar';
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

const Sidebar: React.FC<SidebarProps> = ({
    isOpen, activeTab, onToggle, onConnect, activeTerminalId, terminals,
    onTypeCommand, onOpenKnowledgeSource, knowledgeTarget,
}) => {
    const [editingScriptId, setEditingScriptId] = useState<string | null>(null);
    const scriptListRef = useRef<any>(null);
    const toast = useToast();

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

    return (
        <>
        <ProductSidebar isOpen={isOpen} activeTab={activeTab} onToggle={onToggle}>
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
        </ProductSidebar>

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

export default Sidebar;
