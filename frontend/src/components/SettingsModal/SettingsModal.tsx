import React, { useState, useEffect, useRef, useMemo } from 'react';
import { TbRobot, TbPalette, TbKeyboard, TbLayoutGrid, TbBooks, TbShieldCheck, TbLock, TbSettings, TbInfoCircle, TbSearch, TbPlugConnected, TbMinus, TbPlus, TbRefresh, TbCheck, TbSun, TbMoon, TbUsers } from 'react-icons/tb';
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
    session_share?: {
        enabled: boolean;
        remote_url: string;
        branch: string;
        secret_key: string;
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
    progress?: number;
    progressLabel?: string;
}

// 会话共享同步状态（结构对齐后端 SessionShareStatus）
interface SessionShareStatus {
    enabled: boolean;
    configured: boolean;
    hasSecretKey: boolean;
    running: boolean;
    pendingCount: number;
    entryCount: number;
    owner?: string;
    lastSyncAt?: string;
    lastSyncSuccess: boolean;
    lastSyncMessage?: string;
    remoteURL?: string;
    branch?: string;
    progress?: number;
    progressLabel?: string;
}

type TabId = 'llm' | 'appearance' | 'terminal' | 'highlight' | 'shortcuts' | 'broadcast' | 'knowledge' | 'sessionshare' | 'aiagent' | 'whitelist' | 'fileaccess' | 'experimental' | 'about';

// Skill 安装条目：每个 AI Agent 目录一行，独立保存检测状态/版本/消息。
// 支撑多个 coding agent（Claude Code / Cursor / Codex 等）并用的场景（issue #54）。
type SkillState = 'unknown' | 'not_installed' | 'up_to_date' | 'outdated';
interface SkillEntry {
    id: string;
    dir: string;
    state: SkillState;
    installedVer: string;
    builtinVer: string;
    msg: string;
}

// 多 skill 目录持久化（issue #54）：
//   opscopilot:skillDirs —— JSON 字符串数组（当前）
//   opscopilot:skillDir  —— 旧版单值（向后兼容，存在则迁移为单元素数组）
const SKILL_DIRS_KEY = 'opscopilot:skillDirs';
const SKILL_DIR_LEGACY_KEY = 'opscopilot:skillDir';

function newSkillId(): string {
    // @ts-ignore
    if (globalThis.crypto?.randomUUID) return globalThis.crypto.randomUUID();
    return `s_${Math.random().toString(16).slice(2)}_${Date.now()}`;
}

function loadSkillDirs(): string[] {
    try {
        const raw = localStorage.getItem(SKILL_DIRS_KEY);
        if (raw) {
            const arr = JSON.parse(raw);
            if (Array.isArray(arr)) return arr.filter((x: any) => typeof x === 'string');
        }
    } catch { /* ignore */ }
    // 兼容旧版单目录存储
    const legacy = localStorage.getItem(SKILL_DIR_LEGACY_KEY);
    return legacy ? [legacy] : [];
}

function saveSkillDirs(dirs: string[]): void {
    localStorage.setItem(SKILL_DIRS_KEY, JSON.stringify(dirs));
}

// 单个 skill 目录的状态徽章文案（多目录列表每行展示，issue #54）
function skillBadgeLabel(state: SkillState): string {
    switch (state) {
        case 'up_to_date': return '已最新';
        case 'outdated': return '可更新';
        case 'not_installed': return '未安装';
        default: return '未检测';
    }
}

// 状态徽章样式：用颜色区分安装状态
function skillBadgeStyle(state: SkillState): React.CSSProperties {
    const map: Record<SkillState, { bg: string; fg: string }> = {
        up_to_date: { bg: 'var(--success-tint)', fg: 'var(--severity-success)' },
        outdated: { bg: 'var(--warning-tint)', fg: 'var(--warning)' },
        not_installed: { bg: 'var(--danger-tint)', fg: 'var(--severity-danger)' },
        unknown: { bg: colors.bgHover, fg: colors.textTertiary },
    };
    const c = map[state];
    return {
        display: 'inline-block',
        padding: '2px 8px',
        borderRadius: '999px',
        fontSize: font.xs,
        backgroundColor: c.bg,
        color: c.fg,
        whiteSpace: 'nowrap',
        flexShrink: 0,
    };
}

// 全局安装/更新按钮文案：按所有目录的聚合状态决定（任一未安装→安装，任一过期→更新，否则→重新安装）
function aggregateInstallLabel(entries: SkillEntry[]): string {
    const checked = entries.filter(e => e.state !== 'unknown');
    if (checked.some(e => e.state === 'not_installed')) return '全部安装';
    if (checked.some(e => e.state === 'outdated')) return '全部更新';
    if (checked.length > 0) return '全部重新安装';
    return '安装/更新';
}

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

const defaultSessionShare = {
    enabled: false,
    remote_url: '',
    branch: 'main',
    secret_key: ''
};

const normalizeSessionShare = (sessionShare?: AppConfig['session_share']) => ({
    ...defaultSessionShare,
    ...sessionShare,
    remote_url: (sessionShare?.remote_url || '').trim(),
    branch: ((sessionShare?.branch || defaultSessionShare.branch).trim() || defaultSessionShare.branch),
    secret_key: (sessionShare?.secret_key || '').trim()
});

const defaultSessionShareStatus: SessionShareStatus = {
    enabled: false,
    configured: false,
    hasSecretKey: false,
    running: false,
    pendingCount: 0,
    entryCount: 0,
    lastSyncSuccess: false,
    lastSyncMessage: '',
    progress: 0
};

const defaultPatchSyncStatus: PatchSyncStatus = {
    enabled: false,
    configured: false,
    running: false,
    pendingCount: 0,
    lastSyncSuccess: false,
    lastSyncMessage: '',
    progress: 0
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
    // 最近一次已落盘配置的 JSON 快照，用于判断是否存在未保存改动（加载/保存成功后更新）
    const [savedConfigJson, setSavedConfigJson] = useState<string | null>(null);
    // 关闭页面时存在未保存改动 → 弹确认框
    const [showUnsavedConfirm, setShowUnsavedConfirm] = useState(false);
    const [activeTab, setActiveTab] = useState<TabId>('llm');
    const [importDir, setImportDir] = useState('');
    const [importLoading, setImportLoading] = useState(false);
    const [importMsg, setImportMsg] = useState('');
    // AI 接入：skill 安装/更新（多目录列表，支撑多个 coding agent 并用，issue #54）
    const [skillEntries, setSkillEntries] = useState<SkillEntry[]>([]);
    const [skillLoading, setSkillLoading] = useState(false);
    // skill 是否需要关注（未配置 / 任一目录未安装或过期）→ 「AI 接入」导航项亮红点
    const [skillNeedsAttention, setSkillNeedsAttention] = useState(false);
    // 高亮规则是否有存量问题（语法错 / 灾难正则，常因用户直改 JSON 引入）→ 「突出显示」导航项亮红点
    const [highlightIssues, setHighlightIssues] = useState<{ name: string; issues: string[] }[]>([]);
    const [searchQuery, setSearchQuery] = useState('');
    const [patchSyncStatus, setPatchSyncStatus] = useState<PatchSyncStatus>(defaultPatchSyncStatus);
    const [patchSyncLoading, setPatchSyncLoading] = useState(false);
    const [sessionShareStatus, setSessionShareStatus] = useState<SessionShareStatus>(defaultSessionShareStatus);
    const [sessionShareLoading, setSessionShareLoading] = useState(false);
    const searchInputRef = useRef<HTMLInputElement>(null);

    // Navigation items structure
    const navItems: NavItem[] = [
        { id: 'llm', label: '模型服务', icon: TbRobot({}), category: 'AI' },
        { id: 'appearance', label: '外观', icon: TbSun({}), category: '终端', keywords: ['终端外观', '终端', '字体', '字号', 'theme', '主题'] },
        { id: 'highlight', label: '突出显示', icon: TbPalette({}), category: '终端' },
        { id: 'shortcuts', label: '快捷键', icon: TbKeyboard({}), category: '交互' },
        { id: 'broadcast', label: '多窗口', icon: TbLayoutGrid({}), category: '交互' },
        { id: 'knowledge', label: '知识共享', icon: TbBooks({}), category: '知识' },
        { id: 'sessionshare', label: '会话共享', icon: TbUsers({}), category: '知识', keywords: ['连接共享', '共享会话', 'session share'] },
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

    // 「会话共享」页激活期间低频轮询状态：让 当前状态/最近同步时间
    // 实时反映启动同步与后台同步的进度，而不是只显示打开页面时的快照
    useEffect(() => {
        if (!isOpen || activeTab !== 'sessionshare') return;
        const id = setInterval(() => { void loadSessionShareStatus(true); }, 2000);
        return () => clearInterval(id);
    }, [isOpen, activeTab]);

    // 「知识共享」页同样实时轮询（两个共享页行为一致）
    useEffect(() => {
        if (!isOpen || activeTab !== 'knowledge') return;
        const id = setInterval(() => { void loadPatchSyncStatus(true); }, 2000);
        return () => clearInterval(id);
    }, [isOpen, activeTab]);

    useEffect(() => {
        if (isOpen) {
            loadSettings();
            loadPatchSyncStatus();
            loadSessionShareStatus();
            setMsg('');
            setShowUnsavedConfirm(false);
            setImportDir('');
            setImportMsg('');
            setSearchQuery('');
            setActiveTab('llm');

            // 回填上次使用的 skill 目录列表，并据此判断「AI 接入」导航项是否需要亮红点。
            // 兼容旧版单目录存储（opscopilot:skillDir）→ 自动迁移为单元素数组。
            const dirs = loadSkillDirs();
            const entries: SkillEntry[] = dirs.map(d => ({
                id: newSkillId(), dir: d, state: 'unknown', installedVer: '', builtinVer: '', msg: '',
            }));
            setSkillEntries(entries);
            if (entries.length === 0) {
                // 从未配置过任何 skill 目录 → 引导用户去配置/安装，亮红点
                setSkillNeedsAttention(true);
            } else {
                // 已配置过：静默批量检测，任一目录非 up_to_date 即亮红点
                void checkSkills(entries);
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
                const next: AppConfig = {
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
                    session_share: normalizeSessionShare(cfg.session_share),
                    command_query_shortcut: cfg.command_query_shortcut || 'Ctrl+K',
                };
                setConfig(next);
                setSavedConfigJson(JSON.stringify(next));
            }
        } catch (e) {
            console.error(e);
            setMsg('加载设置失败');
        } finally {
            setLoading(false);
        }
    };

    // silent = true 时不切换 loading 态，供定时轮询使用（避免按钮禁用态闪烁）
    const loadPatchSyncStatus = async (silent = false) => {
        if (!silent) setPatchSyncLoading(true);
        try {
            // @ts-ignore
            const raw = await window.go.main.App.GetPatchSyncStatus();
            const parsed = raw ? JSON.parse(raw) : {};
            setPatchSyncStatus({
                ...defaultPatchSyncStatus,
                ...parsed
            });
            return { ...defaultPatchSyncStatus, ...parsed } as PatchSyncStatus;
        } catch (e) {
            console.error(e);
            setPatchSyncStatus({
                ...defaultPatchSyncStatus,
                lastSyncMessage: '加载同步状态失败'
            });
            return null;
        } finally {
            if (!silent) setPatchSyncLoading(false);
        }
    };

    // silent = true 时不切换 loading 态，供定时轮询使用（避免按钮禁用态闪烁）
    const loadSessionShareStatus = async (silent = false) => {
        if (!silent) setSessionShareLoading(true);
        try {
            // @ts-ignore
            const raw = await window.go.main.App.GetSessionShareStatus();
            const parsed = raw ? JSON.parse(raw) : {};
            setSessionShareStatus({
                ...defaultSessionShareStatus,
                ...parsed
            });
            return { ...defaultSessionShareStatus, ...parsed } as SessionShareStatus;
        } catch (e) {
            console.error(e);
            setSessionShareStatus({
                ...defaultSessionShareStatus,
                lastSyncMessage: '加载会话共享状态失败'
            });
            return null;
        } finally {
            if (!silent) setSessionShareLoading(false);
        }
    };

    const formatShortcutLabel = (shortcut: string) => {
        const normalized = (shortcut || '').trim();
        return normalized || 'Ctrl+K';
    };

    // 是否有未保存改动：对比当前工作副本与最近一次已落盘快照。
    // 主题不经过 config（走 onThemeChange 即时落盘），因此不会误报为脏。
    const isDirty = useMemo(() => {
        if (!config || savedConfigJson === null) return false;
        return JSON.stringify(config) !== savedConfigJson;
    }, [config, savedConfigJson]);

    // 统一落盘逻辑，保存成功返回 true。closeAfter 为 true 时保存成功后关闭页面，
    // 用于「保存并关闭」；普通「保存更改」只提示成功、不关闭。
    const persistConfig = async (closeAfter: boolean): Promise<boolean> => {
        if (!config) return false;
        setLoading(true);
        try {
            const nextConfig = {
                ...config,
                cli: normalizeCliConfig(config.cli),
                terminal: normalizeTerminalConfig(config.terminal),
                patch_store: normalizePatchStore(config.patch_store),
                session_share: normalizeSessionShare(config.session_share)
            };
            setConfig(nextConfig);
            // @ts-ignore
            const err = await window.go.main.App.SaveSettings(nextConfig);
            if (err) {
                setMsg('错误: ' + err);
                return false;
            }
            // 落盘成功后更新快照，避免 isDirty 仍判定为「有未保存改动」
            setSavedConfigJson(JSON.stringify(nextConfig));
            setMsg('设置已保存！');
            await loadPatchSyncStatus();
            await loadSessionShareStatus();
            if (onCompletionDelayChange && nextConfig.completion_delay !== undefined) {
                onCompletionDelayChange(nextConfig.completion_delay);
            }
            if (onHighlightRulesChange) {
                onHighlightRulesChange(nextConfig.highlight_rules || []);
            }
            if (onTerminalConfigChange) {
                onTerminalConfigChange(nextConfig.terminal);
            }
            if (closeAfter) {
                onClose();
            } else {
                setTimeout(() => setMsg(''), 2000);
            }
            return true;
        } catch (e: any) {
            setMsg('错误: ' + e.toString());
            return false;
        } finally {
            setLoading(false);
        }
    };

    const handleSave = () => {
        void persistConfig(false);
    };

    const handleClose = () => {
        if (isDirty) {
            setShowUnsavedConfirm(true);
            return;
        }
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

    const handleSessionShareChange = (key: keyof NonNullable<AppConfig['session_share']>, value: string | boolean) => {
        if (!config) return;
        setConfig({
            ...config,
            session_share: {
                ...normalizeSessionShare(config.session_share),
                [key]: value
            }
        });
    };

    const handleRetrySessionShareSync = async () => {
        // 表单有未保存更改时先保存：点「立即重试同步」的意图是
        // 用当前填写的内容同步，而非上次落盘的旧配置
        if (isDirty) {
            const saved = await persistConfig(false);
            if (!saved) return; // 保存失败时中止，错误信息已展示
        }
        setSessionShareLoading(true);
        try {
            // @ts-ignore
            const err = await window.go.main.App.RetrySessionShareSync();
            if (err) {
                setMsg('会话共享同步失败: ' + err);
                setSessionShareLoading(false);
                return;
            }
            setMsg('正在同步会话共享...');

            // 同步在后端异步执行：轮询状态直到结束，让「当前状态」
            // 实时从 同步中 → 完成/失败 过渡，而不是卡在旧状态直到突然完成
            for (let i = 0; i < 45; i++) {
                await new Promise(r => setTimeout(r, 800));
                const status = await loadSessionShareStatus(true);
                if (status && !status.running) {
                    setMsg(status.lastSyncSuccess ? '会话共享同步完成' : `会话共享同步失败: ${status.lastSyncMessage || '未知错误'}`);
                    break;
                }
            }
        } catch (e: any) {
            setMsg('会话共享同步失败: ' + e.toString());
        } finally {
            setSessionShareLoading(false);
        }
    };

    const renderSessionShareState = () => {
        if (sessionShareLoading) return '正在读取状态...';
        if (sessionShareStatus.running) {
            const label = sessionShareStatus.progressLabel || '同步中';
            const pct = Math.max(5, Math.min(100, sessionShareStatus.progress ?? 5));
            return `${label}（${pct}%）`;
        }
        if (!sessionShareStatus.enabled) return '已关闭';
        if (!sessionShareStatus.configured) return '待配置仓库';
        if (!sessionShareStatus.hasSecretKey) return '待配置共享密钥';
        if (sessionShareStatus.lastSyncMessage) return sessionShareStatus.lastSyncMessage;
        return '待同步';
    };

    const handleRetryPatchSync = async () => {
        // 表单有未保存更改时先保存（与会话共享的重试行为一致）
        if (isDirty) {
            const saved = await persistConfig(false);
            if (!saved) return;
        }

        setPatchSyncLoading(true);
        try {
            // @ts-ignore
            const err = await window.go.main.App.RetryPatchSync();
            if (err) {
                setMsg('补丁同步失败: ' + err);
                setPatchSyncLoading(false);
                return;
            }
            setMsg('正在同步知识共享...');

            // 同步在后端异步执行：轮询状态直到结束
            for (let i = 0; i < 45; i++) {
                await new Promise(r => setTimeout(r, 800));
                const status = await loadPatchSyncStatus(true);
                if (status && !status.running) {
                    setMsg(status.lastSyncSuccess ? '知识共享同步完成' : `知识共享同步失败: ${status.lastSyncMessage || '未知错误'}`);
                    break;
                }
            }
        } catch (e: any) {
            setMsg('补丁同步失败: ' + e.toString());
        } finally {
            setPatchSyncLoading(false);
        }
    };

    const renderPatchSyncState = () => {
        if (patchSyncLoading) return '正在读取同步状态...';
        if (patchSyncStatus.running) {
            const label = patchSyncStatus.progressLabel || '同步中';
            const pct = Math.max(5, Math.min(100, patchSyncStatus.progress ?? 5));
            return `${label}（${pct}%）`;
        }
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

    // 多目录列表操作（issue #54）：增/改/删一个 skill 目录条目。
    // 改动后同步持久化目录数组，并重置该条目状态（避免显示陈旧的版本对比）。
    const persistSkillDirs = (next: SkillEntry[]) => {
        setSkillEntries(next);
        saveSkillDirs(next.map(e => e.dir.trim()).filter(d => d !== ''));
    };
    const addSkillDir = () => {
        persistSkillDirs([...skillEntries, { id: newSkillId(), dir: '', state: 'unknown', installedVer: '', builtinVer: '', msg: '' }]);
    };
    const updateSkillDir = (id: string, dir: string) => {
        persistSkillDirs(skillEntries.map(e => e.id === id ? { ...e, dir, state: 'unknown', installedVer: '', builtinVer: '', msg: '' } : e));
    };
    const removeSkillDir = (id: string) => {
        persistSkillDirs(skillEntries.filter(e => e.id !== id));
    };

    // 批量检测所有目录下是否已安装 skill 以及版本是否最新（issue #54 多目录）。
    // 对每个 entry 独立调用 CheckSkillStatus，并发执行；任一非 up_to_date 则导航亮红点。
    const checkSkills = async (entries: SkillEntry[]) => {
        const valid = entries.filter(e => e.dir.trim() !== '');
        if (valid.length === 0) {
            setSkillNeedsAttention(true);
            return;
        }
        // @ts-ignore
        const app: any = window.go?.main?.App;
        if (!app?.CheckSkillStatus) return;
        const results = await Promise.all(valid.map(async e => {
            try {
                const raw = await app.CheckSkillStatus(e.dir.trim());
                const r = raw ? JSON.parse(raw) : {};
                if (r.success === false) {
                    return { id: e.id, state: 'unknown' as SkillState, installedVer: '', builtinVer: '', msg: r.error || '检测失败' };
                }
                const state: SkillState = r.state || 'unknown';
                let msg = '检测完成';
                if (state === 'not_installed') msg = '该目录下尚未安装 OpsCopilot skill';
                else if (state === 'up_to_date') msg = `已是最新版本（v${r.installed}）`;
                else if (state === 'outdated') msg = `已安装 v${r.installed}，可更新至 v${r.builtin}`;
                return { id: e.id, state, installedVer: r.installed || '', builtinVer: r.builtin || '', msg };
            } catch (err: any) {
                return { id: e.id, state: 'unknown' as SkillState, installedVer: '', builtinVer: '', msg: '检测失败: ' + err.toString() };
            }
        }));
        setSkillEntries(prev => prev.map(e => {
            const r = results.find(x => x.id === e.id);
            return r ? { ...e, state: r.state, installedVer: r.installedVer, builtinVer: r.builtinVer, msg: r.msg } : e;
        }));
        // 任一已检测目录非 up_to_date，即需要关注（导航亮红点）。
        // 用最新的 results 判断，而非可能过时的传入 entries。
        setSkillNeedsAttention(results.some(r => r.state !== 'up_to_date'));
    };

    const handleCheckSkill = async () => {
        const trimmed = skillEntries.map(e => e.dir.trim());
        if (skillEntries.length === 0 || trimmed.every(d => d === '')) {
            setSkillNeedsAttention(true);
            return;
        }
        setSkillLoading(true);
        await checkSkills(skillEntries);
        setSkillLoading(false);
    };

    // 批量安装（或更新）所有目录下的 opscopilot-ops/ 子目录（issue #54 多目录）。
    // 逐目录串行安装（避免并发写盘竞态），完成后统一刷新检测状态。
    const handleInstallSkill = async () => {
        const valid = skillEntries.filter(e => e.dir.trim() !== '');
        if (valid.length === 0) {
            setSkillNeedsAttention(true);
            return;
        }
        setSkillLoading(true);
        // @ts-ignore
        const app: any = window.go?.main?.App;
        if (!app?.InstallSkill) {
            setSkillLoading(false);
            return;
        }
        // 标记进行中
        setSkillEntries(prev => prev.map(e => e.dir.trim() === '' ? e : { ...e, msg: '正在安装...' }));
        const outcomes: { id: string; ok: boolean; msg: string }[] = [];
        for (const e of valid) {
            try {
                const raw = await app.InstallSkill(e.dir.trim());
                const r = raw ? JSON.parse(raw) : {};
                if (r.success === false) {
                    outcomes.push({ id: e.id, ok: false, msg: r.error || '安装失败' });
                } else {
                    outcomes.push({ id: e.id, ok: true, msg: `已安装到 ${r.path}（v${r.version}）` });
                }
            } catch (err: any) {
                outcomes.push({ id: e.id, ok: false, msg: '安装失败: ' + err.toString() });
            }
        }
        setSkillEntries(prev => prev.map(e => {
            const o = outcomes.find(x => x.id === e.id);
            return o ? { ...e, msg: o.msg } : e;
        }));
        // 安装后统一刷新检测状态
        await checkSkills(skillEntries);
        setSkillLoading(false);
    };

    if (!isOpen || !config) return null;

    // AI 接入 tab 顶部的状态横幅：当 skill 需要关注时（未配置/未安装/过期），
    // 明确告诉用户「红点是因为什么 + 下一步该干嘛」，避免点开 tab 后不知所措。
    // 多目录场景（issue #54）按聚合状态展示：无目录 / 部分未安装 / 部分可更新。
    const renderSkillAttentionBanner = () => {
        const valid = skillEntries.filter(e => e.dir.trim() !== '');
        const checked = valid.filter(e => e.state !== 'unknown');
        const notInstalled = checked.filter(e => e.state === 'not_installed').length;
        const outdated = checked.filter(e => e.state === 'outdated').length;

        let message = '';
        let tone: 'warning' | 'accent' = 'accent';
        if (valid.length === 0) {
            message = '尚未配置 skill 目录，AI Agent 无法调用 OpsCopilot。请填写目录后点击「检测状态」。';
            tone = 'warning';
        } else if (checked.length === 0) {
            message = `已配置 ${valid.length} 个目录，请点击「检测状态」查看安装情况。`;
            tone = 'accent';
        } else if (notInstalled > 0) {
            message = `${notInstalled} 个目录下尚未安装 OpsCopilot skill，请点击「安装」。`;
            tone = 'warning';
        } else if (outdated > 0) {
            message = `${outdated} 个目录的 skill 有新版本可更新，建议点击「更新」。`;
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
                            <div style={{ ...styles.rowDesc, marginLeft: '0', marginBottom: '10px' }}>
                                开启后，故障排查会话归档时会把排查场景自动推送到团队共享 Git 仓库；
                                团队成员归档的知识也会同步合并到你的本地知识库，供 AI 检索和问答使用。
                            </div>
                            <div style={styles.row}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>启用知识共享</div>
                                    <div style={styles.rowDesc}>
                                        保存后立即生效并触发一次同步；关闭后归档的知识仅保留在本地
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
                                        {patchSyncStatus.lastSyncMessage || '存在未保存更改时点击会先保存再同步'}
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
                                        onClick={() => { void loadPatchSyncStatus(); }}
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

            case 'sessionshare':
                return (
                    <div style={styles.settingsGroup}>
                        <div style={styles.card}>
                            <div style={styles.cardTitle}>会话共享</div>
                            <div style={{ ...styles.rowDesc, marginLeft: '0', marginBottom: '10px' }}>
                                开启后，每次连接成功会自动把该连接信息（密码加密后）推送到团队共享 Git 仓库；
                                团队成员配置相同的仓库地址、分支与共享密钥，即可在会话管理面板互相看到最近使用的连接。
                            </div>
                            <div style={styles.row}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>启用会话共享</div>
                                    <div style={styles.rowDesc}>
                                        保存后立即生效并触发一次同步；关闭后不再记录与推送
                                    </div>
                                </div>
                                <div style={styles.rowRight}>
                                    <Switch
                                        checked={!!config.session_share?.enabled}
                                        onChange={(v) => handleSessionShareChange('enabled', v)}
                                    />
                                    <span style={{ color: colors.textSecondary, fontSize: font.base }}>
                                        {config.session_share?.enabled ? '已开启' : '已关闭'}
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
                                        value={config.session_share?.remote_url || ''}
                                        onChange={(e) => handleSessionShareChange('remote_url', e.target.value)}
                                        placeholder="例如：git@github.com:team/opscopilot-sessions.git"
                                    />
                                </div>
                            </div>
                            <div style={styles.row}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>同步分支</div>
                                    <div style={styles.rowDesc}>
                                        共享者标识自动读取本机 `git config user.name`，无需手动填写
                                    </div>
                                </div>
                                <div style={styles.rowRight}>
                                    <input
                                        style={styles.inputWide}
                                        value={config.session_share?.branch || defaultSessionShare.branch}
                                        onChange={(e) => handleSessionShareChange('branch', e.target.value)}
                                        placeholder="main"
                                    />
                                </div>
                            </div>
                            <div style={styles.row}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>共享密钥</div>
                                    <div style={styles.rowDesc}>
                                        用于加密仓库中的连接密码；团队成员需配置同一密钥才能互相解密连接
                                    </div>
                                </div>
                                <div style={styles.rowRight}>
                                    <input
                                        style={styles.inputWide}
                                        type="password"
                                        value={config.session_share?.secret_key || ''}
                                        onChange={(e) => handleSessionShareChange('secret_key', e.target.value)}
                                        placeholder="团队约定的共享密钥"
                                        autoComplete="new-password"
                                    />
                                </div>
                            </div>
                        </div>
                        <div style={styles.card}>
                            <div style={styles.cardTitle}>同步状态</div>
                            <div style={{ ...styles.statusGrid, maxWidth: '760px' }}>
                                <div style={styles.statusCard}>
                                    <div style={styles.statusCardLabel}>当前状态</div>
                                    <div style={styles.statusCardValue}>{renderSessionShareState()}</div>
                                </div>
                                <div style={styles.statusCard}>
                                    <div style={styles.statusCardLabel}>共享条目</div>
                                    <div style={styles.statusCardValue}>{sessionShareStatus.entryCount}</div>
                                </div>
                                <div style={styles.statusCard}>
                                    <div style={styles.statusCardLabel}>待推送登录</div>
                                    <div style={styles.statusCardValue}>{sessionShareStatus.pendingCount}</div>
                                </div>
                                <div style={styles.statusCard}>
                                    <div style={styles.statusCardLabel}>最近同步时间</div>
                                    <div style={styles.statusCardValue}>{sessionShareStatus.lastSyncAt || '暂无'}</div>
                                </div>
                            </div>
                            <div style={styles.cardDivider} />
                            <div style={styles.row}>
                                <div style={styles.rowLeft}>
                                    <div style={styles.rowLabel}>手动重试</div>
                                    <div style={styles.rowDesc}>
                                        {sessionShareStatus.lastSyncMessage || '存在未保存更改时点击会先保存再同步'}
                                    </div>
                                </div>
                                <div style={styles.rowRight}>
                                    <button
                                        onClick={handleRetrySessionShareSync}
                                        style={styles.secondaryButton}
                                        disabled={sessionShareLoading || sessionShareStatus.running || !sessionShareStatus.enabled || !sessionShareStatus.configured}
                                    >
                                        {sessionShareStatus.running ? '正在同步...' : '立即重试同步'}
                                    </button>
                                    <button
                                        onClick={() => { void loadSessionShareStatus(); }}
                                        style={styles.secondaryButton}
                                        disabled={sessionShareLoading}
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
                            <div style={{ ...styles.rowDesc, marginLeft: '0', marginBottom: '10px' }}>
                                以 Claude Code skill 格式安装到指定目录（opscopilot-ops/ 子目录），
                                AI Agent 即可调用 OpsCopilot 执行运维操作和故障诊断。
                                支持配置多个目录，以支撑多个 coding agent（Claude Code / Cursor / Codex 等）并用。
                            </div>

                            {/* 多目录列表：每行一个目录 + 状态徽章 + 删除按钮 + 该行检测/安装结果 */}
                            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                                {skillEntries.length === 0 && (
                                    <div style={{ ...styles.rowDesc, marginLeft: '0' }}>
                                        暂无目录，点击下方「添加目录」开始配置。
                                    </div>
                                )}
                                {skillEntries.map((e) => (
                                    <div key={e.id} style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
                                        <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
                                            <input
                                                style={{ ...styles.inputWide, flex: 1 }}
                                                value={e.dir}
                                                onChange={(ev) => updateSkillDir(e.id, ev.target.value)}
                                                placeholder="例如：C:\\Users\\xxx\\.claude\\skills"
                                            />
                                            <span style={skillBadgeStyle(e.state)}>{skillBadgeLabel(e.state)}</span>
                                            <button
                                                onClick={() => removeSkillDir(e.id)}
                                                style={{ ...styles.secondaryButton, padding: '4px 10px' }}
                                                title="移除该目录"
                                                disabled={skillLoading}
                                            >
                                                ×
                                            </button>
                                        </div>
                                        {e.msg && (
                                            <div style={{
                                                ...styles.rowDesc,
                                                marginLeft: '0',
                                                color: e.state === 'up_to_date' ? colors.success : colors.textSecondary,
                                            }}>
                                                {e.msg}
                                            </div>
                                        )}
                                    </div>
                                ))}
                            </div>

                            {/* 全局操作：添加目录 + 批量检测/安装。操作粒度为全部目录（issue #54）。 */}
                            <div style={{ display: 'flex', gap: '8px', marginTop: '12px', alignItems: 'center', flexWrap: 'wrap' }}>
                                <button
                                    onClick={addSkillDir}
                                    style={styles.secondaryButton}
                                    disabled={skillLoading}
                                >
                                    + 添加目录
                                </button>
                                <div style={{ flex: 1 }} />
                                <button
                                    onClick={handleCheckSkill}
                                    style={styles.secondaryButton}
                                    disabled={skillLoading || skillEntries.length === 0}
                                >
                                    {skillLoading ? '检测中...' : '检测状态'}
                                </button>
                                <button
                                    onClick={handleInstallSkill}
                                    style={styles.secondaryButton}
                                    disabled={skillLoading || skillEntries.length === 0}
                                >
                                    {aggregateInstallLabel(skillEntries)}
                                </button>
                            </div>
                            <div style={{ ...styles.rowDesc, marginLeft: '0', marginTop: '8px' }}>
                                命令路径会自动替换为本机 opscopilot.exe 的绝对路径。
                            </div>
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
                    {/* 左上角返回入口：把标题做成可点击的「← 系统设置」，
                        贴近视口左上角、贴近用户高频的左侧导航区。
                        右上 × 与右下「取消」保留不动，这里只是新增更顺手的入口。 */}
                    <button
                        onClick={handleClose}
                        style={styles.backTitle}
                        title="返回"
                        className="settings-back-title"
                    >
                        <span style={styles.backArrow} className="settings-back-arrow">←</span>
                        <span>系统设置</span>
                    </button>
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

            {/* 关闭确认：存在未保存改动时提醒用户保存/放弃/继续编辑 */}
            {showUnsavedConfirm && (
                <div style={styles.unsavedOverlay}>
                    <div style={styles.unsavedModal}>
                        <h3 style={styles.unsavedTitle}>有未保存的更改</h3>
                        <p style={styles.unsavedMessage}>
                            当前设置页还有改动尚未保存，直接关闭将丢失这些改动。
                        </p>
                        <div style={styles.unsavedActions}>
                            <button style={styles.cancelBtn} onClick={() => setShowUnsavedConfirm(false)}>继续编辑</button>
                            <button style={styles.discardBtn} onClick={() => { setShowUnsavedConfirm(false); onClose(); }}>放弃更改</button>
                            <button style={styles.saveBtn} onClick={() => { setShowUnsavedConfirm(false); void persistConfig(true); }} disabled={loading}>
                                保存并关闭
                            </button>
                        </div>
                    </div>
                </div>
            )}
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
    // 左上角「← 系统设置」返回按钮：看似标题，hover 有反馈（hover 效果由
    // settings-back-title / settings-back-arrow 两个 class 在全局 CSS 实现，
    // 因为 React 内联 style 不支持 :hover 伪类）。
    backTitle: {
        margin: 0,
        fontSize: '1.05rem',
        color: colors.textPrimary,
        fontWeight: 600,
        background: 'none',
        border: 'none',
        cursor: 'pointer',
        padding: '6px 10px',
        borderRadius: radius.sm,
        display: 'inline-flex',
        alignItems: 'center',
        gap: '8px',
        fontFamily: 'inherit',
    },
    backArrow: {
        color: colors.textTertiary,
        fontSize: '1rem',
        lineHeight: 1,
        transition: 'color 0.15s ease',
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
        backgroundColor: 'var(--bg-settings-canvas)',
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
        color: 'var(--text-on-accent)',
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
    // 关闭确认弹层：覆盖在设置页之上，zIndex 略高于设置页
    unsavedOverlay: {
        position: 'fixed' as const,
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: colors.overlay,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 2100,
    },
    unsavedModal: {
        backgroundColor: colors.bgSecondary,
        borderRadius: radius.lg,
        padding: '24px',
        width: '420px',
        maxWidth: '90vw',
        boxShadow: '0 4px 12px var(--shadow)',
    },
    unsavedTitle: {
        color: colors.textPrimary,
        fontSize: font.base,
        fontWeight: 600,
        margin: '0 0 12px 0',
    },
    unsavedMessage: {
        color: colors.textSecondary,
        fontSize: font.base,
        margin: '0 0 20px 0',
    },
    unsavedActions: {
        display: 'flex',
        gap: '10px',
        justifyContent: 'flex-end',
    },
    discardBtn: {
        padding: '8px 16px',
        borderRadius: radius.sm,
        border: '1px solid var(--danger-border)',
        backgroundColor: 'var(--danger-bg-subtle)',
        color: colors.danger,
        cursor: 'pointer',
        fontWeight: 500,
        fontSize: font.base,
    },
};

export default SettingsModal;
