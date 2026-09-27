import { expect, test, type Locator, type Page } from '@playwright/test';
import { endpoint, expectAppAlive, quickConnect } from './_helpers';

/**
 * G-TERM-10 多会话并存：两个终端各自收发，互不串号。
 *
 * 守护的回归类：会话串号——终端数据路由错 session（EventsOn key、
 * terminalRefs map、会话清理失误都会造成 A 的输出进 B 的屏幕）。两个端点
 * 横幅不同 + 各自敲哨兵串，交叉验证"各收各的"。
 */
function visibleTerminal(page: Page): Locator {
  return page.locator('[data-testid^="terminal-container-"]:visible').first();
}

async function switchTab(page: Page, name: string) {
  await page.locator('.flexlayout__tab_button', { hasText: name }).first().click();
  await page.waitForTimeout(200);
}

test('G-TERM-10: 双会话并存，切换标签后各自的输入/输出互不串扰', async ({ page }) => {
  test.setTimeout(90_000);
  await page.goto('/');

  const ep = endpoint();
  const A = 'e2e-term-10a';
  const B = 'e2e-term-10b';

  const termA = await quickConnect(page, { name: A, host: ep.sftp.host, port: ep.sftp.port });
  await expect
    .poll(() => termA.innerText(), { timeout: 10_000 })
    .toContain('e2e sftp endpoint');

  const termB = await quickConnect(page, { name: B, host: ep.nosftp.host, port: ep.nosftp.port });
  await expect
    .poll(() => termB.innerText(), { timeout: 10_000 })
    .toContain('e2e nosftp endpoint');

  // 在 B（当前活动终端）敲哨兵 B
  await termB.click();
  await page.keyboard.type('sentinel-B-1031', { delay: 10 });

  // 切回 A，敲哨兵 A
  await switchTab(page, A);
  const activeA = visibleTerminal(page);
  await activeA.click();
  await page.keyboard.type('sentinel-A-0942', { delay: 10 });

  await expect
    .poll(() => activeA.innerText(), { timeout: 10_000, message: 'A 终端应回显 A 的哨兵' })
    .toContain('sentinel-A-0942');

  // 再切到 B：B 有自己的哨兵，且 A 的哨兵不应出现在 B 里（串号即红）
  await switchTab(page, B);
  const activeB = visibleTerminal(page);
  await expect
    .poll(() => activeB.innerText(), { timeout: 10_000, message: 'B 终端应回显 B 的哨兵' })
    .toContain('sentinel-B-1031');
  expect(await activeB.innerText(), 'A 的输出不得出现在 B 的终端（串号即回归）').not.toContain('sentinel-A-0942');

  await expectAppAlive(page);
});

/**
 * G-TERM-11 关闭终端 → 后端会话清理 → 重连可用。
 * 守护：关闭标签后会话句柄/事件监听泄漏（后续重连数据路由错乱、资源耗尽）。
 */
test('G-TERM-11: 关闭终端后可立即重连，新会话输入回显正常', async ({ page }) => {
  test.setTimeout(90_000);
  await page.goto('/');

  const ep = endpoint();
  const NAME = 'e2e-term-11';

  await quickConnect(page, { name: NAME, host: ep.sftp.host, port: ep.sftp.port });

  // 标签右键 → 关闭
  const tab = page.locator('.flexlayout__tab_button', { hasText: NAME }).first();
  await tab.click({ button: 'right' });
  await page.getByText('关闭', { exact: true }).click();
  await expect(page.locator('[data-testid^="terminal-container-"]')).toHaveCount(0, { timeout: 10_000 });

  // 立即重连：必须能建立新会话且数据面正常（清理泄漏会让重连错乱）
  const term = await quickConnect(page, { name: `${NAME}-again`, host: ep.sftp.host, port: ep.sftp.port });
  await term.click();
  await page.keyboard.type('sentinel-reconnect-1105', { delay: 10 });
  await expect
    .poll(() => term.innerText(), { timeout: 10_000, message: '重连后回显必须正常' })
    .toContain('sentinel-reconnect-1105');

  await expectAppAlive(page);
});
