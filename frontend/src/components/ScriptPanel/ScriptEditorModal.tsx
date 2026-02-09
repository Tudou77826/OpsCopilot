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

    // Inject CSS for interactions - must be before any early returns
    useEffect(() => {
        // Inject CSS for hover states and switch toggle
        const style = document.createElement('style');
        style.textContent = `
            @keyframes fadeIn {
                from { opacity: 0; }
                to { opacity: 1; }
            }
            @keyframes slideUp {
                from {
                    opacity: 0;
                    transform: translateY(20px) scale(0.98);
                }
                to {
                    opacity: 1;
                    transform: translateY(0) scale(1);
                }
            }

            /* Switch toggle styles */
            .switch-slider {
                background-color: #424242;
            }
            .switch-checkbox:checked + .switch-slider {
                background-color: #4ade80;
            }
            .switch-checkbox:checked + .switch-slider::after {
                transform: translateX(16px);
            }
            .switch-slider::after {
                content: '';
                position: absolute;
                top: 2px;
                left: 2px;
                width: 16px;
                height: 16px;
                background-color: white;
                border-radius: 50%;
                transition: transform 0.2s ease;
            }

            /* Input focus styles */
            input:focus {
                outline: none;
                border-color: #007acc !important;
                box-shadow: 0 0 0 2px rgba(0, 122, 204, 0.2);
            }

            /* Button hover styles */
            button:hover {
                transform: translateY(-1px);
            }
            .actionButton:hover {
                background-color: #2d2d2d;
                color: #ffffff;
            }
            .actionButton:active {
                transform: translateY(0);
            }
            .closeButton:hover {
                background-color: #2d2d2d;
                color: #ffffff;
            }
            .addButton:hover {
                background-color: #0069b4;
                box-shadow: 0 4px 12px rgba(0, 122, 204, 0.4);
            }
            .saveButton:hover {
                background-color: #0069b4;
                box-shadow: 0 4px 12px rgba(0, 122, 204, 0.4);
            }
            .cancelButton:hover {
                background-color: #3e3e42;
                border-color: #5a5a5a;
            }

            /* Command card hover */
            .commandCard:hover {
                borderColor: #5a5a5a;
                box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
            }

            /* Metadata input hover */
            .metadataInput:hover,
            .delayInputGroup:hover {
                border-color: #5a5a5a;
            }
        `;
        document.head.appendChild(style);

        return () => {
            try {
                document.head.removeChild(style);
            } catch (e) {
                // Style already removed
            }
        };
    }, []);

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
                    <button style={{...styles.closeButton}} className="closeButton" onClick={onClose}>×</button>
                </div>

                <div style={styles.body}>
                    <div style={styles.fieldGroup}>
                        <label style={styles.label}>名称</label>
                        <input
                            style={styles.input}
                            type="text"
                            value={script.name}
                            onChange={(e) => setScript({ ...script, name: e.target.value })}
                            placeholder="脚本名称"
                        />
                    </div>

                    <div style={styles.fieldGroup}>
                        <label style={styles.label}>描述</label>
                        <input
                            style={styles.input}
                            type="text"
                            value={script.description}
                            onChange={(e) => setScript({ ...script, description: e.target.value })}
                            placeholder="脚本描述"
                        />
                    </div>

                    <div style={styles.commandsSection}>
                        <div style={styles.sectionHeader}>
                            <div style={styles.sectionTitleGroup}>
                                <label style={styles.sectionTitle}>命令列表</label>
                                <span style={styles.sectionSubtitle}>{script.commands.length} 条命令</span>
                            </div>
                            <button style={{...styles.addButton}} className="addButton" onClick={handleAddCommand}>
                                <span style={styles.addIcon}>+</span>
                                <span style={styles.addText}>添加命令</span>
                            </button>
                        </div>

                        <div style={styles.commandsList}>
                            {script.commands.map((cmd, idx) => (
                                <div key={idx} style={{...styles.commandCard}} className="commandCard">
                                    {/* Main content area */}
                                    <div style={styles.commandContent}>
                                        <input
                                            style={styles.commandInput}
                                            type="text"
                                            value={cmd.content}
                                            onChange={(e) => handleCommandChange(idx, 'content', e.target.value)}
                                            placeholder="输入命令..."
                                        />

                                        {/* Metadata row */}
                                        <div style={styles.commandMetadata}>
                                            {/* Comment */}
                                            <div style={styles.metadataItem}>
                                                <input
                                                    style={{...styles.metadataInput}}
                                                    className="metadataInput"
                                                    type="text"
                                                    value={cmd.comment}
                                                    onChange={(e) => handleCommandChange(idx, 'comment', e.target.value)}
                                                    placeholder="备注"
                                                />
                                            </div>

                                            {/* Delay */}
                                            <div style={styles.metadataItem}>
                                                <div style={{...styles.delayInputGroup}} className="delayInputGroup">
                                                    <input
                                                        style={styles.delayInput}
                                                        type="number"
                                                        value={cmd.delay}
                                                        onChange={(e) => handleCommandChange(idx, 'delay', parseInt(e.target.value) || 0)}
                                                        placeholder="0"
                                                        min="0"
                                                        step="100"
                                                    />
                                                    <span style={styles.delayUnit}>ms</span>
                                                </div>
                                            </div>

                                            {/* Enabled toggle */}
                                            <div style={styles.metadataItem}>
                                                <label style={styles.switchLabel} title={cmd.enabled ? '已启用' : '已禁用'}>
                                                    <input
                                                        type="checkbox"
                                                        checked={cmd.enabled}
                                                        onChange={(e) => handleCommandChange(idx, 'enabled', e.target.checked)}
                                                        style={styles.switchCheckbox}
                                                        className="switch-checkbox"
                                                    />
                                                    <span style={styles.switchSlider} className="switch-slider"></span>
                                                </label>
                                            </div>

                                            {/* Spacer before delete button */}
                                            <div style={styles.metadataSpacer}></div>
                                        </div>
                                    </div>

                                    {/* Actions */}
                                    <div style={styles.commandActions}>
                                        <button
                                            style={{...styles.actionButton}}
                                            className="actionButton"
                                            onClick={() => handleDeleteCommand(idx)}
                                            title="删除命令"
                                        >
                                            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                                                <path d="M3 6h18M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2"/>
                                            </svg>
                                        </button>
                                    </div>
                                </div>
                            ))}
                        </div>
                    </div>
                </div>

                <div style={styles.footer}>
                    <button style={{...styles.cancelButton}} className="cancelButton" onClick={onClose}>
                        取消
                    </button>
                    <button
                        style={{...styles.saveButton}}
                        className="saveButton"
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
        backgroundColor: 'rgba(0, 0, 0, 0.75)',
        backdropFilter: 'blur(4px)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 5000,
        animation: 'fadeIn 0.2s ease-out',
    },
    modal: {
        width: '960px',
        maxHeight: '85vh',
        backgroundColor: '#1e1e1e',
        borderRadius: '12px',
        border: '1px solid #3e3e42',
        boxShadow: '0 20px 60px rgba(0, 0, 0, 0.5)',
        display: 'flex',
        flexDirection: 'column' as const,
        animation: 'slideUp 0.3s ease-out',
    },
    header: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        padding: '20px 24px',
        borderBottom: '1px solid #3e3e42',
    },
    title: {
        margin: 0,
        fontSize: '18px',
        fontWeight: 600,
        color: '#ffffff',
        letterSpacing: '-0.01em',
    },
    closeButton: {
        width: '36px',
        height: '36px',
        padding: '0',
        backgroundColor: 'transparent',
        border: 'none',
        color: '#858585',
        fontSize: '24px',
        cursor: 'pointer',
        borderRadius: '6px',
        transition: 'all 0.15s ease',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
    },
    body: {
        flex: 1,
        overflowY: 'auto' as const,
        padding: '24px',
    },
    loading: {
        textAlign: 'center',
        padding: '60px',
        color: '#858585',
        fontSize: '14px',
    },
    fieldGroup: {
        marginBottom: '20px',
    },
    label: {
        display: 'block',
        fontSize: '13px',
        color: '#cccccc',
        marginBottom: '8px',
        fontWeight: 500,
        letterSpacing: '0.01em',
    },
    input: {
        width: '100%',
        padding: '10px 14px',
        backgroundColor: '#252526',
        border: '1px solid #3e3e42',
        borderRadius: '6px',
        color: '#ffffff',
        fontSize: '14px',
        boxSizing: 'border-box' as const,
        transition: 'all 0.15s ease',
    },
    commandsSection: {
        marginTop: '24px',
    },
    sectionHeader: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        marginBottom: '16px',
    },
    sectionTitleGroup: {
        display: 'flex',
        alignItems: 'center',
        gap: '12px',
    },
    sectionTitle: {
        fontSize: '15px',
        fontWeight: 600,
        color: '#ffffff',
        letterSpacing: '-0.01em',
    },
    sectionSubtitle: {
        fontSize: '12px',
        color: '#858585',
        backgroundColor: '#2d2d2d',
        padding: '2px 8px',
        borderRadius: '12px',
        fontWeight: 500,
    },
    addButton: {
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
        padding: '8px 16px',
        backgroundColor: '#007acc',
        color: '#ffffff',
        border: 'none',
        borderRadius: '6px',
        cursor: 'pointer',
        fontSize: '13px',
        fontWeight: 500,
        transition: 'all 0.15s ease',
    },
    addIcon: {
        fontSize: '16px',
        lineHeight: 1,
    },
    addText: {
        lineHeight: 1,
    },
    commandsList: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '12px',
    },
    commandCard: {
        display: 'flex',
        gap: '12px',
        padding: '16px',
        backgroundColor: '#252526',
        border: '1px solid #3e3e42',
        borderRadius: '8px',
        transition: 'all 0.15s ease',
        cursor: 'default',
    },
    commandContent: {
        flex: 1,
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '10px',
        minWidth: 0,
    },
    commandInput: {
        width: '100%',
        padding: '10px 14px',
        backgroundColor: '#1e1e1e',
        border: '1px solid #3e3e42',
        borderRadius: '6px',
        color: '#ffffff',
        fontSize: '14px',
        fontFamily: 'Consolas, "JetBrains Mono", Monaco, monospace',
        boxSizing: 'border-box' as const,
        transition: 'all 0.15s ease',
    },
    commandMetadata: {
        display: 'flex',
        gap: '10px',
        alignItems: 'center',
    },
    metadataItem: {
        display: 'flex',
        alignItems: 'center',
    },
    metadataLabel: {
        fontSize: '11px',
        color: '#858585',
        fontWeight: 500,
        textTransform: 'uppercase' as const,
        letterSpacing: '0.05em',
    },
    metadataSpacer: {
        flex: 1,
    },
    metadataInput: {
        width: '180px',
        padding: '6px 10px',
        backgroundColor: '#1e1e1e',
        border: '1px solid #3e3e42',
        borderRadius: '4px',
        color: '#cccccc',
        fontSize: '12px',
        boxSizing: 'border-box' as const,
        transition: 'all 0.15s ease',
    },
    delayInputGroup: {
        display: 'flex',
        alignItems: 'center',
        backgroundColor: '#1e1e1e',
        border: '1px solid #3e3e42',
        borderRadius: '4px',
        overflow: 'hidden',
    },
    delayInput: {
        width: '70px',
        padding: '6px 8px',
        backgroundColor: 'transparent',
        border: 'none',
        color: '#ffffff',
        fontSize: '12px',
        boxSizing: 'border-box' as const,
        outline: 'none',
    },
    delayUnit: {
        padding: '0 8px',
        fontSize: '11px',
        color: '#858585',
        backgroundColor: '#2d2d2d',
        height: '100%',
        display: 'flex',
        alignItems: 'center',
    },
    switchLabel: {
        display: 'flex',
        alignItems: 'center',
        cursor: 'pointer',
        userSelect: 'none' as const,
    },
    switchCheckbox: {
        display: 'none',
    },
    switchSlider: {
        width: '36px',
        height: '20px',
        borderRadius: '10px',
        position: 'relative' as const,
        transition: 'all 0.2s ease',
    },
    switchText: {
        display: 'none',
    },
    commandActions: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '8px',
        paddingTop: '2px',
    },
    actionButton: {
        width: '32px',
        height: '32px',
        padding: '0',
        backgroundColor: 'transparent',
        border: 'none',
        color: '#858585',
        borderRadius: '6px',
        cursor: 'pointer',
        transition: 'all 0.15s ease',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
    },
    footer: {
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '12px',
        padding: '20px 24px',
        borderTop: '1px solid #3e3e42',
    },
    cancelButton: {
        padding: '10px 20px',
        backgroundColor: 'transparent',
        color: '#cccccc',
        border: '1px solid #4d4d4d',
        borderRadius: '6px',
        cursor: 'pointer',
        fontSize: '14px',
        fontWeight: 500,
        transition: 'all 0.15s ease',
    },
    saveButton: {
        padding: '10px 20px',
        backgroundColor: '#007acc',
        color: '#ffffff',
        border: 'none',
        borderRadius: '6px',
        cursor: 'pointer',
        fontSize: '14px',
        fontWeight: 500,
        transition: 'all 0.15s ease',
    },
};

export default ScriptEditorModal;
