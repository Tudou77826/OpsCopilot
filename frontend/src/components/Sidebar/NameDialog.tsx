import React, { useEffect, useState } from 'react';
import ReactDOM from 'react-dom';

interface NameDialogProps {
    title: string;
    defaultValue?: string;
    placeholder?: string;
    onConfirm: (name: string) => void;
    onCancel: () => void;
}

const INVALID_CHARS = /[\\/:*?"<>|]/;

// 应用内新建/重命名对话框：替代原生 prompt()，带空名与非法字符校验。
const NameDialog: React.FC<NameDialogProps> = ({ title, defaultValue = '', placeholder, onConfirm, onCancel }) => {
    const [value, setValue] = useState(defaultValue);
    const invalid = value.trim() === '' || INVALID_CHARS.test(value);

    useEffect(() => {
        const onKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Enter' && !invalid) onConfirm(value.trim());
            if (e.key === 'Escape') onCancel();
        };
        window.addEventListener('keydown', onKeyDown);
        return () => window.removeEventListener('keydown', onKeyDown);
    }, [value, invalid, onConfirm, onCancel]);

    return ReactDOM.createPortal(
        <div style={styles.overlay} onClick={onCancel}>
            <div style={styles.modal} onClick={e => e.stopPropagation()}>
                <div style={styles.header}>
                    <h3 style={styles.title}>{title}</h3>
                </div>
                <div style={styles.body}>
                    <input
                        autoFocus
                        style={styles.input}
                        value={value}
                        placeholder={placeholder}
                        onChange={(e) => setValue(e.target.value)}
                        onFocus={(e) => e.target.select()}
                        data-testid="name-dialog-input"
                    />
                    {invalid ? <div style={styles.hint}>名称不能为空，且不能包含 \ / : * ? " &lt; &gt; | 字符</div> : null}
                </div>
                <div style={styles.footer}>
                    <button onClick={onCancel} style={styles.cancelBtn}>取消</button>
                    <button onClick={() => onConfirm(value.trim())} disabled={invalid} style={invalid ? styles.confirmDisabled : styles.confirmBtn}>
                        确定
                    </button>
                </div>
            </div>
        </div>,
        document.body
    );
};

const styles: Record<string, React.CSSProperties> = {
    overlay: {
        position: 'fixed',
        top: 0, left: 0, right: 0, bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.6)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 9999997,
        backdropFilter: 'blur(4px)',
    },
    modal: {
        backgroundColor: 'var(--bg-secondary)',
        width: '420px',
        maxWidth: '90%',
        borderRadius: 8,
        boxShadow: '0 8px 32px rgba(0, 0, 0, 0.6)',
        border: '1px solid var(--border)',
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
        padding: '0 24px',
    },
    input: {
        width: '100%',
        boxSizing: 'border-box',
        padding: '8px 10px',
        borderRadius: 4,
        border: '1px solid var(--border)',
        backgroundColor: 'var(--bg-primary)',
        color: 'var(--text-primary)',
        outline: 'none',
        fontSize: 14,
    },
    hint: {
        color: 'var(--severity-danger)',
        fontSize: 12,
        marginTop: 8,
    },
    footer: {
        padding: '12px 24px 16px',
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
    confirmDisabled: {
        padding: '7px 18px',
        backgroundColor: 'var(--border)',
        color: 'var(--text-muted)',
        border: 'none',
        borderRadius: 4,
        cursor: 'not-allowed',
        fontSize: 13,
    },
};

export default NameDialog;
