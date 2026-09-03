import { useState, useRef, useEffect, useCallback, useMemo } from 'react';
import { TbClock, TbScreenShare, TbStethoscope, TbMessageChatbot, TbCode, TbBolt, TbBook } from 'react-icons/tb';
import { useToast } from './components/Toast/Toast';
import './App.css';
import { ProductFrame, ProductToolbar, ProductNavigation } from '../../frontend-shell/src/ui/product/ProductChrome';
import { TerminalRef } from './components/Terminal/Terminal';
import FlexLayoutAdapter from './components/FlexLayout/FlexLayoutAdapter';
import QuickCommandPanel from './components/QuickCommandPanel/QuickCommandPanel';
import BottomBar from './components/BottomBar/BottomBar';
import SmartConnectModal from './components/SmartConnectModal/SmartConnectModal';
import Sidebar from './components/Sidebar/Sidebar';
import SettingsModal from './components/SettingsModal/SettingsModal';
import ConfirmCloseModal from './components/ConfirmCloseModal/ConfirmCloseModal';
import CommandQueryOverlay from './components/CommandQueryOverlay/CommandQueryOverlay';
import { useCommandQuery } from '../../frontend-shell/src/ui/product/useCommandQuery';
import { useProductNavigation } from '../../frontend-shell/src/ui/product/useProductNavigation';
import { generateWailsCommand } from './shell-adapter/wailsCommandQuery';
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
    const { settingsOpen:isSettingsOpen, setSettingsOpen:setIsSettingsOpen, sidebarOpen:isSidebarOpen, setSidebarOpen:setIsSidebarOpen,
        tab:sidebarTab, setTab:setSidebarTab, quickOpen:isQuickCommandOpen, setQuickOpen:setIsQuickCommandOpen, toggleSidebar } = useProductNavigation({sidebarOpen:false,quickOpen:false});
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
    const commandQueryShortcut = 'Ctrl+K';
    const { visible:isCommandQueryOpen, setVisible:setIsCommandQueryOpen, query:commandQueryText, setQuery:setCommandQueryText,
        loading:commandQueryLoading, result:commandQueryResult, error:commandQueryError, generate:generateCommand,
        copy:copyGeneratedCommand, type:typeGeneratedCommand } = useCommandQuery({
        generate:generateWailsCommand, type:command => handleQuickCommand(command),
        copy:command => navigator.clipboard.writeText(command), warn:message => toast.warning(message),
    }, activeTerminalId, commandQueryShortcut);
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

    // Force terminal resize when QuickCommandPanel toggles
    useEffect(() => {
        scheduleFitAll(350);
    }, [isQuickCommandOpen, scheduleFitAll]);

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
        <ProductFrame id="app"
            toolbar={<ProductToolbar status={status} theme={theme} onNewConnection={() => setIsSmartModalOpen(true)} onThemeToggle={handleThemeToggle} onSettings={() => setIsSettingsOpen(true)} updateAvailable={updateAvailable} highlightNeedsAttention={highlightNeedsAttention} parsedTimestamp={parsedTimestamp} />}
            terminal={<FlexLayoutAdapter
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
                        />}
            quickCommands={<QuickCommandPanel
                        isOpen={isQuickCommandOpen}
                        onExecute={handleQuickCommand}
                    />}
            sidebar={<Sidebar
                    isOpen={isSidebarOpen}
                    activeTab={sidebarTab}
                    onToggle={() => setIsSidebarOpen(!isSidebarOpen)}
                    onConnect={(config) => handleBatchConnect([config])}
                    activeTerminalId={activeTerminalId}
                    terminals={terminals}
                    onTypeCommand={handleQuickCommand}
                    onOpenKnowledgeSource={handleOpenKnowledgeSource}
                    knowledgeTarget={knowledgeTarget}
                />}
            navigation={<ProductNavigation isSidebarOpen={isSidebarOpen} sidebarTab={sidebarTab} toggleSidebar={toggleSidebar} isQuickCommandOpen={isQuickCommandOpen} onToggleQuickCommands={() => setIsQuickCommandOpen(!isQuickCommandOpen)} />}
            footer={<BottomBar />}
        >

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
        </ProductFrame>
    );
}


export default App;
