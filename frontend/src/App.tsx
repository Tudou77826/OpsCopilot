import { useState, useRef, useEffect } from 'react';
import './App.css';
import { TerminalRef } from './components/Terminal/Terminal';
import LayoutManager from './components/LayoutManager/LayoutManager';
import BroadcastBar from './components/BroadcastBar/BroadcastBar';
import SmartConnectModal from './components/SmartConnectModal/SmartConnectModal';
import Sidebar from './components/Sidebar/Sidebar';
import SettingsModal from './components/SettingsModal/SettingsModal';
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
    const [sidebarTab, setSidebarTab] = useState<'sessions' | 'ai'>('sessions');
    const [terminals, setTerminals] = useState<TerminalSession[]>([]);
    const [layoutMode, setLayoutMode] = useState<'tab' | 'grid'>('tab');
    
    // Store refs to all terminal instances
    const terminalRefs = useRef(new Map<string, TerminalRef>());
    // Store unlisten functions for events
    const unlisteners = useRef(new Map<string, () => void>());

    useEffect(() => {
        // Listen for session closed events from backend
        let cancelClose: (() => void) | undefined;
        // @ts-ignore
        if (window.runtime && window.runtime.EventsOn) {
            // @ts-ignore
            cancelClose = window.runtime.EventsOn("session-closed", (id: string) => {
                removeTerminal(id);
            });
        }
        return () => {
            if (cancelClose) cancelClose();
            // Cleanup all terminal listeners
            unlisteners.current.forEach(u => u());
            unlisteners.current.clear();
        };
    }, []);

    const removeTerminal = (id: string) => {
        setTerminals(prev => prev.filter(t => t.id !== id));
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
        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.Write) {
             // @ts-ignore
            window.go.main.App.Write(id, data);
        }
    };

    const handleBroadcast = (command: string) => {
        const ids = terminals.map(t => t.id);
        if (ids.length === 0) return;
        
        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.Broadcast) {
            // Send command + newline
            const payload = command.endsWith('\n') ? command : command + '\n';
             // @ts-ignore
            window.go.main.App.Broadcast(ids, payload);
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

    // Force layout update when sidebar toggles
    useEffect(() => {
        setTimeout(() => {
            terminalRefs.current.forEach(t => t.fit());
        }, 300); // Wait for transition
    }, [isSidebarOpen]);

    const toggleSidebar = (tab: 'sessions' | 'ai') => {
        if (isSidebarOpen && sidebarTab === tab) {
            // If clicking the active tab, close it
            setIsSidebarOpen(false);
        } else {
            // Open and switch tab
            setIsSidebarOpen(true);
            setSidebarTab(tab);
        }
    };

    return (
        <div id="app" style={{height: '100vh', display: 'flex', flexDirection: 'column'}}>
            <div style={{
                padding: '8px 16px', 
                background: '#333', 
                color: '#fff', 
                display: 'flex', 
                gap: '16px', 
                alignItems: 'center',
                justifyContent: 'space-between'
            }}>
                <div style={{display: 'flex', gap: '16px', alignItems: 'center'}}>
                    <span style={{fontSize: '0.9rem', color: status === '已连接' ? '#4caf50' : '#aaa'}}>
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

            <div style={{flex: 1, position: 'relative', overflow: 'hidden', display: 'flex', flexDirection: 'row'}}>
                <div style={{flex: 1, position: 'relative', overflow: 'hidden'}}>
                    <LayoutManager 
                        terminals={terminals}
                        mode={layoutMode}
                        onTerminalData={handleTerminalData}
                        terminalRefs={terminalRefs}
                        onCloseTerminal={handleCloseTerminal}
                        onRenameTerminal={handleRenameTerminal}
                        onClose={() => {}}
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
                            backgroundColor: (isSidebarOpen && sidebarTab === 'ai') ? '#333' : 'transparent',
                            borderRight: (isSidebarOpen && sidebarTab === 'ai') ? '2px solid #007acc' : '2px solid transparent'
                        }}
                        onClick={() => toggleSidebar('ai')}
                        title="AI 助手"
                    >
                        🤖
                    </div>
                </div>
            </div>

            <BroadcastBar onBroadcast={handleBroadcast} />
            
            <SmartConnectModal 
                isOpen={isSmartModalOpen}
                onClose={() => setIsSmartModalOpen(false)}
                onConnect={handleBatchConnect}
                onParse={handleParseIntent}
            />

            <SettingsModal 
                isOpen={isSettingsOpen}
                onClose={() => setIsSettingsOpen(false)}
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
