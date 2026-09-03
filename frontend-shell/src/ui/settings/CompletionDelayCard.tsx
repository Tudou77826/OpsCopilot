import React from 'react';
import { productSettingsStyles as styles } from './productSettingsStyles';
export function CompletionDelayCard({ value, onChange }: { value: number; onChange(value: number): void }) {
 return (
                        <div style={styles.card}>
                            <div style={styles.cardTitle}>高级功能</div>
                            <div style={styles.row}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>命令补全延迟时间 (毫秒)</div>
                                    <div style={styles.rowDesc}>
                                        设置命令自动补全的触发延迟时间（毫秒）。设置为 0 表示立即触发，设置为 2000 表示延迟 2 秒触发
                                    </div>
                                </div>
                                <div style={styles.rowRight}>
                                    <input
                                        style={styles.inputWide}
                                        type="number"
                                        min="0"
                                        max="2000"
                                        step="50"
                                        value={value}
                                        onChange={(e) => {
                                            const next = Number(e.target.value);
                                            onChange(Math.max(0, Math.min(2000, Number.isFinite(next) ? next : 150)));
                                        }}
                                    />
                                </div>
                            </div>
                        </div>
 );
}
