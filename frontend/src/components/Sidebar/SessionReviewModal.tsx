import React, { useState, useEffect } from 'react';
import ReactDOM from 'react-dom';

interface TimelineEvent {
    timestamp: string;
    type: string;
    content: string;
    metadata?: any;
}

interface SessionReviewModalProps {
    isOpen: boolean;
    onClose: () => void;
    rootCause: string;
    onArchive: (conclusion: string) => void;
}

const SessionReviewModal: React.FC<SessionReviewModalProps> = ({ isOpen, onClose, rootCause, onArchive }) => {
    const [events, setEvents] = useState<TimelineEvent[]>([]);
    const [view, setView] = useState<'timeline' | 'conclusion'>('timeline');
    const [conclusion, setConclusion] = useState('');
    const [isLoading, setIsLoading] = useState(false);
    const [markdownContent, setMarkdownContent] = useState('');

    useEffect(() => {
        if (isOpen) {
            console.log("[SessionReviewModal] Modal opened, loading timeline...");
            loadTimeline();
            setView('timeline');
            setConclusion('');
        }
    }, [isOpen]);

    const loadTimeline = async () => {
        try {
            console.log("[SessionReviewModal] Calling backend GetSessionTimeline...");
            // @ts-ignore
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.GetSessionTimeline) {
                // @ts-ignore
                const sessionData = await window.go.main.App.GetSessionTimeline();
                console.log("[SessionReviewModal] Raw session data received:", sessionData);
                
                if (sessionData && sessionData.timeline && Array.isArray(sessionData.timeline)) {
                    const data = sessionData.timeline;
                    const problem = sessionData.problem || "未指定";

                    // Filter out terminal_output as requested
                    const filtered = data.filter((e: TimelineEvent) => {
                        // 1. Filter out terminal_output
                        if (e.type === 'terminal_output') return false;
                        
                        // 2. Filter out empty content
                        if (!e.content || !e.content.trim()) return false;

                        // 3. Filter out control characters/garbage
                        // Check for common control char placeholders or very short non-alphanumeric garbage
                        const trimmed = e.content.trim();
                        if (trimmed === '' || trimmed === '^V') return false; // Ctrl+V artifact
                        
                        return true;
                    });

                    console.log("[SessionReviewModal] Filtered events:", filtered);
                    setEvents(filtered);

                    // Generate Markdown
                    let md = `# 排查会话记录\n\n`;
                    md += `**排查目标:** ${problem}\n\n`;
                    md += `**根本原因:** ${rootCause}\n\n`;
                    md += `## 详细过程\n\n`;
                    
                    filtered.forEach((e: TimelineEvent) => {
                        // Skip empty content
                        if (!e.content || !e.content.trim()) return;

                        // No timestamp, just Type
                        md += `### ${translateType(e.type)}\n`;
                        md += `${e.content.trim()}\n\n`;
                    });
                    setMarkdownContent(md);

                } else {
                    console.warn("[SessionReviewModal] Session data is empty or invalid format", sessionData);
                    setEvents([]);
                    setMarkdownContent(`# 排查会话记录\n\n**根本原因:** ${rootCause}\n\n（无记录）`);
                }
            } else {
                console.error("[SessionReviewModal] Backend API GetSessionTimeline not available in window.go.main.App");
            }
        } catch (e) {
            console.error("[SessionReviewModal] Error loading timeline:", e);
        } finally {
            setIsLoading(false);
        }
    };

    const handleDelete = (index: number) => {
        const newEvents = [...events];
        newEvents.splice(index, 1);
        setEvents(newEvents);
    };

    const handleSaveDraft = async () => {
        // Save changes to backend
        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.UpdateSessionTimeline) {
            // @ts-ignore
            await window.go.main.App.UpdateSessionTimeline(events);
        }
        onClose();
    };

    const handleAnalyze = async () => {
        setIsLoading(true);
        try {
            // Use the edited markdown content as context for AI
            // @ts-ignore
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.GenerateConclusionWithContext) {
                // @ts-ignore
                const result = await window.go.main.App.GenerateConclusionWithContext(markdownContent, rootCause);
                setConclusion(result);
                setView('conclusion');
            }
        } catch (e) {
            console.error(e);
            alert("生成总结失败: " + e);
        } finally {
            setIsLoading(false);
        }
    };

    const handleArchive = () => {
        onArchive(conclusion);
    };

    if (!isOpen) return null;

    return ReactDOM.createPortal(
        <div style={styles.overlay} onClick={(e) => {
            // Close if clicking overlay (optional, but good for testing)
            if (e.target === e.currentTarget) {
                // onClose(); 
            }
        }}>
            <div style={styles.modal}>
                <div style={styles.header}>
                    <h3 style={styles.title}>
                        {view === 'timeline' ? '编辑排查记录' : '确认排查总结'}
                    </h3>
                    <button onClick={onClose} style={styles.closeButton}>×</button>
                </div>

                <div style={styles.body}>
                    {view === 'timeline' ? (
                        <div style={styles.conclusionContainer}>
                            <textarea
                                value={markdownContent}
                                onChange={(e) => setMarkdownContent(e.target.value)}
                                style={styles.textarea}
                                placeholder="正在生成排查记录..."
                            />
                        </div>
                    ) : (
                        <div style={styles.conclusionContainer}>
                            <textarea
                                value={conclusion}
                                onChange={(e) => setConclusion(e.target.value)}
                                style={styles.textarea}
                                placeholder="AI 正在生成总结..."
                            />
                        </div>
                    )}
                </div>

                <div style={styles.footer}>
                    {view === 'timeline' ? (
                        <>
                            <button 
                                onClick={handleAnalyze} 
                                style={styles.primaryButton}
                                disabled={isLoading}
                            >
                                {isLoading ? '分析中...' : 'AI 解析'}
                            </button>
                        </>
                    ) : (
                        <>
                            <button onClick={() => setView('timeline')} style={styles.secondaryButton}>上一步</button>
                            <button onClick={handleArchive} style={styles.primaryButton}>归档</button>
                        </>
                    )}
                </div>
            </div>
        </div>,
        document.body
    );
};

const translateType = (type: string) => {
    switch (type) {
        case 'user_query': return '用户提问';
        case 'ai_suggestion': return 'AI 建议';
        case 'terminal_input': return '终端执行';
        default: return type;
    }
};

const formatTime = (ts: string) => {
    return new Date(ts).toLocaleTimeString();
};

const styles = {
    overlay: {
        position: 'fixed' as const,
        top: 0, left: 0, right: 0, bottom: 0,
        backgroundColor: 'rgba(0,0,0,0.85)', // Darker background
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 99999, // Super high z-index
        backdropFilter: 'blur(4px)', // Nice blur effect
    },
    modal: {
        backgroundColor: '#252526',
        width: '600px',
        maxWidth: '90%',
        height: '80%',
        display: 'flex',
        flexDirection: 'column' as const,
        borderRadius: '8px',
        boxShadow: '0 4px 12px rgba(0,0,0,0.5)',
        border: '1px solid #333',
    },
    header: {
        padding: '16px',
        borderBottom: '1px solid #333',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
    },
    title: {
        margin: 0,
        color: '#fff',
        fontSize: '16px',
    },
    closeButton: {
        background: 'none',
        border: 'none',
        color: '#ccc',
        fontSize: '24px',
        cursor: 'pointer',
    },
    body: {
        flex: 1,
        overflowY: 'auto' as const,
        padding: '16px',
    },
    footer: {
        padding: '16px',
        borderTop: '1px solid #333',
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '12px',
    },
    timelineList: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '12px',
    },
    eventItem: {
        backgroundColor: '#333',
        borderRadius: '6px',
        padding: '12px',
    },
    eventHeader: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        marginBottom: '8px',
    },
    eventType: {
        color: '#4ec9b0',
        fontSize: '13px',
        fontWeight: 'bold' as const,
    },
    eventTime: {
        color: '#888',
        fontSize: '12px',
    },
    deleteButton: {
        background: 'none',
        border: 'none',
        cursor: 'pointer',
        fontSize: '14px',
        padding: '4px',
    },
    eventContent: {
        color: '#ccc',
        fontSize: '13px',
        whiteSpace: 'pre-wrap' as const,
        fontFamily: 'monospace',
    },
    empty: {
        textAlign: 'center' as const,
        color: '#888',
        marginTop: '40px',
    },
    conclusionContainer: {
        height: '100%',
        display: 'flex',
        flexDirection: 'column' as const,
    },
    textarea: {
        flex: 1,
        backgroundColor: '#1e1e1e',
        color: '#fff',
        border: '1px solid #333',
        borderRadius: '4px',
        padding: '12px',
        resize: 'none' as const,
        fontFamily: 'inherit',
        fontSize: '14px',
        lineHeight: '1.5',
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
        padding: '8px 16px',
        backgroundColor: '#333',
        color: '#ccc',
        border: '1px solid #555',
        borderRadius: '4px',
        cursor: 'pointer',
    },
};

export default SessionReviewModal;