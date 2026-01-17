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
    const tone = result.error ? '#ff8a8a' : '#8a8a8a';
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
                <pre style={{ ...styles.pre, color: result.error ? '#ffb3b3' : '#d6d6d6' }}>
                    {content}
                </pre>
            )}
        </div>
    );
}

const styles: Record<string, React.CSSProperties> = {
    container: {
        borderRadius: '10px',
        border: '1px solid #2a2a2a',
        backgroundColor: '#141414',
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
        color: '#eaeaea',
        fontWeight: 700
    },
    cmd: {
        fontSize: '11px',
        color: '#8a8a8a',
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
        borderTop: '1px solid #2a2a2a',
        fontSize: '12px',
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-word',
        fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
        backgroundColor: '#101010'
    }
};

