import { expect, test, type Page } from '@playwright/test';
import { endpoint, expectAppAlive, quickConnect } from './_helpers';

/**
 * G-SET-29 字号切换的 fit 完整性：行区不越界、最后一行不贴底。
 *
 * 回归背景（两代症状同根因）：FitAddon 用"父元素 computed height − xterm 自身
 * padding"计算可用空间，而视觉留白曾加在 fit 父元素 .terminal-host 上——
 * border-box 下 computed height 含 padding，fit 账面比真实内容区多出留白尺寸，
 * 特定 视口/字号/DPI 相位下会多排一行：轻则最后一行侵入底部留白（贴住底栏），
 * 重则被裁出显示范围。修复后留白在 Terminal 根包裹层，fit 记账归位。
 *
 * 用两个 DPR（整数 1 与分数 1.5，分数缩放改变行高量化相位）× 字号增减循环，
 * 每步断言：行矩形右/底不超出根内容盒，且与底缘留白 ≥ 6px（8px 设计留白
 * 减去亚像素容差）。
 */
async function fontCycleStaysInBox(page: Page, dpr: number) {
  const ep = endpoint();
  const term = await quickConnect(page, { name: `e2e-set-29-${dpr}`, host: ep.sftp.host, port: ep.sftp.port });
  await page.waitForTimeout(500);

  for (const key of ['Control+=', 'Control+=', 'Control+-', 'Control+-', 'Control+-', 'Control+0']) {
    await term.click();
    await page.keyboard.press(key);
    await page.waitForTimeout(400);

    const m = await page.evaluate(() => {
      const host = (document.querySelector('[data-testid^="terminal-container-"]:not([style*="display: none"]) .terminal-host')
        ?? document.querySelector('.terminal-host')) as HTMLElement | null;
      const rowsEl = host?.querySelector('.xterm-rows');
      if (!host || !rowsEl) return { err: true as const };
      const root = host.parentElement!;
      const rootRect = root.getBoundingClientRect();
      const padBottom = parseFloat(getComputedStyle(root).paddingBottom) || 0;
      const rr = rowsEl.getBoundingClientRect();
      return {
        overBottom: rr.bottom - (rootRect.bottom - padBottom),
        overRight: rr.right - rootRect.right,
        gapToBottom: rootRect.bottom - rr.bottom,
      };
    });
    expect(m.err ?? false, '终端 DOM 必须存在').toBe(false);
    // 数值必须在事件后算好；断言的是几何事实，不依赖实现细节
    const geom = m as { overBottom: number; overRight: number; gapToBottom: number };
    expect(geom.overBottom, `dpr=${dpr} ${key}: 行区底部不得超出内容盒（越界=裁切/贴底回归）`).toBeLessThanOrEqual(0.5);
    expect(geom.overRight, `dpr=${dpr} ${key}: 行区右侧不得超出内容盒`).toBeLessThanOrEqual(0.5);
    expect(geom.gapToBottom, `dpr=${dpr} ${key}: 最后一行与底缘留白 ≥6px（设计 8px − 亚像素容差）`).toBeGreaterThanOrEqual(6);
  }
  await expectAppAlive(page);
}

test('G-SET-29: 字号增减循环下终端行区不越界、最后一行不贴底（dpr 1 与 1.5）', async ({ browser }) => {
  test.setTimeout(120_000);
  for (const dpr of [1, 1.5]) {
    const ctx = await browser.newContext({
      viewport: { width: 1280, height: 720 },
      deviceScaleFactor: dpr as any,
      channel: undefined,
    });
    const page = await ctx.newPage();
    await page.goto(endpoint().appUrl);
    await page.waitForTimeout(1500);
    await fontCycleStaysInBox(page, dpr);
    await ctx.close();
  }
});
