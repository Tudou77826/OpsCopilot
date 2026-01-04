import React, { useState, useEffect, useRef } from 'react';
import TroubleshootingStep from './TroubleshootingStep';
import CommandCard from './CommandCard';

interface Message {
    role: 'user' | 'ai';
    content: string;
    timestamp: number;
}

interface SidebarProps {
    isOpen: boolean;
    onToggle: () => void;
    onStart?: () => void;
    onStop?: () => void;
}

const Sidebar: React.FC<SidebarProps> = ({ isOpen, onToggle, onStart, onStop }) => {
    const [isInvestigating, setIsInvestigating] = useState(false);
    const [input, setInput] = useState('');
    const [messages, setMessages] = useState<Message[]>([]);
    const [width, setWidth] = useState(350);
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
             // Calculate width from right edge: Body Width - Mouse X
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

    const handleStart = () => {
        setIsInvestigating(true);
        if (onStart) onStart();
        setMessages([{
            role: 'ai',
            content: '已开始排查会话。请告诉我您遇到的问题现象。',
            timestamp: Date.now()
        }]);
    };

    const handleStop = () => {
        setIsInvestigating(false);
        if (onStop) onStop();
        setMessages(prev => [...prev, {
            role: 'ai',
            content: '会话已结束。',
            timestamp: Date.now()
        }]);
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

        // Integration: Call Backend AskAI
        try {
            // @ts-ignore - access wails runtime
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.AskAI) {
                // @ts-ignore
                const response = await window.go.main.App.AskAI(userMsg.content);
                
                setMessages(prev => [...prev, {
                    role: 'ai',
                    content: response,
                    timestamp: Date.now()
                }]);
            } else {
                 // Fallback for UI development without backend
                 setMessages(prev => [...prev, {
                    role: 'ai',
                    content: "Error: Backend not connected (AskAI method missing). Please rebuild Wails app.",
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
            // Not JSON, fall through to text render
        }
        return <div style={styles.messageContent}>{content}</div>;
    };

    if (!isOpen) {
        return (
            <div style={styles.collapsedContainer}>
                <button 
                    onClick={onToggle}
                    style={styles.toggleButton}
                    title="Toggle Sidebar"
                    aria-label="Toggle Sidebar"
                >
                    🤖
                </button>
            </div>
        );
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
                <h3 style={styles.title}>AI 助手</h3>
                <button onClick={onToggle} style={styles.closeButton} aria-label="Toggle Sidebar">×</button>
            </div>

            {/* Content */}
            <div style={styles.content}>
                {!isInvestigating ? (
                    <div style={styles.emptyState}>
                        <div style={styles.icon}>🔍</div>
                        <p style={styles.emptyText}>点击下方按钮开始智能排查</p>
                        <button onClick={handleStart} style={styles.primaryButton}>
                            开始排查
                        </button>
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
            </div>

            {/* Footer */}
            {isInvestigating && (
                <div style={styles.footer}>
                    <div style={styles.toolbar}>
                        <button onClick={handleStop} style={styles.secondaryButton}>结束排查</button>
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
                </div>
            )}
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
        // Width is handled by inline style
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
        borderBottom: '1px solid #333',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
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
    }
};

export default Sidebar;
