import React from 'react';
import { TbMoon, TbSun } from 'react-icons/tb';
import type { Theme } from '../appearanceTypes';
import { colors, radius, font } from './settingsStyles';

/**
 * 主题选择卡（暗色/亮色双卡单选）。
 * 从 Wails SettingsModal 外观页抽取为共享实现。
 */
export interface ThemeChoiceCardProps {
    theme: Theme;
    onThemeChange: (theme: Theme) => void;
}

const ThemeChoiceCard: React.FC<ThemeChoiceCardProps> = ({ theme, onThemeChange }) => (
    <div style={styles.card}>
        <div style={styles.cardTitle}>主题</div>
        <div style={styles.row}>
            <div style={styles.rowLeft}>
                <div style={styles.rowLabel}>界面主题</div>
                <div style={styles.rowDesc}>
                    切换后立即生效并保存到配置；终端配色、界面背景与文字颜色会同步适配。
                </div>
            </div>
            <div style={styles.rowRight}>
                <div style={styles.choiceRow} role="radiogroup" aria-label="界面主题">
                    <button
                        type="button"
                        role="radio"
                        aria-checked={theme === 'dark'}
                        style={{ ...styles.choiceCard, ...(theme === 'dark' ? styles.choiceCardActive : {}) }}
                        onClick={() => onThemeChange('dark')}
                    >
                        {TbMoon({ size: 16 })}
                        <span>暗色</span>
                    </button>
                    <button
                        type="button"
                        role="radio"
                        aria-checked={theme === 'light'}
                        style={{ ...styles.choiceCard, ...(theme === 'light' ? styles.choiceCardActive : {}) }}
                        onClick={() => onThemeChange('light')}
                    >
                        {TbSun({ size: 16 })}
                        <span>亮色</span>
                    </button>
                </div>
            </div>
        </div>
    </div>
);

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
    choiceRow: { display: 'flex', gap: '8px' },
    choiceCard: {
        display: 'inline-flex', alignItems: 'center', gap: '6px',
        padding: '8px 14px',
        borderRadius: radius.sm,
        border: `1px solid ${colors.borderPrimary}`,
        backgroundColor: 'var(--bg-primary)',
        color: colors.textSecondary,
        cursor: 'pointer', fontSize: font.base,
    },
    choiceCardActive: {
        border: `1px solid ${colors.accent}`,
        backgroundColor: 'var(--bg-hover)',
        color: colors.textPrimary,
        boxShadow: '0 0 0 1px var(--accent)',
    },
};

export default ThemeChoiceCard;
