import React, { useEffect, useRef, useState } from 'react';
import { ConnectionConfig } from '../types';
import ConnectionConfigForm from '../connection/ConnectionConfigForm';
import { SessionManagerRuntime } from '../ports';
import { useToast } from '../feedback/Toast';

type Props = {
    isOpen: boolean;
    /** create = 保存为新连接（不建立会话）；edit = 更新已有连接。 */
    mode: 'create' | 'edit';
    /** edit 模式必填。 */
    sessionId?: string;
    initialConfig: ConnectionConfig;
    /** create 模式的落点文件夹；空表示根。 */
    parentId?: string;
    /** 落点的展示名，用于告知用户会保存到哪里。 */
    parentLabel?: string;
    onClose: () => void;
    onSaved: () => void;
    runtime: SessionManagerRuntime;
};

/**
 * 连接属性弹窗，新建与编辑共用。
 *
 * 与旧版的关键差异：不再提供自由文本"分组"输入。归属只由会话树里的位置决定
 * （拖拽移动），因此不存在"字段值与实际位置不一致"的可能——这正是之前
 * config.group 漂移的根因。
 */
const ConnectionPropertiesModal: React.FC<Props> = ({
    isOpen,
    mode,
    sessionId,
    initialConfig,
    parentId = '',
    parentLabel,
    onClose,
    onSaved,
    runtime,
}) => {
    const [config, setConfig] = useState<ConnectionConfig>(initialConfig);
    const [saving, setSaving] = useState(false);
    const toast = useToast();
    // 遮罩误关防护：记录 mousedown 起点，只有按下与松开都发生在遮罩自身才关闭。
    // 否则在输入框内拖选文本、鼠标越过弹窗边界后松开时，浏览器会把 click 派发到
    // 遮罩（公共祖先），直接关闭会丢掉整个弹窗的编辑内容。
    const overlayPressValidRef = useRef(false);

    useEffect(() => {
        if (!isOpen) return;
        setConfig(initialConfig);
        setSaving(false);
    }, [isOpen, initialConfig]);

    if (!isOpen) return null;

    const handleSubmit = async () => {
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
            const payload = normalizeConfig(config);
            if (mode === 'create') {
                await runtime.createConnection(payload, parentId);
            } else {
                if (!sessionId) throw new Error('缺少会话 ID');
                await runtime.updateConnection(sessionId, payload);
            }
            onSaved();
            onClose();
        } catch (e: any) {
            toast.error(e?.toString?.() || '保存失败');
            setSaving(false);
        }
    };

    const destination = parentLabel ? `将保存到「${parentLabel}」` : '将保存到根目录';

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
                    <h2 style={styles.title}>{mode === 'create' ? '新建连接' : '编辑连接'}</h2>
                    {mode === 'create' && <span style={styles.destination}>{destination}</span>}
                </div>
                <div style={styles.body}>
                    <ConnectionConfigForm
                        config={config}
                        onChange={setConfig}
                        idPrefix={`${mode}-${sessionId ?? 'new'}`}
                        showName={true}
                        // 归属由树里的位置决定，弹窗内不提供分组输入。
                        showGroup={false}
                    />
                </div>
                <div style={styles.footer}>
                    <button onClick={onClose} style={styles.cancelButton} disabled={saving}>取消</button>
                    <button onClick={handleSubmit} style={styles.submitButton} disabled={saving}>
                        {saving ? '保存中...' : (mode === 'create' ? '保存' : '保存修改')}
                    </button>
                </div>
            </div>
        </div>
    );
};

/** 去掉空字符串字段，避免把空值当成"清空该字段"写回后端。 */
function normalizeConfig(config: ConnectionConfig): ConnectionConfig {
    const payload: ConnectionConfig = {
        ...config,
        port: config.port || 22,
        host: config.host.trim(),
        user: config.user.trim(),
        name: (config.name || '').trim(),
    };
    delete (payload as any).group;
    if (!payload.name) delete (payload as any).name;
    if (!payload.password) delete (payload as any).password;
    if (!payload.rootPassword) delete (payload as any).rootPassword;

    if (payload.bastion) {
        const bastion = payload.bastion;
        const cleaned: ConnectionConfig = {
            ...bastion,
            port: bastion.port || 22,
            host: (bastion.host || '').trim(),
            user: (bastion.user || '').trim(),
            name: (bastion.name || '').trim(),
        };
        if (!cleaned.name) delete (cleaned as any).name;
        if (!cleaned.password) delete (cleaned as any).password;
        payload.bastion = cleaned;
    }
    return payload;
}

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
        alignItems: 'baseline',
        gap: '10px',
        marginBottom: '12px',
    },
    title: {
        margin: 0,
        fontSize: '1.2rem',
    },
    destination: {
        fontSize: '12px',
        color: 'var(--text-muted)',
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

export default ConnectionPropertiesModal;
