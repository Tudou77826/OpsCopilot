import { ITheme } from '@xterm/xterm';
import { ISearchDecorationOptions } from '@xterm/addon-search';
import { Theme } from './appearanceTypes';

/**
 * 终端完整配色方案。每个方案都必须显式提供 foreground / cursor / cursorAccent /
 * selectionBackground / 16 ANSI + 16 bright ANSI —— xterm.js 缺省回退的是暗色调色板，
 * 只换 background 不补全这些字段会导致「白底白字」「亮黄不可读」等问题。
 *
 * dark：基于历史背景 #1e1e1e，补 VS Code Dark+ 取向的 16 色。
 * light：One Half Light（Windows Terminal/GitHub 风格），yellow 已调暗为 #C18401。
 * 取值来源见调研报告（windowsterminalthemes.dev / xterm.js ITheme typings）。
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
    // ANSI 16 色：One Half Light 取值，yellow 用 #C18401（纯黄在白底不可读）
    black: '#383a42',
    red: '#e45649',
    green: '#50a14f',
    yellow: '#c18401',
    blue: '#0184bc',
    magenta: '#a626a4',
    cyan: '#0997b3',
    white: '#faf8f3',
    brightBlack: '#4f525e',
    brightRed: '#e06c75',
    brightGreen: '#3fb950',
    brightYellow: '#b08800',
    brightBlue: '#218bff',
    brightMagenta: '#a626a4',
    brightCyan: '#39c5cf',
    brightWhite: '#ffffff',
};

export const terminalSchemes: Record<Theme, ITheme> = {
    dark: darkScheme,
    light: lightScheme,
};

/** 返回当前主题对应的终端配色方案 */
export const getTerminalTheme = (theme: Theme): ITheme => terminalSchemes[theme] ?? darkScheme;

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
