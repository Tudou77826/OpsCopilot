import { useState, useRef, useEffect } from 'react';
import './App.css';
import { TerminalRef } from './components/Terminal/Terminal';
import LayoutManager from './components/LayoutManager/LayoutManager';
import QuickCommandDrawer from './components/QuickCommandDrawer/QuickCommandDrawer';
import SmartConnectModal from './components/SmartConnectModal/SmartConnectModal';
import Sidebar from './components/Sidebar/Sidebar';
import SettingsModal from './components/SettingsModal/SettingsModal';
import ConfirmCloseModal from './components/ConfirmCloseModal/ConfirmCloseModal';
import { ConnectionConfig } from './types';

interface TerminalSession {
    id: string;
    title: string;
}

function App() {
    const [status, setStatus] = useState("就绪");
    const [isSmartModalOpen, setIsSmartModalOpen] = useState(false);
    const [isSettingsOpen, setIsSettingsOpen] = useState(false);
    const [isSidebarOpen, setIsSidebarOpen] = useState(false);
    const [sidebarTab, setSidebarTab] = useState<'sessions' | 'troubleshoot' | 'chat'>('sessions');
    const [terminals, setTerminals] = useState<TerminalSession[]>([]);
    const [layoutMode, setLayoutMode] = useState<'tab' | 'grid'>('tab');
    const [activeTerminalId, setActiveTerminalId] = useState<string | null>(null);
    const [isBroadcastMode, setIsBroadcastMode] = useState(false);
    const [broadcastIds, setBroadcastIds] = useState<string[]>([]);
    const [isConfirmCloseOpen, setIsConfirmCloseOpen] = useState(false);
    const [confirmCloseMessage, setConfirmCloseMessage] = useState("");
    const [completionDelay, setCompletionDelay] = useState(150);

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
    // Store unlisten functions for events
    const unlisteners = useRef(new Map<string, () => void>());

    useEffect(() => {
        // Listen for session closed events from backend
        let cancelClose: (() => void) | undefined;
        let cancelConfirmClose: (() => void) | undefined;

        // @ts-ignore
        if (window.runtime && window.runtime.EventsOn) {
            // @ts-ignore
            cancelClose = window.runtime.EventsOn("session-closed", (id: string) => {
                removeTerminal(id);
            });

            // Listen for confirm-close event from backend
            // @ts-ignore
            cancelConfirmClose = window.runtime.EventsOn("confirm-close", (data: any) => {
                console.log("[App] Received confirm-close event:", data);
                setConfirmCloseMessage(data.message || "确定要关闭应用吗？");
                setIsConfirmCloseOpen(true);
            });
        }
        return () => {
            if (cancelClose) cancelClose();
            if (cancelConfirmClose) cancelConfirmClose();
            // Cleanup all terminal listeners
            unlisteners.current.forEach(u => u());
            unlisteners.current.clear();
        };
    }, []);

    // Load completion delay from settings on mount
    useEffect(() => {
        const loadCompletionDelay = async () => {
            try {
                // @ts-ignore
                if (window.go && window.go.main && window.go.main.App && window.go.main.App.GetSettings) {
                    // @ts-ignore
                    const cfg = await window.go.main.App.GetSettings();
                    if (cfg && cfg.completion_delay !== undefined) {
                        setCompletionDelay(cfg.completion_delay);
                    }
                }
            } catch (e) {
                console.error('Failed to load completion delay:', e);
            }
        };
        loadCompletionDelay();
    }, []);

    const removeTerminal = (id: string) => {
        setTerminals(prev => prev.filter(t => t.id !== id));
        setBroadcastIds(prev => prev.filter(bid => bid !== id));
        // Remove listener
        if (unlisteners.current.has(id)) {
            unlisteners.current.get(id)?.();
            unlisteners.current.delete(id);
        }
        terminalRefs.current.delete(id);
    };

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
                        title: config.name || `${config.user}@${config.host}`
                    };

                    setTerminals(prev => [...prev, newTerminal]);

                    // Listen for data for this specific session
                    // @ts-ignore
                    const cancel = window.runtime.EventsOn(`terminal-data:${newSessionId}`, (data: string) => {
                        terminalRefs.current.get(newSessionId)?.write(data);
                    });
                    unlisteners.current.set(newSessionId, cancel);

                } else {
                    setStatus("错误: " + result.message);
                }
            } else {
                setStatus("Wails 运行时未就绪");
            }
        } catch (e) {
            setStatus("错误: " + e);
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

    const handleTerminalData = (id: string, data: string) => {
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
    };

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

    const handleToggleTerminalBroadcast = (id: string) => {
        if (!isBroadcastMode) return;

        setBroadcastIds(prev => {
            if (prev.includes(id)) {
                return prev.filter(bid => bid !== id);
            } else {
                return [...prev, id];
            }
        });
    };

    const handleQuickCommand = (command: string) => {
        if (!activeTerminalId) {
            alert("请先选择一个激活的终端");
            return;
        }

        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.Write) {
            const payload = command.endsWith('\n') ? command : command + '\n';
            // @ts-ignore
            window.go.main.App.Write(activeTerminalId, payload);
        }
    };

    const handleCloseTerminal = (id: string) => {
        // Close session in backend
        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.CloseSession) {
            // @ts-ignore
            window.go.main.App.CloseSession(id);
        }
        // Remove from UI
        removeTerminal(id);
    };

    const handleRenameTerminal = (id: string, newTitle: string) => {
        setTerminals(prev => prev.map(t =>
            t.id === id ? { ...t, title: newTitle } : t
        ));
    };

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
                        title: `${term.title} (Copy)`
                    };
                    setTerminals(prev => [...prev, newTerminal]);

                    // Listen
                    // @ts-ignore
                    const cancel = window.runtime.EventsOn(`terminal-data:${result.sessionId}`, (data: string) => {
                        terminalRefs.current.get(result.sessionId)?.write(data);
                    });
                    unlisteners.current.set(result.sessionId, cancel);
                } else {
                    alert("复制失败: " + result.message);
                }
            });
        } else {
            alert("后端不支持复制会话 (DuplicateSession not implemented)");
        }
    };

    // Force layout update when sidebar toggles
    useEffect(() => {
        setTimeout(() => {
            terminalRefs.current.forEach(t => t.fit());
        }, 300); // Wait for transition
    }, [isSidebarOpen]);

    const [isQuickCommandDrawerOpen, setIsQuickCommandDrawerOpen] = useState(false);

    // Force terminal resize when QuickCommandDrawer toggles
    useEffect(() => {
        setTimeout(() => {
            terminalRefs.current.forEach(t => t.fit());
        }, 350); // Wait for transition (300ms)
    }, [isQuickCommandDrawerOpen]);

    const toggleSidebar = (tab: 'sessions' | 'troubleshoot' | 'chat') => {
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
        console.log("[App] User confirmed close");
        setIsConfirmCloseOpen(false);
        // Call backend to force quit
        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.ForceQuit) {
            // @ts-ignore
            window.go.main.App.ForceQuit();
        }
    };

    const handleCancelClose = () => {
        console.log("[App] User cancelled close");
        setIsConfirmCloseOpen(false);
    };

    return (
        <div id="app" style={{ height: '100vh', display: 'flex', flexDirection: 'column' }}>
            <div style={{
                padding: '8px 16px',
                background: '#333',
                color: '#fff',
                display: 'flex',
                gap: '16px',
                alignItems: 'center',
                justifyContent: 'space-between'
            }}>
                <div style={{ display: 'flex', gap: '16px', alignItems: 'center' }}>
                    <span style={{ fontSize: '0.9rem', color: status === '已连接' ? '#4caf50' : '#aaa' }}>
                        {status}
                    </span>
                    <button onClick={() => setIsSmartModalOpen(true)} style={styles.primaryBtn}>
                        + 新建连接
                    </button>
                    <button onClick={() => setIsSettingsOpen(true)} style={styles.iconBtn} title="设置">
                        ⚙️
                    </button>
                </div>

                <div style={styles.toggleGroup}>
                    <button
                        style={layoutMode === 'tab' ? styles.activeToggle : styles.toggle}
                        onClick={() => setLayoutMode('tab')}
                    >
                        标签模式
                    </button>
                    <button
                        style={layoutMode === 'grid' ? styles.activeToggle : styles.toggle}
                        onClick={() => setLayoutMode('grid')}
                    >
                        网格模式
                    </button>
                </div>
            </div>

            <div style={{ flex: 1, position: 'relative', overflow: 'hidden', display: 'flex', flexDirection: 'row' }}>
                <div style={{ flex: 1, position: 'relative', overflow: 'hidden' }}>
                    <LayoutManager
                        terminals={terminals}
                        mode={layoutMode}
                        onTerminalData={handleTerminalData}
                        terminalRefs={terminalRefs}
                        onCloseTerminal={handleCloseTerminal}
                        onRenameTerminal={handleRenameTerminal}
                        onDuplicateTerminal={handleDuplicateTerminal}
                        onActiveTerminalChange={setActiveTerminalId}
                        onClose={() => { }}
                        isBroadcastMode={isBroadcastMode}
                        broadcastIds={broadcastIds}
                        onToggleTerminalBroadcast={handleToggleTerminalBroadcast}
                        completionDelay={completionDelay}
                    />
                </div>

                <Sidebar
                    isOpen={isSidebarOpen}
                    activeTab={sidebarTab}
                    onToggle={() => setIsSidebarOpen(!isSidebarOpen)}
                    onConnect={(config) => handleBatchConnect([config])}
                />

                {/* Right Nav (Icon Bar) */}
                <div style={styles.rightNav}>
                    <div
                        style={{
                            ...styles.navIcon,
                            backgroundColor: (isSidebarOpen && sidebarTab === 'sessions') ? '#333' : 'transparent',
                            borderRight: (isSidebarOpen && sidebarTab === 'sessions') ? '2px solid #007acc' : '2px solid transparent'
                        }}
                        onClick={() => toggleSidebar('sessions')}
                        title="会话管理"
                    >
                        🖥️
                    </div>
                    <div
                        style={{
                            ...styles.navIcon,
                            backgroundColor: (isSidebarOpen && sidebarTab === 'troubleshoot') ? '#333' : 'transparent',
                            borderRight: (isSidebarOpen && sidebarTab === 'troubleshoot') ? '2px solid #007acc' : '2px solid transparent'
                        }}
                        onClick={() => toggleSidebar('troubleshoot')}
                        title="定位助手"
                    >
                        🩺
                    </div>
                    <div
                        style={{
                            ...styles.navIcon,
                            backgroundColor: (isSidebarOpen && sidebarTab === 'chat') ? '#333' : 'transparent',
                            borderRight: (isSidebarOpen && sidebarTab === 'chat') ? '2px solid #007acc' : '2px solid transparent'
                        }}
                        onClick={() => toggleSidebar('chat')}
                        title="AI 问答"
                    >
                        💬
                    </div>
                </div>
            </div>

            <QuickCommandDrawer
                onExecute={handleQuickCommand}
                isOpen={isQuickCommandDrawerOpen}
                onToggle={() => setIsQuickCommandDrawerOpen(!isQuickCommandDrawerOpen)}
            />

            <SmartConnectModal
                isOpen={isSmartModalOpen}
                onClose={() => setIsSmartModalOpen(false)}
                onConnect={handleBatchConnect}
                onParse={handleParseIntent}
            />

            <SettingsModal
                isOpen={isSettingsOpen}
                onClose={() => setIsSettingsOpen(false)}
                isBroadcastMode={isBroadcastMode}
                onToggleBroadcast={handleToggleBroadcast}
                onCompletionDelayChange={setCompletionDelay}
            />

            <ConfirmCloseModal
                isOpen={isConfirmCloseOpen}
                message={confirmCloseMessage}
                onConfirm={handleConfirmClose}
                onCancel={handleCancelClose}
            />
        </div>
    );
}

const styles = {
    primaryBtn: {
        padding: '6px 12px',
        backgroundColor: '#007acc',
        color: 'white',
        border: 'none',
        borderRadius: '4px',
        cursor: 'pointer',
        fontWeight: 'bold' as const,
    },
    toggleGroup: {
        display: 'flex',
        backgroundColor: '#1e1e1e',
        borderRadius: '4px',
        overflow: 'hidden',
        border: '1px solid #444',
    },
    toggle: {
        padding: '6px 12px',
        backgroundColor: 'transparent',
        color: '#ccc',
        border: 'none',
        cursor: 'pointer',
    },
    activeToggle: {
        padding: '6px 12px',
        backgroundColor: '#007acc',
        color: 'white',
        border: 'none',
        cursor: 'pointer',
    },
    iconBtn: {
        background: 'none',
        border: 'none',
        fontSize: '1.2rem',
        cursor: 'pointer',
        padding: '4px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        marginLeft: '8px',
    },
    rightNav: {
        width: '48px',
        backgroundColor: '#252526',
        display: 'flex',
        flexDirection: 'column' as const,
        alignItems: 'center',
        borderLeft: '1px solid #333',
        paddingTop: '10px',
    },
    navIcon: {
        width: '100%',
        height: '48px',
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
