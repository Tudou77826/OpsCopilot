import React, { useState, useEffect, useRef } from 'react';
import TroubleshootingStep from './TroubleshootingStep';
import CommandCard from './CommandCard';
import SessionReviewModal from './SessionReviewModal';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeHighlight from 'rehype-highlight';
import 'highlight.js/styles/github-dark.css';
// @ts-ignore
import { EventsOn } from '../../../wailsjs/runtime/runtime';

interface Message {
    role: 'user' | 'ai';
    content: string;
    timestamp: number;
}

interface AgentStatusEvent {
    runId?: string;
    stage: string;
    message: string;
    ts: number;
}

interface TroubleshootResult {
    opsCopilotAnswer: string;
    externalAnswer: string;
    integratedAnswer: string;
    opsCopilotReady: boolean;
    externalReady: boolean;
    integratedReady: boolean;
    externalError?: string;
}

interface TroubleshootingPanelProps {
    onStart?: () => void;
    onStop?: () => void;
}

const TroubleshootingPanel: React.FC<TroubleshootingPanelProps> = ({ onStart, onStop }) => {
    const [isInvestigating, setIsInvestigating] = useState(false);
    const [input, setInput] = useState('');
    const [messages, setMessages] = useState<Message[]>([]);
    const [agentStatus, setAgentStatus] = useState<{ stage: string; message: string } | null>(null);
    const [agentStatusHistory, setAgentStatusHistory] = useState<AgentStatusEvent[]>([]);
    const [lastUsedDocs, setLastUsedDocs] = useState<string[]>([]);
    const usedDocsRef = useRef<Set<string>>(new Set());
    const [isStopping, setIsStopping] = useState(false);
    const [rootCause, setRootCause] = useState('');
    const [isReviewModalOpen, setIsReviewModalOpen] = useState(false);
    const [isPolishing, setIsPolishing] = useState(false);
    const messagesEndRef = useRef<HTMLDivElement>(null);
    const [viewMode, setViewMode] = useState<'opscopilot' | 'external' | 'integrated'>('opscopilot');
    const [troubleshootResult, setTroubleshootResult] = useState<TroubleshootResult | null>(null);

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
                content: '请先在下方输入您遇到的问题，然后点击“发送”开始排查。',
                timestamp: Date.now()
            }]);
            return;
        }

        setIsInvestigating(true);
        if (onStart) onStart();
        
        const problem = input;
        
        setMessages(prev => [...prev, {
            role: 'user',
            content: problem,
            timestamp: Date.now()
        }]);
        
        setMessages(prev => [...prev, {
            role: 'ai',
            content: `已开始排查会话。问题描述：${problem}\n正在为您分析...`,
            timestamp: Date.now()
        }]);

        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.StartSession) {
            // @ts-ignore
            await window.go.main.App.StartSession(problem);
        }

        try {
            setAgentStatus({ stage: 'thinking', message: '正在分析问题...' });
            setAgentStatusHistory([]);
            setLastUsedDocs([]);
            usedDocsRef.current = new Set();
            let cancelStatus: (() => void) | undefined;
            try {
                if (EventsOn) {
                    cancelStatus = EventsOn("agent:status", (...args: any[]) => {
                        const data = args?.[0] ?? {};
                        const stage = String(data?.stage ?? '');
                        const message = String(data?.message ?? '');
                        const runId = data?.runId ? String(data.runId) : undefined;
                        if (!stage || !message) return;

                        setAgentStatus({ stage, message });
                        setAgentStatusHistory(prev => {
                            const last = prev[prev.length - 1];
                            if (last && last.stage === stage && last.message === message) return prev;
                            const next = [...prev, { runId, stage, message, ts: Date.now() }];
                            return next.slice(-8);
                        });

                        if (stage === 'reading') {
                            const doc = extractDocFromReadingMessage(message);
                            if (doc) usedDocsRef.current.add(doc);
                        }
                    });
                }
            } catch (err) {
                console.error("Failed to register event listener:", err);
            }

            // @ts-ignore
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.AskTroubleshoot) {
                // @ts-ignore
                const response = await window.go.main.App.AskTroubleshoot(problem);
                let parsedResponse = response;
                let result: TroubleshootResult | null = null;

                try {
                    const jsonMatch = response.match(/\{[\s\S]*\}/);
                    if (jsonMatch) {
                        result = JSON.parse(jsonMatch[0]);
                        if (result && result.opsCopilotReady) {
                            parsedResponse = result.opsCopilotAnswer;
                        }
                    }
                } catch (e) {
                    console.error('Failed to parse troubleshoot result:', e);
                }

                if (result) {
                    setTroubleshootResult(result);
                } else {
                    setMessages(prev => [...prev, {
                        role: 'ai',
                        content: parsedResponse,
                        timestamp: Date.now()
                    }]);
                }
            } else {
                 // Fallback to AskAI if AskTroubleshoot is not available (e.g. bindings not updated yet)
                 // @ts-ignore
                 if (window.go && window.go.main && window.go.main.App && window.go.main.App.AskAI) {
                    // @ts-ignore
                    const response = await window.go.main.App.AskAI(problem);
                    setMessages(prev => [...prev, {
                        role: 'ai',
                        content: response,
                        timestamp: Date.now()
                    }]);
                }
            }
            if (cancelStatus) cancelStatus();
        } catch (e: any) {
            console.error("Initial AI analysis failed", e);
        } finally {
            setAgentStatus(null);
            setAgentStatusHistory([]);
            setLastUsedDocs(Array.from(usedDocsRef.current));
        }
        
        setInput('');
    };

    const handleStopClick = () => {
        setIsStopping(true);
    };

    const handleConfirmStop = async () => {
        setIsReviewModalOpen(true);
    };

    const handleArchive = async (conclusion: string) => {
        setIsReviewModalOpen(false);
        setIsStopping(false);
        setIsInvestigating(false);
        if (onStop) onStop();

        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.StopSession) {
            // @ts-ignore
            await window.go.main.App.StopSession(rootCause, conclusion);
            
            setMessages(prev => [...prev, {
                role: 'ai',
                content: conclusion || '会话已结束并归档。',
                timestamp: Date.now()
            }]);
        } else {
            setMessages(prev => [...prev, {
                role: 'ai',
                content: '会话已结束。',
                timestamp: Date.now()
            }]);
        }
        
        setRootCause('');
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

        setAgentStatus({ stage: 'thinking', message: '正在分析...' });
        setAgentStatusHistory([]);
        setLastUsedDocs([]);
        usedDocsRef.current = new Set();
        let cancelStatus: (() => void) | undefined;
        try {
            if (EventsOn) {
                cancelStatus = EventsOn("agent:status", (...args: any[]) => {
                    const data = args?.[0] ?? {};
                    const stage = String(data?.stage ?? '');
                    const message = String(data?.message ?? '');
                    const runId = data?.runId ? String(data.runId) : undefined;
                    if (!stage || !message) return;

                    setAgentStatus({ stage, message });
                    setAgentStatusHistory(prev => {
                        const last = prev[prev.length - 1];
                        if (last && last.stage === stage && last.message === message) return prev;
                        const next = [...prev, { runId, stage, message, ts: Date.now() }];
                        return next.slice(-8);
                    });

                    if (stage === 'reading') {
                        const doc = extractDocFromReadingMessage(message);
                        if (doc) usedDocsRef.current.add(doc);
                    }
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
                let parsedResponse = response;
                let result: TroubleshootResult | null = null;
                
                try {
                    const jsonMatch = response.match(/\{[\s\S]*\}/);
                    if (jsonMatch) {
                        result = JSON.parse(jsonMatch[0]);
                        if (result && result.opsCopilotReady) {
                            parsedResponse = result.opsCopilotAnswer;
                        }
                    }
                } catch (e) {
                    console.error('Failed to parse troubleshoot result:', e);
                }

                if (result) {
                    setTroubleshootResult(result);
                } else {
                    setMessages(prev => [...prev, {
                        role: 'ai',
                        content: parsedResponse,
                        timestamp: Date.now()
                    }]);
                }
            } else {
                 // Fallback to AskAI if AskTroubleshoot is not available (e.g. bindings not updated yet)
                 // @ts-ignore
                 if (window.go && window.go.main && window.go.main.App && window.go.main.App.AskAI) {
                    // @ts-ignore
                    const response = await window.go.main.App.AskAI(userMsg.content);
                    
                    setMessages(prev => [...prev, {
                        role: 'ai',
                        content: response,
                        timestamp: Date.now()
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
                timestamp: Date.now()
            }]);
        } finally {
            if (cancelStatus) cancelStatus();
            setAgentStatus(null);
            setAgentStatusHistory([]);
            setLastUsedDocs(Array.from(usedDocsRef.current));
        }
    };

    const handleKeyDown = (e: React.KeyboardEvent) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            handleSend();
        }
    };

    const renderMessageContent = (content: string) => {
        try {
            // Check if content looks like JSON before parsing
            let jsonContent = content.trim();

            // Try to strip Markdown code blocks if present (frontend fallback)
            const markdownMatch = jsonContent.match(/```(?:json)?\s*([\s\S]*?)\s*```/);
            if (markdownMatch) {
                jsonContent = markdownMatch[1].trim();
            }

            if (jsonContent.startsWith('{')) {
                const data = JSON.parse(jsonContent);
                if (data && (Array.isArray(data.steps) || Array.isArray(data.commands) || data.summary)) {
                    return (
                        <div style={styles.structuredResponse}>
                            {/* Summary section - shows comprehensive analysis */}
                            {data.summary && (
                                <div style={styles.section}>
                                    <h4 style={styles.sectionTitle}>综合分析</h4>
                                    <div style={{...styles.messageContent, paddingBottom: '12px'}}>
                                        <ReactMarkdown
                                            remarkPlugins={[remarkGfm]}
                                            rehypePlugins={[rehypeHighlight]}
                                            components={{
                                                h1: ({node, ...props}) => <h1 style={{...props.style, fontSize: '1.3em', fontWeight: 'bold', marginBottom: '0.5em', marginTop: '0.8em'}} {...props} />,
                                                h2: ({node, ...props}) => <h2 style={{...props.style, fontSize: '1.15em', fontWeight: 'bold', marginBottom: '0.5em', marginTop: '0.6em'}} {...props} />,
                                                h3: ({node, ...props}) => <h3 style={{...props.style, fontSize: '1.05em', fontWeight: 'bold', marginBottom: '0.5em', marginTop: '0.5em'}} {...props} />,
                                                p: ({node, ...props}) => <p style={{...props.style, marginBottom: '0.6em', lineHeight: '1.5'}} {...props} />,
                                                ul: ({node, ...props}) => <ul style={{...props.style, paddingLeft: '1.5em', marginBottom: '0.6em'}} {...props} />,
                                                ol: ({node, ...props}) => <ol style={{...props.style, paddingLeft: '1.5em', marginBottom: '0.6em'}} {...props} />,
                                                li: ({node, ...props}) => <li style={{...props.style, marginBottom: '0.25em'}} {...props} />,
                                                code: ({node, inline, ...props}: any) => inline
                                                    ? <code style={{backgroundColor: '#2a2a2a', padding: '2px 6px', borderRadius: '3px', fontSize: '0.9em'}} {...props} />
                                                    : <code style={{display: 'block', backgroundColor: '#1a1a1a', padding: '10px', borderRadius: '4px', overflowX: 'auto', marginBottom: '0.6em', fontSize: '0.85em'}} {...props} />,
                                                strong: ({node, ...props}) => <strong style={{fontWeight: 'bold'}} {...props} />,
                                                blockquote: ({node, ...props}) => <blockquote style={{borderLeft: '3px solid #555', paddingLeft: '0.8em', fontStyle: 'italic', color: '#999', marginBottom: '0.6em'}} {...props} />,
                                            }}
                                        >
                                            {data.summary}
                                        </ReactMarkdown>
                                    </div>
                                </div>
                            )}

                            {/* Steps section */}
                            {Array.isArray(data.steps) && data.steps.length > 0 && (
                                <div style={styles.section}>
                                    <h4 style={styles.sectionTitle}>排查思路</h4>
                                    {data.steps.map((step: any, idx: number) => (
                                        <TroubleshootingStep key={idx} step={step} index={idx} />
                                    ))}
                                </div>
                            )}

                            {/* Commands section */}
                            {Array.isArray(data.commands) && data.commands.length > 0 && (
                                <div style={styles.section}>
                                    <h4 style={styles.sectionTitle}>建议命令</h4>
                                    {data.commands.map((cmd: any, idx: number) => (
                                        <CommandCard
                                            key={idx}
                                            command={cmd.command}
                                            description={cmd.description}
                                        />
                                    ))}
                                </div>
                            )}
                        </div>
                    );
                }
            }
        } catch (e) {
            console.error("Failed to parse structured response:", e);
        }
        // Render as Markdown if not structured JSON
        return (
            <div style={styles.messageContent}>
                <ReactMarkdown
                    remarkPlugins={[remarkGfm]}
                    rehypePlugins={[rehypeHighlight]}
                    components={{
                        h1: ({node, ...props}) => <h1 style={{...props.style, fontSize: '1.5em', fontWeight: 'bold', marginBottom: '0.5em', marginTop: '1em'}} {...props} />,
                        h2: ({node, ...props}) => <h2 style={{...props.style, fontSize: '1.3em', fontWeight: 'bold', marginBottom: '0.5em', marginTop: '0.8em'}} {...props} />,
                        h3: ({node, ...props}) => <h3 style={{...props.style, fontSize: '1.1em', fontWeight: 'bold', marginBottom: '0.5em', marginTop: '0.6em'}} {...props} />,
                        p: ({node, ...props}) => <p style={{...props.style, marginBottom: '0.8em', lineHeight: '1.5'}} {...props} />,
                        ul: ({node, ...props}) => <ul style={{...props.style, paddingLeft: '1.5em', marginBottom: '0.8em'}} {...props} />,
                        ol: ({node, ...props}) => <ol style={{...props.style, paddingLeft: '1.5em', marginBottom: '0.8em'}} {...props} />,
                        li: ({node, ...props}) => <li style={{...props.style, marginBottom: '0.3em'}} {...props} />,
                        code: ({node, inline, ...props}: any) => inline
                            ? <code style={{backgroundColor: '#2a2a2a', padding: '2px 6px', borderRadius: '3px', fontSize: '0.9em'}} {...props} />
                            : <code style={{display: 'block', backgroundColor: '#1a1a1a', padding: '12px', borderRadius: '6px', overflowX: 'auto', marginBottom: '1em'}} {...props} />,
                        strong: ({node, ...props}) => <strong style={{fontWeight: 'bold'}} {...props} />,
                        blockquote: ({node, ...props}) => <blockquote style={{borderLeft: '3px solid #555', paddingLeft: '1em', fontStyle: 'italic', color: '#999', marginBottom: '0.8em'}} {...props} />,
                    }}
                >
                    {content}
                </ReactMarkdown>
            </div>
        );
    };

    const renderViewContent = () => {
        if (!troubleshootResult) return null;

        switch (viewMode) {
            case 'opscopilot':
                return renderMessageContent(troubleshootResult.opsCopilotAnswer);
            case 'external':
                if (troubleshootResult.externalError) {
                    return <div style={{...styles.messageContent, color: '#ff6b6b'}}>{troubleshootResult.externalError}</div>;
                }
                return renderMessageContent(troubleshootResult.externalAnswer || '外部定位结果加载中...');
            case 'integrated':
                return renderMessageContent(troubleshootResult.integratedAnswer || '综合答复生成中...');
            default:
                return null;
        }
    };

    return (
        <div style={styles.container}>
            {!isInvestigating ? (
                <div style={styles.emptyState}>
                    <div style={styles.icon}>🩺</div>
                    <p style={styles.emptyText}>请输入您遇到的问题，并点击“开始排查”</p>
                    <div style={{width: '100%', padding: '0 20px', boxSizing: 'border-box', display: 'flex', flexDirection: 'column', gap: '10px'}}>
                        <textarea
                            value={input}
                            onChange={(e) => setInput(e.target.value)}
                            placeholder="例如：服务器 CPU 占用率过高..."
                            style={{...styles.textarea, minHeight: '80px', backgroundColor: '#333'}}
                        />
                        <button onClick={handleStart} style={styles.primaryButton}>
                            开始排查
                        </button>
                    </div>
                </div>
            ) : (
                <div style={styles.chatContainer}>
                    <div style={styles.messageList}>
                        {troubleshootResult && (
                            <div style={styles.viewSwitcher}>
                                <button
                                    onClick={() => setViewMode('opscopilot')}
                                    disabled={!troubleshootResult.opsCopilotReady}
                                    style={{
                                        ...(viewMode === 'opscopilot' ? styles.activeViewBtn : styles.viewBtn),
                                        opacity: troubleshootResult.opsCopilotReady ? 1 : 0.4,
                                        backgroundColor: viewMode === 'opscopilot' ? '#007acc' : (troubleshootResult.opsCopilotReady ? '#4CAF50' : '#666')
                                    }}
                                >
                                    OpsCopilot {troubleshootResult.opsCopilotReady ? '✓' : troubleshootResult.opsCopilotReady === false ? '✗' : '⏳'}
                                </button>
                                <button
                                    onClick={() => setViewMode('external')}
                                    disabled={!troubleshootResult.externalReady}
                                    style={{
                                        ...(viewMode === 'external' ? styles.activeViewBtn : styles.viewBtn),
                                        opacity: troubleshootResult.externalReady ? 1 : 0.4,
                                        backgroundColor: viewMode === 'external' ? '#007acc' : (troubleshootResult.externalReady ? '#4CAF50' : '#666')
                                    }}
                                >
                                    外部定位 {troubleshootResult.externalReady ? '✓' : troubleshootResult.externalReady === false ? '✗' : '⏳'}
                                </button>
                                <button
                                    onClick={() => setViewMode('integrated')}
                                    disabled={!troubleshootResult.integratedReady}
                                    style={{
                                        ...(viewMode === 'integrated' ? styles.activeViewBtn : styles.viewBtn),
                                        opacity: troubleshootResult.integratedReady ? 1 : 0.4,
                                        backgroundColor: viewMode === 'integrated' ? '#007acc' : (troubleshootResult.integratedReady ? '#4CAF50' : '#666')
                                    }}
                                >
                                    综合答复 {troubleshootResult.integratedReady ? '✓' : '⏳'}
                                </button>
                            </div>
                        )}
                        {troubleshootResult ? (
                            <div style={{
                                ...styles.messageItem,
                                alignSelf: 'flex-start',
                                backgroundColor: '#333',
                                maxWidth: '95%',
                                width: '95%'
                            }}>
                                {renderViewContent()}
                            </div>
                        ) : (
                            messages.map((msg, idx) => (
                                <div key={idx} style={{
                                    ...styles.messageItem,
                                    alignSelf: msg.role === 'user' ? 'flex-end' : 'flex-start',
                                    backgroundColor: msg.role === 'user' ? '#007acc' : '#333',
                                    maxWidth: msg.role === 'user' ? '85%' : '95%'
                                }}>
                                    {msg.role === 'ai' ? renderMessageContent(msg.content) : (
                                        <div style={styles.messageContent}>{msg.content}</div>
                                    )}
                                </div>
                            ))
                        )}
                        <div ref={messagesEndRef} />
                        {agentStatus && (
                            <div style={styles.statusIndicator}>
                                <span style={styles.spinner}>⚙️</span> {agentStatus.message}
                            </div>
                        )}
                        {agentStatus && agentStatusHistory.length > 0 && (
                            <div style={styles.statusHistory}>
                                {agentStatusHistory.slice(-5).map((s, idx) => (
                                    <div key={idx} style={styles.statusHistoryLine}>
                                        {s.stage}: {s.message}
                                    </div>
                                ))}
                            </div>
                        )}
                        {!troubleshootResult && !agentStatus && lastUsedDocs.length > 0 && (
                            <div style={styles.usedDocsBox}>
                                <div style={styles.usedDocsTitle}>本次参考文档</div>
                                <div style={styles.usedDocsList}>
                                    {lastUsedDocs.map((d) => (
                                        <span key={d} style={styles.usedDocChip}>{d}</span>
                                    ))}
                                </div>
                            </div>
                        )}
                    </div>
                </div>
            )}

            {isInvestigating && (
                <div style={styles.footer}>
                    {isStopping ? (
                        <div style={styles.stopContainer}>
                            <div style={styles.inputWrapper}>
                                <input 
                                    type="text" 
                                    value={rootCause}
                                    onChange={(e) => setRootCause(e.target.value)}
                                    placeholder="请输入根本原因 (Root Cause)..."
                                    style={styles.rootCauseInput}
                                    autoFocus
                                />
                                <button 
                                    onClick={handlePolishRootCause} 
                                    style={styles.magicButton}
                                    title="AI 润色"
                                    disabled={isPolishing}
                                >
                                    {isPolishing ? '...' : '✨'}
                                </button>
                            </div>
                            <div style={styles.stopActions}>
                                <button onClick={() => setIsStopping(false)} style={styles.secondaryButton}>取消</button>
                                <button onClick={handleConfirmStop} style={styles.primaryButton}>确认结束</button>
                            </div>
                        </div>
                    ) : (
                        <>
                            <div style={styles.toolbar}>
                                <button onClick={handleStopClick} style={styles.secondaryButton}>结束排查</button>
                            </div>
                            <div style={styles.inputBox}>
                                <textarea
                                    value={input}
                                    onChange={(e) => setInput(e.target.value)}
                                    onKeyDown={handleKeyDown}
                                    placeholder="输入问题或现象..."
                                    style={styles.textarea}
                                    className="hide-scrollbar"
                                    rows={1}
                                />
                                <button onClick={handleSend} style={styles.sendButton}>发送</button>
                            </div>
                        </>
                    )}
                </div>
            )}

            <SessionReviewModal 
                isOpen={isReviewModalOpen}
                onClose={() => setIsReviewModalOpen(false)}
                rootCause={rootCause}
                onArchive={handleArchive}
            />
        </div>
    );
};

const styles = {
    container: {
        display: 'flex',
        flexDirection: 'column' as const,
        height: '100%',
        color: '#ccc',
    },
    emptyState: {
        flex: 1,
        display: 'flex',
        flexDirection: 'column' as const,
        alignItems: 'center',
        justifyContent: 'center',
        padding: '20px',
        color: '#888',
    },
    icon: {
        fontSize: '48px',
        marginBottom: '16px',
    },
    emptyText: {
        marginBottom: '20px',
    },
    primaryButton: {
        padding: '8px 16px',
        backgroundColor: '#007acc',
        color: 'white',
        border: 'none',
        borderRadius: '4px',
        cursor: 'pointer',
    },
    secondaryButton: {
        padding: '4px 8px',
        backgroundColor: '#333',
        color: '#ccc',
        border: '1px solid #555',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '12px',
    },
    chatContainer: {
        flex: 1,
        overflowY: 'auto' as const,
        padding: '10px',
        minHeight: 0, // Critical for nested flex scrolling
    },
    messageList: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '10px',
    },
    messageItem: {
        maxWidth: '85%',
        padding: '8px 12px',
        borderRadius: '8px',
        color: '#fff',
        wordBreak: 'break-word' as const,
    },
    messageContent: {
        fontSize: '13px',
        lineHeight: '1.4',
    },
    footer: {
        padding: '10px',
        backgroundColor: '#252526',
        borderTop: '1px solid #333',
    },
    toolbar: {
        display: 'flex',
        justifyContent: 'flex-end',
        marginBottom: '8px',
    },
    inputBox: {
        display: 'flex',
        gap: '8px',
    },
    textarea: {
        flex: 1,
        backgroundColor: '#3c3c3c',
        border: 'none',
        borderRadius: '4px',
        color: '#fff',
        padding: '8px',
        resize: 'none' as const,
        outline: 'none',
        fontFamily: 'inherit',
    },
    sendButton: {
        padding: '0 12px',
        backgroundColor: '#007acc',
        color: 'white',
        border: 'none',
        borderRadius: '4px',
        cursor: 'pointer',
    },
    statusIndicator: {
        padding: '8px 12px',
        color: '#888',
        fontSize: '12px',
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
        fontStyle: 'italic',
        animation: 'fadeIn 0.3s ease',
    },
    spinner: {
        display: 'inline-block',
        animation: 'spin 2s linear infinite',
    },
    statusHistory: {
        padding: '6px 12px 10px 12px',
        borderLeft: '2px solid #333',
        marginLeft: '8px',
        color: '#777',
        fontSize: '12px',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '4px',
    },
    statusHistoryLine: {
        whiteSpace: 'pre-wrap' as const,
        wordBreak: 'break-word' as const,
    },
    usedDocsBox: {
        padding: '10px 12px',
        backgroundColor: '#1f1f1f',
        border: '1px solid #333',
        borderRadius: '8px',
        color: '#aaa',
        maxWidth: '95%',
    },
    usedDocsTitle: {
        fontSize: '12px',
        color: '#888',
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
        backgroundColor: '#2a2a2a',
        border: '1px solid #3a3a3a',
        color: '#bbb',
        fontSize: '12px',
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
        color: '#aaa',
        textTransform: 'uppercase' as const,
        letterSpacing: '0.5px',
    },
    stopContainer: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '8px',
    },
    inputWrapper: {
        display: 'flex',
        gap: '8px',
        alignItems: 'center',
    },
    rootCauseInput: {
        flex: 1,
        padding: '8px',
        backgroundColor: '#3c3c3c',
        border: '1px solid #555',
        borderRadius: '4px',
        color: '#fff',
        fontSize: '13px',
        outline: 'none',
        boxSizing: 'border-box' as const,
    },
    magicButton: {
        background: 'none',
        border: '1px solid #555',
        borderRadius: '4px',
        color: '#ffd700',
        cursor: 'pointer',
        fontSize: '16px',
        padding: '6px 10px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
    },
    stopActions: {
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '8px',
    },
    viewSwitcher: {
        display: 'flex',
        gap: '8px',
        marginBottom: '10px',
        padding: '8px',
        backgroundColor: '#2a2a2a',
        borderRadius: '8px',
        border: '1px solid #3a3a3a',
    },
    viewBtn: {
        flex: 1,
        padding: '6px 12px',
        backgroundColor: '#3c3c3c',
        color: '#ccc',
        border: '1px solid #555',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '12px',
        transition: 'all 0.2s ease',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        gap: '4px',
    },
    activeViewBtn: {
        flex: 1,
        padding: '6px 12px',
        backgroundColor: '#007acc',
        color: 'white',
        border: '1px solid #007acc',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '12px',
        transition: 'all 0.2s ease',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        gap: '4px',
    }
};

const existing = document.getElementById('opscopilot-animations');
if (!existing) {
    const styleSheet = document.createElement("style");
    styleSheet.id = 'opscopilot-animations';
    styleSheet.textContent = `
        @keyframes spin { 100% { transform: rotate(360deg); } }
    `;
    document.head.appendChild(styleSheet);
}

export default TroubleshootingPanel;
