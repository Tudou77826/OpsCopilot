import React, { useEffect, useRef, forwardRef, useImperativeHandle, useState, useCallback } from 'react';
import { Terminal } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import { SearchAddon } from 'xterm-addon-search';
import 'xterm/css/xterm.css';
import CompletionOverlay, { CompletionData, CompletionSuggestion } from './CompletionOverlay';
import SearchPanel from './SearchPanel';
import { HighlightRule, TerminalConfig } from './highlightTypes';

interface TerminalProps {
    id: string;
    sessionID?: string;  // SSH session ID for backend PTY resize
    onData?: (data: string) => void;
    completionDelay?: number;  // Completion delay in milliseconds
    terminalConfig?: TerminalConfig;
    highlightRules?: HighlightRule[];
}

export interface TerminalRef {
    write: (data: string) => void;
    fit: () => void;
    getCursorScreenPosition: () => { x: number; y: number } | null;
    focus: () => void;
}

const TerminalComponent = forwardRef<TerminalRef, TerminalProps>(({ id, sessionID, onData, completionDelay = 150, terminalConfig, highlightRules }, ref) => {
    const terminalRef = useRef<HTMLDivElement>(null);
    const xtermRef = useRef<Terminal | null>(null);
    const fitAddonRef = useRef<FitAddon | null>(null);
    const onDataRef = useRef<((data: string) => void) | undefined>(onData);
    const sessionIDRef = useRef<string | undefined>(sessionID);
    const completionDelayRef = useRef<number>(completionDelay);
    const terminalConfigRef = useRef<TerminalConfig | undefined>(undefined);
    const highlightRulesRef = useRef<HighlightRule[] | undefined>(undefined);

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

    const [searchVisible, setSearchVisible] = useState(false);
    const [searchQuery, setSearchQuery] = useState('');
    const [searchCaseSensitive, setSearchCaseSensitive] = useState(false);
    const [searchRegexMode, setSearchRegexMode] = useState(false);
    const [searchCountText, setSearchCountText] = useState('');
    const searchAddonRef = useRef<SearchAddon | null>(null);
    const searchCountTimerRef = useRef<number | null>(null);
    const searchInputRef = useRef<HTMLInputElement>(null);
    const searchVisibleRef = useRef(false);
    const searchQueryRef = useRef('');
    const searchCountTokenRef = useRef(0);
    const searchCaseSensitiveRef = useRef(false);
    const searchRegexModeRef = useRef(false);
    const searchDecorationsRef = useRef<Map<number, { marker: any; decos: any[] }>>(new Map());
    const searchHighlightTimerRef = useRef<number | null>(null);
    const searchHighlightTokenRef = useRef(0);
    const currentSearchDecoRef = useRef<{ marker: any; deco: any } | null>(null);

    const highlightEnabledRef = useRef<boolean>(true);
    const decorationsRef = useRef<Map<number, any[]>>(new Map());
    const highlightTimerRef = useRef<number | null>(null);

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
        searchVisibleRef.current = searchVisible;
        searchQueryRef.current = searchQuery;
        searchCaseSensitiveRef.current = searchCaseSensitive;
        searchRegexModeRef.current = searchRegexMode;
    }, [searchQuery, searchVisible, searchCaseSensitive, searchRegexMode]);

    useEffect(() => {
        onDataRef.current = onData;
        sessionIDRef.current = sessionID;
        completionDelayRef.current = completionDelay;
        terminalConfigRef.current = terminalConfig;
        highlightRulesRef.current = highlightRules;
        highlightEnabledRef.current = terminalConfig?.highlight_enabled ?? true;
    }, [onData, sessionID, completionDelay, terminalConfig, highlightRules]);

    const getSearchEnabled = () => terminalConfigRef.current?.search_enabled ?? true;

    const closeSearch = useCallback(() => {
        setSearchVisible(false);
        setSearchQuery('');
        setSearchCountText('');
        searchCountTokenRef.current++;
        searchHighlightTokenRef.current++;
        if (searchHighlightTimerRef.current) {
            window.clearTimeout(searchHighlightTimerRef.current);
            searchHighlightTimerRef.current = null;
        }
        for (const entry of searchDecorationsRef.current.values()) {
            for (const d of entry.decos) {
                try { d.dispose?.(); } catch { }
            }
            try { entry.marker?.dispose?.(); } catch { }
        }
        searchDecorationsRef.current.clear();
        if (currentSearchDecoRef.current) {
            try { currentSearchDecoRef.current.deco?.dispose?.(); } catch { }
            try { currentSearchDecoRef.current.marker?.dispose?.(); } catch { }
            currentSearchDecoRef.current = null;
        }
        if (xtermRef.current) {
            xtermRef.current.clearSelection();
            xtermRef.current.focus();
        }
    }, []);

    useEffect(() => {
        if (!searchVisible) return;
        const onKeyDown = (e: KeyboardEvent) => {
            if (e.key !== 'Escape') return;
            e.preventDefault();
            e.stopPropagation();
            closeSearch();
        };
        window.addEventListener('keydown', onKeyDown, true);
        return () => window.removeEventListener('keydown', onKeyDown, true);
    }, [closeSearch, searchVisible]);

    const clearCurrentSearchDecoration = useCallback(() => {
        if (currentSearchDecoRef.current) {
            try { currentSearchDecoRef.current.deco?.dispose?.(); } catch { }
            try { currentSearchDecoRef.current.marker?.dispose?.(); } catch { }
            currentSearchDecoRef.current = null;
        }
    }, []);

    useEffect(() => {
        if (!searchVisible) {
            clearCurrentSearchDecoration();
            return;
        }
        if (!searchQuery.trim()) {
            clearCurrentSearchDecoration();
            return;
        }
        clearCurrentSearchDecoration();
    }, [clearCurrentSearchDecoration, searchQuery, searchRegexMode, searchCaseSensitive, searchVisible]);

    const updateCurrentSearchDecoration = useCallback(() => {
        const term = xtermRef.current;
        if (!term) return;
        clearCurrentSearchDecoration();
        // @ts-ignore
        const pos = term.getSelectionPosition?.();
        if (!pos) return;
        const start = pos.start;
        const end = pos.end;
        if (!start || !end) return;
        let sx = start.x ?? 0;
        let sy = start.y ?? 0;
        let ex = end.x ?? sx;
        let ey = end.y ?? sy;
        if (sy > ey || (sy === ey && sx > ex)) {
            [sx, ex] = [ex, sx];
            [sy, ey] = [ey, sy];
        }

        const buffer = term.buffer.active;
        const lineIdx = Math.max(0, sy);
        const cursorAbs = buffer.baseY + buffer.cursorY;
        const offset = lineIdx - cursorAbs;
        const marker = term.registerMarker(offset);
        if (!marker) return;

        const x = Math.max(0, sx);
        const width = Math.max(1, sy === ey ? (ex - sx) : (term.cols - sx));
        const deco = term.registerDecoration({
            marker,
            x,
            width,
            backgroundColor: '#f59e0b',
            foregroundColor: '#000000',
            layer: 'top',
        });
        if (!deco) {
            try { marker.dispose?.(); } catch { }
            return;
        }
        currentSearchDecoRef.current = { marker, deco };
    }, [clearCurrentSearchDecoration]);

    const scheduleSearchCount = useCallback(() => {
        if (searchCountTimerRef.current) {
            window.clearTimeout(searchCountTimerRef.current);
            searchCountTimerRef.current = null;
        }
        if (!searchVisible) return;
        const q = searchQuery.trim();
        if (!q) {
            setSearchCountText('');
            return;
        }
        const term = xtermRef.current;
        if (!term) return;
        const token = ++searchCountTokenRef.current;

        const run = () => {
            const max = 1000;
            const buffer = term.buffer.active;
            let count = 0;
            let re: RegExp | null = null;
            let needleLower = '';
            let needle = q;
            if (searchRegexMode) {
                const flags = `g${searchCaseSensitive ? '' : 'i'}`;
                try {
                    re = new RegExp(q, flags);
                } catch {
                    setSearchCountText('');
                    return;
                }
            } else if (!searchCaseSensitive) {
                needleLower = q.toLowerCase();
            }

            let i = 0;
            const step = (deadline?: IdleDeadline) => {
                if (token !== searchCountTokenRef.current) return;
                const start = performance.now();
                for (; i < buffer.length; i++) {
                    if (token !== searchCountTokenRef.current) return;
                    const line = buffer.getLine(i)?.translateToString(true) || '';
                    if (!line) continue;
                    if (re) {
                        re.lastIndex = 0;
                        let m: RegExpExecArray | null;
                        while ((m = re.exec(line)) !== null) {
                            count++;
                            if (count >= max) break;
                            if (m.index === re.lastIndex) re.lastIndex++;
                        }
                    } else {
                        const hay = searchCaseSensitive ? line : line.toLowerCase();
                        const n = searchCaseSensitive ? needle : needleLower;
                        let idx = 0;
                        while ((idx = hay.indexOf(n, idx)) !== -1) {
                            count++;
                            if (count >= max) break;
                            idx += Math.max(1, n.length);
                        }
                    }
                    if (count >= max) break;
                    if (deadline && deadline.timeRemaining() < 4) break;
                    if (!deadline && performance.now() - start > 8) break;
                }

                if (count >= max || i >= buffer.length) {
                    setSearchCountText(count >= max ? `>=${max}` : `${count}`);
                    return;
                }

                if (typeof (window as any).requestIdleCallback === 'function') {
                    (window as any).requestIdleCallback(step);
                } else {
                    window.setTimeout(() => step(), 0);
                }
            };

            if (typeof (window as any).requestIdleCallback === 'function') {
                (window as any).requestIdleCallback(step);
            } else {
                window.setTimeout(() => step(), 0);
            }
        };

        searchCountTimerRef.current = window.setTimeout(() => {
            searchCountTimerRef.current = null;
            if (token !== searchCountTokenRef.current) return;
            run();
        }, 180);
    }, [searchCaseSensitive, searchQuery, searchRegexMode, searchVisible]);

    useEffect(() => {
        scheduleSearchCount();
    }, [scheduleSearchCount]);

    const doSearchNext = useCallback(() => {
        const addon = searchAddonRef.current;
        if (!addon || !xtermRef.current) return;
        const q = searchQueryRef.current.trim();
        if (!q) return;
        let ok = false;
        try {
            ok = addon.findNext(q, { caseSensitive: searchCaseSensitiveRef.current, regex: searchRegexModeRef.current, incremental: false });
        } catch {
            // ignore
        }
        if (ok) updateCurrentSearchDecoration();
        else clearCurrentSearchDecoration();
        searchInputRef.current?.focus();
    }, [clearCurrentSearchDecoration, updateCurrentSearchDecoration]);

    const doSearchPrev = useCallback(() => {
        const addon = searchAddonRef.current;
        if (!addon || !xtermRef.current) return;
        const q = searchQueryRef.current.trim();
        if (!q) return;
        let ok = false;
        try {
            ok = addon.findPrevious(q, { caseSensitive: searchCaseSensitiveRef.current, regex: searchRegexModeRef.current, incremental: false });
        } catch {
            // ignore
        }
        if (ok) updateCurrentSearchDecoration();
        else clearCurrentSearchDecoration();
        searchInputRef.current?.focus();
    }, [clearCurrentSearchDecoration, updateCurrentSearchDecoration]);

    const scheduleSearchHighlightAll = useCallback((delayMs: number, opts: { visible: boolean; query: string; caseSensitive: boolean; regexMode: boolean }) => {
        if (searchHighlightTimerRef.current) {
            window.clearTimeout(searchHighlightTimerRef.current);
            searchHighlightTimerRef.current = null;
        }

        const token = ++searchHighlightTokenRef.current;
        const visible = opts.visible;
        const q = opts.query.trim();
        const caseSensitive = opts.caseSensitive;
        const regexMode = opts.regexMode;

        searchHighlightTimerRef.current = window.setTimeout(() => {
            searchHighlightTimerRef.current = null;

            const term = xtermRef.current;
            if (!term) return;

            if (!visible || !q) {
            for (const entry of searchDecorationsRef.current.values()) {
                for (const d of entry.decos) {
                        try { d.dispose?.(); } catch { }
                    }
                try { entry.marker?.dispose?.(); } catch { }
                }
                searchDecorationsRef.current.clear();
                return;
            }

            for (const entry of searchDecorationsRef.current.values()) {
                for (const d of entry.decos) {
                    try { d.dispose?.(); } catch { }
                }
                try { entry.marker?.dispose?.(); } catch { }
            }
            searchDecorationsRef.current.clear();

            const buffer = term.buffer.active;
            const maxDecos = 2000;
            const bg = '#f6e05e';
            const fg = '#000000';

            let regex: RegExp | null = null;
            let needle = q;
            let needleLower = '';
            if (regexMode) {
                const flags = `g${caseSensitive ? '' : 'i'}`;
                try {
                    regex = new RegExp(q, flags);
                } catch {
                    return;
                }
            } else if (!caseSensitive) {
                needleLower = q.toLowerCase();
            }

            let i = 0;
            let total = 0;
            const step = (deadline?: IdleDeadline) => {
                if (token !== searchHighlightTokenRef.current) return;

                const startTs = performance.now();
                for (; i < buffer.length; i++) {
                    if (token !== searchHighlightTokenRef.current) return;
                    const line = buffer.getLine(i)?.translateToString(true) || '';
                    if (!line) continue;

                    const matches: Array<{ s: number; e: number }> = [];
                    if (regex) {
                        regex.lastIndex = 0;
                        let m: RegExpExecArray | null;
                        while ((m = regex.exec(line)) !== null) {
                            const s = m.index;
                            const e = s + Math.max(1, m[0]?.length || 1);
                            matches.push({ s, e });
                            if (matches.length >= 50) break;
                            if (m.index === regex.lastIndex) regex.lastIndex++;
                        }
                    } else {
                        const hay = caseSensitive ? line : line.toLowerCase();
                        const n = caseSensitive ? needle : needleLower;
                        let idx = 0;
                        while ((idx = hay.indexOf(n, idx)) !== -1) {
                            matches.push({ s: idx, e: idx + Math.max(1, n.length) });
                            if (matches.length >= 50) break;
                            idx += Math.max(1, n.length);
                        }
                    }

                    if (matches.length > 0) {
                        const cursorAbs = buffer.baseY + buffer.cursorY;
                        const offset = i - cursorAbs;
                        const marker = term.registerMarker(offset);
                        if (marker) {
                            const ds: any[] = [];
                            for (const m of matches) {
                                if (total >= maxDecos) break;
                                const deco = term.registerDecoration({
                                    marker,
                                    x: m.s,
                                    width: Math.max(1, m.e - m.s),
                                    backgroundColor: bg,
                                    foregroundColor: fg,
                                    layer: 'bottom',
                                });
                                if (deco) {
                                    ds.push(deco);
                                    total++;
                                }
                            }
                            if (ds.length > 0) {
                                searchDecorationsRef.current.set(i, { marker, decos: ds });
                            } else {
                                try { marker.dispose?.(); } catch { }
                            }
                        }
                    }

                    if (total >= maxDecos) break;
                    if (deadline && deadline.timeRemaining() < 4) break;
                    if (!deadline && performance.now() - startTs > 8) break;
                }

                if (total >= maxDecos || i >= buffer.length) return;

                if (typeof (window as any).requestIdleCallback === 'function') {
                    (window as any).requestIdleCallback(step);
                } else {
                    window.setTimeout(() => step(), 0);
                }
            };

            if (typeof (window as any).requestIdleCallback === 'function') {
                (window as any).requestIdleCallback(step);
            } else {
                window.setTimeout(() => step(), 0);
            }
        }, Math.max(0, delayMs));
    }, []);

    useEffect(() => {
        scheduleSearchHighlightAll(120, { visible: searchVisible, query: searchQuery, caseSensitive: searchCaseSensitive, regexMode: searchRegexMode });
    }, [scheduleSearchHighlightAll, searchVisible, searchQuery, searchCaseSensitive, searchRegexMode]);

    const clearDecorations = useCallback(() => {
        const m = decorationsRef.current;
        for (const ds of m.values()) {
            for (const d of ds) {
                try { d.dispose?.(); } catch { }
            }
        }
        m.clear();
    }, []);

    const scheduleHighlightScan = useCallback((delayMs: number) => {
        if (highlightTimerRef.current) {
            window.clearTimeout(highlightTimerRef.current);
            highlightTimerRef.current = null;
        }
        highlightTimerRef.current = window.setTimeout(() => {
            highlightTimerRef.current = null;
            const term = xtermRef.current;
            if (!term) return;
            if (!highlightEnabledRef.current) {
                clearDecorations();
                return;
            }

            const rules = (highlightRulesRef.current || []).slice().filter(r => r && r.is_enabled);
            if (rules.length === 0) {
                clearDecorations();
                return;
            }
            rules.sort((a, b) => (a.priority ?? 0) - (b.priority ?? 0));

            const buffer = term.buffer.active;
            const viewportY = buffer.viewportY;
            const cursorAbs = buffer.baseY + buffer.cursorY;
            const start = Math.max(0, viewportY - 100);
            const end = Math.min(buffer.length - 1, viewportY + term.rows - 1 + 100);

            for (const [k, ds] of decorationsRef.current.entries()) {
                if (k < start || k > end) {
                    for (const d of ds) {
                        try { d.dispose?.(); } catch { }
                    }
                    decorationsRef.current.delete(k);
                }
            }

            let totalDecos = 0;
            for (const ds of decorationsRef.current.values()) totalDecos += ds.length;

            const compiled = rules.map(r => {
                let pattern = r.pattern || '';
                let extraI = false;
                if (pattern.startsWith('(?i)')) {
                    extraI = true;
                    pattern = pattern.slice(4);
                }
                const flags = `g${extraI ? 'i' : ''}`;
                try {
                    return { rule: r, re: new RegExp(pattern, flags) };
                } catch {
                    return null;
                }
            }).filter(Boolean) as Array<{ rule: HighlightRule; re: RegExp }>;

            const budgetMs = 10;
            const t0 = performance.now();

            for (let lineIdx = start; lineIdx <= end; lineIdx++) {
                if (performance.now() - t0 > budgetMs) break;
                const line = buffer.getLine(lineIdx)?.translateToString(true) || '';
                if (!line) continue;

                const old = decorationsRef.current.get(lineIdx);
                if (old) {
                    for (const d of old) {
                        try { d.dispose?.(); } catch { }
                    }
                    decorationsRef.current.delete(lineIdx);
                    totalDecos = Math.max(0, totalDecos - old.length);
                }

                const ranges: Array<{ s: number; e: number; style: HighlightRule['style'] }> = [];
                for (const { rule, re } of compiled) {
                    if (ranges.length >= 20) break;
                    re.lastIndex = 0;
                    let m: RegExpExecArray | null;
                    while ((m = re.exec(line)) !== null) {
                        const s = m.index;
                        const e = s + Math.max(1, m[0]?.length || 1);
                        if (s >= e) {
                            if (m.index === re.lastIndex) re.lastIndex++;
                            continue;
                        }
                        const overlap = ranges.some(rg => !(e <= rg.s || s >= rg.e));
                        if (!overlap) {
                            ranges.push({ s, e, style: rule.style });
                        }
                        if (ranges.length >= 20) break;
                        if (m.index === re.lastIndex) re.lastIndex++;
                    }
                    if (ranges.length >= 20) break;
                }

                if (ranges.length === 0) continue;
                ranges.sort((a, b) => a.s - b.s);

                const decos: any[] = [];
                for (const rg of ranges) {
                    if (totalDecos >= 1000) break;
                    const offset = lineIdx - cursorAbs;
                    const marker = term.registerMarker(offset);
                    if (!marker) continue;
                    const bg = rg.style?.background_color;
                    const fg = rg.style?.color;
                    const deco = term.registerDecoration({
                        marker,
                        x: rg.s,
                        width: Math.max(1, rg.e - rg.s),
                        backgroundColor: bg,
                        foregroundColor: fg,
                        layer: 'bottom',
                    });
                    if (deco) {
                        decos.push(deco);
                        totalDecos++;
                    } else {
                        try { marker.dispose?.(); } catch { }
                    }
                }
                if (decos.length > 0) decorationsRef.current.set(lineIdx, decos);
                if (totalDecos >= 1000) break;
            }
        }, Math.max(0, delayMs));
    }, [clearDecorations]);

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
            scheduleHighlightScan(120);
            if (getSearchEnabled() && searchVisibleRef.current && searchQueryRef.current.trim()) {
                scheduleSearchCount();
                scheduleSearchHighlightAll(120, {
                    visible: searchVisibleRef.current,
                    query: searchQueryRef.current,
                    caseSensitive: searchCaseSensitiveRef.current,
                    regexMode: searchRegexModeRef.current
                });
            }
        },
        fit: () => {
            fitAddonRef.current?.fit();
            setTimeout(() => syncSizeToBackend(), 10);
        },
        focus: () => {
            xtermRef.current?.focus();
        },
        getCursorScreenPosition: () => {
            if (!xtermRef.current || !terminalRef.current) return null;

            const term = xtermRef.current;
            const container = terminalRef.current;

            const buffer = term.buffer.active;
            const cursorY = buffer.cursorY;
            const cursorX = buffer.cursorX;
            const viewportY = buffer.viewportY;
            const actualRow = cursorY - viewportY;

            const containerRect = container.getBoundingClientRect();
            const cellWidth = containerRect.width / term.cols;
            const cellHeight = containerRect.height / term.rows;

            const x = containerRect.left + cursorX * cellWidth;
            const y = containerRect.top + actualRow * cellHeight + cellHeight;
            return { x, y };
        }
    }));

    useEffect(() => {
        if (!terminalRef.current) return;

        const term = new Terminal({
            allowProposedApi: true,
            cursorBlink: true,
            scrollback: terminalConfig?.scrollback || 5000,
            theme: {
                background: '#1e1e1e',
            }
        });
        const fitAddon = new FitAddon();
        term.loadAddon(fitAddon);
        if (terminalConfig?.search_enabled ?? true) {
            const searchAddon = new SearchAddon();
            term.loadAddon(searchAddon);
            searchAddonRef.current = searchAddon;
        } else {
            searchAddonRef.current = null;
        }

        term.open(terminalRef.current);
        fitAddon.fit();

        fitAddonRef.current = fitAddon;
        xtermRef.current = term;

        setTimeout(() => syncSizeToBackend(), 100);
        scheduleHighlightScan(0);

        const onScrollDispose = term.onScroll(() => {
            scheduleHighlightScan(120);
            if (getSearchEnabled() && searchVisibleRef.current && searchQueryRef.current.trim()) {
                scheduleSearchHighlightAll(200, {
                    visible: searchVisibleRef.current,
                    query: searchQueryRef.current,
                    caseSensitive: searchCaseSensitiveRef.current,
                    regexMode: searchRegexModeRef.current
                });
            }
        });

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
            if (arg.type === 'keydown') {
                if (getSearchEnabled() && arg.ctrlKey && arg.code === 'KeyF') {
                    arg.preventDefault();
                    const selection = term.getSelection();
                    if (!searchVisibleRef.current) {
                        setSearchVisible(true);
                        if (selection) {
                            setSearchQuery(selection.slice(0, 200));
                        }
                        window.setTimeout(() => searchInputRef.current?.focus(), 0);
                    } else {
                        closeSearch();
                    }
                    return false;
                }
            }

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

            // Shift+Tab - handle completion acceptance
            if (arg.key === 'Tab' && arg.shiftKey && arg.type === 'keydown' && completionVisibleRef.current) {
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
                scheduleHighlightScan(120);
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
                        scheduleHighlightScan(120);
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
                        scheduleHighlightScan(120);
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
            if (searchCountTimerRef.current) {
                window.clearTimeout(searchCountTimerRef.current);
                searchCountTimerRef.current = null;
            }
            if (highlightTimerRef.current) {
                window.clearTimeout(highlightTimerRef.current);
                highlightTimerRef.current = null;
            }
            if (searchHighlightTimerRef.current) {
                window.clearTimeout(searchHighlightTimerRef.current);
                searchHighlightTimerRef.current = null;
            }
            for (const entry of searchDecorationsRef.current.values()) {
                for (const d of entry.decos) {
                    try { d.dispose?.(); } catch { }
                }
                try { entry.marker?.dispose?.(); } catch { }
            }
            searchDecorationsRef.current.clear();
            if (currentSearchDecoRef.current) {
                try { currentSearchDecoRef.current.deco?.dispose?.(); } catch { }
                try { currentSearchDecoRef.current.marker?.dispose?.(); } catch { }
                currentSearchDecoRef.current = null;
            }
            onScrollDispose.dispose();
            clearDecorations();
            window.removeEventListener('resize', handleResize);
            terminalRef.current?.removeEventListener('paste', handlePaste);
            terminalRef.current?.removeEventListener('auxclick', handleAuxClick);
            terminalRef.current?.removeEventListener('contextmenu', handleContextMenu);
            term.dispose();
        };
    }, [
        clearDecorations,
        fetchCompletions,
        handleCompletionSelect,
        handleNavigate,
        scheduleHighlightScan,
        terminalConfig?.scrollback,
        terminalConfig?.search_enabled,
    ]);

    useEffect(() => {
        scheduleHighlightScan(0);
    }, [highlightRules, terminalConfig?.highlight_enabled, scheduleHighlightScan]);

    return (
        <div style={{ width: '100%', height: '100%', overflow: 'hidden', position: 'relative' }}>
            <div
                id={`terminal-${id}`}
                data-testid={`terminal-container-${id}`}
                ref={terminalRef}
                style={{ width: '100%', height: '100%', overflow: 'hidden' }}
            />
            <SearchPanel
                visible={searchVisible && getSearchEnabled()}
                query={searchQuery}
                onQueryChange={(v) => setSearchQuery(v)}
                onClose={closeSearch}
                onNext={doSearchNext}
                onPrev={doSearchPrev}
                caseSensitive={searchCaseSensitive}
                onCaseSensitiveChange={setSearchCaseSensitive}
                regexMode={searchRegexMode}
                onRegexModeChange={setSearchRegexMode}
                matchText={searchCountText}
                ref={searchInputRef}
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
        </div>
    );
});

export default TerminalComponent;
