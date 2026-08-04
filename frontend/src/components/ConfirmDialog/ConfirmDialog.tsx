import React, { useState, useCallback, useRef } from 'react';
import ReactDOM from 'react-dom';

interface ConfirmChoice {
    label: string;
    value: string;
    danger?: boolean;
    primary?: boolean;
}

interface ConfirmOptions {
    title?: string;
    message: string;
    confirmText?: string;
    cancelText?: string;
    danger?: boolean;
    /** 多按钮模式：提供后取代默认"确定/取消"按钮组，点击返回对应 value，取消返回 null */
    choices?: ConfirmChoice[];
}

interface ConfirmState extends ConfirmOptions {
    visible: boolean;
    resolve: ((value: boolean | string | null) => void) | null;
}

const INITIAL_STATE: ConfirmState = {
    visible: false,
    title: '',
    message: '',
    confirmText: '确定',
    cancelText: '取消',
    danger: false,
    choices: undefined,
    resolve: null,
};

let _setState: React.Dispatch<React.SetStateAction<ConfirmState>> | null = null;

export const confirmDialog = {
    show: (options: ConfirmOptions): Promise<boolean | string | null> => {
        return new Promise(resolve => {
            if (!_setState) {
                // Fallback 环境（无 React 宿主）：多按钮退化为原生 confirm
                if (options.choices && options.choices.length > 0) {
                    const confirmed = window.confirm(`${options.message}\n\n[${options.choices.map(c => c.label).join(' / ')}]`);
                    resolve(confirmed ? options.choices![0].value : null);
                } else {
                    resolve(window.confirm(options.message));
                }
                return;
            }
            _setState({
                visible: true,
                title: options.title || '确认操作',
                message: options.message,
                confirmText: options.confirmText || '确定',
                cancelText: options.cancelText || '取消',
                danger: options.danger ?? false,
                choices: options.choices,
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

    const handleChoice = useCallback((value: string) => {
        state.resolve?.(value);
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
                    {state.choices && state.choices.length > 0 ? (
                        <>
                            {state.choices.map(c => (
                                <button
                                    key={c.value}
                                    onClick={() => handleChoice(c.value)}
                                    style={c.danger ? styles.dangerBtn : c.primary ? styles.confirmBtn : styles.cancelBtn}
                                >
                                    {c.label}
                                </button>
                            ))}
                            <button onClick={handleCancel} style={styles.cancelBtn}>
                                {state.cancelText}
                            </button>
                        </>
                    ) : (
                        <>
                            <button onClick={handleCancel} style={styles.cancelBtn}>
                                {state.cancelText}
                            </button>
                            <button
                                onClick={handleConfirm}
                                style={state.danger ? styles.dangerBtn : styles.confirmBtn}
                            >
                                {state.confirmText}
                            </button>
                        </>
                    )}
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
        backgroundColor: 'var(--bg-secondary)',
        width: '420px',
        maxWidth: '90%',
        borderRadius: 8,
        boxShadow: '0 8px 32px rgba(0, 0, 0, 0.6)',
        border: '1px solid var(--border)',
        animation: 'slideIn 0.2s ease-out',
    },
    header: {
        padding: '20px 24px 12px',
    },
    title: {
        margin: 0,
        color: 'var(--text-primary)',
        fontSize: 16,
        fontWeight: 600,
    },
    body: {
        padding: '0 24px 20px',
    },
    message: {
        color: 'var(--text-secondary)',
        fontSize: 14,
        lineHeight: 1.6,
        margin: 0,
        whiteSpace: 'pre-line',
    },
    footer: {
        padding: '12px 24px 16px',
        borderTop: '1px solid var(--border)',
        display: 'flex',
        justifyContent: 'flex-end',
        gap: 10,
    },
    cancelBtn: {
        padding: '7px 18px',
        backgroundColor: 'var(--border)',
        color: 'var(--text-secondary)',
        border: '1px solid var(--border-strong)',
        borderRadius: 4,
        cursor: 'pointer',
        fontSize: 13,
        fontWeight: 500,
    },
    confirmBtn: {
        padding: '7px 18px',
        backgroundColor: 'var(--accent)',
        color: 'var(--text-on-accent)',
        border: 'none',
        borderRadius: 4,
        cursor: 'pointer',
        fontSize: 13,
        fontWeight: 600,
    },
    dangerBtn: {
        padding: '7px 18px',
        backgroundColor: 'var(--danger)',
        color: 'var(--text-on-accent)',
        border: 'none',
        borderRadius: 4,
        cursor: 'pointer',
        fontSize: 13,
        fontWeight: 600,
    },
};
