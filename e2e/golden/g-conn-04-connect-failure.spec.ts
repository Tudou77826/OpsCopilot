import { expect, test, type Page } from '@playwright/test';
import { attemptConnect, endpoint, expectAppAlive } from './_helpers';

/**
 * G-CONN-04 连接失败路径：错误必须"快速且明确"，不许无反应。
 *
 * 守护的回归类：连接失败无提示/假死（错误路径长期缺测试——历史回归多发生在
 * 主路径修好后、错误路径被顺手改坏）。两类失败各测一条：
 *   1. 认证失败（密码错误）：服务端明确拒绝；
 *   2. 网络不可达（端口无人监听）：连接拒绝。
 * 每条都要求 12s 内弹出「连接失败」弹窗，关闭后应用存活。
 */
async function expectConnectFailure(page: Page, name: string, port: number, password: string) {
  const ep = endpoint();
  await attemptConnect(page, { name, host: ep.sftp.host, port, password });

  const errModal = page.locator('[aria-label="连接失败"]');
  await expect(errModal).toBeVisible({ timeout: 12_000 });
  await errModal.getByRole('button', { name: '关闭' }).click();
  await expect(errModal).toBeHidden({ timeout: 5000 });
  await expectAppAlive(page);
}

test('G-CONN-04a: 密码错误限时弹出明确错误，关闭后应用存活', async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto('/');
  const ep = endpoint();
  await expectConnectFailure(page, 'e2e-conn-04a', ep.sftp.port, 'wrong-password-e2e');
});

test('G-CONN-04b: 端口不可达限时弹出明确错误，不假死', async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto('/');
  // 34319 无人监听：连接拒绝必须快速显式失败，而不是挂到超时
  await expectConnectFailure(page, 'e2e-conn-04b', 34319, 'test');
});
