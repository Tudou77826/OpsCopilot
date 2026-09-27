import { expect, test } from '@playwright/test';
import { endpoint, expectAppAlive, quickConnect } from './_helpers';

/**
 * G-TERM-09 终端会话 smoke：连接 → 输入 → 回显。
 *
 * fakessh 的 shell 是全量回显：客户端发的每个字节原样返回。因此
 * "敲下的命令出现在终端里" 证明的是完整数据面往返：
 *   浏览器键盘 → xterm → Wails 绑定 → sshclient → PTY → fakessh → 原路返回 → DOM 渲染
 * 任何一层断掉（绑定丢失、会话串号、PTY 假死、DOM 渲染崩溃）此用例变红。
 * 时限即断言：每步 10s 内完成，挂起类回归在此暴露。
 */
test('G-TERM-09: 双击连接建立终端，键盘输入经 SSH 往返回显到 DOM', async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto('/');

  const ep = endpoint();
  const NAME = 'e2e-term-09';
  const SENTINEL = 'uptime-e2e-0942';

  const term = await quickConnect(page, { name: NAME, host: ep.sftp.host, port: ep.sftp.port }, 15_000);

  // 连接横幅必须出现（证明读到的是服务端输出，不是本地输入残留）
  await expect
    .poll(() => term.innerText(), { timeout: 10_000, message: '等待 SSH 横幅' })
    .toContain('e2e sftp endpoint');

  // 敲命令 + 回车：fakessh 原样回显，命令文本应出现在终端 DOM 里
  await term.click();
  await page.keyboard.type(SENTINEL, { delay: 15 });
  await page.keyboard.press('Enter');
  await expect
    .poll(() => term.innerText(), { timeout: 10_000, message: '等待输入回显（SSH 数据面往返）' })
    .toContain(SENTINEL);

  await expectAppAlive(page);
});
