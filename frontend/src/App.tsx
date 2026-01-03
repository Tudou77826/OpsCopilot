import { useState, useRef, useEffect } from 'react';
import './App.css';
import { TerminalRef } from './components/Terminal/Terminal';
import ConnectionModal from './components/ConnectionModal/ConnectionModal';
import LayoutManager from './components/LayoutManager/LayoutManager';
import BroadcastBar from './components/BroadcastBar/BroadcastBar';
import SmartConnectModal from './components/SmartConnectModal/SmartConnectModal';
import SettingsModal from './components/SettingsModal/SettingsModal';

interface TerminalSession {
    id: string;
    title: string;
}

interface ConnectionConfig {
    name?: string;
    host: string;
    port: number;
    user: string;
    password?: string;
    rootPassword?: string;
    bastion?: ConnectionConfig;
}

function App() {
    const [status, setStatus] = useState("Ready");
    const [isModalOpen, setIsModalOpen] = useState(false);
    const [isSmartModalOpen, setIsSmartModalOpen] = useState(false);
    const [isSettingsOpen, setIsSettingsOpen] = useState(false);
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
        setStatus("Connecting...");
        try {
            // @ts-ignore
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.Connect) {
                 // @ts-ignore
                const result = await window.go.main.App.Connect(config);
                
                if (result.success) {
                    setStatus("Connected");
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
                    setStatus("Error: " + result.message);
                }
            } else {
                setStatus("Wails runtime not ready");
            }
        } catch (e) {
            setStatus("Error: " + e);
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
        throw new Error("Wails runtime not ready");
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
                    <span style={{fontSize: '0.9rem', color: status === 'Connected' ? '#4caf50' : '#aaa'}}>
                        {status}
                    </span>
                    <button onClick={() => setIsSmartModalOpen(true)} style={{...styles.primaryBtn, backgroundColor: '#8e44ad'}}>
                        🤖 AI Connect
                    </button>
                    <button onClick={() => setIsModalOpen(true)} style={styles.primaryBtn}>
                        + New Connection
                    </button>
                    <button onClick={() => setIsSettingsOpen(true)} style={styles.iconBtn} title="Settings">
                        ⚙️
                    </button>
                </div>
                
                <div style={styles.toggleGroup}>
                    <button 
                        style={layoutMode === 'tab' ? styles.activeToggle : styles.toggle}
                        onClick={() => setLayoutMode('tab')}
                    >
                        Tab Mode
                    </button>
                    <button 
                        style={layoutMode === 'grid' ? styles.activeToggle : styles.toggle}
                        onClick={() => setLayoutMode('grid')}
                    >
                        Grid Mode
                    </button>
                </div>
            </div>

            <div style={{flex: 1, position: 'relative', overflow: 'hidden'}}>
                <LayoutManager 
                    terminals={terminals}
                    mode={layoutMode}
                    onTerminalData={handleTerminalData}
                    terminalRefs={terminalRefs}
                    onCloseTerminal={handleCloseTerminal}
                    onRenameTerminal={handleRenameTerminal}
                />
            </div>

            <BroadcastBar onBroadcast={handleBroadcast} />
            
            <ConnectionModal 
                isOpen={isModalOpen}
                onClose={() => setIsModalOpen(false)}
                onConnect={handleConnect}
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
    }
};

export default App;
