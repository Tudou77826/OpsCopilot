// Real browser regression against a running desktop preview. Only this isolated
// browser context substitutes AI replies; no commands or SSH connections run.
import { chromium, expect } from '@playwright/test';
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';

const out = path.resolve('e2e/.runtime/assistant-icon-layout');
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ channel: 'msedge', headless: true });
const page = await browser.newPage({ viewport: { width: 1100, height: 800 } });
const errors = [];
const checks = {};
page.on('pageerror', error => errors.push(String(error)));
const stable = (a, b) => {
  assert.ok(a && b, 'Button must remain visible');
  for (const key of ['x', 'y', 'width', 'height'])
    assert.ok(Math.abs(a[key] - b[key]) < 1, `${key} moved: ${a[key]} -> ${b[key]}`);
};
try {
  await page.goto(process.env.OPSCOPILOT_UI_URL || 'http://127.0.0.1:34175/');
  await page.waitForFunction(() => window.go?.main?.App?.StartAssistantTurn);
  await page.evaluate(() => {
    const api = window.go.main.App;
    const state = { turnId: 'icon-layout-fixture', target: { tabId: '', connectionId: '' }, phase: 'running', message: '',
      output: Array.from({ length: 55 }, (_, i) => `### 段落 ${i}\n\n用于检查滚动和按钮布局的内容。`).join('\n\n') };
    window.__assistantIconFixture = state;
    // Match the Wails bridge: each response is a fresh decoded snapshot.
    api.StartAssistantTurn = async () => structuredClone(state);
    api.AssistantControlState = async () => structuredClone(state);
    api.StopAssistantTurn = async () => { state.phase = 'stopped'; };
  });
  const chat = page.locator('.ai-chat-panel');
  if (!await chat.isVisible()) await page.getByRole('button', { name: 'AI 助手', exact: true }).click();
  const actions = chat.getByRole('toolbar', { name: '对话操作' });
  await expect(actions.getByRole('button', { name: '新建对话' })).toBeVisible();
  await expect(actions.getByRole('button', { name: '关闭助手' })).toBeVisible();
  const resource = chat.getByRole('button', { name: '管理 Skills 和快捷命令' });
  assert.equal(await resource.evaluate(el => !!el.closest('.ai-composer-toolbar')), true);
  assert.equal(await actions.locator('button').count(), 2);
  await page.screenshot({ path: path.join(out, 'idle.png') });
  checks.headerAndResourcePlacement = true;

  const editor = chat.getByPlaceholder('输入问题…', { exact: true });
  await editor.fill('仅测试界面布局');
  await chat.getByRole('button', { name: '发送', exact: true }).click();
  await chat.locator('.ai-rich-text h3').first().waitFor();
  for (const question of ['补充今天的时间范围', '补充相关服务名称']) {
    await editor.fill(question);
    await chat.getByRole('button', { name: '加入队列', exact: true }).click();
  }
  await page.evaluate(() => {
    const state = window.__assistantIconFixture;
    state.prompt = { id: 'layout-approval', kind: 'approval', title: '确认测试操作', description: '仅验证卡片布局，不执行命令。', multiple: false, expires: '', choices: [] };
  });
  const card = chat.getByRole('region', { name: '确认测试操作' });
  await expect(card).toBeVisible();
  const approval = await card.boundingBox(), activity = await chat.locator('.ai-chat-activity').boundingBox();
  assert.ok(approval.y + approval.height <= activity.y + 1);
  await page.screenshot({ path: path.join(out, 'approval-above-status.png') });
  checks.approvalAboveStatus = true;
  await page.evaluate(() => { delete window.__assistantIconFixture.prompt; });
  await expect(card).toHaveCount(0);
  const history = chat.getByLabel('对话消息');
  const scrollUp = () => history.evaluate(el => { el.scrollTop = 0; el.dispatchEvent(new Event('scroll')); });
  await scrollUp();
  const latest = chat.getByRole('button', { name: '回到最新', exact: true });
  await expect(latest).toBeVisible();
  const before = await latest.boundingBox();
  await page.mouse.move(before.x + before.width / 2, before.y + before.height / 2);
  stable(before, await latest.boundingBox());
  await page.mouse.down();
  stable(before, await latest.boundingBox());
  // Approval/queue/composer changes can resize the message viewport mid-press.
  await chat.locator('.ai-panel-footer').evaluate(el => { el.style.paddingBottom = '80px'; });
  await expect.poll(() => history.evaluate(el => el.clientHeight)).toBeLessThan(500);
  stable(before, await latest.boundingBox());
  await page.mouse.up();
  await expect(latest).toHaveCount(0);
  await expect.poll(() => history.evaluate(el => el.scrollHeight - el.clientHeight - el.scrollTop)).toBeLessThan(2);
  checks.pressAndReflowStable = true;
  checks.singleClickReturnsToLatest = true;
  checks.before = before;

  // Releasing outside must not leave a stale pinned button or invoke the click.
  await scrollUp();
  const second = await latest.boundingBox();
  await page.mouse.move(second.x + 17, second.y + 17);
  await page.mouse.down();
  await chat.locator('.ai-panel-footer').evaluate(el => { el.style.paddingBottom = '0px'; });
  stable(second, await latest.boundingBox());
  const rect = await history.boundingBox();
  await page.mouse.move(rect.x + 5, rect.y + 5);
  await page.mouse.up();
  await expect.poll(async () => (await latest.boundingBox()).y).toBeGreaterThan(second.y + 50);
  assert.equal(await history.evaluate(el => el.scrollTop), 0);
  checks.outsideReleaseRecovers = true;

  // Hover survives a state update hiding the target, then leaves cleanly.
  const third = await latest.boundingBox();
  await page.mouse.move(third.x + 17, third.y + 17);
  await history.evaluate(el => { el.scrollTop = el.scrollHeight; el.dispatchEvent(new Event('scroll')); });
  await expect(latest).toBeVisible();
  await page.mouse.move(10, 10);
  await expect(latest).toHaveCount(0);
  checks.hoverSurvivesScrollUpdate = true;

  await chat.getByRole('button', { name: '停止本轮', exact: true }).click();
  await expect(chat.getByRole('button', { name: '继续发送', exact: true })).toBeEnabled();
  await expect(chat.getByRole('button', { name: '继续发送', exact: true })).toHaveAttribute('title', /不重试/);
  await page.screenshot({ path: path.join(out, 'paused-queue.png') });
  checks.pausedQueueExplainsResume = true;

  await editor.fill('/');
  await expect(chat.getByRole('button', { name: '管理快捷命令', exact: true })).toBeVisible();
  const sameResource = await chat.getByRole('button', { name: '管理快捷命令', exact: true }).evaluate(el => el.querySelector('svg').innerHTML);
  assert.equal(sameResource, await resource.locator('svg').innerHTML());
  await page.screenshot({ path: path.join(out, 'command-menu.png') });
  checks.commandResourceIconConsistent = true;
  await editor.fill('');

  await scrollUp();
  await expect(latest).toBeVisible();
  await actions.getByRole('button', { name: '关闭助手' }).click();
  await expect(latest).not.toBeVisible();
  await page.getByRole('button', { name: 'AI 助手', exact: true }).click();
  await page.setViewportSize({ width: 764, height: 744 });
  await expect(actions.getByRole('button', { name: '关闭助手' })).toBeVisible();
  const a = await actions.boundingBox(), c = await chat.boundingBox();
  assert.ok(a.x >= c.x && a.x + a.width <= c.x + c.width + 1);
  await page.screenshot({ path: path.join(out, 'narrow.png') });
  checks.hiddenPanelHidesFloatingButton = true;
  checks.narrowHeaderFits = true;
  assert.deepEqual(errors, []);
  checks.noPageErrors = true;
  await writeFile(path.join(out, 'result.json'), JSON.stringify(checks, null, 2));
  console.log(JSON.stringify(checks, null, 2));
} catch (error) {
  await page.screenshot({ path: path.join(out, 'failure.png') }).catch(() => {});
  throw error;
} finally {
  await browser.close();
}
