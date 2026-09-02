import React, { useEffect, useRef } from 'react';

export interface CompletionSuggestion {
    type: string;
    text: string;
    display_text: string;
    description: string;
}

export interface CompletionData {
    suggestions: CompletionSuggestion[];
    replace_from: number;
    replace_to: number;
}

interface CompletionOverlayProps {
    visible: boolean;
    position: { x: number; y: number };
    completions: CompletionData;
    selectedIndex: number;
    onSelect: (suggestion: CompletionSuggestion) => void;
    onNavigate: (direction: 'up' | 'down') => void;
    onClose: () => void;
}

const CompletionOverlay: React.FC<CompletionOverlayProps> = ({
    visible,
    position,
    completions,
    selectedIndex,
    onSelect,
    onNavigate,
    onClose,
}) => {
    const overlayRef = useRef<HTMLDivElement>(null);
    const itemRefs = useRef<(HTMLDivElement | null)[]>([]);

    // Keyboard & mouse handling
    useEffect(() => {
        if (!visible) return;

        const handleKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Escape') {
                e.preventDefault();
                onClose();
            }
        };

        const handleMouseDown = (e: MouseEvent) => {
            if (overlayRef.current && !overlayRef.current.contains(e.target as Node)) {
                onClose();
            }
        };

        window.addEventListener('keydown', handleKeyDown);
        window.addEventListener('mousedown', handleMouseDown);
        return () => {
            window.removeEventListener('keydown', handleKeyDown);
            window.removeEventListener('mousedown', handleMouseDown);
        };
    }, [visible, onClose]);

    // Scroll selected item into view
    useEffect(() => {
        if (selectedIndex >= 0 && itemRefs.current[selectedIndex]) {
            itemRefs.current[selectedIndex]?.scrollIntoView({
                block: 'nearest',
            });
        }
    }, [selectedIndex]);

    if (!visible || completions.suggestions.length === 0) {
        return null;
    }

    // Calculate position to stay within viewport
    const maxY = window.innerHeight - 300; // Leave 300px from bottom
    const adjustedY = Math.min(position.y, maxY);
    const maxX = window.innerWidth - 320; // Leave 320px from right
    const adjustedX = Math.min(position.x, maxX);

    return (
        <div
            ref={overlayRef}
            style={{
                ...styles.container,
                left: adjustedX,
                top: adjustedY,
            }}
        >
            <div style={styles.header}>
                <span>{completions.suggestions.length} 个建议</span>
                <span style={styles.headerHint}>
                    <span style={styles.kbd}>Shift</span>+<span style={styles.kbd}>Tab</span> 接受
                    <span style={styles.kbd}>↑</span><span style={styles.kbd}>↓</span> 选择
                    <span style={styles.kbd}>Esc</span> 关闭
                </span>
            </div>
            <div style={styles.list}>
                {completions.suggestions.map((suggestion, index) => (
                    <div
                        key={index}
                        ref={(el) => (itemRefs.current[index] = el)}
                        style={{
                            ...styles.item,
                            ...(index === selectedIndex ? styles.selectedItem : {}),
                        }}
                        onClick={() => onSelect(suggestion)}
                    >
                        <div style={styles.itemMain}>
                            <span style={styles.itemText}>{suggestion.display_text}</span>
                            <span style={styles.itemType}>{getTypeLabel(suggestion.type)}</span>
                        </div>
                        {suggestion.description && (
                            <div style={styles.itemDescription}>{suggestion.description}</div>
                        )}
                    </div>
                ))}
            </div>
        </div>
    );
};

const getTypeLabel = (type: string): string => {
    switch (type) {
        case 'command':
            return '命令';
        case 'option':
            return '选项';
        case 'argument':
            return '参数';
        default:
            return '';
    }
};

const styles = {
    container: {
        position: 'fixed' as const,
        backgroundColor: 'var(--bg-secondary)',
        border: '1px solid var(--border)',
        borderRadius: '4px',
        boxShadow: '0 4px 12px rgba(0, 0, 0, 0.5)',
        zIndex: 3000,
        minWidth: '300px',
        maxWidth: '500px',
        maxHeight: '300px',
        display: 'flex',
        flexDirection: 'column' as const,
    },
    header: {
        padding: '6px 12px',
        backgroundColor: 'var(--bg-primary)',
        borderBottom: '1px solid var(--border)',
        color: 'var(--text-muted)',
        fontSize: '11px',
        borderTopLeftRadius: '4px',
        borderTopRightRadius: '4px',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
    },
    headerHint: {
        fontSize: '10px',
        color: 'var(--text-disabled)',
        display: 'flex',
        alignItems: 'center',
        gap: '2px',
    },
    kbd: {
        display: 'inline-block',
        padding: '1px 4px',
        fontSize: '10px',
        fontFamily: 'inherit',
        color: 'var(--text-tertiary)',
        backgroundColor: 'var(--bg-input)',
        border: '1px solid var(--border-strong)',
        borderRadius: '2px',
        lineHeight: '1.2',
        marginLeft: '2px',
        marginRight: '2px',
    },
    list: {
        overflowY: 'auto' as const,
        maxHeight: '250px',
    },
    item: {
        padding: '8px 12px',
        cursor: 'pointer',
        borderBottom: '1px solid var(--border-subtle)',
        transition: 'background-color 0.1s',
    },
    selectedItem: {
        backgroundColor: 'var(--bg-active)',
        borderBottom: '1px solid var(--accent)',
    },
    itemMain: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        marginBottom: '2px',
    },
    itemText: {
        color: 'var(--text-primary)',
        fontSize: '13px',
        fontFamily: 'var(--font-mono)',
        fontWeight: 500,
        flex: 1,
    },
    itemType: {
        fontSize: '10px',
        color: 'var(--text-muted)',
        backgroundColor: 'var(--bg-input)',
        padding: '2px 6px',
        borderRadius: '2px',
        marginLeft: '8px',
    },
    itemDescription: {
        color: 'var(--text-tertiary)',
        fontSize: '11px',
        marginTop: '2px',
    },
};

export default CompletionOverlay;
