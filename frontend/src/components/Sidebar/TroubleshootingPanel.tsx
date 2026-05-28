import React, { useState, useEffect, useRef } from 'react';
import { TbBrain, TbTarget, TbSearch, TbBook, TbWriting, TbRefresh, TbAlertTriangle, TbSettings, TbSparkles } from 'react-icons/tb';
import TroubleshootingStep from './TroubleshootingStep';
import CommandCard from './CommandCard';
import SessionReviewModal, { ArchiveParams } from './SessionReviewModal';
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

interface TroubleshootingPanelProps {
    onStart?: () => void;
    onStop?: () => void;
}

// Stage display configuration: maps backend stage names to user-friendly labels, icons, and colors
const STAGE_CONFIG: Record<string, { label: string; icon: React.ReactNode; color: string }> = {
    thinking:       { label: '分析中',   icon: TbBrain({size: 14}), color: '#8b9cf7' },
    catalog_match:  { label: '匹配知识库', icon: TbTarget({size: 14}), color: '#7ac5d8' },
    grepping:       { label: '搜索关键词', icon: TbSearch({size: 14}), color: '#d4a843' },
    reading:        { label: '查阅文档', icon: TbBook({size: 14}), color: '#7ac5d8' },
    answering:      { label: '生成回答', icon: TbWriting({size: 14}), color: '#6ecf8a' },
    retrying:       { label: '重试中',   icon: TbRefresh({size: 14}), color: '#e0a050' },
    error:          { label: '出错',     icon: TbAlertTriangle({size: 14}), color: '#e06060' },
};

function getStageConfig(stage: string) {
    return STAGE_CONFIG[stage] || { label: stage, icon: TbSettings({size: 14}), color: '#888' };
}

const TroubleshootingPanel: React.FC<TroubleshootingPanelProps> = ({ onStart, onStop }) => {
    const [isInvestigating, setIsInvestigating] = useState(false);
    const [input, setInput] = useState('');
    const [messages, setMessages] = useState<Message[]>([]);
    const [agentStatus, setAgentStatus] = useState<{ stage: string; message: string } | null>(null);
    const [agentStatusHistory, setAgentStatusHistory] = useState<AgentStatusEvent[]>([]);
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
        if (onStart) onStart();

        const problem = input;
        setOriginalProblem(problem);

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
            setAgentStatus({ stage: 'thinking', message: '正在分析问题，扫描知识库目录...' });
            setAgentStatusHistory([]);
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
                    timestamp: Date.now()
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
                        timestamp: Date.now()
                    }]);
                }
            }
            if (cancelStatus) cancelStatus();
            if (cancelContext) cancelContext();
        } catch (e: any) {
            console.error("Initial AI analysis failed", e);
        } finally {
            setAgentStatus(null);
            setAgentStatusHistory([]);
            setLastUsedDocs(Array.from(usedDocsRef.current));
            setContextUsage(null);
        }
        
        setInput('');
    };

    const handleCancelClick = async () => {
        if (confirm('确定要取消定位吗？这将清空当前的所有记录。')) {
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
        setCatalogMatches([]);
        catalogMatchRef.current = [];
        setLastUsedDocs([]);
        usedDocsRef.current = new Set();
        setIsStopping(false);
        setRootCause('');
        setOriginalProblem('');
        if (onStart) onStart();
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
                    timestamp: Date.now()
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
            if (cancelContext) cancelContext();
            setAgentStatus(null);
            setAgentStatusHistory([]);
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

        // 1. 尝试从 JSON 包装中提取文本
        try {
            const trimmed = text.trim();
            if (trimmed.startsWith('{')) {
                const data = JSON.parse(trimmed);
                for (const key of ['summary', 'content', 'answer', 'text']) {
                    if (data[key] && typeof data[key] === 'string') {
                        text = data[key];
                        break;
                    }
                }
            }
        } catch {}

        // 2. 确保标题前有空行（修复 `文字\n## 标题` → `文字\n\n## 标题`）
        text = text.replace(/([^\n])\n(#{1,6} )/g, '$1\n\n$2');

        return text;
    };

    const renderMessageContent = (content: string) => {
        content = preprocessContent(content);
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

    return (
        <div style={styles.container}>
            {!isInvestigating ? (
                <div style={styles.emptyState}>
                    <div style={styles.icon}>🩺</div>
                    <p style={styles.emptyText}>请输入您遇到的问题，并点击"开始排查"</p>
                    <div style={{width: '100%', padding: '0 20px', boxSizing: 'border-box', display: 'flex', flexDirection: 'column', gap: '10px'}}>

                        {/* 问题输入区域 */}
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
                        {messages.map((msg, idx) => (
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
                        }
                        <div ref={messagesEndRef} />
                        {agentStatus && (() => {
                            const cfg = getStageConfig(agentStatus.stage);
                            return (
                                <div style={styles.statusIndicator}>
                                    <span style={{...styles.stageIcon, color: cfg.color}}>{cfg.icon}</span>
                                    <span style={{...styles.stageLabel, color: cfg.color}}>{cfg.label}</span>
                                    {agentStatus.stage === 'catalog_match' ? (
                                        <span style={styles.stageBreadcrumb}>{agentStatus.message}</span>
                                    ) : agentStatus.stage === 'thinking' ? (
                                        <span style={styles.stageMessage}>{agentStatus.message}</span>
                                    ) : (
                                        <span style={styles.stageMessage}>{agentStatus.message.replace(cfg.label, '')}</span>
                                    )}
                                </div>
                            );
                        })()}
                        {contextUsage && (() => {
                            const ratio = contextUsage.used / contextUsage.max;
                            const pct = Math.min(ratio * 100, 100);
                            return (
                                <div style={styles.contextBar}>
                                    <div style={styles.contextBarTrack}>
                                        <div style={{
                                            ...styles.contextBarFill,
                                            width: `${pct}%`,
                                            backgroundColor: ratio > 0.8 ? '#e06060' : '#4a9eda',
                                        }} />
                                    </div>
                                    <span style={styles.contextBarLabel}>
                                        {Math.round(contextUsage.used / 1000)}K / {Math.round(contextUsage.max / 1000)}K
                                    </span>
                                </div>
                            );
                        })()}
                        {agentStatus && agentStatusHistory.length > 1 && (
                            <div style={styles.statusHistory}>
                                {agentStatusHistory.slice(0, -1).map((s, idx) => {
                                    const cfg = getStageConfig(s.stage);
                                    // Extract meaningful detail from the message
                                    let detail = '';
                                    if (s.stage === 'catalog_match') {
                                        const parts = s.message.split(' › ');
                                        detail = parts[parts.length - 1] || '';
                                    } else if (s.stage === 'grepping') {
                                        // Extract keyword from "正在搜索关键词: xxx..."
                                        const m = s.message.match(/关键词:\s*(.+?)(\.\.\.)?$/);
                                        detail = m ? m[1] : '';
                                    } else if (s.stage === 'reading') {
                                        // Extract doc name from "正在阅读文档: xxx..."
                                        const m = s.message.match(/文档:\s*(.+?)(\.\.\.)?$/);
                                        detail = m ? m[1] : '';
                                    }
                                    return (
                                        <div key={idx} style={styles.statusHistoryLine}>
                                            <span style={{...styles.historyIcon, color: cfg.color}}>{cfg.icon}</span>
                                            <span style={{...styles.historyLabel, color: cfg.color}}>{cfg.label}</span>
                                            {detail && <span style={styles.historyDetail}>{detail}</span>}
                                        </div>
                                    );
                                })}
                            </div>
                        )}
                        {!agentStatus && catalogMatches.length > 0 && (
                            <div style={styles.usedDocsBox}>
                                <div style={styles.usedDocsTitle}>知识库匹配路径</div>
                                <div style={styles.usedDocsList}>
                                    {catalogMatches.map((path, idx) => {
                                        const parts = path.split(' › ');
                                        return (
                                            <span key={idx} style={styles.catalogPathChip}>
                                                {parts.map((part, pi) => (
                                                    <span key={pi}>
                                                        {pi > 0 && <span style={styles.catalogPathSep}> / </span>}
                                                        <span style={{
                                                            color: pi === parts.length - 1 ? '#ccc' : '#777',
                                                            fontWeight: pi === parts.length - 1 ? 500 : 400,
                                                        }}>{part}</span>
                                                    </span>
                                                ))}
                                            </span>
                                        );
                                    })}
                                </div>
                            </div>
                        )}
                        {!agentStatus && lastUsedDocs.length > 0 && catalogMatches.length === 0 && (
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
                                    {isPolishing ? '...' : TbSparkles({size: 14})}
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
                                <button onClick={handleCancelClick} style={styles.cancelButton}>
                                    <span style={styles.cancelButtonIcon}>✕</span>
                                    <span style={styles.cancelButtonText}>取消定位</span>
                                </button>
                                <button onClick={handleStopClick} style={styles.stopButton}>
                                    <span style={styles.stopButtonIcon}>⏹</span>
                                    <span style={styles.stopButtonText}>结束排查</span>
                                </button>
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
                onClose={() => {
                    setIsReviewModalOpen(false);
                }}
                rootCause={rootCause}
                problem={originalProblem}
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
    stopButton: {
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
        padding: '8px 14px',
        backgroundColor: '#007acc',
        color: '#fff',
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
        backgroundColor: '#f44336',
        color: '#fff',
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
    messageItem: {
        maxWidth: '85%',
        padding: '10px 14px',
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
        gap: '8px',
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
        backgroundColor: '#1e1e1e',
        borderRadius: '6px',
        margin: '0 4px',
        border: '1px solid #2a2a2a',
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
        color: '#999',
        whiteSpace: 'nowrap' as const,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
    },
    stageBreadcrumb: {
        fontSize: '12px',
        color: '#999',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
    },
    statusHistory: {
        padding: '4px 12px',
        borderLeft: '2px solid #2a2a2a',
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
        color: '#666',
        marginLeft: '2px',
    },
    usedDocsBox: {
        padding: '10px 12px',
        backgroundColor: '#1f1f1f',
        border: '1px solid #333',
        borderRadius: '8px',
        color: '#aaa',
        maxWidth: '95%',
    },
    catalogPathChip: {
        padding: '2px 8px',
        borderRadius: '999px',
        backgroundColor: '#2a2a2a',
        border: '1px solid #3a3a3a',
        color: '#bbb',
        fontSize: '11px',
    },
    catalogPathSep: {
        color: '#555',
        margin: '0 1px',
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
        backgroundColor: '#333',
        overflow: 'hidden' as const,
    },
    contextBarFill: {
        height: '100%',
        borderRadius: '2px',
        transition: 'width 0.3s ease',
    },
    contextBarLabel: {
        fontSize: '10px',
        color: '#666',
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
