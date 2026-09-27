import { expect, test } from '@playwright/test';
import { endpoint, expectAppAlive, openFileTransferTab, quickConnect, remotePane } from './_helpers';

/**
 * G-FILE-18 无 SFTP 端点：文件面板必须"降级但活着"（v1.10.3/v1.10.4 回归类）。
 *
 * v1.10.3：SCP/无 SFTP 会话曾渲染成历史遗留表单（与其他场景不一致）；
 * v1.10.4：SFTP 探测失败触发 shell 回退时，su 空密码导致 10~20s 假死。
 *
 * fakessh 无 SFTP 人格（通道直接拒绝）正好逼出 shell 回退路径；回显 shell
 * 无法给出有效列表（结果为空是预期的），此用例守护的是两件事：
 *   1. 面板渲染成完整 FilePane（有路径栏/过滤框），不是遗留表单；
 *   2. FTList 在硬时限内返回（空列表也算），绝不无限挂起，应用不白屏。
 */
test('G-FILE-18: 无 SFTP 端点文件面板降级不挂起、不退化为表单', async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto('/');

  const ep = endpoint();
  const NAME = 'e2e-file-18';

  await quickConnect(page, { name: NAME, host: ep.nosftp.host, port: ep.nosftp.port }, 15_000);
  await openFileTransferTab(page, NAME);

  const remote = remotePane(page);

  // FilePane 形态断言：路径栏（进入按钮）与过滤框必须存在——
  // 退化为旧表单（v1.10.3 回归形态）时二者消失
  await expect(remote.getByRole('button', { name: '进入' })).toBeVisible({ timeout: 15_000 });
  await expect(remote.locator('[data-testid="file-filter-远端"]')).toBeVisible();

  // 后端必须快速给出结论（v1.10.4 回归是 su 空密码假死 10~20s 无反应）：
  // 要么面板弹出分类提示 SFTP_NOT_SUPPORTED，要么 shell 回退完成、列表落定。
  // 两者之一在 15s 内出现即证明 FTList 没有挂起。
  await expect
    .poll(
      async () => {
        const settled = (await remote.innerText()).includes('暂无数据');
        const classified = await page.getByText('SFTP_NOT_SUPPORTED').isVisible().catch(() => false);
        return settled || classified;
      },
      { timeout: 15_000, message: 'SFTP 不可用结论/回退结果必须在时限内返回（挂起即回归）' },
    )
    .toBe(true);

  await expectAppAlive(page);
});
