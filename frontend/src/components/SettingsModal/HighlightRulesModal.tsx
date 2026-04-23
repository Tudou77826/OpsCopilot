import React, { useMemo, useState, useEffect } from 'react';
import { HighlightRule } from '../Terminal/highlightTypes';

interface HighlightRulesModalProps {
    isOpen: boolean;
    rules: HighlightRule[];
    onChange: (rules: HighlightRule[]) => void;
    onSave?: (rules: HighlightRule[]) => Promise<void>;
    onClose: () => void;
}

type RiskLevel = 'safe' | 'moderate' | 'high' | 'severe';

interface PatternRisk {
    level: RiskLevel;
    issues: string[];
    canEnable: boolean;
}

function newId() {
    // @ts-ignore
    if (globalThis.crypto && globalThis.crypto.randomUUID) {
        // @ts-ignore
        return globalThis.crypto.randomUUID();
    }
    return `r_${Math.random().toString(16).slice(2)}_${Date.now()}`;
}

function assessPatternRisk(pattern: string): PatternRisk {
    const p = pattern.trim();
    if (!p) return { level: 'safe', issues: [], canEnable: true };

    const issues: string[] = [];
    let level: RiskLevel = 'safe';

    if (p.length > 500) {
        issues.push(`正则非常长 (${p.length}字符)`);
        level = 'high';
    } else if (p.length > 200) {
        issues.push(`正则较长 (${p.length}字符)`);
        level = 'moderate';
    }

    if (/\(\.\*\)\+/.test(p) || /\(\.\+\)\+/.test(p)) {
        issues.push('包含嵌套量词，可能导致指数级匹配');
        if (level === 'safe') {
            level = 'high';
        } else if (level === 'moderate') {
            level = 'severe';
        }
    }

    if (/^(\.\*|\.\+)[+*]/.test(p)) {
        issues.push('灾难性回溯模式');
        level = 'severe';
    }

    if (/\(\?:?[^)]*[+*][^)]*\)[+*]/.test(p)) {
        issues.push('复杂的嵌套量词组合');
        if (level === 'safe') {
            level = 'moderate';
        }
    }

    return {
        level,
        issues,
        canEnable: level !== 'severe'
    };
}

function areRulesEqual(a: HighlightRule[], b: HighlightRule[]): boolean {
    if (a.length !== b.length) return false;
    for (let i = 0; i < a.length; i++) {
        if (a[i].id !== b[i].id) return false;
        if (a[i].name !== b[i].name) return false;
        if (a[i].pattern !== b[i].pattern) return false;
        if (a[i].is_enabled !== b[i].is_enabled) return false;
        if (a[i].priority !== b[i].priority) return false;
        if (JSON.stringify(a[i].style) !== JSON.stringify(b[i].style)) return false;
    }
    return true;
}

interface UnsavedChangesModalProps {
    isOpen: boolean;
    changedCount: number;
    onSave: () => void;
    onDiscard: () => void;
    onCancel: () => void;
}

function UnsavedChangesModal({ isOpen, changedCount, onSave, onDiscard, onCancel }: UnsavedChangesModalProps) {
    if (!isOpen) return null;

    return (
        <div style={unsavedStyles.overlay}>
            <div style={unsavedStyles.modal}>
                <h3 style={unsavedStyles.title}>确认关闭？</h3>
                <p style={unsavedStyles.message}>
                    您有 {changedCount} 条规则的更改尚未保存。
                </p>
                <div style={unsavedStyles.actions}>
                    <button style={unsavedStyles.cancelBtn} onClick={onCancel}>
                        继续编辑
                    </button>
                    <button style={unsavedStyles.discardBtn} onClick={onDiscard}>
                        放弃更改
                    </button>
                    <button style={unsavedStyles.saveBtn} onClick={onSave}>
                        保存并关闭
                    </button>
                </div>
            </div>
        </div>
    );
}

export default function HighlightRulesModal({ isOpen, rules, onChange, onSave, onClose }: HighlightRulesModalProps) {
    const [draft, setDraft] = useState<HighlightRule[]>(rules);
    const [editingId, setEditingId] = useState<string | null>(null);
    const [hoveredBgOption, setHoveredBgOption] = useState<string | null>(null);
    const [isDirty, setIsDirty] = useState(false);
    const [showUnsavedWarning, setShowUnsavedWarning] = useState(false);
    const [riskAcknowledged, setRiskAcknowledged] = useState<Record<string, boolean>>({});

    useEffect(() => {
        if (isOpen) {
            setDraft(rules);
            setEditingId(null);
            setRiskAcknowledged({});
        }
    }, [isOpen, rules]);

    useEffect(() => {
        const hasChanges = !areRulesEqual(draft, rules);
        setIsDirty(hasChanges);
    }, [draft, rules]);

    const sorted = useMemo(() => {
        return [...draft].sort((a, b) => (a.priority ?? 0) - (b.priority ?? 0));
    }, [draft]);

    if (!isOpen) return null;

    const update = (next: HighlightRule[]) => {
        setDraft(next);
    };

    const applyChanges = async () => {
        if (onSave) {
            try {
                await onSave(sorted);
            } catch {
                return;
            }
        } else {
            onChange(sorted);
        }
        setIsDirty(false);
    };

    const applyAndClose = async () => {
        if (onSave) {
            try {
                await onSave(sorted);
            } catch {
                return;
            }
        } else {
            onChange(sorted);
        }
        onClose();
    };

    const handleXClick = () => {
        if (isDirty) {
            setShowUnsavedWarning(true);
        } else {
            onClose();
        }
    };

    const addRule = () => {
        const r: HighlightRule = {
            id: newId(),
            name: '新规则',
            pattern: '',
            is_enabled: false,
            priority: sorted.length > 0 ? (sorted[sorted.length - 1].priority + 10) : 10,
            style: { background_color: '#1d3a5a', color: '#ffffff', font_weight: 'bold' }
        };
        setDraft([...sorted, r]);
        setEditingId(r.id);
    };

    const removeRule = (id: string) => {
        update(sorted.filter(r => r.id !== id));
        if (editingId === id) setEditingId(null);
    };

    const move = (id: string, dir: -1 | 1) => {
        const idx = sorted.findIndex(r => r.id === id);
        if (idx < 0) return;
        const j = idx + dir;
        if (j < 0 || j >= sorted.length) return;
        const a = sorted[idx];
        const b = sorted[j];
        const next = [...sorted];
        next[idx] = { ...b, priority: a.priority };
        next[j] = { ...a, priority: b.priority };
        update(next);
    };

    const patch = (id: string, partial: Partial<HighlightRule>) => {
        update(sorted.map(r => (r.id === id ? { ...r, ...partial } : r)));
    };

    const patchStyle = (id: string, partial: Partial<HighlightRule['style']>) => {
        update(sorted.map(r => {
            if (r.id !== id) return r;
            const current = { ...(r.style || {}) };
            if ('background_color' in partial) {
                const v = partial.background_color;
                if (!v) {
                    delete (current as any).background_color;
                } else {
                    (current as any).background_color = v;
                }
            }
            if ('color' in partial && partial.color !== undefined) {
                (current as any).color = partial.color;
            }
            if ('font_weight' in partial && partial.font_weight !== undefined) {
                (current as any).font_weight = partial.font_weight;
            }
            if ('text_decoration' in partial && partial.text_decoration !== undefined) {
                (current as any).text_decoration = partial.text_decoration;
            }
            if ('opacity' in partial && partial.opacity !== undefined) {
                (current as any).opacity = partial.opacity;
            }
            return { ...r, style: current };
        }));
    };

    const isEditing = (id: string) => editingId === id;

    const getChangedCount = () => {
        let count = 0;
        for (const d of draft) {
            const original = rules.find(r => r.id === d.id);
            if (!original) {
                count++;
            } else {
                if (d.name !== original.name || d.pattern !== original.pattern ||
                    d.is_enabled !== original.is_enabled ||
                    JSON.stringify(d.style) !== JSON.stringify(original.style)) {
                    count++;
                }
            }
        }
        return count;
    };

    return (
        <>
            <div style={styles.overlay}>
                <div style={styles.modal} onClick={(e) => e.stopPropagation()}>
                    {/* Header */}
                    <div style={styles.header}>
                        <div style={styles.titleContainer}>
                            <h2 style={styles.title}>突出显示集</h2>
                            {isDirty && <span style={styles.unsavedIndicator}>● 未保存</span>}
                        </div>
                        <button onClick={handleXClick} style={styles.closeBtn}>×</button>
                    </div>

                    {/* Body */}
                    <div style={styles.body}>
                        {/* Toolbar */}
                        <div style={styles.toolbar}>
                            <button style={styles.primaryButton} onClick={addRule}>+ 新建规则</button>
                            <div style={styles.hint}>
                                {isDirty ? `提示: 有 ${getChangedCount()} 条未保存更改` : '优先级越小越先匹配'}
                            </div>
                        </div>

                        {/* Rules List */}
                        <div style={styles.list}>
                            {sorted.length === 0 && <div style={styles.empty}>暂无规则，点击上方按钮添加</div>}
                            {sorted.map((r, i) => {
                                const risk = assessPatternRisk(r.pattern);
                                const canEnable = risk.canEnable && (risk.level === 'safe' || riskAcknowledged[r.id]);
                                const editing = isEditing(r.id);

                                return (
                                    <div key={r.id} style={styles.item}>
                                        {/* Item Header */}
                                        <div style={styles.itemHeader}>
                                            <div style={styles.itemLeft}>
                                                <div style={styles.ruleInfo}>
                                                    {/* Enable Switch */}
                                                    <label style={styles.switch}>
                                                        <input
                                                            type="checkbox"
                                                            checked={!!r.is_enabled}
                                                            onChange={() => patch(r.id, { is_enabled: !r.is_enabled && canEnable })}
                                                            disabled={!canEnable}
                                                        />
                                                        <span style={{
                                                            ...styles.slider,
                                                            ...(risk.level !== 'safe' ? { backgroundColor: '#ff9800' } : {})
                                                        }}></span>
                                                    </label>
                                                    <span style={styles.nameText}>{r.name || '未命名'}</span>
                                                    {risk.level !== 'safe' && (
                                                        <span style={{
                                                            ...styles.riskBadge,
                                                            backgroundColor: risk.level === 'moderate' ? '#7a5c2e' :
                                                                           risk.level === 'high' ? '#5c2e2e' :
                                                                           '#4a1a1a',
                                                            color: risk.level === 'moderate' ? '#ffcc80' :
                                                                   risk.level === 'high' ? '#ff9980' :
                                                                   '#ff8080'
                                                        }}>
                                                            {risk.level === 'moderate' ? '中等风险' :
                                                             risk.level === 'high' ? '高风险' :
                                                             '严重风险'}
                                                        </span>
                                                    )}
                                                </div>
                                                <div style={styles.statusText}>
                                                    {r.is_enabled ? '已启用' : '已禁用'}
                                                </div>
                                            </div>
                                            <div style={styles.actions}>
                                                <button
                                                    style={styles.editBtn}
                                                    onClick={() => setEditingId(editing ? null : r.id)}
                                                    title={editing ? '收起编辑' : '展开编辑'}
                                                >
                                                    <span>{editing ? '▾' : '▸'}</span>
                                                    <span>{editing ? '收起' : '编辑'}</span>
                                                </button>
                                                <button
                                                    style={styles.iconBtn}
                                                    onClick={() => move(r.id, -1)}
                                                    disabled={i === 0}
                                                    title="上移"
                                                >
                                                    ↑
                                                </button>
                                                <button
                                                    style={styles.iconBtn}
                                                    onClick={() => move(r.id, 1)}
                                                    disabled={i === sorted.length - 1}
                                                    title="下移"
                                                >
                                                    ↓
                                                </button>
                                                <button
                                                    style={styles.iconBtn}
                                                    onClick={() => removeRule(r.id)}
                                                    title="删除"
                                                >
                                                    ×
                                                </button>
                                            </div>
                                        </div>

                                        {/* Expanded Edit Area */}
                                        {editing && (
                                            <div style={styles.expanded}>
                                                <div style={styles.field}>
                                                    <label style={styles.fieldLabel}>规则名称</label>
                                                    <input
                                                        value={r.name}
                                                        onChange={(e) => patch(r.id, { name: e.target.value })}
                                                        style={styles.input}
                                                        placeholder="例如：错误信息"
                                                    />
                                                </div>

                                                <div style={styles.field}>
                                                    <label style={styles.fieldLabel}>匹配模式（正则表达式）</label>
                                                    <input
                                                        value={r.pattern}
                                                        onChange={(e) => {
                                                            patch(r.id, { pattern: e.target.value });
                                                        }}
                                                        style={styles.input}
                                                        placeholder="例如：(?i)\\b(error|fail)\\b"
                                                    />
                                                    {risk.level !== 'safe' && (
                                                        <div style={{
                                                            ...styles.warningBox,
                                                            backgroundColor: risk.level === 'moderate' ? 'rgba(255, 152, 0, 0.1)' :
                                                                             risk.level === 'high' ? 'rgba(255, 87, 34, 0.1)' :
                                                                             'rgba(211, 47, 47, 0.1)',
                                                            borderColor: risk.level === 'moderate' ? '#ff9800' :
                                                                          risk.level === 'high' ? '#ff5722' :
                                                                          '#d32f2f'
                                                        }}>
                                                            <div style={styles.warningHeader}>
                                                                <span style={styles.warningIcon}>
                                                                    {risk.level === 'moderate' ? '⚠️' :
                                                                     risk.level === 'high' ? '⚠️' :
                                                                     '🚫'}
                                                                </span>
                                                                <span style={{
                                                                    ...styles.warningTitle,
                                                                    color: risk.level === 'moderate' ? '#ff9800' :
                                                                           risk.level === 'high' ? '#ff5722' :
                                                                           '#d32f2f'
                                                                }}>
                                                                    {risk.level === 'moderate' ? '中等风险' :
                                                                     risk.level === 'high' ? '高风险' :
                                                                     '严重风险'}
                                                                </span>
                                                            </div>
                                                            <ul style={styles.warningList}>
                                                                {risk.issues.map((issue, idx) => (
                                                                    <li key={idx} style={styles.warningItem}>{issue}</li>
                                                                ))}
                                                            </ul>
                                                            {risk.level === 'high' && (
                                                                <label style={styles.ackLabel}>
                                                                    <input
                                                                        type="checkbox"
                                                                        checked={riskAcknowledged[r.id] || false}
                                                                        onChange={(e) => setRiskAcknowledged({
                                                                            ...riskAcknowledged,
                                                                            [r.id]: e.target.checked
                                                                        })}
                                                                        style={styles.ackCheckbox}
                                                                    />
                                                                    我了解风险，仍要启用此规则
                                                                </label>
                                                            )}
                                                            {risk.level === 'severe' && (
                                                                <div style={styles.severeMessage}>
                                                                    此模式过于危险，必须修复后才能启用
                                                                </div>
                                                            )}
                                                        </div>
                                                    )}
                                                </div>

                                                <div style={styles.row}>
                                                    <div style={styles.col}>
                                                        <label style={styles.fieldLabel}>背景色</label>
                                                        <label
                                                            style={{
                                                                ...styles.bgOption,
                                                                color: hoveredBgOption === r.id ? '#fff' : '#ccc'
                                                            }}
                                                            onMouseEnter={() => setHoveredBgOption(r.id)}
                                                            onMouseLeave={() => setHoveredBgOption(null)}
                                                        >
                                                            <input
                                                                type="checkbox"
                                                                checked={!r.style?.background_color}
                                                                onChange={(e) => patchStyle(r.id, { background_color: e.target.checked ? '' : '#1d3a5a' })}
                                                                style={{...styles.checkbox, position: 'absolute', opacity: 0, pointerEvents: 'none'}}
                                                            />
                                                            <span style={{
                                                                ...styles.customCheckbox,
                                                                borderColor: !r.style?.background_color ? '#5a8a6a' : (hoveredBgOption === r.id ? '#666' : '#555'),
                                                                backgroundColor: !r.style?.background_color ? '#1a2a24' : (hoveredBgOption === r.id ? '#2a2a2a' : '#1e1e1e'),
                                                            }}>
                                                                {!r.style?.background_color && <span style={styles.checkmark}>✓</span>}
                                                            </span>
                                                            <span style={styles.bgOptionText}>使用终端背景色</span>
                                                        </label>
                                                        {r.style?.background_color && (
                                                            <div style={styles.colorInput}>
                                                                <input
                                                                    type="color"
                                                                    value={r.style?.background_color || '#1d3a5a'}
                                                                    onChange={(e) => patchStyle(r.id, { background_color: e.target.value })}
                                                                    style={styles.colorPicker}
                                                                />
                                                                <input
                                                                    value={r.style?.background_color || ''}
                                                                    onChange={(e) => patchStyle(r.id, { background_color: e.target.value })}
                                                                    style={styles.input}
                                                                    placeholder="#RRGGBB"
                                                                />
                                                            </div>
                                                        )}
                                                    </div>
                                                    <div style={styles.col}>
                                                        <label style={styles.fieldLabel}>文字颜色</label>
                                                        <div style={styles.colorInput}>
                                                            <input
                                                                type="color"
                                                                value={r.style?.color || '#ffffff'}
                                                                onChange={(e) => patchStyle(r.id, { color: e.target.value })}
                                                                style={styles.colorPicker}
                                                            />
                                                            <input
                                                                value={r.style?.color || ''}
                                                                onChange={(e) => patchStyle(r.id, { color: e.target.value })}
                                                                style={styles.input}
                                                                placeholder="#RRGGBB"
                                                            />
                                                        </div>
                                                    </div>
                                                </div>
                                                <div style={styles.field}>
                                                    <label style={styles.fieldLabel}>字重</label>
                                                    <select
                                                        value={r.style?.font_weight || 'normal'}
                                                        onChange={(e) => patchStyle(r.id, { font_weight: e.target.value })}
                                                        style={styles.select}
                                                    >
                                                        <option value="normal">常规</option>
                                                        <option value="bold">粗体</option>
                                                    </select>
                                                </div>

                                                {/* Preview */}
                                                {r.pattern && (
                                                    <div style={styles.field}>
                                                        <label style={styles.fieldLabel}>效果预览</label>
                                                        <div style={styles.previewBg}>
                                                            <span style={{
                                                                backgroundColor: r.style?.background_color ? r.style.background_color : 'unset',
                                                                color: r.style?.color || '#ffffff',
                                                                fontWeight: r.style?.font_weight as any || 'normal',
                                                                padding: '2px 6px',
                                                                borderRadius: '3px',
                                                                whiteSpace: 'nowrap',
                                                                textDecoration: r.style?.text_decoration || 'none',
                                                                opacity: r.style?.opacity !== undefined ? r.style.opacity : 1
                                                            }}>
                                                                {r.name || '未命名'} 示例文本
                                                            </span>
                                                        </div>
                                                    </div>
                                                )}
                                            </div>
                                        )}
                                    </div>
                                );
                            })}
                        </div>
                    </div>

                    {/* Footer */}
                    <div style={styles.footer}>
                        <div style={styles.summary}>
                            共 {sorted.length} 条规则，{sorted.filter(r => r.is_enabled).length} 条已启用
                        </div>
                        <div style={styles.footerActions}>
                            <button style={styles.cancelBtn} onClick={handleXClick}>取消</button>
                            <button style={styles.saveBtn} onClick={applyChanges}>保存更改</button>
                        </div>
                    </div>
                </div>
            </div>

            {/* Unsaved Confirmation */}
            <UnsavedChangesModal
                isOpen={showUnsavedWarning}
                changedCount={getChangedCount()}
                onSave={() => {
                    applyAndClose();
                    setShowUnsavedWarning(false);
                }}
                onDiscard={() => {
                    onClose();
                    setShowUnsavedWarning(false);
                }}
                onCancel={() => setShowUnsavedWarning(false)}
            />
        </>
    );
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
        zIndex: 2100,
        padding: '20px',
    },
    modal: {
        backgroundColor: '#252526',
        borderRadius: '8px',
        width: '900px',
        maxHeight: '650px',
        height: '650px',
        display: 'flex',
        flexDirection: 'column' as const,
        boxShadow: '0 4px 12px rgba(0, 0, 0, 0.5)',
        overflow: 'hidden',
        alignSelf: 'center' as const,
    },
    header: {
        padding: '16px 24px',
        borderBottom: '1px solid #3c3c3c',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        backgroundColor: '#1e1e1e',
    },
    titleContainer: {
        display: 'flex',
        alignItems: 'center',
        gap: '12px',
    },
    title: {
        margin: 0,
        fontSize: '1.1rem',
        color: '#fff',
        fontWeight: 600,
    },
    unsavedIndicator: {
        color: '#ff9800',
        fontSize: '13px',
        fontWeight: 500,
    },
    closeBtn: {
        background: 'none',
        border: 'none',
        color: '#ccc',
        fontSize: '1.5rem',
        cursor: 'pointer',
        padding: '0',
        width: '32px',
        height: '32px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        borderRadius: '4px',
        ':hover': {
            backgroundColor: '#3c3c3c',
        }
    },
    body: {
        flex: 1,
        display: 'flex',
        flexDirection: 'column' as const,
        overflow: 'hidden',
        minHeight: 0,
    },
    toolbar: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        padding: '16px 24px',
        borderBottom: '1px solid #3c3c3c',
        backgroundColor: '#2D2D2D',
    },
    hint: {
        color: '#888',
        fontSize: '12px',
    },
    list: {
        flex: 1,
        overflowY: 'auto' as const,
        overflowX: 'hidden' as const,
        padding: '16px 24px',
        backgroundColor: '#2D2D2D',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '12px',
        minHeight: 0,
    },
    empty: {
        color: '#666',
        fontSize: '13px',
        textAlign: 'center' as const,
        padding: '40px 0',
    },
    item: {
        border: '1px solid #3c3c3c',
        borderRadius: '6px',
        backgroundColor: '#1e1e1e',
        overflow: 'hidden',
        flexShrink: 0,
    },
    itemHeader: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        padding: '12px',
        gap: '12px',
    },
    itemLeft: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '4px',
        flex: 1,
        minWidth: 0,
    },
    ruleInfo: {
        display: 'flex',
        alignItems: 'center',
        gap: '10px',
    },
    nameText: {
        color: '#ccc',
        fontSize: '13px',
        fontWeight: 500,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
        flex: 1,
    },
    riskBadge: {
        fontSize: '11px',
        padding: '2px 6px',
        borderRadius: '4px',
        fontWeight: 600,
        flexShrink: 0,
    },
    statusText: {
        fontSize: '11px',
        color: '#888',
        marginLeft: '50px',
    },
    switch: {
        position: 'relative' as const,
        display: 'inline-block',
        width: '40px',
        height: '20px',
        flexShrink: 0,
    },
    slider: {
        position: 'absolute' as const,
        cursor: 'pointer',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: '#ccc',
        transition: '.4s',
        borderRadius: '20px',
        ':before': {
            position: 'absolute' as const,
            content: '""',
            height: '16px',
            width: '16px',
            left: '2px',
            bottom: '2px',
            backgroundColor: 'white',
            transition: '.4s',
            borderRadius: '50%',
        }
    },
    actions: {
        display: 'flex',
        gap: '6px',
        flexShrink: 0,
    },
    editBtn: {
        padding: '6px 12px',
        borderRadius: '4px',
        border: '1px solid #5A5A5A',
        backgroundColor: '#3C3C3C',
        color: '#fff',
        cursor: 'pointer',
        fontSize: '12px',
        display: 'flex',
        alignItems: 'center',
        gap: '4px',
        ':hover': {
            backgroundColor: '#4C4C4C',
        }
    },
    iconBtn: {
        padding: '0',
        width: '28px',
        height: '28px',
        borderRadius: '4px',
        border: '1px solid #5A5A5A',
        backgroundColor: '#3C3C3C',
        color: '#bbb',
        cursor: 'pointer',
        fontSize: '14px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        ':hover': {
            backgroundColor: '#4C4C4C',
        }
    },
    expanded: {
        padding: '16px',
        borderTop: '1px solid #3c3c3c',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '12px',
        backgroundColor: '#252526',
    },
    row: {
        display: 'flex',
        gap: '12px',
    },
    col: {
        flex: 1,
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '6px',
    },
    field: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '6px',
    },
    fieldLabel: {
        fontSize: '13px',
        color: '#CCCCCC',
        fontWeight: 500,
    },
    input: {
        padding: '8px 12px',
        borderRadius: '4px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#1e1e1e',
        color: '#fff',
        outline: 'none',
        fontSize: '13px',
        ':focus': {
            borderColor: '#007ACC',
        }
    },
    select: {
        padding: '8px 12px',
        borderRadius: '4px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#1e1e1e',
        color: '#fff',
        outline: 'none',
        fontSize: '13px',
        cursor: 'pointer',
    },
    colorInput: {
        display: 'flex',
        gap: '8px',
        alignItems: 'center',
    },
    colorPicker: {
        width: '40px',
        height: '34px',
        border: '1px solid #3c3c3c',
        borderRadius: '4px',
        cursor: 'pointer',
        padding: '2px',
        backgroundColor: '#1e1e1e',
    },
    bgOption: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        cursor: 'pointer',
        fontSize: '13px',
        color: '#ccc',
        userSelect: 'none' as const,
        position: 'relative' as const,
    },
    customCheckbox: {
        width: '16px',
        height: '16px',
        border: '2px solid #555',
        borderRadius: '3px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        flexShrink: 0,
        backgroundColor: '#1e1e1e',
    },
    checkmark: {
        color: '#7aaa88',
        fontSize: '12px',
        fontWeight: 'bold',
        lineHeight: 1,
    },
    bgOptionText: {
        display: 'inline-flex',
        alignItems: 'center',
    },
    checkbox: {
        width: '16px',
        height: '16px',
        cursor: 'pointer',
        flexShrink: 0,
    },
    warningBox: {
        padding: '12px',
        borderRadius: '4px',
        border: '1px solid',
        marginTop: '8px',
    },
    warningHeader: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        marginBottom: '8px',
    },
    warningIcon: {
        fontSize: '14px',
    },
    warningTitle: {
        fontWeight: 600,
        fontSize: '13px',
    },
    warningList: {
        margin: '0 0 12px 0',
        paddingLeft: '24px',
    },
    warningItem: {
        fontSize: '12px',
        color: '#ccc',
        marginBottom: '4px',
    },
    ackLabel: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        fontSize: '12px',
        color: '#ccc',
        cursor: 'pointer',
    },
    ackCheckbox: {
        width: '14px',
        height: '14px',
        cursor: 'pointer',
    },
    severeMessage: {
        fontSize: '12px',
        color: '#ff8080',
        marginTop: '8px',
    },
    previewBg: {
        backgroundColor: '#0d0d0d',
        padding: '12px',
        borderRadius: '4px',
        border: '1px solid #3c3c3c',
    },
    footer: {
        padding: '16px 24px',
        borderTop: '1px solid #3c3c3c',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        backgroundColor: '#1e1e1e',
    },
    summary: {
        color: '#888',
        fontSize: '13px',
    },
    footerActions: {
        display: 'flex',
        gap: '12px',
    },
    saveBtn: {
        padding: '8px 20px',
        borderRadius: '4px',
        border: 'none',
        backgroundColor: '#007acc',
        color: '#fff',
        cursor: 'pointer',
        fontWeight: 500,
        fontSize: '13px',
        ':hover': {
            backgroundColor: '#005a9e',
        }
    },
    cancelBtn: {
        padding: '8px 20px',
        borderRadius: '4px',
        border: '1px solid #5A5A5A',
        backgroundColor: 'transparent',
        color: '#ccc',
        cursor: 'pointer',
        fontWeight: 500,
        fontSize: '13px',
        ':hover': {
            backgroundColor: '#3C3C3C',
        }
    },
    primaryButton: {
        padding: '8px 16px',
        borderRadius: '4px',
        border: 'none',
        backgroundColor: '#007acc',
        color: '#fff',
        cursor: 'pointer',
        fontWeight: 500,
        fontSize: '13px',
        ':hover': {
            backgroundColor: '#005a9e',
        }
    },
};

const unsavedStyles = {
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
        zIndex: 2200,
    },
    modal: {
        backgroundColor: '#252526',
        borderRadius: '8px',
        padding: '24px',
        width: '400px',
        boxShadow: '0 4px 12px rgba(0, 0, 0, 0.5)',
    },
    title: {
        color: '#fff',
        fontSize: '16px',
        fontWeight: 600,
        margin: '0 0 12px 0',
    },
    message: {
        color: '#ccc',
        fontSize: '13px',
        margin: '0 0 20px 0',
    },
    actions: {
        display: 'flex',
        gap: '10px',
        justifyContent: 'flex-end',
    },
    cancelBtn: {
        padding: '8px 16px',
        borderRadius: '4px',
        border: '1px solid #5A5A5A',
        backgroundColor: 'transparent',
        color: '#ccc',
        cursor: 'pointer',
        fontWeight: 500,
        fontSize: '13px',
    },
    discardBtn: {
        padding: '8px 16px',
        borderRadius: '4px',
        border: '1px solid #6c4a4a',
        backgroundColor: '#5c3a3a',
        color: '#ff8080',
        cursor: 'pointer',
        fontSize: '13px',
    },
    saveBtn: {
        padding: '8px 16px',
        borderRadius: '4px',
        border: 'none',
        backgroundColor: '#007acc',
        color: '#fff',
        cursor: 'pointer',
        fontWeight: 500,
        fontSize: '13px',
    }
};
