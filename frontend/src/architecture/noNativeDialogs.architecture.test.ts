import fs from 'node:fs';
import path from 'node:path';
import { describe, expect, it } from 'vitest';

const REPO_ROOT = path.resolve(__dirname, '..', '..', '..');

const SCAN_ROOTS = [
  path.join(REPO_ROOT, 'frontend', 'src'),
  path.join(REPO_ROOT, 'frontend-shell', 'src'),
];

/**
 * 唯一允许出现 window.confirm 兜底的文件：仅在无 React 宿主的无头单测环境下触发。
 * 真实桌面端入口（main.tsx）必须挂载 <ConfirmDialogInternal />，确保线上绝不触发原生弹窗。
 */
const ALLOWLIST = new Set([
  'frontend-shell/src/ui/feedback/ConfirmDialog.tsx',
]);

const NATIVE_DIALOG_PATTERNS: Array<{ name: string; re: RegExp }> = [
  {
    name: 'window/globalThis/self native dialog',
    re: /\b(?:window|globalThis|self)\s*\.\s*(alert|confirm|prompt)\s*\(/g,
  },
  {
    name: 'bare native dialog',
    re: /(?<![.\w$])(alert|confirm|prompt)\s*\(/g,
  },
];

function collectSourceFiles(dir: string, out: string[] = []): string[] {
  if (!fs.existsSync(dir)) return out;
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === 'node_modules' || entry.name === 'dist') continue;
      collectSourceFiles(full, out);
    } else if (/\.(ts|tsx|js|jsx)$/.test(entry.name)) {
      if (/\.(test|spec)\.(ts|tsx|js|jsx)$/.test(entry.name) || entry.name.endsWith('.d.ts')) {
        continue;
      }
      out.push(full);
    }
  }
  return out;
}

/** 剥离注释（保留换行数，便于报告精确行号）。 */
function stripCommentsPreserveLines(source: string): string {
  return source
    .replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, ' '))
    .replace(/(^|[^:\\])\/\/[^\n]*/g, (m, prefix) => prefix + ' '.repeat(m.length - prefix.length));
}

describe('Architecture: forbid native browser dialogs (alert / confirm / prompt)', () => {
  it('contains zero native alert(), confirm(), or prompt() calls in frontend & frontend-shell source', () => {
    const violations: string[] = [];

    for (const root of SCAN_ROOTS) {
      for (const file of collectSourceFiles(root)) {
        const rel = path.relative(REPO_ROOT, file).replace(/\\/g, '/');
        if (ALLOWLIST.has(rel)) continue;

        const raw = fs.readFileSync(file, 'utf8');
        const cleaned = stripCommentsPreserveLines(raw);
        const lines = cleaned.split(/\r?\n/);

        lines.forEach((line, idx) => {
          for (const { re } of NATIVE_DIALOG_PATTERNS) {
            re.lastIndex = 0;
            let match: RegExpExecArray | null;
            while ((match = re.exec(line)) !== null) {
              violations.push(`${rel}:${idx + 1} -> ${match[0].trim()}`);
            }
          }
        });
      }
    }

    expect(
      violations,
      `发现禁止使用的原生浏览器弹窗（会暴露 WebView 特征），请改用 confirmDialog.show / confirmDialog.prompt / useToast：\n${violations.join('\n')}`,
    ).toEqual([]);
  });

  it('mounts <ConfirmDialogInternal /> in desktop and shell dev entrypoints so fallback never fires at runtime', () => {
    const entrypoints = [
      path.join(REPO_ROOT, 'frontend', 'src', 'main.tsx'),
      path.join(REPO_ROOT, 'frontend-shell', 'src', 'dev', 'main.tsx'),
    ];
    for (const entry of entrypoints) {
      const content = fs.readFileSync(entry, 'utf8');
      expect(content, `${path.relative(REPO_ROOT, entry)} 必须挂载 <ConfirmDialogInternal />`).toMatch(
        /<ConfirmDialogInternal\b/,
      );
    }
  });
});
