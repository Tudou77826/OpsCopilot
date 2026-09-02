import { describe, expect, it } from 'vitest';
import {
    getTerminalFontStack,
    normalizeTerminalConfig,
    normalizeTerminalFontFamily,
} from './terminalAppearance';

describe('terminal appearance', () => {
    it('keeps bundled fonts and migrates system fonts to the default', () => {
        expect(normalizeTerminalFontFamily('Fira Code')).toBe('Fira Code');
        expect(normalizeTerminalFontFamily('Source Code Pro')).toBe('Source Code Pro');
        expect(normalizeTerminalFontFamily('IBM Plex Mono')).toBe('IBM Plex Mono');
        expect(normalizeTerminalFontFamily('Inconsolata')).toBe('Inconsolata');
        expect(normalizeTerminalFontFamily('Consolas')).toBe('JetBrains Mono');
    });

    it('uses the selected bundled font stack', () => {
        expect(getTerminalFontStack('Source Code Pro')).toContain('Source Code Pro');
        expect(getTerminalFontStack('Inconsolata')).toContain('Inconsolata');
        expect(normalizeTerminalConfig({
            scrollback: 5000,
            search_enabled: true,
            highlight_enabled: true,
            font_family: 'Cascadia Mono',
            font_size: 14,
        }).font_family).toBe('JetBrains Mono');
    });
});
