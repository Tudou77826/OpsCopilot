import React, { createContext, useContext, useState, useCallback, useRef } from 'react';
import ReactDOM from 'react-dom';

export type ToastType = 'success' | 'error' | 'info' | 'warning';

interface ToastItem {
    id: number;
    type: ToastType;
    message: string;
    duration: number;
}

export interface ToastContextValue {
    show: (type: ToastType, message: string, duration?: number) => void;
    success: (message: string) => void;
    error: (message: string) => void;
    info: (message: string) => void;
    warning: (message: string) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

const TYPE_CONFIG: Record<ToastType, { color: string; icon: string; defaultDuration: number }> = {
    success: { color: 'var(--success)', icon: '✓', defaultDuration: 3000 },
    error: { color: 'var(--danger)', icon: '✗', defaultDuration: 5000 },
    info: { color: 'var(--accent)', icon: 'i', defaultDuration: 3000 },
    warning: { color: 'var(--warning)', icon: '!', defaultDuration: 4000 },
};

const ToastItemComponent: React.FC<{ item: ToastItem; onClose: (id: number) => void }> = ({ item, onClose }) => {
    const config = TYPE_CONFIG[item.type];
    const [exiting, setExiting] = React.useState(false);

    const handleClose = () => {
        setExiting(true);
        setTimeout(() => onClose(item.id), 200);
    };

    React.useEffect(() => {
        const timer = setTimeout(handleClose, item.duration);
        return () => clearTimeout(timer);
    }, [item.duration]);

    return (
        <div style={{
            ...styles.toast,
            borderLeft: `4px solid ${config.color}`,
            animation: exiting ? 'toastFadeOut 0.2s ease-in forwards' : 'toastSlideIn 0.25s ease-out',
        }}>
            <div style={{ ...styles.iconBadge, backgroundColor: config.color }}>
                <span style={styles.iconText}>{config.icon}</span>
            </div>
            <span style={styles.message}>{item.message}</span>
            <button onClick={handleClose} style={styles.closeBtn}>&times;</button>
        </div>
    );
};

const ToastContainer: React.FC<{ toasts: ToastItem[]; onClose: (id: number) => void }> = ({ toasts, onClose }) => {
    if (toasts.length === 0) return null;

    return ReactDOM.createPortal(
        <div style={styles.container}>
            {toasts.map(item => (
                <ToastItemComponent key={item.id} item={item} onClose={onClose} />
            ))}
        </div>,
        document.body
    );
};

export const ToastProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const [toasts, setToasts] = useState<ToastItem[]>([]);
    const idRef = useRef(0);

    const show = useCallback((type: ToastType, message: string, duration?: number) => {
        const id = ++idRef.current;
        const config = TYPE_CONFIG[type];
        setToasts(prev => [{ id, type, message, duration: duration ?? config.defaultDuration }, ...prev]);
    }, []);

    const remove = useCallback((id: number) => {
        setToasts(prev => prev.filter(t => t.id !== id));
    }, []);

    const value: ToastContextValue = {
        show,
        success: (msg: string) => show('success', msg),
        error: (msg: string) => show('error', msg),
        info: (msg: string) => show('info', msg),
        warning: (msg: string) => show('warning', msg),
    };

    return (
        <ToastContext.Provider value={value}>
            {children}
            <ToastContainer toasts={toasts} onClose={remove} />
        </ToastContext.Provider>
    );
};

export const useToast = (): ToastContextValue => {
    const ctx = useContext(ToastContext);
    if (!ctx) throw new Error('useToast must be used within ToastProvider');
    return ctx;
};

const styles: Record<string, React.CSSProperties> = {
    container: {
        position: 'fixed',
        top: 16,
        right: 16,
        zIndex: 9999999,
        display: 'flex',
        flexDirection: 'column',
        gap: 8,
        pointerEvents: 'none',
    },
    toast: {
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        padding: '10px 14px',
        backgroundColor: 'var(--bg-tooltip)',
        borderRadius: 6,
        boxShadow: '0 4px 16px rgba(0, 0, 0, 0.4)',
        color: 'var(--text-on-accent)',
        fontSize: 13,
        lineHeight: 1.4,
        maxWidth: 380,
        minWidth: 200,
        pointerEvents: 'auto',
        backdropFilter: 'blur(8px)',
    },
    iconBadge: {
        width: 20,
        height: 20,
        borderRadius: '50%',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        flexShrink: 0,
    },
    iconText: {
        color: 'var(--text-on-accent)',
        fontSize: 11,
        fontWeight: 700,
    },
    message: {
        flex: 1,
    },
    closeBtn: {
        background: 'none',
        border: 'none',
        color: 'var(--text-muted)',
        cursor: 'pointer',
        fontSize: 16,
        padding: '0 2px',
        lineHeight: 1,
        flexShrink: 0,
    },
};
