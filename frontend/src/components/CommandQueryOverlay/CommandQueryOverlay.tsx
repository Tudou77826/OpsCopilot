import React, { useEffect, useRef, useState, useCallback, MouseEvent } from 'react';

export interface CommandQueryResult {
    command: string;
    explanation?: string;
}

interface HistoryEntry {
    id: string;
    query: string;
    command: string;
    pinned: boolean;
    createdAt: number;
}

const HISTORY_KEY = 'opscopilot:commandQueryHistory';
const MAX_UNPINNED = 30;

function loadHistory(): HistoryEntry[] {
    try {
        const raw = localStorage.getItem(HISTORY_KEY);
        if (!raw) return [];
        return JSON.parse(raw);
    } catch {
        return [];
    }
}

function saveHistory(entries: HistoryEntry[]) {
    localStorage.setItem(HISTORY_KEY, JSON.stringify(entries));
}

const PinIcon: React.FC<{ active: boolean }> = ({ active }) => (
    <svg width="12" height="12" viewBox="0 0 16 16" fill={active ? 'var(--warning)' : 'var(--text-disabled)'}>
        <path d="M8 1.3l1.8 4.0H14l-3.4 2.7 1.3 4.3L8 9.8 4.1 12.3l1.3-4.3L2 5.3h4.2z"/>
    </svg>
);

const TrashIcon: React.FC = () => (
    <svg width="12" height="12" viewBox="0 0 16 16" fill="var(--text-disabled)">
        <path d="M6.5 1a.5.5 0 0 0-.5.5V2H3.5a.5.5 0 0 0 0 1h9a.5.5 0 0 0 0-1H10v-.5a.5.5 0 0 0-.5-.5h-3zM4.5 4l.5 9.5a1 1 0 0 0 1 .5h4a1 1 0 0 0 1-.5L11.5 4h-7z"/>
    </svg>
);

interface CommandQueryOverlayProps {
    visible: boolean;
    query: string;
    loading: boolean;
    result: CommandQueryResult | null;
    error: string;
    onQueryChange: (value: string) => void;
    onGenerate: () => void;
    onRegenerate: () => void;
    onCopy: () => void;
    onType: () => void;
    onClose: () => void;
    onSelectHistory?: (entry: HistoryEntry) => void;
}

const CommandQueryOverlay: React.FC<CommandQueryOverlayProps> = ({
    visible,
    query,
    loading,
    result,
    error,
    onQueryChange,
    onGenerate,
    onRegenerate,
    onCopy,
    onType,
    onClose,
    onSelectHistory,
}) => {
    const inputRef = useRef<HTMLInputElement>(null);
    const [history, setHistory] = useState<HistoryEntry[]>([]);
    const [showHistory, setShowHistory] = useState(true);
    const [hoveredId, setHoveredId] = useState<string | null>(null);
    const [copied, setCopied] = useState(false);

    // 加载历史
    useEffect(() => {
        if (visible) {
            setHistory(loadHistory());
            setShowHistory(true);
        }
    }, [visible]);

    // 生成成功后写入历史
    useEffect(() => {
        if (!visible || !result?.command || !query.trim()) return;
        setHistory(prev => {
            if (prev.some(e => e.command === result.command && e.query === query.trim())) return prev;
            const entry: HistoryEntry = {
                id: Date.now().toString(36) + Math.random().toString(36).slice(2, 6),
                query: query.trim(),
                command: result.command,
                pinned: false,
                createdAt: Date.now(),
            };
            const next = [entry, ...prev];
            const pinned = next.filter(e => e.pinned);
            const unpinned = next.filter(e => !e.pinned).slice(0, MAX_UNPINNED);
            const merged = [...pinned, ...unpinned];
            saveHistory(merged);
            return merged;
        });
    }, [visible, result?.command, query]);

    // Escape 关闭
    useEffect(() => {
        if (!visible) return;
        const handleKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Escape') {
                e.preventDefault();
                onClose();
            }
        };
        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [visible, onClose]);

    // 自动聚焦
    useEffect(() => {
        if (!visible) return;
        const t = setTimeout(() => inputRef.current?.focus(), 0);
        return () => clearTimeout(t);
    }, [visible]);

    const togglePin = useCallback((id: string) => {
        setHistory(prev => {
            const next = prev.map(e => e.id === id ? { ...e, pinned: !e.pinned } : e);
            saveHistory(next);
            return next;
        });
    }, []);

    const deleteEntry = useCallback((id: string) => {
        setHistory(prev => {
            const next = prev.filter(e => e.id !== id);
            saveHistory(next);
            return next;
        });
    }, []);

    const selectHistory = useCallback((entry: HistoryEntry) => {
        setShowHistory(false);
        if (onSelectHistory) {
            onSelectHistory(entry);
        } else {
            onQueryChange(entry.query);
        }
    }, [onQueryChange, onSelectHistory]);

    if (!visible) return null;

    const canOperate = !!result?.command && !loading;

    const sorted = [...history].sort((a, b) => {
        if (a.pinned !== b.pinned) return a.pinned ? -1 : 1;
        return b.createdAt - a.createdAt;
    });

    const shouldShowHistory = showHistory && !query.trim() && sorted.length > 0;

    return (
        <>
            <div style={styles.backdrop} onClick={onClose} />
            <div style={styles.container}>
                <div style={styles.searchSection}>
                    <input
                        ref={inputRef}
                        style={styles.input}
                        value={query}
                        onChange={(e) => {
                            const val = e.target.value;
                            onQueryChange(val);
                            setShowHistory(!val.trim());
                        }}
                        onFocus={() => { if (!query.trim()) setShowHistory(true); }}
                        placeholder="描述你的诉求，例如：查端口是否被占用"
                        onKeyDown={(e) => {
                            if (e.key === 'Enter') {
                                e.preventDefault();
                                setShowHistory(false);
                                onGenerate();
                            }
                        }}
                    />
                    <button style={styles.primaryBtn} onClick={onGenerate} disabled={loading || !query.trim()}>
                        {loading ? '生成中…' : '生成'}
                    </button>
                </div>

                {shouldShowHistory && (
                    <div style={styles.historyList}>
                        {sorted.map(entry => (
                            <div
                                key={entry.id}
                                style={{
                                    ...styles.historyItem,
                                    backgroundColor: hoveredId === entry.id ? 'var(--bg-elevated)' : 'transparent',
                                }}
                                onClick={() => selectHistory(entry)}
                                onMouseEnter={() => setHoveredId(entry.id)}
                                onMouseLeave={() => setHoveredId(null)}
                            >
                                <div style={styles.historyContent}>
                                    <span style={styles.historyCommand}>{entry.command}</span>
                                    <span style={styles.historyDesc}>
                                        {entry.query.length > 40 ? entry.query.slice(0, 40) + '…' : entry.query}
                                    </span>
                                </div>
                                <div style={styles.historyActions} onClick={(e: MouseEvent) => e.stopPropagation()}>
                                    <button
                                        style={styles.iconBtn}
                                        onClick={() => togglePin(entry.id)}
                                        title={entry.pinned ? '取消钉住' : '钉住'}
                                    >
                                        <PinIcon active={entry.pinned} />
                                    </button>
                                    <button
                                        style={styles.iconBtn}
                                        onClick={() => deleteEntry(entry.id)}
                                        title="删除"
                                    >
                                        <TrashIcon />
                                    </button>
                                </div>
                            </div>
                        ))}
                    </div>
                )}

                {result?.command && (
                    <div style={styles.resultSection}>
                        <div style={styles.resultBox}>
                            <div style={styles.commandLine}>{result.command}</div>
                            {result.explanation && <div style={styles.explanation}>{result.explanation}</div>}
                        </div>
                        <div style={styles.actionsRow}>
                            <button
                                style={copied ? styles.copiedBtn : styles.secondaryBtn}
                                onClick={() => {
                                    onCopy();
                                    setCopied(true);
                                    setTimeout(() => setCopied(false), 1200);
                                }}
                                disabled={!canOperate}
                            >
                                {copied ? '✓ 已复制' : '复制'}
                            </button>
                            <button style={styles.secondaryBtn} onClick={onType} disabled={!canOperate}>键入</button>
                            <button style={styles.secondaryBtn} onClick={onRegenerate} disabled={loading || !query.trim()}>重新生成</button>
                        </div>
                    </div>
                )}
                {!!error && !result?.command && (
                    <div style={styles.errorText}>{error}</div>
                )}
            </div>
        </>
    );
};

const styles: Record<string, React.CSSProperties> = {
    backdrop: {
        position: 'fixed',
        left: 0,
        top: 0,
        right: 0,
        bottom: 0,
        zIndex: 3500,
        backgroundColor: 'rgba(0, 0, 0, 0.4)',
    },
    container: {
        position: 'fixed',
        top: '30%',
        left: '50%',
        transform: 'translate(-50%, -30%)',
        zIndex: 3600,
        width: '480px',
        backgroundColor: 'var(--bg-primary)',
        border: '1px solid var(--border)',
        borderRadius: '10px',
        boxShadow: '0 12px 40px rgba(0, 0, 0, 0.7)',
        overflow: 'hidden',
        color: 'var(--text-secondary)',
        display: 'flex',
        flexDirection: 'column',
    },
    searchSection: {
        display: 'flex',
        gap: '10px',
        padding: '16px 16px 12px',
        borderBottom: '2px solid var(--border)',
        backgroundColor: 'var(--bg-tertiary)',
    },
    input: {
        flex: 1,
        backgroundColor: 'var(--bg-primary)',
        border: '2px solid var(--text-disabled)',
        borderRadius: '8px',
        color: 'var(--text-primary)',
        fontSize: '14px',
        padding: '12px 14px',
        outline: 'none',
        fontWeight: 500,
    },
    primaryBtn: {
        backgroundColor: 'var(--accent)',
        border: 'none',
        color: 'var(--text-primary)',
        borderRadius: '6px',
        padding: '10px 16px',
        fontSize: '13px',
        fontWeight: 600,
        cursor: 'pointer',
        whiteSpace: 'nowrap',
    },
    historyList: {
        maxHeight: '360px',
        overflowY: 'auto',
    },
    historyItem: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '6px 14px',
        cursor: 'pointer',
        borderBottom: '1px solid var(--bg-elevated)',
        transition: 'background-color 0.1s',
    },
    historyContent: {
        display: 'flex',
        flexDirection: 'column',
        gap: '1px',
        overflow: 'hidden',
        flex: 1,
        minWidth: 0,
    },
    historyCommand: {
        fontFamily: 'var(--font-mono)',
        fontSize: '12px',
        color: 'var(--text-secondary)',
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
    },
    historyDesc: {
        fontSize: '11px',
        color: 'var(--text-disabled)',
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
    },
    historyActions: {
        display: 'flex',
        gap: '2px',
        flexShrink: 0,
        marginLeft: '8px',
        opacity: 0.4,
    },
    iconBtn: {
        width: '20px',
        height: '20px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        border: 'none',
        borderRadius: '3px',
        background: 'transparent',
        color: 'var(--text-muted)',
        cursor: 'pointer',
        padding: 0,
    },
    resultSection: {
        display: 'flex',
        flexDirection: 'column',
        gap: '8px',
        padding: '12px 16px 14px',
    },
    resultBox: {
        backgroundColor: 'var(--bg-secondary)',
        border: '1px solid var(--border)',
        borderRadius: '6px',
        padding: '10px 12px',
    },
    commandLine: {
        fontFamily: 'var(--font-mono)',
        fontSize: '13px',
        color: 'var(--text-primary)',
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-word',
        lineHeight: 1.5,
    },
    explanation: {
        marginTop: '8px',
        color: 'var(--text-tertiary)',
        fontSize: '12px',
        lineHeight: 1.4,
    },
    errorText: {
        color: 'var(--severity-danger)',
        fontSize: '12px',
        lineHeight: 1.4,
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-word',
        padding: '8px 16px',
    },
    actionsRow: {
        display: 'flex',
        gap: '8px',
        justifyContent: 'flex-end',
    },
    secondaryBtn: {
        backgroundColor: 'var(--border-subtle)',
        border: '1px solid var(--border)',
        color: 'var(--text-secondary)',
        borderRadius: '6px',
        padding: '6px 10px',
        fontSize: '12px',
        cursor: 'pointer',
    },
    copiedBtn: {
        backgroundColor: 'var(--success-bg-subtle)',
        border: '1px solid var(--success)',
        color: 'var(--severity-success)',
        borderRadius: '6px',
        padding: '6px 10px',
        fontSize: '12px',
        cursor: 'default',
    },
};

export default CommandQueryOverlay;
