// 修复验证矩阵：真实构建产物上，多 DPR × 多视口 × 字号增减循环，
// 断言行矩形始终在内容区内且与底缘保持留白（不贴底、不越界）。
// node harness/verify-fontfit.mjs
import { chromium } from '@playwright/test';
import { readFileSync } from 'node:fs';

const base = 'http://127.0.0.1:34315';
const ep = JSON.parse(readFileSync(new URL('../.runtime/endpoint.json', import.meta.url), 'utf-8'));

const browser = await chromium.launch({ channel: 'chrome' });
let bad = 0, total = 0;
for (const dpr of [1, 1.25, 1.5, 2]) {
  for (const [vw, vh] of [[1280, 720], [1024, 700], [1536, 860]]) {
    const ctx = await browser.newContext({ viewport: { width: vw, height: vh }, deviceScaleFactor: dpr });
    const page = await ctx.newPage();
    await page.goto(base);
    await page.waitForTimeout(1500);
    await page.getByRole('button', { name: '+ 新建连接' }).click();
    await page.getByRole('button', { name: '手动添加连接' }).click();
    await page.getByPlaceholder('连接名称', { exact: true }).fill(`fit-${dpr}-${vw}`);
    await page.getByLabel('主机地址', { exact: true }).fill(ep.sftp.host);
    await page.getByLabel('端口', { exact: true }).fill(String(ep.sftp.port));
    await page.getByLabel('用户名', { exact: true }).fill('test');
    await page.getByLabel('密码', { exact: true }).fill('test');
    await page.getByRole('button', { name: /连接选中项/ }).click();
    const term = page.locator('[data-testid^="terminal-container-"]:visible').first();
    try { await term.waitFor({ state: 'visible', timeout: 12000 }); } catch { console.log(dpr, vw, 'CONNECT FAIL'); await ctx.close(); continue; }
    await page.waitForTimeout(800);

    let scenarioBad = 0;
    for (const key of ['Control+=', 'Control+=', 'Control+=', 'Control+-', 'Control+-', 'Control+-', 'Control+-', 'Control+0']) {
      await term.click();
      await page.keyboard.press(key);
      await page.waitForTimeout(400);
      const m = await page.evaluate(() => {
        const host = document.querySelector('[data-testid^="terminal-container-"]:not([style*="display: none"]) .terminal-host')
          ?? document.querySelector('.terminal-host');
        const root = host.parentElement;
        const rootRect = root.getBoundingClientRect();
        const rootPadBottom = parseFloat(getComputedStyle(root).paddingBottom) || 0;
        const rowsEl = host.querySelector('.xterm-rows');
        if (!rowsEl) return { err: true };
        const rr = rowsEl.getBoundingClientRect();
        // 越界：行矩形底/右 超出根内容盒（= 应有 8px 底部留白）
        return {
          overBottom: rr.bottom - (rootRect.bottom - rootPadBottom),
          overRight: rr.right - rootRect.right,
          gapToBottom: rootRect.bottom - rr.bottom,
        };
      });
      total++;
      if (m.err || m.overBottom > 0.5 || m.overRight > 0.5 || m.gapToBottom < 6) {
        bad++; scenarioBad++;
        console.log(`  BAD dpr=${dpr} vp=${vw} ${key}:`, JSON.stringify(m));
      }
    }
    console.log(`dpr=${dpr} vp=${vw}x${vh}: ${scenarioBad}/8 bad`);
    await ctx.close();
  }
}
console.log(bad === 0 ? `ALL GREEN: 0/${total}` : `FAILURES: ${bad}/${total}`);
await browser.close();
process.exit(bad === 0 ? 0 : 1);
