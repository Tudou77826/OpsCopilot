import React, { useState, useEffect, useRef } from 'react';
import TroubleshootingStep from './TroubleshootingStep';
import CommandCard from './CommandCard';
import SessionReviewModal from './SessionReviewModal';
import SessionManager from './SessionManager';
import { ConnectionConfig } from '../../types';

interface Message {
    role: 'user' | 'ai';
    content: string;
    timestamp: number;
}

interface SidebarProps {
    isOpen: boolean;
    activeTab: 'sessions' | 'ai';
    onToggle: () => void;
    onStart?: () => void;
    onStop?: () => void;
    onConnect: (config: ConnectionConfig) => void;
}

const Sidebar: React.FC<SidebarProps> = ({ isOpen, activeTab, onToggle, onStart, onStop, onConnect }) => {
    // const [activeTab, setActiveTab] = useState<'ai' | 'sessions'>('sessions'); // Now controlled by prop
    const [isInvestigating, setIsInvestigating] = useState(false);
    const [input, setInput] = useState('');
    const [messages, setMessages] = useState<Message[]>([]);
    const [width, setWidth] = useState(350);
    const [isStopping, setIsStopping] = useState(false);
    const [rootCause, setRootCause] = useState('');
    const [isReviewModalOpen, setIsReviewModalOpen] = useState(false);
    const [isPolishing, setIsPolishing] = useState(false);
    const messagesEndRef = useRef<HTMLDivElement>(null);

    const scrollToBottom = () => {
        messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
    };

    useEffect(() => {
        scrollToBottom();
    }, [messages]);

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
        } catch (e: any) {
            console.error("Initial AI analysis failed", e);
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
            const data = JSON.parse(content);
            if (data && (data.steps || data.commands)) {
                return (
                    <div style={styles.structuredResponse}>
                        {data.steps && data.steps.length > 0 && (
                            <div style={styles.section}>
                                <h4 style={styles.sectionTitle}>排查思路</h4>
                                {data.steps.map((step: string, idx: number) => (
                                    <TroubleshootingStep key={idx} step={step} index={idx} />
                                ))}
                            </div>
                        )}
                        
                        {data.commands && data.commands.length > 0 && (
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
        } catch (e) {
        }
        return <div style={styles.messageContent}>{content}</div>;
    };

    if (!isOpen) {
        // Return null or empty div because App.tsx handles the collapsed view icons now
        return null;
    }

    return (
        <div style={{...styles.container, width: width, position: 'relative'}}>
            <style>{`
                .hide-scrollbar::-webkit-scrollbar {
                    display: none;
                }
                .hide-scrollbar {
                    -ms-overflow-style: none;
                    scrollbar-width: none;
                }
            `}</style>
            <div
                style={styles.resizeHandle}
                onMouseDown={startResizing}
            />

            {/* Header */}
            <div style={styles.header}>
                <h3 style={styles.title}>
                    {activeTab === 'ai' ? 'AI 助手' : '会话管理'}
                </h3>
                <button onClick={onToggle} style={styles.closeButton} aria-label="Toggle Sidebar">×</button>
            </div>

            <div style={styles.mainArea}>
                {/* Content Area */}
                <div style={styles.content}>
                {activeTab === 'sessions' ? (
                    <SessionManager onConnect={onConnect} />
                ) : (
                    <>
                        {!isInvestigating ? (
                            <div style={styles.emptyState}>
                                <div style={styles.icon}>🔍</div>
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
                                    ))}
                                    <div ref={messagesEndRef} />
                                </div>
                            </div>
                        )}
                    </>
                )}
                </div>
            </div>

            {/* Footer - Only for AI Tab when investigating */}
            {activeTab === 'ai' && isInvestigating && (
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
    collapsedContainer: {
        width: '40px',
        backgroundColor: '#252526',
        borderLeft: '1px solid #333',
        display: 'flex',
        flexDirection: 'column' as const,
        alignItems: 'center',
        paddingTop: '10px',
    },
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
    toggleButton: {
        background: 'none',
        border: 'none',
        color: '#ccc',
        cursor: 'pointer',
        fontSize: '20px',
        padding: '5px',
    },
    content: {
        flex: 1,
        overflow: 'hidden',
        display: 'flex',
        flexDirection: 'column' as const,
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
    }
};

export default Sidebar;
