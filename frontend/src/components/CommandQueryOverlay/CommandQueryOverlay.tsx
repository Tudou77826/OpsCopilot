import React, { useEffect, useMemo, useRef } from 'react';

export interface CommandQueryResult {
    command: string;
    explanation?: string;
}

interface CommandQueryOverlayProps {
    visible: boolean;
    position: { x: number; y: number };
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
}

const CommandQueryOverlay: React.FC<CommandQueryOverlayProps> = ({
    visible,
    position,
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
}) => {
    const inputRef = useRef<HTMLInputElement>(null);

    const adjustedPosition = useMemo(() => {
        const width = 420;
        const height = result ? 230 : 160;
        const x = Math.min(position.x, window.innerWidth - width - 12);
        const y = Math.min(position.y, window.innerHeight - height - 12);
        return { x: Math.max(12, x), y: Math.max(12, y) };
    }, [position.x, position.y, result]);

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

    useEffect(() => {
        if (!visible) return;
        const t = setTimeout(() => inputRef.current?.focus(), 0);
        return () => clearTimeout(t);
    }, [visible]);

    if (!visible) return null;

    const canOperate = !!result?.command && !loading;

    return (
        <>
            <div style={styles.backdrop} onClick={onClose} />
            <div style={{ ...styles.container, left: adjustedPosition.x, top: adjustedPosition.y }}>
                <div style={styles.header}>
                    <div style={styles.title}>命令查询</div>
                    <button style={styles.closeBtn} onClick={onClose} aria-label="关闭">×</button>
                </div>

                <div style={styles.body}>
                    <div style={styles.inputRow}>
                        <input
                            ref={inputRef}
                            style={styles.input}
                            value={query}
                            onChange={(e) => onQueryChange(e.target.value)}
                            placeholder="描述你的诉求，例如：查端口是否被占用"
                            onKeyDown={(e) => {
                                if (e.key === 'Enter') {
                                    e.preventDefault();
                                    onGenerate();
                                }
                            }}
                        />
                        <button style={styles.primaryBtn} onClick={onGenerate} disabled={loading || !query.trim()}>
                            {loading ? '生成中…' : '生成'}
                        </button>
                    </div>

                    {result?.command && (
                        <div style={styles.resultBox}>
                            <div style={styles.commandLine}>{result.command}</div>
                            {result.explanation && <div style={styles.explanation}>{result.explanation}</div>}
                        </div>
                    )}
                    {!!error && (
                        <div style={styles.errorText}>{error}</div>
                    )}

                    <div style={styles.actionsRow}>
                        <button style={styles.secondaryBtn} onClick={onCopy} disabled={!canOperate}>复制</button>
                        <button style={styles.secondaryBtn} onClick={onType} disabled={!canOperate}>键入</button>
                        <button style={styles.secondaryBtn} onClick={onRegenerate} disabled={loading || !query.trim()}>重新生成</button>
                    </div>
                </div>
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
        backgroundColor: 'transparent',
    },
    container: {
        position: 'fixed',
        zIndex: 3600,
        width: '420px',
        backgroundColor: '#252526',
        border: '1px solid #454545',
        borderRadius: '8px',
        boxShadow: '0 8px 24px rgba(0, 0, 0, 0.6)',
        overflow: 'hidden',
        color: '#ccc',
    },
    header: {
        height: '36px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '0 10px',
        backgroundColor: '#1e1e1e',
        borderBottom: '1px solid #333',
    },
    title: {
        fontSize: '12px',
        color: '#ccc',
        userSelect: 'none',
    },
    closeBtn: {
        width: '28px',
        height: '28px',
        borderRadius: '6px',
        border: 'none',
        background: 'transparent',
        color: '#ccc',
        fontSize: '18px',
        cursor: 'pointer',
    },
    body: {
        padding: '10px',
        display: 'flex',
        flexDirection: 'column',
        gap: '10px',
    },
    inputRow: {
        display: 'flex',
        gap: '8px',
    },
    input: {
        flex: 1,
        backgroundColor: '#1e1e1e',
        border: '1px solid #3c3c3c',
        borderRadius: '6px',
        color: '#ccc',
        fontSize: '12px',
        padding: '8px 10px',
        outline: 'none',
    },
    primaryBtn: {
        backgroundColor: '#007acc',
        border: 'none',
        color: '#fff',
        borderRadius: '6px',
        padding: '8px 10px',
        fontSize: '12px',
        cursor: 'pointer',
        whiteSpace: 'nowrap',
    },
    resultBox: {
        backgroundColor: '#1e1e1e',
        border: '1px solid #333',
        borderRadius: '6px',
        padding: '8px 10px',
    },
    commandLine: {
        fontFamily: 'monospace',
        fontSize: '12px',
        color: '#fff',
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-word',
    },
    explanation: {
        marginTop: '6px',
        color: '#aaa',
        fontSize: '11px',
        lineHeight: 1.4,
    },
    errorText: {
        color: '#f48771',
        fontSize: '11px',
        lineHeight: 1.4,
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-word',
    },
    actionsRow: {
        display: 'flex',
        gap: '8px',
        justifyContent: 'flex-end',
    },
    secondaryBtn: {
        backgroundColor: '#2e2e2e',
        border: '1px solid #444',
        color: '#ccc',
        borderRadius: '6px',
        padding: '6px 10px',
        fontSize: '12px',
        cursor: 'pointer',
    },
};

export default CommandQueryOverlay;
