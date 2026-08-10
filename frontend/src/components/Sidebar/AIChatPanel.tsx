import React, { useEffect, useRef, useState } from 'react';
import {
    AIComposer,
    AgentTraceEvent,
    CitationList,
    KnowledgeReference,
    KnowledgeTarget,
    RetrievalTrace,
    RichText,
    referencesFromDocuments,
} from '../AI';
// @ts-ignore
import { EventsOn } from '../../../wailsjs/runtime/runtime';

interface Message {
    role: 'user' | 'ai';
    content: string;
    timestamp: number;
    trace?: AgentTraceEvent[];
    references?: KnowledgeReference[];
}

interface AIChatPanelProps {
    activeTerminalTitle?: string;
    onOpenSource?: (target: Omit<KnowledgeTarget, 'requestId'>) => void;
}

const extractDocFromReadingMessage = (message: string): string | null => {
    const marker = '正在阅读文档:';
    const index = message.indexOf(marker);
    if (index === -1) return null;
    return message.slice(index + marker.length).trim().replace(/\.\.\.$/, '').trim() || null;
};

const AIChatPanel: React.FC<AIChatPanelProps> = ({ activeTerminalTitle, onOpenSource }) => {
    const [input, setInput] = useState('');
    const [messages, setMessages] = useState<Message[]>([]);
    const [agentTrace, setAgentTrace] = useState<AgentTraceEvent[]>([]);
    const [busy, setBusy] = useState(false);
    const traceRef = useRef<AgentTraceEvent[]>([]);
    const usedDocsRef = useRef<Set<string>>(new Set());
    const messagesEndRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    }, [messages, agentTrace]);

    const handleSend = async () => {
        const question = input.trim();
        if (!question || busy) return;

        setMessages(previous => [...previous, { role: 'user', content: question, timestamp: Date.now() }]);
        setInput('');
        setBusy(true);
        const initialTrace: AgentTraceEvent[] = [{ stage: 'thinking', message: '正在思考...', ts: Date.now() }];
        traceRef.current = initialTrace;
        usedDocsRef.current = new Set();
        setAgentTrace(initialTrace);

        let cancelStatus: (() => void) | undefined;
        try {
            try {
                if (EventsOn) {
                    cancelStatus = EventsOn('agent:status', (...args: any[]) => {
                        const data = args?.[0] ?? {};
                        const stage = String(data?.stage ?? '');
                        const message = String(data?.message ?? '');
                        if (!stage || !message) return;

                        const event: AgentTraceEvent = {
                            runId: data?.runId ? String(data.runId) : undefined,
                            stage,
                            message,
                            ts: Date.now(),
                        };
                        const previous = traceRef.current;
                        const last = previous[previous.length - 1];
                        if (!last || last.stage !== stage || last.message !== message) {
                            traceRef.current = [...previous, event].slice(-8);
                            setAgentTrace(traceRef.current);
                        }
                        if (stage === 'reading') {
                            const document = extractDocFromReadingMessage(message);
                            if (document) usedDocsRef.current.add(document);
                        }
                    });
                }
            } catch (listenerError) {
                console.error('Failed to register event listener:', listenerError);
            }

            let response: string;
            // @ts-ignore
            if (window.go?.main?.App?.AskAI) {
                // @ts-ignore
                response = String(await window.go.main.App.AskAI(question) ?? '');
            } else {
                response = 'Error: Backend not connected.';
            }
            setMessages(previous => [...previous, {
                role: 'ai',
                content: response,
                timestamp: Date.now(),
                trace: [...traceRef.current],
                references: referencesFromDocuments(Array.from(usedDocsRef.current)),
            }]);
        } catch (error: any) {
            setMessages(previous => [...previous, {
                role: 'ai',
                content: `Error: ${error?.message || String(error)}`,
                timestamp: Date.now(),
                trace: [...traceRef.current],
                references: referencesFromDocuments(Array.from(usedDocsRef.current)),
            }]);
        } finally {
            cancelStatus?.();
            setAgentTrace([]);
            setBusy(false);
        }
    };

    const handleNewChat = () => {
        setMessages([]);
        setInput('');
        setAgentTrace([]);
        traceRef.current = [];
        usedDocsRef.current = new Set();
    };

    return (
        <div className="ai-panel ai-chat-panel" style={styles.container}>
            <div className="ai-chat-subheader">
                <span className="ai-session-state"><i /> 知识增强</span>
                <button onClick={handleNewChat} className="ai-new-chat-button">+ 新建对话</button>
            </div>

            <div className="ai-conversation-scroll">
                {messages.length === 0 ? (
                    <div className="ai-empty-stage ai-chat-empty">
                        <div className="ai-empty-glyph" aria-hidden="true"><span>AI</span><i /><i /><i /></div>
                        <span className="ai-empty-eyebrow">KNOWLEDGE ASSISTANT</span>
                        <h4>询问运维知识</h4>
                        <p>了解机制、对比方案，或从现象中寻找可能的原因。</p>
                    </div>
                ) : (
                    <div className="ai-message-list">
                        {messages.map((message, index) => message.role === 'user' ? (
                            <div key={`${message.timestamp}-${index}`} className="ai-user-message" data-testid="message-item">
                                <RichText content={message.content} />
                            </div>
                        ) : (
                            <article key={`${message.timestamp}-${index}`} className="ai-assistant-message" data-testid="message-item">
                                <RetrievalTrace events={message.trace || []} active={false} />
                                <RichText content={message.content} />
                                {!!message.references?.length && (
                                    <section style={styles.references}>
                                        <h4 style={styles.sectionTitle}>参考文档</h4>
                                        <CitationList references={message.references} onOpenSource={onOpenSource} />
                                    </section>
                                )}
                            </article>
                        ))}
                        {busy && <RetrievalTrace events={agentTrace} active />}
                        <div ref={messagesEndRef} />
                    </div>
                )}
            </div>

            <div className="ai-panel-footer">
                <AIComposer
                    value={input}
                    onChange={setInput}
                    onSend={handleSend}
                    disabled={busy}
                    placeholder="输入问题…"
                    contexts={activeTerminalTitle ? [{ id: 'terminal', label: `当前终端 · ${activeTerminalTitle}`, active: true }] : []}
                />
            </div>
        </div>
    );
};

const styles = {
    container: { display: 'flex', flexDirection: 'column' as const, height: '100%', color: 'var(--text-secondary)' },
    header: { padding: '8px 14px', borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' },
    headerHint: { color: 'var(--text-muted)', fontSize: '11px' },
    newChatBtn: { padding: '4px 8px', backgroundColor: 'transparent', border: '1px solid var(--border-strong)', borderRadius: '4px', color: 'var(--text-secondary)', fontSize: '12px', cursor: 'pointer' },
    chatContainer: { flex: 1, overflowY: 'auto' as const, padding: '16px 18px', minHeight: 0 },
    emptyState: { display: 'flex', flexDirection: 'column' as const, alignItems: 'center', justifyContent: 'center', height: '100%', color: 'var(--text-muted)', textAlign: 'center' as const },
    emptyMark: { width: '42px', height: '42px', display: 'grid', placeItems: 'center', border: '1px solid var(--border-strong)', borderRadius: '10px', color: 'var(--accent)', fontWeight: 700, letterSpacing: '0.05em' },
    emptyTitle: { margin: '14px 0 4px', color: 'var(--text-primary)', fontSize: '14px' },
    emptyText: { margin: 0, maxWidth: '260px', fontSize: '12px', lineHeight: 1.6 },
    messageList: { display: 'flex', flexDirection: 'column' as const, gap: '16px' },
    userMessage: { alignSelf: 'flex-end', maxWidth: '85%', padding: '8px 12px', borderRadius: '8px 8px 2px 8px', background: 'var(--accent)', color: 'var(--text-on-accent)', overflow: 'hidden' },
    aiMessage: { width: '100%', color: 'var(--text-primary)', overflow: 'hidden' },
    references: { marginTop: '18px' },
    sectionTitle: { margin: '0 0 8px', color: 'var(--text-muted)', fontSize: '11px', fontWeight: 600, textTransform: 'uppercase' as const, letterSpacing: '0.08em' },
    footer: { padding: '10px 12px 12px', backgroundColor: 'var(--bg-secondary)', borderTop: '1px solid var(--border)' },
};

export default AIChatPanel;
