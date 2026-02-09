import React, { useEffect, useState } from 'react';

interface ScriptCommand {
    index: number;
    content: string;
    comment: string;
    delay: number;
    enabled: boolean;
}

interface Script {
    id: string;
    name: string;
    description: string;
    commands: ScriptCommand[];
}

interface ScriptEditorModalProps {
    isOpen: boolean;
    scriptId: string | null;
    onClose: () => void;
    onSave: () => void;
}

const ScriptEditorModal: React.FC<ScriptEditorModalProps> = ({
    isOpen,
    scriptId,
    onClose,
    onSave
}) => {
    const [script, setScript] = useState<Script | null>(null);
    const [loading, setLoading] = useState(false);
    const [saving, setSaving] = useState(false);

    useEffect(() => {
        if (isOpen && scriptId) {
            loadScript();
        }
    }, [isOpen, scriptId]);

    const loadScript = async () => {
        if (!scriptId) return;

        setLoading(true);
        try {
            // @ts-ignore
            const result = await window.go.main.App.LoadScript(scriptId);
            setScript(result);
        } catch (err: any) {
            alert('加载脚本失败: ' + err.message);
            onClose();
        } finally {
            setLoading(false);
        }
    };

    const handleSave = async () => {
        if (!script) return;

        setSaving(true);
        try {
            // @ts-ignore
            await window.go.main.App.UpdateScript(script);
            onSave();
            onClose();
        } catch (err: any) {
            alert('保存失败: ' + err.message);
        } finally {
            setSaving(false);
        }
    };

    const handleCommandChange = (index: number, field: keyof ScriptCommand, value: any) => {
        if (!script) return;

        const updatedCommands = [...script.commands];
        updatedCommands[index] = {
            ...updatedCommands[index],
            [field]: value
        };

        setScript({
            ...script,
            commands: updatedCommands
        });
    };

    const handleAddCommand = () => {
        if (!script) return;

        const newCommand: ScriptCommand = {
            index: script.commands.length,
            content: '',
            comment: '',
            delay: 0,
            enabled: true
        };

        setScript({
            ...script,
            commands: [...script.commands, newCommand]
        });
    };

    const handleDeleteCommand = (index: number) => {
        if (!script) return;
        if (!confirm('确定要删除这条命令吗？')) return;

        const updatedCommands = script.commands.filter((_, i) => i !== index);
        // 重新编号
        updatedCommands.forEach((cmd, i) => {
            cmd.index = i;
        });

        setScript({
            ...script,
            commands: updatedCommands
        });
    };

    if (!isOpen) return null;

    if (loading) {
        return (
            <div style={styles.overlay}>
                <div style={styles.modal}>
                    <div style={styles.loading}>加载中...</div>
                </div>
            </div>
        );
    }

    if (!script) {
        return null;
    }

    return (
        <div style={styles.overlay}>
            <div style={styles.modal}>
                <div style={styles.header}>
                    <h2 style={styles.title}>编辑脚本</h2>
                    <button style={styles.closeButton} onClick={onClose}>×</button>
                </div>

                <div style={styles.body}>
                    <div style={styles.fieldGroup}>
                        <label style={styles.label}>名称:</label>
                        <input
                            style={styles.input}
                            type="text"
                            value={script.name}
                            onChange={(e) => setScript({ ...script, name: e.target.value })}
                            placeholder="脚本名称"
                        />
                    </div>

                    <div style={styles.fieldGroup}>
                        <label style={styles.label}>描述:</label>
                        <input
                            style={styles.input}
                            type="text"
                            value={script.description}
                            onChange={(e) => setScript({ ...script, description: e.target.value })}
                            placeholder="脚本描述"
                        />
                    </div>

                    <div style={styles.commandsSection}>
                        <div style={styles.commandsHeader}>
                            <label style={styles.label}>命令列表:</label>
                            <button style={styles.addButton} onClick={handleAddCommand}>
                                + 添加命令
                            </button>
                        </div>

                        <div style={styles.commandsList}>
                            {script.commands.map((cmd, idx) => (
                                <div key={idx} style={styles.commandRow}>
                                    <div style={styles.commandIndex}>{cmd.index + 1}</div>

                                    <div style={styles.commandMain}>
                                        <input
                                            style={styles.commandInput}
                                            type="text"
                                            value={cmd.content}
                                            onChange={(e) => handleCommandChange(idx, 'content', e.target.value)}
                                            placeholder="命令内容"
                                        />
                                    </div>

                                    <div style={styles.commandMeta}>
                                        <input
                                            style={styles.metaInput}
                                            type="text"
                                            value={cmd.comment}
                                            onChange={(e) => handleCommandChange(idx, 'comment', e.target.value)}
                                            placeholder="备注"
                                        />

                                        <input
                                            style={styles.delayInput}
                                            type="number"
                                            value={cmd.delay}
                                            onChange={(e) => handleCommandChange(idx, 'delay', parseInt(e.target.value) || 0)}
                                            placeholder="延迟(ms)"
                                            min="0"
                                            step="100"
                                        />

                                        <label style={styles.checkboxLabel}>
                                            <input
                                                type="checkbox"
                                                checked={cmd.enabled}
                                                onChange={(e) => handleCommandChange(idx, 'enabled', e.target.checked)}
                                            />
                                            启用
                                        </label>

                                        <button
                                            style={styles.deleteCommandButton}
                                            onClick={() => handleDeleteCommand(idx)}
                                            title="删除"
                                        >
                                            🗑️
                                        </button>
                                    </div>
                                </div>
                            ))}
                        </div>
                    </div>
                </div>

                <div style={styles.footer}>
                    <button style={styles.cancelButton} onClick={onClose}>
                        取消
                    </button>
                    <button
                        style={styles.saveButton}
                        onClick={handleSave}
                        disabled={saving}
                    >
                        {saving ? '保存中...' : '保存'}
                    </button>
                </div>
            </div>
        </div>
    );
};

const styles: Record<string, React.CSSProperties> = {
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
        zIndex: 5000,
    },
    modal: {
        width: '900px',
        maxHeight: '80vh',
        backgroundColor: '#1e1e1e',
        borderRadius: '8px',
        border: '1px solid #333',
        display: 'flex',
        flexDirection: 'column' as const,
    },
    header: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        padding: '16px 20px',
        borderBottom: '1px solid #333',
    },
    title: {
        margin: 0,
        fontSize: '16px',
        fontWeight: 600,
        color: '#e0e0e0',
    },
    closeButton: {
        width: '32px',
        height: '32px',
        padding: '0',
        backgroundColor: 'transparent',
        border: 'none',
        color: '#b0b0b0',
        fontSize: '24px',
        cursor: 'pointer',
        borderRadius: '4px',
    },
    body: {
        flex: 1,
        overflowY: 'auto' as const,
        padding: '20px',
    },
    loading: {
        textAlign: 'center',
        padding: '40px',
        color: '#b0b0b0',
    },
    fieldGroup: {
        marginBottom: '16px',
    },
    label: {
        display: 'block',
        fontSize: '13px',
        color: '#b0b0b0',
        marginBottom: '6px',
        fontWeight: 500,
    },
    input: {
        width: '100%',
        padding: '8px 12px',
        backgroundColor: '#2d2d2d',
        border: '1px solid #3c3c3c',
        borderRadius: '4px',
        color: '#e0e0e0',
        fontSize: '13px',
        boxSizing: 'border-box' as const,
    },
    commandsSection: {
        marginTop: '20px',
    },
    commandsHeader: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        marginBottom: '12px',
    },
    addButton: {
        padding: '6px 12px',
        backgroundColor: '#1976d2',
        color: 'white',
        border: 'none',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '12px',
    },
    commandsList: {
        border: '1px solid #3c3c3c',
        borderRadius: '4px',
        overflow: 'hidden',
    },
    commandRow: {
        display: 'grid',
        gridTemplateColumns: '40px 1fr',
        padding: '8px',
        backgroundColor: '#252526',
        borderBottom: '1px solid #3c3c3c',
        gap: '8px',
    },
    commandIndex: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontSize: '12px',
        color: '#757575',
        fontWeight: 600,
    },
    commandMain: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '6px',
    },
    commandInput: {
        width: '100%',
        padding: '6px 10px',
        backgroundColor: '#1e1e1e',
        border: '1px solid #3c3c3c',
        borderRadius: '4px',
        color: '#e0e0e0',
        fontSize: '13px',
        fontFamily: 'Consolas, Monaco, monospace',
        boxSizing: 'border-box' as const,
    },
    commandMeta: {
        display: 'flex',
        gap: '8px',
        alignItems: 'center',
    },
    metaInput: {
        flex: 1,
        padding: '4px 8px',
        backgroundColor: '#1e1e1e',
        border: '1px solid #3c3c3c',
        borderRadius: '4px',
        color: '#b0b0b0',
        fontSize: '12px',
    },
    delayInput: {
        width: '80px',
        padding: '4px 8px',
        backgroundColor: '#1e1e1e',
        border: '1px solid #3c3c3c',
        borderRadius: '4px',
        color: '#e0e0e0',
        fontSize: '12px',
    },
    checkboxLabel: {
        display: 'flex',
        alignItems: 'center',
        gap: '4px',
        fontSize: '12px',
        color: '#b0b0b0',
        cursor: 'pointer',
    },
    deleteCommandButton: {
        width: '24px',
        height: '24px',
        padding: '0',
        backgroundColor: '#d32f2f',
        border: 'none',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '12px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
    },
    footer: {
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '12px',
        padding: '16px 20px',
        borderTop: '1px solid #333',
    },
    cancelButton: {
        padding: '8px 16px',
        backgroundColor: '#424242',
        color: '#e0e0e0',
        border: '1px solid #555',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '13px',
    },
    saveButton: {
        padding: '8px 16px',
        backgroundColor: '#1976d2',
        color: 'white',
        border: 'none',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '13px',
        fontWeight: 500,
    },
};

export default ScriptEditorModal;
