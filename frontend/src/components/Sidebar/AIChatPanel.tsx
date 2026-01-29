import React, { useState, useEffect, useRef } from 'react';
import MessageRenderer from './MessageRenderer';
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

const AIChatPanel: React.FC = () => {
    const [input, setInput] = useState('');
    const [messages, setMessages] = useState<Message[]>([]);
    const [agentStatus, setAgentStatus] = useState<{ stage: string; message: string } | null>(null);
    const [agentStatusHistory, setAgentStatusHistory] = useState<AgentStatusEvent[]>([]);
    const [lastUsedDocs, setLastUsedDocs] = useState<string[]>([]);
    const usedDocsRef = useRef<Set<string>>(new Set());
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

    const handleSend = async () => {
        if (!input.trim()) return;
        
        const userMsg: Message = {
            role: 'user',
            content: input,
            timestamp: Date.now()
        };
        
        setMessages(prev => [...prev, userMsg]);
        setInput('');

        setAgentStatus({ stage: 'thinking', message: '正在思考...' });
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

    const handleNewChat = () => {
        setMessages([]);
        setInput('');
        setAgentStatus(null);
        setAgentStatusHistory([]);
        setLastUsedDocs([]);
        usedDocsRef.current = new Set();
    };

    return (
        <div style={styles.container}>
            <div style={styles.header}>
                <button onClick={handleNewChat} style={styles.newChatBtn}>
                    + 新建对话
                </button>
            </div>

            <div style={styles.chatContainer}>
                {messages.length === 0 ? (
                    <div style={styles.emptyState}>
                        <div style={styles.icon}>💬</div>
                        <p style={styles.emptyText}>有问题？随时问我！</p>
                    </div>
                ) : (
                    <div style={styles.messageList}>
                        {messages.map((msg, idx) => (
                            <div key={idx} style={{
                                ...styles.messageItem,
                                alignSelf: msg.role === 'user' ? 'flex-end' : 'flex-start',
                                backgroundColor: msg.role === 'user' ? '#007acc' : '#333',
                                maxWidth: msg.role === 'user' ? '85%' : '95%'
                            }} data-testid="message-item">
                                <MessageRenderer content={msg.content} role={msg.role} />
                            </div>
                        ))}
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
                        {!agentStatus && lastUsedDocs.length > 0 && (
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
                )}
            </div>

            <div style={styles.footer}>
                <div style={styles.inputBox}>
                    <textarea
                        value={input}
                        onChange={(e) => setInput(e.target.value)}
                        onKeyDown={handleKeyDown}
                        placeholder="输入问题..."
                        style={styles.textarea}
                        className="hide-scrollbar"
                        rows={1}
                    />
                    <button onClick={handleSend} style={styles.sendButton}>发送</button>
                </div>
            </div>
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
    header: {
        padding: '10px 16px',
        borderBottom: '1px solid #333',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
    },
    title: {
        fontSize: '14px',
        fontWeight: 'bold' as const,
        color: '#fff',
    },
    newChatBtn: {
        padding: '4px 8px',
        backgroundColor: 'transparent',
        border: '1px solid #555',
        borderRadius: '4px',
        color: '#ccc',
        fontSize: '12px',
        cursor: 'pointer',
    },
    chatContainer: {
        flex: 1,
        overflowY: 'auto' as const,
        padding: '10px',
        minHeight: 0, // Critical for nested flex scrolling
    },
    emptyState: {
        flex: 1,
        display: 'flex',
        flexDirection: 'column' as const,
        alignItems: 'center',
        justifyContent: 'center',
        height: '100%',
        color: '#888',
    },
    icon: {
        fontSize: '48px',
        marginBottom: '16px',
    },
    emptyText: {
        marginBottom: '20px',
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
        overflow: 'hidden',
    },
    footer: {
        padding: '10px',
        backgroundColor: '#252526',
        borderTop: '1px solid #333',
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
};

// Add style tag for animations if not exists
const styleSheet = document.createElement("style");
styleSheet.textContent = `
    @keyframes spin { 100% { transform: rotate(360deg); } }
`;
document.head.appendChild(styleSheet);

export default AIChatPanel;
