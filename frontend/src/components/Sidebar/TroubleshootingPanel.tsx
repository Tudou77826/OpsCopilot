import React, { useState, useEffect, useRef } from 'react';
import { TbArrowRight, TbFileDescription, TbSparkles, TbX } from 'react-icons/tb';
import SessionReviewModal, { ArchiveParams } from './SessionReviewModal';
import { confirmDialog } from '../ConfirmDialog/ConfirmDialog';
import {
    AIComposer,
    AgentTraceEvent,
    KnowledgeAnswer,
    KnowledgeTarget,
    RetrievalTrace,
    RichText,
    parseKnowledgeResponse,
    referencesFromDocuments,
} from '../AI';
// @ts-ignore
import { EventsOn } from '../../../wailsjs/runtime/runtime';

interface Message {
    role: 'user' | 'ai';
    content: string;
    timestamp: number;
    trace?: AgentTraceEvent[];
    references?: string[];
}

interface TroubleshootingPanelProps {
    activeTerminalTitle?: string;
    onTypeCommand?: (command: string) => void;
    onOpenSource?: (target: Omit<KnowledgeTarget, 'requestId'>) => void;
}

const TroubleshootingPanel: React.FC<TroubleshootingPanelProps> = ({ activeTerminalTitle, onTypeCommand, onOpenSource }) => {
    const [isInvestigating, setIsInvestigating] = useState(false);
    const [input, setInput] = useState('');
    const [messages, setMessages] = useState<Message[]>([]);
    const [agentStatus, setAgentStatus] = useState<{ stage: string; message: string } | null>(null);
    const [agentStatusHistory, setAgentStatusHistory] = useState<AgentTraceEvent[]>([]);
    const agentTraceRef = useRef<AgentTraceEvent[]>([]);
    const [contextUsage, setContextUsage] = useState<{ used: number; max: number } | null>(null);
    const [catalogMatches, setCatalogMatches] = useState<string[]>([]);
    const catalogMatchRef = useRef<string[]>([]);
    const [lastUsedDocs, setLastUsedDocs] = useState<string[]>([]);
    const usedDocsRef = useRef<Set<string>>(new Set());
    const [isStopping, setIsStopping] = useState(false);
    const [rootCause, setRootCause] = useState('');
    const [originalProblem, setOriginalProblem] = useState('');
    const [isReviewModalOpen, setIsReviewModalOpen] = useState(false);
    const [isPolishing, setIsPolishing] = useState(false);
    const messagesEndRef = useRef<HTMLDivElement>(null);

    const extractDocFromReadingMessage = (message: string): string | null => {
        const idx = message.indexOf('正在阅读文档:');
        if (idx === -1) return null;
        const after = message.slice(idx + '正在阅读文档:'.length).trim();
        return after.replace(/\.\.\.$/, '').trim() || null;
    };

    const scrollToBottom = () => {
        messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
    };

    useEffect(() => {
        scrollToBottom();
    }, [messages, agentStatus]);

    const handleStart = async () => {
        if (!input.trim()) {
            setMessages([{
                role: 'ai',
                content: '请先在下方输入您遇到的问题，然后点击"发送"开始排查。',
                timestamp: Date.now()
            }]);
            return;
        }

        setIsInvestigating(true);

        const problem = input;
        setOriginalProblem(problem);

        setMessages(prev => [...prev, {
            role: 'user',
            content: problem,
            timestamp: Date.now()
        }]);
        
        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.StartSession) {
            // @ts-ignore
            await window.go.main.App.StartSession(problem);
        }

        try {
            setAgentStatus({ stage: 'thinking', message: '正在分析问题，扫描知识库目录...' });
            setAgentStatusHistory([]);
            agentTraceRef.current = [];
            setCatalogMatches([]);
            catalogMatchRef.current = [];
            setLastUsedDocs([]);
            usedDocsRef.current = new Set();
            let cancelStatus: (() => void) | undefined;
            let cancelContext: (() => void) | undefined;
            try {
                if (EventsOn) {
                    cancelStatus = EventsOn("agent:status", (...args: any[]) => {
                        const data = args?.[0] ?? {};
                        const stage = String(data?.stage ?? '');
                        const message = String(data?.message ?? '');
                        const runId = data?.runId ? String(data.runId) : undefined;
                        if (!stage || !message) return;

                        setAgentStatus({ stage, message });
                        const last = agentTraceRef.current[agentTraceRef.current.length - 1];
                        if (!last || last.stage !== stage || last.message !== message) {
                            agentTraceRef.current = [...agentTraceRef.current, { runId, stage, message, ts: Date.now() }].slice(-8);
                            setAgentStatusHistory(agentTraceRef.current);
                        }

                        if (stage === 'reading') {
                            const doc = extractDocFromReadingMessage(message);
                            if (doc) usedDocsRef.current.add(doc);
                        }
                        if (stage === 'catalog_match') {
                            catalogMatchRef.current = [...catalogMatchRef.current, message];
                            setCatalogMatches([...catalogMatchRef.current]);
                        }
                    });
                    cancelContext = EventsOn("agent:context", (...args: any[]) => {
                        const data = args?.[0] ?? {};
                        setContextUsage({
                            used: parseInt(data?.usedTokens ?? '0'),
                            max: parseInt(data?.maxTokens ?? '75000'),
                        });
                    });
                }
            } catch (err) {
                console.error("Failed to register event listener:", err);
            }

            // @ts-ignore
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.AskTroubleshoot) {
                // @ts-ignore
                const response = await window.go.main.App.AskTroubleshoot(problem);

                setMessages(prev => [...prev, {
                    role: 'ai',
                    content: response,
                    timestamp: Date.now(),
                    trace: [...agentTraceRef.current],
                    references: Array.from(usedDocsRef.current),
                }]);
            } else {
                 // Fallback to AskAI if AskTroubleshoot is not available (e.g. bindings not updated yet)
                 // @ts-ignore
                 if (window.go && window.go.main && window.go.main.App && window.go.main.App.AskAI) {
                    // @ts-ignore
                    const response = await window.go.main.App.AskAI(problem);
                    setMessages(prev => [...prev, {
                        role: 'ai',
                        content: response,
                        timestamp: Date.now(),
                        trace: [...agentTraceRef.current],
                        references: Array.from(usedDocsRef.current),
                    }]);
                }
            }
            if (cancelStatus) cancelStatus();
            if (cancelContext) cancelContext();
        } catch (e: any) {
            console.error("Initial AI analysis failed", e);
        } finally {
            setAgentStatus(null);
            setLastUsedDocs(Array.from(usedDocsRef.current));
            setContextUsage(null);
        }
        
        setInput('');
    };

    const handleCancelClick = async () => {
        const ok = await confirmDialog.show({
            message: '确定要取消定位吗？这将清空当前的所有记录。',
            danger: true,
        });
        if (ok) {
            try {
                // @ts-ignore
                await window.go.main.App.CancelSession();
            } catch (err) {
                console.error('CancelSession error:', err);
            }
            handleReset();
        }
    };

    const handleStopClick = () => {
        setIsStopping(true);
    };

    const handleConfirmStop = async () => {
        setIsReviewModalOpen(true);
    };

    const handleReset = () => {
        setIsInvestigating(false);
        setInput('');
        setMessages([]);
        setAgentStatus(null);
        setAgentStatusHistory([]);
        agentTraceRef.current = [];
        setCatalogMatches([]);
        catalogMatchRef.current = [];
        setLastUsedDocs([]);
        usedDocsRef.current = new Set();
        setIsStopping(false);
        setRootCause('');
        setOriginalProblem('');
    };

    const handleArchive = async (params: ArchiveParams): Promise<{ success: boolean; message: string }> => {
        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.ArchiveSession) {
            try {
                // @ts-ignore
                const result = await window.go.main.App.ArchiveSession(rootCause, params.conclusion, params.service, params.module, params.targetFile);
                const parsed = JSON.parse(result);
                if (parsed.success) {
                    setMessages(prev => [...prev, {
                        role: 'ai',
                        content: parsed.conclusion || '会话已结束并归档。',
                        timestamp: Date.now()
                    }]);
                    // 不在这里关闭 Modal，让 SessionReviewModal 显示庆祝动画后自己关闭
                    // 清理状态由 SessionReviewModal 的 onClose 回调处理
                    return { success: true, message: '归档成功' };
                } else {
                    return { success: false, message: parsed.error || '未知错误' };
                }
            } catch (e) {
                return { success: false, message: `归档异常: ${e}` };
            }
        } else {
            setMessages(prev => [...prev, {
                role: 'ai',
                content: '会话已结束。',
                timestamp: Date.now()
            }]);
            return { success: true, message: '会话已结束' };
        }
    };

    const handlePolishRootCause = async () => {
        if (!rootCause.trim()) return;
        setIsPolishing(true);
        try {
            // @ts-ignore
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.PolishRootCause) {
                // @ts-ignore
                const polished = await window.go.main.App.PolishRootCause(rootCause);
                if (polished && !polished.startsWith("Error")) {
                    setRootCause(polished);
                }
            }
        } catch (e) {
            console.error("Polish failed", e);
        } finally {
            setIsPolishing(false);
        }
    };

    const handleSend = async () => {
        if (!input.trim()) return;
        
        const userMsg: Message = {
            role: 'user',
            content: input,
            timestamp: Date.now()
        };
        
        setMessages(prev => [...prev, userMsg]);
        setInput('');

        setAgentStatus({ stage: 'thinking', message: '正在分析问题，扫描知识库目录...' });
        setAgentStatusHistory([]);
        agentTraceRef.current = [];
        setCatalogMatches([]);
        catalogMatchRef.current = [];
        setLastUsedDocs([]);
        usedDocsRef.current = new Set();
        let cancelStatus: (() => void) | undefined;
        let cancelContext: (() => void) | undefined;
        try {
            if (EventsOn) {
                cancelStatus = EventsOn("agent:status", (...args: any[]) => {
                    const data = args?.[0] ?? {};
                    const stage = String(data?.stage ?? '');
                    const message = String(data?.message ?? '');
                    const runId = data?.runId ? String(data.runId) : undefined;
                    if (!stage || !message) return;

                    setAgentStatus({ stage, message });
                    const last = agentTraceRef.current[agentTraceRef.current.length - 1];
                    if (!last || last.stage !== stage || last.message !== message) {
                        agentTraceRef.current = [...agentTraceRef.current, { runId, stage, message, ts: Date.now() }].slice(-8);
                        setAgentStatusHistory(agentTraceRef.current);
                    }

                    if (stage === 'reading') {
                        const doc = extractDocFromReadingMessage(message);
                        if (doc) usedDocsRef.current.add(doc);
                    }
                    if (stage === 'catalog_match') {
                        catalogMatchRef.current = [...catalogMatchRef.current, message];
                        setCatalogMatches([...catalogMatchRef.current]);
                    }
                });
                cancelContext = EventsOn("agent:context", (...args: any[]) => {
                    const data = args?.[0] ?? {};
                    setContextUsage({
                        used: parseInt(data?.usedTokens ?? '0'),
                        max: parseInt(data?.maxTokens ?? '75000'),
                    });
                });
            }
        } catch (err) {
            console.error("Failed to register event listener:", err);
        }

        try {
            // @ts-ignore
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.AskTroubleshoot) {
                // @ts-ignore
                const response = await window.go.main.App.AskTroubleshoot(userMsg.content);

                setMessages(prev => [...prev, {
                    role: 'ai',
                    content: response,
                    timestamp: Date.now(),
                    trace: [...agentTraceRef.current],
                    references: Array.from(usedDocsRef.current),
                }]);
            } else {
                 // Fallback to AskAI if AskTroubleshoot is not available (e.g. bindings not updated yet)
                 // @ts-ignore
                 if (window.go && window.go.main && window.go.main.App && window.go.main.App.AskAI) {
                    // @ts-ignore
                    const response = await window.go.main.App.AskAI(userMsg.content);

                    setMessages(prev => [...prev, {
                        role: 'ai',
                        content: response,
                        timestamp: Date.now(),
                        trace: [...agentTraceRef.current],
                        references: Array.from(usedDocsRef.current),
                    }]);
                } else {
                     setMessages(prev => [...prev, {
                        role: 'ai',
                        content: "Error: Backend not connected.",
                        timestamp: Date.now()
                    }]);
                }
            }
        } catch (e: any) {
            setMessages(prev => [...prev, {
                role: 'ai',
                content: "Error: " + e.toString(),
                timestamp: Date.now(),
                trace: [...agentTraceRef.current],
            }]);
        } finally {
            if (cancelStatus) cancelStatus();
            if (cancelContext) cancelContext();
            setAgentStatus(null);
            setLastUsedDocs(Array.from(usedDocsRef.current));
            setContextUsage(null);
        }
    };

    const handleKeyDown = (e: React.KeyboardEvent) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            handleSend();
        }
    };

    const preprocessContent = (content: string): string => {
        if (!content) return '';
        let text = content;
        try {
            const trimmed = text.trim();
            if (trimmed.startsWith('{')) {
                const data = JSON.parse(trimmed);
                if (!Array.isArray(data.steps) && !Array.isArray(data.commands)) {
                    for (const key of ['summary', 'content', 'answer', 'text']) {
                        if (typeof data[key] === 'string') {
                            text = data[key];
                            break;
                        }
                    }
                }
            }
        } catch {}
        return text.replace(/([^\n])\n(#{1,6} )/g, '$1\n\n$2');
    };

    const renderMessageContent = (message: Message) => {
        const structured = parseKnowledgeResponse(message.content);
        if (structured) {
            return (
                <KnowledgeAnswer
                    response={structured}
                    references={referencesFromDocuments(message.references || [])}
                    onTypeCommand={onTypeCommand}
                    onOpenSource={onOpenSource}
                />
            );
        }
        return <RichText content={preprocessContent(message.content)} />;
    };

    return (
        <div className="ai-panel ai-troubleshoot-panel" style={styles.container}>
            {isReviewModalOpen ? (
                <SessionReviewModal
                    isOpen={isReviewModalOpen}
                    onClose={() => {
                        setIsReviewModalOpen(false);
                    }}
                    rootCause={rootCause}
                    problem={originalProblem}
                    onArchive={handleArchive}
                    inline
                />
            ) : (
            <>
            {!isInvestigating ? (
                <div className="ai-empty-stage">
                    <div className="ai-empty-glyph" aria-hidden="true"><span>OC</span><i /><i /><i /></div>
                    <span className="ai-empty-eyebrow">OPS KNOWLEDGE ENGINE</span>
                    <h4>从现象开始定位</h4>
                    <p>描述错误、影响范围和发生时间，AI 将结合知识库给出可验证的定位思路。</p>
                    <div className="ai-empty-composer">
                        <AIComposer
                            value={input}
                            onChange={setInput}
                            onSend={handleStart}
                            placeholder="例如：服务器 CPU 占用率过高..."
                            contexts={activeTerminalTitle ? [{ id: 'terminal', label: `● SSH · ${activeTerminalTitle}`, active: true }] : []}
                        />
                    </div>
                </div>
            ) : (
                <>
                <div className="ai-session-bar">
                    <span className="ai-session-state"><i /> 定位会话</span>
                    <span className="ai-session-topic" title={originalProblem}>{originalProblem}</span>
                    <div className="ai-session-actions">
                        <button type="button" onClick={handleCancelClick} title="取消并清空当前定位">{TbX({ size: 14 })}<span>取消</span></button>
                    </div>
                </div>
                <div className="ai-conversation-scroll">
                    <div className="ai-message-list">
                        {messages.map((msg, idx) => (
                                <div key={idx} className={msg.role === 'user' ? 'ai-user-message' : 'ai-assistant-message'}>
                                    {msg.role === 'ai' ? (
                                        <>
                                            {msg.trace && msg.trace.length > 0 && <RetrievalTrace events={msg.trace} active={false} />}
                                            {renderMessageContent(msg)}
                                        </>
                                    ) : (
                                        <div>{msg.content}</div>
                                    )}
                                </div>
                            ))
                        }
                        <div ref={messagesEndRef} />
                        {agentStatus && <RetrievalTrace events={agentStatusHistory.length > 0 ? agentStatusHistory : [{ stage: agentStatus.stage, message: agentStatus.message, ts: Date.now() }]} active />}
                        {contextUsage && (() => {
                            const ratio = contextUsage.used / contextUsage.max;
                            const pct = Math.min(ratio * 100, 100);
                            return (
                                <div style={styles.contextBar}>
                                    <div style={styles.contextBarTrack}>
                                        <div style={{
                                            ...styles.contextBarFill,
                                            width: `${pct}%`,
                                            backgroundColor: ratio > 0.8 ? 'var(--severity-danger)' : 'var(--severity-info)',
                                        }} />
                                    </div>
                                    <span style={styles.contextBarLabel}>
                                        {Math.round(contextUsage.used / 1000)}K / {Math.round(contextUsage.max / 1000)}K
                                    </span>
                                </div>
                            );
                        })()}
                    </div>
                </div>
                </>
            )}

            {isInvestigating && (
                <div className="ai-panel-footer">
                    {isStopping ? (
                        <div className="ai-stop-confirmation">
                            <div className="ai-stop-confirmation-heading">
                                <strong>整理排查结论</strong>
                                <span>先补充根本原因，再进入结论编辑</span>
                            </div>
                            <div className="ai-stop-confirmation-input">
                                <input
                                    type="text"
                                    value={rootCause}
                                    onChange={(e) => setRootCause(e.target.value)}
                                    placeholder="简要补充根本原因（可稍后完善）"
                                    autoFocus
                                />
                                <button
                                    className="ai-stop-polish-button"
                                    onClick={handlePolishRootCause}
                                    title="AI 润色"
                                    disabled={isPolishing}
                                >
                                    {isPolishing ? '...' : TbSparkles({size: 14})}
                                </button>
                            </div>
                            <div className="ai-stop-confirmation-actions">
                                <button className="is-secondary" onClick={() => setIsStopping(false)}>返回排查</button>
                                <button className="is-primary" onClick={handleConfirmStop}><span>继续编辑结论</span>{TbArrowRight({ size: 14 })}</button>
                            </div>
                        </div>
                    ) : (
                        <>
                            <div className="ai-conclusion-transition">
                                <span>有明确结论后，可整理为知识记录</span>
                                <button type="button" onClick={handleStopClick} title="整理本次排查结论">
                                    {TbFileDescription({ size: 14 })}
                                    <span>整理排查结论</span>
                                </button>
                            </div>
                            <AIComposer
                                value={input}
                                onChange={setInput}
                                onSend={handleSend}
                                placeholder="补充现象或继续追问…"
                                contexts={activeTerminalTitle ? [{ id: 'terminal', label: `● SSH · ${activeTerminalTitle}`, active: true }] : []}
                                disabled={Boolean(agentStatus)}
                            />
                        </>
                    )}
                </div>
            )}
            </>
            )}
        </div>
    );
};

const styles = {
    container: {
        display: 'flex',
        flexDirection: 'column' as const,
        height: '100%',
        color: 'var(--text-secondary)',
    },
    emptyState: {
        flex: 1,
        display: 'flex',
        flexDirection: 'column' as const,
        alignItems: 'center',
        justifyContent: 'center',
        padding: '20px',
        color: 'var(--text-muted)',
    },
    icon: {
        fontSize: '48px',
        marginBottom: '16px',
    },
    emptyText: {
        marginBottom: '20px',
        textAlign: 'center' as const,
        lineHeight: 1.5,
    },
    emptyComposer: {
        width: '100%',
        maxWidth: '480px',
        margin: '0 auto',
    },
    stopButton: {
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
        padding: '8px 14px',
        backgroundColor: 'var(--accent)',
        color: 'var(--text-on-accent)',
        border: 'none',
        borderRadius: '6px',
        cursor: 'pointer',
        fontSize: '13px',
        fontWeight: '500',
        transition: 'all 0.2s ease',
        boxShadow: '0 2px 8px rgba(0, 122, 204, 0.3)',
    },
    stopButtonIcon: {
        fontSize: '16px',
        display: 'inline-flex',
        alignItems: 'center',
    },
    stopButtonText: {
        fontSize: '13px',
        fontWeight: '500',
    },
    cancelButton: {
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
        padding: '8px 14px',
        backgroundColor: 'var(--danger)',
        color: 'var(--text-on-accent)',
        border: 'none',
        borderRadius: '6px',
        cursor: 'pointer',
        fontSize: '13px',
        fontWeight: '500',
        transition: 'all 0.2s ease',
        boxShadow: '0 2px 8px rgba(244, 67, 54, 0.3)',
    },
    cancelButtonIcon: {
        fontSize: '16px',
        display: 'inline-flex',
        alignItems: 'center',
    },
    cancelButtonText: {
        fontSize: '13px',
        fontWeight: '500',
    },
    chatContainer: {
        flex: 1,
        display: 'flex',
        flexDirection: 'column' as const,
        minHeight: 0, // Critical for nested flex scrolling
        overflow: 'hidden' as const,
    },
    messageList: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '10px',
        overflowY: 'auto' as const,
        padding: '10px',
        flex: 1,
    },
    userMessageItem: {
        maxWidth: '85%',
        padding: '9px 11px',
        borderRadius: '8px 8px 2px 8px',
        backgroundColor: 'var(--accent)',
        color: 'var(--text-primary)',
        wordBreak: 'break-word' as const,
    },
    aiMessageItem: {
        width: '100%',
        maxWidth: '100%',
        padding: '4px 2px 10px',
        color: 'var(--text-primary)',
        wordBreak: 'break-word' as const,
    },
    messageContent: {
        fontSize: '13px',
        lineHeight: '1.4',
    },
    footer: {
        backgroundColor: 'var(--bg-secondary)',
    },
    toolbar: {
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '8px',
        padding: '8px 10px 0',
    },
    inputBox: {
        display: 'flex',
        gap: '8px',
    },
    textarea: {
        flex: 1,
        backgroundColor: 'var(--bg-input)',
        border: 'none',
        borderRadius: '4px',
        color: 'var(--text-primary)',
        padding: '8px',
        resize: 'none' as const,
        outline: 'none',
        fontFamily: 'inherit',
    },
    sendButton: {
        padding: '0 12px',
        backgroundColor: 'var(--accent)',
        color: 'var(--text-on-accent)',
        border: 'none',
        borderRadius: '4px',
        cursor: 'pointer',
    },
    statusIndicator: {
        padding: '8px 12px',
        color: 'var(--text-muted)',
        fontSize: '12px',
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
        backgroundColor: 'var(--bg-primary)',
        borderRadius: '6px',
        margin: '0 4px',
        border: '1px solid var(--bg-elevated)',
    },
    stageIcon: {
        fontSize: '14px',
        display: 'inline-flex',
        alignItems: 'center',
        animation: 'pulse 1.5s ease-in-out infinite',
    },
    stageLabel: {
        fontSize: '12px',
        fontWeight: '600' as const,
        whiteSpace: 'nowrap' as const,
    },
    stageMessage: {
        fontSize: '12px',
        color: 'var(--text-tertiary)',
        whiteSpace: 'nowrap' as const,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
    },
    stageBreadcrumb: {
        fontSize: '12px',
        color: 'var(--text-tertiary)',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
    },
    statusHistory: {
        padding: '4px 12px',
        borderLeft: '2px solid var(--bg-elevated)',
        marginLeft: '14px',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '3px',
    },
    statusHistoryLine: {
        display: 'flex',
        alignItems: 'center',
        gap: '5px',
        opacity: 0.5,
    },
    historyIcon: {
        fontSize: '11px',
        display: 'inline-flex',
    },
    historyLabel: {
        fontSize: '11px',
        fontWeight: '500' as const,
    },
    historyDetail: {
        fontSize: '11px',
        color: 'var(--text-disabled)',
        marginLeft: '2px',
    },
    usedDocsBox: {
        padding: '10px 12px',
        backgroundColor: 'var(--bg-primary)',
        border: '1px solid var(--border)',
        borderRadius: '8px',
        color: 'var(--text-tertiary)',
        maxWidth: '95%',
    },
    catalogPathChip: {
        padding: '2px 8px',
        borderRadius: '999px',
        backgroundColor: 'var(--bg-elevated)',
        border: '1px solid var(--border)',
        color: 'var(--text-secondary)',
        fontSize: '11px',
    },
    catalogPathSep: {
        color: 'var(--text-disabled)',
        margin: '0 1px',
    },
    usedDocsTitle: {
        fontSize: '12px',
        color: 'var(--text-muted)',
        marginBottom: '8px',
    },
    usedDocsList: {
        display: 'flex',
        flexWrap: 'wrap' as const,
        gap: '6px',
    },
    usedDocChip: {
        padding: '2px 8px',
        borderRadius: '999px',
        backgroundColor: 'var(--bg-elevated)',
        border: '1px solid var(--border)',
        color: 'var(--text-secondary)',
        fontSize: '12px',
    },
    contextBar: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        padding: '4px 12px',
        margin: '0 4px',
    },
    contextBarTrack: {
        flex: 1,
        height: '4px',
        borderRadius: '2px',
        backgroundColor: 'var(--bg-input)',
        overflow: 'hidden' as const,
    },
    contextBarFill: {
        height: '100%',
        borderRadius: '2px',
        transition: 'width 0.3s ease',
    },
    contextBarLabel: {
        fontSize: '10px',
        color: 'var(--text-disabled)',
        whiteSpace: 'nowrap' as const,
    },
    structuredResponse: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '12px',
    },
    section: {
        display: 'flex',
        flexDirection: 'column' as const,
    },
    sectionTitle: {
        margin: '0 0 8px 0',
        fontSize: '12px',
        color: 'var(--text-tertiary)',
        textTransform: 'uppercase' as const,
        letterSpacing: '0.5px',
    },
};

const existing = document.getElementById('opscopilot-animations');
if (!existing) {
    const styleSheet = document.createElement("style");
    styleSheet.id = 'opscopilot-animations';
    styleSheet.textContent = `
        @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.5; } }
    `;
    document.head.appendChild(styleSheet);
}

export default TroubleshootingPanel;
