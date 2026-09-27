import { expect, test } from '@playwright/test';
import {
  createConnection,
  endpoint,
  expectAppAlive,
  flattenConnections,
  readSessions,
  treeRow,
} from './_helpers';

/**
 * G-CONN-05 会话树管理动作：复制连接、删除连接，全部磁盘级断言。
 *
 * 守护的回归类：与 #71 / v1.10.2 同族——树出参/入参的任一字段在"非编辑"
 * 动作（复制、删除、移动）里丢失或错删。核心断言：
 *   - 复制产生的副本必须完整携带 root_password（完整配置副本语义）；
 *   - 删除只删目标一条，不殃及同端点的其他连接。
 */
test('G-CONN-05: 复制连接完整保留 root 密码，删除只删目标且落盘一致', async ({ page }) => {
  test.setTimeout(90_000);
  await page.goto('/');

  const ep = endpoint();
  const NAME = 'e2e-conn-05';
  const SECRET = 'root-secret-conn-05';

  // 用 nosftp 端点：后端按 (协议,主机,端口) 去重，conn-01 已保存 sftp 端点，
  // 再存同端点会被 ErrDuplicateEndpoint 拒绝——树操作用例需要独立端点
  const sameEndpoint = () =>
    flattenConnections(readSessions()).filter(
      (c: any) => c.config?.host === ep.nosftp.host && String(c.config?.port) === String(ep.nosftp.port),
    );

  await createConnection(page, { name: NAME, host: ep.nosftp.host, port: ep.nosftp.port, rootPassword: SECRET });

  // --- 复制连接：副本落盘，root 密码完整 ---
  await treeRow(page, NAME).click({ button: 'right' });
  await page.getByText('复制连接', { exact: true }).click();

  await expect.poll(sameEndpoint, { timeout: 10_000, message: '等待副本落盘' }).toHaveLength(2);
  for (const c of sameEndpoint()) {
    expect(c.config.root_password, '源与副本的 root 密码都必须完整落盘').toBe(SECRET);
  }

  // --- 删除源连接：只删源，副本保留 ---
  await treeRow(page, NAME).first().click({ button: 'right' });
  await page.getByText('删除', { exact: true }).click();
  await page.getByRole('button', { name: '确定', exact: true }).click();

  await expect.poll(sameEndpoint, { timeout: 10_000, message: '等待删除落盘' }).toHaveLength(1);

  await expectAppAlive(page);
});
