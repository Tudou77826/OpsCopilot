import React from 'react';
import { TbMoon, TbSun } from 'react-icons/tb';
import type { Theme } from '../appearanceTypes';
import { productSettingsStyles as styles } from './productSettingsStyles';
export interface ThemeChoiceCardProps { theme: Theme; onThemeChange(theme: Theme): void }
export default function ThemeChoiceCard({ theme, onThemeChange }: ThemeChoiceCardProps) {
 return (
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
                                    <div style={styles.themeChoiceRow} role="radiogroup" aria-label="界面主题">
                                        <button
                                            type="button"
                                            role="radio"
                                            aria-checked={theme === 'dark'}
                                            style={{
                                                ...styles.themeChoiceCard,
                                                ...(theme === 'dark' ? styles.themeChoiceCardActive : {}),
                                            }}
                                            onClick={() => onThemeChange?.('dark')}
                                        >
                                            {TbMoon({ size: 16 })}
                                            <span>暗色</span>
                                        </button>
                                        <button
                                            type="button"
                                            role="radio"
                                            aria-checked={theme === 'light'}
                                            style={{
                                                ...styles.themeChoiceCard,
                                                ...(theme === 'light' ? styles.themeChoiceCardActive : {}),
                                            }}
                                            onClick={() => onThemeChange?.('light')}
                                        >
                                            {TbSun({ size: 16 })}
                                            <span>亮色</span>
                                        </button>
                                    </div>
                                </div>
                            </div>
                        </div>
 );
}
