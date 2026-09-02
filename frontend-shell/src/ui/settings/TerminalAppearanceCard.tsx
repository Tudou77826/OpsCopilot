import React, { useEffect } from 'react';
import { TbCheck, TbMinus, TbPlus, TbRefresh } from 'react-icons/tb';
import {
    TERMINAL_FONT_OPTIONS,
    clampTerminalFontSize,
    MIN_TERMINAL_FONT_SIZE,
    MAX_TERMINAL_FONT_SIZE,
    DEFAULT_TERMINAL_FONT_SIZE,
} from '../Terminal/terminalAppearance';
import type { TerminalConfig } from '../Terminal/highlightTypes';
import { colors, radius, font } from './settingsStyles';

/**
 * 终端外观卡（字体族预览卡 + 字号步进 + 恢复默认）。
 * 从 Wails SettingsModal 的 renderTerminalAppearance 抽取为共享实现；
 * 样式自包含（hover 态经注入 <style>，等价 frontend/src/style.css 的
 * .terminal-font-card / .terminal-font-size-input）。
 */
export interface TerminalAppearanceCardProps {
    terminal: TerminalConfig;
    onChange: (next: TerminalConfig) => void;
}

let styleInjected = false;
const injectCardStyle = () => {
    if (styleInjected || typeof document === 'undefined') return;
    styleInjected = true;
    const style = document.createElement('style');
    style.textContent = `
        .shared-terminal-font-card { transition: border-color .15s, background-color .15s, transform .15s; }
        .shared-terminal-font-card:hover { border-color: var(--border-strong); background-color: var(--bg-tertiary); }
        .shared-terminal-font-card[aria-checked="true"]:hover { border-color: var(--accent); }
        .shared-terminal-font-size-input::-webkit-inner-spin-button,
        .shared-terminal-font-size-input::-webkit-outer-spin-button { -webkit-appearance: none; margin: 0; }
        .shared-terminal-font-size-input { -moz-appearance: textfield; }
    `;
    document.head.appendChild(style);
};

const TerminalAppearanceCard: React.FC<TerminalAppearanceCardProps> = ({ terminal, onChange }) => {
    useEffect(() => { injectCardStyle(); }, []);
    const normalized = { ...terminal, font_size: clampTerminalFontSize(terminal.font_size) };
    const update = (patch: Partial<TerminalConfig>) => onChange({ ...normalized, ...patch });

    return (
        <div style={styles.card}>
            <div style={styles.cardTitle}>外观与字体</div>
            <div style={{ ...styles.row, alignItems: 'flex-start', paddingBottom: '8px' }}>
                <div style={styles.rowLeft}>
                    <div style={styles.rowLabel}>字体</div>
                    <div style={styles.rowDesc}>选择终端使用的等宽字体，字体卡片中展示实际渲染效果</div>
                </div>
            </div>
            <div style={styles.fontList} role="radiogroup" aria-label="终端字体">
                {TERMINAL_FONT_OPTIONS.map(option => {
                    const selected = normalized.font_family === option.value;
                    return (
                        <button
                            key={option.value}
                            type="button"
                            className="shared-terminal-font-card"
                            role="radio"
                            aria-checked={selected}
                            aria-label={`${option.label}：${option.description}`}
                            style={{ ...styles.fontCard, ...(selected ? styles.fontCardSelected : {}) }}
                            onClick={() => update({ font_family: option.value })}
                        >
                            <span style={styles.fontHeader}>
                                <span>
                                    <strong style={styles.fontName}>{option.label}</strong>
                                    <span style={styles.fontDesc}>{option.description}</span>
                                </span>
                                <span style={{ ...styles.fontSelected, opacity: selected ? 1 : 0 }} aria-hidden="true">
                                    {TbCheck({ size: 14 })}
                                </span>
                            </span>
                            <span style={{ ...styles.fontSample, fontFamily: option.stack }}>
                                ops@node:~$ ls -la&nbsp;&nbsp;01Il0O&nbsp;&nbsp;()[]
                            </span>
                        </button>
                    );
                })}
            </div>
            <div style={styles.divider} />
            <div style={styles.row}>
                <div style={styles.rowLeft}>
                    <label style={styles.rowLabel} htmlFor="shared-terminal-font-size">字号</label>
                    <div style={styles.rowDesc}>
                        支持 {MIN_TERMINAL_FONT_SIZE}–{MAX_TERMINAL_FONT_SIZE}px；终端内可使用 Ctrl + 滚轮快速调整。
                    </div>
                </div>
                <div style={styles.rowRight}>
                    <div style={styles.sizeRow}>
                        <button
                            type="button"
                            style={styles.sizeButton}
                            onClick={() => update({ font_size: normalized.font_size - 1 })}
                            disabled={normalized.font_size <= MIN_TERMINAL_FONT_SIZE}
                            aria-label="减小终端字号"
                        >
                            {TbMinus({ size: 16 })}
                        </button>
                        <div style={styles.sizeInputWrap}>
                            <input
                                id="shared-terminal-font-size"
                                className="shared-terminal-font-size-input"
                                type="number"
                                min={MIN_TERMINAL_FONT_SIZE}
                                max={MAX_TERMINAL_FONT_SIZE}
                                style={styles.sizeInput}
                                value={normalized.font_size}
                                onChange={(e) => update({ font_size: Number(e.target.value) })}
                            />
                            <span style={styles.sizeUnit}>px</span>
                        </div>
                        <button
                            type="button"
                            style={styles.sizeButton}
                            onClick={() => update({ font_size: normalized.font_size + 1 })}
                            disabled={normalized.font_size >= MAX_TERMINAL_FONT_SIZE}
                            aria-label="增大终端字号"
                        >
                            {TbPlus({ size: 16 })}
                        </button>
                        <button
                            type="button"
                            style={styles.resetButton}
                            onClick={() => update({ font_family: 'JetBrains Mono', font_size: DEFAULT_TERMINAL_FONT_SIZE })}
                        >
                            {TbRefresh({ size: 15 })}
                            恢复默认
                        </button>
                    </div>
                </div>
            </div>
        </div>
    );
};

const styles: Record<string, React.CSSProperties> = {
    card: {
        backgroundColor: 'var(--bg-secondary)',
        border: `1px solid ${colors.borderSubtle}`,
        borderRadius: radius.md,
        padding: '16px',
        display: 'flex',
        flexDirection: 'column',
        gap: '12px',
    },
    cardTitle: { fontSize: font.lg, fontWeight: 600, color: colors.textPrimary },
    row: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px' },
    rowLeft: { display: 'flex', flexDirection: 'column', gap: '2px', minWidth: 0 },
    rowRight: { display: 'flex', alignItems: 'center', justifyContent: 'flex-end', flexShrink: 0 },
    rowLabel: { fontSize: font.base, color: colors.textPrimary, fontWeight: 500 },
    rowDesc: { fontSize: font.sm, color: colors.textTertiary },
    divider: { height: 1, backgroundColor: colors.borderSubtle },
    fontList: {
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))',
        gap: '8px',
    },
    fontCard: {
        display: 'flex',
        flexDirection: 'column',
        gap: '8px',
        padding: '10px 12px',
        borderRadius: radius.md,
        border: `1px solid ${colors.borderPrimary}`,
        backgroundColor: 'var(--bg-primary)',
        cursor: 'pointer',
        textAlign: 'left',
    },
    fontCardSelected: {
        border: `1px solid ${colors.accent}`,
        backgroundColor: 'var(--bg-hover)',
        boxShadow: '0 0 0 1px var(--accent)',
    },
    fontHeader: { display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: '8px' },
    fontName: { fontSize: font.base, color: colors.textPrimary, marginRight: '6px' },
    fontDesc: { fontSize: font.xs, color: colors.textTertiary },
    fontSelected: { color: colors.accent, display: 'inline-flex' },
    fontSample: {
        fontSize: '13px',
        color: colors.textSecondary,
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
    },
    sizeRow: { display: 'flex', alignItems: 'center', gap: '6px' },
    sizeButton: {
        width: '28px', height: '28px', padding: 0,
        display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
        borderRadius: radius.sm,
        border: `1px solid ${colors.borderPrimary}`,
        backgroundColor: 'var(--bg-primary)',
        color: colors.textPrimary, cursor: 'pointer',
    },
    sizeInputWrap: {
        display: 'flex', alignItems: 'center',
        border: `1px solid ${colors.borderPrimary}`,
        borderRadius: radius.sm,
        backgroundColor: 'var(--bg-primary)',
        overflow: 'hidden',
    },
    sizeInput: {
        width: '52px', padding: '5px 4px', textAlign: 'center',
        border: 'none', backgroundColor: 'transparent',
        color: colors.textPrimary, fontSize: font.base, outline: 'none',
    },
    sizeUnit: { padding: '0 8px', fontSize: font.sm, color: colors.textTertiary, backgroundColor: 'var(--bg-tertiary)', alignSelf: 'stretch', display: 'flex', alignItems: 'center' },
    resetButton: {
        display: 'inline-flex', alignItems: 'center', gap: '5px',
        marginLeft: '4px', padding: '5px 10px',
        borderRadius: radius.sm,
        border: `1px solid ${colors.borderPrimary}`,
        backgroundColor: 'var(--bg-hover)',
        color: colors.textPrimary, cursor: 'pointer', fontSize: font.sm,
    },
};

export default TerminalAppearanceCard;
