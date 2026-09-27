import { expect, test } from '@playwright/test';
import { endpoint, expectAppAlive, quickConnect } from './_helpers';

/**
 * G-SET-28 视口尺寸变化：终端 refit 不崩、不白屏、数据面不断。
 *
 * 守护的回归类：resize 链路（Terminal.tsx 的 ruleController/onResize 定时器、
 * flex-layout 重排、xterm fit）——历史上有过 resize 定时器堆积与渲染崩溃。
 * 连续变几个尺寸后：应用存活、终端可见、仍能输入回显。
 */
test('G-SET-28: 连续改变窗口尺寸，终端存活且输入回显正常', async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto('/');

  const ep = endpoint();
  const term = await quickConnect(page, { name: 'e2e-set-28', host: ep.sftp.host, port: ep.sftp.port });

  for (const size of [
    { width: 800, height: 600 },
    { width: 1600, height: 900 },
    { width: 1024, height: 768 },
    { width: 1280, height: 720 },
  ]) {
    await page.setViewportSize(size);
    await page.waitForTimeout(300);
  }

  await expectAppAlive(page);
  const termNow = page.locator('[data-testid^="terminal-container-"]:visible').first();
  await expect(termNow).toBeVisible({ timeout: 5000 });
  await termNow.click();
  await page.keyboard.type('sentinel-resize-2817', { delay: 10 });
  await expect
    .poll(() => termNow.innerText(), { timeout: 10_000, message: 'resize 后输入回显必须正常' })
    .toContain('sentinel-resize-2817');
});
