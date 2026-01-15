import React, { useEffect, useRef, forwardRef, useImperativeHandle, useState, useCallback } from 'react';
import { Terminal } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import 'xterm/css/xterm.css';
import CompletionOverlay, { CompletionData, CompletionSuggestion } from './CompletionOverlay';

interface TerminalProps {
    id: string;
    sessionID?: string;  // SSH session ID for backend PTY resize
    onData?: (data: string) => void;
    completionDelay?: number;  // Completion delay in milliseconds
}

export interface TerminalRef {
    write: (data: string) => void;
    fit: () => void;
}

const TerminalComponent = forwardRef<TerminalRef, TerminalProps>(({ id, sessionID, onData, completionDelay = 150 }, ref) => {
    const terminalRef = useRef<HTMLDivElement>(null);
    const xtermRef = useRef<Terminal | null>(null);
    const fitAddonRef = useRef<FitAddon | null>(null);
    const onDataRef = useRef<((data: string) => void) | undefined>(onData);
    const sessionIDRef = useRef<string | undefined>(sessionID);
    const completionDelayRef = useRef<number>(completionDelay);

    // Completion state
    const [completionVisible, setCompletionVisible] = useState(false);
    const [completionPosition, setCompletionPosition] = useState({ x: 0, y: 0 });
    const [completionData, setCompletionData] = useState<CompletionData>({ suggestions: [], replace_from: 0, replace_to: 0 });
    const [selectedIndex, setSelectedIndex] = useState(0);

    // Track current input line for completion (simple buffer, no interference)
    const currentInputRef = useRef('');
    const currentLineRef = useRef('');
    const promptStartRef = useRef(true);
    const debounceTimerRef = useRef<NodeJS.Timeout | null>(null);

    // Refs to avoid useEffect re-runs
    const completionVisibleRef = useRef(false);
    const completionDataRef = useRef<CompletionData>({ suggestions: [], replace_from: 0, replace_to: 0 });
    const selectedIndexRef = useRef(0);

    // Sync refs with state
    useEffect(() => {
        completionVisibleRef.current = completionVisible;
        completionDataRef.current = completionData;
        selectedIndexRef.current = selectedIndex;
    }, [completionVisible, completionData, selectedIndex]);

    useEffect(() => {
        onDataRef.current = onData;
        sessionIDRef.current = sessionID;
        completionDelayRef.current = completionDelay;
    }, [onData, sessionID, completionDelay]);

    // Helper function to sync terminal size to backend PTY
    const syncSizeToBackend = () => {
        if (!sessionIDRef.current || !xtermRef.current) return;

        const cols = xtermRef.current.cols;
        const rows = xtermRef.current.rows;

        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.ResizeTerminal) {
            // @ts-ignore
            window.go.main.App.ResizeTerminal(sessionIDRef.current, cols, rows);
        }
    };

    // Fetch completions from backend
    const fetchCompletions = useCallback(async (input: string) => {
        // @ts-ignore
        if (!window.go || !window.go.main || !window.go.main.App || !window.go.main.App.GetCompletions) {
            return;
        }

        try {
            // @ts-ignore
            const result = await window.go.main.App.GetCompletions(input, input.length);
            const data: CompletionData = JSON.parse(result);

            if (data.suggestions.length > 0) {
                setCompletionData(data);
                setSelectedIndex(0);
                setCompletionVisible(true);
                updateCompletionPosition();
            } else {
                setCompletionVisible(false);
            }
        } catch (error) {
            console.error('Failed to fetch completions:', error);
            setCompletionVisible(false);
        }
    }, []);

    // Calculate and update completion overlay position
    const updateCompletionPosition = useCallback(() => {
        if (!xtermRef.current || !terminalRef.current) return;

        const term = xtermRef.current;
        const container = terminalRef.current;

        // Get actual cursor position from terminal buffer
        const buffer = term.buffer.active;
        const cursorY = buffer.cursorY;
        const cursorX = buffer.cursorX;

        // Get viewport info to calculate actual screen position
        const viewportY = buffer.viewportY; // Scroll offset
        const actualRow = cursorY - viewportY; // Row relative to viewport

        const containerRect = container.getBoundingClientRect();

        // Calculate pixel position using container dimensions
        const cellWidth = containerRect.width / term.cols;
        const cellHeight = containerRect.height / term.rows;

        // Position right below the cursor
        const x = containerRect.left + cursorX * cellWidth;
        const y = containerRect.top + actualRow * cellHeight + cellHeight;

        setCompletionPosition({ x, y });
    }, []);

    // Handle completion selection
    const handleCompletionSelect = useCallback((suggestion: CompletionSuggestion) => {
        if (!onDataRef.current) return;

        const { replace_from, replace_to } = completionDataRef.current;

        // Calculate how many characters to delete
        const charsToDelete = replace_to - replace_from;

        // Send backspaces to delete the partial word
        for (let i = 0; i < charsToDelete; i++) {
            onDataRef.current('\x7f'); // Backspace
        }

        // Send the completion text
        onDataRef.current(suggestion.text);

        // Update our local tracking
        const before = currentInputRef.current.slice(0, replace_from);
        currentInputRef.current = before + suggestion.text;

        setCompletionVisible(false);
    }, []);

    // Handle keyboard navigation
    const handleNavigate = useCallback((direction: 'up' | 'down') => {
        if (!completionVisibleRef.current) return;

        setSelectedIndex(prev => {
            const suggestionsLength = completionDataRef.current.suggestions.length;
            if (direction === 'up') {
                return prev <= 0 ? suggestionsLength - 1 : prev - 1;
            } else {
                return prev >= suggestionsLength - 1 ? 0 : prev + 1;
            }
        });
    }, []);

    useImperativeHandle(ref, () => ({
        write: (data: string) => {
            xtermRef.current?.write(data);
        },
        fit: () => {
            fitAddonRef.current?.fit();
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

        setTimeout(() => syncSizeToBackend(), 100);

        term.onData((data) => {
            // Pass all data through to backend immediately
            onDataRef.current?.(data);

            // Handle special keys for completion
            if (data === '\r' || data === '\n') {
                // Enter - clear input tracking
                currentInputRef.current = '';
                currentLineRef.current = '';
                promptStartRef.current = true;
                setCompletionVisible(false);
                if (debounceTimerRef.current) {
                    clearTimeout(debounceTimerRef.current);
                }
                return;
            }

            if (data === '\x03') {
                // Ctrl+C - clear everything
                currentInputRef.current = '';
                currentLineRef.current = '';
                promptStartRef.current = true;
                setCompletionVisible(false);
                if (debounceTimerRef.current) {
                    clearTimeout(debounceTimerRef.current);
                }
                return;
            }

            if (data === '\x1b') {
                // Escape - hide completion
                setCompletionVisible(false);
                return;
            }

            if (data === '\x7f' || data === '\x08') {
                // Backspace
                if (currentInputRef.current.length > 0) {
                    currentInputRef.current = currentInputRef.current.slice(0, -1);
                }
                setCompletionVisible(false);
                completionVisibleRef.current = false; // Sync update to avoid race condition
                triggerCompletion();
                return;
            }

            // Regular printable character
            if (data.length === 1 && data >= ' ') {
                currentInputRef.current += data;
                setCompletionVisible(false);
                completionVisibleRef.current = false; // Sync update to avoid race condition
                triggerCompletion();
            }
        });

        const triggerCompletion = () => {
            if (debounceTimerRef.current) {
                clearTimeout(debounceTimerRef.current);
            }

            debounceTimerRef.current = setTimeout(() => {
                const input = currentInputRef.current;
                // Only trigger for alphanumeric input
                if (input.length > 0 && /[a-zA-Z0-9_-]/.test(input[input.length - 1])) {
                    fetchCompletions(input);
                } else {
                    setCompletionVisible(false);
                }
            }, completionDelayRef.current);
        };

        // Add key handler for Tab
        term.attachCustomKeyEventHandler((arg) => {
            // Arrow keys - handle completion navigation
            if (completionVisibleRef.current && arg.type === 'keydown') {
                if (arg.key === 'ArrowUp') {
                    arg.preventDefault();
                    setSelectedIndex(prev => {
                        const suggestionsLength = completionDataRef.current.suggestions.length;
                        return prev <= 0 ? suggestionsLength - 1 : prev - 1;
                    });
                    return false;
                }
                if (arg.key === 'ArrowDown') {
                    arg.preventDefault();
                    setSelectedIndex(prev => {
                        const suggestionsLength = completionDataRef.current.suggestions.length;
                        return prev >= suggestionsLength - 1 ? 0 : prev + 1;
                    });
                    return false;
                }
            }

            // Tab key - handle completion
            if (arg.key === 'Tab' && arg.type === 'keydown' && completionVisibleRef.current) {
                arg.preventDefault();
                const data = completionDataRef.current;
                if (data.suggestions.length > 0 && selectedIndexRef.current >= 0) {
                    handleCompletionSelect(data.suggestions[selectedIndexRef.current]);
                }
                return false;
            }

            // Ctrl+C (Copy)
            if (arg.ctrlKey && arg.code === 'KeyC' && arg.type === 'keydown') {
                const selection = term.getSelection();
                if (selection) {
                    navigator.clipboard.writeText(selection);
                    return false;
                }
            }

            // Ctrl+V (Paste)
            if (arg.ctrlKey && arg.code === 'KeyV' && arg.type === 'keydown') {
                return false;
            }

            return true;
        });

        // Paste handler
        const handlePaste = (e: ClipboardEvent) => {
            e.preventDefault();
            e.stopPropagation();
            const text = e.clipboardData?.getData('text');
            if (text) {
                currentInputRef.current += text;
                setCompletionVisible(false);
                completionVisibleRef.current = false;
                triggerCompletion();
                term.paste(text);
            }
        };
        terminalRef.current.addEventListener('paste', handlePaste);

        // Middle click paste
        const handleAuxClick = (e: MouseEvent) => {
            if (e.button === 1) {
                e.preventDefault();
                navigator.clipboard.readText().then(text => {
                    if (text) {
                        currentInputRef.current += text;
                        setCompletionVisible(false);
                        completionVisibleRef.current = false;
                        triggerCompletion();
                        term.paste(text);
                    }
                    term.focus();
                });
            }
        };
        terminalRef.current.addEventListener('auxclick', handleAuxClick);

        // Right click context menu
        const handleContextMenu = (e: MouseEvent) => {
            e.preventDefault();
            e.stopPropagation();
            const selection = term.getSelection();
            if (selection) {
                navigator.clipboard.writeText(selection);
                term.clearSelection();
            } else {
                navigator.clipboard.readText().then(text => {
                    if (text) {
                        currentInputRef.current += text;
                        setCompletionVisible(false);
                        completionVisibleRef.current = false;
                        triggerCompletion();
                        term.paste(text);
                    }
                    term.focus();
                });
            }
        };
        terminalRef.current.addEventListener('contextmenu', handleContextMenu);

        // Window resize
        const handleResize = () => {
            fitAddon.fit();
            setTimeout(() => syncSizeToBackend(), 10);
        };
        window.addEventListener('resize', handleResize);

        return () => {
            if (debounceTimerRef.current) {
                clearTimeout(debounceTimerRef.current);
            }
            window.removeEventListener('resize', handleResize);
            terminalRef.current?.removeEventListener('paste', handlePaste);
            terminalRef.current?.removeEventListener('auxclick', handleAuxClick);
            terminalRef.current?.removeEventListener('contextmenu', handleContextMenu);
            term.dispose();
        };
    }, [fetchCompletions, handleCompletionSelect]);

    return (
        <>
            <div
                id={`terminal-${id}`}
                data-testid={`terminal-container-${id}`}
                ref={terminalRef}
                style={{ width: '100%', height: '100%', overflow: 'hidden' }}
            />
            <CompletionOverlay
                visible={completionVisible}
                position={completionPosition}
                completions={completionData}
                selectedIndex={selectedIndex}
                onSelect={handleCompletionSelect}
                onNavigate={handleNavigate}
                onClose={() => setCompletionVisible(false)}
            />
        </>
    );
});

export default TerminalComponent;
