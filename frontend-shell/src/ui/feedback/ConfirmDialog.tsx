import React, { useState, useCallback, useRef } from 'react';
import ReactDOM from 'react-dom';
import { usePortalRoot } from '../Surface';

export interface ConfirmChoice {
    label: string;
    value: string;
    danger?: boolean;
    primary?: boolean;
}

/** 复选框配置：确认类弹窗可附带一个勾选项（如"全部覆盖"）。 */
export interface ConfirmCheckbox {
    label: string;
}

/** 带复选框弹窗的返回：用户选择的动作 + 复选框最终勾选状态。 */
export interface ConfirmResult {
    value: boolean | string | null;
    checked: boolean;
}

export interface ConfirmOptions {
    title?: string;
    message: string;
    confirmText?: string;
    cancelText?: string;
    danger?: boolean;
    /** 多按钮模式：提供后取代默认"确定/取消"按钮组，点击返回对应 value，取消返回 null */
    choices?: ConfirmChoice[];
    /** 附带复选框（配合 showWithCheckbox 使用；show 忽略该字段） */
    checkbox?: ConfirmCheckbox;
}

interface ConfirmState extends ConfirmOptions {
    visible: boolean;
    checked: boolean;
    resolve: ((value: boolean | string | null, checked: boolean) => void) | null;
}

const INITIAL_STATE: ConfirmState = {
    visible: false,
    title: '',
    message: '',
    confirmText: '确定',
    cancelText: '取消',
    danger: false,
    choices: undefined,
    checkbox: undefined,
    checked: false,
    resolve: null,
};

let _setState: React.Dispatch<React.SetStateAction<ConfirmState>> | null = null;

function open(options: ConfirmOptions): void {
    if (!_setState) {
        throw new Error('confirmDialog: 无 React 宿主');
    }
    _setState({
        visible: true,
        title: options.title || '确认操作',
        message: options.message,
        confirmText: options.confirmText || '确定',
        cancelText: options.cancelText || '取消',
        danger: options.danger ?? false,
        choices: options.choices,
        checkbox: options.checkbox,
        checked: false,
        resolve: null,
    });
}

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
            open(options);
            _setState(prev => ({ ...prev, resolve: (value) => resolve(value) }));
        });
    },
    /** 带复选框版本：返回用户选择的动作与复选框勾选状态。 */
    showWithCheckbox: (options: ConfirmOptions & { checkbox: ConfirmCheckbox }): Promise<ConfirmResult> => {
        return new Promise(resolve => {
            if (!_setState) {
                const confirmed = window.confirm(`${options.message}\n\n[${options.checkbox.label}]`);
                resolve({ value: confirmed ? true : null, checked: false });
                return;
            }
            open(options);
            _setState(prev => ({ ...prev, resolve: (value, checked) => resolve({ value, checked }) }));
        });
    },
};

export const ConfirmDialogInternal: React.FC = () => {
    const portalRoot = usePortalRoot();
    const [state, setState] = useState<ConfirmState>(INITIAL_STATE);
    const stateRef = useRef(state);
    stateRef.current = state;

    React.useEffect(() => {
        _setState = setState;
        return () => {
            if (_setState === setState) _setState = null;
            stateRef.current.resolve?.(false, stateRef.current.checked);
        };
    }, []);

    const close = (value: boolean | string | null) => {
        state.resolve?.(value, state.checked);
        setState(prev => ({ ...prev, visible: false, resolve: null }));
    };

    const handleConfirm = useCallback(() => close(true), [state.resolve, state.checked]);
    const handleCancel = useCallback(() => close(false), [state.resolve, state.checked]);
    const handleChoice = useCallback((value: string) => close(value), [state.resolve, state.checked]);
    const toggleChecked = useCallback((checked: boolean) => {
        setState(prev => ({ ...prev, checked }));
    }, []);

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
                {state.checkbox ? (
                    <div style={styles.checkboxRow}>
                        <label style={styles.checkboxLabel}>
                            <input
                                type="checkbox"
                                checked={state.checked}
                                onChange={e => toggleChecked(e.target.checked)}
                            />
                            <span>{state.checkbox.label}</span>
                        </label>
                    </div>
                ) : null}
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
        portalRoot
    );
};

export default ConfirmDialogInternal;

const styles: Record<string, React.CSSProperties> = {
    overlay: {
        position: 'fixed',
        top: 0, left: 0, right: 0, bottom: 0,
        backgroundColor: 'var(--overlay)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 9999998,
        backdropFilter: 'blur(4px)',
        animation: 'fadeIn 0.15s ease-out',
    },
    modal: {
        backgroundColor: 'var(--bg-dialog)',
        width: '420px',
        maxWidth: '90%',
        borderRadius: 8,
        boxShadow: 'var(--shadow-dialog)',
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
    checkboxRow: {
        padding: '0 24px 12px',
    },
    checkboxLabel: {
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        color: 'var(--text-secondary)',
        fontSize: 13,
        cursor: 'pointer',
        userSelect: 'none',
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
