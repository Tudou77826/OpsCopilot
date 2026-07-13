import { TerminalConfig } from './highlightTypes';

export const DEFAULT_TERMINAL_FONT_FAMILY = 'JetBrains Mono';
export const DEFAULT_TERMINAL_FONT_SIZE = 14;
export const MIN_TERMINAL_FONT_SIZE = 10;
export const MAX_TERMINAL_FONT_SIZE = 32;

export const TERMINAL_FONT_OPTIONS = [
    {
        value: 'JetBrains Mono',
        label: 'JetBrains Mono',
        description: '字符辨识度高，默认推荐',
        stack: "'JetBrains Mono', monospace",
    },
    {
        value: 'Fira Code',
        label: 'Fira Code',
        description: '紧凑清晰，适合高密度输出',
        stack: "'Fira Code', monospace",
    },
    {
        value: 'Source Code Pro',
        label: 'Source Code Pro',
        description: '字形舒展，长时间阅读友好',
        stack: "'Source Code Pro', monospace",
    },
    {
        value: 'IBM Plex Mono',
        label: 'IBM Plex Mono',
        description: '字形规整，工程感强',
        stack: "'IBM Plex Mono', monospace",
    },
    {
        value: 'Inconsolata',
        label: 'Inconsolata',
        description: '字面轻盈，适合窄窗口',
        stack: "'Inconsolata', monospace",
    },
] as const;

export const normalizeTerminalFontFamily = (fontFamily?: string) => {
    const value = fontFamily?.trim();
    return TERMINAL_FONT_OPTIONS.some(option => option.value === value)
        ? value as typeof TERMINAL_FONT_OPTIONS[number]['value']
        : DEFAULT_TERMINAL_FONT_FAMILY;
};

export const clampTerminalFontSize = (value?: number) => {
    const size = Number.isFinite(value) ? Math.round(value as number) : DEFAULT_TERMINAL_FONT_SIZE;
    return Math.min(MAX_TERMINAL_FONT_SIZE, Math.max(MIN_TERMINAL_FONT_SIZE, size));
};

export const normalizeTerminalConfig = (config?: TerminalConfig): TerminalConfig => ({
    scrollback: config?.scrollback && config.scrollback > 0 ? config.scrollback : 5000,
    search_enabled: config?.search_enabled ?? true,
    highlight_enabled: config?.highlight_enabled ?? true,
    font_family: normalizeTerminalFontFamily(config?.font_family),
    font_size: clampTerminalFontSize(config?.font_size),
});

export const getTerminalFontStack = (fontFamily?: string) => {
    const normalized = normalizeTerminalFontFamily(fontFamily);
    return TERMINAL_FONT_OPTIONS.find(option => option.value === normalized)?.stack
        || TERMINAL_FONT_OPTIONS[0].stack;
};
