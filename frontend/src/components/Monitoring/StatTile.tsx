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
        border: '1px solid #2a2a2a',
        backgroundColor: '#141414',
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
        backgroundColor: '#1f1f1f',
        borderRadius: '8px',
        border: '1px solid #2a2a2a'
    },
    label: {
        fontSize: '11px',
        color: '#a8a8a8'
    },
    value: {
        fontSize: '16px',
        fontWeight: 800,
        color: '#f2f2f2',
        letterSpacing: '0.2px',
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis'
    },
    sub: {
        fontSize: '11px',
        color: '#8a8a8a',
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis'
    },
    neutral: {
        boxShadow: '0 0 0 rgba(0,0,0,0)'
    },
    good: {
        borderColor: '#1f3a2a'
    },
    warn: {
        borderColor: '#3a2f1f'
    },
    bad: {
        borderColor: '#3a1f1f'
    }
};

