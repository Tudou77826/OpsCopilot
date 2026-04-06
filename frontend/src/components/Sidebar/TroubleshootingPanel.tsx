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

interface MCPStatus {
    servers: Record<string, boolean>;
}

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
    const [mcpToolEnhanced, setMcpToolEnhanced] = useState(false);
    const [showExternalHelp, setShowExternalHelp] = useState(false);
    const [mcpStatus, setMcpStatus] = useState<MCPStatus | null>(null);

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

    // Load MCP status on mount
    useEffect(() => {
        const loadMcpStatus = async () => {
            try {
                console.log('[MCP] Loading status...');
                // @ts-ignore
                if (window.go && window.go.main && window.go.main.App && window.go.main.App.GetMCPStatus) {
                    // @ts-ignore
                    const statusStr = await window.go.main.App.GetMCPStatus();
                    console.log('[MCP] Raw status:', statusStr);
                    const status = JSON.parse(statusStr) as MCPStatus;
                    console.log('[MCP] Parsed status:', status);
                    setMcpStatus(status);
                } else {
                    console.log('[MCP] GetMCPStatus not available');
                }
            } catch (e) {
                console.error('[MCP] Failed to load status:', e);
            }
        };
        loadMcpStatus();
    }, []); // Empty deps - load once on mount

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

        // Reset troubleshootResult to clear previous structured response
        setTroubleshootResult(null);

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
                const response = await window.go.main.App.AskTroubleshoot(problem, mcpToolEnhanced);
                let parsedResponse = response;
                let result: TroubleshootResult | null = null;

                try {
                    // Only try to parse as JSON if it starts with '{' (JSON object)
                    if (response.trim().startsWith('{')) {
                        result = JSON.parse(response);
                        // Validate that it's a proper TroubleshootResult
                        if (result && result.opsCopilotReady) {
                            parsedResponse = result.opsCopilotAnswer;
                        } else {
                            // Not a valid structured response, treat as plain text
                            result = null;
                        }
                    }
                } catch (e) {
                    console.error('Failed to parse troubleshoot result:', e);
                    result = null;
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
        setLastUsedDocs([]);
        usedDocsRef.current = new Set();
        setViewMode('opscopilot');
        setTroubleshootResult(null);
        setIsStopping(false);
        setRootCause('');
        if (onStart) onStart();
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
                const response = await window.go.main.App.AskTroubleshoot(userMsg.content, mcpToolEnhanced);
                let parsedResponse = response;
                let result: TroubleshootResult | null = null;

                try {
                    // Only try to parse as JSON if it starts with '{' (JSON object)
                    if (response.trim().startsWith('{')) {
                        result = JSON.parse(response);
                        // Validate that it's a proper TroubleshootResult
                        if (result && result.opsCopilotReady) {
                            parsedResponse = result.opsCopilotAnswer;
                        } else {
                            // Not a valid structured response, treat as plain text
                            result = null;
                        }
                    }
                } catch (e) {
                    console.error('Failed to parse troubleshoot result:', e);
                    result = null;
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
                return renderMessageContent(troubleshootResult.externalAnswer || '加载中...');
            case 'integrated':
                return renderMessageContent(troubleshootResult.integratedAnswer || '生成中...');
            default:
                return null;
        }
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

                        {/* 高级选项 - 默认折叠，包含 MCP 工具增强 */}
                        <details style={styles.advancedOptions}>
                            <summary style={styles.advancedSummary}>高级选项</summary>
                            <div style={styles.advancedContent}>
                                {/* MCP 工具增强模式开关 */}
                                <div style={styles.enhancedModeToggle}>
                                    <div style={styles.toggleContainer}>
                                        <label style={styles.switchLabel} title={mcpToolEnhanced ? '已启用 MCP 工具增强' : '已禁用 MCP 工具增强'}>
                                            <input
                                                type="checkbox"
                                                checked={mcpToolEnhanced}
                                                onChange={(e) => setMcpToolEnhanced(e.target.checked)}
                                                style={styles.switchCheckbox}
                                                className="troubleshoot-switch-checkbox"
                                            />
                                            <span style={styles.switchSlider} className="troubleshoot-switch-slider"></span>
                                        </label>
                                        <span style={styles.toggleText}>工具增强</span>
                                    </div>
                                    <div
                                        style={styles.helpIcon}
                                        onClick={() => setShowExternalHelp(!showExternalHelp)}
                                    >
                                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                                            <circle cx="12" cy="12" r="10"/>
                                            <path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/>
                                            <line x1="12" y1="17" x2="12.01" y2="17"/>
                                        </svg>
                                    </div>
                                </div>

                                {/* MCP 工具帮助说明 */}
                                {showExternalHelp && (
                                    <div style={styles.externalHelpBox}>
                                        <div style={styles.helpContent}>
                                            启用后可调用配置的诊断工具进行深度分析。需要在设置中配置服务器。
                                        </div>
                                    </div>
                                )}
                            </div>
                        </details>

                        <button onClick={handleStart} style={styles.primaryButton}>
                            开始排查
                        </button>
                    </div>
                </div>
            ) : (
                <div style={styles.chatContainer}>
                    {troubleshootResult && (
                        <div style={styles.viewSwitcher}>
                            <button
                                role="tab"
                                aria-selected={viewMode === 'opscopilot'}
                                onClick={() => setViewMode('opscopilot')}
                                disabled={!troubleshootResult.opsCopilotReady}
                                style={{
                                    ...(viewMode === 'opscopilot' ? styles.activeViewBtn : styles.viewBtn),
                                    opacity: troubleshootResult.opsCopilotReady ? 1 : 0.5,
                                    cursor: troubleshootResult.opsCopilotReady ? 'pointer' : 'default'
                                }}
                            >
                                <span style={styles.tabIcon}>
                                    {troubleshootResult.opsCopilotReady === true ? (
                                        <span style={styles.statusIcon} aria-label="完成">✓</span>
                                    ) : troubleshootResult.opsCopilotReady === false ? (
                                        <span style={styles.statusIconError} aria-label="失败">✗</span>
                                    ) : (
                                        <span style={styles.loadingSpinner} aria-label="加载中">⏳</span>
                                    )}
                                </span>
                                <span style={styles.tabLabel}>OpsCopilot</span>
                            </button>
                            <button
                                role="tab"
                                aria-selected={viewMode === 'external'}
                                onClick={() => setViewMode('external')}
                                disabled={!troubleshootResult.externalReady}
                                style={{
                                    ...(viewMode === 'external' ? styles.activeViewBtn : styles.viewBtn),
                                    opacity: troubleshootResult.externalReady ? 1 : 0.5,
                                    cursor: troubleshootResult.externalReady ? 'pointer' : 'default'
                                }}
                            >
                                <span style={styles.tabIcon}>
                                    {troubleshootResult.externalReady === true ? (
                                        <span style={styles.statusIcon} aria-label="完成">✓</span>
                                    ) : troubleshootResult.externalReady === false ? (
                                        <span style={styles.statusIconError} aria-label="失败">✗</span>
                                    ) : (
                                        <span style={styles.loadingSpinner} aria-label="加载中">⏳</span>
                                    )}
                                </span>
                                <span style={styles.tabLabel}>外部定位</span>
                            </button>
                            <button
                                role="tab"
                                aria-selected={viewMode === 'integrated'}
                                onClick={() => setViewMode('integrated')}
                                disabled={!troubleshootResult.integratedReady}
                                style={{
                                    ...(viewMode === 'integrated' ? styles.activeViewBtn : styles.viewBtn),
                                    opacity: troubleshootResult.integratedReady ? 1 : 0.5,
                                    cursor: troubleshootResult.integratedReady ? 'pointer' : 'default'
                                }}
                            >
                                <span style={styles.tabIcon}>
                                    {troubleshootResult.integratedReady === true ? (
                                        <span style={styles.statusIcon} aria-label="完成">✓</span>
                                    ) : (
                                        <span style={styles.loadingSpinner} aria-label="加载中">⏳</span>
                                    )}
                                </span>
                                <span style={styles.tabLabel}>综合答复</span>
                            </button>
                        </div>
                    )}
                    <div style={styles.messageList}>
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
                                <button onClick={handleCancelClick} style={styles.cancelButton}>
                                    <span style={styles.cancelButtonIcon}>✕</span>
                                    <span style={styles.cancelButtonText}>取消定位</span>
                                </button>
                                <button onClick={handleStopClick} style={styles.stopButton}>
                                    <span style={styles.stopButtonIcon}>⏹</span>
                                    <span style={styles.stopButtonText}>结束排查</span>
                                </button>
                            </div>

                            {/* 增强模式状态显示（只读） - 简化为小标签 */}
                            {mcpToolEnhanced && (
                                <div style={styles.enhancedModeTag}>
                                    <span style={styles.enhancedModeTagText}>工具增强</span>
                                    {mcpStatus && mcpStatus.servers && Object.keys(mcpStatus.servers).length > 0 && (
                                        <span style={styles.enhancedModeTagDot}
                                            title={Object.entries(mcpStatus.servers)
                                                .map(([name, ready]) => `${name}: ${ready ? '已连接' : '未连接'}`)
                                                .join('\n')}
                                        >
                                            {Object.values(mcpStatus.servers).some(Boolean) ? '●' : '○'}
                                        </span>
                                    )}
                                </div>
                            )}

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
        fontStyle: 'italic',
        animation: 'fadeIn 0.3s ease',
    },
    spinner: {
        display: 'inline-block',
        animation: 'spin 2s linear infinite',
        fontStyle: 'normal',
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
        gap: '0',
        padding: '0',
        backgroundColor: '#252526',
        borderBottom: '1px solid #3e3e42',
        flexShrink: 0,
    },
    tabIcon: {
        fontSize: '14px',
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        minWidth: '20px',
    },
    tabLabel: {
        fontSize: '13px',
        fontWeight: '400',
    },
    statusIcon: {
        color: '#4CAF50',
        fontSize: '14px',
        fontWeight: '500',
    },
    statusIconError: {
        color: '#f44336',
        fontSize: '14px',
        fontWeight: '500',
    },
    loadingSpinner: {
        color: '#666',
        fontSize: '14px',
        animation: 'spin 1s linear infinite',
    },
    viewBtn: {
        flex: 1,
        padding: '8px 12px',
        backgroundColor: 'transparent',
        color: '#888',
        border: 'none',
        borderRight: '1px solid #3e3e42',
        borderRadius: '0',
        cursor: 'pointer',
        fontSize: '12px',
        fontWeight: '400',
        transition: 'all 0.15s ease',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        gap: '6px',
    },
    activeViewBtn: {
        flex: 1,
        padding: '8px 12px',
        backgroundColor: '#37373d',
        color: '#fff',
        border: 'none',
        borderRight: '1px solid #3e3e42',
        borderRadius: '0',
        cursor: 'pointer',
        fontSize: '12px',
        fontWeight: '400',
        transition: 'all 0.15s ease',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        gap: '6px',
    },
    enhancedModeSection: {
        padding: '8px 12px',
        borderBottom: '1px solid #3a3a3a',
    },
    checkboxLabel: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        cursor: 'pointer',
    },
    checkbox: {
        width: '16px',
        height: '16px',
        cursor: 'pointer',
    },
    checkboxText: {
        color: '#ccc',
        fontSize: '13px',
    },
    enhancedConfig: {
        marginTop: '10px',
        padding: '12px',
        backgroundColor: '#1a1a1a',
        borderRadius: '6px',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '8px',
    },
    configRow: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
    },
    configLabel: {
        color: '#999',
        fontSize: '12px',
        minWidth: '100px',
    },
    configInput: {
        flex: 1,
        backgroundColor: '#252526',
        color: '#ddd',
        border: '1px solid #3a3a3a',
        borderRadius: '4px',
        padding: '6px 10px',
        fontSize: '13px',
        outline: 'none',
    },
    editButton: {
        backgroundColor: '#3a3a3a',
        color: '#ccc',
        border: '1px solid #4a4a4a',
        borderRadius: '4px',
        padding: '4px 12px',
        fontSize: '12px',
        cursor: 'pointer',
    },
    customVarEditor: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '6px',
        paddingLeft: '108px',
    },
    varRow: {
        display: 'flex',
        gap: '6px',
        alignItems: 'center',
    },
    varInput: {
        backgroundColor: '#252526',
        color: '#ddd',
        border: '1px solid #3a3a3a',
        borderRadius: '4px',
        padding: '6px 10px',
        fontSize: '13px',
        outline: 'none',
    },
    deleteButton: {
        backgroundColor: '#5c3a3a',
        color: '#ff8080',
        border: '1px solid #6c4a4a',
        borderRadius: '4px',
        padding: '4px 8px',
        fontSize: '11px',
        cursor: 'pointer',
    },
    addButton: {
        backgroundColor: '#3a5a3a',
        color: '#80cc80',
        border: '1px solid #4a6a4a',
        borderRadius: '4px',
        padding: '4px 12px',
        fontSize: '12px',
        cursor: 'pointer',
    },
    enhancedModeConfig: {
        width: '100%',
        padding: '12px',
        backgroundColor: '#1a1a1a',
        borderRadius: '8px',
        border: '1px solid #3a3a3a',
    },
    enhancedModeHeader: {
        marginBottom: '10px',
    },
    enhancedModeStatus: {
        padding: '8px 12px',
        backgroundColor: '#1a2a1a',
        borderRadius: '6px',
        border: '1px solid #3a4a3a',
    },
    statusItem: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '4px',
    },
    statusLabel: {
        color: '#7aaa88',
        fontSize: '12px',
        fontWeight: '500',
    },
    statusDetail: {
        color: '#888',
        fontSize: '11px',
        paddingLeft: '16px',
    },
    enhancedModeToggle: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '10px 12px',
        backgroundColor: '#2a2a2a',
        borderRadius: '6px',
        border: '1px solid #3a3a3a',
    },
    toggleContainer: {
        display: 'flex',
        alignItems: 'center',
        gap: '10px',
    },
    toggleText: {
        color: '#ccc',
        fontSize: '13px',
        fontWeight: '400',
    },
    helpIcon: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: '20px',
        height: '20px',
        color: '#888',
        cursor: 'pointer',
        borderRadius: '4px',
        transition: 'all 0.15s ease',
    },
    externalHelpBox: {
        padding: '12px',
        backgroundColor: '#1e1e1e',
        borderRadius: '6px',
        border: '1px solid #3a3a3a',
        marginTop: '8px',
    },
    helpTitle: {
        fontSize: '13px',
        fontWeight: '600',
        color: '#e0e0e0',
        marginBottom: '8px',
    },
    helpContent: {
        fontSize: '12px',
        color: '#b0b0b0',
        lineHeight: '1.5',
        marginBottom: '10px',
    },
    helpSection: {
        marginBottom: '8px',
    },
    helpSectionTitle: {
        fontSize: '12px',
        fontWeight: '500',
        color: '#ccc',
        marginBottom: '4px',
    },
    helpSectionContent: {
        fontSize: '12px',
        color: '#999',
        lineHeight: '1.4',
        paddingLeft: '4px',
    },
    inlineCode: {
        backgroundColor: '#2a2a2a',
        color: '#4ec9b0',
        padding: '2px 6px',
        borderRadius: '3px',
        fontFamily: 'var(--font-mono)',
        fontSize: '11px',
    },
    serverStatusList: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '4px',
        marginTop: '8px',
    },
    serverStatusItem: {
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
    },
    serverStatusDot: {
        width: '6px',
        height: '6px',
        borderRadius: '50%',
    },
    serverStatusName: {
        color: '#bbb',
        fontSize: '11px',
    },
    serverStatusText: {
        fontSize: '11px',
        fontWeight: '500',
    },
    mcpStatusBox: {
        padding: '10px 12px',
        backgroundColor: '#1e1e1e',
        borderRadius: '6px',
        border: '1px solid #3a3a3a',
        marginTop: '10px',
    },
    mcpStatusTitle: {
        fontSize: '12px',
        fontWeight: '600',
        color: '#e0e0e0',
        marginBottom: '8px',
    },
    mcpServerList: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '4px',
    },
    mcpServerItem: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        padding: '4px 0',
    },
    mcpStatusDot: {
        width: '8px',
        height: '8px',
        borderRadius: '50%',
    },
    mcpServerName: {
        color: '#bbb',
        fontSize: '13px',
        flex: 1,
    },
    mcpStatusText: {
        fontSize: '12px',
        fontWeight: '500',
    },
    advancedOptions: {
        width: '100%',
        fontSize: '12px',
    },
    advancedSummary: {
        padding: '6px 0',
        color: '#888',
        cursor: 'pointer',
        userSelect: 'none' as const,
        outline: 'none',
        listStyle: 'none',
        display: 'flex',
        alignItems: 'center',
        gap: '4px',
    },
    advancedContent: {
        padding: '8px 0',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '8px',
    },
    mcpStatusBoxCompact: {
        display: 'flex',
        flexWrap: 'wrap' as const,
        gap: '8px',
    },
    mcpServerItemCompact: {
        display: 'flex',
        alignItems: 'center',
        gap: '4px',
    },
    mcpServerNameCompact: {
        color: '#999',
        fontSize: '11px',
    },
    enhancedModeTag: {
        display: 'inline-flex',
        alignItems: 'center',
        gap: '4px',
        padding: '2px 8px',
        backgroundColor: '#2a3a2a',
        borderRadius: '4px',
        fontSize: '11px',
    },
    enhancedModeTagText: {
        color: '#7a9a7a',
    },
    enhancedModeTagDot: {
        color: '#4ade80',
        fontSize: '8px',
        cursor: 'help',
    },
    switchLabel: {
        display: 'flex',
        alignItems: 'center',
        cursor: 'pointer',
        userSelect: 'none' as const,
    },
    switchCheckbox: {
        display: 'none',
    },
    switchSlider: {
        width: '36px',
        height: '20px',
        borderRadius: '10px',
        position: 'relative' as const,
        transition: 'all 0.2s ease',
    },
};

const existing = document.getElementById('opscopilot-animations');
if (!existing) {
    const styleSheet = document.createElement("style");
    styleSheet.id = 'opscopilot-animations';
    styleSheet.textContent = `
        @keyframes spin { 100% { transform: rotate(360deg); } }

        /* Troubleshooting switch toggle styles */
        .troubleshoot-switch-slider {
            background-color: #424242;
        }
        .troubleshoot-switch-checkbox:checked + .troubleshoot-switch-slider {
            background-color: #4ade80;
        }
        .troubleshoot-switch-checkbox:checked + .troubleshoot-switch-slider::after {
            transform: translateX(16px);
        }
        .troubleshoot-switch-slider::after {
            content: '';
            position: absolute;
            top: 2px;
            left: 2px;
            width: 16px;
            height: 16px;
            background-color: white;
            border-radius: 50%;
            transition: transform 0.2s ease;
        }

        /* Advanced options details/summary styles */
        details summary::-webkit-details-marker {
            display: none;
        }
        details summary::before {
            content: '▸';
            display: inline-block;
            font-size: 10px;
            transition: transform 0.2s ease;
        }
        details[open] summary::before {
            transform: rotate(90deg);
        }
    `;
    document.head.appendChild(styleSheet);
}

export default TroubleshootingPanel;
