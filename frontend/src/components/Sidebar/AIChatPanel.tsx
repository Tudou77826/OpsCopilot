import React, { useState, useEffect, useRef } from 'react';
import MessageRenderer from './MessageRenderer';

interface Message {
    role: 'user' | 'ai';
    content: string;
    timestamp: number;
}

const AIChatPanel: React.FC = () => {
    const [input, setInput] = useState('');
    const [messages, setMessages] = useState<Message[]>([]);
    const messagesEndRef = useRef<HTMLDivElement>(null);

    const scrollToBottom = () => {
        messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
    };

    useEffect(() => {
        scrollToBottom();
    }, [messages]);

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

    const handleNewChat = () => {
        setMessages([]);
        setInput('');
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
};

export default AIChatPanel;
