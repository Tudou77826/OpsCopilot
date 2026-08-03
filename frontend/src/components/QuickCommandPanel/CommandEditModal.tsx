import React, { useState, useEffect } from 'react';
import { QuickCommand } from './types';

interface CommandEditModalProps {
    isOpen: boolean;
    command: QuickCommand | null;
    isNew: boolean;
    availableGroups: string[];
    onSave: (command: QuickCommand) => void;
    onCancel: () => void;
    defaultGroup?: string;
}

const CommandEditModal: React.FC<CommandEditModalProps> = ({
    isOpen,
    command,
    isNew,
    availableGroups,
    onSave,
    onCancel,
    defaultGroup = 'default',
}) => {
    const [editCmd, setEditCmd] = useState<QuickCommand | null>(null);
    const [isNewGroupMode, setIsNewGroupMode] = useState(false);

    useEffect(() => {
        if (isOpen && command) {
            setEditCmd({ ...command });
            setIsNewGroupMode(false);
        } else if (isOpen && isNew) {
            setEditCmd({
                id: Date.now().toString(),
                name: '',
                content: '',
                group: defaultGroup,
            });
            setIsNewGroupMode(false);
        }
    }, [isOpen, command, isNew, defaultGroup]);

    if (!isOpen || !editCmd) return null;

    const handleSave = () => {
        if (!editCmd.name.trim()) return;
        onSave(editCmd);
    };

    return (
        <div style={styles.overlay} data-testid="command-edit-modal">
            <div style={styles.modal}>
                <h3 style={styles.title}>{isNew ? '新建命令' : '编辑命令'}</h3>

                <div style={styles.formGroup}>
                    <label style={styles.label}>名称</label>
                    <input
                        style={styles.input}
                        value={editCmd.name}
                        onChange={e => setEditCmd({ ...editCmd, name: e.target.value })}
                        placeholder="命令名称"
                        autoFocus
                        data-testid="command-name-input"
                    />
                </div>

                <div style={styles.formGroup}>
                    <label style={styles.label}>分组</label>
                    {isNewGroupMode ? (
                        <input
                            style={styles.input}
                            value={editCmd.group === '__new__' ? '' : editCmd.group || ''}
                            onChange={e => setEditCmd({ ...editCmd, group: e.target.value })}
                            placeholder="输入新分组名"
                            autoFocus
                            onBlur={e => {
                                if (!e.target.value.trim()) {
                                    setEditCmd({ ...editCmd, group: availableGroups[0] || 'default' });
                                    setIsNewGroupMode(false);
                                }
                            }}
                            data-testid="command-group-input"
                        />
                    ) : (
                        <select
                            style={styles.input}
                            value={editCmd.group || 'default'}
                            onChange={e => {
                                if (e.target.value === '__new__') {
                                    setIsNewGroupMode(true);
                                    setEditCmd({ ...editCmd, group: '__new__' });
                                } else {
                                    setEditCmd({ ...editCmd, group: e.target.value });
                                }
                            }}
                            data-testid="command-group-select"
                        >
                            {availableGroups.map(g => (
                                <option key={g} value={g}>{g}</option>
                            ))}
                            <option value="__new__" style={{ color: 'var(--accent)' }}>+ 新建分组...</option>
                        </select>
                    )}
                </div>

                <div style={styles.formGroup}>
                    <label style={styles.label}>命令内容</label>
                    <textarea
                        style={styles.textarea}
                        value={editCmd.content}
                        onChange={e => setEditCmd({ ...editCmd, content: e.target.value })}
                        placeholder="输入命令"
                        data-testid="command-content-textarea"
                    />
                </div>

                <div style={styles.actions}>
                    <button onClick={onCancel} style={styles.cancelBtn} data-testid="command-edit-cancel">
                        取消
                    </button>
                    <button onClick={handleSave} style={styles.saveBtn} data-testid="command-edit-save">
                        保存
                    </button>
                </div>
            </div>
        </div>
    );
};

const styles = {
    overlay: {
        position: 'fixed' as const,
        top: 0, left: 0, right: 0, bottom: 0,
        backgroundColor: 'rgba(0,0,0,0.5)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 1000,
    },
    modal: {
        backgroundColor: 'var(--bg-secondary)',
        padding: '20px',
        borderRadius: '8px',
        width: '320px',
        border: '1px solid var(--border)',
    },
    title: {
        color: 'var(--text-primary)',
        marginTop: 0,
        marginBottom: '16px',
        fontSize: '16px',
    },
    formGroup: {
        marginBottom: '12px',
    },
    label: {
        display: 'block',
        color: 'var(--text-secondary)',
        marginBottom: '4px',
        fontSize: '12px',
    },
    input: {
        width: '100%',
        padding: '6px 8px',
        backgroundColor: 'var(--border)',
        border: '1px solid var(--border-strong)',
        color: 'var(--text-primary)',
        borderRadius: '4px',
        boxSizing: 'border-box' as const,
        fontSize: '13px',
    },
    textarea: {
        width: '100%',
        height: '80px',
        padding: '6px 8px',
        backgroundColor: 'var(--border)',
        border: '1px solid var(--border-strong)',
        color: 'var(--text-primary)',
        borderRadius: '4px',
        resize: 'none' as const,
        boxSizing: 'border-box' as const,
        fontFamily: 'monospace',
        fontSize: '13px',
    },
    actions: {
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '8px',
        marginTop: '20px',
    },
    saveBtn: {
        padding: '6px 16px',
        backgroundColor: 'var(--accent)',
        color: 'var(--text-on-accent)',
        border: 'none',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '13px',
    },
    cancelBtn: {
        padding: '6px 16px',
        backgroundColor: 'var(--bg-elevated)',
        color: 'var(--text-secondary)',
        border: '1px solid var(--border-strong)',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '13px',
    },
};

export default CommandEditModal;
