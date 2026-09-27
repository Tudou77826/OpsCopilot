import { expect, test } from '@playwright/test';
import { endpoint, expectAppAlive, quickConnect } from './_helpers';

/**
 * G-TERM-12 「连接 → cd 目录 → grep 日志 → Ctrl+F 回搜」的日常信息流。
 *
 * 覆盖三段：
 *   1. 命令输入路径：cd / grep 命令的完整往返（键盘→SSH→回显→渲染）；
 *   2. 刷屏场景：bulk 端点在 shell 建立时由服务端主动下发 150 行日志——
 *      模拟 grep 大输出的真实方向（服务器→客户端），渲染不得卡死；
 *   3. Ctrl+F 终端搜索：xterm 是视口虚拟渲染（DOM 只含可见行），
 *      "滚动历史里的 token 搜索后必须出现在可见区"是跳转语义的硬断言。
 *
 * 边界说明：fakessh 是回显服务器，cd/grep 的命令语义（目录状态、过滤结果）
 * 属于远端行为，由 D 组真实服务器用例覆盖；本用例守住端到端交互链路。
 */
test('G-TERM-12: cd/grep 命令往返、服务端刷屏渲染不卡死、Ctrl+F 搜索跳转命中历史行', async ({ page }) => {
  test.setTimeout(90_000);
  await page.goto('/');

  const ep = endpoint();
  const EARLY = ep.bulk.earlyToken;

  // --- 1. 日常命令输入：cd 目录、grep 日志（往返即可见） ---
  const term = await quickConnect(page, { name: 'e2e-term-12a', host: ep.sftp.host, port: ep.sftp.port });
  const rowsA = term.locator('.xterm-rows');
  await term.click();
  for (const cmd of ['cd /var/log/app', 'grep ERROR app.log.2026-09-27']) {
    await page.keyboard.type(cmd, { delay: 8 });
    await page.keyboard.press('Enter');
  }
  await expect
    .poll(() => rowsA.innerText(), { timeout: 10_000, message: 'cd/grep 命令应回显在终端' })
    .toContain('grep ERROR app.log.2026-09-27');

  // --- 2+3. 刷屏端点：连接即被推送 150 行日志，然后 Ctrl+F 回搜历史行 ---
  const term2 = await quickConnect(page, { name: 'e2e-term-12b', host: ep.bulk.host, port: ep.bulk.port }, 20_000);
  const rows = term2.locator('.xterm-rows');

  // 刷屏完成判据：末行进入可见区（视口只渲染可见行，末行=最后下发）
  await expect
    .poll(() => rows.innerText(), { timeout: 20_000, message: '服务端刷屏必须在时限内渲染完成（卡死即红）' })
    .toContain(ep.bulk.lastMarker);

  // 早 token 已被刷出视口：可见区不应包含它（确认它只在滚动历史里）
  expect(await rows.innerText(), '搜索前早 token 应已滚出可见区').not.toContain(EARLY);

  // Ctrl+F 搜索：跳转到滚动历史中的命中行
  await term2.click();
  await page.keyboard.press('Control+f');
  const searchInput = page.getByPlaceholder('搜索…');
  await expect(searchInput).toBeVisible({ timeout: 5_000 });

  await searchInput.fill(EARLY);
  // 增量搜索按设计保持视口（输入时不跳转），按 Enter「下一个」才滚动到命中行
  await searchInput.press('Enter');
  await expect
    .poll(() => rows.innerText(), { timeout: 10_000, message: '搜索必须把历史命中行跳进可见区' })
    .toContain(EARLY);

  // Esc 关闭搜索后终端仍可用
  await page.keyboard.press('Escape');
  await expect(searchInput).toBeHidden({ timeout: 5_000 });
  await term2.click();
  await page.keyboard.type('sentinel-after-search-1212', { delay: 8 });
  await expect
    .poll(() => rows.innerText(), { timeout: 10_000, message: '关搜索后输入必须仍回显' })
    .toContain('sentinel-after-search-1212');

  await expectAppAlive(page);
});
