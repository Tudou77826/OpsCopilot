import React from 'react';
import { TbCheck, TbMinus, TbPlus, TbRefresh } from 'react-icons/tb';
import { DEFAULT_TERMINAL_FONT_SIZE, MAX_TERMINAL_FONT_SIZE, MIN_TERMINAL_FONT_SIZE, TERMINAL_FONT_OPTIONS, clampTerminalFontSize, normalizeTerminalConfig } from '../Terminal/terminalAppearance';
import type { TerminalConfig } from '../Terminal/highlightTypes';
import { productSettingsStyles as styles } from './productSettingsStyles';
export interface TerminalAppearanceCardProps { terminal: TerminalConfig; onChange(next: TerminalConfig): void }
export default function TerminalAppearanceCard({ terminal: value, onChange }: TerminalAppearanceCardProps) {
 const terminal = normalizeTerminalConfig(value);
 const fontSize = clampTerminalFontSize(terminal.font_size);
 const updateTerminal = (patch: Partial<TerminalConfig>) => onChange(normalizeTerminalConfig({ ...terminal, ...patch }));
        return (
            <div style={styles.card}>
                <div style={styles.cardTitle}>终端外观</div>
                {/* 字体：独立区块占满卡片宽度，卡片 grid 自适应多列 */}
                <div style={{ ...styles.row, ...styles.rowTop, paddingBottom: '8px' }}>
                    <div style={styles.rowLeft}>
                        <div style={styles.rowLabel} id="terminal-font-family-label">字体</div>
                        <div style={styles.rowDesc}>
                            选择终端使用的等宽字体，字体卡片中展示实际渲染效果
                        </div>
                    </div>
                </div>
                <div
                    style={styles.fontPreviewList}
                    role="radiogroup"
                    aria-labelledby="terminal-font-family-label"
                >
                    {TERMINAL_FONT_OPTIONS.map(option => {
                        const selected = terminal.font_family === option.value;
                        return (
                            <button
                                key={option.value}
                                type="button"
                                className="terminal-font-card"
                                role="radio"
                                aria-checked={selected}
                                aria-label={`${option.label}：${option.description}`}
                                style={{
                                    ...styles.fontPreviewCard,
                                    ...(selected ? styles.fontPreviewCardSelected : {}),
                                }}
                                onClick={() => updateTerminal({ font_family: option.value })}
                            >
                                <span style={styles.fontPreviewHeader}>
                                    <span>
                                        <strong style={styles.fontPreviewName}>{option.label}</strong>
                                        <span style={styles.fontPreviewDescription}>{option.description}</span>
                                    </span>
                                    <span style={{
                                        ...styles.fontSelectedIndicator,
                                        opacity: selected ? 1 : 0,
                                    }} aria-hidden="true">
                                        {TbCheck({ size: 14 })}
                                    </span>
                                </span>
                                <span style={{ ...styles.fontPreviewSample, fontFamily: option.stack }}>
                                    ops@node:~$ ls -la&nbsp;&nbsp;01Il0O&nbsp;&nbsp;()[]
                                </span>
                            </button>
                        );
                    })}
                </div>
                <div style={styles.cardDivider} />
                <div style={styles.row}>
                    <div style={styles.rowLeft}>
                        <label style={styles.rowLabel} htmlFor="terminal-font-size">字号</label>
                        <div style={styles.rowDesc}>
                            支持 {MIN_TERMINAL_FONT_SIZE}–{MAX_TERMINAL_FONT_SIZE}px；终端内可使用 Ctrl + 滚轮、Ctrl +/− 和 Ctrl + 0 快速调整。
                        </div>
                    </div>
                    <div style={styles.rowRight}>
                        <div style={styles.fontSizeRow}>
                            <button
                                type="button"
                                style={styles.fontSizeButton}
                                onClick={() => updateTerminal({ font_size: fontSize - 1 })}
                                disabled={fontSize <= MIN_TERMINAL_FONT_SIZE}
                                aria-label="减小终端字号"
                            >
                                {TbMinus({ size: 16 })}
                            </button>
                            <div style={styles.fontSizeInputWrap}>
                                <input
                                    id="terminal-font-size"
                                    className="terminal-font-size-input"
                                    type="number"
                                    min={MIN_TERMINAL_FONT_SIZE}
                                    max={MAX_TERMINAL_FONT_SIZE}
                                    style={styles.fontSizeInput}
                                    value={fontSize}
                                    onChange={(event) => updateTerminal({ font_size: Number(event.target.value) })}
                                />
                                <span style={styles.fontSizeUnit}>px</span>
                            </div>
                            <button
                                type="button"
                                style={styles.fontSizeButton}
                                onClick={() => updateTerminal({ font_size: fontSize + 1 })}
                                disabled={fontSize >= MAX_TERMINAL_FONT_SIZE}
                                aria-label="增大终端字号"
                            >
                                {TbPlus({ size: 16 })}
                            </button>
                            <button
                                type="button"
                                style={styles.resetAppearanceButton}
                                onClick={() => updateTerminal({ font_family: 'JetBrains Mono', font_size: DEFAULT_TERMINAL_FONT_SIZE })}
                            >
                                {TbRefresh({ size: 15 })}
                                恢复默认
                            </button>
                        </div>
                    </div>
                </div>
            </div>
        );
}
