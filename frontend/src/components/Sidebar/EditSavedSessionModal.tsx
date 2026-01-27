import React, { useEffect, useState } from 'react';
import { ConnectionConfig } from '../../types';
import ConnectionConfigForm from '../ConnectionConfigForm/ConnectionConfigForm';

type Props = {
    isOpen: boolean;
    sessionId: string;
    initialConfig: ConnectionConfig;
    onClose: () => void;
    onSaved: () => void;
};

const EditSavedSessionModal: React.FC<Props> = ({ isOpen, sessionId, initialConfig, onClose, onSaved }) => {
    const [config, setConfig] = useState<ConnectionConfig>(initialConfig);
    const [saving, setSaving] = useState(false);

    useEffect(() => {
        if (!isOpen) return;
        setConfig(initialConfig);
        setSaving(false);
    }, [isOpen, initialConfig]);

    if (!isOpen) return null;

    const handleSave = async () => {
        if (!config.host?.trim()) {
            alert('主机地址不能为空');
            return;
        }
        if (!config.user?.trim()) {
            alert('用户名不能为空');
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
                    alert('跳板机主机不能为空');
                    setSaving(false);
                    return;
                }
                payload.bastion = bastionClean;
            }

            const resp = await window.go.main.App.UpdateSavedSession(sessionId, payload);
            if (resp && resp.startsWith('Error:')) {
                alert(resp);
                setSaving(false);
                return;
            }
            onSaved();
            onClose();
        } catch (e: any) {
            alert(e?.toString?.() || '保存失败');
            setSaving(false);
        }
    };

    return (
        <div style={styles.overlay} onClick={onClose}>
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
        backgroundColor: '#2d2d2d',
        padding: '20px',
        borderRadius: '8px',
        width: '640px',
        maxHeight: '90vh',
        display: 'flex',
        flexDirection: 'column' as const,
        boxShadow: '0 4px 6px rgba(0, 0, 0, 0.1)',
        color: '#fff',
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
        border: '1px solid #555',
        backgroundColor: 'transparent',
        color: '#fff',
        cursor: 'pointer',
    },
    submitButton: {
        padding: '10px 16px',
        borderRadius: '6px',
        border: 'none',
        backgroundColor: '#007acc',
        color: '#fff',
        cursor: 'pointer',
        fontWeight: 'bold' as const,
    },
};

export default EditSavedSessionModal;
