import React, { useState, useEffect, useRef, useMemo } from 'react';
import KeysMap from './KeysMap';
import HighlightRulesModal from './HighlightRulesModal';
import { HighlightRule, TerminalConfig } from '../Terminal/highlightTypes';

interface AppConfig {
    llm: {
        APIKey: string;
        BaseURL: string;
        FastModel: string;
        ComplexModel: string;
        Model?: string;
    };
    prompts: {
        [key: string]: string;
    };
    log: {
        dir: string;
    };
    docs: {
        dir: string;
    };
    experimental?: {
        monitoring?: boolean;
        external_troubleshoot_script_path?: string;
    };
    terminal?: TerminalConfig;
    highlight_rules?: HighlightRule[];
    completion_delay: number;
    command_query_shortcut: string;
}

interface SettingsModalProps {
    isOpen: boolean;
    onClose: () => void;
    isBroadcastMode?: boolean;
    onToggleBroadcast?: (enabled: boolean) => void;
    onCompletionDelayChange?: (delay: number) => void;
    onOpenFileTransfer?: () => void;
    onTerminalConfigChange?: (cfg: TerminalConfig) => void;
    onHighlightRulesChange?: (rules: HighlightRule[]) => void;
}

type TabId = 'llm' | 'prompts' | 'terminal' | 'highlight' | 'shortcuts' | 'broadcast' | 'filetransfer' | 'experimental';

interface NavItem {
    id: TabId;
    label: string;
    icon: string;
    category: string;
}

const SettingsModal: React.FC<SettingsModalProps> = ({
    isOpen,
    onClose,
    isBroadcastMode,
    onToggleBroadcast,
    onCompletionDelayChange,
    onOpenFileTransfer,
    onTerminalConfigChange,
    onHighlightRulesChange
}) => {
    const [config, setConfig] = useState<AppConfig | null>(null);
    const [loading, setLoading] = useState(false);
    const [msg, setMsg] = useState('');
    const [activeTab, setActiveTab] = useState<TabId>('llm');
    const [rulesModalOpen, setRulesModalOpen] = useState(false);
    const [importDir, setImportDir] = useState('');
    const [importLoading, setImportLoading] = useState(false);
    const [importMsg, setImportMsg] = useState('');
    const [searchQuery, setSearchQuery] = useState('');
    const searchInputRef = useRef<HTMLInputElement>(null);

    // Navigation items structure
    const navItems: NavItem[] = [
        { id: 'llm', label: '模型服务', icon: '🤖', category: 'AI' },
        { id: 'prompts', label: 'AI提示词', icon: '💬', category: 'AI' },
        { id: 'terminal', label: '终端设置', icon: '🖥️', category: '终端' },
        { id: 'highlight', label: '突出显示', icon: '🎨', category: '终端' },
        { id: 'shortcuts', label: '快捷键', icon: '⌨️', category: '交互' },
        { id: 'broadcast', label: '多窗口', icon: '🪟', category: '交互' },
        { id: 'filetransfer', label: '文件传输', icon: '📁', category: '工具' },
        { id: 'experimental', label: '高级选项', icon: '🔧', category: '系统' },
    ];

    // Filter navigation items based on search query
    const filteredNavItems = useMemo(() => {
        if (!searchQuery.trim()) {
            return navItems;
        }
        const query = searchQuery.toLowerCase();
        return navItems.filter(item =>
            item.label.toLowerCase().includes(query) ||
            item.category.toLowerCase().includes(query) ||
            item.id.toLowerCase().includes(query)
        );
    }, [searchQuery, navItems]);

    useEffect(() => {
        if (isOpen) {
            loadSettings();
            setMsg('');
            setImportDir('');
            setImportMsg('');
            setSearchQuery('');
            setActiveTab('llm');
        }
    }, [isOpen]);

    // Focus search box when tab changes
    useEffect(() => {
        if (isOpen && searchInputRef.current) {
            searchInputRef.current.focus();
        }
    }, [activeTab, isOpen]);

    // Keyboard shortcuts
    useEffect(() => {
        const handleKeyDown = (e: KeyboardEvent) => {
            if (!isOpen) return;

            // Ctrl/Cmd + S: Save
            if ((e.ctrlKey || e.metaKey) && e.key === 's') {
                e.preventDefault();
                handleSave();
            }

            // Escape: Close
            if (e.key === 'Escape') {
                handleClose();
            }

            // Ctrl/Cmd + F: Focus search
            if ((e.ctrlKey || e.metaKey) && e.key === 'f') {
                e.preventDefault();
                searchInputRef.current?.focus();
            }
        };

        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [isOpen, config]);

    // Auto-select first result when searching
    useEffect(() => {
        if (searchQuery.trim() && filteredNavItems.length > 0) {
            const firstVisible = filteredNavItems[0];
            if (activeTab !== firstVisible.id) {
                setActiveTab(firstVisible.id);
            }
        }
    }, [searchQuery, filteredNavItems]);

    const loadSettings = async () => {
        setLoading(true);
        try {
            // @ts-ignore
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.GetSettings) {
                // @ts-ignore
                const cfg = await window.go.main.App.GetSettings();
                const llmCfg = cfg.llm || {};
                const fastModel = llmCfg.FastModel || llmCfg.Model || '';
                const complexModel = llmCfg.ComplexModel || '';
                const terminal: TerminalConfig = cfg.terminal || { scrollback: 5000, search_enabled: true, highlight_enabled: true };
                const highlight_rules: HighlightRule[] = Array.isArray(cfg.highlight_rules) ? cfg.highlight_rules : [];
                setConfig({
                    ...cfg,
                    llm: {
                        ...llmCfg,
                        FastModel: fastModel,
                        ComplexModel: complexModel,
                    },
                    experimental: {
                        external_troubleshoot_script_path: cfg.experimental?.external_troubleshoot_script_path || '',
                    },
                    terminal,
                    highlight_rules,
                    command_query_shortcut: cfg.command_query_shortcut || 'Ctrl+K',
                });
            }
        } catch (e) {
            console.error(e);
            setMsg('加载设置失败');
        } finally {
            setLoading(false);
        }
    };

    const formatShortcutLabel = (shortcut: string) => {
        const normalized = (shortcut || '').trim();
        return normalized || 'Ctrl+K';
    };

    const handleSave = async () => {
        if (!config) return;
        setLoading(true);
        try {
            // @ts-ignore
            const err = await window.go.main.App.SaveSettings(config);
            if (err) {
                setMsg('错误: ' + err);
            } else {
                setMsg('设置已保存！');
                if (onCompletionDelayChange && config.completion_delay !== undefined) {
                    onCompletionDelayChange(config.completion_delay);
                }
                if (onTerminalConfigChange && config.terminal) {
                    onTerminalConfigChange(config.terminal);
                }
                if (onHighlightRulesChange) {
                    onHighlightRulesChange(config.highlight_rules || []);
                }
                setTimeout(() => {
                    setMsg('');
                    onClose();
                }, 1000);
            }
        } catch (e: any) {
            setMsg('错误: ' + e.toString());
        } finally {
            setLoading(false);
        }
    };

    const handleClose = () => {
        onClose();
    };

    const handleChange = (section: keyof AppConfig, key: string, value: string) => {
        if (!config) return;
        const sectionValue = config[section];
        if (typeof sectionValue === 'object' && sectionValue !== null) {
            setConfig({
                ...config,
                [section]: {
                    ...sectionValue,
                    [key]: value
                }
            });
        }
    };

    const handleCompletionDelayChange = (value: number) => {
        if (!config) return;
        setConfig({
            ...config,
            completion_delay: value
        });
    };

    const handlePromptChange = (key: string, value: string) => {
        if (!config) return;
        setConfig({
            ...config,
            prompts: {
                ...config.prompts,
                [key]: value
            }
        });
    };

    const handleImportConfig = async () => {
        const dir = (importDir || '').trim();
        if (!dir) {
            setImportMsg('请输入旧版本目录路径');
            return;
        }
        setImportLoading(true);
        setImportMsg('正在导入配置...');
        try {
            // @ts-ignore
            const result = await window.go.main.App.ImportConfigFromDirectory(dir);
            setImportMsg(result || '导入完成');
            if (typeof result === 'string' && (result.includes('已成功导入') || result.includes('配置导入成功'))) {
                await loadSettings();
            }
        } catch (e: any) {
            setImportMsg('导入失败: ' + e.toString());
        } finally {
            setImportLoading(false);
        }
    };

    if (!isOpen || !config) return null;

    // Render tab content
    const renderTabContent = () => {
        switch (activeTab) {
            case 'llm':
                return (
                    <div style={styles.settingsGroup}>
                        <div style={styles.groupTitle}>基础配置</div>
                        <div style={styles.settingItem}>
                            <label style={styles.settingLabel}>API 地址 (Base URL)</label>
                            <input
                                style={styles.input}
                                value={config.llm.BaseURL}
                                onChange={(e) => handleChange('llm', 'BaseURL', e.target.value)}
                                placeholder="https://api.openai.com/v1"
                            />
                        </div>
                        <div style={styles.settingItem}>
                            <label style={styles.settingLabel}>API 密钥 (API Key)</label>
                            <input
                                style={styles.input}
                                type="password"
                                value={config.llm.APIKey}
                                onChange={(e) => handleChange('llm', 'APIKey', e.target.value)}
                            />
                        </div>
                        <div style={styles.groupTitle}>模型选择</div>
                        <div style={styles.settingItem}>
                            <label style={styles.settingLabel}>快速模型（简单任务）</label>
                            <input
                                style={styles.input}
                                value={config.llm.FastModel}
                                onChange={(e) => handleChange('llm', 'FastModel', e.target.value)}
                                placeholder="deepseek-chat"
                            />
                        </div>
                        <div style={styles.settingItem}>
                            <label style={styles.settingLabel}>复杂模型（长上下文任务）</label>
                            <input
                                style={styles.input}
                                value={config.llm.ComplexModel}
                                onChange={(e) => handleChange('llm', 'ComplexModel', e.target.value)}
                                placeholder="glm46"
                            />
                        </div>
                    </div>
                );

            case 'prompts':
                return (
                    <div style={styles.settingsGroup}>
                        <div style={styles.groupTitle}>系统提示词</div>
                        {[
                            { key: 'smart_connect', label: '智能连接系统提示词 (Smart Connect)' },
                            { key: 'qa_prompt', label: 'AI 问答提示词 (AI Chat Agent)' },
                            { key: 'troubleshoot_prompt', label: '问题排查提示词 (Troubleshooting Agent)' },
                            { key: 'conclusion_prompt', label: '故障总结提示词 (Conclusion Agent)' },
                            { key: 'polish_prompt', label: '内容润色提示词 (Polishing Agent)' },
                        ].map(({ key, label }) => (
                            <div key={key} style={styles.settingItem}>
                                <label style={styles.settingLabel}>{label}</label>
                                <textarea
                                    style={styles.textarea}
                                    value={config.prompts[key] || ''}
                                    onChange={(e) => handlePromptChange(key, e.target.value)}
                                    rows={8}
                                />
                            </div>
                        ))}
                    </div>
                );

            case 'terminal':
                return (
                    <div style={styles.settingsGroup}>
                        <div style={styles.groupTitle}>显示设置</div>
                        <div style={styles.settingItem}>
                            <label style={styles.settingLabel}>Scrollback 历史行数</label>
                            <input
                                style={styles.input}
                                type="number"
                                min="500"
                                max="20000"
                                step="500"
                                value={config.terminal?.scrollback ?? 5000}
                                onChange={(e) => {
                                    const v = Math.max(500, Math.min(20000, parseInt(e.target.value) || 5000));
                                    setConfig({
                                        ...config,
                                        terminal: {
                                            ...(config.terminal || { scrollback: 5000, search_enabled: true, highlight_enabled: true }),
                                            scrollback: v
                                        }
                                    });
                                }}
                            />
                            <div style={styles.settingDescription}>
                                影响可搜索与可高亮的历史行数；调整后建议重启应用使其完全生效
                            </div>
                        </div>
                    </div>
                );

            case 'highlight':
                return (
                    <div style={styles.settingsGroup}>
                        <div style={styles.groupTitle}>突出显示规则</div>
                        <div style={styles.settingItem}>
                            <label style={styles.settingLabel}>管理突出显示集</label>
                            <button
                                onClick={() => setRulesModalOpen(true)}
                                style={styles.secondaryButton}
                            >
                                打开突出显示设置
                            </button>
                            <div style={styles.settingDescription}>
                                当前已启用 {config.highlight_rules?.filter(r => r.is_enabled).length || 0} 条规则
                            </div>
                        </div>
                    </div>
                );

            case 'shortcuts':
                return (
                    <div style={styles.settingsGroup}>
                        <div style={styles.groupTitle}>键盘快捷键</div>
                        <KeysMap commandQueryShortcut={formatShortcutLabel(config.command_query_shortcut)} />
                    </div>
                );

            case 'broadcast':
                return (
                    <div style={styles.settingsGroup}>
                        <div style={styles.groupTitle}>多窗口广播模式</div>
                        <div style={styles.settingItem}>
                            <label style={styles.settingLabel}>启用广播模式</label>
                            <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                                <label style={styles.switch}>
                                    <input
                                        type="checkbox"
                                        checked={!!isBroadcastMode}
                                        onChange={(e) => {
                                            if (onToggleBroadcast) {
                                                onToggleBroadcast(e.target.checked);
                                            }
                                        }}
                                    />
                                    <span style={styles.slider}></span>
                                </label>
                                <span style={{ color: '#ccc', fontSize: '0.9rem' }}>
                                    {isBroadcastMode ? '已开启' : '已关闭'}
                                </span>
                            </div>
                            <div style={styles.settingDescription}>
                                开启后，默认将当前所有打开的终端加入广播组。您可以在标签页上单独切换每个终端的广播状态。
                            </div>
                        </div>
                    </div>
                );

            case 'filetransfer':
                return (
                    <div style={styles.settingsGroup}>
                        <div style={styles.groupTitle}>文件传输</div>
                        <div style={styles.settingItem}>
                            <label style={styles.settingLabel}>打开文件传输窗口</label>
                            <button
                                onClick={() => {
                                    if (onOpenFileTransfer) onOpenFileTransfer();
                                    onClose();
                                }}
                                style={styles.secondaryButton}
                            >
                                打开文件传输
                            </button>
                            <div style={styles.settingDescription}>
                                打开后可在终端旁边并行使用文件传输功能
                            </div>
                        </div>
                    </div>
                );

            case 'experimental':
                return (
                    <div style={styles.settingsGroup}>
                        <div style={styles.groupTitle}>目录设置</div>
                        <div style={styles.settingItem}>
                            <label style={styles.settingLabel}>日志目录</label>
                            <input
                                style={styles.input}
                                value={config.log?.dir || ''}
                                onChange={(e) => handleChange('log', 'dir', e.target.value)}
                                placeholder="例如：C:\\Users\\xxx\\Logs"
                            />
                            <div style={styles.settingDescription}>
                                日志文件存储目录，留空使用默认路径
                            </div>
                        </div>
                        <div style={styles.settingItem}>
                            <label style={styles.settingLabel}>知识库目录</label>
                            <input
                                style={styles.input}
                                value={config.docs?.dir || ''}
                                onChange={(e) => handleChange('docs', 'dir', e.target.value)}
                                placeholder="例如：C:\\Users\\xxx\\Documents\\knowledge"
                            />
                            <div style={styles.settingDescription}>
                                本地文档知识库目录，用于 AI 问答增强
                            </div>
                        </div>
                        <div style={styles.groupTitle}>快捷键设置</div>
                        <div style={styles.settingItem}>
                            <label style={styles.settingLabel}>命令查询快捷键</label>
                            <input
                                style={styles.input}
                                value={formatShortcutLabel(config.command_query_shortcut)}
                                onChange={(e) => {
                                    setConfig({
                                        ...config,
                                        command_query_shortcut: e.target.value
                                    });
                                }}
                                placeholder="例如：Ctrl+K"
                            />
                            <div style={styles.settingDescription}>
                                呼出命令查询弹窗的快捷键组合（支持 Ctrl+字母、Ctrl+Shift+字母 等格式）
                            </div>
                        </div>
                        <div style={styles.groupTitle}>高级功能</div>
                        <div style={styles.settingItem}>
                            <label style={styles.settingLabel}>命令补全延迟时间 (毫秒)</label>
                            <input
                                style={styles.input}
                                type="number"
                                min="0"
                                max="2000"
                                step="50"
                                value={config.completion_delay || 150}
                                onChange={(e) => {
                                    const value = parseInt(e.target.value) || 150;
                                    setConfig({
                                        ...config,
                                        completion_delay: Math.max(0, Math.min(2000, value))
                                    });
                                }}
                            />
                            <div style={styles.settingDescription}>
                                设置命令自动补全的触发延迟时间（毫秒）。设置为 0 表示立即触发，设置为 2000 表示延迟 2 秒触发
                            </div>
                        </div>
                        <div style={styles.settingItem}>
                            <label style={styles.settingLabel}>MCP 服务器路径</label>
                            <input
                                style={styles.input}
                                value={config.experimental?.external_troubleshoot_script_path || ''}
                                onChange={(e) => handleChange('experimental', 'external_troubleshoot_script_path', e.target.value)}
                                placeholder="例如：C:\\mcp-servers\\diagnostic-server.exe"
                            />
                            <div style={styles.settingDescription}>
                                MCP (Model Context Protocol) 服务器路径。配置后，AI 在问题排查时可调用 MCP 提供的诊断工具
                            </div>
                        </div>
                        <div style={styles.groupTitle}>配置管理</div>
                        <div style={styles.settingItem}>
                            <label style={styles.settingLabel}>导入旧版本配置</label>
                            <div style={{ display: 'flex', gap: '10px', alignItems: 'center', flexWrap: 'wrap' as const }}>
                                <input
                                    style={{ ...styles.input, flex: 1, minWidth: '320px' }}
                                    value={importDir}
                                    onChange={(e) => setImportDir(e.target.value)}
                                    placeholder="例如：C:\\Users\\xxx\\OldOpsCopilot"
                                />
                                <button
                                    onClick={handleImportConfig}
                                    style={styles.secondaryButton}
                                    disabled={importLoading}
                                >
                                    {importLoading ? '正在导入...' : '开始导入'}
                                </button>
                            </div>
                            {importMsg ? (
                                <div style={styles.settingDescription}>{importMsg}</div>
                            ) : (
                                <div style={styles.settingDescription}>
                                    支持导入 config.json / prompts.json / quick_commands.json / highlight_rules.json；导入前会自动备份当前配置到 .bak 文件
                                </div>
                            )}
                        </div>
                    </div>
                );

            default:
                return null;
        }
    };

    // Get current breadcrumb path
    const getBreadcrumb = () => {
        const currentItem = navItems.find(item => item.id === activeTab);
        return `系统设置 > ${currentItem?.label}`;
    };

    return (
        <div style={styles.overlay}>
            <style>{`
                input:checked + span {
                    background-color: #2196F3 !important;
                }
                input:focus + span {
                    box-shadow: 0 0 1px #2196F3;
                }
                input:checked + span:before {
                    transform: translateX(20px);
                }
                label input {
                    opacity: 0;
                    width: 0;
                    height: 0;
                }
            `}</style>
            <div style={styles.modal}>
                {/* Header */}
                <div style={styles.header}>
                    <h2 style={styles.title}>系统设置</h2>
                    <button onClick={handleClose} style={styles.closeBtn}>×</button>
                </div>

                {/* Main Content Area */}
                <div style={styles.mainContent}>
                    {/* Left Sidebar */}
                    <div style={styles.sidebar}>
                        <div style={styles.searchBox}>
                            <input
                                ref={searchInputRef}
                                style={styles.searchInput}
                                placeholder="🔍 搜索设置..."
                                value={searchQuery}
                                onChange={(e) => setSearchQuery(e.target.value)}
                            />
                        </div>
                        <nav style={styles.nav}>
                            {filteredNavItems.length > 0 ? (
                                filteredNavItems.map((item) => (
                                    <div
                                        key={item.id}
                                        style={{
                                            ...styles.navItem,
                                            ...(activeTab === item.id ? styles.navItemActive : {})
                                        }}
                                        onClick={() => setActiveTab(item.id)}
                                    >
                                        <span style={styles.navIcon}>{item.icon}</span>
                                        <span style={styles.navText}>{item.label}</span>
                                    </div>
                                ))
                            ) : (
                                <div style={styles.noResults}>没有找到匹配的设置项</div>
                            )}
                        </nav>
                    </div>

                    {/* Right Content Area */}
                    <div style={styles.contentArea}>
                        {/* Breadcrumb */}
                        <div style={styles.breadcrumb}>
                            {getBreadcrumb()}
                        </div>

                        {/* Settings Content */}
                        <div style={styles.settingsContent}>
                            {renderTabContent()}
                        </div>
                    </div>
                </div>

                {/* Highlight Rules Modal */}
                <HighlightRulesModal
                    isOpen={rulesModalOpen}
                    rules={config.highlight_rules || []}
                    onChange={(rules) => {
                        setConfig({
                            ...config,
                            highlight_rules: rules
                        });
                    }}
                    onClose={() => setRulesModalOpen(false)}
                />

                {/* Footer */}
                <div style={styles.footer}>
                    <div style={styles.statusMsg}>{msg}</div>
                    <div style={styles.footerActions}>
                        <button onClick={handleClose} style={styles.cancelBtn}>取消</button>
                        <button onClick={handleSave} style={styles.saveBtn} disabled={loading}>
                            {loading ? '正在保存...' : '保存更改'}
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
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.7)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 2000,
    },
    modal: {
        backgroundColor: '#252526',
        borderRadius: '8px',
        width: '900px',
        height: '650px',
        display: 'flex',
        flexDirection: 'column' as const,
        boxShadow: '0 4px 12px rgba(0, 0, 0, 0.5)',
        color: '#ccc',
        overflow: 'hidden',
    },
    header: {
        padding: '16px 24px',
        borderBottom: '1px solid #3c3c3c',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        backgroundColor: '#1e1e1e',
    },
    title: {
        margin: 0,
        fontSize: '1.1rem',
        color: '#fff',
        fontWeight: 600,
    },
    closeBtn: {
        background: 'none',
        border: 'none',
        color: '#ccc',
        fontSize: '1.5rem',
        cursor: 'pointer',
        padding: '0',
        width: '32px',
        height: '32px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        borderRadius: '4px',
        ':hover': {
            backgroundColor: '#3c3c3c',
        }
    },
    mainContent: {
        display: 'flex',
        flex: 1,
        overflow: 'hidden',
    },
    sidebar: {
        width: '220px',
        backgroundColor: '#252526',
        borderRight: '1px solid #3E3E42',
        display: 'flex',
        flexDirection: 'column' as const,
        padding: '12px 0',
    },
    searchBox: {
        padding: '0 12px 12px',
    },
    searchInput: {
        width: '100%',
        padding: '8px 12px',
        backgroundColor: '#3C3C3C',
        border: '1px solid #5A5A5A',
        borderRadius: '4px',
        color: '#FFFFFF',
        fontSize: '13px',
        outline: 'none',
        boxSizing: 'border-box' as 'border-box',
        ':focus': {
            borderColor: '#007ACC',
        }
    },
    nav: {
        flex: 1,
        overflowY: 'auto' as const,
    },
    navItem: {
        display: 'flex',
        alignItems: 'center',
        gap: '12px',
        padding: '10px 12px',
        cursor: 'pointer',
        fontSize: '13px',
        color: '#CCCCCC',
        borderRadius: '4px',
        margin: '0 8px',
        ':hover': {
            backgroundColor: '#37373D',
        }
    },
    navItemActive: {
        backgroundColor: '#37373D',
        color: '#FFFFFF',
        fontWeight: 500,
    },
    navIcon: {
        fontSize: '16px',
        width: '20px',
        textAlign: 'center' as const,
    },
    navText: {
        flex: 1,
    },
    noResults: {
        padding: '20px 12px',
        textAlign: 'center' as const,
        color: '#888',
        fontSize: '13px',
    },
    contentArea: {
        flex: 1,
        padding: '20px 24px',
        overflowY: 'auto' as const,
        backgroundColor: '#2D2D2D',
    },
    breadcrumb: {
        fontSize: '12px',
        color: '#888',
        marginBottom: '16px',
        paddingBottom: '8px',
        borderBottom: '1px solid #3E3E42',
    },
    settingsContent: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '8px',
    },
    settingsGroup: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '20px',
    },
    groupTitle: {
        fontSize: '13px',
        fontWeight: 600,
        color: '#E0E0E0',
        marginTop: '8px',
        marginBottom: '4px',
        paddingBottom: '6px',
        borderBottom: '1px solid #3E3E42',
    },
    settingItem: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '8px',
    },
    settingLabel: {
        fontSize: '13px',
        color: '#CCCCCC',
        fontWeight: 500,
    },
    settingDescription: {
        fontSize: '11px',
        color: '#999999',
        lineHeight: '1.4',
        marginTop: '-4px',
    },
    input: {
        padding: '8px 12px',
        borderRadius: '4px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#1e1e1e',
        color: '#fff',
        outline: 'none',
        fontSize: '13px',
        ':focus': {
            borderColor: '#007ACC',
        }
    },
    textarea: {
        padding: '8px 12px',
        borderRadius: '4px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#1e1e1e',
        color: '#fff',
        outline: 'none',
        fontFamily: 'monospace',
        fontSize: '12px',
        resize: 'vertical' as const,
        ':focus': {
            borderColor: '#007ACC',
        }
    },
    secondaryButton: {
        padding: '8px 16px',
        borderRadius: '4px',
        border: '1px solid #5A5A5A',
        backgroundColor: '#3C3C3C',
        color: '#fff',
        cursor: 'pointer',
        fontSize: '13px',
        ':hover': {
            backgroundColor: '#4C4C4C',
            borderColor: '#6A6A6A',
        }
    },
    footer: {
        padding: '16px 24px',
        borderTop: '1px solid #3c3c3c',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        backgroundColor: '#1e1e1e',
    },
    footerActions: {
        display: 'flex',
        gap: '12px',
    },
    statusMsg: {
        color: '#4caf50',
        fontSize: '13px',
    },
    saveBtn: {
        padding: '8px 20px',
        borderRadius: '4px',
        border: 'none',
        backgroundColor: '#007acc',
        color: '#fff',
        cursor: 'pointer',
        fontWeight: 500,
        fontSize: '13px',
        ':hover': {
            backgroundColor: '#005a9e',
        }
    },
    cancelBtn: {
        padding: '8px 20px',
        borderRadius: '4px',
        border: '1px solid #5A5A5A',
        backgroundColor: 'transparent',
        color: '#ccc',
        cursor: 'pointer',
        fontWeight: 500,
        fontSize: '13px',
        ':hover': {
            backgroundColor: '#3C3C3C',
        }
    },
    switch: {
        position: 'relative' as const,
        display: 'inline-block',
        width: '40px',
        height: '20px',
    },
    slider: {
        position: 'absolute' as const,
        cursor: 'pointer',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: '#ccc',
        transition: '.4s',
        borderRadius: '20px',
        ':before': {
            position: 'absolute' as const,
            content: '""',
            height: '16px',
            width: '16px',
            left: '2px',
            bottom: '2px',
            backgroundColor: 'white',
            transition: '.4s',
            borderRadius: '50%',
        }
    }
};

export default SettingsModal;
