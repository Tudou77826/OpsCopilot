import { describe, expect, it } from 'vitest';
import css from './styles/shell-theme.css?raw';
import { getTerminalTheme } from './terminalSchemes';
import { btnPrimary, settingsCard } from './settings/settingsStyles';

function tokens(selector: string): Record<string, string> {
  const block = css.slice(css.indexOf(selector)).split('}')[0];
  return Object.fromEntries([...block.matchAll(/(--[\w-]+):\s*([^;]+);/g)].map(m => [m[1], m[2].trim()]));
}

function luminance(hex: string) {
  const rgb = hex.slice(1).match(/../g)!.map(v => parseInt(v, 16) / 255);
  const linear = rgb.map(v => v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4);
  return linear[0] * 0.2126 + linear[1] * 0.7152 + linear[2] * 0.0722;
}

function contrast(a: string, b: string) {
  const values = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (values[0] + 0.05) / (values[1] + 0.05);
}

describe('shared light theme contract', () => {
  const light = tokens(':root[data-theme="light"] {');

  it('uses the same foreground and canvas for xterm and surrounding UI', () => {
    const terminal = getTerminalTheme('light');
    expect(terminal.background).toBe(light['--bg-primary']);
    expect(terminal.foreground).toBe(light['--text-primary']);
    expect(terminal.cursorAccent).toBe(light['--bg-primary']);
  });

  it('keeps enabled text legible on every base surface', () => {
    for (const text of ['--text-primary', '--text-secondary', '--text-tertiary', '--text-muted']) {
      for (const bg of ['--bg-primary', '--bg-secondary', '--bg-tertiary', '--bg-elevated']) {
        expect(contrast(light[text], light[bg]), `${text} on ${bg}`).toBeGreaterThanOrEqual(4.5);
      }
    }
    expect(contrast(light['--text-on-accent'], light['--accent'])).toBeGreaterThanOrEqual(4.5);
  });

  it('provides light-specific logo, dialog and shadow tokens', () => {
    expect(light['--brand-logo-filter']).toContain('brightness(0)');
    expect(light['--bg-dialog']).toBe(light['--bg-elevated']);
    expect(light['--shadow-dialog']).toContain('0.16');
    expect(css).toContain('filter: var(--brand-logo-filter)');
  });

  it('retains the dark terminal palette and unfiltered dark logo', () => {
    expect(getTerminalTheme('dark').background).toBe('#1e1e1e');
    expect(tokens(':root {')['--brand-logo-filter']).toBe('none');
  });

  it('keeps the light canvas, dialog and terminal warm white', () => {
    for (const key of ['--bg-primary', '--bg-elevated', '--bg-dialog', '--bg-input']) {
      const [r, g, b] = light[key].slice(1).match(/../g)!.map(v => parseInt(v, 16));
      expect(r).toBeGreaterThan(g);
      expect(g).toBeGreaterThan(b);
      expect(b).toBeGreaterThanOrEqual(240);
      expect(r - b).toBeLessThanOrEqual(20);
    }
  });

  it('uses contrasting labels on primary buttons and elevated settings cards', () => {
    expect(btnPrimary.color).toBe('var(--text-on-accent)');
    expect(settingsCard.backgroundColor).toBe('var(--bg-dialog)');
  });
});
