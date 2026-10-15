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
import type { AssistantRuntime } from '../../../../frontend-shell/src/assistant/runtime';
import { wailsAssistantRuntime } from '../../shell-adapter/wailsAssistantRuntime';

interface Message {
    role: 'user' | 'ai';
    content: string;
    timestamp: number;
    trace?: AgentTraceEvent[];
    references?: KnowledgeReference[];
}

interface AIChatPanelProps {
    runtime?: AssistantRuntime;
    activeTerminalTitle?: string;
    onOpenSource?: (target: Omit<KnowledgeTarget, 'requestId'>) => void;
}

const extractDocFromReadingMessage = (message: string): string | null => {
    const marker = '正在阅读文档:';
    const index = message.indexOf(marker);
    if (index === -1) return null;
    return message.slice(index + marker.length).trim().replace(/\.\.\.$/, '').trim() || null;
};

const AIChatPanel: React.FC<AIChatPanelProps> = ({ activeTerminalTitle, onOpenSource, runtime = wailsAssistantRuntime }) => {
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
                if (runtime.subscribeStatus) {
                    cancelStatus = runtime.subscribeStatus(data => {
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

            const response = await runtime.ask(question);
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
        <div className="ai-panel ai-chat-panel" style={{...styles.container, flex: 1, minHeight: 0}}>
            <div className="ai-chat-subheader">
                <span className="ai-session-state"><i /> 知识增强</span>
                <button disabled={busy} onClick={handleNewChat} className="ai-new-chat-button">+ 新建对话</button>
            </div>

            <div className="ai-conversation-scroll">
                {messages.length > 0 && (
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
    container: { display: 'flex', flexDirection: 'column' as const, color: 'var(--text-secondary)' },
    references: { marginTop: '18px' },
    sectionTitle: { margin: '0 0 8px', color: 'var(--text-muted)', fontSize: '11px', fontWeight: 600, textTransform: 'uppercase' as const, letterSpacing: '0.08em' },
};

export default AIChatPanel;
