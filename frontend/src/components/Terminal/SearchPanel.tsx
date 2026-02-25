import React, { useEffect, forwardRef, useState, useCallback, useRef } from 'react';

interface SearchPanelProps {
    visible: boolean;
    query: string;
    onQueryChange: (v: string) => void;
    onClose: () => void;
    onNext: () => void;
    onPrev: () => void;
    caseSensitive: boolean;
    onCaseSensitiveChange: (v: boolean) => void;
    regexMode: boolean;
    onRegexModeChange: (v: boolean) => void;
    matchText?: string;
}

const SearchPanel = forwardRef<HTMLInputElement, SearchPanelProps>(function SearchPanel({
    visible,
    query,
    onQueryChange,
    onClose,
    onNext,
    onPrev,
    caseSensitive,
    onCaseSensitiveChange,
    regexMode,
    onRegexModeChange,
    matchText
}: SearchPanelProps, inputRef) {

    // 拖动相关状态
    const [position, setPosition] = useState({ left: 12, bottom: 12 });
    const [isDragging, setIsDragging] = useState(false);
    const dragStartRef = useRef({ x: 0, y: 0, left: 0, bottom: 0 });

    useEffect(() => {
        if (!visible) return;
        const id = window.setTimeout(() => {
            if (inputRef && typeof inputRef !== 'function' && inputRef.current) {
                inputRef.current.focus();
            }
        }, 0);
        return () => window.clearTimeout(id);
    }, [visible]);

    // 拖动处理
    const handleMouseDown = useCallback((e: React.MouseEvent) => {
        if ((e.target as HTMLElement).closest('input, button, label')) return;
        e.preventDefault();
        setIsDragging(true);
        dragStartRef.current = {
            x: e.clientX,
            y: e.clientY,
            left: position.left,
            bottom: position.bottom
        };
    }, [position]);

    useEffect(() => {
        if (!isDragging) return;

        const handleMouseMove = (e: MouseEvent) => {
            const deltaX = e.clientX - dragStartRef.current.x;
            const deltaY = e.clientY - dragStartRef.current.y;

            setPosition({
                left: dragStartRef.current.left + deltaX,
                bottom: dragStartRef.current.bottom - deltaY
            });
        };

        const handleMouseUp = () => {
            setIsDragging(false);
        };

        window.addEventListener('mousemove', handleMouseMove);
        window.addEventListener('mouseup', handleMouseUp);

        return () => {
            window.removeEventListener('mousemove', handleMouseMove);
            window.removeEventListener('mouseup', handleMouseUp);
        };
    }, [isDragging]);

    if (!visible) return null;

    return (
        <div style={{ ...styles.wrap, left: position.left, bottom: position.bottom, cursor: isDragging ? 'grabbing' : 'default' }} onMouseDown={handleMouseDown}>
            <div style={styles.dragHandle}>
                <div style={styles.dragIndicator} />
            </div>
            <div style={styles.row}>
                <div style={styles.icon}>🔍</div>
                <input
                    ref={inputRef}
                    value={query}
                    onChange={(e) => onQueryChange(e.target.value)}
                    onKeyDown={(e) => {
                        if (e.ctrlKey && e.code === 'KeyF') {
                            e.preventDefault();
                            e.stopPropagation();
                            return;
                        }
                        if (e.key === 'Escape') {
                            e.preventDefault();
                            e.stopPropagation();
                            onClose();
                            return;
                        }
                        if (e.key === 'Enter') {
                            e.preventDefault();
                            e.stopPropagation();
                            if (e.shiftKey) onPrev();
                            else onNext();
                            return;
                        }
                    }}
                    style={styles.input}
                    placeholder="搜索…"
                />
                <div style={styles.counter}>{matchText || ''}</div>
                <button style={styles.btn} onClick={onPrev} title="上一个 (Shift+Enter)">◀</button>
                <button style={styles.btn} onClick={onNext} title="下一个 (Enter)">▶</button>
                <button style={styles.btnClose} onClick={onClose} title="关闭 (Esc)">×</button>
            </div>
            <div style={styles.row2}>
                <label style={styles.opt}>
                    <input type="checkbox" checked={caseSensitive} onChange={(e) => onCaseSensitiveChange(e.target.checked)} />
                    <span style={styles.optText}>Aa</span>
                </label>
                <label style={styles.opt}>
                    <input type="checkbox" checked={regexMode} onChange={(e) => onRegexModeChange(e.target.checked)} />
                    <span style={styles.optText}>.*</span>
                </label>
            </div>
        </div>
    );
});

export default SearchPanel;

const styles: Record<string, React.CSSProperties> = {
    wrap: {
        position: 'absolute',
        left: 12,
        bottom: 12,
        zIndex: 20,
        backgroundColor: '#141414',
        border: '1px solid #2a2a2a',
        borderRadius: '10px',
        padding: '8px 10px',
        color: '#ddd',
        minWidth: '420px',
        boxShadow: '0 6px 20px rgba(0,0,0,0.35)',
        display: 'flex',
        flexDirection: 'column',
        gap: '6px',
        userSelect: 'none'
    },
    dragHandle: {
        display: 'flex',
        justifyContent: 'center',
        padding: '2px 0 4px',
        cursor: 'grab'
    },
    dragIndicator: {
        width: '36px',
        height: '4px',
        backgroundColor: '#3a3a3a',
        borderRadius: '2px'
    },
    row: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px'
    },
    row2: {
        display: 'flex',
        alignItems: 'center',
        gap: '10px'
    },
    icon: {
        width: '18px',
        textAlign: 'center'
    },
    input: {
        flex: 1,
        backgroundColor: '#202020',
        color: '#ddd',
        border: '1px solid #2a2a2a',
        borderRadius: '8px',
        padding: '8px 12px',
        outline: 'none',
        fontSize: '13px'
    },
    counter: {
        fontSize: '11px',
        color: '#8a8a8a',
        minWidth: '56px',
        textAlign: 'right'
    },
    btn: {
        backgroundColor: '#202020',
        color: '#ddd',
        border: '1px solid #2a2a2a',
        borderRadius: '8px',
        padding: '4px 8px',
        cursor: 'pointer',
        fontSize: '12px'
    },
    btnClose: {
        backgroundColor: '#202020',
        color: '#ddd',
        border: '1px solid #2a2a2a',
        borderRadius: '8px',
        padding: '4px 10px',
        cursor: 'pointer',
        fontSize: '14px',
        lineHeight: '14px'
    },
    opt: {
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
        color: '#bbb',
        fontSize: '12px'
    },
    optText: {
        color: '#bbb',
        fontSize: '12px'
    }
};
