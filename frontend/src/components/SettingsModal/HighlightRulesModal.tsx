import React, { useMemo, useState, useEffect } from 'react';
import { HighlightRule } from '../Terminal/highlightTypes';

interface HighlightRulesModalProps {
    isOpen: boolean;
    rules: HighlightRule[];
    onChange: (rules: HighlightRule[]) => void;
    onClose: () => void;
}

// 风险等级类型
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

// 评估正则表达式的风险等级
function assessPatternRisk(pattern: string): PatternRisk {
    const p = pattern.trim();
    if (!p) return { level: 'safe', issues: [], canEnable: true };

    const issues: string[] = [];
    let level: RiskLevel = 'safe';

    // 长度检查
    if (p.length > 500) {
        issues.push(`正则非常长 (${p.length}字符)`);
        level = 'high';
    } else if (p.length > 200) {
        issues.push(`正则较长 (${p.length}字符)`);
        level = 'moderate';
    }

    // 嵌套量词检查
    if (/\(\.\*\)\+/.test(p) || /\(\.\+\)\+/.test(p)) {
        issues.push('包含嵌套量词，可能导致指数级匹配');
        if (level === 'safe') {
            level = 'high';
        } else if (level === 'moderate') {
            level = 'severe';
        }
    }

    // 灾难性模式检查
    if (/^(\.\*|\.\+)[+*]/.test(p)) {
        issues.push('灾难性回溯模式');
        level = 'severe';
    }

    // 其他复杂嵌套
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

// 深度比较两个规则数组是否相同
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

// Toggle Switch 子组件
interface ToggleSwitchProps {
    enabled: boolean;
    disabled: boolean;
    locked: boolean;
    onToggle: () => void;
    reason?: string;
}

function ToggleSwitch({ enabled, disabled, locked, onToggle, reason }: ToggleSwitchProps) {
    const getTrackColor = () => {
        if (locked) return '#ff9800';
        if (enabled) return '#4caf50';
        return '#555';
    };

    const getThumbPosition = () => {
        return enabled ? 'calc(100% - 18px)' : '2px';
    };

    return (
        <div
            style={{
                ...styles.toggleSwitch,
                backgroundColor: getTrackColor(),
                cursor: disabled ? 'not-allowed' : 'pointer',
                opacity: disabled ? 0.5 : 1
            }}
            onClick={disabled ? undefined : onToggle}
            title={reason}
        >
            <div
                style={{
                    ...styles.toggleThumb,
                    left: getThumbPosition()
                }}
            >
                {locked && <span style={styles.lockIcon}>🔒</span>}
            </div>
        </div>
    );
}

// 未保存更改确认弹窗
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
        <div style={styles.confirmBackdrop}>
            <div style={styles.confirmModal}>
                <h3 style={styles.confirmTitle}>⚠️ 确认关闭？</h3>
                <p style={styles.confirmMessage}>
                    您有 {changedCount} 条规则的更改尚未保存。
                </p>
                <div style={styles.confirmActions}>
                    <button style={styles.confirmCancelBtn} onClick={onCancel}>
                        继续编辑
                    </button>
                    <button style={styles.confirmDiscardBtn} onClick={onDiscard}>
                        放弃更改
                    </button>
                    <button style={styles.confirmSaveBtn} onClick={onSave}>
                        保存并关闭
                    </button>
                </div>
            </div>
        </div>
    );
}

export default function HighlightRulesModal({ isOpen, rules, onChange, onClose }: HighlightRulesModalProps) {
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

    // 检测是否有未保存的更改
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

    const applyChanges = () => {
        onChange(sorted);
        // Reset the dirty state after saving
        setIsDirty(false);
    };

    const applyAndClose = () => {
        onChange(sorted);
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

    // 计算未保存的更改数量
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
            <div style={styles.backdrop} onClick={onClose}>
                <div style={styles.modal} onClick={(e) => e.stopPropagation()}>
                    <div style={styles.header}>
                        <div style={styles.titleContainer}>
                            <div style={styles.title}>突出显示集</div>
                            {isDirty && <div style={styles.unsavedIndicator}>● 未保存</div>}
                        </div>
                        <button style={styles.x} onClick={handleXClick}>×</button>
                    </div>

                    <div style={styles.body}>
                        <div style={styles.toolbar}>
                            <button style={styles.primary} onClick={addRule}>+ 新建规则</button>
                            <div style={styles.hint}>
                                {isDirty ? `提示: 有 ${getChangedCount()} 条未保存更改` : '优先级越小越先匹配'}
                            </div>
                        </div>

                        <div style={styles.list} className="hide-scrollbar">
                            {sorted.length === 0 && <div style={styles.empty}>暂无规则，点击上方按钮添加</div>}
                            {sorted.map((r, i) => {
                                const risk = assessPatternRisk(r.pattern);
                                const canEnable = risk.canEnable && (risk.level === 'safe' || riskAcknowledged[r.id]);
                                const editing = isEditing(r.id);

                                return (
                                    <div key={r.id} style={styles.item}>
                                        {/* 顶部栏：启用开关 + 操作按钮 */}
                                        <div style={styles.itemHeader}>
                                            <div style={styles.itemLeft}>
                                                <div style={styles.ruleInfo}>
                                                    <ToggleSwitch
                                                        enabled={!!r.is_enabled}
                                                        disabled={!canEnable}
                                                        locked={risk.level === 'severe' || (risk.level === 'high' && !riskAcknowledged[r.id])}
                                                        onToggle={() => patch(r.id, { is_enabled: !r.is_enabled && canEnable })}
                                                        reason={risk.level !== 'safe' ? `风险等级: ${risk.level}` : undefined}
                                                    />
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
                                                    {risk.level !== 'safe' && r.is_enabled && ' (风险已确认)'}
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
                                                    onClick={() => {
                                                        const idx = sorted.findIndex(x => x.id === r.id);
                                                        if (idx <= 0) return;
                                                        const target = sorted[0];
                                                        const current = sorted[idx];
                                                        const next = [...sorted];
                                                        next[0] = { ...current, priority: target.priority };
                                                        next[idx] = { ...target, priority: current.priority };
                                                        for (let j = 1; j < idx; j++) {
                                                            next[j].priority = next[j-1].priority + 1;
                                                        }
                                                        update(next);
                                                    }}
                                                    disabled={i === 0}
                                                    title="置顶"
                                                >
                                                    ⇱
                                                </button>
                                                <button
                                                    style={styles.iconBtn}
                                                    onClick={() => {
                                                        const idx = sorted.findIndex(x => x.id === r.id);
                                                        if (idx < 0 || idx >= sorted.length - 1) return;
                                                        const target = sorted[sorted.length - 1];
                                                        const current = sorted[idx];
                                                        const next = [...sorted];
                                                        next[sorted.length - 1] = { ...current, priority: target.priority };
                                                        next[idx] = { ...target, priority: current.priority };
                                                        for (let j = idx + 1; j < sorted.length - 1; j++) {
                                                            next[j].priority = next[j-1].priority + 1;
                                                        }
                                                        update(next);
                                                    }}
                                                    disabled={i === sorted.length - 1}
                                                    title="置底"
                                                >
                                                    ⇲
                                                </button>
                                                <button
                                                    style={{ ...styles.iconBtn, ...styles.deleteBtn }}
                                                    onClick={() => removeRule(r.id)}
                                                    title="删除"
                                                >
                                                    ×
                                                </button>
                                            </div>
                                        </div>

                                        {/* 展开的编辑区域 */}
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
                                                            const v = e.target.value;
                                                            patch(r.id, { pattern: v });
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
                                                    {r.pattern && risk.level === 'safe' && (
                                                        <div style={styles.previewText}>
                                                            ✅ 预览：匹配 <code style={styles.code}>{r.pattern}</code>
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
                                                                transform: hoveredBgOption === r.id ? 'scale(1.05)' : 'scale(1)'
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

                                                {/* 效果预览 */}
                                                {r.pattern && (
                                                    <div style={styles.previewBox}>
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

                    <div style={styles.footer}>
                        <div style={styles.summary}>
                            共 {sorted.length} 条规则，{sorted.filter(r => r.is_enabled).length} 条已启用
                        </div>
                        <div style={styles.footerBtns}>
                            <button style={styles.cancelBtn} onClick={handleXClick}>取消</button>
                            <button style={styles.primary} onClick={applyChanges}>保存更改</button>
                        </div>
                    </div>
                </div>
            </div>

            {/* 未保存更改确认弹窗 */}
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

const styles: Record<string, React.CSSProperties> = {
    backdrop: {
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0,0,0,0.6)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 2000
    },
    modal: {
        width: '900px',
        maxWidth: '95vw',
        height: '80vh',
        maxHeight: '800px',
        minHeight: '600px',
        backgroundColor: '#252526',
        border: '1px solid #1f1f1f',
        borderRadius: '12px',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        boxShadow: '0 8px 32px rgba(0,0,0,0.5)'
    },
    header: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '16px 20px',
        borderBottom: '1px solid #1f1f1f',
        backgroundColor: '#1e1e1e',
        flexShrink: 0
    },
    titleContainer: {
        display: 'flex',
        alignItems: 'center',
        gap: '12px'
    },
    title: {
        color: '#fff',
        fontWeight: 600,
        fontSize: '16px',
        letterSpacing: '0.3px'
    },
    unsavedIndicator: {
        color: '#ff9800',
        fontSize: '12px',
        fontWeight: 500
    },
    x: {
        background: 'transparent',
        color: '#999',
        border: 'none',
        borderRadius: '6px',
        width: '36px',
        height: '32px',
        cursor: 'pointer',
        fontSize: '24px',
        lineHeight: '24px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        transition: 'all 0.2s'
    },
    body: {
        flex: 1,
        minHeight: 0,
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden'
    },
    toolbar: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '16px 20px',
        borderBottom: '1px solid #1f1f1f',
        backgroundColor: '#1a1a1a',
        flexShrink: 0
    },
    hint: {
        color: '#8a8a8a',
        fontSize: '12px'
    },
    list: {
        flex: 1,
        minHeight: 0,
        overflowY: 'auto',
        overflowX: 'hidden',
        padding: '16px 20px',
        display: 'flex',
        flexDirection: 'column',
        gap: '8px'
    },
    empty: {
        color: '#666',
        fontSize: '13px',
        textAlign: 'center',
        padding: '40px 0'
    },
    item: {
        border: '1px solid #2a2a2a',
        borderRadius: '8px',
        backgroundColor: '#1a1a1a',
        overflow: 'visible',
        transition: 'all 0.2s'
    },
    itemHeader: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        padding: '10px 12px',
        gap: '12px',
        backgroundColor: '#222'
    },
    itemLeft: {
        display: 'flex',
        flexDirection: 'column',
        gap: '4px',
        flex: 1,
        minWidth: 0
    },
    ruleInfo: {
        display: 'flex',
        alignItems: 'center',
        gap: '10px',
        flex: 1,
        minWidth: 0
    },
    statusText: {
        fontSize: '11px',
        color: '#888',
        marginLeft: '50px'
    },
    // Toggle Switch 样式
    toggleSwitch: {
        position: 'relative' as const,
        width: '44px',
        height: '22px',
        borderRadius: '11px',
        flexShrink: 0,
        transition: 'all 0.2s'
    },
    toggleThumb: {
        position: 'absolute' as const,
        top: '2px',
        width: '18px',
        height: '18px',
        borderRadius: '50%',
        backgroundColor: '#fff',
        transition: 'left 0.2s',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontSize: '10px'
    },
    lockIcon: {
        fontSize: '10px'
    },
    checkbox: {
        width: '16px',
        height: '16px',
        cursor: 'pointer',
        flexShrink: 0,
        border: '1px solid #555',
        borderRadius: '3px',
        backgroundColor: '#1e1e1e'
    },
    nameText: {
        color: '#ddd',
        fontSize: '13px',
        fontWeight: 500,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
        flex: 1
    },
    riskBadge: {
        fontSize: '11px',
        padding: '2px 6px',
        borderRadius: '4px',
        fontWeight: 600,
        flexShrink: 0
    },
    actions: {
        display: 'flex',
        gap: '4px',
        flexShrink: 0
    },
    editBtn: {
        backgroundColor: '#333',
        color: '#bbb',
        border: '1px solid #444',
        borderRadius: '4px',
        padding: '0 8px',
        height: '26px',
        cursor: 'pointer',
        fontSize: '12px',
        display: 'flex',
        alignItems: 'center',
        gap: '4px',
        transition: 'all 0.2s'
    },
    iconBtn: {
        backgroundColor: '#333',
        color: '#bbb',
        border: '1px solid #444',
        borderRadius: '4px',
        width: '28px',
        height: '26px',
        cursor: 'pointer',
        fontSize: '14px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        transition: 'all 0.2s',
        padding: 0
    },
    deleteBtn: {
        color: '#ff8080',
        borderColor: '#5c3a3a'
    },
    expanded: {
        padding: '16px',
        borderTop: '1px solid #2a2a2a',
        display: 'flex',
        flexDirection: 'column',
        gap: '12px',
        backgroundColor: '#151515',
        overflow: 'visible'
    },
    row: {
        display: 'flex',
        gap: '12px'
    },
    col: {
        flex: 1,
        display: 'flex',
        flexDirection: 'column',
        gap: '6px'
    },
    field: {
        display: 'flex',
        flexDirection: 'column',
        gap: '6px'
    },
    fieldLabel: {
        color: '#999',
        fontSize: '12px',
        fontWeight: 500
    },
    input: {
        backgroundColor: '#1e1e1e',
        color: '#ddd',
        border: '1px solid #3a3a3a',
        borderRadius: '6px',
        padding: '8px 10px',
        outline: 'none',
        fontSize: '13px',
        transition: 'border-color 0.2s'
    },
    select: {
        backgroundColor: '#1e1e1e',
        color: '#ddd',
        border: '1px solid #3a3a3a',
        borderRadius: '6px',
        padding: '8px 10px',
        outline: 'none',
        fontSize: '13px',
        cursor: 'pointer'
    },
    colorInput: {
        display: 'flex',
        gap: '8px',
        alignItems: 'center'
    },
    bgOption: {
        display: 'flex',
        alignItems: 'center',
        gap: '10px',
        cursor: 'pointer',
        fontSize: '13px',
        color: '#ccc',
        marginBottom: '8px',
        userSelect: 'none',
        position: 'relative',
        transition: 'opacity 0.2s'
    },
    customCheckbox: {
        width: '18px',
        height: '18px',
        border: '2px solid #555',
        borderRadius: '4px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        flexShrink: 0,
        transition: 'all 0.2s',
        backgroundColor: '#1e1e1e'
    },
    checkmark: {
        color: '#7aaa88',
        fontSize: '12px',
        fontWeight: 'bold',
        lineHeight: 1
    },
    bgOptionText: {
        display: 'inline-flex',
        alignItems: 'center',
        transition: 'color 0.2s'
    },
    colorPicker: {
        width: '40px',
        height: '34px',
        border: '1px solid #3a3a3a',
        borderRadius: '6px',
        cursor: 'pointer',
        padding: '2px',
        backgroundColor: '#1e1e1e'
    },
    warningBox: {
        padding: '12px',
        borderRadius: '6px',
        border: '1px solid',
        marginTop: '8px'
    },
    warningHeader: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        marginBottom: '8px'
    },
    warningIcon: {
        fontSize: '16px'
    },
    warningTitle: {
        fontWeight: 600,
        fontSize: '13px'
    },
    warningList: {
        margin: '0 0 12px 0',
        paddingLeft: '24px'
    },
    warningItem: {
        fontSize: '12px',
        color: '#ccc',
        marginBottom: '4px'
    },
    ackLabel: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        fontSize: '12px',
        color: '#ccc',
        cursor: 'pointer'
    },
    ackCheckbox: {
        width: '14px',
        height: '14px',
        cursor: 'pointer'
    },
    severeMessage: {
        fontSize: '12px',
        color: '#ff8080',
        marginTop: '8px'
    },
    previewText: {
        color: '#8a8a8a',
        fontSize: '12px'
    },
    code: {
        backgroundColor: '#2a2a2a',
        padding: '2px 6px',
        borderRadius: '3px',
        fontFamily: 'monospace',
        fontSize: '11px'
    },
    previewBox: {
        display: 'flex',
        flexDirection: 'column',
        gap: '6px'
    },
    previewBg: {
        backgroundColor: '#0d0d0d',
        padding: '12px',
        borderRadius: '6px',
        border: '1px solid #2a2a2a'
    },
    footer: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        padding: '16px 20px',
        borderTop: '1px solid #1f1f1f',
        backgroundColor: '#1e1e1e',
        flexShrink: 0
    },
    summary: {
        color: '#8a8a8a',
        fontSize: '13px'
    },
    footerBtns: {
        display: 'flex',
        gap: '10px'
    },
    cancelBtn: {
        backgroundColor: '#3a3a3a',
        color: '#ddd',
        border: '1px solid #4a4a4a',
        borderRadius: '6px',
        padding: '8px 20px',
        cursor: 'pointer',
        fontSize: '13px',
        transition: 'all 0.2s'
    },
    primary: {
        backgroundColor: '#007acc',
        color: '#fff',
        border: 'none',
        borderRadius: '6px',
        padding: '8px 20px',
        cursor: 'pointer',
        fontSize: '13px',
        fontWeight: 600,
        transition: 'all 0.2s'
    },
    // 未保存确认弹窗样式
    confirmBackdrop: {
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0,0,0,0.7)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 3000
    },
    confirmModal: {
        backgroundColor: '#252526',
        border: '1px solid #1f1f1f',
        borderRadius: '12px',
        padding: '24px',
        width: '400px',
        boxShadow: '0 8px 32px rgba(0,0,0,0.5)'
    },
    confirmTitle: {
        color: '#fff',
        fontSize: '18px',
        fontWeight: 600,
        margin: '0 0 12px 0'
    },
    confirmMessage: {
        color: '#ccc',
        fontSize: '14px',
        margin: '0 0 20px 0'
    },
    confirmActions: {
        display: 'flex',
        gap: '10px',
        justifyContent: 'flex-end'
    },
    confirmCancelBtn: {
        backgroundColor: '#3a3a3a',
        color: '#ddd',
        border: '1px solid #4a4a4a',
        borderRadius: '6px',
        padding: '8px 16px',
        cursor: 'pointer',
        fontSize: '13px'
    },
    confirmDiscardBtn: {
        backgroundColor: '#5c3a3a',
        color: '#ff8080',
        border: '1px solid #6c4a4a',
        borderRadius: '6px',
        padding: '8px 16px',
        cursor: 'pointer',
        fontSize: '13px'
    },
    confirmSaveBtn: {
        backgroundColor: '#007acc',
        color: '#fff',
        border: 'none',
        borderRadius: '6px',
        padding: '8px 16px',
        cursor: 'pointer',
        fontSize: '13px',
        fontWeight: 600
    }
};
