import React, { useEffect, useMemo, useRef, useState } from 'react';
import FilesPanel from '../Sidebar/FilesPanel';

interface TerminalSessionLite {
    id: string;
    title: string;
}

interface FileTransferWindowProps {
    isOpen: boolean;
    onClose: () => void;
    activeTerminalId: string | null;
    terminals: TerminalSessionLite[];
}

const FileTransferWindow: React.FC<FileTransferWindowProps> = ({ isOpen, onClose, activeTerminalId, terminals }) => {
    const [minimized, setMinimized] = useState(false);
    const [pos, setPos] = useState<{ x: number; y: number } | null>(null);
    const draggingRef = useRef<{ startX: number; startY: number; originX: number; originY: number } | null>(null);
    const winRef = useRef<HTMLDivElement | null>(null);

    const initialPos = useMemo(() => {
        const pad = 24;
        const w = 980;
        const h = 680;
        const x = Math.max(pad, Math.floor((window.innerWidth - w) / 2));
        const y = Math.max(pad, Math.floor((window.innerHeight - h) / 2));
        return { x, y, w, h };
    }, []);

    useEffect(() => {
        if (isOpen) {
            setMinimized(false);
            setPos({ x: initialPos.x, y: initialPos.y });
        }
    }, [isOpen, initialPos.x, initialPos.y]);

    useEffect(() => {
        const onMove = (e: MouseEvent) => {
            const d = draggingRef.current;
            if (!d) return;
            const dx = e.clientX - d.startX;
            const dy = e.clientY - d.startY;
            const nextX = d.originX + dx;
            const nextY = d.originY + dy;
            const el = winRef.current;
            const rect = el ? el.getBoundingClientRect() : null;
            const w = rect?.width ?? initialPos.w;
            const h = rect?.height ?? initialPos.h;
            const pad = 10;
            const maxX = Math.max(pad, window.innerWidth - w - pad);
            const maxY = Math.max(pad, window.innerHeight - h - pad);
            setPos({
                x: Math.min(Math.max(pad, nextX), maxX),
                y: Math.min(Math.max(pad, nextY), maxY)
            });
        };
        const onUp = () => {
            draggingRef.current = null;
        };
        window.addEventListener('mousemove', onMove);
        window.addEventListener('mouseup', onUp);
        return () => {
            window.removeEventListener('mousemove', onMove);
            window.removeEventListener('mouseup', onUp);
        };
    }, [initialPos.h, initialPos.w]);

    if (!isOpen) return null;

    if (minimized) {
        return (
            <div style={styles.minBar}>
                <div style={styles.title}>文件传输</div>
                <div style={{ flex: 1 }} />
                <button style={styles.minBtn} onClick={() => setMinimized(false)} aria-label="还原">
                    还原
                </button>
                <button style={styles.closeBtn} onClick={onClose} aria-label="关闭">
                    ×
                </button>
            </div>
        );
    }

    return (
        <div
            ref={winRef}
            style={{
                ...styles.window,
                left: pos ? pos.x : initialPos.x,
                top: pos ? pos.y : initialPos.y,
                width: initialPos.w,
                height: initialPos.h
            }}
        >
            <div
                style={styles.header}
                onMouseDown={(e) => {
                    const target = e.target as HTMLElement;
                    if (target.closest('button')) return;
                    const rect = winRef.current?.getBoundingClientRect();
                    if (!rect) return;
                    draggingRef.current = {
                        startX: e.clientX,
                        startY: e.clientY,
                        originX: rect.left,
                        originY: rect.top
                    };
                }}
            >
                <div style={styles.title}>文件传输</div>
                <div style={{ flex: 1 }} />
                <button style={styles.minBtn} onClick={() => setMinimized(true)} aria-label="最小化">
                    最小化
                </button>
                <button style={styles.closeBtn} onClick={onClose} aria-label="关闭">
                    ×
                </button>
            </div>
            <div style={styles.body}>
                <FilesPanel activeTerminalId={activeTerminalId} terminals={terminals} />
            </div>
        </div>
    );
};

const styles: Record<string, React.CSSProperties> = {
    window: {
        position: 'fixed',
        backgroundColor: '#1e1e1e',
        border: '1px solid #333',
        borderRadius: '10px',
        overflow: 'hidden' as const,
        display: 'flex',
        flexDirection: 'column',
        zIndex: 4000,
        boxShadow: '0 12px 40px rgba(0,0,0,0.55)',
        resize: 'both',
        minWidth: '720px',
        minHeight: '520px'
    },
    header: {
        height: '44px',
        display: 'flex',
        alignItems: 'center',
        padding: '0 12px',
        backgroundColor: '#252526',
        borderBottom: '1px solid #333',
        gap: '10px',
        cursor: 'move',
        userSelect: 'none'
    },
    title: {
        fontSize: '13px',
        color: '#fff',
        fontWeight: 600
    },
    minBar: {
        position: 'fixed',
        right: '16px',
        bottom: '16px',
        zIndex: 4000,
        height: '40px',
        width: '260px',
        backgroundColor: '#252526',
        border: '1px solid #333',
        borderRadius: '10px',
        display: 'flex',
        alignItems: 'center',
        padding: '0 10px',
        gap: '10px',
        boxShadow: '0 10px 30px rgba(0,0,0,0.55)'
    },
    closeBtn: {
        width: '32px',
        height: '28px',
        borderRadius: '6px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#1e1e1e',
        color: '#ddd',
        cursor: 'pointer',
        fontSize: '18px',
        lineHeight: '18px'
    },
    minBtn: {
        height: '28px',
        borderRadius: '6px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#1e1e1e',
        color: '#ddd',
        cursor: 'pointer',
        fontSize: '12px',
        padding: '0 10px'
    },
    body: {
        flex: 1,
        minHeight: 0,
        overflow: 'auto'
    }
};

export default FileTransferWindow;
