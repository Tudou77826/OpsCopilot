import { expect, test } from '@playwright/test';
import {
  connectSession,
  createConnection,
  endpoint,
  findConnection,
  openSessionsSidebar,
  treeRow,
} from './_helpers';

/**
 * G-CONN-01 新建-落盘-编辑-重连往返（黄金用例 A 组第一条）。
 *
 * 守护的回归类（全部曾在生产发生）：
 * - #71：Wails 入参 rootPassword 因 tag 不匹配被静默丢弃，保存即清空 root 密码；
 * - v1.10.2 修复的树出参丢失：GetConnectionTree 下划线 tag，前端读不到
 *   rootPassword —— 编辑弹窗显示空，改名保存即清空磁盘值；
 * - HostKey 丢失（同机制）。
 *
 * 断言直接读隔离实例目录里的 sessions.json（真实落盘形态，下划线键）——
 * 线格式断言，前端方言问题在此不存在。
 */
test('G-CONN-01: 新建连接保存后 root 密码完整落盘，编辑改名不丢，从树可连接', async ({ page }) => {
  test.setTimeout(90_000);
  await page.goto('/');
  await openSessionsSidebar(page);

  const ep = endpoint();
  const NAME = 'e2e-conn-01';
  const NAME_RENAMED = 'e2e-conn-01-renamed';
  const ROOT_SECRET = 'root-secret-e2e';

  // --- 1. 走真实 UI 新建连接（空白右键 → 新建连接） ---
  await createConnection(page, {
    name: NAME,
    host: ep.sftp.host,
    port: ep.sftp.port,
    rootPassword: ROOT_SECRET,
  });

  // --- 2. 落盘断言：root 密码原样进 sessions.json（下划线键的线格式） ---
  await expect
    .poll(() => findConnection(NAME)?.config?.root_password, { timeout: 8000, message: '等待新连接落盘' })
    .toBe(ROOT_SECRET);
  const saved = findConnection(NAME);
  expect(saved.config.password, '登录密码落盘').toBe(ep.sftp.password);
  expect(saved.config.user, '用户名落盘').toBe(ep.sftp.user);
  expect(String(saved.config.port), '端口落盘').toBe(String(ep.sftp.port));

  // --- 3. 编辑往返：改名保存，root 密码必须原样保留 ---
  await treeRow(page, NAME).click({ button: 'right' });
  await page.getByText('编辑连接', { exact: true }).click();

  // 编辑弹窗预填的 Root 密码必须可见（树出参回归：读不到会显示空）
  await expect(page.getByLabel('Root 密码', { exact: true })).toHaveValue(ROOT_SECRET, { timeout: 8000 });
  await page.getByLabel('连接名称', { exact: true }).fill(NAME_RENAMED);
  await page.getByRole('button', { name: '保存修改' }).click();

  await expect
    .poll(() => findConnection(NAME_RENAMED)?.config?.root_password, {
      timeout: 8000,
      message: '等待改名落盘',
    })
    .toBe(ROOT_SECRET);
  expect(findConnection(NAME), '旧名应已不存在').toBeUndefined();

  // --- 4. 从树双击连接：15s 内必须出终端（挂起类回归的 tripwire） ---
  await connectSession(page, NAME_RENAMED, 15_000);
});
