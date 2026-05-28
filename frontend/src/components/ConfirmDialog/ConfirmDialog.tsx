import React, { useState, useCallback, useRef } from 'react';
import ReactDOM from 'react-dom';

interface ConfirmOptions {
    title?: string;
    message: string;
    confirmText?: string;
    cancelText?: string;
    danger?: boolean;
}

interface ConfirmState extends ConfirmOptions {
    visible: boolean;
    resolve: ((value: boolean) => void) | null;
}

const INITIAL_STATE: ConfirmState = {
    visible: false,
    title: '',
    message: '',
    confirmText: '确定',
    cancelText: '取消',
    danger: false,
    resolve: null,
};

let _setState: React.Dispatch<React.SetStateAction<ConfirmState>> | null = null;

export const confirmDialog = {
    show: (options: ConfirmOptions): Promise<boolean> => {
        return new Promise(resolve => {
            if (!_setState) {
                resolve(window.confirm(options.message));
                return;
            }
            _setState({
                visible: true,
                title: options.title || '确认操作',
                message: options.message,
                confirmText: options.confirmText || '确定',
                cancelText: options.cancelText || '取消',
                danger: options.danger ?? false,
                resolve,
            });
        });
    },
};

const ConfirmDialogInternal: React.FC = () => {
    const [state, setState] = useState<ConfirmState>(INITIAL_STATE);
    const stateRef = useRef(state);
    stateRef.current = state;

    React.useEffect(() => {
        _setState = setState;
    }, []);

    const handleConfirm = useCallback(() => {
        state.resolve?.(true);
        setState(prev => ({ ...prev, visible: false, resolve: null }));
    }, [state.resolve]);

    const handleCancel = useCallback(() => {
        state.resolve?.(false);
        setState(prev => ({ ...prev, visible: false, resolve: null }));
    }, [state.resolve]);

    if (!state.visible) return null;

    return ReactDOM.createPortal(
        <div style={styles.overlay} onClick={handleCancel}>
            <div style={styles.modal} onClick={e => e.stopPropagation()}>
                <div style={styles.header}>
                    <h3 style={styles.title}>{state.title}</h3>
                </div>
                <div style={styles.body}>
                    <p style={styles.message}>{state.message}</p>
                </div>
                <div style={styles.footer}>
                    <button onClick={handleCancel} style={styles.cancelBtn}>
                        {state.cancelText}
                    </button>
                    <button
                        onClick={handleConfirm}
                        style={state.danger ? styles.dangerBtn : styles.confirmBtn}
                    >
                        {state.confirmText}
                    </button>
                </div>
            </div>
        </div>,
        document.body
    );
};

export default ConfirmDialogInternal;

const styles: Record<string, React.CSSProperties> = {
    overlay: {
        position: 'fixed',
        top: 0, left: 0, right: 0, bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.6)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 9999998,
        backdropFilter: 'blur(4px)',
        animation: 'fadeIn 0.15s ease-out',
    },
    modal: {
        backgroundColor: '#252526',
        width: '420px',
        maxWidth: '90%',
        borderRadius: 8,
        boxShadow: '0 8px 32px rgba(0, 0, 0, 0.6)',
        border: '1px solid #444',
        animation: 'slideIn 0.2s ease-out',
    },
    header: {
        padding: '20px 24px 12px',
    },
    title: {
        margin: 0,
        color: '#fff',
        fontSize: 16,
        fontWeight: 600,
    },
    body: {
        padding: '0 24px 20px',
    },
    message: {
        color: '#ccc',
        fontSize: 14,
        lineHeight: 1.6,
        margin: 0,
        whiteSpace: 'pre-line',
    },
    footer: {
        padding: '12px 24px 16px',
        borderTop: '1px solid #333',
        display: 'flex',
        justifyContent: 'flex-end',
        gap: 10,
    },
    cancelBtn: {
        padding: '7px 18px',
        backgroundColor: '#333',
        color: '#ccc',
        border: '1px solid #555',
        borderRadius: 4,
        cursor: 'pointer',
        fontSize: 13,
        fontWeight: 500,
    },
    confirmBtn: {
        padding: '7px 18px',
        backgroundColor: '#0e639c',
        color: '#fff',
        border: 'none',
        borderRadius: 4,
        cursor: 'pointer',
        fontSize: 13,
        fontWeight: 600,
    },
    dangerBtn: {
        padding: '7px 18px',
        backgroundColor: '#d32f2f',
        color: '#fff',
        border: 'none',
        borderRadius: 4,
        cursor: 'pointer',
        fontSize: 13,
        fontWeight: 600,
    },
};
