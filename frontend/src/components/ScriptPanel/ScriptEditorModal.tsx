import React, { useEffect, useState } from 'react';

// --- Types ---

interface ScriptVariable {
    name: string;
    display_name: string;
    default_value: string;
    required: boolean;
    description: string;
}

interface ScriptStep {
    command?: string;
    comment?: string;
    delay?: number;
    enabled: boolean;
    original_index?: number;
}

interface Script {
    id: string;
    name: string;
    description: string;
    commands: Array<{
        index: number;
        content: string;
        comment: string;
        delay: number;
        enabled: boolean;
    }>;
    variables: ScriptVariable[];
    steps: ScriptStep[];
}

interface ScriptEditorModalProps {
    isOpen: boolean;
    scriptId: string | null;
    onClose: () => void;
    onSave: () => void;
}

// --- Main Component ---

const ScriptEditorModal: React.FC<ScriptEditorModalProps> = ({
    isOpen,
    scriptId,
    onClose,
    onSave,
}) => {
    const [script, setScript] = useState<Script | null>(null);
    const [loading, setLoading] = useState(false);
    const [saving, setSaving] = useState(false);
    const [varsExpanded, setVarsExpanded] = useState(true);

    // Inject CSS
    useEffect(() => {
        const style = document.createElement('style');
        style.textContent = `
            @keyframes scriptEditorFadeIn { from { opacity: 0; } to { opacity: 1; } }
            @keyframes scriptEditorSlideUp {
                from { opacity: 0; transform: translateY(20px) scale(0.98); }
                to { opacity: 1; transform: translateY(0) scale(1); }
            }
            .se-switch-slider { background-color: #424242; }
            .se-switch-checkbox:checked + .se-switch-slider { background-color: #4ade80; }
            .se-switch-checkbox:checked + .se-switch-slider::after { transform: translateX(16px); }
            .se-switch-slider::after {
                content: ''; position: absolute; top: 2px; left: 2px;
                width: 16px; height: 16px; background-color: white;
                border-radius: 50%; transition: transform 0.2s ease;
            }
            .se-input:focus { outline: none; border-color: #007acc !important; box-shadow: 0 0 0 2px rgba(0, 122, 204, 0.2); }
            .se-btn:hover { transform: translateY(-1px); }
            .se-btn-action:hover { background-color: #2d2d2d; color: #ffffff; }
            .se-btn-close:hover { background-color: #2d2d2d; color: #ffffff; }
            .se-btn-add:hover { background-color: #0069b4; box-shadow: 0 4px 12px rgba(0, 122, 204, 0.4); }
            .se-btn-save:hover { background-color: #0069b4; box-shadow: 0 4px 12px rgba(0, 122, 204, 0.4); }
            .se-btn-cancel:hover { background-color: #3e3e42; border-color: #5a5a5a; }
            .se-command-card:hover { border-color: #5a5a5a; box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3); }
            .se-meta-input:hover, .se-delay-group:hover { border-color: #5a5a5a; }
        `;
        document.head.appendChild(style);
        return () => { try { document.head.removeChild(style); } catch {} };
    }, []);

    useEffect(() => {
        if (isOpen && scriptId) loadScript();
    }, [isOpen, scriptId]);

    const loadScript = async () => {
        if (!scriptId) return;
        setLoading(true);
        try {
            // @ts-ignore
            const result = await window.go.main.App.LoadScript(scriptId);
            if (!result.steps || result.steps.length === 0) {
                result.steps = (result.commands || []).map((cmd: any) => ({
                    command: cmd.content,
                    comment: cmd.comment || '',
                    delay: cmd.delay || 0,
                    enabled: cmd.enabled !== false,
                    original_index: cmd.index,
                }));
            }
            if (!result.variables) result.variables = [];
            setScript(result);
        } catch (err: any) {
            alert('加载脚本失败: ' + (err.message || err));
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
            alert('保存失败: ' + (err.message || err));
        } finally {
            setSaving(false);
        }
    };

    // --- Step management ---

    const updateSteps = (newSteps: ScriptStep[]) => {
        setScript((prev) => prev ? { ...prev, steps: newSteps } : prev);
    };

    const updateStep = (index: number, update: Partial<ScriptStep>) => {
        if (!script) return;
        const newSteps = [...script.steps];
        newSteps[index] = { ...newSteps[index], ...update };
        updateSteps(newSteps);
    };

    const addCommandStep = () => {
        if (!script) return;
        updateSteps([...script.steps, {
            command: '',
            comment: '',
            delay: 0,
            enabled: true,
        }]);
    };

    const moveStep = (index: number, direction: -1 | 1) => {
        if (!script) return;
        const target = index + direction;
        if (target < 0 || target >= script.steps.length) return;
        const newSteps = [...script.steps];
        [newSteps[index], newSteps[target]] = [newSteps[target], newSteps[index]];
        updateSteps(newSteps);
    };

    const deleteStep = (index: number) => {
        if (!script) return;
        if (!confirm('确定要删除这条命令吗？')) return;
        updateSteps(script.steps.filter((_, i) => i !== index));
    };

    // --- Variable management ---

    const updateVariables = (newVars: ScriptVariable[]) => {
        setScript((prev) => prev ? { ...prev, variables: newVars } : prev);
    };

    const addVariable = () => {
        updateVariables([...(script?.variables || []), {
            name: '',
            display_name: '',
            default_value: '',
            required: false,
            description: '',
        }]);
    };

    const updateVariable = (index: number, update: Partial<ScriptVariable>) => {
        if (!script) return;
        const newVars = [...script.variables];
        newVars[index] = { ...newVars[index], ...update };
        updateVariables(newVars);
    };

    const deleteVariable = (index: number) => {
        if (!confirm('确定要删除这个变量吗？')) return;
        updateVariables(script!.variables.filter((_, i) => i !== index));
    };

    // --- Render ---

    if (!isOpen) return null;

    if (loading) {
        return (
            <div style={styles.overlay}>
                <div style={styles.modal}><div style={styles.loading}>加载中...</div></div>
            </div>
        );
    }

    if (!script) return null;

    return (
        <div style={styles.overlay}>
            <div style={styles.modal}>
                <div style={styles.header}>
                    <h2 style={styles.title}>编辑脚本</h2>
                    <button className="se-btn se-btn-close" style={styles.closeButton} onClick={onClose}>x</button>
                </div>

                <div style={styles.body}>
                    {/* 基本信息 */}
                    <div style={styles.fieldGroup}>
                        <label style={styles.label}>脚本名称</label>
                        <input className="se-input" style={styles.input} type="text" value={script.name}
                            onChange={(e) => setScript({ ...script, name: e.target.value })}
                            placeholder="例如：重启 Nginx 服务" />
                    </div>
                    <div style={styles.fieldGroup}>
                        <label style={styles.label}>描述说明</label>
                        <input className="se-input" style={styles.input} type="text" value={script.description}
                            onChange={(e) => setScript({ ...script, description: e.target.value })}
                            placeholder="简要描述脚本用途" />
                    </div>

                    {/* 变量定义区 */}
                    <div style={styles.variablesSection}>
                        <div style={styles.varSectionHeader} onClick={() => setVarsExpanded(!varsExpanded)}>
                            <div style={styles.sectionTitleGroup}>
                                <label style={styles.sectionTitle}>
                                    变量定义 {varsExpanded ? '\u25BC' : '\u25B8'}
                                </label>
                                <span style={styles.sectionSubtitle}>{script.variables?.length || 0} 个变量</span>
                            </div>
                        </div>
                        {varsExpanded && (
                            <div style={styles.variablesContent}>
                                {/* 表头 */}
                                <div style={styles.varHeader}>
                                    <span style={{...styles.varHeaderCell, flex: 2}}>变量名</span>
                                    <span style={{...styles.varHeaderCell, flex: 2}}>显示名称</span>
                                    <span style={{...styles.varHeaderCell, flex: 2}}>默认值</span>
                                    <span style={{...styles.varHeaderCell, width: '36px', textAlign: 'center'}}>必填</span>
                                    <span style={{...styles.varHeaderCell, width: '24px'}}></span>
                                </div>
                                {(script.variables || []).map((v, idx) => (
                                    <div key={idx} style={styles.varRow}>
                                        <input className="se-input" style={{...styles.varInput, flex: 2}} type="text" value={v.name}
                                            onChange={(e) => updateVariable(idx, { name: e.target.value })}
                                            placeholder="如 port" />
                                        <input className="se-input" style={{...styles.varInput, flex: 2}} type="text" value={v.display_name}
                                            onChange={(e) => updateVariable(idx, { display_name: e.target.value })}
                                            placeholder="如 端口号" />
                                        <input className="se-input" style={{...styles.varInput, flex: 2}} type="text" value={v.default_value}
                                            onChange={(e) => updateVariable(idx, { default_value: e.target.value })}
                                            placeholder="如 8080" />
                                        <label style={styles.switchLabel} title={v.required ? '回放时必填' : '可选'}>
                                            <input type="checkbox" checked={v.required}
                                                onChange={(e) => updateVariable(idx, { required: e.target.checked })}
                                                className="se-switch-checkbox" style={styles.switchCheckbox} />
                                            <span className="se-switch-slider" style={styles.switchSlider}></span>
                                        </label>
                                        <button className="se-btn se-btn-action" style={styles.varDeleteBtn}
                                            onClick={() => deleteVariable(idx)} title="删除变量">
                                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                                                <path d="M18 6L6 18M6 6l12 12" />
                                            </svg>
                                        </button>
                                    </div>
                                ))}
                                {(!script.variables || script.variables.length === 0) && (
                                    <div style={styles.varEmpty}>暂无变量，点击下方按钮添加</div>
                                )}
                                <button className="se-btn se-btn-add" style={styles.addVarButton} onClick={addVariable}>
                                    + 添加变量
                                </button>
                            </div>
                        )}
                    </div>

                    {/* 命令步骤区 */}
                    <div style={styles.commandsSection}>
                        <div style={styles.sectionHeader}>
                            <div style={styles.sectionTitleGroup}>
                                <label style={styles.sectionTitle}>命令列表</label>
                                <span style={styles.sectionSubtitle}>{script.steps.length} 条命令</span>
                            </div>
                            <button className="se-btn se-btn-add" style={styles.addButtonStyle} onClick={addCommandStep}>
                                + 添加命令
                            </button>
                        </div>

                        <div style={styles.commandsList}>
                            {script.steps.map((step, idx) => (
                                <div key={idx} className="se-command-card" style={{
                                    ...styles.commandCard,
                                    opacity: step.enabled ? 1 : 0.5,
                                }}>
                                    {/* 序号 */}
                                    <div style={styles.stepIndex}>{idx + 1}</div>

                                    <div style={styles.commandContent}>
                                        <input className="se-input" style={styles.commandInput} type="text" value={step.command || ''}
                                            onChange={(e) => updateStep(idx, { command: e.target.value })}
                                            placeholder="输入命令，用 ${变量名} 引用变量" />
                                        <div style={styles.commandMetadata}>
                                            <input className="se-input se-meta-input" style={styles.metadataInput} type="text"
                                                value={step.comment || ''} onChange={(e) => updateStep(idx, { comment: e.target.value })}
                                                placeholder="备注说明" />
                                            <div className="se-delay-group" style={styles.delayInputGroup}>
                                                <input className="se-input" style={styles.delayInput} type="number" value={step.delay || 0}
                                                    onChange={(e) => updateStep(idx, { delay: parseInt(e.target.value) || 0 })}
                                                    placeholder="0" min="0" step="100" />
                                                <span style={styles.delayUnit}>ms</span>
                                            </div>
                                            <label style={styles.switchLabel} title={step.enabled ? '已启用' : '已禁用'}>
                                                <input type="checkbox" checked={step.enabled}
                                                    onChange={(e) => updateStep(idx, { enabled: e.target.checked })}
                                                    className="se-switch-checkbox" style={styles.switchCheckbox} />
                                                <span className="se-switch-slider" style={styles.switchSlider}></span>
                                            </label>
                                        </div>
                                    </div>

                                    <div style={styles.commandActions}>
                                        <button className="se-btn se-btn-action" style={styles.moveButton}
                                            onClick={() => moveStep(idx, -1)} title="上移"
                                            disabled={idx === 0}>
                                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                                                <path d="M18 15l-6-6-6 6"/>
                                            </svg>
                                        </button>
                                        <button className="se-btn se-btn-action" style={styles.moveButton}
                                            onClick={() => moveStep(idx, 1)} title="下移"
                                            disabled={idx === script.steps.length - 1}>
                                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                                                <path d="M6 9l6 6 6-6"/>
                                            </svg>
                                        </button>
                                        <button className="se-btn se-btn-action" style={styles.actionButton} onClick={() => deleteStep(idx)} title="删除命令">
                                            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                                                <path d="M3 6h18M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2" />
                                            </svg>
                                        </button>
                                    </div>
                                </div>
                            ))}
                        </div>
                    </div>
                </div>

                <div style={styles.footer}>
                    <button className="se-btn se-btn-cancel" style={styles.cancelButton} onClick={onClose}>
                        取消
                    </button>
                    <button className="se-btn se-btn-save" style={styles.saveButton} onClick={handleSave} disabled={saving}>
                        {saving ? '保存中...' : '保存'}
                    </button>
                </div>
            </div>
        </div>
    );
};

// --- Styles ---

const styles: Record<string, React.CSSProperties> = {
    overlay: {
        position: 'fixed' as const, top: 0, left: 0, right: 0, bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.75)', backdropFilter: 'blur(4px)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        zIndex: 5000, animation: 'scriptEditorFadeIn 0.2s ease-out',
    },
    modal: {
        width: '960px', maxHeight: '85vh', backgroundColor: '#1e1e1e',
        borderRadius: '12px', border: '1px solid #3e3e42',
        boxShadow: '0 20px 60px rgba(0, 0, 0, 0.5)',
        display: 'flex', flexDirection: 'column' as const,
        animation: 'scriptEditorSlideUp 0.3s ease-out',
    },
    header: {
        display: 'flex', justifyContent: 'space-between', alignItems: 'center',
        padding: '20px 24px', borderBottom: '1px solid #3e3e42',
    },
    title: { margin: 0, fontSize: '18px', fontWeight: 600, color: '#ffffff' },
    closeButton: {
        width: '36px', height: '36px', padding: 0, backgroundColor: 'transparent',
        border: 'none', color: '#858585', fontSize: '24px', cursor: 'pointer',
        borderRadius: '6px', display: 'flex', alignItems: 'center', justifyContent: 'center',
    },
    body: { flex: 1, overflowY: 'auto' as const, padding: '24px' },
    loading: { textAlign: 'center', padding: '60px', color: '#858585', fontSize: '14px' },
    fieldGroup: { marginBottom: '20px' },
    label: { display: 'block', fontSize: '13px', color: '#cccccc', marginBottom: '8px', fontWeight: 500 },
    input: {
        width: '100%', padding: '10px 14px', backgroundColor: '#252526',
        border: '1px solid #3e3e42', borderRadius: '6px', color: '#ffffff',
        fontSize: '14px', boxSizing: 'border-box' as const,
    },

    // 变量区
    variablesSection: { marginTop: '20px', border: '1px solid #3e3e42', borderRadius: '8px', overflow: 'hidden' },
    varSectionHeader: {
        display: 'flex', justifyContent: 'space-between', alignItems: 'center',
        padding: '12px 16px', backgroundColor: '#252526', cursor: 'pointer', userSelect: 'none' as const,
    },
    variablesContent: { padding: '12px 16px', backgroundColor: '#1e1e1e' },
    varHeader: {
        display: 'flex', gap: '6px', alignItems: 'center',
        marginBottom: '8px', paddingBottom: '6px', borderBottom: '1px solid #333',
    },
    varHeaderCell: { fontSize: '11px', color: '#757575', fontWeight: 600, letterSpacing: '0.03em' },
    varRow: { display: 'flex', gap: '6px', alignItems: 'center', marginBottom: '6px' },
    varInput: {
        padding: '6px 8px', backgroundColor: '#252526', border: '1px solid #3e3e42',
        borderRadius: '4px', color: '#ffffff', fontSize: '12px', minWidth: 0,
    },
    varEmpty: { textAlign: 'center' as const, color: '#757575', fontSize: '12px', padding: '12px 0' },
    varDeleteBtn: {
        width: '24px', height: '24px', padding: 0, backgroundColor: 'transparent',
        border: 'none', color: '#858585', cursor: 'pointer', fontSize: '14px',
        borderRadius: '4px', display: 'flex', alignItems: 'center', justifyContent: 'center',
    },
    addVarButton: {
        padding: '6px 12px', backgroundColor: '#007acc', color: '#ffffff',
        border: 'none', borderRadius: '4px', cursor: 'pointer', fontSize: '12px', fontWeight: 500, marginTop: '4px',
    },
    switchLabel: { display: 'flex', alignItems: 'center', justifyContent: 'center', cursor: 'pointer', userSelect: 'none' as const },
    switchCheckbox: { display: 'none' },
    switchSlider: { width: '36px', height: '20px', borderRadius: '10px', position: 'relative' as const, transition: 'all 0.2s ease' },

    // 命令区
    commandsSection: { marginTop: '24px' },
    sectionHeader: {
        display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px',
    },
    sectionTitleGroup: { display: 'flex', alignItems: 'center', gap: '12px' },
    sectionTitle: { fontSize: '15px', fontWeight: 600, color: '#ffffff' },
    sectionSubtitle: {
        fontSize: '12px', color: '#858585', backgroundColor: '#2d2d2d',
        padding: '2px 8px', borderRadius: '12px', fontWeight: 500,
    },
    addButtonStyle: {
        padding: '8px 16px', backgroundColor: '#007acc', color: '#ffffff',
        border: 'none', borderRadius: '6px', cursor: 'pointer',
        fontSize: '13px', fontWeight: 500,
    },
    commandsList: { display: 'flex', flexDirection: 'column' as const, gap: '8px' },

    // 命令卡片
    commandCard: {
        display: 'flex', gap: '10px', padding: '12px 16px', backgroundColor: '#252526',
        border: '1px solid #3e3e42', borderRadius: '8px', alignItems: 'flex-start',
        transition: 'all 0.15s ease',
    },
    stepIndex: {
        width: '24px', height: '24px', minWidth: '24px',
        backgroundColor: '#3e3e42', borderRadius: '50%',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        fontSize: '11px', fontWeight: 700, color: '#b0b0b0', marginTop: '8px',
    },
    commandContent: { flex: 1, display: 'flex', flexDirection: 'column' as const, gap: '8px', minWidth: 0 },
    commandInput: {
        width: '100%', padding: '10px 14px', backgroundColor: '#1e1e1e',
        border: '1px solid #3e3e42', borderRadius: '6px', color: '#ffffff',
        fontSize: '14px', fontFamily: 'var(--font-mono)', boxSizing: 'border-box' as const,
    },
    commandMetadata: { display: 'flex', gap: '10px', alignItems: 'center' },
    metadataInput: {
        width: '200px', padding: '6px 10px', backgroundColor: '#1e1e1e',
        border: '1px solid #3e3e42', borderRadius: '4px', color: '#cccccc',
        fontSize: '12px', boxSizing: 'border-box' as const,
    },
    delayInputGroup: {
        display: 'flex', alignItems: 'center', backgroundColor: '#1e1e1e',
        border: '1px solid #3e3e42', borderRadius: '4px', overflow: 'hidden',
    },
    delayInput: {
        width: '60px', padding: '6px 8px', backgroundColor: 'transparent',
        border: 'none', color: '#ffffff', fontSize: '12px', boxSizing: 'border-box' as const, outline: 'none',
    },
    delayUnit: { padding: '0 8px', fontSize: '11px', color: '#858585', backgroundColor: '#2d2d2d', height: '100%', display: 'flex', alignItems: 'center' },
    metadataSpacer: { flex: 1 },
    commandActions: { display: 'flex', gap: '2px', paddingTop: '6px' },
    moveButton: {
        width: '28px', height: '28px', padding: 0, backgroundColor: 'transparent',
        border: 'none', color: '#666', borderRadius: '4px', cursor: 'pointer',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
    },
    actionButton: {
        width: '28px', height: '28px', padding: 0, backgroundColor: 'transparent',
        border: 'none', color: '#858585', borderRadius: '4px', cursor: 'pointer',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
    },

    // 底部
    footer: {
        display: 'flex', justifyContent: 'flex-end', gap: '12px',
        padding: '20px 24px', borderTop: '1px solid #3e3e42',
    },
    cancelButton: {
        padding: '10px 20px', backgroundColor: 'transparent', color: '#cccccc',
        border: '1px solid #4d4d4d', borderRadius: '6px', cursor: 'pointer',
        fontSize: '14px', fontWeight: 500,
    },
    saveButton: {
        padding: '10px 20px', backgroundColor: '#007acc', color: '#ffffff',
        border: 'none', borderRadius: '6px', cursor: 'pointer',
        fontSize: '14px', fontWeight: 500,
    },
};

export default ScriptEditorModal;
