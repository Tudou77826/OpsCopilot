import React, { useEffect, useRef, useState } from 'react';
import { ConnectionConfig } from '../types';
import ConnectionConfigForm from '../connection/ConnectionConfigForm';
import { SessionManagerRuntime } from '../ports';
import { useToast } from '../feedback/Toast';

type Props = {
    isOpen: boolean;
    sessionId: string;
    initialConfig: ConnectionConfig;
    onClose: () => void;
    onSaved: () => void;
    runtime: SessionManagerRuntime;
};

const EditSavedSessionModal: React.FC<Props> = ({ isOpen, sessionId, initialConfig, onClose, onSaved, runtime }) => {
    const [config, setConfig] = useState<ConnectionConfig>(initialConfig);
    const [saving, setSaving] = useState(false);
    const toast = useToast();
    // 遮罩误关防护:记录 mousedown 起点,只有按下与松开(即 click)都发生在遮罩自身
    // 才关闭。否则在输入框内拖选文本、鼠标越过弹窗边界后松开时,浏览器会把 click
    // 派发到遮罩(公共祖先),直接 onClose 会丢掉整个弹窗的编辑内容。
    const overlayPressValidRef = useRef(false);

    useEffect(() => {
        if (!isOpen) return;
        setConfig(initialConfig);
        setSaving(false);
    }, [isOpen, initialConfig]);

    if (!isOpen) return null;

    const handleSave = async () => {
        if (!config.host?.trim()) {
            toast.warning('主机地址不能为空');
            return;
        }
        if (!config.user?.trim()) {
            toast.warning('用户名不能为空');
            return;
        }
        setSaving(true);
        try {
            const payload: ConnectionConfig = {
                ...config,
                port: config.port || 22,
                host: config.host.trim(),
                user: config.user.trim(),
                name: (config.name || '').trim(),
                group: (config.group || '').trim(),
            };
            if (payload.group === '') delete (payload as any).group;
            if (payload.name === '') delete (payload as any).name;
            if (payload.password === '') delete (payload as any).password;
            if (payload.rootPassword === '') delete (payload as any).rootPassword;
            if (payload.bastion) {
                const b = payload.bastion;
                const bastionClean: ConnectionConfig = {
                    ...b,
                    port: b.port || 22,
                    host: (b.host || '').trim(),
                    user: (b.user || '').trim(),
                    name: (b.name || '').trim(),
                };
                if (bastionClean.name === '') delete (bastionClean as any).name;
                if (bastionClean.password === '') delete (bastionClean as any).password;
                if (!bastionClean.host) {
                    toast.warning('跳板机主机不能为空');
                    setSaving(false);
                    return;
                }
                payload.bastion = bastionClean;
            }

            const group = payload.group || '';
            await runtime.updateSession(sessionId, payload, group);
            onSaved();
            onClose();
        } catch (e: any) {
            toast.error(e?.toString?.() || '保存失败');
            setSaving(false);
        }
    };

    return (
        <div
            style={styles.overlay}
            onMouseDown={(e) => {
                overlayPressValidRef.current = e.target === e.currentTarget;
            }}
            onClick={(e) => {
                if (e.target === e.currentTarget && overlayPressValidRef.current) {
                    onClose();
                }
                overlayPressValidRef.current = false;
            }}
        >
            <div style={styles.modal} onClick={(e) => e.stopPropagation()}>
                <div style={styles.header}>
                    <h2 style={styles.title}>编辑连接</h2>
                </div>
                <div style={styles.body}>
                    <ConnectionConfigForm
                        config={config}
                        onChange={setConfig}
                        idPrefix={`edit-${sessionId}`}
                        showName={true}
                        showGroup={true}
                    />
                </div>
                <div style={styles.footer}>
                    <button onClick={onClose} style={styles.cancelButton} disabled={saving}>取消</button>
                    <button onClick={handleSave} style={styles.submitButton} disabled={saving}>
                        {saving ? '保存中...' : '保存'}
                    </button>
                </div>
            </div>
        </div>
    );
};

const styles = {
    overlay: {
        position: 'fixed' as const,
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.7)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 1100,
    },
    modal: {
        backgroundColor: 'var(--bg-tertiary)',
        padding: '20px',
        borderRadius: '8px',
        width: '640px',
        maxHeight: '90vh',
        display: 'flex',
        flexDirection: 'column' as const,
        boxShadow: '0 4px 6px rgba(0, 0, 0, 0.1)',
        color: 'var(--text-primary)',
    },
    header: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        marginBottom: '12px',
    },
    title: {
        margin: 0,
        fontSize: '1.2rem',
    },
    body: {
        overflowY: 'auto' as const,
        paddingRight: '4px',
    },
    footer: {
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '10px',
        marginTop: '16px',
    },
    cancelButton: {
        padding: '10px 16px',
        borderRadius: '6px',
        border: '1px solid var(--border-strong)',
        backgroundColor: 'transparent',
        color: 'var(--text-primary)',
        cursor: 'pointer',
    },
    submitButton: {
        padding: '10px 16px',
        borderRadius: '6px',
        border: 'none',
        backgroundColor: 'var(--accent)',
        color: 'var(--text-on-accent)',
        cursor: 'pointer',
        fontWeight: 'bold' as const,
    },
};

export default EditSavedSessionModal;
