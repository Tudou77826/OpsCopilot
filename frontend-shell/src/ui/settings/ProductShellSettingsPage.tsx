import React from 'react';
import { TbInfoCircle } from 'react-icons/tb';
import type { Theme } from '../appearanceTypes';
import type { TerminalConfig, HighlightRule } from '../Terminal/highlightTypes';
import { normalizeTerminalConfig } from '../Terminal/terminalAppearance';
import { colors, font } from './settingsStyles';
import { productSettingsStyles as styles } from './productSettingsStyles';
import ThemeChoiceCard from './ThemeChoiceCard';
import TerminalAppearanceCard from './TerminalAppearanceCard';
import HighlightRulesModal from './HighlightRulesModal';
import KeysMap from './KeysMap';
export type ShellPageId = 'appearance' | 'highlight' | 'shortcuts';
export interface ShellPageConfig { terminal?: TerminalConfig; highlight_rules?: HighlightRule[]; command_query_shortcut: string }
export function ProductShellSettingsPage<T extends ShellPageConfig>({ activeTab, config, setConfig, theme, onThemeChange, highlightIssues }: {
 activeTab: ShellPageId; config: T; setConfig(value: T): void; theme: Theme; onThemeChange(theme: Theme): void;
 highlightIssues: { name: string; issues: string[] }[];
}) {
 const formatShortcutLabel = (value: string) => (value || '').trim() || 'Ctrl+K';
 switch(activeTab) {
            case 'appearance':
                return (
                    <div style={styles.settingsGroup}>
                        <ThemeChoiceCard theme={theme} onThemeChange={onThemeChange} />
                        {/* 终端外观已收纳在本页 */}
                        <TerminalAppearanceCard terminal={normalizeTerminalConfig(config.terminal)} onChange={terminal => setConfig({ ...config, terminal })} />
                    </div>
                );

            case 'highlight':
                return (
                    <div style={styles.settingsGroup}>
                        {highlightIssues.length > 0 && (
                            <div style={{ ...styles.attentionBanner, borderLeftColor: colors.warning, marginBottom: '4px' }}>
                                <span style={{ color: colors.warning }}>{TbInfoCircle({ size: 16 })}</span>
                                <div style={styles.attentionText}>
                                    <div style={{ marginBottom: '6px' }}>
                                        检测到 {highlightIssues.length} 条规则存在问题（语法错误或灾难性正则，已自动失效），建议前往下方编辑修改：
                                    </div>
                                    <ul style={{ margin: 0, paddingLeft: '20px' }}>
                                        {highlightIssues.map((it, idx) => (
                                            <li key={idx} style={{ fontSize: font.sm, color: colors.textSecondary }}>
                                                <strong>{it.name}</strong>：{it.issues.join('；')}
                                            </li>
                                        ))}
                                    </ul>
                                </div>
                            </div>
                        )}
                        <div style={styles.card}>
                            <div style={styles.cardTitle}>突出显示规则</div>
                            {/* 规则编辑直接内嵌在本页，无需二层弹窗 */}
                            <HighlightRulesModal
                                isOpen={true}
                                rules={config.highlight_rules || []}
                                onChange={(rules) => {
                                    setConfig({ ...config, highlight_rules: rules });
                                }}
                                onClose={() => {}}
                                embedded
                            />
                        </div>
                    </div>
                );

            case 'shortcuts':
                return (
                    <div style={styles.settingsGroup}>
                        <div style={styles.card}>
                            <div style={styles.cardTitle}>快捷键配置</div>
                            <div style={styles.row}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>命令查询快捷键</div>
                                    <div style={styles.rowDesc}>
                                        呼出命令查询弹窗的快捷键组合（支持 Ctrl+字母、Ctrl+Shift+字母 等格式）
                                    </div>
                                </div>
                                <div style={styles.rowRight}>
                                    <input
                                        style={styles.inputWide}
                                        value={formatShortcutLabel(config.command_query_shortcut)}
                                        onChange={(e) => {
                                            setConfig({
                                                ...config,
                                                command_query_shortcut: e.target.value
                                            });
                                        }}
                                        placeholder="例如：Ctrl+K"
                                    />
                                </div>
                            </div>
                        </div>
                        <div style={styles.card}>
                            <div style={styles.cardTitle}>快捷键说明</div>
                            <KeysMap commandQueryShortcut={formatShortcutLabel(config.command_query_shortcut)} />
                        </div>
                    </div>
                );


 }
}
