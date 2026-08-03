import React from 'react';

export type StatTone = 'neutral' | 'good' | 'warn' | 'bad';

interface StatTileProps {
    icon: string;
    label: string;
    value: string;
    sub?: string;
    tone?: StatTone;
    onClick?: () => void;
}

export default function StatTile({ icon, label, value, sub, tone = 'neutral', onClick }: StatTileProps) {
    const toneStyle =
        tone === 'good' ? styles.good :
            tone === 'warn' ? styles.warn :
                tone === 'bad' ? styles.bad :
                    styles.neutral;

    return (
        <div
            style={{ ...styles.tile, ...toneStyle, cursor: onClick ? 'pointer' : 'default' }}
            onClick={onClick}
            role={onClick ? 'button' : undefined}
        >
            <div style={styles.topRow}>
                <div style={styles.icon}>{icon}</div>
                <div style={styles.label}>{label}</div>
            </div>
            <div style={styles.value}>{value}</div>
            {sub && <div style={styles.sub}>{sub}</div>}
        </div>
    );
}

const styles: Record<string, React.CSSProperties> = {
    tile: {
        borderRadius: '10px',
        border: '1px solid var(--bg-elevated)',
        backgroundColor: 'var(--bg-elevated)',
        padding: '10px',
        display: 'flex',
        flexDirection: 'column',
        gap: '6px',
        minWidth: 0
    },
    topRow: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px'
    },
    icon: {
        width: '24px',
        height: '24px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontSize: '16px',
        backgroundColor: 'var(--bg-primary)',
        borderRadius: '8px',
        border: '1px solid var(--bg-elevated)'
    },
    label: {
        fontSize: '11px',
        color: 'var(--text-tertiary)'
    },
    value: {
        fontSize: '16px',
        fontWeight: 800,
        color: 'var(--text-primary)',
        letterSpacing: '0.2px',
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis'
    },
    sub: {
        fontSize: '11px',
        color: 'var(--text-muted)',
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis'
    },
    neutral: {
        boxShadow: '0 0 0 rgba(0,0,0,0)'
    },
    good: {
        borderColor: 'var(--success-bg-subtle)'
    },
    warn: {
        borderColor: 'var(--warning-bg-subtle)'
    },
    bad: {
        borderColor: 'var(--danger-bg-subtle)'
    }
};

