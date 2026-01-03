import React, { useEffect, useRef, forwardRef, useImperativeHandle } from 'react';
import { Terminal } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import 'xterm/css/xterm.css';

interface TerminalProps {
    id: string;
    onData?: (data: string) => void;
}

export interface TerminalRef {
    write: (data: string) => void;
    fit: () => void;
}

const TerminalComponent = forwardRef<TerminalRef, TerminalProps>(({ id, onData }, ref) => {
    const terminalRef = useRef<HTMLDivElement>(null);
    const xtermRef = useRef<Terminal | null>(null);
    const fitAddonRef = useRef<FitAddon | null>(null);

    useImperativeHandle(ref, () => ({
        write: (data: string) => {
            xtermRef.current?.write(data);
        },
        fit: () => {
            fitAddonRef.current?.fit();
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

        term.onData((data) => {
            onData?.(data);
        });

        // Handle resize
        const handleResize = () => {
            fitAddon.fit();
        };
        window.addEventListener('resize', handleResize);

        return () => {
            window.removeEventListener('resize', handleResize);
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
