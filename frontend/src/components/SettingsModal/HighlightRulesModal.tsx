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

    React.useEffect(() => {
        if (isOpen) setDraft(rules);
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
        update([...sorted, r]);
    };

    const removeRule = (id: string) => {
        update(sorted.filter(r => r.id !== id));
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
        update(sorted.map(r => (r.id === id ? { ...r, style: { ...(r.style || {}), ...partial } } : r)));
    };

    return (
        <div style={styles.backdrop} onClick={onClose}>
            <div style={styles.modal} onClick={(e) => e.stopPropagation()}>
                <div style={styles.header}>
                    <div style={styles.title}>终端高亮规则</div>
                    <button style={styles.x} onClick={onClose}>×</button>
                </div>

                <div style={styles.body}>
                    <div style={styles.toolbar}>
                        <button style={styles.btn} onClick={addRule}>+ 添加规则</button>
                        <div style={styles.hint}>优先级越小越先匹配；高风险正则默认禁用。</div>
                    </div>

                    <div style={styles.list} className="hide-scrollbar">
                        {sorted.length === 0 && <div style={styles.empty}>暂无规则</div>}
                        {sorted.map((r, i) => {
                            const risky = isHighRiskPattern(r.pattern);
                            return (
                                <div key={r.id} style={styles.item}>
                                    <div style={styles.itemTop}>
                                        <label style={styles.check}>
                                            <input
                                                type="checkbox"
                                                checked={!!r.is_enabled}
                                                onChange={(e) => patch(r.id, { is_enabled: e.target.checked && !risky })}
                                            />
                                            <span style={styles.checkText}>{r.is_enabled ? '启用' : '禁用'}</span>
                                        </label>
                                        <div style={styles.actions}>
                                            <button style={styles.smallBtn} onClick={() => move(r.id, -1)} disabled={i === 0}>↑</button>
                                            <button style={styles.smallBtn} onClick={() => move(r.id, 1)} disabled={i === sorted.length - 1}>↓</button>
                                            <button style={styles.smallBtn} onClick={() => removeRule(r.id)}>删除</button>
                                        </div>
                                    </div>

                                    <div style={styles.grid}>
                                        <div style={styles.field}>
                                            <div style={styles.label}>名称</div>
                                            <input
                                                value={r.name}
                                                onChange={(e) => patch(r.id, { name: e.target.value })}
                                                style={styles.input}
                                            />
                                        </div>
                                        <div style={styles.field}>
                                            <div style={styles.label}>优先级</div>
                                            <input
                                                value={String(r.priority ?? 0)}
                                                onChange={(e) => patch(r.id, { priority: parseInt(e.target.value) || 0 })}
                                                style={styles.input}
                                            />
                                        </div>
                                    </div>

                                    <div style={styles.field}>
                                        <div style={styles.label}>正则</div>
                                        <input
                                            value={r.pattern}
                                            onChange={(e) => {
                                                const v = e.target.value;
                                                const riskyNow = isHighRiskPattern(v);
                                                patch(r.id, { pattern: v, is_enabled: riskyNow ? false : r.is_enabled });
                                            }}
                                            style={styles.input}
                                            placeholder="例如：(?i)\\b(error|fail|fatal)\\b"
                                        />
                                        {risky && <div style={styles.warn}>该正则可能有性能风险，已强制禁用（可修改后再启用）。</div>}
                                    </div>

                                    <div style={styles.grid}>
                                        <div style={styles.field}>
                                            <div style={styles.label}>背景色</div>
                                            <input
                                                value={r.style?.background_color || ''}
                                                onChange={(e) => patchStyle(r.id, { background_color: e.target.value })}
                                                style={styles.input}
                                                placeholder="#RRGGBB"
                                            />
                                        </div>
                                        <div style={styles.field}>
                                            <div style={styles.label}>文字色</div>
                                            <input
                                                value={r.style?.color || ''}
                                                onChange={(e) => patchStyle(r.id, { color: e.target.value })}
                                                style={styles.input}
                                                placeholder="#RRGGBB"
                                            />
                                        </div>
                                    </div>
                                </div>
                            );
                        })}
                    </div>
                </div>

                <div style={styles.footer}>
                    <button style={styles.btn2} onClick={onClose}>取消</button>
                    <button style={styles.primary} onClick={applyAndClose}>保存</button>
                </div>
            </div>
        </div>
    );
}

const styles: Record<string, React.CSSProperties> = {
    backdrop: {
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0,0,0,0.5)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 2000
    },
    modal: {
        width: '860px',
        maxWidth: '92vw',
        height: '720px',
        maxHeight: '92vh',
        backgroundColor: '#252526',
        border: '1px solid #1f1f1f',
        borderRadius: '12px',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden'
    },
    header: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '12px 14px',
        borderBottom: '1px solid #1f1f1f'
    },
    title: { color: '#fff', fontWeight: 800, fontSize: '14px' },
    x: {
        background: 'transparent',
        color: '#aaa',
        border: '1px solid #2a2a2a',
        borderRadius: '8px',
        width: '32px',
        height: '28px',
        cursor: 'pointer',
        fontSize: '18px',
        lineHeight: '18px'
    },
    body: {
        flex: 1,
        minHeight: 0,
        display: 'flex',
        flexDirection: 'column'
    },
    toolbar: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '12px 14px',
        borderBottom: '1px solid #1f1f1f'
    },
    hint: { color: '#8a8a8a', fontSize: '12px' },
    list: {
        flex: 1,
        minHeight: 0,
        overflow: 'auto',
        padding: '14px',
        display: 'flex',
        flexDirection: 'column',
        gap: '10px'
    },
    empty: { color: '#8a8a8a', fontSize: '12px' },
    item: {
        border: '1px solid #2a2a2a',
        borderRadius: '12px',
        backgroundColor: '#141414',
        padding: '12px',
        display: 'flex',
        flexDirection: 'column',
        gap: '10px'
    },
    itemTop: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        gap: '10px'
    },
    actions: { display: 'flex', gap: '6px' },
    smallBtn: {
        backgroundColor: '#202020',
        color: '#ddd',
        border: '1px solid #2a2a2a',
        borderRadius: '8px',
        padding: '4px 10px',
        cursor: 'pointer',
        fontSize: '12px'
    },
    btn: {
        backgroundColor: '#202020',
        color: '#ddd',
        border: '1px solid #2a2a2a',
        borderRadius: '8px',
        padding: '6px 10px',
        cursor: 'pointer',
        fontSize: '12px'
    },
    btn2: {
        backgroundColor: '#202020',
        color: '#ddd',
        border: '1px solid #2a2a2a',
        borderRadius: '8px',
        padding: '8px 14px',
        cursor: 'pointer',
        fontSize: '12px'
    },
    primary: {
        backgroundColor: '#007acc',
        color: '#fff',
        border: 'none',
        borderRadius: '8px',
        padding: '8px 14px',
        cursor: 'pointer',
        fontSize: '12px',
        fontWeight: 700
    },
    footer: {
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '10px',
        padding: '12px 14px',
        borderTop: '1px solid #1f1f1f'
    },
    check: { display: 'flex', alignItems: 'center', gap: '8px' },
    checkText: { color: '#ddd', fontSize: '12px' },
    grid: {
        display: 'grid',
        gridTemplateColumns: '1fr 160px',
        gap: '10px'
    },
    field: { display: 'flex', flexDirection: 'column', gap: '6px' },
    label: { color: '#a8a8a8', fontSize: '12px' },
    input: {
        backgroundColor: '#202020',
        color: '#ddd',
        border: '1px solid #2a2a2a',
        borderRadius: '8px',
        padding: '8px 10px',
        outline: 'none',
        fontSize: '12px'
    },
    warn: { color: '#ffbf66', fontSize: '12px' }
};

