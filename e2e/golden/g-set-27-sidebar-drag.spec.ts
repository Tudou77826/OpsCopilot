import { expect, test } from '@playwright/test';
import { expectAppAlive, openSessionsSidebar } from './_helpers';

/**
 * G-SET-27 侧栏拖拽越界不白屏（v1.10.5 卡死回归的 E2E 化）。
 *
 * v1.10.5 修复的竞态：setState updater 读取 drag.current，与 pointerup
 * 清空 ref 竞态 → undefined.width → 顶层无错误边界 → React 整树卸载白屏。
 * 越界拖动（大量被钳制的 move + 松手）最易命中。
 *
 * 该竞态依赖真实浏览器的连续事件延迟调度，jsdom/act 冲洗模型无法复现
 * （单测三种构造均无法让旧代码变红）——只能在 E2E 层守护。
 */
test('G-SET-27: 侧栏拖到最大值后继续拖动并松手，应用不白屏不卡死', async ({ page }) => {
  await page.goto('/');
  await openSessionsSidebar(page);

  const handle = page.locator('[role="separator"][aria-label="调整侧栏宽度"]');
  await expect(handle).toBeVisible();

  const shell = handle.locator('..');

  // 越界拖动：从手柄一路拖到视口最左（远超 800 上限），多轮，每轮松手
  for (let round = 0; round < 4; round++) {
    const box = await handle.boundingBox();
    expect(box, '手柄必须始终存在（白屏即消失）').toBeTruthy();
    const y = box!.y + box!.height / 2;
    await page.mouse.move(box!.x + box!.width / 2, y);
    await page.mouse.down();
    // 连续小幅移动触发大量被钳制的 pointermove（正是竞态放大器）
    for (const x of [600, 400, 200, 100, 20, 5]) {
      await page.mouse.move(x, y, { steps: 3 });
    }
    await page.mouse.up();
    await page.waitForTimeout(80);
  }

  // 白屏守护：React 树卸载后 #root 会变空
  await expectAppAlive(page);

  // 交互存活：树仍可见、宽度被钳制且未塌缩
  await expect(page.locator('[data-testid="session-tree"]')).toBeVisible({ timeout: 5000 });
  const widthAfter = await shell.evaluate((el) => el.getBoundingClientRect().width);
  expect(widthAfter, '宽度应被钳制在上限附近，不因崩溃而归零').toBeGreaterThan(200);
  expect(widthAfter).toBeLessThanOrEqual(801);

  // 越界后往回拖：仍可正常响应（上限 ≠ 卡死）
  const box = await handle.boundingBox();
  const y = box!.y + box!.height / 2;
  await page.mouse.move(box!.x + box!.width / 2, y);
  await page.mouse.down();
  await page.mouse.move(box!.x + 200, y, { steps: 5 });
  await page.mouse.up();
  await expect(page.locator('[data-testid="session-tree"]')).toBeVisible({ timeout: 5000 });
});
