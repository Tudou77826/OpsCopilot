import React, { useEffect, useRef, forwardRef, useImperativeHandle } from 'react';
import { Terminal } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import 'xterm/css/xterm.css';

interface TerminalProps {
    id: string;
    sessionID?: string;  // SSH session ID for backend PTY resize
    onData?: (data: string) => void;
}

export interface TerminalRef {
    write: (data: string) => void;
    fit: () => void;
}

const TerminalComponent = forwardRef<TerminalRef, TerminalProps>(({ id, sessionID, onData }, ref) => {
    const terminalRef = useRef<HTMLDivElement>(null);
    const xtermRef = useRef<Terminal | null>(null);
    const fitAddonRef = useRef<FitAddon | null>(null);

    // Helper function to sync terminal size to backend PTY
    const syncSizeToBackend = () => {
        if (!sessionID || !xtermRef.current) return;
        
        const cols = xtermRef.current.cols;
        const rows = xtermRef.current.rows;
        
        // Call backend to resize PTY
        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.ResizeTerminal) {
            // @ts-ignore
            window.go.main.App.ResizeTerminal(sessionID, cols, rows);
        }
    };

    useImperativeHandle(ref, () => ({
        write: (data: string) => {
            xtermRef.current?.write(data);
        },
        fit: () => {
            fitAddonRef.current?.fit();
            // Sync size to backend after fit
            setTimeout(() => syncSizeToBackend(), 10);
        }
    }));

    useEffect(() => {
        if (!terminalRef.current) return;

        const term = new Terminal({
            cursorBlink: true,
            theme: {
                background: '#1e1e1e',
            }
        });
        const fitAddon = new FitAddon();
        term.loadAddon(fitAddon);

        term.open(terminalRef.current);
        fitAddon.fit();
        
        fitAddonRef.current = fitAddon;
        xtermRef.current = term;

        // Sync initial size to backend PTY
        setTimeout(() => syncSizeToBackend(), 100);

        term.onData((data) => {
            onData?.(data);
        });

        // Add Clipboard support
        term.attachCustomKeyEventHandler((arg) => {
            // Ctrl+C (Copy)
            if (arg.ctrlKey && arg.code === 'KeyC' && arg.type === 'keydown') {
                const selection = term.getSelection();
                if (selection) {
                    navigator.clipboard.writeText(selection);
                    return false; // Prevent sending Ctrl+C to backend if text is selected
                }
            }
            // Ctrl+V (Paste)
            if (arg.ctrlKey && arg.code === 'KeyV' && arg.type === 'keydown') {
                // Prevent default browser paste behavior to avoid double paste
                // We handle it manually via clipboard API
                return false; 
            }
            return true;
        });

        // Add Global Paste Listener for the terminal container
        // This handles Ctrl+V (browser native), Middle Click (sometimes), and Right Click Paste (if context menu allowed)
        // But since we override context menu and auxclick, this mainly catches Ctrl+V that slips through or other paste events
        const handlePaste = (e: ClipboardEvent) => {
             e.preventDefault();
             e.stopPropagation();
             const text = e.clipboardData?.getData('text');
             if (text) {
                 onData?.(text);
             }
        };
        terminalRef.current.addEventListener('paste', handlePaste);

        // Mouse Middle Click Paste
        terminalRef.current.addEventListener('auxclick', (e) => {
            if (e.button === 1) { // Middle click
                e.preventDefault();
                navigator.clipboard.readText().then(text => {
                    onData?.(text);
                    term.focus();
                });
            }
        });

        // Mouse Right Click Copy/Paste
        terminalRef.current.addEventListener('contextmenu', (e) => {
            e.preventDefault(); // Prevent default context menu
            e.stopPropagation();
            const selection = term.getSelection();
            if (selection) {
                // Copy if there is a selection
                navigator.clipboard.writeText(selection);
                term.clearSelection();
            } else {
                // Paste if no selection
                navigator.clipboard.readText().then(text => {
                    onData?.(text);
                    term.focus();
                });
            }
        });

        // Handle resize
        const handleResize = () => {
            fitAddon.fit();
            // Sync size to backend after window resize
            setTimeout(() => syncSizeToBackend(), 10);
        };
        window.addEventListener('resize', handleResize);

        return () => {
            window.removeEventListener('resize', handleResize);
            terminalRef.current?.removeEventListener('paste', handlePaste);
            term.dispose();
        };
    }, []);

    return (
        <div 
            id={`terminal-${id}`} 
            data-testid={`terminal-container-${id}`} 
            ref={terminalRef} 
            style={{ width: '100%', height: '100%', overflow: 'hidden' }}
        />
    );
});

export default TerminalComponent;
