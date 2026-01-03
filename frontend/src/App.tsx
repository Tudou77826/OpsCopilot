import { useState, useRef, useEffect } from 'react';
import './App.css';
import TerminalComponent, { TerminalRef } from './components/Terminal/Terminal';
import ConnectionModal from './components/ConnectionModal/ConnectionModal';

// Placeholder for Wails bindings which will be generated
const wails = window as any;

function App() {
    const terminalRef = useRef<TerminalRef>(null);
    const [status, setStatus] = useState("Disconnected");
    const [isModalOpen, setIsModalOpen] = useState(false);

    useEffect(() => {
        // Listen to backend events
        let cancelData: (() => void) | undefined;
        let cancelStatus: (() => void) | undefined;
        // @ts-ignore
        if (window.runtime && window.runtime.EventsOn) {
             // @ts-ignore
            cancelData = window.runtime.EventsOn("terminal-data", (data: string) => {
                terminalRef.current?.write(data);
            });
             // @ts-ignore
            cancelStatus = window.runtime.EventsOn("connection-status", (status: string) => {
                setStatus(status);
            });
        }
        
        return () => {
            if (cancelData) cancelData();
            if (cancelStatus) cancelStatus();
        };
    }, []);

    const handleConnect = async (config: any) => {
        setStatus("Connecting...");
        try {
            // @ts-ignore
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.Connect) {
                 // @ts-ignore
                const result = await window.go.main.App.Connect(config);
                setStatus(result);
                // Give some time for UI to settle then fit
                setTimeout(() => terminalRef.current?.fit(), 100);
            } else {
                setStatus("Wails runtime not ready");
            }
        } catch (e) {
            setStatus("Error: " + e);
        }
    };

    const handleData = (data: string) => {
        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.Write) {
             // @ts-ignore
            window.go.main.App.Write(data);
        }
    };

    return (
        <div id="app" style={{height: '100vh', display: 'flex', flexDirection: 'column'}}>
            <div style={{padding: '10px', background: '#333', color: '#fff', display: 'flex', gap: '10px', alignItems: 'center'}}>
                <span>Status: {status}</span>
                <button onClick={() => setIsModalOpen(true)} style={{padding: '5px 10px', cursor: 'pointer'}}>New Connection</button>
            </div>
            <div style={{flex: 1, position: 'relative', background: '#000'}}>
                <TerminalComponent id="main-term" ref={terminalRef} onData={handleData} />
            </div>
            <ConnectionModal 
                isOpen={isModalOpen}
                onClose={() => setIsModalOpen(false)}
                onConnect={handleConnect}
            />
        </div>
    );
}

export default App;
