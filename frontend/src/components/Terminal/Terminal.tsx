import React, { useEffect, useRef, forwardRef, useImperativeHandle, useState, useCallback } from 'react';
import { Terminal } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import { SearchAddon } from 'xterm-addon-search';
import 'xterm/css/xterm.css';
import CompletionOverlay, { CompletionData, CompletionSuggestion } from './CompletionOverlay';
import SearchPanel from './SearchPanel';
import { HighlightRule, TerminalConfig } from './highlightTypes';
import {
    DEFAULT_TERMINAL_FONT_SIZE,
    clampTerminalFontSize,
    getTerminalFontStack,
} from './terminalAppearance';
import { parseTimestamp, TimestampResult } from '../../utils/timestampParser';

interface TerminalProps {
    id: string;
    sessionID?: string;  // SSH session ID for backend PTY resize
    onData?: (data: string) => void;
    completionDelay?: number;  // Completion delay in milliseconds
    terminalConfig?: TerminalConfig;
    onFontSizeChange?: (fontSize: number) => void;
    highlightRules?: HighlightRule[];
    /** 选区解析回调，用于置顶栏展示时间戳等解析结果 */
    onSelectionParsed?: (result: TimestampResult | null) => void;
}

export interface TerminalRef {
    write: (data: string) => void;
    fit: () => void;
    prepareForExternalInput: () => void;
    getCursorScreenPosition: () => { x: number; y: number } | null;
    focus: () => void;
}

const TerminalComponent = forwardRef<TerminalRef, TerminalProps>(({ id, sessionID, onData, completionDelay = 150, terminalConfig, onFontSizeChange, highlightRules, onSelectionParsed }, ref) => {
    const terminalRef = useRef<HTMLDivElement>(null);
    const xtermRef = useRef<Terminal | null>(null);
    const fitAddonRef = useRef<FitAddon | null>(null);
    const onDataRef = useRef<((data: string) => void) | undefined>(onData);
    const onSelectionParsedRef = useRef<((result: TimestampResult | null) => void) | undefined>(onSelectionParsed);
    const sessionIDRef = useRef<string | undefined>(sessionID);
    const completionDelayRef = useRef<number>(completionDelay);
    const terminalConfigRef = useRef<TerminalConfig | undefined>(undefined);
    const onFontSizeChangeRef = useRef<((fontSize: number) => void) | undefined>(onFontSizeChange);
    const pendingFontSizeRef = useRef(DEFAULT_TERMINAL_FONT_SIZE);
    const zoomIndicatorTimerRef = useRef<number | null>(null);
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
    const [zoomIndicatorSize, setZoomIndicatorSize] = useState<number | null>(null);
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

    // Write buffering refs (Change 1: frame-level flush)
    const writeQueueRef = useRef<string[]>([]);
    const flushScheduledRef = useRef<boolean>(false);

    // Post-output highlight scan refs (Change 2: scan only after output stabilizes)
    const lastOutputAtRef = useRef<number>(0);
    const postScanTimerRef = useRef<number | null>(null);
    const sizeSyncTimerRef = useRef<number | null>(null);
    const layoutRestoreRafRef = useRef<number | null>(null);
    const layoutRestoreTimerRefs = useRef<number[]>([]);
    const stickToBottomRef = useRef<boolean>(true);
    const suppressScrollTrackingRef = useRef<boolean>(false);

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
        onSelectionParsedRef.current = onSelectionParsed;
        sessionIDRef.current = sessionID;
        completionDelayRef.current = completionDelay;
        terminalConfigRef.current = terminalConfig;
        onFontSizeChangeRef.current = onFontSizeChange;
        pendingFontSizeRef.current = clampTerminalFontSize(terminalConfig?.font_size);
        highlightRulesRef.current = highlightRules;
        highlightEnabledRef.current = terminalConfig?.highlight_enabled ?? true;
    }, [onData, onSelectionParsed, sessionID, completionDelay, terminalConfig, onFontSizeChange, highlightRules]);

    const getSearchEnabled = () => terminalConfigRef.current?.search_enabled ?? true;

    const showZoomIndicator = useCallback((fontSize: number) => {
        setZoomIndicatorSize(fontSize);
        if (zoomIndicatorTimerRef.current !== null) {
            window.clearTimeout(zoomIndicatorTimerRef.current);
        }
        zoomIndicatorTimerRef.current = window.setTimeout(() => {
            zoomIndicatorTimerRef.current = null;
            setZoomIndicatorSize(null);
        }, 3200);
    }, []);

    const adjustTerminalFontSize = useCallback((next: number | ((current: number) => number)) => {
        const current = pendingFontSizeRef.current;
        const fontSize = clampTerminalFontSize(typeof next === 'function' ? next(current) : next);
        if (fontSize === current) return;
        pendingFontSizeRef.current = fontSize;
        showZoomIndicator(fontSize);
        onFontSizeChangeRef.current?.(fontSize);
    }, [showZoomIndicator]);

    useEffect(() => () => {
        if (zoomIndicatorTimerRef.current !== null) {
            window.clearTimeout(zoomIndicatorTimerRef.current);
        }
    }, []);

    useEffect(() => {
        const container = terminalRef.current;
        if (!container) return;
        const handleWheel = (event: WheelEvent) => {
            if (!event.ctrlKey) return;
            event.preventDefault();
            event.stopPropagation();
            adjustTerminalFontSize(current => current + (event.deltaY < 0 ? 1 : -1));
        };
        container.addEventListener('wheel', handleWheel, { passive: false });
        return () => container.removeEventListener('wheel', handleWheel);
    }, [adjustTerminalFontSize]);

    const clearSearchHitDecorations = useCallback(() => {
        for (const entry of searchDecorationsRef.current.values()) {
            for (const d of entry.decos) {
                try { d.dispose?.(); } catch { }
            }
            try { entry.marker?.dispose?.(); } catch { }
        }
        searchDecorationsRef.current.clear();
    }, []);

    const clearSearchDecorations = useCallback(() => {
        clearSearchHitDecorations();
        if (currentSearchDecoRef.current) {
            try { currentSearchDecoRef.current.deco?.dispose?.(); } catch { }
            try { currentSearchDecoRef.current.marker?.dispose?.(); } catch { }
            currentSearchDecoRef.current = null;
        }
    }, [clearSearchHitDecorations]);

    const closeSearch = useCallback(() => {
        searchVisibleRef.current = false;
        searchQueryRef.current = '';
        setSearchVisible(false);
        setSearchQuery('');
        setSearchCountText('');
        searchCountTokenRef.current++;
        searchHighlightTokenRef.current++;
        if (searchCountTimerRef.current) {
            window.clearTimeout(searchCountTimerRef.current);
            searchCountTimerRef.current = null;
        }
        if (searchHighlightTimerRef.current) {
            window.clearTimeout(searchHighlightTimerRef.current);
            searchHighlightTimerRef.current = null;
        }
        clearSearchDecorations();
        if (xtermRef.current) {
            xtermRef.current.clearSelection();
            xtermRef.current.focus();
        }
    }, [clearSearchDecorations]);

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

        if (!visible || !q) {
            clearSearchDecorations();
            return;
        }

        searchHighlightTimerRef.current = window.setTimeout(() => {
            searchHighlightTimerRef.current = null;

            if (token !== searchHighlightTokenRef.current) return;
            const term = xtermRef.current;
            if (!term) return;

            if (!searchVisibleRef.current || searchQueryRef.current.trim() !== q) {
                clearSearchDecorations();
                return;
            }

            clearSearchHitDecorations();

            const buffer = term.buffer.active;
            const maxDecos = 800;
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
    }, [clearSearchDecorations, clearSearchHitDecorations]);

    useEffect(() => {
        scheduleSearchHighlightAll(120, { visible: searchVisible, query: searchQuery, caseSensitive: searchCaseSensitive, regexMode: searchRegexMode });
    }, [scheduleSearchHighlightAll, searchVisible, searchQuery, searchCaseSensitive, searchRegexMode]);

    // --- Change 1: Flush writes once per animation frame ---
    const flushWrites = useCallback(() => {
        flushScheduledRef.current = false;
        const queue = writeQueueRef.current;
        if (queue.length === 0) return;
        const merged = queue.join('');
        writeQueueRef.current = [];
        xtermRef.current?.write(merged);
        lastOutputAtRef.current = Date.now();
    }, []);

    // --- Change 2: Schedule highlight scan only after output stabilizes ---
    const schedulePostOutputScan = useCallback(() => {
        if (postScanTimerRef.current) {
            window.clearTimeout(postScanTimerRef.current);
            postScanTimerRef.current = null;
        }
        postScanTimerRef.current = window.setTimeout(() => {
            postScanTimerRef.current = null;
            // Only scan if output has been stable for >= 300ms
            if (Date.now() - lastOutputAtRef.current >= 300) {
                scheduleHighlightScan(0);
            } else {
                // Output is still happening, reschedule
                schedulePostOutputScan();
            }
        }, 300);
    }, []);

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
            const start = Math.max(0, viewportY - 40);
            const end = Math.min(buffer.length - 1, viewportY + term.rows - 1 + 40);

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

            const budgetMs = 12;
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
                    if (totalDecos >= 1500) break;
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
                if (totalDecos >= 1500) break;
            }
        }, Math.max(0, delayMs));
    }, [clearDecorations]);

    // Helper function to sync terminal size to backend PTY
    const syncSizeToBackend = useCallback(() => {
        if (!sessionIDRef.current || !xtermRef.current) return;

        const cols = xtermRef.current.cols;
        const rows = xtermRef.current.rows;

        // Guard: skip unreasonably small sizes (e.g. 2x1 from a hidden terminal)
        if (cols < 5 || rows < 2) return;

        // @ts-ignore
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.ResizeTerminal) {
            // @ts-ignore
            window.go.main.App.ResizeTerminal(sessionIDRef.current, cols, rows);
        }
    }, []);

    const scheduleSizeSync = useCallback((delayMs = 10) => {
        if (sizeSyncTimerRef.current) {
            window.clearTimeout(sizeSyncTimerRef.current);
            sizeSyncTimerRef.current = null;
        }
        sizeSyncTimerRef.current = window.setTimeout(() => {
            sizeSyncTimerRef.current = null;
            syncSizeToBackend();
        }, delayMs);
    }, [syncSizeToBackend]);

    const isTerminalMeasurable = (container: HTMLDivElement) => {
        const style = window.getComputedStyle(container);
        if (style.display === 'none' || style.visibility === 'hidden') return false;

        const rect = container.getBoundingClientRect();
        return rect.width > 0 && rect.height > 0 && container.getClientRects().length > 0;
    };

    const hasUsableFitDimensions = (fitAddon: FitAddon) => {
        const dimensions = fitAddon.proposeDimensions();
        if (!dimensions) return false;
        return Number.isFinite(dimensions.cols)
            && Number.isFinite(dimensions.rows)
            && dimensions.cols >= 5
            && dimensions.rows >= 2;
    };

    const isAtBottom = (term: Terminal) => {
        const buffer = term.buffer.active;
        return Math.max(0, buffer.baseY - buffer.viewportY) <= 1;
    };

    const trackScrollPosition = (term: Terminal) => {
        const container = terminalRef.current;
        if (!container || !isTerminalMeasurable(container) || suppressScrollTrackingRef.current) return;
        stickToBottomRef.current = isAtBottom(term);
    };

    const withSuppressedScrollTracking = (fn: () => void) => {
        suppressScrollTrackingRef.current = true;
        try {
            fn();
        } finally {
            window.setTimeout(() => {
                suppressScrollTrackingRef.current = false;
            }, 0);
        }
    };

    const syncViewportDomToBottom = () => {
        const viewport = terminalRef.current?.querySelector<HTMLElement>('.xterm-viewport');
        if (!viewport) return;
        viewport.scrollTop = Math.max(0, viewport.scrollHeight - viewport.clientHeight);
    };

    const scrollToBottomAndRefresh = useCallback(() => {
        const term = xtermRef.current;
        if (!term) return;
        stickToBottomRef.current = true;
        withSuppressedScrollTracking(() => {
            term.scrollToBottom();
            syncViewportDomToBottom();
            term.refresh(0, Math.max(0, term.rows - 1));
            syncViewportDomToBottom();
        });
    }, []);

    const restoreViewport = (term: Terminal, bottomOffset: number, forceBottom: boolean) => {
        const buffer = term.buffer.active;
        withSuppressedScrollTracking(() => {
            if (forceBottom) {
                term.scrollToBottom();
                syncViewportDomToBottom();
                return;
            }
            term.scrollToLine(Math.max(0, buffer.baseY - bottomOffset));
        });
    };

    const runFitAndRefresh = useCallback((syncBackend = true) => {
        const container = terminalRef.current;
        const term = xtermRef.current;
        const fitAddon = fitAddonRef.current;
        if (!container || !term || !fitAddon || !isTerminalMeasurable(container)) return false;
        if (!hasUsableFitDimensions(fitAddon)) return false;

        const buffer = term.buffer.active;
        const bottomOffset = Math.max(0, buffer.baseY - buffer.viewportY);
        const shouldStickToBottom = stickToBottomRef.current;

        fitAddon.fit();
        restoreViewport(term, bottomOffset, shouldStickToBottom);
        term.refresh(0, Math.max(0, term.rows - 1));
        if (shouldStickToBottom) {
            syncViewportDomToBottom();
        }

        if (syncBackend) {
            scheduleSizeSync();
        }
        return true;
    }, [scheduleSizeSync]);

    const clearLayoutRestoreTimer = useCallback(() => {
        if (layoutRestoreRafRef.current !== null) {
            window.cancelAnimationFrame?.(layoutRestoreRafRef.current);
            layoutRestoreRafRef.current = null;
        }
        for (const timer of layoutRestoreTimerRefs.current) {
            window.clearTimeout(timer);
        }
        layoutRestoreTimerRefs.current = [];
    }, []);

    const restoreTerminalLayout = useCallback(() => {
        clearLayoutRestoreTimer();
        runFitAndRefresh(true);

        if (typeof window.requestAnimationFrame === 'function') {
            layoutRestoreRafRef.current = window.requestAnimationFrame(() => {
                layoutRestoreRafRef.current = null;
                runFitAndRefresh(true);
            });
        }
        layoutRestoreTimerRefs.current = [50, 150].map(delay => {
            const timer = window.setTimeout(() => {
                layoutRestoreTimerRefs.current = layoutRestoreTimerRefs.current.filter(t => t !== timer);
                runFitAndRefresh(true);
            }, delay);
            return timer;
        });
    }, [clearLayoutRestoreTimer, runFitAndRefresh]);

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
            // Change 1: Buffer writes, flush once per animation frame
            writeQueueRef.current.push(data);
            if (!flushScheduledRef.current) {
                flushScheduledRef.current = true;
                requestAnimationFrame(flushWrites);
            }
            // Change 2: Schedule post-output scan instead of immediate scan
            schedulePostOutputScan();
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
            restoreTerminalLayout();
        },
        prepareForExternalInput: () => {
            scrollToBottomAndRefresh();
            xtermRef.current?.focus();
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
            fontFamily: getTerminalFontStack(terminalConfig?.font_family),
            fontSize: clampTerminalFontSize(terminalConfig?.font_size),
            fontWeight: '400',
            fontWeightBold: '700',
            theme: {
                background: '#1e1e1e',
                // 双击选词/拖选选区背景色，沿用 VS Code 深色主题的深蓝，避免默认半透明色在深背景下不显眼（#43）
                selectionBackground: '#264f78',
                selectionInactiveBackground: '#1e3a5f',
            },
            // 扩展分隔符：加入终端常见分隔符 /:=| 等
            // 但不加 . 和 _，这样 sopuesr.iii_yuyu、my_var 等标识符保持为整体
            wordSeparator: ' ()[]{}\'\"`,;:/\\|=<>!@#$%^&*~',
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

        // 给终端画布加内边距，避免字符紧贴边缘
        const xtermEl = terminalRef.current.querySelector('.xterm');
        if (xtermEl instanceof HTMLElement) {
            xtermEl.style.padding = '4px 8px';
        }
        fitAddonRef.current = fitAddon;
        xtermRef.current = term;

        restoreTerminalLayout();
        scheduleSizeSync(100);
        scheduleHighlightScan(0);

        const onScrollDispose = term.onScroll(() => {
            trackScrollPosition(term);
            schedulePostOutputScan();
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
            stickToBottomRef.current = true;
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

        // 选区监听：解析时间戳并上报
        let selectionDebounceTimer: number | null = null;
        const onSelectionChangeDispose = term.onSelectionChange(() => {
            // 防抖 150ms，避免拖拽时频繁触发
            if (selectionDebounceTimer) {
                window.clearTimeout(selectionDebounceTimer);
            }
            selectionDebounceTimer = window.setTimeout(() => {
                selectionDebounceTimer = null;
                const selection = term.getSelection();
                if (!selection) {
                    // 清空时上报 null，让置顶栏隐藏
                    onSelectionParsedRef.current?.(null);
                    return;
                }
                const result = parseTimestamp(selection);
                onSelectionParsedRef.current?.(result);
            }, 150);
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
                if (arg.ctrlKey && !arg.altKey && !arg.metaKey) {
                    if (arg.code === 'Equal' || arg.code === 'NumpadAdd') {
                        arg.preventDefault();
                        adjustTerminalFontSize(current => current + 1);
                        return false;
                    }
                    if (arg.code === 'Minus' || arg.code === 'NumpadSubtract') {
                        arg.preventDefault();
                        adjustTerminalFontSize(current => current - 1);
                        return false;
                    }
                    if (arg.code === 'Digit0' || arg.code === 'Numpad0') {
                        arg.preventDefault();
                        adjustTerminalFontSize(DEFAULT_TERMINAL_FONT_SIZE);
                        return false;
                    }
                }
                if (getSearchEnabled() && arg.ctrlKey && arg.code === 'KeyF') {
                    arg.preventDefault();
                    const selection = term.getSelection();
                    if (!searchVisibleRef.current) {
                        searchVisibleRef.current = true;
                        setSearchVisible(true);
                        if (selection) {
                            const nextQuery = selection.slice(0, 200);
                            searchQueryRef.current = nextQuery;
                            setSearchQuery(nextQuery);
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
                scrollToBottomAndRefresh();
                currentInputRef.current += text;
                setCompletionVisible(false);
                completionVisibleRef.current = false;
                triggerCompletion();
                term.paste(text);
                schedulePostOutputScan();
            }
        };
        terminalRef.current.addEventListener('paste', handlePaste);

        // Middle click: paste selection directly if selected, otherwise paste from clipboard
        const handleAuxClick = (e: MouseEvent) => {
            if (e.button === 1) {
                e.preventDefault();
                const selection = term.getSelection();
                if (selection) {
                    scrollToBottomAndRefresh();
                    currentInputRef.current += selection;
                    setCompletionVisible(false);
                    completionVisibleRef.current = false;
                    triggerCompletion();
                    term.paste(selection);
                    schedulePostOutputScan();
                    term.clearSelection();
                    term.focus();
                } else {
                    navigator.clipboard.readText().then(text => {
                        if (text) {
                            scrollToBottomAndRefresh();
                            currentInputRef.current += text;
                            setCompletionVisible(false);
                            completionVisibleRef.current = false;
                            triggerCompletion();
                            term.paste(text);
                            schedulePostOutputScan();
                        }
                        term.focus();
                    });
                }
            }
        };
        terminalRef.current.addEventListener('auxclick', handleAuxClick);

        // Right click: copy selection to clipboard, or paste from clipboard
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
                        scrollToBottomAndRefresh();
                        currentInputRef.current += text;
                        setCompletionVisible(false);
                        completionVisibleRef.current = false;
                        triggerCompletion();
                        term.paste(text);
                        schedulePostOutputScan();
                    }
                    term.focus();
                });
            }
        };
        terminalRef.current.addEventListener('contextmenu', handleContextMenu);

        // Window resize
        const handleResize = () => {
            restoreTerminalLayout();
        };
        window.addEventListener('resize', handleResize);

        let resizeObserver: ResizeObserver | null = null;
        if (typeof ResizeObserver !== 'undefined') {
            resizeObserver = new ResizeObserver(() => {
                restoreTerminalLayout();
            });
            resizeObserver.observe(terminalRef.current);
        }

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
            if (sizeSyncTimerRef.current) {
                window.clearTimeout(sizeSyncTimerRef.current);
                sizeSyncTimerRef.current = null;
            }
            if (searchHighlightTimerRef.current) {
                window.clearTimeout(searchHighlightTimerRef.current);
                searchHighlightTimerRef.current = null;
            }
            clearLayoutRestoreTimer();
            resizeObserver?.disconnect();
            clearSearchDecorations();
            if (selectionDebounceTimer) {
                window.clearTimeout(selectionDebounceTimer);
                selectionDebounceTimer = null;
            }
            onSelectionChangeDispose.dispose();
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
        restoreTerminalLayout,
        scheduleSizeSync,
        clearLayoutRestoreTimer,
        clearSearchDecorations,
        scrollToBottomAndRefresh,
        scheduleHighlightScan,
        terminalConfig?.search_enabled,
        adjustTerminalFontSize,
    ]);

    useEffect(() => {
        const term = xtermRef.current;
        if (!term) return;

        const nextScrollback = terminalConfig?.scrollback || 5000;
        if (term.options.scrollback === nextScrollback) return;

        term.options.scrollback = nextScrollback;
        restoreTerminalLayout();
    }, [restoreTerminalLayout, terminalConfig?.scrollback]);

    useEffect(() => {
        const term = xtermRef.current;
        if (!term) return;

        const fontFamily = getTerminalFontStack(terminalConfig?.font_family);
        const fontSize = clampTerminalFontSize(terminalConfig?.font_size);
        if (term.options.fontFamily === fontFamily && term.options.fontSize === fontSize) return;

        term.options.fontFamily = fontFamily;
        term.options.fontSize = fontSize;
        pendingFontSizeRef.current = fontSize;
        restoreTerminalLayout();
    }, [restoreTerminalLayout, terminalConfig?.font_family, terminalConfig?.font_size]);

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
                onQueryChange={(v) => {
                    searchQueryRef.current = v;
                    setSearchQuery(v);
                }}
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
            {zoomIndicatorSize !== null && (
                <div
                    role="group"
                    aria-label="终端缩放"
                    style={zoomIndicatorStyles.container}
                >
                    <span role="status" aria-live="polite" style={zoomIndicatorStyles.current}>
                        字号 {zoomIndicatorSize}px
                    </span>
                    <span style={zoomIndicatorStyles.separator} aria-hidden="true" />
                    {zoomIndicatorSize === DEFAULT_TERMINAL_FONT_SIZE ? (
                        <span style={zoomIndicatorStyles.defaultLabel}>默认</span>
                    ) : (
                        <button
                            type="button"
                            style={zoomIndicatorStyles.resetButton}
                            onMouseDown={(event) => event.preventDefault()}
                            onClick={() => {
                                adjustTerminalFontSize(DEFAULT_TERMINAL_FONT_SIZE);
                                setZoomIndicatorSize(null);
                            }}
                            aria-label={`恢复默认字号 ${DEFAULT_TERMINAL_FONT_SIZE}px`}
                        >
                            恢复 {DEFAULT_TERMINAL_FONT_SIZE}px
                        </button>
                    )}
                </div>
            )}
        </div>
    );
});

const zoomIndicatorStyles: Record<string, React.CSSProperties> = {
    container: {
        position: 'absolute',
        right: '14px',
        bottom: '12px',
        zIndex: 20,
        display: 'flex',
        alignItems: 'center',
        gap: '9px',
        minHeight: '30px',
        padding: '0 10px',
        border: '1px solid #4a4a4a',
        borderRadius: '5px',
        backgroundColor: 'rgba(43, 43, 43, 0.96)',
        boxShadow: '0 4px 14px rgba(0, 0, 0, 0.35)',
        color: '#d4d4d4',
        fontSize: '12px',
        lineHeight: 1,
        userSelect: 'none',
    },
    current: {
        color: '#f0f0f0',
        fontWeight: 500,
    },
    separator: {
        width: '1px',
        height: '14px',
        backgroundColor: '#555555',
    },
    defaultLabel: {
        color: '#909090',
    },
    resetButton: {
        minHeight: '28px',
        padding: '0',
        border: 'none',
        background: 'transparent',
        color: '#4aa3df',
        font: 'inherit',
        cursor: 'pointer',
    },
};

export default TerminalComponent;
