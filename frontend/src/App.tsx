import { useState, useRef, useEffect, useCallback, useMemo } from 'react';
import { TbClock, TbScreenShare, TbStethoscope, TbMessageChatbot, TbCode, TbBolt, TbBook } from 'react-icons/tb';
import { useToast } from './components/Toast/Toast';
import './App.css';
import logo from './assets/images/logo-universal.png';
import { TerminalRef } from './components/Terminal/Terminal';
import FlexLayoutAdapter from './components/FlexLayout/FlexLayoutAdapter';
import QuickCommandPanel from './components/QuickCommandPanel/QuickCommandPanel';
import BottomBar from './components/BottomBar/BottomBar';
import SmartConnectModal from './components/SmartConnectModal/SmartConnectModal';
import Sidebar from './components/Sidebar/Sidebar';
import SettingsModal from './components/SettingsModal/SettingsModal';
import ConfirmCloseModal from './components/ConfirmCloseModal/ConfirmCloseModal';
import CommandQueryOverlay, { CommandQueryResult } from './components/CommandQueryOverlay/CommandQueryOverlay';
import ConnectErrorModal from './components/ConnectErrorModal/ConnectErrorModal';
import { ConnectionConfig, SessionStatus, SessionDisconnectedEvent } from './types';
import { HighlightRule, TerminalConfig } from './components/Terminal/highlightTypes';
import { assessPattern } from './components/Terminal/highlight/regexSafety';
import { normalizeTerminalConfig } from './components/Terminal/terminalAppearance';
import { Theme } from './components/appearanceTypes';
import { DEFAULT_THEME, normalizeTheme, persistTheme, readPersistedTheme } from './components/appearance';
import { TimestampResult } from './utils/timestampParser';
import { KnowledgeTarget } from './components/AI';

interface TerminalSession {
    id: string;
    title: string;
    status: SessionStatus;
    config?: ConnectionConfig;
    disconnectReason?: string;
}

const DAILY_UPDATE_CHECK_HOUR = 8;
const ATTENTION_DOT_COLOR = 'var(--danger)';

const getDelayUntilNextDailyUpdateCheck = (now = new Date()) => {
    const next = new Date(now);
    next.setHours(DAILY_UPDATE_CHECK_HOUR, 0, 0, 0);
    if (next.getTime() <= now.getTime()) {
        next.setDate(next.getDate() + 1);
    }
    return next.getTime() - now.getTime();
};

function App() {
    const toast = useToast();
    const [status, setStatus] = useState("就绪");
    const [isSmartModalOpen, setIsSmartModalOpen] = useState(false);
    const [isSettingsOpen, setIsSettingsOpen] = useState(false);
    const [isSidebarOpen, setIsSidebarOpen] = useState(false);
    const [sidebarTab, setSidebarTab] = useState<'sessions' | 'troubleshoot' | 'chat' | 'script' | 'knowledge'>('sessions');
    const [terminals, setTerminals] = useState<TerminalSession[]>([]);
    const [activeTerminalId, setActiveTerminalId] = useState<string | null>(null);
    const [knowledgeTarget, setKnowledgeTarget] = useState<KnowledgeTarget | null>(null);
    const [isBroadcastMode, setIsBroadcastMode] = useState(false);
    const [broadcastIds, setBroadcastIds] = useState<string[]>([]);
    const [isConfirmCloseOpen, setIsConfirmCloseOpen] = useState(false);
    const [confirmCloseMessage, setConfirmCloseMessage] = useState("");
    const [completionDelay, setCompletionDelay] = useState(150);
    const [terminalConfig, setTerminalConfig] = useState<TerminalConfig>(() => normalizeTerminalConfig());
    // 主题初值读 localStorage（与 index.html 防闪屏脚本同源），保证 React 首帧与内联脚本一致。
    const [theme, setTheme] = useState<Theme>(() => readPersistedTheme());
    const [highlightRules, setHighlightRules] = useState<HighlightRule[]>([]);
    // 高亮规则存量校验：存在语法错/灾难正则（!canEnable）时，外层设置按钮亮红点，
    // 与 SettingsModal 内「突出显示」导航项的红点形成层层引导。直接由 highlightRules 派生，
    // 规则一变（含 JSON 直改后重新加载）即重算，无需额外同步。
    const highlightNeedsAttention = useMemo(
        () => highlightRules.some(r => !assessPattern(r.pattern || '').canEnable),
        [highlightRules]
    );
    const [isCommandQueryOpen, setIsCommandQueryOpen] = useState(false);
    const [commandQueryText, setCommandQueryText] = useState('');
    const [commandQueryLoading, setCommandQueryLoading] = useState(false);
    const [commandQueryResult, setCommandQueryResult] = useState<CommandQueryResult | null>(null);
    const [commandQueryError, setCommandQueryError] = useState('');
    const commandQueryShortcut = 'Ctrl+K';
    // connectError 携带的额外信息：reopenNewConnect 标记「新建连接失败」（区别于重连失败），
    // failedConfigs 保存失败的配置，供关闭错误弹窗后带回 SmartConnectModal 编辑重试。
    const [connectErrors, setConnectErrors] = useState<{ title: string; message: string; reopenNewConnect?: boolean; failedConfigs?: ConnectionConfig[] }[]>([]);
    // 连接失败带回的配置：dismissConnectError 写入，SmartConnectModal 打开时作为 initialConfigs 预填。
    const [reconnectSeedConfigs, setReconnectSeedConfigs] = useState<ConnectionConfig[]>([]);
    const [parsedTimestamp, setParsedTimestamp] = useState<TimestampResult | null>(null);
    const [updateAvailable, setUpdateAvailable] = useState(false);
    const statusResetTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

    // Refs to hold latest state for callbacks
    const isBroadcastModeRef = useRef(isBroadcastMode);
    const broadcastIdsRef = useRef(broadcastIds);

    // Update refs when state changes
    useEffect(() => {
        isBroadcastModeRef.current = isBroadcastMode;
    }, [isBroadcastMode]);

    useEffect(() => {
        broadcastIdsRef.current = broadcastIds;
    }, [broadcastIds]);
    const terminalRefs = useRef(new Map<string, TerminalRef>());
    const terminalConfigStateRef = useRef<TerminalConfig>(terminalConfig);
    const terminalConfigSaveTimerRef = useRef<number | null>(null);

    const handleTerminalConfigChange = useCallback((nextConfig: TerminalConfig) => {
        const normalized = normalizeTerminalConfig(nextConfig);
        terminalConfigStateRef.current = normalized;
        setTerminalConfig(normalized);

        if (terminalConfigSaveTimerRef.current !== null) {
            window.clearTimeout(terminalConfigSaveTimerRef.current);
        }
        terminalConfigSaveTimerRef.current = window.setTimeout(async () => {
            terminalConfigSaveTimerRef.current = null;
            try {
                // @ts-ignore
                await window.go?.main?.App?.SaveTerminalConfig?.(terminalConfigStateRef.current);
            } catch (error) {
                console.error('Failed to save terminal settings:', error);
            }
        }, 350);
    }, []);

    const handleTerminalFontSizeChange = useCallback((fontSize: number) => {
        handleTerminalConfigChange({ ...terminalConfigStateRef.current, font_size: fontSize });
    }, [handleTerminalConfigChange]);

    useEffect(() => () => {
        if (terminalConfigSaveTimerRef.current !== null) {
            window.clearTimeout(terminalConfigSaveTimerRef.current);
        }
    }, []);
    const scheduleFitAll = useCallback((delay = 120) => {
        setTimeout(() => {
            terminalRefs.current.forEach(t => t.fit());
        }, delay);
    }, []);
    // 主题变化时广播到所有已打开终端（仿 scheduleFitAll 的全实例遍历范式）。
    // 新建的 tab 由 theme prop 经 FlexLayoutAdapter 透传覆盖。
    const applyThemeAll = useCallback((nextTheme: Theme) => {
        terminalRefs.current.forEach(t => t.applyTheme?.(nextTheme));
    }, []);

    // 主题驱动：写 data-theme 属性（CSS 变量据此切换）+ 持久化 localStorage +
    // 广播到所有终端实例。data-theme 与首屏内联脚本写的是同一属性，首帧已就位，这里负责后续切换。
    useEffect(() => {
        document.documentElement.dataset.theme = theme;
        persistTheme(theme);
        applyThemeAll(theme);
    }, [theme, applyThemeAll]);

    // 主题切换（顶栏快捷按钮 + 设置面板共用）：更新 state 并落盘后端。
    // useEffect([theme]) 会负责写 data-theme/localStorage/广播终端；这里只做持久化到配置文件。
    const themeSaveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const handleThemeChange = useCallback((nextTheme: Theme) => {
        setTheme(nextTheme);
        if (themeSaveTimerRef.current) clearTimeout(themeSaveTimerRef.current);
        themeSaveTimerRef.current = setTimeout(async () => {
            themeSaveTimerRef.current = null;
            try {
                // @ts-ignore
                const cfg = await window.go?.main?.App?.GetSettings?.();
                if (cfg) {
                    cfg.appearance = { theme: nextTheme };
                    // @ts-ignore
                    await window.go?.main?.App?.SaveSettings?.(cfg);
                }
            } catch (e) {
                console.error('Failed to save appearance:', e);
            }
        }, 200);
    }, []);
    const handleThemeToggle = useCallback(() => {
        handleThemeChange(theme === 'dark' ? 'light' : 'dark');
    }, [theme, handleThemeChange]);
    // Store unlisten functions for events
    const unlisteners = useRef(new Map<string, () => void>());
    const activeConnectError = connectErrors.length > 0 ? connectErrors[0] : null;

    const enqueueConnectError = (
        title: string,
        message: string,
        extra?: { reopenNewConnect?: boolean; failedConfigs?: ConnectionConfig[] }
    ) => {
        setConnectErrors(prev => [...prev, { title, message, ...extra }]);
    };

    const dismissConnectError = () => {
        // activeConnectError 是当前展示的错误（队列首条）。仅「新建连接失败」关闭后
        // 重开配置界面并带回失败配置；重连失败保持原行为，留在当前 tab。
        if (activeConnectError?.reopenNewConnect && activeConnectError.failedConfigs && activeConnectError.failedConfigs.length > 0) {
            setReconnectSeedConfigs(activeConnectError.failedConfigs);
            setIsSmartModalOpen(true);
        }
        setConnectErrors(prev => prev.slice(1));
    };

    const setStatusWithAutoReset = (message: string, timeoutMs = 3000) => {
        if (statusResetTimerRef.current) {
            clearTimeout(statusResetTimerRef.current);
            statusResetTimerRef.current = null;
        }
        setStatus(message);
        statusResetTimerRef.current = setTimeout(() => {
            setStatus(prev => (prev === message ? "就绪" : prev));
            statusResetTimerRef.current = null;
        }, timeoutMs);
    };

    useEffect(() => {
        // Listen for session closed events from backend
        let cancelClose: (() => void) | undefined;
        let cancelDisconnected: (() => void) | undefined;
        let cancelConfirmClose: (() => void) | undefined;

        // @ts-ignore
        if (window.runtime && window.runtime.EventsOn) {
            // @ts-ignore
            cancelClose = window.runtime.EventsOn("session-closed", (id: string) => {
                removeTerminal(id);
            });

            // Listen for session-disconnected events (保留会话，不关闭tab)
            // @ts-ignore
            cancelDisconnected = window.runtime.EventsOn("session-disconnected", (event: SessionDisconnectedEvent) => {
                console.debug("[App] Session disconnected:", event);
                setTerminals(prev => prev.map(t =>
                    t.id === event.sessionId
                        ? { ...t, status: SessionStatus.DISCONNECTED, disconnectReason: event.message }
                        : t
                ));
            });

            // Listen for confirm-close event from backend
            // @ts-ignore
            cancelConfirmClose = window.runtime.EventsOn("confirm-close", (data: any) => {
                console.debug("[App] Received confirm-close event:", data);
                setConfirmCloseMessage(data.message || "确定要关闭应用吗？");
                setIsConfirmCloseOpen(true);
            });
        }
        return () => {
            if (statusResetTimerRef.current) {
                clearTimeout(statusResetTimerRef.current);
                statusResetTimerRef.current = null;
            }
            if (cancelClose) cancelClose();
            if (cancelDisconnected) cancelDisconnected();
            if (cancelConfirmClose) cancelConfirmClose();
            // Cleanup all terminal listeners
            unlisteners.current.forEach(u => u());
            unlisteners.current.clear();
        };
    }, []);

    // Load settings on mount
    useEffect(() => {
        const loadSettings = async () => {
            try {
                // @ts-ignore
                if (window.go && window.go.main && window.go.main.App && window.go.main.App.GetSettings) {
                    // @ts-ignore
                    const cfg = await window.go.main.App.GetSettings();
                    if (cfg && cfg.completion_delay !== undefined) {
                        setCompletionDelay(cfg.completion_delay);
                    }
                    if (cfg && cfg.terminal) {
                        const normalized = normalizeTerminalConfig(cfg.terminal);
                        terminalConfigStateRef.current = normalized;
                        setTerminalConfig(normalized);
                    }
                    if (cfg && cfg.appearance) {
                        // 后端权威值校正 localStorage（防止漂移/首访/跨设备）
                        const nextTheme = normalizeTheme(cfg.appearance.theme);
                        setTheme(nextTheme);
                    }
                    if (cfg && Array.isArray(cfg.highlight_rules)) {
                        setHighlightRules(cfg.highlight_rules);
                    }
                }
            } catch (e) {
                console.error('Failed to load settings:', e);
            }
        };
        loadSettings();

        // Check for updates on startup and then once per day at 08:00 local time.
        let updateCheckTimer: ReturnType<typeof setTimeout> | undefined;
        let disposed = false;

        const checkUpdate = async () => {
            try {
                // @ts-ignore
                const raw = await window.go?.main?.App?.CheckUpdate?.();
                if (disposed) return;
                if (raw) {
                    const result = typeof raw === 'string' ? JSON.parse(raw) : raw;
                    if (result.error) return;
                    setUpdateAvailable(!!result.hasUpdate);
                }
            } catch { /* silent */ }
        };

        const scheduleDailyUpdateCheck = () => {
            updateCheckTimer = setTimeout(async () => {
                await checkUpdate();
                if (!disposed) {
                    scheduleDailyUpdateCheck();
                }
            }, getDelayUntilNextDailyUpdateCheck());
        };

        checkUpdate();
        scheduleDailyUpdateCheck();

        return () => {
            disposed = true;
            if (updateCheckTimer) {
                clearTimeout(updateCheckTimer);
            }
        };
    }, []);

    useEffect(() => {
        const isEditableTarget = (target: EventTarget | null) => {
            const el = target as HTMLElement | null;
            if (!el) return false;
            if (el.classList?.contains('xterm-helper-textarea')) return false;
            if (el.closest?.('.xterm')) return false;
            const tag = el.tagName?.toLowerCase();
            if (tag === 'input' || tag === 'textarea' || tag === 'select') return true;
            if ((el as any).isContentEditable) return true;
            return false;
        };

        const eventToShortcut = (e: KeyboardEvent) => {
            const parts: string[] = [];
            if (e.ctrlKey) parts.push('Ctrl');
            if (e.altKey) parts.push('Alt');
            if (e.shiftKey) parts.push('Shift');
            if (e.metaKey) parts.push('Meta');

            if (e.key === 'Control' || e.key === 'Alt' || e.key === 'Shift' || e.key === 'Meta') return '';
            const mainKey = e.key.length === 1 ? e.key.toUpperCase() : e.key;
            parts.push(mainKey);
            return parts.join('+');
        };

        const matchesShortcut = (e: KeyboardEvent, shortcut: string) => {
            const normalized = (shortcut || '').trim();
            if (!normalized) return false;
            return eventToShortcut(e).toLowerCase() === normalized.toLowerCase();
        };

        const openCommandQuery = () => {
            if (!activeTerminalId) {
                toast.warning("请先选择一个激活的终端");
                return;
            }
            setCommandQueryText('');
            setCommandQueryError('');
            setCommandQueryResult(null);
            setIsCommandQueryOpen(true);
        };

        const handleKeyDown = (e: KeyboardEvent) => {
            if (isEditableTarget(e.target)) return;
            if (matchesShortcut(e, commandQueryShortcut)) {
                e.preventDefault();
                e.stopPropagation();
                openCommandQuery();
            }
        };

        window.addEventListener('keydown', handleKeyDown, true);
        return () => window.removeEventListener('keydown', handleKeyDown, true);
    }, [activeTerminalId]);

    const generateCommand = async (overrideText?: string) => {
        const text = (overrideText ?? commandQueryText).trim();
        if (!text) return;
        setCommandQueryText(overrideText ?? commandQueryText);
        setCommandQueryLoading(true);
        setCommandQueryError('');
        try {
            // @ts-ignore
            if (!window.go || !window.go.main || !window.go.main.App || !window.go.main.App.GenerateLinuxCommand) {
                setCommandQueryError('Wails 运行时未就绪');
                return;
            }
            // @ts-ignore
            const resp = await window.go.main.App.GenerateLinuxCommand(text);
            if (typeof resp === 'string' && resp.startsWith('Error:')) {
                setCommandQueryError(resp);
                setCommandQueryResult(null);
                return;
            }
            const parsed = JSON.parse(resp) as CommandQueryResult;
            setCommandQueryResult(parsed);
        } catch (e: any) {
            setCommandQueryError(e?.toString?.() || '生成失败');
            setCommandQueryResult(null);
        } finally {
            setCommandQueryLoading(false);
        }
    };

    const copyGeneratedCommand = () => {
        const cmd = commandQueryResult?.command?.trim();
        if (!cmd) return;
        navigator.clipboard.writeText(cmd);
    };

    const typeGeneratedCommand = () => {
        const cmd = commandQueryResult?.command?.trim();
        if (!cmd) return;
        setIsCommandQueryOpen(false);
        handleQuickCommand(cmd);
    };

    const removeTerminal = useCallback((id: string) => {
        setTerminals(prev => prev.filter(t => t.id !== id));
        setBroadcastIds(prev => prev.filter(bid => bid !== id));
        // Remove listener
        if (unlisteners.current.has(id)) {
            unlisteners.current.get(id)?.();
            unlisteners.current.delete(id);
        }
        terminalRefs.current.delete(id);
    }, []);

    const handleConnect = async (config: any) => {
        setStatus("正在连接...");
        try {
            // @ts-ignore
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.Connect) {
                // @ts-ignore
                const result = await window.go.main.App.Connect(config);

                if (result.success) {
                    setStatus("已连接");
                    const newSessionId = result.sessionId;
                    const newTerminal: TerminalSession = {
                        id: newSessionId,
                        title: config.name || `${config.user}@${config.host}`,
                        status: SessionStatus.CONNECTED,
                        config: config,  // 保存配置用于重连
                    };

                    setTerminals(prev => [...prev, newTerminal]);

                    // Listen for data for this specific session
                    // @ts-ignore
                    const cancel = window.runtime.EventsOn(`terminal-data:${newSessionId}`, (data: string) => {
                        terminalRefs.current.get(newSessionId)?.write(data);
                    });
                    unlisteners.current.set(newSessionId, cancel);

                } else {
                    setStatus("就绪");
                    const connectLabel = config?.name || (config?.user && config?.host ? `${config.user}@${config.host}` : (config?.host || '未知目标'));
                    enqueueConnectError(`连接失败：${connectLabel}`, result.message || '未知错误', {
                        reopenNewConnect: true,
                        failedConfigs: [config as ConnectionConfig],
                    });
                }
            } else {
                setStatus("就绪");
                enqueueConnectError("连接失败：运行时未就绪", "Wails 运行时未就绪", {
                    reopenNewConnect: true,
                    failedConfigs: [config as ConnectionConfig],
                });
            }
        } catch (e) {
            const errMsg = (e as any)?.message ? String((e as any).message) : String(e);
            setStatus("就绪");
            const connectLabel = config?.name || (config?.user && config?.host ? `${config.user}@${config.host}` : (config?.host || '未知目标'));
            enqueueConnectError(`连接失败：${connectLabel}`, errMsg || '未知错误', {
                reopenNewConnect: true,
                failedConfigs: [config as ConnectionConfig],
            });
        }
    };

    const handleReconnect = async (sessionId: string) => {
        const term = terminals.find(t => t.id === sessionId);
        if (!term || !term.config) {
            enqueueConnectError("重连失败", "会话配置不存在");
            return;
        }

        setStatus("正在重连...");
        try {
            // @ts-ignore
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.ReconnectSession) {
                // @ts-ignore
                const result = await window.go.main.App.ReconnectSession(sessionId);

                if (result.success) {
                    setStatus("已重连");
                    // 更新状态为已连接
                    setTerminals(prev => prev.map(t =>
                        t.id === sessionId
                            ? { ...t, status: SessionStatus.CONNECTED }
                            : t
                    ));

                    // 移除旧的监听器，避免重复接收数据
                    const oldCancel = unlisteners.current.get(sessionId);
                    if (oldCancel) {
                        oldCancel();
                    }

                    // 重新监听终端数据（使用原sessionId）
                    // @ts-ignore
                    const cancel = window.runtime.EventsOn(`terminal-data:${sessionId}`, (data: string) => {
                        terminalRefs.current.get(sessionId)?.write(data);
                    });
                    unlisteners.current.set(sessionId, cancel);
                } else {
                    setStatus("重连失败");
                    enqueueConnectError("重连失败", result.message || '未知错误');
                }
            } else {
                setStatus("重连失败");
                enqueueConnectError("重连失败", "运行时未就绪");
            }
        } catch (e) {
            setStatus("重连失败");
            const errMsg = (e as any)?.message ? String((e as any).message) : String(e);
            enqueueConnectError("重连失败", errMsg || '未知错误');
        }
    };

    const handleBatchConnect = (configs: ConnectionConfig[]) => {
        configs.forEach(config => handleConnect(config));
    };

    const handleParseIntent = async (input: string): Promise<ConnectionConfig[]> => {
        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.ParseIntent) {
            // @ts-ignore
            return await window.go.main.App.ParseIntent(input);
        }
        throw new Error("Wails 运行时未就绪");
    };

    const handleTerminalData = useCallback((id: string, data: string) => {
        // Use Refs to get latest state inside callback closure
        const currentBroadcastMode = isBroadcastModeRef.current;
        const currentBroadcastIds = broadcastIdsRef.current;

        // If broadcast mode is on AND current terminal is in broadcast group
        if (currentBroadcastMode && currentBroadcastIds.includes(id)) {
            // @ts-ignore
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.Broadcast) {
                // Ensure broadcastIds is an array of strings
                const targetIds = Array.from(currentBroadcastIds);

                // @ts-ignore
                window.go.main.App.Broadcast(targetIds, data);
            }
        } else {
            // Standard single terminal write
            // @ts-ignore
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.Write) {
                // @ts-ignore
                window.go.main.App.Write(id, data);
            }
        }
    }, []);

    const handleToggleBroadcast = (enabled: boolean) => {
        setIsBroadcastMode(enabled);
        if (enabled) {
            // Add all current terminals to broadcast group
            const allIds = terminals.map(t => t.id);
            setBroadcastIds(allIds);
        } else {
            // Clear broadcast group
            setBroadcastIds([]);
        }
    };

    // 从指定标签开启广播:广播组初始仅含该标签(区别于 handleToggleBroadcast 的"全选")
    // 用于标签右键菜单「开启广播(仅本标签)」入口。
    const handleStartBroadcastFrom = (id: string) => {
        setIsBroadcastMode(true);
        setBroadcastIds([id]);
    };

    const handleToggleTerminalBroadcast = (id: string) => {
        if (!isBroadcastMode) return;

        setBroadcastIds(prev => {
            if (prev.includes(id)) {
                const next = prev.filter(bid => bid !== id);
                // 退出最后一个标签后,广播组为空 → 自动关闭广播模式,
                // 避免"广播开着但组是空的"这种无意义状态。
                if (next.length === 0) {
                    setIsBroadcastMode(false);
                }
                return next;
            } else {
                return [...prev, id];
            }
        });
    };

    const handleQuickCommand = (command: string) => {
        const targetTerminalId = activeTerminalId;
        if (!targetTerminalId) {
            toast.warning("请先选择一个激活的终端");
            return;
        }

        const targetTerminal = terminalRefs.current.get(targetTerminalId);
        if (!targetTerminal) {
            toast.warning("请先选择一个激活的终端");
            return;
        }

        targetTerminal.prepareForExternalInput();

        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.Write) {
            const payload = command.replace(/[\r\n]+$/g, '');
            // @ts-ignore
            window.go.main.App.Write(targetTerminalId, payload);
        }
        setTimeout(() => {
            terminalRefs.current.get(targetTerminalId)?.focus();
        }, 0);
    };

    const handleOpenKnowledgeSource = useCallback((target: Omit<KnowledgeTarget, 'requestId'>) => {
        setKnowledgeTarget({ ...target, requestId: Date.now() });
        setSidebarTab('knowledge');
        setIsSidebarOpen(true);
    }, []);

    const handleCloseTerminal = useCallback((id: string) => {
        // Close session in backend
        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.CloseSession) {
            // @ts-ignore
            window.go.main.App.CloseSession(id);
        }
        // Remove from UI
        removeTerminal(id);
    }, [removeTerminal]);

    const handleRenameTerminal = useCallback((id: string, newTitle: string) => {
        setTerminals(prev => prev.map(t =>
            t.id === id ? { ...t, title: newTitle } : t
        ));
    }, []);

    const handleDuplicateTerminal = (id: string) => {
        const term = terminals.find(t => t.id === id);
        if (!term) return;

        // Note: We can't actually clone the SSH session easily without re-authenticating.
        // But for now, we can prompt the user or just reuse the config if we had it stored.
        // Since we don't store the full config in TerminalSession, we might need to fetch it from backend or SessionManager.
        // However, looking at handleConnect, we only store id and title.

        // A better approach for "Duplicate" in this context might be:
        // 1. Get the session details from backend (if possible) or
        // 2. Just create a new UI tab that *points* to the same session? No, that's weird.
        // 3. We actually need to reconnect.

        // Given the constraints and current architecture, "Duplicate" implies starting a NEW session with SAME config.
        // But we don't have the config here.

        // WORKAROUND: Ask backend to duplicate session?
        // Or better: Let's assume the user wants to clone the *view* for now, or we just alert "Not implemented" if we can't reconnect.

        // Wait, if we use `window.go.main.App.GetSessionConfig(id)`, we could get it.
        // Let's assume we can implement a backend method `DuplicateSession(id)` which returns a new session ID.

        // For this task, I'll implement the UI wiring. The actual backend duplication might be complex.
        // Let's try to find if we can get the config.

        // Actually, checking SessionManager.tsx, we have `GetSavedSessions`.
        // If this was a saved session, we could find it. If it was an ad-hoc connection, we might not have it.

        // Let's try to call a backend method. If not exists, we'll alert.
        // But wait, the user just asked for the UI feature.
        // "给tab页的标签加一个右键菜单，支持重命名和复制一个标签的功能"

        // I will implement the handler in App.tsx that calls backend to Duplicate.
        // I'll add `DuplicateSession` to backend later if needed, or mock it for now.

        // Let's try to add the method to backend first? Or just implement UI flow.
        // Since I'm in "App.tsx", I'll add the call.

        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.DuplicateSession) {
            // @ts-ignore
            window.go.main.App.DuplicateSession(id).then(result => {
                if (result.success) {
                    const newTerminal: TerminalSession = {
                        id: result.sessionId,
                        title: `${term.title} (Copy)`,
                        status: SessionStatus.CONNECTED,
                        config: term.config
                    };
                    setTerminals(prev => [...prev, newTerminal]);

                    // Listen
                    // @ts-ignore
                    const cancel = window.runtime.EventsOn(`terminal-data:${result.sessionId}`, (data: string) => {
                        terminalRefs.current.get(result.sessionId)?.write(data);
                    });
                    unlisteners.current.set(result.sessionId, cancel);
                } else {
                    toast.error("复制失败: " + result.message);
                }
            });
        } else {
            toast.error("后端不支持复制会话 (DuplicateSession not implemented)");
        }
    };

    // Force layout update when sidebar toggles
    useEffect(() => {
        scheduleFitAll(300);
    }, [isSidebarOpen, scheduleFitAll]);

    const [isQuickCommandOpen, setIsQuickCommandOpen] = useState(false);

    // Force terminal resize when QuickCommandPanel toggles
    useEffect(() => {
        scheduleFitAll(350);
    }, [isQuickCommandOpen, scheduleFitAll]);

    const toggleSidebar = (tab: 'sessions' | 'troubleshoot' | 'chat' | 'script' | 'knowledge') => {
        if (isSidebarOpen && sidebarTab === tab) {
            // If clicking the active tab, close it
            setIsSidebarOpen(false);
        } else {
            // Open and switch tab
            setIsSidebarOpen(true);
            setSidebarTab(tab);
        }
    };

    const handleConfirmClose = () => {
        console.debug("[App] User confirmed close");
        setIsConfirmCloseOpen(false);
        // Call backend to force quit
        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.ForceQuit) {
            // @ts-ignore
            window.go.main.App.ForceQuit();
        }
    };

    const handleCancelClose = () => {
        console.debug("[App] User cancelled close");
        setIsConfirmCloseOpen(false);
    };

    return (
        <div id="app" style={{ height: '100vh', display: 'flex', flexDirection: 'column' }}>
            <div style={{
                padding: '6px 12px',
                background: 'var(--bg-elevated)',
                borderBottom: '1px solid var(--bg-primary)',
                color: 'var(--text-primary)',
                display: 'flex',
                gap: '12px',
                alignItems: 'center',
                justifyContent: 'space-between'
            }}>
                <div style={{ display: 'flex', gap: '16px', alignItems: 'center' }}>
                    {status === '就绪' || status === '已连接' || status === '已重连' ? (
                        <img src={logo} alt="OpsCopilot" style={{ width: 28, height: 28 }} />
                    ) : null}
                    {status !== '就绪' && status !== '已连接' && status !== '已重连' && (
                        <div style={{
                            ...styles.loadingIndicator,
                            color: (status.includes('失败') || status.includes('请先')) ? 'var(--danger)' : 'var(--text-muted)',
                        }}>
                            {!status.includes('失败') && !status.includes('请先') && (
                                <svg className="spin" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
                                    <path d="M12 2a10 10 0 0 1 10 10" />
                                </svg>
                            )}
                            <span>{status}</span>
                        </div>
                    )}
                    <button onClick={() => setIsSmartModalOpen(true)} style={styles.primaryBtn}>
                        + 新建连接
                    </button>
                    <button onClick={handleThemeToggle} style={styles.iconBtnUnified} title={theme === 'dark' ? '切换到亮色' : '切换到暗色'}>
                        {theme === 'dark' ? (
                            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                                <circle cx="12" cy="12" r="4" />
                                <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
                            </svg>
                        ) : (
                            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                                <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
                            </svg>
                        )}
                    </button>
                    <button onClick={() => setIsSettingsOpen(true)} style={{ ...styles.iconBtnUnified, position: 'relative' }} title="设置">
                        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                            <circle cx="12" cy="12" r="3"></circle>
                            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
                        </svg>
                        {(updateAvailable || highlightNeedsAttention) && (
                            <span style={{
                                position: 'absolute',
                                top: '2px',
                                right: '2px',
                                width: '8px',
                                height: '8px',
                                borderRadius: '50%',
                                backgroundColor: ATTENTION_DOT_COLOR,
                                border: '1px solid var(--bg-primary)',
                            }} />
                        )}
                    </button>
                    {parsedTimestamp && (
                        <div style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: '4px',
                            padding: '2px 8px',
                            background: 'var(--bg-input)',
                            borderRadius: '4px',
                            fontSize: '0.75rem',
                        }}>
                            <span style={{ color: 'var(--text-muted)' }}>{TbClock({ size: 12 })}</span>
                            <span style={{ color: 'var(--text-secondary)' }}>{parsedTimestamp.local}</span>
                        </div>
                    )}
                </div>
            </div>

            <div style={{ flex: 1, position: 'relative', overflow: 'hidden', display: 'flex', flexDirection: 'row' }}>
                <div style={{ flex: 1, display: 'flex', flexDirection: 'column', position: 'relative', overflow: 'hidden' }}>
                    <div style={{ flex: 1, position: 'relative', overflow: 'hidden' }}>
                        <FlexLayoutAdapter
                            terminals={terminals}
                            onTerminalData={handleTerminalData}
                            terminalRefs={terminalRefs}
                            onCloseTerminal={handleCloseTerminal}
                            onRenameTerminal={handleRenameTerminal}
                            onDuplicateTerminal={handleDuplicateTerminal}
                            activeTerminalId={activeTerminalId}
                            onActiveTerminalChange={setActiveTerminalId}
                            onReconnect={handleReconnect}
                            isBroadcastMode={isBroadcastMode}
                            broadcastIds={broadcastIds}
                            onToggleTerminalBroadcast={handleToggleTerminalBroadcast}
                            onStartBroadcastFrom={handleStartBroadcastFrom}
                            completionDelay={completionDelay}
                            terminalConfig={terminalConfig}
                            onTerminalFontSizeChange={handleTerminalFontSizeChange}
                            highlightRules={highlightRules}
                            theme={theme}
                            onSelectionParsed={setParsedTimestamp}
                        />
                    </div>

                    <QuickCommandPanel
                        isOpen={isQuickCommandOpen}
                        onExecute={handleQuickCommand}
                    />
                </div>

                <Sidebar
                    isOpen={isSidebarOpen}
                    activeTab={sidebarTab}
                    onToggle={() => setIsSidebarOpen(!isSidebarOpen)}
                    onConnect={(config) => handleBatchConnect([config])}
                    activeTerminalId={activeTerminalId}
                    terminals={terminals}
                    onTypeCommand={handleQuickCommand}
                    onOpenKnowledgeSource={handleOpenKnowledgeSource}
                    knowledgeTarget={knowledgeTarget}
                />

                {/* Right Nav (Icon Bar) */}
                <div style={styles.rightNav}>
                    <div
                        style={{
                            ...styles.navIcon,
                            backgroundColor: (isSidebarOpen && sidebarTab === 'sessions') ? 'var(--bg-elevated)' : 'transparent',
                            borderRight: (isSidebarOpen && sidebarTab === 'sessions') ? '2px solid var(--accent)' : '2px solid transparent'
                        }}
                        onClick={() => toggleSidebar('sessions')}
                        title="会话管理"
                    >
                        {TbScreenShare({ size: 20 })}
                    </div>
                    <div
                        style={{
                            ...styles.navIcon,
                            backgroundColor: (isSidebarOpen && sidebarTab === 'troubleshoot') ? 'var(--bg-elevated)' : 'transparent',
                            borderRight: (isSidebarOpen && sidebarTab === 'troubleshoot') ? '2px solid var(--accent)' : '2px solid transparent'
                        }}
                        onClick={() => toggleSidebar('troubleshoot')}
                        title="定位助手"
                    >
                        {TbStethoscope({ size: 20 })}
                    </div>
                    <div
                        style={{
                            ...styles.navIcon,
                            backgroundColor: (isSidebarOpen && sidebarTab === 'chat') ? 'var(--bg-elevated)' : 'transparent',
                            borderRight: (isSidebarOpen && sidebarTab === 'chat') ? '2px solid var(--accent)' : '2px solid transparent'
                        }}
                        onClick={() => toggleSidebar('chat')}
                        title="AI 问答"
                    >
                        {TbMessageChatbot({ size: 20 })}
                    </div>
                    <div
                        style={{
                            ...styles.navIcon,
                            backgroundColor: (isSidebarOpen && sidebarTab === 'knowledge') ? 'var(--bg-elevated)' : 'transparent',
                            borderRight: (isSidebarOpen && sidebarTab === 'knowledge') ? '2px solid var(--accent)' : '2px solid transparent'
                        }}
                        onClick={() => toggleSidebar('knowledge')}
                        title="知识库"
                    >
                        {TbBook({ size: 20 })}
                    </div>
                    <div
                        style={{
                            ...styles.navIcon,
                            backgroundColor: (isSidebarOpen && sidebarTab === 'script') ? 'var(--bg-elevated)' : 'transparent',
                            borderRight: (isSidebarOpen && sidebarTab === 'script') ? '2px solid var(--accent)' : '2px solid transparent'
                        }}
                        onClick={() => toggleSidebar('script')}
                        title="脚本录制"
                    >
                        {TbCode({ size: 20 })}
                    </div>
                    <div style={{ flex: 1 }} />
                    <div
                        style={{
                            ...styles.navIcon,
                            backgroundColor: isQuickCommandOpen ? 'var(--bg-elevated)' : 'transparent',
                            borderRight: isQuickCommandOpen ? '2px solid var(--accent)' : '2px solid transparent',
                        }}
                        onClick={() => setIsQuickCommandOpen(!isQuickCommandOpen)}
                        title="快捷命令"
                        data-testid="nav-icon-quickcommands"
                    >
                        {TbBolt({ size: 20 })}
                    </div>
                </div>
            </div>

            <BottomBar />

            <SmartConnectModal
                isOpen={isSmartModalOpen}
                onClose={() => {
                    setIsSmartModalOpen(false);
                    setReconnectSeedConfigs([]);
                }}
                onConnect={handleBatchConnect}
                onParse={handleParseIntent}
                initialConfigs={reconnectSeedConfigs}
            />

            <SettingsModal
                isOpen={isSettingsOpen}
                onClose={() => setIsSettingsOpen(false)}
                isBroadcastMode={isBroadcastMode}
                onToggleBroadcast={handleToggleBroadcast}
                onCompletionDelayChange={setCompletionDelay}
                onHighlightRulesChange={setHighlightRules}
                onTerminalConfigChange={handleTerminalConfigChange}
                theme={theme}
                onThemeChange={handleThemeChange}
                updateAvailable={updateAvailable}
            />

            <ConfirmCloseModal
                isOpen={isConfirmCloseOpen}
                message={confirmCloseMessage}
                onConfirm={handleConfirmClose}
                onCancel={handleCancelClose}
            />

            <ConnectErrorModal
                isOpen={!!activeConnectError}
                title={activeConnectError?.title || ''}
                message={activeConnectError?.message || ''}
                onClose={dismissConnectError}
            />

            <CommandQueryOverlay
                visible={isCommandQueryOpen}
                query={commandQueryText}
                loading={commandQueryLoading}
                result={commandQueryResult}
                error={commandQueryError}
                onQueryChange={setCommandQueryText}
                onGenerate={generateCommand}
                onRegenerate={generateCommand}
                onCopy={copyGeneratedCommand}
                onType={typeGeneratedCommand}
                onClose={() => setIsCommandQueryOpen(false)}
                onSelectHistory={(entry) => {
                    setIsCommandQueryOpen(false);
                    handleQuickCommand(entry.command);
                }}
            />
        </div>
    );
}

const styles = {
    primaryBtn: {
        height: '28px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        gap: '4px',
        padding: '0 12px',
        backgroundColor: 'var(--accent)',
        border: 'none',
        borderRadius: '4px',
        color: 'var(--text-on-accent)',
        cursor: 'pointer',
        fontSize: '0.82rem',
        fontWeight: 500 as const,
        transition: 'background-color 0.15s',
    },
    iconBtnUnified: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        backgroundColor: 'transparent',
        border: 'none',
        borderRadius: '4px',
        color: 'var(--text-tertiary)',
        cursor: 'pointer',
        padding: '4px',
        transition: 'color 0.15s',
    },
    loadingIndicator: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        fontSize: '0.8rem',
    },
    rightNav: {
        width: '40px',
        backgroundColor: 'var(--bg-secondary)',
        display: 'flex',
        flexDirection: 'column' as const,
        alignItems: 'center',
        borderLeft: '1px solid var(--border)',
        paddingTop: '10px',
        paddingBottom: '10px',
    },
    navIcon: {
        width: '100%',
        height: '42px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        cursor: 'pointer',
        fontSize: '24px',
        marginBottom: '4px',
        transition: 'background-color 0.2s',
    }
};

export default App;
