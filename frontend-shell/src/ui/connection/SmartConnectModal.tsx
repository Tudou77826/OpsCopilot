import React, { useState, useEffect } from 'react';
import { TbChevronDown, TbEdit, TbPlugConnected, TbPlus, TbServer, TbSparkles, TbTrash } from 'react-icons/tb';
import { ConnectionConfig } from '../types';
import ConnectionConfigForm from './ConnectionConfigForm';

interface SmartConnectModalProps {
    isOpen: boolean;
    onClose: () => void;
    onConnect: (configs: ConnectionConfig[]) => void;
    onParse: (input: string) => Promise<ConnectionConfig[]>;
    /** 打开时预填的连接配置（用于连接失败后带回，让用户改完重试） */
    initialConfigs?: ConnectionConfig[];
    /** 是否展示 AI 智能分析区。宿主无解析能力时隐藏，保留手动添加。 */
    showAi?: boolean;
}

const quickInputExamples = [
    {
        label: '跳板机',
        template: [
            '跳板机：<BastionIp>',
            'sopuser / changeme_123',
            '',
            '目标：<TargetIp1~4>',
            'sopuser / changeme_123',
            'root / changeme_123'
        ].join('\n')
    }
];

const SmartConnectModal: React.FC<SmartConnectModalProps> = ({ isOpen, onClose, onConnect, onParse, initialConfigs, showAi = true }) => {
    const [input, setInput] = useState('');
    const [isLoading, setIsLoading] = useState(false);
    const [parsedConfigs, setParsedConfigs] = useState<ConnectionConfig[]>([]);
    const [selectedIndices, setSelectedIndices] = useState<Set<number>>(new Set());
    const [expandedIndices, setExpandedIndices] = useState<Set<number>>(new Set());
    const [error, setError] = useState('');
    const [showErrorDetails, setShowErrorDetails] = useState(false);

    // 打开/关闭时的状态处理：
    //  - 关闭：重置为初始状态（下次打开若带 initialConfigs 会重新预填）
    //  - 打开：若带 initialConfigs（如连接失败带回的配置），预填进列表并全选+展开，
    //    方便用户直接修改后重试。不带则保持空白，由用户自行输入/解析。
    useEffect(() => {
        if (!isOpen) {
            setInput('');
            setParsedConfigs([]);
            setSelectedIndices(new Set());
            setExpandedIndices(new Set());
            setError('');
            return;
        }
        const seed = initialConfigs && initialConfigs.length > 0
            ? initialConfigs.map(c => ({ ...c, name: c.name || c.host }))
            : [];
        if (seed.length > 0) {
            setParsedConfigs(seed);
            setSelectedIndices(new Set(seed.map((_, i) => i)));
            // 只有一条时直接展开编辑，多条折叠让用户按需展开
            setExpandedIndices(new Set(seed.length === 1 ? [0] : []));
        }
    }, [isOpen, initialConfigs]);

    if (!isOpen) return null;

    const handleParse = async () => {
        if (!input.trim()) return;

        setIsLoading(true);
        setError('');
        setShowErrorDetails(false);
        try {
            const result = await onParse(input);
            // Prevent crash if result is null, and handle empty results
            const configs = result || [];

            if (configs.length === 0) {
                throw new Error("未识别到连接信息。请尝试提供更详细的信息（例如：'连接到 192.168.1.1 用户 root'）。");
            }

            const configsWithName = configs.map(c => ({
                ...c,
                name: c.name || c.host
            }));

            // Append new configs to existing ones
            const startIndex = parsedConfigs.length;
            setParsedConfigs(prev => [...prev, ...configsWithName]);

            // Select the newly added configs
            setSelectedIndices(prev => {
                const newSet = new Set(prev);
                configsWithName.forEach((_, i) => newSet.add(startIndex + i));
                return newSet;
            });

            // If it's the first batch, expand the first one
            if (parsedConfigs.length === 0 && configsWithName.length === 1) {
                setExpandedIndices(new Set([0]));
            }

            // 解析成功后保留输入框文本：用户可能只是打错字，改完重新点解析即可。
            // 不再清空 input，由用户自行决定是否修改或清空。
        } catch (e: any) {
            let errorMsg = e.message || e.toString();
            errorMsg = errorMsg.replace(/^Error: /, '');

            // Friendly error messages
            if (errorMsg.includes("TLS handshake timeout") || errorMsg.includes("timeout")) {
                errorMsg = "连接超时：无法连接到 AI 服务，请检查您的网络。";
            } else if (errorMsg.includes("Cannot read properties of null")) {
                errorMsg = "内部错误：收到无效的 AI 服务响应。";
            }

            setError(errorMsg);
        } finally {
            setIsLoading(false);
        }
    };

    const handleAddManual = () => {
        const newConfig: ConnectionConfig = {
            host: '',
            protocol: 'ssh',
            port: 22,
            user: 'root',
            name: '新连接'
        };
        const newIndex = parsedConfigs.length;
        setParsedConfigs(prev => [...prev, newConfig]);
        setSelectedIndices(prev => new Set(prev).add(newIndex));
        setExpandedIndices(prev => new Set(prev).add(newIndex));
    };

    const handleRemoveConfig = (index: number) => {
        const newConfigs = parsedConfigs.filter((_, i) => i !== index);
        setParsedConfigs(newConfigs);

        // Re-calculate selected/expanded indices is tricky because indices shift.
        // For simplicity, we just clear selections or try to preserve valid ones.
        // A robust way requires IDs, but index is simple for now.
        // Let's just clear selection/expansion to avoid bugs for this iteration.
        setSelectedIndices(new Set());
        setExpandedIndices(new Set());
    };

    const handleConnect = () => {
        // Clone selected configs to avoid mutating state directly
        const toConnect = parsedConfigs
            .filter((_, i) => selectedIndices.has(i))
            .map(c => ({ ...c }));

        // Grouping Logic for multiple connections
        if (toConnect.length > 1) {
            let groupName = `Batch-${new Date().toISOString().slice(0, 10)}`; // Default: Batch-YYYY-MM-DD

            // User requirement: Use Bastion IP if available
            const withBastion = toConnect.find(c => c.bastion && c.bastion.host);
            if (withBastion && withBastion.bastion) {
                groupName = withBastion.bastion.host;
            }

            // Assign group to all
            toConnect.forEach(c => {
                c.group = groupName;
            });
        }

        onConnect(toConnect);
        onClose();
    };

    const toggleSelection = (index: number) => {
        const newSet = new Set(selectedIndices);
        if (newSet.has(index)) {
            newSet.delete(index);
        } else {
            newSet.add(index);
        }
        setSelectedIndices(newSet);
    };

    const toggleExpand = (index: number) => {
        const newSet = new Set(expandedIndices);
        if (newSet.has(index)) {
            newSet.delete(index);
        } else {
            newSet.add(index);
        }
        setExpandedIndices(newSet);
    };

    const updateConfig = (index: number, field: keyof ConnectionConfig | 'bastion.host' | 'bastion.user' | 'bastion.password' | 'bastion.port', value: any) => {
        const newConfigs = [...parsedConfigs];
        const config = { ...newConfigs[index] };

        // Handle nested bastion fields
        if (field.startsWith('bastion.')) {
            if (!config.bastion) {
                config.bastion = { host: '', port: 22, user: '', name: 'Bastion' };
            }
            const bastionField = field.split('.')[1] as keyof ConnectionConfig;
            config.bastion = { ...config.bastion, [bastionField]: value };
        } else {
            // Handle root level fields
            // Name sync logic: if editing Host, and Name equals old Host (or is empty), update Name too
            if (field === 'host') {
                if (!config.name || config.name === config.host || config.name === '新连接') {
                    config.name = value;
                }
            }
            (config as any)[field] = value;
        }

        newConfigs[index] = config;
        setParsedConfigs(newConfigs);
    };

    const updateConfigObject = (index: number, next: ConnectionConfig) => {
        const newConfigs = [...parsedConfigs];
        newConfigs[index] = next;
        setParsedConfigs(newConfigs);
    };

    return (
        <div style={styles.overlay}>
            <div style={styles.modal}>
                <div style={styles.modalHeader}>
                    <div>
                        <h2 style={styles.title}>新建连接</h2>
                        <p style={styles.description}>粘贴连接说明、IP、账号或跳板机信息，自动拆成可连接配置。</p>
                    </div>
                </div>

                {showAi && (
                    <div style={styles.inputSection}>
                    <div style={styles.inputHeader}>
                        <div style={styles.inputTitleGroup}>
                            <div style={styles.inputTitle}>连接描述</div>
                            <div style={styles.inputHint}>写 IP、用户、密码、端口或跳板机信息，AI 会解析为连接配置。</div>
                        </div>
                        <div style={styles.inputActions}>
                            <button
                                type="button"
                                onClick={() => setInput(quickInputExamples[0].template)}
                                style={styles.templateButton}
                                title={quickInputExamples[0].template}
                            >
                                填入跳板机模板
                            </button>
                            <button
                                onClick={handleParse}
                                style={{
                                    ...styles.aiButton,
                                    ...((isLoading || !input.trim()) ? styles.aiButtonDisabled : {})
                                }}
                                disabled={isLoading || !input.trim()}
                            >
                                {TbSparkles({ size: 15 })}
                                {isLoading ? '分析中...' : '智能分析'}
                            </button>
                        </div>
                    </div>
                    {parsedConfigs.length === 0 ? (
                        <textarea
                            value={input}
                            onChange={(e) => setInput(e.target.value)}
                            placeholder={'例如：\n主机：192.168.1.10:22\n用户：root\n密码：<密码>'}
                            style={styles.textarea}
                            rows={3}
                            spellCheck={false}
                        />
                    ) : (
                        <textarea
                            value={input}
                            onChange={(e) => setInput(e.target.value)}
                            placeholder="继续输入连接描述，可追加分析..."
                            style={{ ...styles.textarea, ...styles.textareaCompact }}
                            rows={1}
                            spellCheck={false}
                        />
                    )}
                    {error && (
                        <div style={styles.errorContainer}>
                            <div style={styles.errorMessage}>
                                <span>⚠️ {error.includes('Raw:') ? '解析错误' : error}</span>
                                {error.includes('Raw:') && (
                                    <span
                                        style={styles.detailsLink}
                                        onClick={() => setShowErrorDetails(!showErrorDetails)}
                                    >
                                        {showErrorDetails ? '隐藏详情' : '显示详情'}
                                    </span>
                                )}
                            </div>
                            {showErrorDetails && error.includes('Raw:') && (
                                <pre style={styles.errorDetails}>
                                    {error}
                                </pre>
                            )}
                        </div>
                    )}
                    </div>
                )}
                {!showAi && error && (
                    <div style={{ padding: '0 22px' }}>
                        <div style={styles.errorContainer}>
                            <div style={styles.errorMessage}>
                                <span>⚠️ {error}</span>
                            </div>
                        </div>
                    </div>
                )}

                {/* Results List */}
                <div style={styles.resultSection}>
                    <div style={styles.resultHeader}>
                        <h3 style={styles.subtitle}>连接列表 ({parsedConfigs.length})</h3>
                        <span style={styles.selectedMeta}>已选 {selectedIndices.size}</span>
                        {/* "New Search" removed as requested */}
                    </div>

                    <div style={{
                        ...styles.list,
                        ...(parsedConfigs.length === 0 ? styles.listEmpty : styles.listWithResults)
                    }}>
                        {parsedConfigs.length === 0 && (
                            <div style={styles.emptyState}>
                                <div style={styles.emptyIcon}>
                                    {TbServer({ size: 22 })}
                                </div>
                                <div style={styles.emptyContent}>
                                    <div style={styles.emptyTitle}>暂无连接信息</div>
                                    <div style={styles.emptyText}>使用上方 AI 分析，或先手动添加一条空连接。</div>
                                </div>
                                <button onClick={handleAddManual} style={styles.emptyAction}>
                                    手动添加连接
                                </button>
                            </div>
                        )}
                        {parsedConfigs.map((config, i) => {
                            const isExpanded = expandedIndices.has(i);
                            const isSelected = selectedIndices.has(i);
                            return (
                                <div
                                    key={i}
                                    style={{
                                        ...styles.card,
                                        borderColor: isSelected ? 'var(--accent)' : 'var(--border)',
                                        boxShadow: isSelected ? '0 0 0 1px rgba(0, 122, 204, 0.18)' : 'none'
                                    }}
                                >
                                    {/* Card Header */}
                                    <div style={styles.cardHeader}>
                                        <input
                                            type="checkbox"
                                            checked={isSelected}
                                            onChange={() => toggleSelection(i)}
                                            style={styles.checkbox}
                                        />
                                        <div style={styles.headerInfo}>
                                            <input
                                                style={styles.headerNameInput}
                                                value={config.name || ''}
                                                onChange={(e) => updateConfig(i, 'name', e.target.value)}
                                                placeholder="连接名称"
                                                onClick={(e) => e.stopPropagation()}
                                            />
                                            <span style={styles.headerHost}>
                                                {TbServer({ size: 14 })}
                                                {config.host || '未填写主机'}
                                            </span>
                                        </div>
                                        <div style={styles.cardActions}>
                                            <button onClick={() => toggleExpand(i)} style={styles.iconButton} title={isExpanded ? '收起' : '编辑'} aria-label={isExpanded ? '收起' : '编辑'}>
                                                {isExpanded ? TbChevronDown({ size: 17 }) : TbEdit({ size: 17 })}
                                            </button>
                                            <button onClick={() => handleRemoveConfig(i)} style={styles.iconButton} title="移除" aria-label="移除">
                                                {TbTrash({ size: 17 })}
                                            </button>
                                        </div>
                                    </div>

                                    {/* Expanded Form */}
                                    {isExpanded && (
                                        <div style={styles.cardBody}>
                                            <ConnectionConfigForm
                                                config={config}
                                                onChange={(next) => updateConfigObject(i, next)}
                                                idPrefix={`smart-${i}`}
                                                showName={false}
                                                showGroup={false}
                                            />
                                        </div>
                                    )}
                                </div>
                            );
                        })}
                    </div>

                    <div style={styles.buttonGroup}>
                        <button onClick={handleAddManual} style={styles.secondaryButton}>
                            {TbPlus({ size: 16 })}
                            手动添加
                        </button>
                        <div style={{flex: 1}}></div>
                        <button onClick={onClose} style={styles.cancelButton}>取消</button>
                        <button
                            onClick={handleConnect}
                            style={{
                                ...styles.submitButton,
                                ...(selectedIndices.size === 0 ? styles.submitButtonDisabled : {})
                            }}
                            disabled={selectedIndices.size === 0}
                        >
                            {TbPlugConnected({ size: 16 })}
                            连接选中项 ({selectedIndices.size})
                        </button>
                    </div>
                </div>
            </div>
        </div>
    );
};

const styles = {
    overlay: {
        position: 'fixed' as const,
        top: 0, left: 0, right: 0, bottom: 0,
        backgroundColor: 'var(--overlay)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 1000,
        padding: '28px',
        boxSizing: 'border-box' as const,
    },
    modal: {
        backgroundColor: 'var(--bg-dialog)',
        border: '1px solid var(--border)',
        padding: '0',
        borderRadius: '8px',
        width: 'min(720px, 100%)',
        maxHeight: '86vh',
        display: 'flex',
        flexDirection: 'column' as const,
        boxShadow: 'var(--shadow-dialog)',
        color: 'var(--text-primary)',
        overflow: 'hidden',
    },
    modalHeader: {
        display: 'flex',
        alignItems: 'flex-start',
        justifyContent: 'space-between',
        gap: '24px',
        padding: '16px 22px 10px',
        borderBottom: '1px solid var(--border)',
    },
    title: {
        margin: 0,
        fontSize: '1.18rem',
        lineHeight: 1.25,
        fontWeight: 700,
        letterSpacing: 0,
    },
    description: {
        margin: '5px 0 0',
        color: 'var(--text-tertiary)',
        fontSize: '0.82rem',
        lineHeight: 1.45,
    },
    headerBadge: {
        display: 'inline-flex',
        alignItems: 'center',
        gap: '6px',
        padding: '5px 9px',
        borderRadius: '999px',
        backgroundColor: 'var(--bg-elevated)',
        border: '1px solid var(--border-strong)',
        color: 'var(--text-tertiary)',
        fontSize: '0.78rem',
        whiteSpace: 'nowrap' as const,
    },
    subtitle: {
        margin: 0,
        fontSize: '1rem',
        color: 'var(--text-primary)',
        fontWeight: 650,
    },
    inputSection: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '9px',
        padding: '12px 22px',
        borderBottom: '1px solid var(--border)',
        backgroundColor: 'var(--bg-dialog-section)',
    },
    inputHeader: {
        display: 'flex',
        alignItems: 'flex-start',
        justifyContent: 'space-between',
        gap: '16px',
        minWidth: 0,
    },
    inputTitleGroup: {
        minWidth: 0,
        flex: 1,
    },
    inputTitle: {
        color: 'var(--text-primary)',
        fontSize: '0.92rem',
        fontWeight: 650,
        lineHeight: 1.3,
    },
    inputHint: {
        color: 'var(--text-muted)',
        fontSize: '0.76rem',
        lineHeight: 1.35,
        marginTop: '2px',
    },
    inputActions: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        flex: '0 0 auto',
    },
    textarea: {
        flex: 1,
        width: '100%',
        minHeight: '136px',
        padding: '8px 10px',
        borderRadius: '6px',
        border: '1px solid var(--border)',
        backgroundColor: 'var(--bg-primary)',
        color: 'var(--text-primary)',
        fontSize: '0.84rem',
        lineHeight: 1.45,
        resize: 'vertical' as const,
        boxSizing: 'border-box' as const,
        fontFamily: 'inherit',
        outline: 'none',
    },
    textareaCompact: {
        minHeight: '34px',
        height: '34px',
        resize: 'none' as const,
        overflow: 'hidden',
        lineHeight: 1.3,
    },
    aiButton: {
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        gap: '7px',
        height: '34px',
        minWidth: '96px',
        padding: '0 12px',
        borderRadius: '6px',
        border: 'none',
        backgroundColor: 'var(--accent)',
        color: 'var(--text-on-accent)',
        cursor: 'pointer',
        fontWeight: 'bold' as const,
        fontSize: '0.84rem',
    },
    templateButton: {
        height: '34px',
        padding: '0 10px',
        borderRadius: '6px',
        border: '1px solid var(--border-strong)',
        backgroundColor: 'var(--bg-elevated)',
        color: 'var(--text-secondary)',
        cursor: 'pointer',
        fontSize: '0.82rem',
        whiteSpace: 'nowrap' as const,
    },
    aiButtonDisabled: {
        opacity: 0.48,
        cursor: 'not-allowed',
    },
    errorContainer: {
        backgroundColor: 'var(--danger-tint)',
        border: '1px solid var(--severity-danger)',
        borderRadius: '6px',
        padding: '9px 10px',
    },
    errorMessage: {
        color: 'var(--severity-danger)',
        fontSize: '0.9rem',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
    },
    detailsLink: {
        textDecoration: 'underline',
        cursor: 'pointer',
        fontSize: '0.8rem',
        color: 'var(--severity-danger)',
        marginLeft: '8px',
    },
    errorDetails: {
        marginTop: '8px',
        padding: '8px',
        backgroundColor: 'var(--bg-primary)',
        borderRadius: '4px',
        fontSize: '0.8rem',
        color: 'var(--text-secondary)',
        whiteSpace: 'pre-wrap' as const,
        wordBreak: 'break-word' as const,
        maxHeight: '200px',
        overflowY: 'auto' as const,
        border: '1px solid var(--border)',
    },
    resultSection: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '12px',
        overflow: 'hidden',
        flex: '1 1 auto',
        minHeight: 0,
    },
    resultHeader: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        padding: '11px 22px 0',
    },
    selectedMeta: {
        color: 'var(--text-tertiary)',
        fontSize: '0.8rem',
        padding: '3px 8px',
        border: '1px solid var(--border)',
        borderRadius: '999px',
    },
    resetButton: {
        background: 'none',
        border: 'none',
        color: 'var(--accent)',
        cursor: 'pointer',
        textDecoration: 'underline',
        fontSize: '0.9rem',
    },
    list: {
        overflowY: 'auto' as const,
        padding: '0 22px 4px',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '8px',
        minHeight: 0,
    },
    listEmpty: {
        flex: '0 0 auto',
        maxHeight: 'none',
    },
    listWithResults: {
        flex: '1 1 auto',
        maxHeight: '50vh',
    },
    emptyState: {
        display: 'flex',
        alignItems: 'center',
        gap: '14px',
        padding: '10px 12px',
        color: 'var(--text-tertiary)',
        border: '1px dashed var(--border)',
        borderRadius: '8px',
        backgroundColor: 'var(--bg-elevated)',
    },
    emptyIcon: {
        width: '34px',
        height: '34px',
        borderRadius: '8px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        backgroundColor: 'var(--bg-elevated)',
        color: 'var(--stage-blue)',
        flex: '0 0 auto',
    },
    emptyContent: {
        flex: 1,
        minWidth: 0,
    },
    emptyTitle: {
        color: 'var(--text-primary)',
        fontSize: '0.95rem',
        fontWeight: 650,
        marginBottom: '4px',
    },
    emptyText: {
        color: 'var(--text-tertiary)',
        fontSize: '0.84rem',
    },
    emptyAction: {
        padding: '7px 10px',
        borderRadius: '6px',
        border: '1px solid var(--border-strong)',
        backgroundColor: 'var(--bg-elevated)',
        color: 'var(--text-primary)',
        cursor: 'pointer',
        whiteSpace: 'nowrap' as const,
    },
    card: {
        backgroundColor: 'var(--bg-elevated)',
        border: '1px solid var(--border)',
        borderRadius: '7px',
        overflow: 'hidden',
        flex: '0 0 auto',
    },
    cardHeader: {
        display: 'flex',
        alignItems: 'center',
        padding: '8px 10px',
        backgroundColor: 'var(--bg-input)',
        cursor: 'pointer',
    },
    checkbox: {
        marginRight: '12px',
        cursor: 'pointer',
        width: '16px',
        height: '16px',
    },
    headerInfo: {
        flex: 1,
        display: 'flex',
        alignItems: 'center',
        gap: '12px',
        minWidth: 0,
    },
    headerNameInput: {
        backgroundColor: 'transparent',
        border: 'none',
        borderBottom: '1px solid transparent',
        color: 'var(--text-primary)',
        fontSize: '1rem',
        fontWeight: 'bold' as const,
        width: '160px',
        maxWidth: '42%',
        padding: '2px 0',
        outline: 'none',
    },
    headerHost: {
        display: 'inline-flex',
        alignItems: 'center',
        gap: '5px',
        color: 'var(--text-tertiary)',
        fontSize: '0.9rem',
        minWidth: 0,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap' as const,
    },
    cardActions: {
        display: 'flex',
        gap: '4px',
        flex: '0 0 auto',
    },
    iconButton: {
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: '28px',
        height: '28px',
        background: 'transparent',
        border: '1px solid transparent',
        borderRadius: '5px',
        color: 'var(--text-secondary)',
        padding: 0,
        cursor: 'pointer',
    },
    expandButton: {
        background: 'none',
        border: '1px solid var(--border-strong)',
        borderRadius: '4px',
        color: 'var(--text-secondary)',
        padding: '4px 8px',
        fontSize: '0.8rem',
        cursor: 'pointer',
    },
    cardBody: {
        padding: '12px 18px 12px 12px',
        backgroundColor: 'var(--bg-tertiary)',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '12px',
        maxHeight: '42vh',
        minHeight: '260px',
        overflowY: 'auto' as const,
    },
    row: {
        display: 'flex',
        gap: '16px',
    },
    fieldGroup: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '4px',
    },
    fieldLabel: {
        fontSize: '0.8rem',
        color: 'var(--text-tertiary)',
    },
    input: {
        padding: '8px',
        borderRadius: '4px',
        border: '1px solid var(--border-strong)',
        backgroundColor: 'var(--bg-primary)',
        color: 'var(--text-primary)',
        fontSize: '0.9rem',
    },
    bastionSection: {
        marginTop: '8px',
        borderTop: '1px solid var(--border-strong)',
        paddingTop: '8px',
    },
    bastionHeader: {
        display: 'flex',
        alignItems: 'center',
        marginBottom: '8px',
        color: 'var(--stage-orange)',
        fontWeight: 'bold' as const,
        fontSize: '0.9rem',
    },
    bastionBody: {
        paddingLeft: '16px',
        borderLeft: '2px solid var(--stage-orange)',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '12px',
    },
    buttonGroup: {
        display: 'flex',
        justifyContent: 'flex-end',
        alignItems: 'center',
        gap: '12px',
        padding: '11px 22px 13px',
        borderTop: '1px solid var(--border)',
        backgroundColor: 'var(--bg-tertiary)',
    },
    secondaryButton: {
        display: 'inline-flex',
        alignItems: 'center',
        gap: '6px',
        padding: '7px 11px',
        borderRadius: '6px',
        border: '1px solid var(--border-strong)',
        backgroundColor: 'var(--bg-elevated)',
        color: 'var(--text-primary)',
        cursor: 'pointer',
    },
    cancelButton: {
        padding: '7px 13px',
        borderRadius: '6px',
        border: '1px solid var(--border)',
        backgroundColor: 'transparent',
        color: 'var(--text-primary)',
        cursor: 'pointer',
    },
    submitButton: {
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        gap: '7px',
        padding: '7px 13px',
        borderRadius: '6px',
        border: 'none',
        backgroundColor: 'var(--accent)',
        color: 'var(--text-on-accent)',
        cursor: 'pointer',
        fontWeight: 'bold' as const,
    },
    submitButtonDisabled: {
        opacity: 0.48,
        cursor: 'not-allowed',
    },
};

export default SmartConnectModal;
