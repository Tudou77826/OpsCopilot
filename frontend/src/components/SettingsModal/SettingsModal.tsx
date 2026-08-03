import React, { useState, useEffect, useRef, useMemo } from 'react';
import { TbRobot, TbPalette, TbKeyboard, TbLayoutGrid, TbBooks, TbShieldCheck, TbLock, TbSettings, TbInfoCircle, TbSearch, TbPlugConnected, TbMinus, TbPlus, TbRefresh, TbCheck, TbSun, TbMoon } from 'react-icons/tb';
import KeysMap from './KeysMap';
import HighlightRulesModal from './HighlightRulesModal';
import CommandWhitelistPanel from './CommandWhitelist/CommandWhitelistPanel';
import FileAccessPanel from './FileAccess/FileAccessPanel';
import AboutPanel from './AboutPanel';
import { HighlightRule, TerminalConfig } from '../Terminal/highlightTypes';
import { Theme } from '../appearanceTypes';
import { assessPattern } from '../Terminal/highlight/regexSafety';
import {
    DEFAULT_TERMINAL_FONT_SIZE,
    MAX_TERMINAL_FONT_SIZE,
    MIN_TERMINAL_FONT_SIZE,
    TERMINAL_FONT_OPTIONS,
    clampTerminalFontSize,
    normalizeTerminalConfig,
} from '../Terminal/terminalAppearance';
import { colors, radius, font, inputStyle, btnSecondary, descStyle, labelStyle, pageContainer, settingsCard, cardTitle, settingRow, settingRowTop, settingRowLeft, settingRowRight, settingRowLabel, settingRowDesc, navGroupTitle, navItem, navItemActive, cardDivider, inputWide } from './settingsStyles';
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
    cli?: {
        exec_timeout_sec: number;
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
    onTerminalConfigChange?: (config: TerminalConfig) => void;
    theme?: Theme;
    onThemeChange?: (theme: Theme) => void;
    updateAvailable?: boolean;
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

type TabId = 'llm' | 'appearance' | 'terminal' | 'highlight' | 'shortcuts' | 'broadcast' | 'knowledge' | 'aiagent' | 'whitelist' | 'fileaccess' | 'experimental' | 'about';

interface NavItem {
    id: TabId;
    label: string;
    icon: React.ReactNode;
    category: string;
    // 额外搜索关键词（如合并进其它页面的功能名），搜索时一并匹配
    keywords?: string[];
}

const defaultPatchStore = {
    enabled: false,
    type: 'git',
    remote_url: '',
    branch: 'main'
};

const defaultCliConfig = {
    exec_timeout_sec: 120
};

const normalizeCliConfig = (cli?: AppConfig['cli']) => ({
    exec_timeout_sec: cli?.exec_timeout_sec && cli.exec_timeout_sec > 0 ? cli.exec_timeout_sec : defaultCliConfig.exec_timeout_sec
});

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
    onHighlightRulesChange,
    onTerminalConfigChange,
    theme = 'dark',
    onThemeChange,
    updateAvailable
}) => {
    const [config, setConfig] = useState<AppConfig | null>(null);
    const [loading, setLoading] = useState(false);
    const [msg, setMsg] = useState('');
    const [activeTab, setActiveTab] = useState<TabId>('llm');
    const [importDir, setImportDir] = useState('');
    const [importLoading, setImportLoading] = useState(false);
    const [importMsg, setImportMsg] = useState('');
    // AI 接入：skill 安装/更新
    const [skillDir, setSkillDir] = useState('');
    const [skillLoading, setSkillLoading] = useState(false);
    const [skillMsg, setSkillMsg] = useState('');
    const [skillState, setSkillState] = useState<'unknown' | 'not_installed' | 'up_to_date' | 'outdated'>('unknown');
    // skill 是否需要关注（未配置 / 未安装 / 过期）→ 「AI 接入」导航项亮红点
    const [skillNeedsAttention, setSkillNeedsAttention] = useState(false);
    // 高亮规则是否有存量问题（语法错 / 灾难正则，常因用户直改 JSON 引入）→ 「突出显示」导航项亮红点
    const [highlightIssues, setHighlightIssues] = useState<{ name: string; issues: string[] }[]>([]);
    const [skillInstalledVer, setSkillInstalledVer] = useState('');
    const [skillBuiltinVer, setSkillBuiltinVer] = useState('');
    const [searchQuery, setSearchQuery] = useState('');
    const [patchSyncStatus, setPatchSyncStatus] = useState<PatchSyncStatus>(defaultPatchSyncStatus);
    const [patchSyncLoading, setPatchSyncLoading] = useState(false);
    const searchInputRef = useRef<HTMLInputElement>(null);

    // Navigation items structure
    const navItems: NavItem[] = [
        { id: 'llm', label: '模型服务', icon: TbRobot({}), category: 'AI' },
        { id: 'appearance', label: '外观', icon: TbSun({}), category: '终端', keywords: ['终端外观', '终端', '字体', '字号', 'theme', '主题'] },
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
            item.id.toLowerCase().includes(query) ||
            (item.keywords || []).some(k => k.toLowerCase().includes(query))
        );
    }, [searchQuery, navItems]);

    // 按 category 分组导航项（保持原有顺序，搜索时自动折叠为命中的组）
    const groupedNavItems = useMemo(() => {
        const groups: { category: string; items: NavItem[] }[] = [];
        for (const item of filteredNavItems) {
            const last = groups[groups.length - 1];
            if (last && last.category === item.category) {
                last.items.push(item);
            } else {
                groups.push({ category: item.category, items: [item] });
            }
        }
        return groups;
    }, [filteredNavItems]);

    useEffect(() => {
        if (isOpen) {
            loadSettings();
            loadPatchSyncStatus();
            setMsg('');
            setImportDir('');
            setImportMsg('');
            setSkillMsg('');
            setSkillState('unknown');
            setSkillInstalledVer('');
            setSkillBuiltinVer('');
            setSearchQuery('');
            setActiveTab('llm');

            // 回填上次使用的 skill 目录，并据此判断「AI 接入」导航项是否需要亮红点。
            // 同时写入 skillState/版本号，供 tab 内状态横幅展示准确文案。
            const savedSkillDir = localStorage.getItem('opscopilot:skillDir') || '';
            setSkillDir(savedSkillDir);
            if (!savedSkillDir) {
                // 从未配置过 skill 目录 → 引导用户去配置/安装，亮红点
                setSkillNeedsAttention(true);
            } else {
                // 已配置过：静默检测一次，按 state 决定是否亮点
                // @ts-ignore
                window.go?.main?.App?.CheckSkillStatus?.(savedSkillDir)
                    .then((raw: string) => {
                        const r = raw ? JSON.parse(raw) : {};
                        if (r.success === false) {
                            // 检测出错（目录无效等）不误导，不亮
                            setSkillNeedsAttention(false);
                            return;
                        }
                        setSkillState(r.state || 'unknown');
                        setSkillInstalledVer(r.installed || '');
                        setSkillBuiltinVer(r.builtin || '');
                        setSkillNeedsAttention(r.state !== 'up_to_date');
                    })
                    .catch(() => setSkillNeedsAttention(false));
            }
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
                const terminal = normalizeTerminalConfig(cfg.terminal);
                const highlight_rules: HighlightRule[] = Array.isArray(cfg.highlight_rules) ? cfg.highlight_rules : [];
                // 存量校验：JSON 直改可能引入语法错 / 灾难正则，逐条 assessPattern 检查。
                // 只关注「完全不生效」的规则（!canEnable = severe 或语法错），它们需要用户介入；
                // moderate/high（如「正则较长」）只是提示，不亮红点。
                const ruleIssues = highlight_rules
                    .map(r => {
                        const risk = assessPattern(r.pattern || '');
                        return { name: r.name || r.pattern || '未命名', failed: !risk.canEnable, issues: risk.issues };
                    })
                    .filter(it => it.failed);
                setHighlightIssues(ruleIssues);
                setConfig({
                    ...cfg,
                    llm: {
                        ...llmCfg,
                        FastModel: fastModel,
                        ComplexModel: complexModel,
                    },
                    experimental: cfg.experimental || {},
                    cli: normalizeCliConfig(cfg.cli),
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
                cli: normalizeCliConfig(config.cli),
                terminal: normalizeTerminalConfig(config.terminal),
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
                if (onTerminalConfigChange) {
                    onTerminalConfigChange(nextConfig.terminal);
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
                setSkillNeedsAttention(false);
            } else {
                // 记住这次使用的目录，下次打开面板时自动回填并检测
                localStorage.setItem('opscopilot:skillDir', dir);
                setSkillInstalledVer(r.installed || '');
                setSkillBuiltinVer(r.builtin || '');
                setSkillState(r.state || 'unknown');
                // 同步导航 badge：非 up_to_date（未安装/过期）则亮红点
                setSkillNeedsAttention(r.state !== 'up_to_date');
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

    // AI 接入 tab 顶部的状态横幅：当 skill 需要关注时（未配置/未安装/过期），
    // 明确告诉用户「红点是因为什么 + 下一步该干嘛」，避免点开 tab 后不知所措。
    const renderSkillAttentionBanner = () => {
        let message = '';
        let tone: 'warning' | 'accent' = 'accent';
        if (!skillDir) {
            message = '尚未配置 skill 目录，AI Agent 无法调用 OpsCopilot。请填写目录后点击「检测状态」。';
            tone = 'warning';
        } else if (skillState === 'not_installed') {
            message = '目录下尚未安装 OpsCopilot skill，请点击「安装」。';
            tone = 'warning';
        } else if (skillState === 'outdated') {
            const ver = skillInstalledVer && skillBuiltinVer
                ? `（v${skillInstalledVer} → v${skillBuiltinVer}）`
                : '';
            message = `skill 有新版本可更新${ver}，建议点击「更新」。`;
            tone = 'accent';
        }
        if (!message) return null;

        const color = tone === 'warning' ? colors.warning : colors.accent;
        return (
            <div style={{
                ...styles.attentionBanner,
                borderLeftColor: color,
            }}>
                <span style={{ color }}>{TbInfoCircle({ size: 16 })}</span>
                <span style={styles.attentionText}>{message}</span>
            </div>
        );
    };

    // 终端外观卡片（收纳在「外观」页中，与主题设置同页展示）
    const renderTerminalAppearance = () => {
        const terminal = normalizeTerminalConfig(config.terminal);
        const fontSize = clampTerminalFontSize(terminal.font_size);
        const updateTerminal = (patch: Partial<TerminalConfig>) => {
            setConfig({
                ...config,
                terminal: normalizeTerminalConfig({ ...terminal, ...patch }),
            });
        };
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
    };

    // Render tab content
    const renderTabContent = () => {
        switch (activeTab) {
            case 'llm':
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
                                        value={config.llm.BaseURL}
                                        onChange={(e) => handleChange('llm', 'BaseURL', e.target.value)}
                                        placeholder="https://api.openai.com/v1"
                                    />
                                </div>
                            </div>
                            <div style={styles.row}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>API 密钥 (API Key)</div>
                                    <div style={styles.rowDesc}>用于调用模型服务的身份验证密钥</div>
                                </div>
                                <div style={styles.rowRight}>
                                    <input
                                        style={styles.inputWide}
                                        type="password"
                                        value={config.llm.APIKey}
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

            case 'appearance':
                return (
                    <div style={styles.settingsGroup}>
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
                        {/* 终端外观已收纳在本页 */}
                        {renderTerminalAppearance()}
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

            case 'broadcast':
                return (
                    <div style={styles.settingsGroup}>
                        <div style={styles.card}>
                            <div style={styles.cardTitle}>多窗口广播模式</div>
                            <div style={styles.row}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>启用广播模式</div>
                                    <div style={styles.rowDesc}>
                                        开启后，默认将当前所有打开的终端加入广播组。您可以在标签页上单独切换每个终端的广播状态。
                                    </div>
                                </div>
                                <div style={styles.rowRight}>
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
                            </div>
                        </div>
                    </div>
                );

            case 'knowledge':
                return (
                    <div style={styles.settingsGroup}>
                        <div style={styles.card}>
                            <div style={styles.cardTitle}>知识共享</div>
                            <div style={styles.row}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>启用补丁同步</div>
                                    <div style={styles.rowDesc}>
                                        归档后会将最新场景以补丁形式推送到共享 Git 仓库，保存后立即重载当前同步配置
                                    </div>
                                </div>
                                <div style={styles.rowRight}>
                                    <Switch
                                        checked={!!config.patch_store?.enabled}
                                        onChange={(v) => handlePatchStoreChange('enabled', v)}
                                    />
                                    <span style={{ color: colors.textSecondary, fontSize: font.base }}>
                                        {config.patch_store?.enabled ? '已开启' : '已关闭'}
                                    </span>
                                </div>
                            </div>
                            <div style={styles.cardDivider} />
                            <div style={styles.row}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>Git 仓库地址</div>
                                    <div style={styles.rowDesc}>
                                        支持本地路径、SSH 或 HTTPS 地址；认证依赖本机 Git 凭据、SSH Agent 或系统凭据管理
                                    </div>
                                </div>
                                <div style={styles.rowRight}>
                                    <input
                                        style={styles.inputWide}
                                        value={config.patch_store?.remote_url || ''}
                                        onChange={(e) => handlePatchStoreChange('remote_url', e.target.value)}
                                        placeholder="例如：git@github.com:team/opscopilot-patches.git"
                                    />
                                </div>
                            </div>
                            <div style={styles.row}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>同步分支</div>
                                    <div style={styles.rowDesc}>
                                        作者名自动读取本机 `git config user.name`，无需再手动填写
                                    </div>
                                </div>
                                <div style={styles.rowRight}>
                                    <input
                                        style={styles.inputWide}
                                        value={config.patch_store?.branch || defaultPatchStore.branch}
                                        onChange={(e) => handlePatchStoreChange('branch', e.target.value)}
                                        placeholder="main"
                                    />
                                </div>
                            </div>
                        </div>
                        <div style={styles.card}>
                            <div style={styles.cardTitle}>同步状态</div>
                            <div style={{ ...styles.statusGrid, maxWidth: '760px' }}>
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
                            <div style={styles.cardDivider} />
                            <div style={styles.row}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>手动重试</div>
                                    <div style={styles.rowDesc}>
                                        {patchSyncStatus.lastSyncMessage || '保存配置后会自动刷新运行中的补丁同步实例'}
                                    </div>
                                </div>
                                <div style={styles.rowRight}>
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
                            </div>
                        </div>
                    </div>
                );

            case 'aiagent':
                return (
                    <div style={styles.settingsGroup}>
                        {renderSkillAttentionBanner()}
                        <div style={styles.card}>
                            <div style={styles.cardTitle}>Skill 安装</div>
                            <div style={styles.rowTop}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>安装 Skill 到 AI Agent</div>
                                    <div style={styles.rowDesc}>
                                        以 Claude Code skill 格式安装到指定目录（opscopilot-ops/ 子目录），
                                        AI Agent 即可调用 OpsCopilot 执行运维操作和故障诊断。
                                    </div>
                                </div>
                                <div style={styles.rowRight}>
                                    <input
                                        style={styles.inputWide}
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
                            </div>
                            {skillMsg ? (
                                <div style={{
                                    ...styles.rowDesc,
                                    color: skillState === 'up_to_date' ? colors.success : colors.textSecondary,
                                    marginLeft: '0',
                                }}>
                                    {skillMsg}
                                </div>
                            ) : (
                                <div style={{ ...styles.rowDesc, marginLeft: '0' }}>
                                    命令路径会自动替换为本机 opscopilot.exe 的绝对路径。
                                </div>
                            )}
                        </div>
                        <div style={styles.card}>
                            <div style={styles.cardTitle}>安全闸门</div>
                            <div style={styles.rowTop}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowDesc}>
                                        AI Agent 通过 skill 调用时，非交互式访问均经过以下两道闸门：
                                    </div>
                                    <div style={{ ...styles.rowDesc, marginTop: '6px' }}>
                                        <strong>命令白名单</strong>：按服务器 IP 粒度限制 AI 可执行的命令；
                                        <strong>文件访问控制</strong>：限制 AI 可读写的远程路径和文件大小。
                                    </div>
                                </div>
                                <div style={styles.rowRight}>
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
                        <div style={styles.card}>
                            <div style={styles.cardTitle}>目录设置</div>
                            <div style={styles.row}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>日志目录</div>
                                    <div style={styles.rowDesc}>日志文件存储目录，留空使用默认路径</div>
                                </div>
                                <div style={styles.rowRight}>
                                    <input
                                        style={styles.inputWide}
                                        value={config.log?.dir || ''}
                                        onChange={(e) => handleChange('log', 'dir', e.target.value)}
                                        placeholder="例如：C:\\Users\\xxx\\Logs"
                                    />
                                </div>
                            </div>
                            <div style={styles.row}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>知识库目录</div>
                                    <div style={styles.rowDesc}>本地文档知识库目录，用于 AI 问答增强</div>
                                </div>
                                <div style={styles.rowRight}>
                                    <input
                                        style={styles.inputWide}
                                        value={config.docs?.dir || ''}
                                        onChange={(e) => handleChange('docs', 'dir', e.target.value)}
                                        placeholder="例如：C:\\Users\\xxx\\Documents\\knowledge"
                                    />
                                </div>
                            </div>
                        </div>
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
                                        value={config.completion_delay || 150}
                                        onChange={(e) => {
                                            const value = parseInt(e.target.value) || 150;
                                            setConfig({
                                                ...config,
                                                completion_delay: Math.max(0, Math.min(2000, value))
                                            });
                                        }}
                                    />
                                </div>
                            </div>
                        </div>
                        <div style={styles.card}>
                            <div style={styles.cardTitle}>配置管理</div>
                            <div style={styles.rowTop}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>导入旧版本配置</div>
                                    <div style={styles.rowDesc}>
                                        {importMsg || '支持导入 config.json / quick_commands.json / highlight_rules.json；导入前会自动备份当前配置到 .bak 文件'}
                                    </div>
                                </div>
                                <div style={styles.rowRight}>
                                    <input
                                        style={styles.inputWide}
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
                            </div>
                        </div>
                    </div>
                );

            case 'about':
                return <AboutPanel />;

            default:
                return null;
        }
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
                            <div style={styles.searchInner}>
                                <span style={styles.searchIcon}>{TbSearch({})}</span>
                                <input
                                    ref={searchInputRef}
                                    style={styles.searchInput}
                                    placeholder="搜索设置..."
                                    value={searchQuery}
                                    onChange={(e) => setSearchQuery(e.target.value)}
                                />
                            </div>
                        </div>
                        <nav style={styles.nav}>
                            {groupedNavItems.length > 0 ? (
                                groupedNavItems.map((group) => (
                                    <div key={group.category} style={styles.navGroup}>
                                        <div style={navGroupTitle}>{group.category}</div>
                                        {group.items.map((item) => (
                                            <div
                                                key={item.id}
                                                style={activeTab === item.id ? navItemActive : navItem}
                                                onClick={() => setActiveTab(item.id)}
                                            >
                                                <span style={styles.navIcon}>{item.icon}</span>
                                                <span style={styles.navText}>{item.label}</span>
                                                {((item.id === 'about' && updateAvailable) ||
                                                    (item.id === 'aiagent' && skillNeedsAttention) ||
                                                    (item.id === 'highlight' && highlightIssues.length > 0)) && (
                                                    <span style={styles.navBadge} />
                                                )}
                                            </div>
                                        ))}
                                    </div>
                                ))
                            ) : (
                                <div style={styles.noResults}>没有找到匹配的设置项</div>
                            )}
                        </nav>
                    </div>

                    {/* Right Content Area */}
                    <div style={styles.contentArea}>
                        <div style={styles.pageContent}>
                            {/* Settings Content（页头大标题已移除：侧边栏已高亮当前页，避免标题冗余） */}
                            <div style={styles.settingsContent}>
                                {renderTabContent()}
                            </div>
                        </div>
                    </div>
                </div>

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
        width: '100vw',
        height: '100vh',
        display: 'flex',
        flexDirection: 'column' as const,
        color: colors.textSecondary,
        overflow: 'hidden',
    },
    header: {
        padding: '12px 24px',
        borderBottom: `1px solid ${colors.borderPrimary}`,
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        backgroundColor: colors.bgPrimary,
        flexShrink: 0,
    },
    title: {
        margin: 0,
        fontSize: '1.05rem',
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
        minHeight: 0,
    },
    sidebar: {
        width: '280px',
        backgroundColor: colors.bgSecondary,
        borderRight: `1px solid ${colors.borderPrimary}`,
        display: 'flex',
        flexDirection: 'column' as const,
        padding: '16px 0',
        flexShrink: 0,
    },
    searchBox: {
        padding: '0 16px 16px',
    },
    searchInner: {
        position: 'relative' as const,
    },
    searchIcon: {
        position: 'absolute' as const,
        left: '12px',
        top: '50%',
        transform: 'translateY(-50%)',
        color: colors.textTertiary,
        fontSize: font.lg,
        pointerEvents: 'none' as const,
        display: 'flex',
    },
    searchInput: {
        width: '100%',
        padding: '8px 12px 8px 34px',
        backgroundColor: colors.bgHover,
        border: `1px solid ${colors.borderPrimary}`,
        borderRadius: radius.md,
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
        padding: '0 8px',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '18px',
    },
    navGroup: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '2px',
    },
    navIcon: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: '18px',
        height: '18px',
        fontSize: '15px',
        flexShrink: 0,
    },
    navText: {
        flex: 1,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap' as const,
    },
    // 导航项右侧的更新提示红点，样式与设置按钮齿轮上的点一致
    navBadge: {
        position: 'absolute' as const,
        right: '12px',
        top: '50%',
        transform: 'translateY(-50%)',
        width: '8px',
        height: '8px',
        borderRadius: '50%',
        backgroundColor: colors.danger,
        border: '1px solid var(--bg-primary)',
        flexShrink: 0,
    },
    // tab 顶部状态横幅：左竖边框 + 浅底，醒目但不突兀
    attentionBanner: {
        display: 'flex',
        alignItems: 'flex-start',
        gap: '10px',
        padding: '10px 12px',
        borderRadius: radius.sm,
        backgroundColor: colors.bgPrimary,
        border: `1px solid ${colors.borderPrimary}`,
        borderLeft: '3px solid',
    },
    attentionText: {
        color: colors.textSecondary,
        fontSize: font.sm,
        lineHeight: 1.6,
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
        overflowY: 'auto' as const,
        backgroundColor: colors.bgTertiary,
        minWidth: 0,
    },
    pageContent: {
        ...pageContainer,
    },
    settingsContent: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '24px',
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
    // Orca 风格：卡片容器
    card: {
        ...settingsCard,
    },
    cardTitle: {
        ...cardTitle,
    },
    cardDivider: {
        ...cardDivider,
    },
    // Orca 风格：两列设置行
    row: {
        ...settingRow,
    },
    rowTop: {
        ...settingRowTop,
    },
    rowLeft: {
        ...settingRowLeft,
    },
    rowRight: {
        ...settingRowRight,
    },
    rowLabel: {
        ...settingRowLabel,
    },
    rowDesc: {
        ...settingRowDesc,
    },
    inputWide: {
        ...inputWide,
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
    fontSizeRow: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
    },
    fontPreviewList: {
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))',
        gap: '8px',
    },
    fontPreviewCard: {
        width: '100%',
        minHeight: '70px',
        padding: '10px 12px',
        border: `1px solid ${colors.borderPrimary}`,
        borderRadius: radius.md,
        backgroundColor: colors.bgSecondary,
        color: colors.textSecondary,
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '8px',
        textAlign: 'left' as const,
        cursor: 'pointer',
        transition: 'border-color 160ms ease, background-color 160ms ease',
    },
    fontPreviewCardSelected: {
        border: `1px solid ${colors.accent}`,
        backgroundColor: 'var(--bg-active-soft)',
    },
    fontPreviewHeader: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: '12px',
    },
    fontPreviewName: {
        color: colors.textPrimary,
        fontSize: font.base,
        fontWeight: 600,
    },
    fontPreviewDescription: {
        color: colors.textTertiary,
        fontSize: font.sm,
        marginLeft: '10px',
    },
    fontSelectedIndicator: {
        width: '20px',
        height: '20px',
        borderRadius: '50%',
        backgroundColor: colors.accent,
        color: 'var(--text-on-accent)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        flexShrink: 0,
    },
    fontPreviewSample: {
        color: 'var(--text-secondary)',
        fontSize: '13px',
        lineHeight: 1.45,
        whiteSpace: 'nowrap' as const,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
    },
    themeChoiceRow: {
        display: 'flex',
        gap: '10px',
        flexWrap: 'wrap' as const,
        marginTop: '4px',
    },
    themeChoiceCard: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        gap: '8px',
        minWidth: '130px',
        padding: '11px 18px',
        border: `1px solid ${colors.borderPrimary}`,
        borderRadius: radius.md,
        backgroundColor: colors.bgSecondary,
        color: colors.textSecondary,
        fontSize: font.base,
        fontWeight: 500,
        cursor: 'pointer',
        transition: 'border-color 160ms ease, background-color 160ms ease',
    },
    themeChoiceCardActive: {
        border: `1px solid ${colors.accent}`,
        backgroundColor: 'var(--bg-active-soft)',
        color: colors.textPrimary,
    },
    fontSizeButton: {
        width: '34px',
        height: '34px',
        border: `1px solid ${colors.borderPrimary}`,
        borderRadius: radius.sm,
        backgroundColor: colors.bgSecondary,
        color: colors.textPrimary,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        cursor: 'pointer',
    },
    fontSizeInputWrap: {
        width: '92px',
        height: '34px',
        display: 'flex',
        alignItems: 'center',
        border: `1px solid ${colors.borderPrimary}`,
        borderRadius: radius.sm,
        backgroundColor: colors.bgSecondary,
        overflow: 'hidden',
    },
    fontSizeInput: {
        width: '58px',
        height: '100%',
        padding: '0 8px',
        border: 'none',
        outline: 'none',
        backgroundColor: 'transparent',
        color: colors.textPrimary,
        fontFamily: 'var(--font-mono)',
        fontSize: font.base,
        boxSizing: 'border-box' as const,
    },
    fontSizeUnit: {
        color: colors.textTertiary,
        fontSize: font.sm,
    },
    resetAppearanceButton: {
        height: '34px',
        padding: '0 12px',
        border: `1px solid ${colors.borderPrimary}`,
        borderRadius: radius.sm,
        backgroundColor: 'transparent',
        color: colors.textSecondary,
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
        cursor: 'pointer',
        fontSize: font.base,
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
            backgroundColor: 'var(--accent-hover)',
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
