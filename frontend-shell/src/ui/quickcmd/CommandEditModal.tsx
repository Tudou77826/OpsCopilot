import React, { useState, useEffect, useId, useRef } from 'react';
import { QuickCommand } from '../ports';
import { btnSecondary } from '../settings/settingsStyles';
import './CommandEditModal.css';

const variables = [
    { token: '{{local_ip}}', label: '本地 IP', description: '本机连接 SSH 时使用的 IP', example: '192.0.2.10' },
    { token: '{{remote_ip}}', label: '远程 IP', description: '当前 SSH 会话的目标 IP', example: '198.51.100.20' },
    { token: '{{selected_text}}', label: '终端选中文本', description: '当前终端里选中的一行文字', example: 'REQ-8742' },
    { token: '{{date}}', label: '本地日期', description: '电脑当天的日期 · YYYY-MM-DD' },
];

const variablePattern = /\{\{(?:local_ip|remote_ip|selected_text|date)\}\}/g;

function previewTemplate(content: string) {
    const today = new Date();
    const date = [today.getFullYear(), today.getMonth() + 1, today.getDate()]
        .map(part => String(part).padStart(2, '0')).join('-');
    return content.replace(variablePattern, token => variables.find(variable => variable.token === token)?.example ?? date);
}

interface CommandEditModalProps {
    isOpen: boolean;
    command: QuickCommand | null;
    isNew: boolean;
    availableGroups: string[];
    onSave: (command: QuickCommand) => void;
    onCancel: () => void;
    defaultGroup?: string;
    supportsVariables?: boolean;
}

const CommandEditModal: React.FC<CommandEditModalProps> = ({
    isOpen,
    command,
    isNew,
    availableGroups,
    onSave,
    onCancel,
    defaultGroup = 'default',
    supportsVariables = false,
}) => {
    const [editCmd, setEditCmd] = useState<QuickCommand | null>(null);
    const [isNewGroupMode, setIsNewGroupMode] = useState(false);
    const [variablesOpen, setVariablesOpen] = useState(false);
    const inputRef = useRef<HTMLTextAreaElement>(null);
    const variablesButtonRef = useRef<HTMLButtonElement>(null);
    const contentId = useId();

    useEffect(() => {
        setVariablesOpen(false);
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
    const useVariables = editCmd.useVariables && !!editCmd.content.match(variablePattern);
    const invalidTemplate = useVariables && /[\u0000-\u001f\u007f-\u009f]/.test(editCmd.content);
    const preview = useVariables && !invalidTemplate ? previewTemplate(editCmd.content) : '';
    const canSave = !!editCmd.name.trim() && !groupNameEmpty && !invalidTemplate;

    const handleSave = () => {
        if (!canSave) return;
        onSave({ ...editCmd, useVariables });
    };

    const insertVariable = (token: string) => {
        const input = inputRef.current;
        if (!input) return;
        const start = input.selectionStart;
        const end = input.selectionEnd;
        setEditCmd({
            ...editCmd,
            useVariables: true,
            content: editCmd.content.slice(0, start) + token + editCmd.content.slice(end),
        });
        setVariablesOpen(false);
        requestAnimationFrame(() => {
            input.focus();
            input.setSelectionRange(start + token.length, start + token.length);
        });
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
                    <div className="oc-quick-command-content-heading">
                        <label style={styles.label} htmlFor={contentId}>命令内容</label>
                        {supportsVariables && !isNewGroupMode && (
                            <div className="oc-quick-command-variable-picker"
                                onBlur={event => {
                                    if (!event.currentTarget.contains(event.relatedTarget)) setVariablesOpen(false);
                                }}
                                onKeyDown={event => {
                                    if (event.key === 'Escape' && variablesOpen) {
                                        event.stopPropagation();
                                        setVariablesOpen(false);
                                        variablesButtonRef.current?.focus();
                                    }
                                }}
                            >
                                <button type="button" ref={variablesButtonRef} style={btnSecondary}
                                    aria-expanded={variablesOpen} aria-controls={variablesOpen ? `${contentId}-variables` : undefined}
                                    onClick={() => setVariablesOpen(open => !open)}>
                                    插入变量 <span aria-hidden="true">▾</span>
                                </button>
                                {variablesOpen && (
                                    <div id={`${contentId}-variables`} className="oc-quick-command-variable-options" role="group" aria-label="预置变量">
                                        {variables.map(variable => (
                                            <button type="button" key={variable.token} onClick={() => insertVariable(variable.token)}>
                                                <span>{variable.label}</span>
                                                <small>{variable.description}</small>
                                            </button>
                                        ))}
                                    </div>
                                )}
                            </div>
                        )}
                    </div>
                    <textarea
                        id={contentId}
                        ref={inputRef}
                        style={{ ...styles.input, ...styles.textarea }}
                        value={editCmd.content}
                        onChange={e => setEditCmd({ ...editCmd, content: e.target.value })}
                        placeholder="输入命令"
                        data-testid="command-content-textarea"
                    />
                    {invalidTemplate ? (
                        <div className="oc-quick-command-template-error" role="alert">含变量的快捷指令请输入一行文本</div>
                    ) : preview && (
                        <div className="oc-quick-command-template-preview">
                            <span>示例预览</span>
                            <code title={preview}>{preview}</code>
                        </div>
                    )}
                </div>

                <div style={styles.actions}>
                    <button onClick={onCancel} style={styles.cancelBtn} data-testid="command-edit-cancel">
                        取消
                    </button>
                    <button
                        onClick={handleSave}
                        disabled={!canSave}
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
        backgroundColor: 'var(--scrim-soft)',
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
        fontFamily: 'inherit',
        fontSize: '13px',
    },
    textarea: {
        height: '80px',
        resize: 'none' as const,
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
