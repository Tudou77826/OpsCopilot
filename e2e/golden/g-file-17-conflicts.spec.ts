import { expect, test } from '@playwright/test';
import { writeFile } from 'node:fs/promises';
import { join } from 'node:path';
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

/**
 * G-FILE-17 同名冲突处理：覆盖路径的完整往返（上传冲突 + 下载冲突）。
 *
 * 守护的回归类：v1.10.4 的直接邻居——冲突检测（FTStat）与传输（FTUpload/
 * FTDownload）是两条不同链路，历史回归恰恰发生在"同名存在"这个分支
 * （误触发 shell 回退、su 空密码假死、覆盖不生效、误报不存在）。
 *
 * 用本用例专有的文件名（conflict-up/conflict-dl），不与其他用例共享夹具，
 * 第一次传输永远干净，冲突只在第二次出现。
 */
test('G-FILE-17: 上传/下载同名冲突弹确认，覆盖后磁盘内容逐字节更新', async ({ page }) => {
  test.setTimeout(120_000);
  await page.goto('/');

  const ep = endpoint();
  const NAME = 'e2e-file-17';
  const V1 = 'conflict-payload-v1\n';
  const V2 = 'conflict-payload-v2-overwritten\n';
  const localSrc = join(ep.fixtures.localDir, 'conflict-up.txt');
  const remoteUpload = join(ep.sftp.root, 'conflict-up.txt');
  const remoteSrc = join(ep.sftp.root, 'conflict-dl.txt');
  const localDownload = join(ep.fixtures.downloadDir, 'conflict-dl.txt');

  // 先播种夹具文件，再让面板列目录（面板不会感知外部落盘的新文件）
  await writeFile(localSrc, V1);

  await quickConnect(page, { name: NAME, host: ep.sftp.host, port: ep.sftp.port }, 15_000);
  await openFileTransferTab(page, NAME);
  await navigatePane(remotePane(page), '/');
  await navigatePane(localPane(page), ep.fixtures.localDir);

  // --- 上传冲突：第一次干净落盘，改内容重传 → 确认覆盖 → 沙箱内容变为 v2 ---
  await rowMenuAction(fileRow(localPane(page), 'conflict-up.txt'), page, '上传');
  await waitForFileContent(remoteUpload, V1, 25_000);

  await writeFile(localSrc, V2);
  await rowMenuAction(fileRow(localPane(page), 'conflict-up.txt'), page, '上传');
  await expect(page.getByText('远端已存在同名文件')).toBeVisible({ timeout: 10_000 });
  await page.getByRole('button', { name: '覆盖', exact: true }).click();
  await waitForFileContent(remoteUpload, V2, 25_000);

  // --- 下载冲突：第一次干净落盘，远端内容变化后再下载 → 确认覆盖 → 本地变为 v2 ---
  await navigatePane(localPane(page), ep.fixtures.downloadDir);
  await writeFile(remoteSrc, V1);
  await remotePane(page).getByRole('button', { name: '⟳' }).click(); // 面板不感知外部落盘，手动刷新
  await expect(fileRow(remotePane(page), 'conflict-dl.txt')).toBeVisible({ timeout: 10_000 });
  await rowMenuAction(fileRow(remotePane(page), 'conflict-dl.txt'), page, '下载');
  await waitForFileContent(localDownload, V1, 25_000);

  await writeFile(remoteSrc, V2);
  await rowMenuAction(fileRow(remotePane(page), 'conflict-dl.txt'), page, '下载');
  await expect(page.getByText('本地文件已存在')).toBeVisible({ timeout: 10_000 });
  await page.getByRole('button', { name: '覆盖', exact: true }).click();
  await waitForFileContent(localDownload, V2, 25_000);
});
