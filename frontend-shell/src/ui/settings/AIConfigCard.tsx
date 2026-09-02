import React, { useEffect, useState } from 'react';
import { useToast } from '../feedback/Toast';
import { colors, radius, font } from './settingsStyles';

/**
 * AI 接入配置卡（迭代 A）。
 * 独立于 ShellSettingsRuntime：AI 配置是单独的持久化域（sidecar 数据目录 ai-config.json）。
 * 密钥边界（方案 D1）：status 只含脱敏提示（keyHint），保存时 apiKey 留空 = 保留已存密钥，
 * 明文密钥仅在用户主动保存这一次上行，读取路径永不回明文。
 */
export interface AIConfigStatus {
    configured: boolean;
    keyHint?: string;
    baseURL: string;
    fastModel: string;
    complexModel: string;
    source: 'file' | 'env' | 'none' | string;
}

export interface AIConfigUpdateInput {
    apiKey: string;
    baseURL: string;
    fastModel: string;
    complexModel: string;
}

export interface AIConfigRuntime {
    status(): Promise<AIConfigStatus>;
    save(update: AIConfigUpdateInput): Promise<AIConfigStatus>;
}

export interface AIConfigCardProps {
    runtime: AIConfigRuntime;
}

const AIConfigCard: React.FC<AIConfigCardProps> = ({ runtime }) => {
    const toast = useToast();
    const [status, setStatus] = useState<AIConfigStatus | null>(null);
    const [apiKey, setApiKey] = useState('');
    const [baseURL, setBaseURL] = useState('');
    const [fastModel, setFastModel] = useState('');
    const [complexModel, setComplexModel] = useState('');
    const [saving, setSaving] = useState(false);
    const [loadError, setLoadError] = useState('');

    useEffect(() => {
        let cancelled = false;
        runtime.status()
            .then((s) => {
                if (cancelled) return;
                setStatus(s);
                setBaseURL(s.baseURL);
                setFastModel(s.fastModel);
                setComplexModel(s.complexModel);
            })
            .catch((e) => { if (!cancelled) setLoadError((e as Error)?.message || String(e)); });
        return () => { cancelled = true; };
    }, [runtime]);

    const handleSave = async () => {
        setSaving(true);
        try {
            const next = await runtime.save({ apiKey: apiKey.trim(), baseURL: baseURL.trim(), fastModel: fastModel.trim(), complexModel: complexModel.trim() });
            setStatus(next);
            setBaseURL(next.baseURL);
            setFastModel(next.fastModel);
            setComplexModel(next.complexModel);
            setApiKey('');
            toast.success('AI 配置已保存');
        } catch (e: any) {
            toast.error('保存失败: ' + (e?.message || e));
        } finally {
            setSaving(false);
        }
    };

    const sourceLabel = status?.source === 'env' ? '环境变量' : status?.source === 'file' ? '本地配置' : '未配置';

    return (
        <div style={styles.card}>
            <div style={styles.cardTitle}>AI 接入</div>
            <div style={styles.row}>
                <div style={styles.rowLeft}>
                    <div style={styles.rowLabel}>API 密钥</div>
                    <div style={styles.rowDesc}>
                        {status?.configured
                            ? `已配置（${status.keyHint ?? ''} · ${sourceLabel}）。密钥只在本地后台保存，读取不回明文；留空表示保留现有密钥。`
                            : '未配置。密钥只在本地后台保存，用于命令生成与诊断等 AI 能力。'}
                    </div>
                </div>
                <div style={styles.rowRight}>
                    <input
                        type="password"
                        style={styles.input}
                        value={apiKey}
                        onChange={(e) => setApiKey(e.target.value)}
                        placeholder={status?.configured ? '留空保留现有密钥' : 'sk-…'}
                        autoComplete="off"
                    />
                </div>
            </div>
            <div style={styles.row}>
                <div style={styles.rowLeft}>
                    <label style={styles.rowLabel} htmlFor="ai-base-url">接口地址</label>
                    <div style={styles.rowDesc}>OpenAI 兼容端点（Base URL）。</div>
                </div>
                <div style={styles.rowRight}>
                    <input
                        id="ai-base-url"
                        style={styles.input}
                        value={baseURL}
                        onChange={(e) => setBaseURL(e.target.value)}
                        placeholder="https://api.deepseek.com/v1"
                    />
                </div>
            </div>
            <div style={styles.row}>
                <div style={styles.rowLeft}>
                    <label style={styles.rowLabel} htmlFor="ai-fast-model">快速模型</label>
                    <div style={styles.rowDesc}>单次生成类任务（命令生成、意图解析）使用的模型。</div>
                </div>
                <div style={styles.rowRight}>
                    <input
                        id="ai-fast-model"
                        style={styles.input}
                        value={fastModel}
                        onChange={(e) => setFastModel(e.target.value)}
                        placeholder="deepseek-chat"
                    />
                </div>
            </div>
            <div style={styles.row}>
                <div style={styles.rowLeft}>
                    <label style={styles.rowLabel} htmlFor="ai-complex-model">复杂模型</label>
                    <div style={styles.rowDesc}>诊断与知识问答等 Agent 任务使用的模型。</div>
                </div>
                <div style={styles.rowRight}>
                    <input
                        id="ai-complex-model"
                        style={styles.input}
                        value={complexModel}
                        onChange={(e) => setComplexModel(e.target.value)}
                        placeholder="deepseek-chat"
                    />
                </div>
            </div>
            {loadError && <div style={styles.errorText}>读取配置失败: {loadError}</div>}
            <div style={styles.footerRow}>
                <button style={styles.saveButton} onClick={handleSave} disabled={saving || !!loadError}>
                    {saving ? '保存中…' : '保存 AI 配置'}
                </button>
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
        display: 'flex', flexDirection: 'column', gap: '12px',
    },
    cardTitle: { fontSize: font.lg, fontWeight: 600, color: colors.textPrimary },
    row: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px' },
    rowLeft: { display: 'flex', flexDirection: 'column', gap: '2px', minWidth: 0, flex: 1 },
    rowRight: { display: 'flex', alignItems: 'center', justifyContent: 'flex-end', flexShrink: 0, maxWidth: '55%' },
    rowLabel: { fontSize: font.base, color: colors.textPrimary, fontWeight: 500 },
    rowDesc: { fontSize: font.sm, color: colors.textTertiary },
    input: {
        width: '100%',
        padding: '6px 8px',
        borderRadius: radius.sm,
        border: `1px solid ${colors.borderPrimary}`,
        backgroundColor: 'var(--bg-primary)',
        color: colors.textPrimary,
        fontSize: font.base, outline: 'none',
    },
    errorText: { fontSize: font.sm, color: colors.danger },
    footerRow: { display: 'flex', justifyContent: 'flex-end' },
    saveButton: {
        padding: '7px 16px', borderRadius: radius.sm,
        border: 'none', backgroundColor: colors.accent,
        color: 'var(--text-on-accent)',
        cursor: 'pointer', fontSize: font.base, fontWeight: 500,
    },
};

export default AIConfigCard;
