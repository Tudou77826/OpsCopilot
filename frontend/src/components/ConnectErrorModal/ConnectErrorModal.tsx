import React, { useEffect, useRef, useState } from 'react';
import ReactDOM from 'react-dom';

type Props = {
    isOpen: boolean;
    title: string;
    message: string;
    onClose: () => void;
};

const ConnectErrorModal: React.FC<Props> = ({ isOpen, title, message, onClose }) => {
    const closeBtnRef = useRef<HTMLButtonElement | null>(null);
    const [copied, setCopied] = useState(false);

    useEffect(() => {
        if (!isOpen) return;
        setCopied(false);
        setTimeout(() => closeBtnRef.current?.focus(), 0);
    }, [isOpen]);

    if (!isOpen) return null;

    const onKeyDown = (e: React.KeyboardEvent) => {
        if (e.key === 'Escape') onClose();
    };

    const copy = async () => {
        try {
            await navigator.clipboard.writeText(message);
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
        } catch {
            setCopied(false);
        }
    };

    return ReactDOM.createPortal(
        <div
            style={styles.overlay}
            onClick={(e) => {
                if (e.target === e.currentTarget) onClose();
            }}
            onKeyDown={onKeyDown}
            role="dialog"
            aria-modal="true"
            aria-label="连接失败"
            tabIndex={-1}
        >
            <div style={styles.modal}>
                <div style={styles.header}>
                    <div style={styles.icon}>⚠️</div>
                    <div style={styles.headerText}>
                        <div style={styles.title}>{title}</div>
                        <div style={styles.subtitle}>连接未建立，请检查网络、凭据或跳板机配置</div>
                    </div>
                </div>

                <div style={styles.body}>
                    <pre style={styles.message}>{message || '未知错误'}</pre>
                </div>

                <div style={styles.footer}>
                    <button onClick={copy} style={styles.secondaryButton}>
                        {copied ? '已复制' : '复制错误'}
                    </button>
                    <button ref={closeBtnRef} onClick={onClose} style={styles.primaryButton}>
                        关闭
                    </button>
                </div>
            </div>
        </div>,
        document.body
    );
};

const styles = {
    overlay: {
        position: 'fixed' as const,
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.85)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 999998,
        backdropFilter: 'blur(4px)',
    },
    modal: {
        backgroundColor: '#252526',
        width: '720px',
        maxWidth: '92vw',
        borderRadius: '8px',
        boxShadow: '0 8px 32px rgba(0, 0, 0, 0.6)',
        border: '1px solid #444',
        display: 'flex',
        flexDirection: 'column' as const,
        maxHeight: '86vh',
        overflow: 'hidden',
    },
    header: {
        padding: '18px 20px',
        display: 'flex',
        gap: '12px',
        borderBottom: '1px solid #333',
        alignItems: 'flex-start',
    },
    icon: {
        fontSize: '28px',
        lineHeight: '28px',
        marginTop: '2px',
    },
    headerText: {
        flex: 1,
        minWidth: 0,
    },
    title: {
        color: '#fff',
        fontSize: '16px',
        fontWeight: 600,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap' as const,
    },
    subtitle: {
        marginTop: '4px',
        color: '#aaa',
        fontSize: '12px',
    },
    body: {
        padding: '12px 20px 0 20px',
        overflow: 'auto' as const,
        flex: 1,
    },
    message: {
        margin: 0,
        padding: '12px',
        backgroundColor: '#1e1e1e',
        border: '1px solid #333',
        borderRadius: '6px',
        color: '#ddd',
        fontSize: '12px',
        lineHeight: '1.6',
        whiteSpace: 'pre-wrap' as const,
        wordBreak: 'break-word' as const,
    },
    footer: {
        padding: '14px 20px',
        borderTop: '1px solid #333',
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '10px',
    },
    secondaryButton: {
        padding: '8px 14px',
        backgroundColor: '#333',
        color: '#ccc',
        border: '1px solid #555',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '13px',
        fontWeight: 500,
    },
    primaryButton: {
        padding: '8px 14px',
        backgroundColor: '#d32f2f',
        color: '#fff',
        border: 'none',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '13px',
        fontWeight: 600,
    },
};

export default ConnectErrorModal;
