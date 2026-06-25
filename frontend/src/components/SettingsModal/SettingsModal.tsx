import React, { useState, useEffect, useRef, useMemo } from 'react';
import { TbRobot, TbPalette, TbKeyboard, TbLayoutGrid, TbBooks, TbShieldCheck, TbLock, TbSettings, TbInfoCircle, TbSearch, TbPlugConnected } from 'react-icons/tb';
import KeysMap from './KeysMap';
import HighlightRulesModal from './HighlightRulesModal';
import CommandWhitelistPanel from './CommandWhitelist/CommandWhitelistPanel';
import FileAccessPanel from './FileAccess/FileAccessPanel';
import AboutPanel from './AboutPanel';
import { HighlightRule, TerminalConfig } from '../Terminal/highlightTypes';
import { colors, radius, font, inputStyle, btnSecondary, descStyle, labelStyle, sectionTitle } from './settingsStyles';
import Switch from './Switch';

interface AppConfig {
    llm: {
        APIKey: string;
        BaseURL: string;
        FastModel: string;
        ComplexModel: string;
        Model?: string;
    };
    log: {
        dir: string;
    };
    docs: {
        dir: string;
    };
    experimental?: {
        // 保留结构以便未来扩展
    };
    terminal?: TerminalConfig;
    highlight_rules?: HighlightRule[];
    patch_store?: {
        enabled: boolean;
        type: string;
        remote_url: string;
        branch: string;
    };
    completion_delay: number;
    command_query_shortcut: string;
}

interface SettingsModalProps {
    isOpen: boolean;
    onClose: () => void;
    isBroadcastMode?: boolean;
    onToggleBroadcast?: (enabled: boolean) => void;
    onCompletionDelayChange?: (delay: number) => void;
    onHighlightRulesChange?: (rules: HighlightRule[]) => void;
}

interface PatchSyncStatus {
    enabled: boolean;
    configured: boolean;
    running: boolean;
    pendingCount: number;
    lastSyncAt?: string;
    lastSyncSuccess: boolean;
    lastSyncMessage?: string;
    remoteURL?: string;
    branch?: string;
}

type TabId = 'llm' | 'highlight' | 'shortcuts' | 'broadcast' | 'knowledge' | 'aiagent' | 'whitelist' | 'fileaccess' | 'experimental' | 'about';

interface NavItem {
    id: TabId;
    label: string;
    icon: React.ReactNode;
    category: string;
}

const defaultPatchStore = {
    enabled: false,
    type: 'git',
    remote_url: '',
    branch: 'main'
};

const normalizePatchStore = (patchStore?: AppConfig['patch_store']) => ({
    ...defaultPatchStore,
    ...patchStore,
    type: 'git',
    remote_url: (patchStore?.remote_url || '').trim(),
    branch: ((patchStore?.branch || defaultPatchStore.branch).trim() || defaultPatchStore.branch)
});

const defaultPatchSyncStatus: PatchSyncStatus = {
    enabled: false,
    configured: false,
    running: false,
    pendingCount: 0,
    lastSyncSuccess: false,
    lastSyncMessage: ''
};

const SettingsModal: React.FC<SettingsModalProps> = ({
    isOpen,
    onClose,
    isBroadcastMode,
    onToggleBroadcast,
    onCompletionDelayChange,
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
    // AI 接入：skill 安装/更新
    const [skillDir, setSkillDir] = useState('');
    const [skillLoading, setSkillLoading] = useState(false);
    const [skillMsg, setSkillMsg] = useState('');
    const [skillState, setSkillState] = useState<'unknown' | 'not_installed' | 'up_to_date' | 'outdated'>('unknown');
    const [skillInstalledVer, setSkillInstalledVer] = useState('');
    const [skillBuiltinVer, setSkillBuiltinVer] = useState('');
    const [searchQuery, setSearchQuery] = useState('');
    const [patchSyncStatus, setPatchSyncStatus] = useState<PatchSyncStatus>(defaultPatchSyncStatus);
    const [patchSyncLoading, setPatchSyncLoading] = useState(false);
    const searchInputRef = useRef<HTMLInputElement>(null);

    // Navigation items structure
    const navItems: NavItem[] = [
        { id: 'llm', label: '模型服务', icon: TbRobot({}), category: 'AI' },
        { id: 'highlight', label: '突出显示', icon: TbPalette({}), category: '终端' },
        { id: 'shortcuts', label: '快捷键', icon: TbKeyboard({}), category: '交互' },
        { id: 'broadcast', label: '多窗口', icon: TbLayoutGrid({}), category: '交互' },
        { id: 'knowledge', label: '知识共享', icon: TbBooks({}), category: '知识' },
        { id: 'aiagent', label: 'AI 接入', icon: TbPlugConnected({}), category: 'AI 接入' },
        { id: 'whitelist', label: '命令白名单', icon: TbShieldCheck({}), category: 'AI 接入' },
        { id: 'fileaccess', label: '文件访问控制', icon: TbLock({}), category: 'AI 接入' },
        { id: 'experimental', label: '高级选项', icon: TbSettings({}), category: '系统' },
        { id: 'about', label: '关于', icon: TbInfoCircle({}), category: '系统' },
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
            loadPatchSyncStatus();
            setMsg('');
            setImportDir('');
            setImportMsg('');
            setSkillDir('');
            setSkillMsg('');
            setSkillState('unknown');
            setSkillInstalledVer('');
            setSkillBuiltinVer('');
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
                    experimental: cfg.experimental || {},
                    terminal,
                    highlight_rules,
                    patch_store: normalizePatchStore(cfg.patch_store),
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

    const loadPatchSyncStatus = async () => {
        setPatchSyncLoading(true);
        try {
            // @ts-ignore
            const raw = await window.go.main.App.GetPatchSyncStatus();
            const parsed = raw ? JSON.parse(raw) : {};
            setPatchSyncStatus({
                ...defaultPatchSyncStatus,
                ...parsed
            });
        } catch (e) {
            console.error(e);
            setPatchSyncStatus({
                ...defaultPatchSyncStatus,
                lastSyncMessage: '加载同步状态失败'
            });
        } finally {
            setPatchSyncLoading(false);
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
            const nextConfig = {
                ...config,
                patch_store: normalizePatchStore(config.patch_store)
            };
            setConfig(nextConfig);
            // @ts-ignore
            const err = await window.go.main.App.SaveSettings(nextConfig);
            if (err) {
                setMsg('错误: ' + err);
            } else {
                setMsg('设置已保存！');
                await loadPatchSyncStatus();
                if (onCompletionDelayChange && nextConfig.completion_delay !== undefined) {
                    onCompletionDelayChange(nextConfig.completion_delay);
                }
                if (onHighlightRulesChange) {
                    onHighlightRulesChange(nextConfig.highlight_rules || []);
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

    const handleHighlightRulesSave = async (rules: HighlightRule[]) => {
        if (!config) return;
        const newConfig = { ...config, highlight_rules: rules };
        setConfig(newConfig);
        try {
            // @ts-ignore
            const err = await window.go.main.App.SaveSettings(newConfig);
            if (err) {
                setMsg('错误: ' + err);
            } else {
                if (onHighlightRulesChange) {
                    onHighlightRulesChange(rules);
                }
            }
        } catch (e: any) {
            setMsg('错误: ' + e.toString());
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

    const handlePatchStoreChange = (key: keyof NonNullable<AppConfig['patch_store']>, value: string | boolean) => {
        if (!config) return;
        setConfig({
            ...config,
            patch_store: {
                ...normalizePatchStore(config.patch_store),
                [key]: value
            }
        });
    };

    const handleRetryPatchSync = async () => {
        setPatchSyncLoading(true);
        try {
            // @ts-ignore
            const err = await window.go.main.App.RetryPatchSync();
            if (err) {
                setMsg('同步失败: ' + err);
            } else {
                setMsg('已触发补丁同步');
            }
        } catch (e: any) {
            setMsg('同步失败: ' + e.toString());
        } finally {
            await loadPatchSyncStatus();
        }
    };

    const renderPatchSyncState = () => {
        if (patchSyncLoading) return '正在读取同步状态...';
        if (patchSyncStatus.running) return '同步中';
        if (!patchSyncStatus.enabled) return '已关闭';
        if (!patchSyncStatus.configured) return '待配置仓库';
        if (patchSyncStatus.lastSyncMessage) return patchSyncStatus.lastSyncMessage;
        return '待同步';
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

    // 检测指定目录下是否已安装 skill，以及版本是否最新
    const handleCheckSkill = async () => {
        const dir = (skillDir || '').trim();
        if (!dir) {
            setSkillMsg('请输入 skill 安装目录');
            setSkillState('unknown');
            return;
        }
        setSkillLoading(true);
        setSkillMsg('正在检测...');
        try {
            // @ts-ignore
            const raw = await window.go.main.App.CheckSkillStatus(dir);
            const r = raw ? JSON.parse(raw) : {};
            if (r.success === false) {
                setSkillMsg(r.error || '检测失败');
                setSkillState('unknown');
            } else {
                setSkillInstalledVer(r.installed || '');
                setSkillBuiltinVer(r.builtin || '');
                setSkillState(r.state || 'unknown');
                if (r.state === 'not_installed') {
                    setSkillMsg('该目录下尚未安装 OpsCopilot skill');
                } else if (r.state === 'up_to_date') {
                    setSkillMsg(`已是最新版本（v${r.installed}）`);
                } else if (r.state === 'outdated') {
                    setSkillMsg(`已安装 v${r.installed}，可更新至 v${r.builtin}`);
                } else {
                    setSkillMsg('检测完成');
                }
            }
        } catch (e: any) {
            setSkillMsg('检测失败: ' + e.toString());
            setSkillState('unknown');
        } finally {
            setSkillLoading(false);
        }
    };

    // 安装（或更新）skill 到指定目录下的 opscopilot-ops/ 子目录
    const handleInstallSkill = async () => {
        const dir = (skillDir || '').trim();
        if (!dir) {
            setSkillMsg('请输入 skill 安装目录');
            return;
        }
        setSkillLoading(true);
        setSkillMsg('正在安装...');
        try {
            // @ts-ignore
            const raw = await window.go.main.App.InstallSkill(dir);
            const r = raw ? JSON.parse(raw) : {};
            if (r.success === false) {
                setSkillMsg(r.error || '安装失败');
            } else {
                setSkillMsg(`已安装到 ${r.path}（v${r.version}）`);
                // 安装后刷新状态
                await handleCheckSkill();
            }
        } catch (e: any) {
            setSkillMsg('安装失败: ' + e.toString());
        } finally {
            setSkillLoading(false);
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
                        <div style={styles.groupTitle}>快捷键配置</div>
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
                        <div style={styles.groupTitle}>快捷键说明</div>
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
                                <Switch
                                    checked={!!isBroadcastMode}
                                    onChange={(v) => {
                                        if (onToggleBroadcast) onToggleBroadcast(v);
                                    }}
                                />
                                <span style={{ color: colors.textSecondary, fontSize: font.base }}>
                                    {isBroadcastMode ? '已开启' : '已关闭'}
                                </span>
                            </div>
                            <div style={styles.settingDescription}>
                                开启后，默认将当前所有打开的终端加入广播组。您可以在标签页上单独切换每个终端的广播状态。
                            </div>
                        </div>
                    </div>
                );

            case 'knowledge':
                return (
                    <div style={styles.settingsGroup}>
                        <div style={styles.groupTitle}>知识共享</div>
                        <div style={styles.settingItem}>
                            <label style={styles.settingLabel}>启用补丁同步</label>
                            <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                                <Switch
                                    checked={!!config.patch_store?.enabled}
                                    onChange={(v) => handlePatchStoreChange('enabled', v)}
                                />
                                <span style={{ color: colors.textSecondary, fontSize: font.base }}>
                                    {config.patch_store?.enabled ? '已开启' : '已关闭'}
                                </span>
                            </div>
                            <div style={styles.settingDescription}>
                                归档后会将最新场景以补丁形式推送到共享 Git 仓库，保存后立即重载当前同步配置
                            </div>
                        </div>
                        <div style={styles.settingItem}>
                            <label style={styles.settingLabel}>Git 仓库地址</label>
                            <input
                                style={styles.input}
                                value={config.patch_store?.remote_url || ''}
                                onChange={(e) => handlePatchStoreChange('remote_url', e.target.value)}
                                placeholder="例如：git@github.com:team/opscopilot-patches.git"
                            />
                            <div style={styles.settingDescription}>
                                支持本地路径、SSH 或 HTTPS 地址；认证依赖本机 Git 凭据、SSH Agent 或系统凭据管理
                            </div>
                        </div>
                        <div style={styles.settingItem}>
                            <label style={styles.settingLabel}>同步分支</label>
                            <input
                                style={styles.input}
                                value={config.patch_store?.branch || defaultPatchStore.branch}
                                onChange={(e) => handlePatchStoreChange('branch', e.target.value)}
                                placeholder="main"
                            />
                            <div style={styles.settingDescription}>
                                作者名自动读取本机 `git config user.name`，无需再手动填写
                            </div>
                        </div>
                        <div style={styles.groupTitle}>同步状态</div>
                        <div style={styles.statusGrid}>
                            <div style={styles.statusCard}>
                                <div style={styles.statusCardLabel}>当前状态</div>
                                <div style={styles.statusCardValue}>{renderPatchSyncState()}</div>
                            </div>
                            <div style={styles.statusCard}>
                                <div style={styles.statusCardLabel}>待上传补丁</div>
                                <div style={styles.statusCardValue}>{patchSyncStatus.pendingCount}</div>
                            </div>
                            <div style={styles.statusCard}>
                                <div style={styles.statusCardLabel}>最近同步时间</div>
                                <div style={styles.statusCardValue}>{patchSyncStatus.lastSyncAt || '暂无'}</div>
                            </div>
                        </div>
                        <div style={styles.settingItem}>
                            <label style={styles.settingLabel}>手动重试</label>
                            <div style={{ display: 'flex', gap: '10px', alignItems: 'center', flexWrap: 'wrap' as const }}>
                                <button
                                    onClick={handleRetryPatchSync}
                                    style={styles.secondaryButton}
                                    disabled={patchSyncLoading || patchSyncStatus.running || !patchSyncStatus.enabled || !patchSyncStatus.configured}
                                >
                                    {patchSyncStatus.running ? '正在同步...' : '立即重试同步'}
                                </button>
                                <button
                                    onClick={loadPatchSyncStatus}
                                    style={styles.secondaryButton}
                                    disabled={patchSyncLoading}
                                >
                                    刷新状态
                                </button>
                            </div>
                            <div style={styles.settingDescription}>
                                {patchSyncStatus.lastSyncMessage || '保存配置后会自动刷新运行中的补丁同步实例'}
                            </div>
                        </div>
                    </div>
                );

            case 'aiagent':
                return (
                    <div style={styles.settingsGroup}>
                        <div style={styles.groupTitle}>Skill 安装</div>
                        <div style={styles.settingItem}>
                            <label style={styles.settingLabel}>安装 Skill 到 AI Agent</label>
                            <div style={{ display: 'flex', gap: '10px', alignItems: 'center', flexWrap: 'wrap' as const }}>
                                <input
                                    style={{ ...styles.input, flex: 1, minWidth: '320px' }}
                                    value={skillDir}
                                    onChange={(e) => {
                                        setSkillDir(e.target.value);
                                        // 改动目录后重置状态，避免显示陈旧的版本对比
                                        setSkillState('unknown');
                                        setSkillMsg('');
                                    }}
                                    placeholder="例如：C:\\Users\\xxx\\.claude\\skills"
                                />
                                <button
                                    onClick={handleCheckSkill}
                                    style={styles.secondaryButton}
                                    disabled={skillLoading}
                                >
                                    {skillLoading ? '检测中...' : '检测状态'}
                                </button>
                                <button
                                    onClick={handleInstallSkill}
                                    style={styles.secondaryButton}
                                    disabled={skillLoading}
                                >
                                    {skillState === 'not_installed' ? '安装'
                                        : skillState === 'outdated' ? '更新'
                                        : skillState === 'up_to_date' ? '重新安装'
                                        : '安装/更新'}
                                </button>
                            </div>
                            {skillMsg ? (
                                <div style={{
                                    ...styles.settingDescription,
                                    color: skillState === 'up_to_date' ? colors.success : colors.textSecondary
                                }}>
                                    {skillMsg}
                                </div>
                            ) : (
                                <div style={styles.settingDescription}>
                                    将 OpsCopilot 的 CLI 能力以 Claude Code skill 格式安装到指定目录下的 opscopilot-ops/ 子目录。
                                    安装后，AI Agent（如 Claude Code）即可通过该 skill 调用 OpsCopilot 执行运维操作和故障诊断。
                                    命令路径会自动替换为本机 opscopilot.exe 的绝对路径。
                                </div>
                            )}
                        </div>
                        <div style={styles.groupTitle}>安全闸门</div>
                        <div style={styles.settingItem}>
                            <div style={styles.settingDescription}>
                                AI Agent 通过 skill 调用 OpsCopilot 时，所有非交互式访问（CLI 等）都会强制经过以下两道安全闸门。
                                请在下方对应页签中为 AI Agent 配置允许的操作范围——这两项是「AI 接入」能力的配套约束，缺一不可。
                            </div>
                            <div style={{ display: 'flex', gap: '10px', alignItems: 'center', flexWrap: 'wrap' as const, marginTop: '8px' }}>
                                <button
                                    onClick={() => setActiveTab('whitelist')}
                                    style={styles.secondaryButton}
                                >
                                    {TbShieldCheck({})} 命令白名单 →
                                </button>
                                <button
                                    onClick={() => setActiveTab('fileaccess')}
                                    style={styles.secondaryButton}
                                >
                                    {TbLock({})} 文件访问控制 →
                                </button>
                            </div>
                            <div style={styles.settingDescription}>
                                <strong>命令白名单</strong>：按服务器 IP 粒度限制 AI 可执行的命令；
                                <strong>文件访问控制</strong>：限制 AI 可读写的远程路径和文件大小。
                            </div>
                        </div>
                    </div>
                );

            case 'whitelist':
                return <CommandWhitelistPanel />;

            case 'fileaccess':
                return <FileAccessPanel />;

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
                                    支持导入 config.json / quick_commands.json / highlight_rules.json；导入前会自动备份当前配置到 .bak 文件
                                </div>
                            )}
                        </div>
                    </div>
                );

            case 'about':
                return <AboutPanel />;

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
                            <span style={styles.searchIcon}>{TbSearch({})}</span>
                            <input
                                ref={searchInputRef}
                                style={styles.searchInput}
                                placeholder="搜索设置..."
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
                    onSave={handleHighlightRulesSave}
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
        backgroundColor: colors.overlay,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 2000,
    },
    modal: {
        backgroundColor: colors.bgSecondary,
        borderRadius: radius.lg,
        width: '900px',
        height: '650px',
        display: 'flex',
        flexDirection: 'column' as const,
        boxShadow: '0 4px 12px rgba(0, 0, 0, 0.5)',
        color: colors.textSecondary,
        overflow: 'hidden',
    },
    header: {
        padding: '16px 24px',
        borderBottom: `1px solid ${colors.borderPrimary}`,
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        backgroundColor: colors.bgPrimary,
    },
    title: {
        margin: 0,
        fontSize: '1.1rem',
        color: colors.textPrimary,
        fontWeight: 600,
    },
    closeBtn: {
        background: 'none',
        border: 'none',
        color: colors.textSecondary,
        fontSize: '1.5rem',
        cursor: 'pointer',
        padding: '0',
        width: '32px',
        height: '32px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        borderRadius: radius.sm,
        ':hover': {
            backgroundColor: colors.bgHover,
        }
    },
    mainContent: {
        display: 'flex',
        flex: 1,
        overflow: 'hidden',
    },
    sidebar: {
        width: '220px',
        backgroundColor: colors.bgSecondary,
        borderRight: `1px solid ${colors.borderPrimary}`,
        display: 'flex',
        flexDirection: 'column' as const,
        padding: '12px 0',
    },
    searchBox: {
        padding: '0 12px 12px',
        position: 'relative' as const,
    },
    searchIcon: {
        position: 'absolute' as const,
        left: '24px',
        top: '50%',
        transform: 'translateY(-50%)',
        color: colors.textTertiary,
        fontSize: font.lg,
        pointerEvents: 'none' as const,
    },
    searchInput: {
        width: '100%',
        padding: '8px 12px 8px 32px',
        backgroundColor: colors.bgHover,
        border: `1px solid ${colors.borderPrimary}`,
        borderRadius: radius.sm,
        color: colors.textPrimary,
        fontSize: font.base,
        outline: 'none',
        boxSizing: 'border-box' as 'border-box',
        ':focus': {
            borderColor: colors.accent,
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
        fontSize: font.base,
        color: colors.textSecondary,
        borderRadius: radius.sm,
        margin: '0 8px',
        ':hover': {
            backgroundColor: colors.bgTertiary,
        }
    },
    navItemActive: {
        backgroundColor: colors.bgTertiary,
        color: colors.textPrimary,
        fontWeight: 500,
    },
    navIcon: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: '20px',
        height: '20px',
        fontSize: '16px',
        flexShrink: 0,
    },
    navText: {
        flex: 1,
    },
    noResults: {
        padding: '20px 12px',
        textAlign: 'center' as const,
        color: colors.textTertiary,
        fontSize: font.base,
    },
    contentArea: {
        flex: 1,
        padding: '20px 24px',
        overflowY: 'auto' as const,
        backgroundColor: colors.bgTertiary,
    },
    breadcrumb: {
        fontSize: font.sm,
        color: colors.textTertiary,
        marginBottom: '16px',
        paddingBottom: '8px',
        borderBottom: `1px solid ${colors.borderPrimary}`,
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
        fontSize: font.base,
        fontWeight: 600,
        color: colors.textSecondary,
        marginTop: '8px',
        marginBottom: '4px',
        paddingBottom: '6px',
        borderBottom: `1px solid ${colors.borderPrimary}`,
    },
    settingItem: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '8px',
    },
    settingLabel: {
        ...labelStyle,
    },
    settingDescription: {
        ...descStyle,
        marginTop: '-4px',
    },
    input: {
        ...inputStyle,
    },
    secondaryButton: {
        ...btnSecondary,
    },
    statusGrid: {
        display: 'grid',
        gridTemplateColumns: 'repeat(3, minmax(0, 1fr))',
        gap: '12px',
    },
    statusCard: {
        padding: '14px',
        borderRadius: radius.md,
        border: `1px solid ${colors.borderPrimary}`,
        backgroundColor: colors.bgSecondary,
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '8px',
    },
    statusCardLabel: {
        fontSize: font.sm,
        color: colors.textTertiary,
    },
    statusCardValue: {
        fontSize: font.lg,
        color: colors.textPrimary,
        fontWeight: 600,
        wordBreak: 'break-word' as const,
    },
    footer: {
        padding: '16px 24px',
        borderTop: `1px solid ${colors.borderPrimary}`,
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        backgroundColor: colors.bgPrimary,
    },
    footerActions: {
        display: 'flex',
        gap: '12px',
    },
    statusMsg: {
        color: colors.success,
        fontSize: font.base,
    },
    saveBtn: {
        padding: '8px 20px',
        borderRadius: radius.sm,
        border: 'none',
        backgroundColor: colors.accent,
        color: colors.textPrimary,
        cursor: 'pointer',
        fontWeight: 500,
        fontSize: font.base,
        ':hover': {
            backgroundColor: '#005a9e',
        }
    },
    cancelBtn: {
        padding: '8px 20px',
        borderRadius: radius.sm,
        border: `1px solid ${colors.borderPrimary}`,
        backgroundColor: 'transparent',
        color: colors.textSecondary,
        cursor: 'pointer',
        fontWeight: 500,
        fontSize: font.base,
        ':hover': {
            backgroundColor: colors.bgHover,
        }
    },
};

export default SettingsModal;
