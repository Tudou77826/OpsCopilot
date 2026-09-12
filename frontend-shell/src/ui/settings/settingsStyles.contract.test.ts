// 第 5 步（尺寸类令牌化）的验收：令牌化前后，导出样式对象里每个尺寸字段解析出的
// 像素值必须完全一致。这条用例在令牌化之前跑是绿的（字面量即取值），令牌化之后跑
// 仍是绿的（var() 解析回同名令牌的取值）——两次绿之间的差集就是"外观没变"的证据。
//
// 同时它还是三项防回归约束：刻度名字与取值必须一致（--space-N === Npx）、每个尺寸
// 字段必须走令牌、布局几何（内容限宽等）必须**不**被令牌化。皮肤要改的是密度与圆角，
// 不是布局宽度，最后一条挡住"顺手把 width 也令牌化"的诱惑。
import { describe, expect, it } from 'vitest';
import css from '../styles/shell-theme.css?raw';
import * as styles from './settingsStyles';

function tokens(selector: string): Record<string, string> {
  const block = css.slice(css.indexOf(selector)).split('}')[0];
  return Object.fromEntries([...block.matchAll(/(--[\w-]+):\s*([^;]+);/g)].map(m => [m[1], m[2].trim()]));
}

const root = tokens(':root {');

// 把 var(--x) 换成令牌取值；不是令牌的取值原样返回。只解析一层（这些令牌都不转发）。
function resolve(value: string): string {
  return value.replace(/var\((--[\w-]+)\)/g, (_, name: string) => root[name] ?? `«缺少令牌 ${name}»`);
}

// 令牌化之前各导出对象的尺寸字段取值，逐条抄自 settingsStyles.ts（含有意保留的字面量）。
const frozen: Array<[string, string, string]> = [
  ['inputStyle', 'padding', '8px 12px'],
  ['inputStyle', 'borderRadius', '4px'],
  ['inputStyle', 'fontSize', '13px'],
  ['btnPrimary', 'padding', '8px 16px'],
  ['btnPrimary', 'borderRadius', '4px'],
  ['btnPrimary', 'fontSize', '13px'],
  ['btnSecondary', 'padding', '8px 16px'],
  ['btnSecondary', 'borderRadius', '4px'],
  ['btnSecondary', 'fontSize', '13px'],
  ['btnSmall', 'padding', '4px 10px'],
  ['btnSmall', 'borderRadius', '4px'],
  ['btnSmall', 'fontSize', '12px'],
  ['btnDanger', 'padding', '4px 10px'],
  ['btnDanger', 'borderRadius', '4px'],
  ['btnDanger', 'fontSize', '12px'],
  ['btnGhost', 'padding', '6px 12px'],
  ['btnGhost', 'borderRadius', '4px'],
  ['btnGhost', 'fontSize', '12px'],
  ['sectionCard', 'padding', '16px'],
  ['sectionCard', 'borderRadius', '6px'],
  ['labelStyle', 'fontSize', '13px'],
  ['descStyle', 'fontSize', '12px'],
  ['sectionTitle', 'fontSize', '14px'],
  ['modalContainer', 'borderRadius', '8px'],
  ['modalHeader', 'padding', '16px 24px'],
  ['modalTitle', 'fontSize', '1.1rem'],
  ['modalCloseBtn', 'fontSize', '1.5rem'],
  ['modalCloseBtn', 'padding', '0'],
  ['modalCloseBtn', 'height', '32px'],
  ['modalCloseBtn', 'width', '32px'],
  ['modalCloseBtn', 'borderRadius', '4px'],
  ['pageContainer', 'padding', '20px 40px 32px'],
  ['pageContainer', 'gap', '24px'],
  ['pageContainer', 'maxWidth', '1100px'],
  ['pageHeader', 'gap', '6px'],
  ['pageHeader', 'paddingBottom', '18px'],
  ['pageTitle', 'fontSize', '1.6rem'],
  ['pageTitle', 'gap', '10px'],
  ['pageDesc', 'fontSize', '13px'],
  ['pageDesc', 'maxWidth', '720px'],
  ['settingsCard', 'borderRadius', '8px'],
  ['settingsCard', 'padding', '24px 28px'],
  ['settingsCard', 'gap', '4px'],
  ['cardTitle', 'fontSize', '14px'],
  ['cardTitle', 'marginBottom', '6px'],
  ['settingRow', 'gap', '24px'],
  ['settingRow', 'padding', '12px 0'],
  ['settingRowLeft', 'width', '320px'],
  ['settingRowLeft', 'gap', '4px'],
  ['settingRowRight', 'gap', '10px'],
  ['navGroupTitle', 'margin', '0 12px 6px'],
  ['navGroupTitle', 'fontSize', '11px'],
  ['navGroupTitle', 'letterSpacing', '0.08em'],
  ['navItem', 'gap', '10px'],
  ['navItem', 'padding', '8px 12px'],
  ['navItem', 'fontSize', '13px'],
  ['navItem', 'borderRadius', '6px'],
  ['cardDivider', 'margin', '8px 0'],
  ['cardDivider', 'height', '1'],
  ['inputWide', 'padding', '8px 12px'],
  ['inputWide', 'borderRadius', '4px'],
  ['inputWide', 'fontSize', '13px'],
  ['inputWide', 'minWidth', '240px'],
  ['inputWide', 'maxWidth', '520px'],
];

// 间距刻度：像素值写在名字里，名字与取值必须一致，否则"按刻度换算密度"会算错。
const spaceScale: Record<string, string> = {
  '--space-2': '2px', '--space-4': '4px', '--space-5': '5px', '--space-6': '6px', '--space-7': '7px',
  '--space-8': '8px', '--space-10': '10px', '--space-12': '12px', '--space-14': '14px', '--space-16': '16px',
  '--space-18': '18px', '--space-20': '20px', '--space-24': '24px', '--space-28': '28px', '--space-32': '32px',
  '--space-34': '34px', '--space-40': '40px', '--space-48': '48px', '--space-50': '50px',
};

const nonTokenizable = ['width', 'minWidth', 'maxWidth', 'minHeight', 'maxHeight', 'lineHeight', 'letterSpacing', 'fontWeight'];
const spacingProps = ['padding', 'paddingTop', 'paddingBottom', 'paddingLeft', 'paddingRight', 'margin', 'marginTop', 'marginBottom', 'marginLeft', 'marginRight', 'gap', 'rowGap', 'columnGap', 'height', 'borderRadius', 'fontSize'];

// 有意不令牌化的取值：纯 0、负偏移（布局对齐）、viewport 与百分比（布局）。出现别的
// 字面量就说明漏了令牌。
function tolerable(value: string): boolean {
  const bare = value.replace(/var\(--[\w-]+\)/g, '').replace(/0(\.0+)?/g, '');
  if (!/\d/.test(bare)) return true;                              // '0'、'0 auto'、''
  return /vh|vw|%|auto|calc/.test(bare) || /-\d/.test(bare);       // 布局取值与负偏移
}

function exportedStyleObjects(): Array<[string, Record<string, unknown>]> {
  return Object.entries(styles as unknown as Record<string, unknown>)
    .filter(([name, value]) => typeof value === 'object' && value !== null && name !== 'colors' && name !== 'radius' && name !== 'font')
    .map(([name, value]) => [name, value as Record<string, unknown>]);
}

describe('尺寸令牌刻度', () => {
  it('间距刻度的名字与取值一致', () => {
    const failures = Object.entries(spaceScale)
      .filter(([name, expected]) => root[name] !== expected)
      .map(([name, expected]) => `${name}: 实际 ${root[name] ?? '未声明'}，期望 ${expected}`);
    expect(failures.join('\n')).toBe('');
  });

  it('圆角与字号刻度覆盖到使用者', () => {
    expect(root['--radius-xs']).toBe('3px');
    expect(root['--radius-sm']).toBe('4px');
    expect(root['--radius-md']).toBe('6px');
    expect(root['--radius-lg']).toBe('8px');
    expect(root['--radius-full']).toBe('20px');
    expect(root['--radius-circle']).toBe('50%');
    expect(root['--font-size-xs']).toBe('11px');
    expect(root['--font-size-sm']).toBe('12px');
    expect(root['--font-size-base']).toBe('13px');
    expect(root['--font-size-lg']).toBe('14px');
    expect(root['--font-size-md']).toBe('16px');
    expect(root['--font-size-2xl']).toBe('20px');
    expect(root['--font-size-modal-title']).toBe('1.1rem');
    expect(root['--font-size-modal-close']).toBe('1.5rem');
    expect(root['--font-size-page-title']).toBe('1.6rem');
  });

  it('尺寸刻度不与颜色令牌混在亮色覆写块里（两种模式共用一套尺寸）', () => {
    // 尺寸令牌只应在 :root 声明。落在亮色块里意味着亮色下密度会变，本步不打算这样。
    const light = tokens(':root[data-theme="light"] {');
    const leaked = Object.keys(light).filter(name => /^--(space|radius|font-size)-/.test(name));
    expect(leaked.join('\n')).toBe('');
  });
});

describe('导出样式的尺寸字段', () => {
  it('解析出的像素值与令牌化之前完全一致', () => {
    const failures: string[] = [];
    for (const [name, prop, expected] of frozen) {
      const style = (styles as unknown as Record<string, Record<string, unknown>>)[name];
      const actual = resolve(String(style[prop]));
      if (actual !== expected) failures.push(`${name}.${prop}: ${actual} ≠ ${expected}`);
    }
    expect(failures.join('\n')).toBe('');
  });

  it('每个尺寸字段都走令牌（只有 0 与负偏移可以是字面量）', () => {
    const failures: string[] = [];
    for (const [name, style] of exportedStyleObjects()) {
      for (const prop of spacingProps) {
        const value = style[prop];
        if (typeof value !== 'string') continue;
        if (value.includes('var(--')) continue;
        if (tolerable(value)) continue;
        failures.push(`${name}.${prop}: ${value}`);
      }
    }
    expect(failures.join('\n')).toBe('');
  });

  it('布局几何不被令牌化（皮肤改的是密度与圆角，不是布局宽度）', () => {
    const failures: string[] = [];
    for (const [name, style] of exportedStyleObjects()) {
      for (const prop of nonTokenizable) {
        const value = style[prop];
        if (typeof value !== 'string') continue;
        if (value.includes('var(--')) failures.push(`${name}.${prop} 不应令牌化：${value}`);
      }
    }
    expect(failures.join('\n')).toBe('');
  });
});
