import React, { useEffect, useRef, forwardRef, useImperativeHandle, useState, useCallback } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import CompletionOverlay, { CompletionData, CompletionSuggestion } from './CompletionOverlay';
import SearchPanel from './SearchPanel';
import { HighlightRule, TerminalConfig } from './highlightTypes';
import {
    DEFAULT_TERMINAL_FONT_SIZE,
    clampTerminalFontSize,
    getTerminalFontStack,
} from './terminalAppearance';
import { parseTimestamp, TimestampResult } from '../../utils/timestampParser';
import { SearchController } from './search/SearchController';
import { RuleHighlightController } from './highlight/RuleHighlightController';

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
    const [searchWholeWord, setSearchWholeWord] = useState(false);
    const [searchCountText, setSearchCountText] = useState('');
    const [searchErrorText, setSearchErrorText] = useState('');
    const [searchComposing, setSearchComposing] = useState(false);
    const [zoomIndicatorSize, setZoomIndicatorSize] = useState<number | null>(null);
    const searchControllerRef = useRef<SearchController | null>(null);
    const ruleHighlightControllerRef = useRef<RuleHighlightController | null>(null);
    const searchDebounceTimerRef = useRef<number | null>(null);
    const searchInputRef = useRef<HTMLInputElement>(null);
    const searchVisibleRef = useRef(false);
    const searchQueryRef = useRef('');
    const searchCaseSensitiveRef = useRef(false);
    const searchRegexModeRef = useRef(false);
    const searchWholeWordRef = useRef(false);

    // Write buffering refs (Change 1: frame-level flush)
    const writeQueueRef = useRef<string[]>([]);
    const flushScheduledRef = useRef<boolean>(false);

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
        searchWholeWordRef.current = searchWholeWord;
    }, [searchQuery, searchVisible, searchCaseSensitive, searchRegexMode, searchWholeWord]);

    useEffect(() => {
        onDataRef.current = onData;
        onSelectionParsedRef.current = onSelectionParsed;
        sessionIDRef.current = sessionID;
        completionDelayRef.current = completionDelay;
        terminalConfigRef.current = terminalConfig;
        onFontSizeChangeRef.current = onFontSizeChange;
        pendingFontSizeRef.current = clampTerminalFontSize(terminalConfig?.font_size);
        highlightRulesRef.current = highlightRules;
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

    const closeSearch = useCallback(() => {
        searchVisibleRef.current = false;
        searchQueryRef.current = '';
        setSearchVisible(false);
        setSearchQuery('');
        setSearchCountText('');
        setSearchErrorText('');
        if (searchDebounceTimerRef.current !== null) {
            window.clearTimeout(searchDebounceTimerRef.current);
            searchDebounceTimerRef.current = null;
        }
        searchControllerRef.current?.clear();
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

    const doSearchNext = useCallback(() => {
        const controller = searchControllerRef.current;
        const query = searchQueryRef.current;
        if (!controller || query.length === 0 || searchErrorText) return;
        controller.findNext(query, {
            caseSensitive: searchCaseSensitiveRef.current,
            regex: searchRegexModeRef.current,
            wholeWord: searchWholeWordRef.current,
        });
        searchInputRef.current?.focus();
    }, [searchErrorText]);

    const doSearchPrev = useCallback(() => {
        const controller = searchControllerRef.current;
        const query = searchQueryRef.current;
        if (!controller || query.length === 0 || searchErrorText) return;
        controller.findPrevious(query, {
            caseSensitive: searchCaseSensitiveRef.current,
            regex: searchRegexModeRef.current,
            wholeWord: searchWholeWordRef.current,
        });
        searchInputRef.current?.focus();
    }, [searchErrorText]);

    useEffect(() => {
        if (searchDebounceTimerRef.current !== null) window.clearTimeout(searchDebounceTimerRef.current);
        if (!searchVisible || searchQuery.length === 0) {
            setSearchErrorText('');
            setSearchCountText('');
            searchControllerRef.current?.clear();
            return;
        }
        if (searchComposing) return;
        if (searchRegexMode) {
            try {
                new RegExp(searchQuery);
            } catch {
                setSearchErrorText('正则表达式无效');
                setSearchCountText('');
                searchControllerRef.current?.clear();
                return;
            }
        }
        setSearchErrorText('');
        searchDebounceTimerRef.current = window.setTimeout(() => {
            searchDebounceTimerRef.current = null;
            searchControllerRef.current?.findNext(searchQuery, {
                caseSensitive: searchCaseSensitive,
                regex: searchRegexMode,
                wholeWord: searchWholeWord,
            }, true);
        }, 80);
        return () => {
            if (searchDebounceTimerRef.current !== null) {
                window.clearTimeout(searchDebounceTimerRef.current);
                searchDebounceTimerRef.current = null;
            }
        };
    }, [searchCaseSensitive, searchComposing, searchQuery, searchRegexMode, searchVisible, searchWholeWord]);

    // --- Change 1: Flush writes once per animation frame ---
    const flushWrites = useCallback(() => {
        flushScheduledRef.current = false;
        const queue = writeQueueRef.current;
        if (queue.length === 0) return;
        const merged = queue.join('');
        writeQueueRef.current = [];
        xtermRef.current?.write(merged);
    }, []);

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
            searchControllerRef.current = new SearchController(term, results => {
                if (results.limitReached) {
                    setSearchCountText(`${results.resultCount}+`);
                } else if (results.resultCount === 0) {
                    setSearchCountText('0');
                } else {
                    const current = results.resultIndex >= 0 ? results.resultIndex + 1 : '?';
                    setSearchCountText(`${current} / ${results.resultCount}`);
                }
            });
        } else {
            searchControllerRef.current = null;
        }

        term.open(terminalRef.current);

        // 给终端画布加内边距，避免字符紧贴边缘
        const xtermEl = terminalRef.current.querySelector('.xterm');
        if (xtermEl instanceof HTMLElement) {
            xtermEl.style.padding = '4px 8px';
        }
        fitAddonRef.current = fitAddon;
        xtermRef.current = term;
        const ruleController = new RuleHighlightController(term);
        ruleHighlightControllerRef.current = ruleController;
        ruleController.setRules(highlightRulesRef.current, terminalConfigRef.current?.highlight_enabled ?? true);

        const syncRuleControllerVisibility = () => {
            const container = terminalRef.current;
            ruleController.setVisible(container !== null && !document.hidden && isTerminalMeasurable(container));
        };
        const handleVisibilityChange = () => {
            syncRuleControllerVisibility();
            if (!document.hidden) restoreTerminalLayout();
        };
        document.addEventListener('visibilitychange', handleVisibilityChange);

        restoreTerminalLayout();
        scheduleSizeSync(100);

        const onScrollDispose = term.onScroll(() => {
            trackScrollPosition(term);
            ruleController.schedule('scroll');
        });

        const onWriteParsedDispose = term.onWriteParsed(() => {
            ruleController.schedule('output', 32);
        });

        let lastCols = term.cols;
        const onResizeDispose = term.onResize(({ cols }) => {
            if (cols !== lastCols) {
                lastCols = cols;
                ruleController.invalidate();
            }
            ruleController.schedule('resize');
        });

        const onBufferChangeDispose = term.buffer.onBufferChange(() => {
            ruleController.invalidate();
            ruleController.schedule('buffer');
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
                        window.setTimeout(() => {
                            searchInputRef.current?.focus();
                            searchInputRef.current?.select();
                        }, 0);
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
                syncRuleControllerVisibility();
                restoreTerminalLayout();
            });
            resizeObserver.observe(terminalRef.current);
        }

        return () => {
            if (debounceTimerRef.current) {
                clearTimeout(debounceTimerRef.current);
            }
            if (searchDebounceTimerRef.current !== null) {
                window.clearTimeout(searchDebounceTimerRef.current);
                searchDebounceTimerRef.current = null;
            }
            if (sizeSyncTimerRef.current) {
                window.clearTimeout(sizeSyncTimerRef.current);
                sizeSyncTimerRef.current = null;
            }
            clearLayoutRestoreTimer();
            resizeObserver?.disconnect();
            if (selectionDebounceTimer) {
                window.clearTimeout(selectionDebounceTimer);
                selectionDebounceTimer = null;
            }
            onSelectionChangeDispose.dispose();
            onScrollDispose.dispose();
            onWriteParsedDispose.dispose();
            onResizeDispose.dispose();
            onBufferChangeDispose.dispose();
            searchControllerRef.current?.dispose();
            searchControllerRef.current = null;
            ruleController.dispose();
            ruleHighlightControllerRef.current = null;
            window.removeEventListener('resize', handleResize);
            document.removeEventListener('visibilitychange', handleVisibilityChange);
            terminalRef.current?.removeEventListener('paste', handlePaste);
            terminalRef.current?.removeEventListener('auxclick', handleAuxClick);
            terminalRef.current?.removeEventListener('contextmenu', handleContextMenu);
            term.dispose();
        };
    }, [
        fetchCompletions,
        handleCompletionSelect,
        handleNavigate,
        restoreTerminalLayout,
        scheduleSizeSync,
        clearLayoutRestoreTimer,
        scrollToBottomAndRefresh,
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
        ruleHighlightControllerRef.current?.setRules(
            highlightRules,
            terminalConfig?.highlight_enabled ?? true
        );
    }, [highlightRules, terminalConfig?.highlight_enabled]);

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
                wholeWord={searchWholeWord}
                onWholeWordChange={setSearchWholeWord}
                onCompositionChange={setSearchComposing}
                matchText={searchCountText}
                errorText={searchErrorText}
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
