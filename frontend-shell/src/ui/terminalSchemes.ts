import { ITheme } from '@xterm/xterm';
import { ISearchDecorationOptions } from '@xterm/addon-search';
import { Theme } from './appearanceTypes';

/**
 * 终端最小对比度：与 VS Code 默认值对齐（WCAG AA 4.5:1），亮暗模式统一启用。
 * xterm.js 渲染器在每个 cell 上实时保障前景色与背景色可读，避免白底白字、
 * 256色/真彩色浅字或搜索高亮叠在彩色 ANSI 文字上糊成一片。
 */
export const TERMINAL_MINIMUM_CONTRAST_RATIO = 4.5;

/**
 * 终端完整配色方案。每个方案都必须显式提供 foreground / cursor / cursorAccent /
 * selectionBackground / 16 ANSI + 16 bright ANSI —— xterm.js 缺省回退的是暗色调色板，
 * 只换 background 不补全这些字段会导致「白底白字」「亮黄不可读」等问题。
 *
 * dark：基于历史背景 #1e1e1e，补 VS Code Dark+ 取向的 16 色。
 * light：暖白画布 + WCAG AA 校准的 16 ANSI 色（确保不依赖强行拉色也能自然可读）。
 */
const darkScheme: ITheme = {
    background: '#1e1e1e',
    foreground: '#d4d4d4',
    cursor: '#ffffff',
    cursorAccent: '#1e1e1e',
    selectionBackground: '#264f78',
    selectionInactiveBackground: '#1e3a5f',
    scrollbarSliderBackground: '#444444',
    scrollbarSliderHoverBackground: '#666666',
    scrollbarSliderActiveBackground: '#666666',
    overviewRulerBorder: '#1e1e1e',
    black: '#000000',
    red: '#f44747',
    green: '#6a9955',
    yellow: '#d7ba7d',
    blue: '#569cd6',
    magenta: '#c586c0',
    cyan: '#4ec9b0',
    white: '#d4d4d4',
    brightBlack: '#808080',
    brightRed: '#f48771',
    brightGreen: '#89d185',
    brightYellow: '#ffd700',
    brightBlue: '#6cb6ff',
    brightMagenta: '#d6acec',
    brightCyan: '#67e8e8',
    brightWhite: '#ffffff',
};

const lightScheme: ITheme = {
    background: '#faf8f3',
    foreground: '#39362f',
    cursor: '#554e43',
    cursorAccent: '#faf8f3',
    selectionBackground: '#e6d7bb',
    selectionInactiveBackground: '#eee6d8',
    scrollbarSliderBackground: '#bcaf99',
    scrollbarSliderHoverBackground: '#9a896e',
    scrollbarSliderActiveBackground: '#86765e',
    overviewRulerBorder: '#d9d0c0',
    // ANSI 16 色：针对暖白/冷白画布校准至 ≥ 4.5:1，避免 white/brightWhite 隐形及青/绿/黄过浅
    black: '#383a42',
    red: '#c93b2b',
    green: '#2b7a3d',
    yellow: '#8f6200',
    blue: '#0068a8',
    magenta: '#8e248c',
    cyan: '#00738a',
    white: '#6b655c',
    brightBlack: '#4f525e',
    brightRed: '#c43e48',
    brightGreen: '#1e7e34',
    brightYellow: '#825d00',
    brightBlue: '#0969da',
    brightMagenta: '#8250df',
    brightCyan: '#047885',
    brightWhite: '#39362f',
};

export const terminalSchemes: Record<Theme, ITheme> = {
    dark: darkScheme,
    light: lightScheme,
};

const HEX_6_RE = /^#[0-9a-f]{6}$/i;

const readCssHex = (computed: CSSStyleDeclaration, token: string): string | undefined => {
    const raw = computed.getPropertyValue(token).trim();
    if (HEX_6_RE.test(raw)) return raw;
    if (/^#[0-9a-f]{3}$/i.test(raw)) {
        return `#${raw[1]}${raw[1]}${raw[2]}${raw[2]}${raw[3]}${raw[3]}`;
    }
    return undefined;
};

/**
 * 返回当前主题对应的终端配色方案。
 * 画布/前景/滚动条优先从当前 DOM 根元素的语义令牌读取，使终端与 default/windows/teams
 * 皮肤保持零色差；当请求的 theme 与当前 DOM 不一致或样式表未加载（如单测）时回退静态兜底表。
 */
export const getTerminalTheme = (theme: Theme, root?: HTMLElement): ITheme => {
    const base = terminalSchemes[theme] ?? darkScheme;
    if (typeof document === 'undefined' || typeof getComputedStyle !== 'function') {
        return base;
    }
    const target = root ?? document.documentElement;
    const domTheme = (target.dataset.theme ?? '').trim().toLowerCase();
    const effectiveDomTheme: Theme = domTheme === 'light' ? 'light' : 'dark';
    if (effectiveDomTheme !== theme) {
        return base;
    }
    try {
        const computed = getComputedStyle(target);
        const bg = readCssHex(computed, '--bg-primary');
        const fg = readCssHex(computed, '--text-primary');
        const thumb = readCssHex(computed, '--app-scrollbar-thumb');
        const thumbHover = readCssHex(computed, '--app-scrollbar-thumb-hover');
        const border = readCssHex(computed, '--border');
        if (!bg && !fg && !thumb && !thumbHover && !border) {
            return base;
        }
        return {
            ...base,
            ...(bg ? { background: bg, cursorAccent: bg } : {}),
            ...(fg ? { foreground: fg } : {}),
            ...(thumb ? { scrollbarSliderBackground: thumb } : {}),
            ...(thumbHover ? { scrollbarSliderHoverBackground: thumbHover, scrollbarSliderActiveBackground: thumbHover } : {}),
            ...(border ? { overviewRulerBorder: border } : {}),
        };
    } catch {
        return base;
    }
};

/**
 * 终端搜索命中的高亮配色，随主题切换。
 * 这也是 xterm 的配色数据（需要具体色值，不能用 CSS 变量），所以与终端配色放在同一处。
 * 暗色那套（橄榄底 + 琥珀边）在亮色终端上不可读，故亮色另给一套浅底深边。
 */
export const terminalSearchDecorations: Record<Theme, ISearchDecorationOptions> = {
    dark: {
        matchBackground: '#665c00',
        matchBorder: '#d7ba00',
        matchOverviewRuler: '#d7ba00',
        // 亮琥珀底配浅灰前景只有 1.45，当前匹配基本读不出来；改深琥珀底 + 亮边框定位。
        activeMatchBackground: '#7a4a00',
        activeMatchBorder: '#ffd75f',
        activeMatchColorOverviewRuler: '#f59e0b',
    },
    light: {
        matchBackground: '#ffe08a',
        matchBorder: '#8a5a00',
        matchOverviewRuler: '#8a5a00',
        activeMatchBackground: '#f0a020',
        activeMatchBorder: '#5c3a00',
        activeMatchColorOverviewRuler: '#8a5a00',
    },
};

export const getTerminalSearchDecorations = (theme: Theme): ISearchDecorationOptions =>
    terminalSearchDecorations[theme] ?? terminalSearchDecorations.dark;

/** 新建高亮规则的默认前景/背景，随主题切换（原先写死深蓝底白字，亮色终端下突兀）。 */
export const terminalHighlightDefaults: Record<Theme, { background_color: string; color: string }> = {
    dark: { background_color: '#1d3a5a', color: '#ffffff' },
    light: { background_color: '#fff3c4', color: '#5c4700' },
};

export const getTerminalHighlightDefaults = (theme: Theme) =>
    terminalHighlightDefaults[theme] ?? terminalHighlightDefaults.dark;

// —— 规则高亮对比度归一化（单一机制：用户配一套语义色，亮暗主题自动可读）——

const parseHex6 = (value?: string): string | undefined =>
    value && HEX_6_RE.test(value) ? value.toLowerCase() : undefined;

const hexToRgb = (hex: string): [number, number, number] => [
    parseInt(hex.slice(1, 3), 16),
    parseInt(hex.slice(3, 5), 16),
    parseInt(hex.slice(5, 7), 16),
];

const rgbToHex = (r: number, g: number, b: number): string =>
    '#' + [r, g, b]
        .map(v => Math.max(0, Math.min(255, Math.round(v))).toString(16).padStart(2, '0'))
        .join('');

const relativeLuminance = (hex: string): number => {
    const [r, g, b] = hexToRgb(hex).map(v => {
        const s = v / 255;
        return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
    });
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
};

export const contrastRatio = (a: string, b: string): number => {
    const la = relativeLuminance(a);
    const lb = relativeLuminance(b);
    return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
};

/**
 * 调整前景色亮度直到与目标背景对比度达到 minRatio（默认 4.5），尽量保留原色相。
 */
export const ensureContrastColor = (
    fgHex: string,
    bgHex: string,
    minRatio = TERMINAL_MINIMUM_CONTRAST_RATIO,
): string => {
    if (contrastRatio(fgHex, bgHex) >= minRatio) return fgHex;
    const bgLum = relativeLuminance(bgHex);
    const [r, g, b] = hexToRgb(fgHex);
    const darken = bgLum > 0.5;
    let lo = 0;
    let hi = 1;
    let best = darken ? '#000000' : '#ffffff';
    for (let i = 0; i < 16; i++) {
        const mid = (lo + hi) / 2;
        const candidate = darken
            ? rgbToHex(r * (1 - mid), g * (1 - mid), b * (1 - mid))
            : rgbToHex(r + (255 - r) * mid, g + (255 - g) * mid, b + (255 - b) * mid);
        if (contrastRatio(candidate, bgHex) >= minRatio) {
            best = candidate;
            hi = mid;
        } else {
            lo = mid;
        }
    }
    return best;
};

/**
 * 将高亮规则样式归一化为当前主题下可读的装饰器色值：
 * - 无自定义背景色：校验文字与当前终端背景的对比度，不足 4.5 自动压暗/提亮；
 * - 有自定义背景色：保留背景色，校验文字与规则背景的对比度，不足 4.5 自动校正文字色。
 */
export const resolveRuleHighlightColors = (
    style: { background_color?: string; color?: string } | undefined,
    theme: Theme,
    terminalBg?: string,
): { backgroundColor?: string; foregroundColor?: string } => {
    const backgroundColor = parseHex6(style?.background_color);
    const rawForeground = parseHex6(style?.color);
    if (!rawForeground) {
        return { backgroundColor, foregroundColor: undefined };
    }
    const baseBg = backgroundColor ?? parseHex6(terminalBg) ?? (terminalSchemes[theme]?.background ?? darkScheme.background!);
    const foregroundColor = ensureContrastColor(rawForeground, baseBg, TERMINAL_MINIMUM_CONTRAST_RATIO);
    return { backgroundColor, foregroundColor };
};
