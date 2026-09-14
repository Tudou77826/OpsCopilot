import { describe, expect, it } from 'vitest';
import css from './styles/shell-theme.css?raw';
import hostContract from './styles/teams-host-contract.json';
import { getTerminalTheme, getTerminalSearchDecorations, terminalHighlightDefaults } from './terminalSchemes';
import { currentTheme, normalizeSkin, applySkin, currentSkin, persistSkin, readPersistedSkin, DEFAULT_SKIN, SKIN_STORAGE_KEY, THEME_STORAGE_KEY, DEFAULT_THEME_FOLLOW, THEME_FOLLOW_STORAGE_KEY, normalizeThemeFollow, persistThemeFollow, readPersistedThemeFollow, hostTheme, observeHostTheme, readPersistedSkinChoice, hostTokensAvailable } from './appearance';
import { btnPrimary, settingsCard } from './settings/settingsStyles';

function tokens(selector: string): Record<string, string> {
  const block = css.slice(css.indexOf(selector)).split('}')[0];
  return Object.fromEntries([...block.matchAll(/(--[\w-]+):\s*([^;]+);/g)].map(m => [m[1], m[2].trim()]));
}

function rgbOf(hex: string): [number, number, number] {
  // 令牌里既有 #fff 也有 #ffffff（含 8 位带 alpha），统一展开成 6 位再算。
  const digits = hex.slice(1);
  const expanded = digits.length <= 4 ? digits.replace(/./g, (char) => char + char) : digits;
  return expanded.match(/../g)!.slice(0, 3).map(v => parseInt(v, 16)) as [number, number, number];
}

function luminance(hex: string) {
  const rgb = rgbOf(hex).map(v => v / 255);
  const linear = rgb.map(v => v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4);
  return linear[0] * 0.2126 + linear[1] * 0.7152 + linear[2] * 0.0722;
}

function contrast(a: string, b: string) {
  const values = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (values[0] + 0.05) / (values[1] + 0.05);
}

// --layer-* 是从文字色派生的半透明叠加层，不是实心表面：判断其上的文字对比度必须先按
// alpha 合成到具体底色上。只认 color-mix(in srgb, var(--x) N%, transparent) 这一种写法，
// 出现别的写法就报"无法计算"，逼着后来者显式扩展这个函数而不是悄悄漏测。
function composite(layerValue: string, backdropHex: string, palette: Record<string, string>): string | null {
  const match = layerValue.match(/^color-mix\(in srgb, var\((--[\w-]+)\)\s+([\d.]+)%, transparent\)$/);
  if (!match) return null;
  const base = palette[match[1]];
  if (!base || !/^#[0-9a-fA-F]{3,8}$/.test(base)) return null;
  const alpha = Number(match[2]) / 100;
  const from = rgbOf(base);
  const under = rgbOf(backdropHex);
  return '#' + from.map((v, index) => Math.round(v * alpha + under[index] * (1 - alpha)).toString(16).padStart(2, '0')).join('');
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

// 原来的对比度断言只抽样 4 组文字/背景。这里把两种模式下的语义组合枚举出来，组合取自
// 组件里真实存在的叠放关系（按钮底色配反色文字、状态色当文字用在面板上等），
// 新增令牌或调整取值时对比度会跟着被检查，不依赖人去记（见 docs/theme-design.md 8.3）。
const defaultDark = tokens(':root {');
const defaultLight = { ...defaultDark, ...tokens(':root[data-theme="light"] {') };
// 自带调色板的皮肤（windows）按模式各写一块，合成方式与模式层一致：
// 以该模式的默认取值为底，再叠加皮肤块（暗色那份多一个属性、权重更高）。
const windowsLight = { ...defaultLight, ...tokens(':root[data-skin="windows"] {') };
const windowsDark = { ...defaultDark, ...tokens(':root[data-skin="windows"] {'), ...tokens(':root[data-theme="dark"][data-skin="windows"] {') };
const palettes: Array<[string, Record<string, string>]> = [
  ['暗色', defaultDark],
  // 亮色块是覆写层：未在亮色块声明的令牌应从 :root 继承（如 mode-independent 的 --layer-*），
  // 只解析单块会把它们读成空值，等于漏测。
  ['亮色', defaultLight],
  // 皮肤不是"另一个主题"，而是同一套轴线上的另一组取值，所以对比度矩阵必须覆盖它——
  // 手写调色板最容易出的问题就是某个文字/底色配对比度不够。
  ['windows 亮色', windowsLight],
  ['windows 暗色', windowsDark],
];

// 只含模式层。下面两条断言问的是「模式层是否自足」与「镜像是否等于本体」，
// 皮肤会覆写令牌，把皮肤层混进来等于问错了对象。
const modePalettes = palettes.slice(0, 2);

const textTokens = ['--text-primary', '--text-secondary', '--text-tertiary', '--text-muted'];
// 只列真实承载正文的表面。--bg-active/--bg-active-soft 上叠的是主文字与反色文字（见下方配对），
// 不放 muted 类文字，避免矩阵凭空要求不存在的叠放关系。
const surfaceTokens = [
  '--bg-primary',
  '--bg-secondary',
  '--bg-tertiary',
  '--bg-sunken',
  '--bg-elevated',
  '--bg-input',
  '--bg-hover',
  '--bg-dialog',
  '--bg-settings-canvas',
];
// 半透明层次层：必须叠加在具体底色上才有意义，单独作为"表面"比较会算错。
const layerTokens = ['--layer-1', '--layer-2'];
const layerBackdrops = ['--bg-primary', '--bg-secondary', '--bg-elevated', '--bg-dialog'];
// 状态语义色同时被当作文字与图标色用在面板表面上（ImportStat 的数值、Toast 的图标、FilesPanel 的提示）。
const statusTokens = [
  '--warning',
  '--success',
  '--danger',
  '--info',
  '--severity-warning',
  '--severity-danger',
  '--severity-info',
  '--severity-success',
];
const statusSurfaces = ['--bg-primary', '--bg-secondary', '--bg-elevated', '--bg-dialog'];
// 成对定义的语义前景/底色。
const semanticPairs: Array<[string, string]> = [
  ['--text-on-accent', '--accent'],
  ['--text-on-accent', '--accent-hover'],
  ['--text-primary', '--bg-active'],
  ['--severity-warning', '--warning-bg-subtle'],
  ['--risk-moderate-fg', '--risk-moderate-bg'],
  ['--risk-high-fg', '--risk-high-bg'],
  ['--risk-critical-fg', '--risk-critical-bg'],
  ['--chip-purple-fg', '--chip-purple-bg'],
  ['--icon-folder-fg', '--icon-folder-bg'],
  ['--icon-code-fg', '--icon-code-bg'],
  ['--icon-archive-fg', '--icon-archive-bg'],
  ['--danger', '--danger-bg-subtle'],
  ['--success', '--success-bg-subtle'],
  ['--info', '--info-bg-subtle'],
];

// 暗色的 --bg-dialog 是 var(--bg-tertiary) 这样的引用，计算前先解一层。
function resolve(palette: Record<string, string>, name: string): string {
  const seen = new Set<string>();
  let value = palette[name] ?? '';
  for (;;) {
    const reference = value.match(/^var\((--[\w-]+)\)$/);
    if (!reference || seen.has(reference[1])) return value;
    seen.add(reference[1]);
    value = palette[reference[1]] ?? value;
  }
}

// 一次性收集全部不达标组合而不是断言到第一个就停，便于一次看清缺哪些取值。
function contrastFailures(palette: Record<string, string>, pairs: Array<[string, string]>): string[] {
  const failures: string[] = [];
  for (const [foregroundName, backgroundName] of pairs) {
    const fg = resolve(palette, foregroundName);
    const bg = resolve(palette, backgroundName);
    const color = /^#[0-9a-fA-F]{3,8}$/;
    if (!color.test(fg) || !color.test(bg)) {
      failures.push(`${foregroundName} on ${backgroundName}：取值无法计算（${fg} / ${bg}）`);
      continue;
    }
    const ratio = contrast(fg, bg);
    if (ratio < 4.5) failures.push(`${foregroundName} on ${backgroundName} = ${ratio.toFixed(2)}（要求 4.5）`);
  }
  return failures;
}

const pairsOf = (foregrounds: string[], backgrounds: string[]): Array<[string, string]> =>
  foregrounds.flatMap((foreground) => backgrounds.map((background): [string, string] => [foreground, background]));

// 终端旁路（搜索命中高亮、高亮规则默认色）也是 xterm 数据，走 terminalSchemes，
// 但它们同样必须随主题且可读，否则亮色终端下会出现"浅底浅字"（见 docs/theme-design.md 4.4）。
describe('终端旁路随主题', () => {
  it.each(['dark', 'light'] as const)('%s：搜索命中高亮与终端前景色达到 4.5', (theme) => {
    const terminal = getTerminalTheme(theme);
    const decorations = getTerminalSearchDecorations(theme);
    const foreground = terminal.foreground ?? '#000000';
    const pairs: Array<[string, string | undefined]> = [
      ['匹配底', decorations.matchBackground],
      ['当前匹配底', decorations.activeMatchBackground],
    ];
    const failures = pairs
      .filter(([, background]) => typeof background === 'string')
      .filter(([, background]) => contrast(foreground, background as string) < 4.5)
      .map(([label, background]) => `${label} ${background} 与前景 ${foreground} 对比度 ${contrast(foreground, background as string).toFixed(2)}`);
    expect(failures.join('\n')).toBe('');
  });

  it.each(['dark', 'light'] as const)('%s：新建高亮规则的默认配色可读', (theme) => {
    const defaults = terminalHighlightDefaults[theme];
    expect(contrast(defaults.color, defaults.background_color)).toBeGreaterThanOrEqual(4.5);
  });

  it('两种主题的搜索高亮与默认配色互不相同', () => {
    expect(terminalHighlightDefaults.light).not.toEqual(terminalHighlightDefaults.dark);
    expect(getTerminalSearchDecorations('light').matchBackground).not.toBe(getTerminalSearchDecorations('dark').matchBackground);
  });
});

describe('currentTheme', () => {
  it('以 <html data-theme> 为准，缺省回退暗色', () => {
    const original = document.documentElement.dataset.theme;
    try {
      document.documentElement.dataset.theme = 'light';
      expect(currentTheme()).toBe('light');
      document.documentElement.dataset.theme = 'dark';
      expect(currentTheme()).toBe('dark');
      delete document.documentElement.dataset.theme;
      expect(currentTheme()).toBe('dark');
      document.documentElement.dataset.theme = 'nonsense';
      expect(currentTheme()).toBe('dark');
    } finally {
      if (original === undefined) delete document.documentElement.dataset.theme;
      else document.documentElement.dataset.theme = original;
    }
  });
});

describe.each(palettes)('%s 模式的对比度', (_mode, palette) => {
  it('正文在每一层表面上都达到 4.5', () => {
    expect(contrastFailures(palette, pairsOf(textTokens, surfaceTokens)).join('\n')).toBe('');
  });

  it('半透明层次层叠加到各底色后，正文仍达到 4.5', () => {
    // 层次层上只出现主/次文字：引用块是 --text-secondary、表头是 --text-primary、
    // 行内代码与斑马行继承正文。muted/tertiary 不落在这些层上，不列入（与 --bg-active 同理）。
    const layerTextTokens = ['--text-primary', '--text-secondary'];
    const failures: string[] = [];
    for (const layer of layerTokens) {
      for (const backdrop of layerBackdrops) {
        const composited = composite(resolve(palette, layer), resolve(palette, backdrop), palette);
        if (!composited) {
          failures.push(`${layer} over ${backdrop}：无法按 alpha 合成（${resolve(palette, layer)}）`);
          continue;
        }
        for (const text of layerTextTokens) {
          const ratio = contrast(resolve(palette, text), composited);
          if (ratio < 4.5) failures.push(`${text} on ${layer} over ${backdrop} = ${ratio.toFixed(2)}（要求 4.5）`);
        }
      }
    }
    expect(failures.join('\n')).toBe('');
  });

  it('状态色当文字用在面板表面上时达到 4.5', () => {
    expect(contrastFailures(palette, pairsOf(statusTokens, statusSurfaces)).join('\n')).toBe('');
  });

  it('语义前景色在对应底色上达到 4.5', () => {
    expect(contrastFailures(palette, semanticPairs).join('\n')).toBe('');
  });
});

// 皮肤是第二个轴：宿主决定用哪套视觉语言，用户偏好明暗是另一件事，两者各存各的键。
// 本步只建立机制（teams 块暂不声明令牌），所以"桌面壳无副作用"目前是恒等于模式层；
// 第 7 步把 doc 6.3 的映射写进 teams 块后，下面第 5 条依然要成立——因为兜底值就是模式层取值，
// 这正是 6.2 那条"宿主缺变量时降级为模式层"的语义，它比截图更精确且可进 CI。
describe('皮肤轴', () => {
  // doc 6.3 首批映射表的 OpsCopilot 列（尺寸族各取一个代表）。
  const mappedTokens = [
    '--bg-primary', '--bg-secondary', '--bg-tertiary', '--bg-elevated', '--bg-input', '--bg-hover',
    '--bg-active', '--bg-active-soft', '--border', '--border-subtle', '--border-focus',
    '--text-primary', '--text-secondary', '--text-tertiary', '--text-muted', '--text-disabled', '--text-on-accent',
    '--accent', '--accent-hover', '--accent-soft', '--success', '--success-bg-subtle',
    '--info', '--info-bg-subtle', '--danger', '--danger-bg-subtle', '--shadow-dialog',
    '--radius-sm', '--space-8', '--font-size-base',
  ];

  // 皮肤块的取值形如 var(--ui-color-bg, var(--mode-bg-primary))：桌面壳没有宿主变量，解析结果
  // 必然是兜底值，而兜底是模式层取值的镜像。出现别的写法就返回一个必定不等的标记，
  // 逼着后续步骤显式扩展这里，而不是静默漏测。
  //
  // 为什么兜底必须是独立的镜像令牌：实测（真实浏览器）三种写法的解析结果——
  //   var(--ui-x, var(--同名令牌)) → 空值（自定义属性自我引用成环，两者一起失效）
  //   var(--ui-x)                 → 空值（不是回落到上一条声明，也不会自动降级）
  //   var(--ui-x, var(--mode-x))  → 兜底生效；宿主变量存在时取宿主值（22px 覆盖 11px）
  // 所以"省略兜底"与"兜底引用自身"都会让页面塌掉，形状与镜像两条断言缺一不可。
  function skinValueOf(declared: string | undefined, palette: Record<string, string>): string {
    if (declared === undefined) return '';
    const match = declared.match(/^var\((--ui-[\w-]+),\s*(.+)\)$/);
    if (!match) return `«皮肤块写法未扩展解析：${declared}»`;
    const hostValue = palette[match[1]];
    if (hostValue !== undefined) return hostValue;
    return match[2].replace(/var\((--[\w-]+)\)/g, (_all, token: string) => palette[token] ?? `«缺少兜底 ${token}»`);
  }

  const teamsBlock = tokens(':root[data-skin="teams"] {');

  it('宿主契约快照可信：非空、名字规范、带出处', () => {
    expect(Array.isArray(hostContract.variables)).toBe(true);
    expect(hostContract.variables.length).toBeGreaterThan(0);
    expect(hostContract.hostRevision).toMatch(/^[0-9a-f]{40}$/);
    const malformed = hostContract.variables.filter((name) => !/^--ui-[a-z0-9]+(-[a-z0-9]+)*$/.test(name));
    expect(malformed.join('\n')).toBe('');
  });

  it('归一化只接受 default/teams，其余回退 default', () => {
    expect(normalizeSkin('teams')).toBe('teams');
    expect(normalizeSkin('TEAMS ')).toBe('teams');
    expect(normalizeSkin('default')).toBe('default');
    for (const value of [undefined, null, '', 'dark', 'teams-x', 'nonsense']) {
      expect(normalizeSkin(value), String(value)).toBe(DEFAULT_SKIN);
    }
  });

  it('皮肤键与主题键相互独立', () => {
    expect(SKIN_STORAGE_KEY).not.toBe(THEME_STORAGE_KEY);
    const themeBefore = window.localStorage.getItem(THEME_STORAGE_KEY);
    persistSkin('teams');
    expect(readPersistedSkin()).toBe('teams');
    expect(window.localStorage.getItem(SKIN_STORAGE_KEY)).toBe('teams');
    // 写皮肤不得动主题键：合成一个键会让"换宿主皮肤"顺带丢掉用户的明暗选择
    expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe(themeBefore);
    persistSkin('nonsense' as never);
    expect(readPersistedSkin()).toBe(DEFAULT_SKIN);
  });

  it('applySkin 写到与 data-theme 同一元素，currentSkin 读回', () => {
    const original = document.documentElement.dataset.skin;
    try {
      expect(applySkin('teams')).toBe('teams');
      expect(document.documentElement.dataset.skin).toBe('teams');
      expect(currentSkin()).toBe('teams');
      applySkin('default');
      expect(currentSkin()).toBe('default');
      applySkin('nonsense' as never);
      expect(currentSkin()).toBe('default');
    } finally {
      if (original === undefined) delete document.documentElement.dataset.skin;
      else document.documentElement.dataset.skin = original;
    }
  });

  it('teams 块声明在亮色块之后（同权重靠后者生效）', () => {
    const light = css.indexOf(':root[data-theme="light"] {');
    const teams = css.indexOf(':root[data-skin="teams"] {');
    expect(light, '找不到亮色块').toBeGreaterThan(-1);
    expect(teams, '找不到皮肤块').toBeGreaterThan(light);
  });

  it('6.3 映射清单里的令牌都真实存在（否则等价断言会空转）', () => {
    const declared = tokens(':root {');
    const missing = mappedTokens.filter((name) => !(name in declared));
    expect(missing.join('\n')).toBe('');
  });

  it('无宿主变量时 teams 与 default 逐项等值（皮肤不得泄漏到桌面壳）', () => {
    const failures: string[] = [];
    for (const [label, palette] of modePalettes) {
      for (const name of mappedTokens) {
        const modeValue = resolve(palette, name);
        const declared = teamsBlock[name];
        const skinValue = declared === undefined ? modeValue : skinValueOf(declared, palette);
        if (skinValue !== modeValue) failures.push(`${label} ${name}: teams ${skinValue} ≠ default ${modeValue}`);
      }
    }
    expect(failures.join('\n')).toBe('');
  });

  it('皮肤块的取值形状一律是 var(--ui-*, var(--mode-*))', () => {
    const offenders = Object.entries(teamsBlock)
      .filter(([, value]) => !/^var\(--ui-[\w-]+,\s*var\(--mode-[\w-]+\)\)$/.test(value))
      .map(([name, value]) => `${name}: ${value}`);
    expect(offenders.join('\n')).toBe('');
  });

  it('皮肤块引用的宿主变量都在快照清单里', () => {
    const known = new Set(hostContract.variables);
    const referenced = Object.values(teamsBlock).flatMap((value) => [...value.matchAll(/var\((--ui-[\w-]+)/g)].map((m) => m[1]));
    expect(referenced.length, '皮肤块没有引用任何宿主变量，这条断言会空转').toBeGreaterThan(0);
    const unknown = [...new Set(referenced)].filter((name) => !known.has(name));
    expect(unknown.join('\n')).toBe('');
  });

  it('兜底镜像与模式层本体逐一相等（错一个字就会兜出另一个颜色）', () => {
    const failures: string[] = [];
    for (const [label, palette] of modePalettes) {
      for (const name of Object.keys(teamsBlock)) {
        const mirror = `--mode-${name.slice(2)}`;
        const expected = resolve(palette, name);
        const actual = resolve(palette, mirror);
        if (actual !== expected) failures.push(`${label} ${mirror}=${actual || '«未声明»'} ≠ ${name}=${expected}`);
      }
    }
    expect(failures.join('\n')).toBe('');
  });

  // 宿主变量不得往上渗进模式层：模式层必须自足，否则桌面壳（没有宿主变量）会跟着宿主一起塌。
  it('模式层不引用宿主变量（只有皮肤块可以）', () => {
    const modeLayer = palettes.flatMap(([, palette]) => Object.entries(palette));
    const offenders = modeLayer
      .filter(([, value]) => value.includes('var(--ui-'))
      .map(([name, value]) => `${name}: ${value}`);
    expect(offenders.join('\n')).toBe('');
  });
});

// 步骤 8：模式跟随宿主。跟随与否、跟随到的值、以及"宿主没有明暗"这三种状态必须能分开判断——
// 把它们合成一个值是这套逻辑最容易出的错（宿主没声明明暗时会被误当成"宿主说是亮色"）。
describe('模式跟随宿主', () => {
  const root = () => document.documentElement;
  // 用例会改 documentElement 的属性与 localStorage，跑完必须复原，否则污染同文件里的其它断言。
  const withAttrs = async (run: () => void | Promise<void>) => {
    const saved = { theme: root().dataset.theme, skin: root().dataset.skin, follow: window.localStorage.getItem(THEME_FOLLOW_STORAGE_KEY), stored: window.localStorage.getItem(THEME_STORAGE_KEY) };
    try {
      await run();
    } finally {
      if (saved.theme === undefined) delete root().dataset.theme; else root().dataset.theme = saved.theme;
      if (saved.skin === undefined) delete root().dataset.skin; else root().dataset.skin = saved.skin;
      if (saved.follow === null) window.localStorage.removeItem(THEME_FOLLOW_STORAGE_KEY); else window.localStorage.setItem(THEME_FOLLOW_STORAGE_KEY, saved.follow);
      if (saved.stored === null) window.localStorage.removeItem(THEME_STORAGE_KEY); else window.localStorage.setItem(THEME_STORAGE_KEY, saved.stored);
    }
  };

  it('归一化只接受 host/manual，其余回退默认 host', () => {
    expect(DEFAULT_THEME_FOLLOW).toBe('host');
    expect(normalizeThemeFollow('manual')).toBe('manual');
    expect(normalizeThemeFollow('HOST')).toBe('host');
    for (const bad of [undefined, null, '', 'dark', 'light', 'follow', 'host-x']) expect(normalizeThemeFollow(bad as never)).toBe('host');
  });

  it('写跟随键不动主题键与皮肤键', async () => {
    await withAttrs(() => {
      window.localStorage.setItem(THEME_STORAGE_KEY, 'dark');
      const themeBefore = window.localStorage.getItem(THEME_STORAGE_KEY);
      persistThemeFollow('manual');
      expect(readPersistedThemeFollow()).toBe('manual');
      persistThemeFollow('host');
      expect(readPersistedThemeFollow()).toBe('host');
      expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe(themeBefore);
      expect(root().dataset.skin).toBeUndefined();
    });
  });

  it('宿主没有明暗时返回 undefined 而不是回退默认主题', async () => {
    await withAttrs(() => {
      delete root().dataset.theme;
      expect(hostTheme()).toBeUndefined();
      root().dataset.theme = 'dark';
      expect(hostTheme()).toBe('dark');
      root().dataset.theme = 'light';
      expect(hostTheme()).toBe('light');
      root().dataset.theme = 'nonsense';
      expect(hostTheme(), '宿主写了非法值应当视为"没有明暗"').toBeUndefined();
    });
  });

  it('宿主运行期改明暗会被回调，解除监听后不再回调', async () => {
    // MutationObserver 的回调在下一轮任务投递，所以必须真的等一次宏任务，
    // 不能在同一个同步块里断言（那样看到的永远是空数组）。
    const settle = () => new Promise<void>(resolve => setTimeout(resolve, 0));
    await withAttrs(async () => {
      root().dataset.theme = 'light';
      const seen: string[] = [];
      const stop = observeHostTheme(theme => seen.push(theme));
      root().dataset.theme = 'dark';
      await settle();
      expect(seen).toEqual(['dark']);
      stop();
      root().dataset.theme = 'light';
      await settle();
      expect(seen, '解除监听后不应再收到回调').toEqual(['dark']);
    });
  });
});

// 皮肤入口的前置判断：没选过 ≠ 选成了 default；有没有宿主令牌决定入口该不该出现。
describe('皮肤选择的前置判断', () => {
  const withSkinKey = async (run: () => void | Promise<void>) => {
    const saved = window.localStorage.getItem(SKIN_STORAGE_KEY);
    try {
      await run();
    } finally {
      if (saved === null) window.localStorage.removeItem(SKIN_STORAGE_KEY);
      else window.localStorage.setItem(SKIN_STORAGE_KEY, saved);
    }
  };

  it('没选过时返回 undefined，选过才返回具体值', async () => {
    await withSkinKey(() => {
      window.localStorage.removeItem(SKIN_STORAGE_KEY);
      expect(readPersistedSkinChoice()).toBeUndefined();
      persistSkin('teams');
      expect(readPersistedSkinChoice()).toBe('teams');
      persistSkin('default');
      expect(readPersistedSkinChoice(), '选成 default 与没选过必须能分开').toBe('default');
      window.localStorage.setItem(SKIN_STORAGE_KEY, 'nonsense');
      expect(readPersistedSkinChoice()).toBeUndefined();
    });
  });

  it('hostTokensAvailable 以 --ui-color-bg 是否解析出值为准', async () => {
    const html = document.documentElement;
    const saved = html.getAttribute('style');
    try {
      html.removeAttribute('style');
      expect(hostTokensAvailable(), '没有宿主变量时应为 false').toBe(false);
      html.style.setProperty('--ui-color-bg', '#fafafa');
      expect(hostTokensAvailable(), '有宿主变量时应为 true').toBe(true);
    } finally {
      if (saved === null) html.removeAttribute('style');
      else html.setAttribute('style', saved);
    }
  });
});
