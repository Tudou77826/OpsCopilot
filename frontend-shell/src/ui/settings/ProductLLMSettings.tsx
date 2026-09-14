import React from 'react';
import { productSettingsStyles as styles } from './productSettingsStyles';
export interface ProductLLMConfig { APIKey: string; BaseURL: string; FastModel: string; ComplexModel: string }
export function ProductLLMSettings({ value, onChange, keyDescription }: { value: ProductLLMConfig; onChange(next: ProductLLMConfig): void; keyDescription?: string }) {
 const config = { llm: value };
 const handleChange = (_section: string, field: keyof ProductLLMConfig, next: string) => onChange({ ...value, [field]: next });
                return (
                    <div style={styles.settingsGroup}>
                        <div style={styles.card}>
                            <div style={styles.cardTitle}>基础配置</div>
                            <div style={styles.row}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>API 地址 (Base URL)</div>
                                    <div style={styles.rowDesc}>模型服务的 API 端点地址</div>
                                </div>
                                <div style={styles.rowRight}>
                                    <input
                                        style={styles.inputWide}
                                        aria-label="API 地址" value={config.llm.BaseURL}
                                        onChange={(e) => handleChange('llm', 'BaseURL', e.target.value)}
                                        placeholder="https://api.openai.com/v1"
                                    />
                                </div>
                            </div>
                            <div style={styles.row}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>API 密钥 (API Key)</div>
                                    <div style={styles.rowDesc}>{keyDescription || '用于调用模型服务的身份验证密钥'}</div>
                                </div>
                                <div style={styles.rowRight}>
                                    <input
                                        style={styles.inputWide}
                                        type="password"
                                        aria-label="API 密钥" autoComplete="off" value={config.llm.APIKey}
                                        onChange={(e) => handleChange('llm', 'APIKey', e.target.value)}
                                    />
                                </div>
                            </div>
                        </div>
                        <div style={styles.card}>
                            <div style={styles.cardTitle}>模型选择</div>
                            <div style={styles.row}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>快速模型（简单任务）</div>
                                    <div style={styles.rowDesc}>用于意图识别、命令补全等轻量任务</div>
                                </div>
                                <div style={styles.rowRight}>
                                    <input
                                        style={styles.inputWide}
                                        value={config.llm.FastModel}
                                        onChange={(e) => handleChange('llm', 'FastModel', e.target.value)}
                                        placeholder="deepseek-chat"
                                    />
                                </div>
                            </div>
                            <div style={styles.row}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>复杂模型（长上下文任务）</div>
                                    <div style={styles.rowDesc}>用于故障诊断、知识问答等复杂任务</div>
                                </div>
                                <div style={styles.rowRight}>
                                    <input
                                        style={styles.inputWide}
                                        value={config.llm.ComplexModel}
                                        onChange={(e) => handleChange('llm', 'ComplexModel', e.target.value)}
                                        placeholder="glm46"
                                    />
                                </div>
                            </div>
                        </div>
                    </div>
                );
}
