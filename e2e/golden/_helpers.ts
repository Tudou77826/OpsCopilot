// 黄金用例共享助手：端点信息、落盘断言、通用 UI 动作。
// 文件名以 _ 开头，Playwright 不会把它当用例收集。
import { expect, type Locator, type Page } from '@playwright/test';
import { existsSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';

export interface EndpointInfo {
  appUrl: string;
  appDir: string;
  sftp: { host: string; port: number; user: string; password: string; root: string };
  nosftp: { host: string; port: number; user: string; password: string };
  bulk: { host: string; port: number; user: string; password: string; earlyToken: string; lastMarker: string };
  fixtures: {
    localDir: string;
    downloadDir: string;
    uploadSrc: string;
    uploadPayload: string;
    downloadPayload: string;
  };
}

let cached: EndpointInfo | null = null;

/** harness 写下的环境接线（端口、目录），测试进程直接读。 */
export function endpoint(): EndpointInfo {
  if (!cached) {
    const file = resolve(process.cwd(), '.runtime', 'endpoint.json');
    cached = JSON.parse(readFileSync(file, 'utf-8')) as EndpointInfo;
  }
  return cached;
}

// ── 落盘断言（线格式，绕开一切前端方言） ──

/** 读应用目录里的 sessions.json（下划线键的真实落盘形态）。 */
export function readSessions(): any[] {
  return JSON.parse(readFileSync(resolve(endpoint().appDir, 'sessions.json'), 'utf-8'));
}

/** 展平连接树，返回全部连接节点。 */
export function flattenConnections(nodes: any[]): any[] {
  const out: any[] = [];
  for (const n of nodes ?? []) {
    if (n.type === 'connection' || n.config) out.push(n);
    if (n.children) out.push(...flattenConnections(n.children));
  }
  return out;
}

export function findConnection(name: string): any | undefined {
  return flattenConnections(readSessions()).find((c) => c.name === name);
}

/** 轮询等待文件出现且内容等于 expected（磁盘级断言，容忍传输完成的时间差）。 */
export async function waitForFileContent(file: string, expected: string, timeoutMs = 20_000) {
  await expect
    .poll(
      async () => {
        try {
          return readFileSync(file, 'utf-8');
        } catch {
          return '';
        }
      },
      { timeout: timeoutMs, message: `等待文件落盘: ${file}` },
    )
    .toBe(expected);
}

export async function expectFileGone(file: string, timeoutMs = 20_000) {
  await expect
    .poll(() => existsSync(file), { timeout: timeoutMs, message: `等待文件消失: ${file}` })
    .toBe(false);
}

// ── 通用 UI 动作 ──

/** 打开右侧导航的会话管理侧栏（点击可能早于 React 挂载完成，按状态重试）。 */
export async function openSessionsSidebar(page: Page) {
  const nav = page.locator('[title="会话管理"]');
  await expect(nav).toBeVisible({ timeout: 8000 });
  const tree = page.locator('[data-testid="session-tree"]');
  for (let i = 0; i < 3; i++) {
    if (await tree.isVisible()) return;
    await nav.click();
    await page.waitForTimeout(800);
  }
  await expect(tree).toBeVisible({ timeout: 5000 });
}

/** 会话树里的连接行（按名称）。 */
export function treeRow(page: Page, name: string): Locator {
  return page.locator('[data-testid^="tree-row-"]').filter({ hasText: name });
}

export interface ConnFormOpts {
  name: string;
  host: string;
  port: number;
  user?: string;
  password?: string;
  rootPassword?: string;
}

/** 走真实 UI 建一个连接并保存进会话树（树空白右键 → 新建连接 → 表单 → 保存）。 */
export async function createConnection(page: Page, opts: ConnFormOpts) {
  await openSessionsSidebar(page);
  // 空白右键：点树容器底部（避开已有行；顶部坐标在空树时可能落到占位文案外）
  const tree = page.locator('[data-testid="session-tree"]');
  const box = await tree.boundingBox();
  await tree.click({
    button: 'right',
    position: { x: Math.min(60, (box?.width ?? 100) / 2), y: Math.max(5, (box?.height ?? 40) - 15) },
  });
  await page.getByText('新建连接', { exact: true }).click();

  await expect(page.getByLabel('连接名称', { exact: true })).toBeVisible({ timeout: 8000 });
  await page.getByLabel('连接名称', { exact: true }).fill(opts.name);
  await page.getByLabel('主机地址', { exact: true }).fill(opts.host);
  await page.getByLabel('端口', { exact: true }).fill(String(opts.port));
  await page.getByLabel('用户名', { exact: true }).fill(opts.user ?? 'test');
  await page.getByLabel('密码', { exact: true }).fill(opts.password ?? 'test');
  if (opts.rootPassword !== undefined) {
    await page.getByLabel('Root 密码', { exact: true }).fill(opts.rootPassword);
  }
  await page.getByRole('button', { name: /^保存/ }).click();

  await expect(treeRow(page, opts.name)).toBeVisible({ timeout: 8000 });
}

/**
 * 快速连接（不落盘）：顶栏「+ 新建连接」→ 手动添加连接 → 内联表单 → 连接选中项。
 * 只提交连接动作，不等结果——成功/失败路径各自断言。
 */
export async function attemptConnect(
  page: Page,
  opts: { name: string; host: string; port: number; user?: string; password?: string },
) {
  await page.getByRole('button', { name: '+ 新建连接' }).click();
  await page.getByRole('button', { name: '手动添加连接' }).click();

  // 智能对话框内联卡片的"连接名称"是 placeholder 字段，无 label
  const nameInput = page.getByPlaceholder('连接名称', { exact: true });
  await expect(nameInput).toBeVisible({ timeout: 8000 });
  await nameInput.fill(opts.name);
  await page.getByLabel('主机地址', { exact: true }).fill(opts.host);
  await page.getByLabel('端口', { exact: true }).fill(String(opts.port));
  await page.getByLabel('用户名', { exact: true }).fill(opts.user ?? 'test');
  await page.getByLabel('密码', { exact: true }).fill(opts.password ?? 'test');
  await page.getByRole('button', { name: /连接选中项/ }).click();
}

/** 快速连接并等待终端出现；时限即断言——挂起类回归在此变红。 */
export async function quickConnect(
  page: Page,
  opts: { name: string; host: string; port: number; user?: string; password?: string },
  timeoutMs = 15_000,
): Promise<Locator> {
  await attemptConnect(page, opts);
  const term = page.locator('[data-testid^="terminal-container-"]:visible').first();
  await expect(term).toBeVisible({ timeout: timeoutMs });
  return term;
}

/** 双击连接建立终端会话；时限即断言——挂起类回归在此变红。 */
export async function connectSession(page: Page, name: string, timeoutMs = 15_000): Promise<Locator> {
  await treeRow(page, name).dblclick();
  const term = page.locator('[data-testid^="terminal-container-"]:visible').first();
  await expect(term).toBeVisible({ timeout: timeoutMs });
  return term;
}

/** 终端标签右键 → 文件传输，打开该会话的文件面板。 */
export async function openFileTransferTab(page: Page, sessionName: string) {
  const tab = page.locator('.flexlayout__tab_button', { hasText: sessionName }).first();
  await tab.click({ button: 'right' });
  await page.getByText('文件传输', { exact: true }).click();
  await expect(page.locator('[data-testid="file-pane-远端"]')).toBeVisible({ timeout: 15_000 });
}

export function localPane(page: Page): Locator {
  return page.locator('[data-testid="file-pane-本地"]');
}
export function remotePane(page: Page): Locator {
  return page.locator('[data-testid="file-pane-远端"]');
}

/**
 * 面板路径栏输入目录并"进入"。路径输入框是"进入"按钮的前一个兄弟 input。
 * 进入后断言输入框回显目标路径：goLocal/goRemote 是先拉列表再提交路径状态，
 * 回显到位 = 导航真正生效，后续以 localPath/remotePath 为目标的动作
 * （下载落点等）才不会拿到旧目录。
 */
export async function navigatePane(pane: Locator, dir: string) {
  const go = pane.getByRole('button', { name: '进入' });
  const input = go.locator('xpath=preceding-sibling::input[1]');
  await input.fill(dir);
  await go.click();
  await expect(input).toHaveValue(dir, { timeout: 10_000 });
}

/** 面板里的文件行（行元素带 title=文件名）。 */
export function fileRow(pane: Locator, name: string): Locator {
  return pane.locator(`[title="${name}"]`).first();
}

/**
 * 对文件行执行"左键选中 → 右键菜单 → 点菜单项"。
 * 跳过左键直接右键时，菜单项回调读到的选中集合还是旧的（右键同步选中
 * 进不了回调闭包），会走"请先选择"分支——与真实用户操作路径一致为先选再右键。
 */
export async function rowMenuAction(row: Locator, page: Page, itemLabel: string) {
  await row.click();
  await row.click({ button: 'right' });
  await page.getByText(itemLabel, { exact: true }).click();
}

/** React 整树卸载（白屏）守护：#root 必须仍有内容。 */
export async function expectAppAlive(page: Page) {
  const alive = await page.evaluate(() => (document.getElementById('root')?.children.length ?? 0) > 0);
  expect(alive, '应用存活（React 整树卸载/白屏即回归）').toBe(true);
}
