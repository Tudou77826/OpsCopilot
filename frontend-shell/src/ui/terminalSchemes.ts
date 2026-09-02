import { ITheme } from '@xterm/xterm';
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
