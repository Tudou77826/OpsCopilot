import React, { useRef, useCallback, useState } from 'react';

interface GroupStripProps {
    groups: string[];
    selectedGroup: string;
    onSelectGroup: (group: string) => void;
    onAddGroup: () => void;
}

const ACTIVE_COLOR = 'var(--accent)';

const GroupStrip: React.FC<GroupStripProps> = ({ groups, selectedGroup, onSelectGroup, onAddGroup }) => {
    const [hoveredGroup, setHoveredGroup] = useState<string | null>(null);
    const idx = groups.indexOf(selectedGroup);

    const wheelAccum = useRef(0);

    const handleWheel = useCallback((e: React.WheelEvent) => {
        e.preventDefault();
        wheelAccum.current += e.deltaY;
        if (wheelAccum.current > 150 && idx < groups.length - 1) {
            onSelectGroup(groups[idx + 1]);
            wheelAccum.current = 0;
        } else if (wheelAccum.current < -150 && idx > 0) {
            onSelectGroup(groups[idx - 1]);
            wheelAccum.current = 0;
        }
    }, [idx, groups, onSelectGroup]);

    const hasPrev = idx > 0;
    const hasNext = idx < groups.length - 1;

    return (
        <div
            style={styles.wrap}
            onWheel={handleWheel}
            data-testid="group-strip"
        >
            {hasPrev && (
                <div
                    style={{
                        ...styles.peek,
                        ...styles.peekTop,
                        color: hoveredGroup === groups[idx - 1] ? 'var(--text-muted)' : 'var(--text-disabled)',
                    }}
                    onClick={() => onSelectGroup(groups[idx - 1])}
                    onMouseEnter={() => setHoveredGroup(groups[idx - 1])}
                    onMouseLeave={() => setHoveredGroup(null)}
                    title={groups[idx - 1]}
                    data-testid={`group-item-${groups[idx - 1]}`}
                >
                    {groups[idx - 1]}
                </div>
            )}

            <div
                style={{
                    ...styles.activeCard,
                    backgroundColor: ACTIVE_COLOR,
                    boxShadow: hoveredGroup === selectedGroup
                        ? '0 0 12px rgba(0,122,204,0.3)'
                        : '0 2px 6px rgba(0,122,204,0.2)',
                }}
                onMouseEnter={() => setHoveredGroup(selectedGroup)}
                onMouseLeave={() => setHoveredGroup(null)}
                data-testid={`group-item-${selectedGroup}`}
            >
                {selectedGroup}
            </div>

            {hasNext && (
                <div
                    style={{
                        ...styles.peek,
                        ...styles.peekBottom,
                        color: hoveredGroup === groups[idx + 1] ? 'var(--text-muted)' : 'var(--text-disabled)',
                    }}
                    onClick={() => onSelectGroup(groups[idx + 1])}
                    onMouseEnter={() => setHoveredGroup(groups[idx + 1])}
                    onMouseLeave={() => setHoveredGroup(null)}
                    title={groups[idx + 1]}
                    data-testid={`group-item-${groups[idx + 1]}`}
                >
                    {groups[idx + 1]}
                </div>
            )}

            <div style={styles.addBtn} onClick={onAddGroup} data-testid="group-add-btn" title="新建分组">
                <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
                    <line x1="6" y1="2" x2="6" y2="10" />
                    <line x1="2" y1="6" x2="10" y2="6" />
                </svg>
            </div>
        </div>
    );
};

const styles = {
    wrap: {
        width: '64px',
        display: 'flex',
        flexDirection: 'column' as const,
        alignItems: 'center',
        justifyContent: 'center',
        gap: '4px',
        padding: '10px 6px',
        backgroundColor: 'var(--bg-primary)',
        borderLeft: '1px solid var(--border-subtle)',
        userSelect: 'none' as const,
        flex: '0 0 64px',
    },
    activeCard: {
        width: '52px',
        padding: '7px 0',
        borderRadius: '6px',
        fontSize: '10px',
        fontWeight: 600,
        color: 'var(--text-primary)',
        textAlign: 'center' as const,
        cursor: 'default',
        transition: 'box-shadow 0.25s ease',
        letterSpacing: '0.3px',
    },
    peek: {
        width: '40px',
        padding: '4px 0',
        borderRadius: '4px',
        fontSize: '8px',
        fontWeight: 500,
        textAlign: 'center' as const,
        whiteSpace: 'nowrap' as const,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        cursor: 'pointer',
        backgroundColor: 'transparent',
        transition: 'color 0.2s ease',
    },
    peekTop: {},
    peekBottom: {},
    addBtn: {
        marginTop: '4px',
        color: 'var(--text-disabled)',
        cursor: 'pointer',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        flexShrink: 0,
        transition: 'color 0.15s',
    },
};

export default GroupStrip;