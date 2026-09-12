import React, { useEffect, useState } from 'react';
import { TbInfoCircle } from 'react-icons/tb';
import { assessPattern } from '../Terminal/highlight/regexSafety';
import type { HighlightRule } from '../Terminal/highlightTypes';
import type { TerminalConfig } from '../Terminal/highlightTypes';
import { normalizeTerminalConfig } from '../Terminal/terminalAppearance';
import type { Theme } from '../appearanceTypes';
import { useToast } from '../feedback/Toast';
import ThemeChoiceCard from './ThemeChoiceCard';
import TerminalAppearanceCard from './TerminalAppearanceCard';
import HighlightRulesModal from './HighlightRulesModal';
import AIConfigCard from './AIConfigCard';
import type { AIConfigRuntime } from './AIConfigCard';
import Switch from './Switch';
import { colors, radius, font } from './settingsStyles';

/**
 * Shell 设置切片：主题 / 终端外观 / 补全延迟 / 高亮规则。
 * Wails 在其完整设置弹窗中复用同一组 section 组件；
 * Sidecar/插件宿主直接使用本弹窗（ShellSettingsRuntime 负责持久化）。
 */
export interface ShellSettings {
    theme: Theme;
    terminal: TerminalConfig;
    completionDelay: number;
    highlightRules: HighlightRule[];
    /** 命令查询（AI 生成命令）快捷键，如 "Ctrl+K"。 */
    commandQueryShortcut?: string;
}

export interface ShellSettingsRuntime {
    load(): Promise<ShellSettings>;
    save(next: ShellSettings): Promise<void>;
}

export interface ShellSettingsModalProps {
    isOpen: boolean;
    onClose: () => void;
    runtime: ShellSettingsRuntime;
    /** 宿主实时应用回调（主题/终端/补全/高亮）；保存由 runtime 负责 */
    onApply: (next: ShellSettings) => void;
    /** 初始值（宿主已加载的当前态），避免弹窗打开时闪烁 */
    initial?: ShellSettings | null;
    /** AI 接入配置（独立持久化域）。未注入时 AI 配置卡不渲染（能力边界纪律）。 */
    aiRuntime?: AIConfigRuntime;
}

interface HighlightIssue { name: string; issues: string[] }

const computeHighlightIssues = (rules: HighlightRule[]): HighlightIssue[] => {
    const out: HighlightIssue[] = [];
    for (const r of rules) {
        const risk = assessPattern(r.pattern || '');
        const issues: string[] = [];
        if (risk.syntaxError) issues.push('语法错误');
        if (risk.level === 'severe') issues.push('灾难性正则');
        if (issues.length) out.push({ name: r.name, issues });
    }
    return out;
};

const ShellSettingsModal: React.FC<ShellSettingsModalProps> = ({ isOpen, onClose, runtime, onApply, initial, aiRuntime }) => {
    const toast = useToast();
    const [settings, setSettings] = useState<ShellSettings | null>(initial ?? null);
    const [saving, setSaving] = useState(false);

    useEffect(() => {
        if (!isOpen) return;
        if (initial) { setSettings(initial); return; }
        let cancelled = false;
        runtime.load().then((s) => { if (!cancelled) setSettings(s); })
            .catch((e) => toast.error('读取设置失败: ' + (e?.message || e)));
        return () => { cancelled = true; };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [isOpen, runtime]);

    if (!isOpen) return null;

    const update = (patch: Partial<ShellSettings>) => {
        setSettings((prev) => {
            if (!prev) return prev;
            const next = { ...prev, ...patch };
            onApply(next);
            return next;
        });
    };

    const handleSave = async () => {
        if (!settings) return;
        setSaving(true);
        try {
            await runtime.save(settings);
            toast.success('设置已保存');
            onClose();
        } catch (e: any) {
            toast.error('保存失败: ' + (e?.message || e));
        } finally {
            setSaving(false);
        }
    };

    const issues = settings ? computeHighlightIssues(settings.highlightRules) : [];

    return (
        <div style={styles.overlay}>
            <div style={styles.modal}>
                <div style={styles.header}>
                    <h2 style={styles.title}>Shell 设置</h2>
                    <button style={styles.closeButton} onClick={onClose} aria-label="关闭">×</button>
                </div>
                <div style={styles.body}>
                    {!settings ? (
                        <div style={styles.loading}>加载中...</div>
                    ) : (
                        <>
                            <ThemeChoiceCard
                                theme={settings.theme}
                                onThemeChange={(theme) => update({ theme })}
                            />
                            <TerminalAppearanceCard
                                terminal={normalizeTerminalConfig(settings.terminal)}
                                onChange={(terminal) => update({ terminal })}
                            />
                            {aiRuntime && <AIConfigCard runtime={aiRuntime} />}
                            <div style={styles.card}>
                                <div style={styles.cardTitle}>补全延迟</div>
                                <div style={styles.row}>
                                    <div style={styles.rowLeft}>
                                        <div style={styles.rowLabel}>命令补全延迟</div>
                                        <div style={styles.rowDesc}>
                                            输入停顿多久后弹出补全建议（0–2000ms），值越小响应越快、请求越频繁。
                                        </div>
                                    </div>
                                    <div style={styles.rowRight}>
                                        <input
                                            type="number"
                                            min={0}
                                            max={2000}
                                            step={10}
                                            style={styles.numberInput}
                                            value={settings.completionDelay}
                                            onChange={(e) => update({ completionDelay: Math.max(0, Math.min(2000, Number(e.target.value) || 0)) })}
                                        />
                                        <span style={styles.unit}>ms</span>
                                    </div>
                                </div>
                                <div style={styles.row}>
                                    <div style={styles.rowLeft}>
                                        <label style={styles.rowLabel} htmlFor="shared-command-query-shortcut">命令查询快捷键</label>
                                        <div style={styles.rowDesc}>
                                            呼出 AI 命令生成弹窗的快捷键（支持 Ctrl+字母、Ctrl+Shift+字母 等格式）。
                                        </div>
                                    </div>
                                    <div style={styles.rowRight}>
                                        <input
                                            id="shared-command-query-shortcut"
                                            style={styles.shortcutInput}
                                            value={settings.commandQueryShortcut ?? 'Ctrl+K'}
                                            onChange={(e) => update({ commandQueryShortcut: e.target.value })}
                                            placeholder="例如：Ctrl+K"
                                        />
                                    </div>
                                </div>
                            </div>
                            <div style={styles.card}>
                                <div style={styles.cardTitle}>终端行为</div>
                                <div style={styles.row}>
                                    <div style={styles.rowLeft}>
                                        <div style={styles.rowLabel}>滚动回滚行数</div>
                                        <div style={styles.rowDesc}>
                                            终端保留的历史输出行数（500–10000），超出部分自动丢弃。
                                        </div>
                                    </div>
                                    <div style={styles.rowRight}>
                                        <input
                                            type="number"
                                            min={500}
                                            max={10000}
                                            step={500}
                                            style={styles.numberInput}
                                            value={settings.terminal.scrollback || 5000}
                                            onChange={(e) => {
                                                const next = Math.max(500, Math.min(10000, Math.round(Number(e.target.value) || 5000) / 500 * 500));
                                                update({ terminal: { ...settings.terminal, scrollback: next } });
                                            }}
                                        />
                                        <span style={styles.unit}>行</span>
                                    </div>
                                </div>
                                <div style={styles.row}>
                                    <div style={styles.rowLeft}>
                                        <div style={styles.rowLabel}>搜索</div>
                                        <div style={styles.rowDesc}>
                                            在终端内启用 Ctrl+F 搜索输出内容。
                                        </div>
                                    </div>
                                    <div style={styles.rowRight}>
                                        <Switch
                                            checked={settings.terminal.search_enabled ?? true}
                                            onChange={(search_enabled) => update({ terminal: { ...settings.terminal, search_enabled } })}
                                        />
                                    </div>
                                </div>
                                <div style={styles.row}>
                                    <div style={styles.rowLeft}>
                                        <div style={styles.rowLabel}>高亮</div>
                                        <div style={styles.rowDesc}>
                                            按下方规则高亮匹配到的终端输出内容。
                                        </div>
                                    </div>
                                    <div style={styles.rowRight}>
                                        <Switch
                                            checked={settings.terminal.highlight_enabled ?? true}
                                            onChange={(highlight_enabled) => update({ terminal: { ...settings.terminal, highlight_enabled } })}
                                        />
                                    </div>
                                </div>
                            </div>
                            <div style={styles.card}>
                                <div style={styles.cardTitle}>突出显示规则</div>
                                {issues.length > 0 && (
                                    <div style={styles.issueBanner}>
                                        <span style={{ color: colors.warning, display: 'inline-flex' }}>{TbInfoCircle({ size: 16 })}</span>
                                        <div style={{ fontSize: font.sm, color: colors.textSecondary }}>
                                            {issues.length} 条规则存在语法错误或灾难性正则，已自动失效，建议修改。
                                        </div>
                                    </div>
                                )}
                                <HighlightRulesModal
                                    isOpen={true}
                                    rules={settings.highlightRules}
                                    onChange={(highlightRules) => update({ highlightRules })}
                                    onClose={() => {}}
                                    embedded
                                />
                            </div>
                        </>
                    )}
                </div>
                <div style={styles.footer}>
                    <button style={styles.cancelButton} onClick={onClose} disabled={saving}>取消</button>
                    <button style={styles.saveButton} onClick={handleSave} disabled={saving || !settings}>
                        {saving ? '保存中...' : '保存'}
                    </button>
                </div>
            </div>
        </div>
    );
};

const styles: Record<string, React.CSSProperties> = {
    overlay: {
        position: 'fixed', top: 0, left: 0, right: 0, bottom: 0,
        backgroundColor: 'var(--overlay)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        zIndex: 1200,
    },
    modal: {
        width: 'min(680px, calc(100vw - 48px))',
        maxHeight: '86vh',
        backgroundColor: 'var(--bg-primary)',
        border: `1px solid ${colors.borderPrimary}`,
        borderRadius: radius.lg,
        display: 'flex', flexDirection: 'column',
        boxShadow: 'var(--shadow-dialog)',
    },
    header: {
        display: 'flex', justifyContent: 'space-between', alignItems: 'center',
        padding: 'var(--space-16) var(--space-20)', borderBottom: `1px solid ${colors.borderSubtle}`,
    },
    title: { margin: 0, fontSize: 'var(--font-size-md)', fontWeight: 600, color: colors.textPrimary },
    closeButton: {
        border: 'none', background: 'transparent', color: colors.textTertiary,
        fontSize: 'var(--font-size-2xl)', cursor: 'pointer', padding: '0 var(--space-4)', lineHeight: 1,
    },
    body: { flex: 1, overflowY: 'auto', padding: 'var(--space-16) var(--space-20)', display: 'flex', flexDirection: 'column', gap: 'var(--space-12)' },
    loading: { textAlign: 'center', padding: 'var(--space-48)', color: colors.textTertiary, fontSize: font.base },
    card: {
        backgroundColor: 'var(--bg-secondary)',
        border: `1px solid ${colors.borderSubtle}`,
        borderRadius: radius.md,
        padding: 'var(--space-16)',
        display: 'flex', flexDirection: 'column', gap: 'var(--space-12)',
    },
    cardTitle: { fontSize: font.lg, fontWeight: 600, color: colors.textPrimary },
    row: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 'var(--space-16)' },
    rowLeft: { display: 'flex', flexDirection: 'column', gap: 'var(--space-2)', minWidth: 0 },
    rowRight: { display: 'flex', alignItems: 'center', justifyContent: 'flex-end', flexShrink: 0, gap: 'var(--space-6)' },
    rowLabel: { fontSize: font.base, color: colors.textPrimary, fontWeight: 500 },
    rowDesc: { fontSize: font.sm, color: colors.textTertiary },
    numberInput: {
        width: '76px', padding: 'var(--space-6) var(--space-8)', textAlign: 'right',
        borderRadius: radius.sm, border: `1px solid ${colors.borderPrimary}`,
        backgroundColor: 'var(--bg-primary)', color: colors.textPrimary,
        fontSize: font.base, outline: 'none',
    },
    unit: { fontSize: font.sm, color: colors.textTertiary },
    shortcutInput: {
        width: '110px', padding: 'var(--space-6) var(--space-8)', textAlign: 'left',
        borderRadius: radius.sm, border: `1px solid ${colors.borderPrimary}`,
        backgroundColor: 'var(--bg-primary)', color: colors.textPrimary,
        fontSize: font.base, outline: 'none',
    },
    issueBanner: {
        display: 'flex', gap: 'var(--space-8)', alignItems: 'flex-start',
        padding: 'var(--space-8) var(--space-10)',
        borderRadius: radius.sm,
        backgroundColor: 'var(--bg-hover)',
        borderLeft: `3px solid ${colors.warning}`,
    },
    footer: {
        display: 'flex', justifyContent: 'flex-end', gap: 'var(--space-10)',
        padding: 'var(--space-14) var(--space-20)', borderTop: `1px solid ${colors.borderSubtle}`,
    },
    cancelButton: {
        padding: 'var(--space-8) var(--space-16)', borderRadius: radius.sm,
        border: `1px solid ${colors.borderPrimary}`,
        backgroundColor: 'transparent', color: colors.textSecondary,
        cursor: 'pointer', fontSize: font.base,
    },
    saveButton: {
        padding: 'var(--space-8) var(--space-20)', borderRadius: radius.sm,
        border: 'none', backgroundColor: colors.accent,
        color: 'var(--text-on-accent)',
        cursor: 'pointer', fontSize: font.base, fontWeight: 500,
    },
};

export default ShellSettingsModal;
