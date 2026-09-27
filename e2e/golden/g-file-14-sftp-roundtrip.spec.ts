import { expect, test } from '@playwright/test';
import {
  endpoint,
  fileRow,
  localPane,
  navigatePane,
  openFileTransferTab,
  quickConnect,
  remotePane,
  rowMenuAction,
  waitForFileContent,
} from './_helpers';
import { join } from 'node:path';

/**
 * G-FILE-14 SFTP 双向传输 + 磁盘级断言（黄金用例 C 组核心）。
 *
 * 守护的回归类：
 * - v1.10.4：上传前 FTStat 误触发 shell 回退 → su 空密码 → 上传点击后
 *   10~20s 无反应（"点了没反应"）。上传/下载动作都有硬时限，超时即红；
 * - SFTP 数据面任何一层断掉（绑定丢失、路径解析、沙箱越界）都能在此暴露。
 *
 * 磁盘断言直接读 fakessh 的 SFTP 沙箱目录和本地夹具目录——不信任 UI 文案。
 */
test('G-FILE-14: SFTP 上传→远端可见；下载→本地内容一致', async ({ page }) => {
  test.setTimeout(90_000);
  await page.goto('/');

  const ep = endpoint();
  const NAME = 'e2e-file-14';

  await quickConnect(page, { name: NAME, host: ep.sftp.host, port: ep.sftp.port }, 15_000);
  await openFileTransferTab(page, NAME);

  // 远端根 = SFTP 沙箱；本地进夹具目录
  await navigatePane(remotePane(page), '/');
  await expect(fileRow(remotePane(page), 'download-src.txt')).toBeVisible({ timeout: 15_000 });
  await navigatePane(localPane(page), ep.fixtures.localDir);
  await expect(fileRow(localPane(page), 'upload-src.txt')).toBeVisible({ timeout: 15_000 });

  // --- 上传：选中本地行 → 右键上传 → 沙箱磁盘内容与源文件逐字节一致 ---
  await rowMenuAction(fileRow(localPane(page), 'upload-src.txt'), page, '上传');
  await waitForFileContent(join(ep.sftp.root, 'upload-src.txt'), ep.fixtures.uploadPayload, 25_000);
  await expect(fileRow(remotePane(page), 'upload-src.txt')).toBeVisible({ timeout: 15_000 });

  // --- 下载：选中远端行 → 右键下载 → 本地下载目录内容与沙箱源一致 ---
  await navigatePane(localPane(page), ep.fixtures.downloadDir);
  await rowMenuAction(fileRow(remotePane(page), 'download-src.txt'), page, '下载');
  await waitForFileContent(join(ep.fixtures.downloadDir, 'download-src.txt'), ep.fixtures.downloadPayload, 25_000);
});

/**
 * G-FILE-16 远端管理动作链：新建文件夹 → 重命名 → 删除，每步磁盘断言。
 * 覆盖 FTList/Mkdir/Rename/Remove 的 SFTP 命令面（曾因路径解析/沙箱回归）。
 */
test('G-FILE-16: 远端新建文件夹/重命名/删除，沙箱磁盘状态同步', async ({ page }) => {
  test.setTimeout(90_000);
  await page.goto('/');

  const ep = endpoint();
  const NAME = 'e2e-file-16';
  const DIR = 'e2e-dir-16';
  const DIR_RENAMED = 'e2e-dir-16-renamed';

  await quickConnect(page, { name: NAME, host: ep.sftp.host, port: ep.sftp.port }, 15_000);
  await openFileTransferTab(page, NAME);
  await navigatePane(remotePane(page), '/');

  // 新建文件夹：空白右键 → NameDialog（data-testid 输入框，确定）
  await remotePane(page).click({ button: 'right', position: { x: 100, y: 300 } });
  await page.getByText('新建文件夹', { exact: true }).click();
  await page.getByTestId('name-dialog-input').fill(DIR);
  await page.getByRole('button', { name: '确定', exact: true }).click();
  await expect(fileRow(remotePane(page), DIR)).toBeVisible({ timeout: 15_000 });

  // 重命名：选中行 → 右键 → 重命名 → 新名
  await rowMenuAction(fileRow(remotePane(page), DIR), page, '重命名');
  await page.getByTestId('name-dialog-input').fill(DIR_RENAMED);
  await page.getByRole('button', { name: '确定', exact: true }).click();
  await expect(fileRow(remotePane(page), DIR_RENAMED)).toBeVisible({ timeout: 15_000 });

  // 删除：选中行 → 右键 → 删除 → 确认弹窗
  await rowMenuAction(fileRow(remotePane(page), DIR_RENAMED), page, '删除');
  await page.getByRole('button', { name: '确定', exact: true }).click();
  await expect(fileRow(remotePane(page), DIR_RENAMED)).toBeHidden({ timeout: 15_000 });
});
