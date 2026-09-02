import React, { useState, useEffect } from 'react';
import { QuickCommand } from '../ports';

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
            // 从分组条 "+" 进入时 group 为 __new__：直接进入新分组输入模式，
            // 否则用户必须先切到其它分组再切回来才能看到输入框
            setIsNewGroupMode(command.group === '__new__');
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

    // 新分组模式下分组名必填：留空（含初始占位 __new__）不允许保存，
    // 避免命令被静默塞进第一个分组
    const effectiveGroup = editCmd.group === '__new__' ? '' : (editCmd.group || '');
    const groupNameEmpty = isNewGroupMode && !effectiveGroup.trim();
    const canSave = !!editCmd.name.trim() && !groupNameEmpty;

    const handleSave = () => {
        if (!canSave) return;
        onSave(editCmd);
    };

    return (
        <div style={styles.overlay} data-testid="command-edit-modal">
            <div style={styles.modal}>
                <h3 style={styles.title}>
                    {isNew && isNewGroupMode ? '新建分组' : isNew ? '新建命令' : '编辑命令'}
                </h3>

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
                        <div style={styles.groupRow}>
                            <input
                                style={{ ...styles.input, flex: 1 }}
                                value={editCmd.group === '__new__' ? '' : editCmd.group || ''}
                                onChange={e => setEditCmd({ ...editCmd, group: e.target.value })}
                                placeholder="输入新分组名"
                                autoFocus
                                data-testid="command-group-input"
                            />
                            {/* 显式的退出入口：回到选择已有分组。
                                不做 onBlur 自动回退——点到其它字段就丢掉输入框是"流程不通"的元凶之一 */}
                            <button
                                style={styles.groupRevertBtn}
                                title="返回选择已有分组"
                                data-testid="command-group-revert"
                                onClick={() => {
                                    setEditCmd({ ...editCmd, group: availableGroups[0] || 'default' });
                                    setIsNewGroupMode(false);
                                }}
                            >
                                ✕
                            </button>
                        </div>
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
                    <button
                        onClick={handleSave}
                        style={{ ...styles.saveBtn, ...(canSave ? {} : styles.saveBtnDisabled) }}
                        data-testid="command-edit-save"
                    >
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
    groupRow: {
        display: 'flex',
        gap: '4px',
        alignItems: 'stretch',
    },
    groupRevertBtn: {
        flex: '0 0 auto',
        width: '30px',
        backgroundColor: 'var(--bg-elevated)',
        color: 'var(--text-muted)',
        border: '1px solid var(--border-strong)',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '12px',
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
    saveBtnDisabled: {
        opacity: 0.45,
        cursor: 'not-allowed',
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
