import React, { useMemo, useState } from 'react';
import { HighlightRule } from '../Terminal/highlightTypes';

interface HighlightRulesModalProps {
    isOpen: boolean;
    rules: HighlightRule[];
    onChange: (rules: HighlightRule[]) => void;
    onClose: () => void;
}

function newId() {
    // @ts-ignore
    if (globalThis.crypto && globalThis.crypto.randomUUID) {
        // @ts-ignore
        return globalThis.crypto.randomUUID();
    }
    return `r_${Math.random().toString(16).slice(2)}_${Date.now()}`;
}

function isHighRiskPattern(pattern: string): boolean {
    const p = pattern.trim();
    if (!p) return false;
    if (p.length > 200) return true;
    if (/\(\.\*\)\+/.test(p) || /\(\.\+\)\+/.test(p)) return true;
    if (/\(\.\*\)\*/.test(p) && /\+/.test(p)) return true;
    if (/\)\+/.test(p) && /(\+|\*)\)/.test(p)) return true;
    if (/\(\?:?[^)]*[+*][^)]*\)[+*]/.test(p)) return true;
    return false;
}

export default function HighlightRulesModal({ isOpen, rules, onChange, onClose }: HighlightRulesModalProps) {
    const [draft, setDraft] = useState<HighlightRule[]>(rules);
    const [editingId, setEditingId] = useState<string | null>(null);
    const [hoveredBgOption, setHoveredBgOption] = useState<string | null>(null);

    React.useEffect(() => {
        if (isOpen) {
            setDraft(rules);
            setEditingId(null);
        }
    }, [isOpen, rules]);

    const sorted = useMemo(() => {
        return [...draft].sort((a, b) => (a.priority ?? 0) - (b.priority ?? 0));
    }, [draft]);

    if (!isOpen) return null;

    const update = (next: HighlightRule[]) => {
        setDraft(next);
    };

    const applyAndClose = () => {
        onChange(sorted);
        onClose();
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

    return (
        <div style={styles.backdrop} onClick={onClose}>
            <div style={styles.modal} onClick={(e) => e.stopPropagation()}>
                <div style={styles.header}>
                    <div style={styles.title}>突出显示集</div>
                    <button style={styles.x} onClick={onClose}>×</button>
                </div>

                <div style={styles.body}>
                    <div style={styles.toolbar}>
                        <button style={styles.primary} onClick={addRule}>+ 新建规则</button>
                        <div style={styles.hint}>优先级越小越先匹配；高风险正则自动禁用</div>
                    </div>

                    <div style={styles.list} className="hide-scrollbar">
                        {sorted.length === 0 && <div style={styles.empty}>暂无规则，点击上方按钮添加</div>}
                        {sorted.map((r, i) => {
                            const risky = isHighRiskPattern(r.pattern);
                            const editing = isEditing(r.id);

                            return (
                                <div key={r.id} style={styles.item}>
                                    {/* 顶部栏：启用开关 + 操作按钮 */}
                                    <div style={styles.itemHeader}>
                                        <div style={styles.itemLeft}>
                                            <label style={styles.switchLabel}>
                                                <input
                                                    type="checkbox"
                                                    checked={!!r.is_enabled}
                                                    onChange={(e) => patch(r.id, { is_enabled: e.target.checked && !risky })}
                                                    style={styles.checkbox}
                                                />
                                                <span style={{
                                                    ...styles.statusDot,
                                                    backgroundColor: r.is_enabled ? '#4caf50' : '#555',
                                                    boxShadow: r.is_enabled ? '0 0 0 3px rgba(76, 175, 80, 0.2)' : 'none'
                                                }} />
                                                <span style={styles.nameText}>{r.name || '未命名'}</span>
                                                {risky && <span style={styles.riskBadge}>高风险</span>}
                                            </label>
                                        </div>
                                        <div style={styles.actions}>
                                            <button
                                                style={styles.iconBtn}
                                                onClick={() => setEditingId(editing ? null : r.id)}
                                                title={editing ? '收起' : '编辑'}
                                            >
                                                {editing ? '✕' : '✎'}
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
                                                        const riskyNow = isHighRiskPattern(v);
                                                        patch(r.id, { pattern: v, is_enabled: riskyNow ? false : r.is_enabled });
                                                    }}
                                                    style={styles.input}
                                                    placeholder="例如：(?i)\\b(error|fail)\\b"
                                                />
                                                {risky && <div style={styles.warnText}>该正则可能有性能风险，已自动禁用</div>}
                                                {r.pattern && !risky && (
                                                    <div style={styles.previewText}>
                                                        预览：匹配 <code style={styles.code}>{r.pattern}</code>
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
                        <button style={styles.cancelBtn} onClick={onClose}>取消</button>
                        <button style={styles.primary} onClick={applyAndClose}>保存更改</button>
                    </div>
                </div>
            </div>
        </div>
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
    title: {
        color: '#fff',
        fontWeight: 600,
        fontSize: '16px',
        letterSpacing: '0.3px'
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
        alignItems: 'center',
        gap: '10px',
        flex: 1,
        minWidth: 0
    },
    switchLabel: {
        display: 'flex',
        alignItems: 'center',
        gap: '10px',
        cursor: 'pointer',
        flex: 1,
        minWidth: 0
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
    statusDot: {
        width: '12px',
        height: '12px',
        borderRadius: '50%',
        flexShrink: 0,
        transition: 'all 0.2s',
        boxShadow: '0 0 0 2px transparent'
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
        backgroundColor: '#5c2e2e',
        color: '#ff9980',
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
    warnText: {
        color: '#ff9980',
        fontSize: '12px'
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
    }
};
