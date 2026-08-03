import React, { useMemo, useState } from 'react';

export interface CommandResult {
    command: string;
    output: string;
    error?: string;
}

interface CommandBlockProps {
    title: string;
    result: CommandResult;
    defaultOpen?: boolean;
}

export default function CommandBlock({ title, result, defaultOpen = false }: CommandBlockProps) {
    const [open, setOpen] = useState(defaultOpen);
    const tone = result.error ? 'var(--severity-danger)' : 'var(--text-muted)';
    const content = result.error ? result.error : (result.output || '(no output)');

    const commandText = useMemo(() => {
        const c = (result.command || '').trim();
        if (!c) return '';
        if (c.length > 90) return c.slice(0, 90) + '...';
        return c;
    }, [result.command]);

    return (
        <div style={styles.container}>
            <div style={styles.header} onClick={() => setOpen(v => !v)} role="button">
                <div style={styles.left}>
                    <div style={styles.title}>{title}</div>
                    {commandText && <div style={styles.cmd}>{commandText}</div>}
                </div>
                <div style={{ ...styles.chev, color: tone }}>{open ? '▾' : '▸'}</div>
            </div>
            {open && (
                <pre style={{ ...styles.pre, color: result.error ? 'var(--severity-danger)' : 'var(--text-secondary)' }}>
                    {content}
                </pre>
            )}
        </div>
    );
}

const styles: Record<string, React.CSSProperties> = {
    container: {
        borderRadius: '10px',
        border: '1px solid var(--bg-elevated)',
        backgroundColor: 'var(--bg-elevated)',
        overflow: 'hidden'
    },
    header: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: '12px',
        padding: '10px 12px',
        cursor: 'pointer',
        userSelect: 'none'
    },
    left: {
        display: 'flex',
        flexDirection: 'column',
        gap: '4px',
        minWidth: 0
    },
    title: {
        fontSize: '12px',
        color: 'var(--text-primary)',
        fontWeight: 700
    },
    cmd: {
        fontSize: '11px',
        color: 'var(--text-muted)',
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis'
    },
    chev: {
        fontSize: '14px',
        flexShrink: 0
    },
    pre: {
        margin: 0,
        padding: '10px 12px',
        borderTop: '1px solid var(--bg-elevated)',
        fontSize: '12px',
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-word',
        fontFamily: 'var(--font-mono)',
        backgroundColor: 'var(--bg-primary)'
    }
};

