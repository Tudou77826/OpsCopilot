import React from 'react';

interface CardProps {
    title?: string;
    right?: React.ReactNode;
    children: React.ReactNode;
    style?: React.CSSProperties;
}

export default function Card({ title, right, children, style }: CardProps) {
    return (
        <div style={{ ...styles.card, ...style }}>
            {(title || right) && (
                <div style={styles.header}>
                    <div style={styles.title}>{title}</div>
                    <div style={styles.right}>{right}</div>
                </div>
            )}
            <div style={styles.body}>{children}</div>
        </div>
    );
}

const styles: Record<string, React.CSSProperties> = {
    card: {
        border: '1px solid #2e2e2e',
        borderRadius: '10px',
        backgroundColor: '#1b1b1b',
        overflow: 'hidden'
    },
    header: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '10px 12px',
        borderBottom: '1px solid #2a2a2a',
        backgroundColor: '#181818'
    },
    title: {
        fontSize: '12px',
        color: '#cfcfcf',
        fontWeight: 700,
        letterSpacing: '0.2px'
    },
    right: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px'
    },
    body: {
        padding: '12px'
    }
};

