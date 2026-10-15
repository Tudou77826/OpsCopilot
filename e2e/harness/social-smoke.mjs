// Real browser -> Wails bindings -> business files -> Keyring -> HTTP -> social.db.
import { chromium, expect } from '@playwright/test';
import assert from 'node:assert/strict';
import { readFile, writeFile, mkdir } from 'node:fs/promises';
import path from 'node:path';
import { startSocialReview } from './social-launch.mjs';
import { captureCompanionReview } from './companion-review.mjs';

const environment = await startSocialReview({port:Number(process.env.SOCIAL_TEST_PORT || 34115)});
const ep = environment.endpoint;
const checks = [];
let browser;
let page;
const check = description => { checks.push(description); process.stdout.write(`[social-test] PASS ${description}\n`); };
try {
  browser = await chromium.launch({ headless: true, channel: process.env.E2E_BROWSER_CHANNEL || 'msedge' });
  page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  const go = (method, ...args) => page.evaluate(async ({ method, args }) => window.go.main.App[method](...args), { method, args });
  async function openMedals() {
    await page.goto(ep.appURL);
    await page.waitForFunction(() => !!window.go?.main?.App);
    // The Wails HTTP listener precedes the native OnStartup callback. Wait for
    // real service initialization before mounting the browser settings page.
    await expect.poll(async () => (await go('GetServiceCenterSettings')).choice, { timeout: 20000 }).toBe('disabled');
    await page.reload();
    await expect(page.locator('.sc-overlay')).toHaveCount(0);
    await page.getByTestId('nav-icon-companion').click();
    await expect(page.getByRole('heading', { name: '伙伴助手', exact: true })).toBeVisible();
    await page.getByRole('button', { name: /^(开启养成|养成与奖章)$/ }).click();
    if (!(await go('GetSettings')).experimental?.companion_enabled) {
      await page.getByText('高级选项', { exact: true }).click();
      await page.getByLabel('启用伙伴养成与功能奖章', { exact: true }).check();
      await page.getByRole('button', { name: '保存更改', exact: true }).click();
    }
    await page.getByRole('navigation').getByText('伙伴与奖章', { exact: true }).click();
    await expect(page.locator('.oc-achievements-medal')).toHaveCount(8);
    await expect(page.locator('.oc-achievements')).toHaveAttribute('aria-busy', 'false');
  }
  const stats = status => Object.fromEntries(status.summary.progress.map(item => [item.kind, item.count]));
  async function api(member, method, route, body, expected = 200) {
    const response = await fetch(ep.serviceURL + route, { method, headers: { Authorization: `Bearer ${member.token}`, 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) });
    assert.equal(response.status, expected, `${method} ${route}`);
    return response.json();
  }
  await openMedals();
  assert.equal(Object.values(stats(await go('AchievementStatus'))).reduce((a, b) => a + b, 0), 0);
  check('真实桌面设置中显示八类奖章，新存档计数为零');
  await page.getByRole('button', { name: '免费开蛋', exact: true }).click();
  await expect(page.getByRole('button', { name: '修改名字', exact: true })).toBeVisible();
  const adopted = await go('CompanionSnapshot');
  assert.equal(adopted.pet.xp, 0); assert.equal(adopted.pet.paletteId, 'mint');
  await page.getByRole('button', { name: '休息一会', exact: true }).click();
  await expect(page.locator('.oc-companion-scene img')).toHaveAttribute('data-action', 'sleep');
  await page.getByRole('button', { name: '打个招呼', exact: true }).click();
  await expect(page.locator('.oc-companion-scene img')).toHaveAttribute('data-action', 'greet');
  await page.getByRole('button', { name: '修改名字', exact: true }).click();
  await page.getByLabel('伙伴名称', { exact: true }).fill('本地微光二号');
  await page.getByRole('button', { name: '保存名称', exact: true }).click();
  await expect(page.locator('.oc-companion-overview')).toContainText('本地微光二号');
  await assert.rejects(go('CompanionRename', '过期修改', adopted.revision));
  await assert.rejects(go('CompanionHatch', (await go('CompanionSnapshot')).revision));
  check('免费开蛋、改名与本地回应通过正式接口；陈旧编辑和不足50点的重开被拒绝，操作不赠送经验');

  const scripts = [];
  for (let i = 0; i < 3; i++) {
    const script = await go('CreateScript', `本地实测脚本 ${i + 1}`, '本地测试产物');
    script.commands = [{ content: `echo social-script-${i + 1}`, enabled: true, delay: 0, comment: '' }];
    await go('UpdateScript', script);
    await assert.rejects(go('UpdateScript', script)); // stale editor version must be rejected
    await go('UpdateScript', await go('LoadScript', script.id));
    scripts.push(script);
  }
  const commands = [];
  for (let i = 0; i < 3; i++) {
    const command = { id: `social-local-command-${i}`, name: `本地实测指令 ${i + 1}`, content: `echo social-command-${i + 1}`, group: 'default' };
    assert.equal(await go('AddQuickCommand', command), '');
    assert.equal(await go('UpdateQuickCommand', command.id, command), '');
    commands.push(command);
  }
  for (let i = 1; i <= 5; i++) {
    assert.equal(JSON.parse(await go('GetKnowledgeFileContent', `guide-${i}.md`)).success, true);
    await go('GetKnowledgeFileContent', `guide-${i}.md`);
  }
  await go('GetKnowledgeFileContent', 'missing.md');
  const connection = await go('Connect', { name: '本地 SSH 传输实测', host: '127.0.0.1', port: ep.sshPort, user: 'test', password: 'test', protocol: 'ssh' });
  assert.equal(connection.success, true, connection.message);
  for (const script of scripts) { await go('ReplayScript', script.id, connection.sessionId); await go('ReplayScript', script.id, connection.sessionId); }
  for (let i = 0; i < 2; i++) {
    const next = await go('Connect', { name: '经验上限验证', host: '127.0.0.1', port: ep.sshPort, user: 'test', password: 'test', protocol: 'ssh' });
    assert.equal(next.success, true);
  }
  await expect.poll(async () => (await go('CompanionSnapshot')).pet.xp).toBe(12);
  const grown = await go('CompanionSnapshot'); assert.equal(grown.todayXP, 12); assert.equal(grown.level, 2);
  check('实际连接与脚本派发获得经验，重复业务结果按规则结算，每日总上限 12，奖章继续统计');
  for (const command of commands) {
    const dispatched = await go('DispatchQuickCommand', command.id, [connection.sessionId, 'missing-session']);
    assert.equal(dispatched.sent, 1); assert.equal(dispatched.failed, 1);
    await go('DispatchQuickCommand', command.id, [connection.sessionId]);
  }
  await assert.rejects(go('DispatchQuickCommand', commands[0].id, ['missing-session']));
  await go('StartSession', '本地实测：检查服务状态');
  assert.equal(await go('StopSession', '测试配置错误', '已修正配置并验证服务正常'), '已修正配置并验证服务正常');
  for (let i = 1; i <= 2; i++) {
    await go('StartSession', `本地实测归档 ${i}`);
    assert.equal(JSON.parse(await go('ArchiveSession', '测试配置错误', `归档结论 ${i}：已修正并验证`, 'local-test', 'service', '')).success, true);
  }
  const counts = stats(await go('AchievementStatus'));
  assert.deepEqual(counts, { 'knowledge-archive': 2, 'script-created': 3, 'quick-command-created': 3, 'case-completed': 3, 'knowledge-read': 5, 'script-used': 3, 'quick-command-used': 3, 'active-day': 1 });
  const archive = await readFile(path.join(ep.appDir, 'docs/archive/local-test_service.md'), 'utf8');
  assert.ok(archive.includes('归档结论 1') && archive.includes('归档结论 2'));
  check('八类业务结果真实采集；重复保存、查阅、派发不重复计数，失败派发不计数，同文件归档保留两条记录');

  await page.getByRole('button', { name: '刷新进度', exact: true }).click();
  await expect(page.locator('.oc-achievements')).toHaveAttribute('aria-busy', 'false');
  await page.getByLabel('排名服务地址', { exact: true }).fill(ep.serviceURL);
  await page.getByLabel('成员凭据', { exact: true }).fill(ep.members.self.token);
  await page.getByRole('button', { name: '连接成员账号', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('归属');
  await page.getByLabel('将这份本地使用记录归属到我的成员账号').check();
  await page.getByLabel('成员凭据', { exact: true }).fill(ep.members.self.token);
  await page.getByRole('button', { name: '连接成员账号', exact: true }).click();
  await expect(page.getByRole('button', { name: '同意公开八项统计并参与', exact: true })).toBeVisible();
  check('已有本地成果需明确认领，连接凭据后仍未自动参与排名');
  await page.getByRole('button', { name: '同意公开八项统计并参与', exact: true }).click();
  await expect(page.getByRole('button', { name: '退出排名', exact: true })).toBeEnabled();
  for (const label of ['知识沉淀者', '脚本工匠', '知识探索者']) await page.locator('.oc-achievements-medal').filter({ hasText: label }).click();
  await expect(page.locator('.oc-achievements-medal[aria-pressed="true"]')).toHaveCount(3);
  await expect(page.locator('.oc-achievements-medal').filter({ hasText: '指令工匠' })).toBeDisabled();
  await page.getByRole('button', { name: '保存展示选择', exact: true }).click();
  await expect(page.locator('.oc-achievements')).toHaveAttribute('aria-busy', 'false');
  const peerURL = await environment.startPeerApp();
  const peerPage = await browser.newPage();
  peerPage.on('pageerror', error => errors.push(error.message));
  await peerPage.goto(peerURL);
  await peerPage.waitForFunction(() => !!window.go?.main?.App);
  const peerGo = (method, ...args) => peerPage.evaluate(async ({ method, args }) => window.go.main.App[method](...args), { method, args });
  await expect.poll(async () => (await peerGo('GetServiceCenterSettings')).choice, { timeout: 20000 }).toBe('disabled');
  // The second desktop also opts in through the real settings save boundary.
  const peerSettings = await peerGo('GetSettings');
  peerSettings.experimental = { ...peerSettings.experimental, companion_enabled: true };
  assert.equal(await peerGo('SaveSettings', peerSettings), '');
  const peerPet=await peerGo('CompanionHatch',0);
  await peerGo('CompanionRename','同伴琥珀',peerPet.revision);
  const peerScript = await peerGo('CreateScript', '另一位成员的真实脚本', '隔离桌面实测');
  peerScript.commands = [{ content: 'echo peer-script', enabled: true, delay: 0, comment: '' }];
  await peerGo('UpdateScript', peerScript);
  await peerGo('SocialConnect', ep.serviceURL, ep.members.peer.token, true);
  await peerGo('SocialParticipation', true);
  await peerGo('SocialShowcase', ['script-created']);
  const visible = await api(ep.members.peer, 'GET', `/api/v1/social/profiles/${ep.members.self.member.userId}`);
  assert.deepEqual(stats({ summary: visible.achievements }), counts);
  assert.equal(visible.medals.length, 3);
  assert.equal(visible.companion.name, '本地微光二号'); assert.equal(visible.companion.paletteId, 'mint');
  assert.deepEqual(Object.keys(visible.companion).sort(), ['level', 'name', 'paletteId', 'speciesId']);
  for (const forbidden of ['objectId', 'sourceId', 'reference', 'password', 'echo social-script-', ep.appDir]) assert.ok(!JSON.stringify(visible).includes(forbidden));
  await api(ep.members.outsider, 'GET', `/api/v1/social/profiles/${ep.members.self.member.userId}`, undefined, 404);
  await page.getByRole('button', { name: '刷新排名', exact: true }).click();
  await expect(page.locator('.oc-achievements-table tbody tr')).toHaveCount(1);
  await page.locator('.oc-achievements-table').getByRole('button', { name: '本地实测成员', exact: true }).click();
  await expect(page.getByRole('region', { name: '成员资料' })).toBeVisible();
  await page.getByLabel('分类榜').selectOption('script-created');
  await expect(page.locator('.oc-achievements-table tbody tr')).toHaveCount(2);
  await expect(page.locator('.oc-achievements-table tbody tr').first()).toContainText('3 个');
  await page.locator('.oc-achievements-table').getByRole('button', { name: '同组织实测成员', exact: true }).click();
  await expect(page.getByRole('region', { name: '成员资料' })).toContainText('同组织实测成员');
  await expect(page.getByRole('region', { name: '成员资料' })).toContainText('同伴琥珀');
  await expect(page.locator('.oc-companion-public-scene img')).toHaveAttribute('data-palette', 'mint');
  assert.equal((await go('SocialRankings', 'script-created', 0, 20, '')).entries[1].count, 1);
  check('两个独立桌面真实保存并同步；三枚精选上限、分类排名和他人资料可查看；跨组织不可见，公开资料无业务明细');

  await go('DeleteScript', scripts[2].id);
  assert.equal(await go('DeleteQuickCommand', commands[2].id), '');
  let state = await go('SocialSync');
  assert.equal(stats(state)['script-created'], 2); assert.equal(stats(state)['quick-command-created'], 2);
  assert.equal(stats(state)['script-used'], 3); assert.equal(stats(state)['quick-command-used'], 3);
  const afterDelete = await api(ep.members.peer, 'GET', `/api/v1/social/profiles/${ep.members.self.member.userId}`);
  assert.equal(stats({ summary: afterDelete.achievements })['script-created'], 2);
  const ledgerBefore = await readFile(path.join(ep.appDir, 'achievements.json'), 'utf8');
  assert.ok(!ledgerBefore.includes(ep.members.self.token));
  check('删除重算制作数量，保留已发生的使用数量；成员凭据不进入本地 JSON');

  const oldPage = await api(ep.members.peer, 'GET', '/api/v1/social/rankings?kind=script-created&offset=0&limit=20');
  environment.stopService();
  await page.getByRole('button', { name: '退出排名', exact: true }).click();
  await expect(page.locator('.oc-achievements').getByRole('status')).toContainText('公开资料可能仍可见');
  state = await go('AchievementStatus'); assert.equal(state.participationPending, false);
  const queued = { id: 'social-local-offline', name: '离线保存实测', content: 'echo offline', group: 'default' };
  assert.equal(await go('AddQuickCommand', queued), '');
  await environment.restartApp();
  await openMedals();
  const restored = await go('CompanionSnapshot');
  assert.equal(restored.pet.id, adopted.pet.id); assert.equal(restored.pet.xp, 12); assert.equal(restored.pet.name, '本地微光二号');
  await expect(page.locator('.oc-achievements-medal[aria-pressed="true"]')).toHaveCount(3);
  state = await go('AchievementStatus'); assert.equal(state.participationPending, false); assert.equal(stats(state)['quick-command-created'], 3);
  await environment.startService();
  await page.getByRole('button', { name: '立即同步', exact: true }).click();
  await expect(page.getByRole('button', { name: '同意公开八项统计并参与', exact: true })).toBeEnabled();
  state = await go('AchievementStatus'); assert.equal(state.participationPending, undefined); assert.ok(state.pending > 0);
  await api(ep.members.peer, 'GET', `/api/v1/social/profiles/${ep.members.self.member.userId}`, undefined, 404);
  await api(ep.members.peer, 'GET', `/api/v1/social/rankings?kind=script-created&offset=0&limit=20&snapshotId=${oldPage.snapshotId}`, undefined, 409);
  check('离线退出持久化并明确提示；应用与服务重启后先确认退出，暂停成果上传，旧榜单快照失效');
  await page.getByRole('button', { name: '同意公开八项统计并参与', exact: true }).click();
  await expect(page.getByRole('button', { name: '退出排名', exact: true })).toBeEnabled();
  state = await go('AchievementStatus'); assert.equal(state.pending, 0);
  await go('SocialSync');
  assert.deepEqual(stats(await go('AchievementStatus')), stats(state));
  check('重新参与后读取 Windows Keyring 凭据并补传离线成果，重复同步保持计数稳定');
  await page.getByText('高级选项', { exact: true }).click();
  await page.getByLabel('启用伙伴养成与功能奖章', { exact: true }).uncheck();
  await page.getByRole('button', { name: '保存更改', exact: true }).click();
  await expect(page.getByTestId('nav-icon-companion')).toHaveCount(1);
  await expect(page.getByRole('navigation').getByText('伙伴与奖章', { exact: true })).toHaveCount(1);
  const companionFile = path.join(ep.appDir, 'companion.json');
  const disabledPet = await readFile(companionFile, 'utf8');
  const disabledLedger = await readFile(path.join(ep.appDir, 'achievements.json'), 'utf8');
  const disabledConnection = await go('Connect', { name: '关闭采集验证', host: '127.0.0.1', port: ep.sshPort, user: 'test', password: 'test', protocol: 'ssh' });
  assert.equal(disabledConnection.success, true);
  await assert.rejects(go('CompanionSnapshot'));
  assert.equal(await readFile(companionFile, 'utf8'), disabledPet);
  assert.equal(await readFile(path.join(ep.appDir, 'achievements.json'), 'utf8'), disabledLedger);
  await page.getByLabel('启用伙伴养成与功能奖章', { exact: true }).check();
  await page.getByRole('button', { name: '保存更改', exact: true }).click();
  await expect(page.getByTestId('nav-icon-companion')).toHaveCount(1);
  await page.getByRole('navigation').getByText('伙伴与奖章', { exact: true }).click();
  await expect(page.getByRole('button', { name: '修改名字', exact: true })).toBeVisible();
  const enabledAgain = await go('CompanionSnapshot');
  assert.equal(enabledAgain.pet.id, adopted.pet.id); assert.equal(enabledAgain.pet.xp, 12);
  check('设置关闭保留助手入口、暂停养成采集且存档逐字保留；重新启用恢复同一伙伴和进度');
  await page.getByRole('button', { name: '刷新进度', exact: true }).click();
  await page.getByLabel('分类榜').selectOption('script-created');
  await expect(page.locator('.oc-achievements-table tbody tr')).toHaveCount(2);
  await page.getByRole('button', { name: '查看我的资料', exact: true }).click();
  await expect(page.getByRole('region', { name: '成员资料' })).toBeVisible();
  assert.deepEqual(errors, []);
  const evidence = path.join(ep.runDir, 'evidence'); await mkdir(evidence);
  await page.screenshot({ path: path.join(evidence, 'social-live-desktop.png'), fullPage: true });
  await captureCompanionReview(page, evidence, ep.appURL);
  check('侧栏伙伴承接真实问答，切换保留对话与草稿；养成页明暗主题与两种窗口宽度无溢出，改名和减少动态效果生效，他人资料使用正式素材');
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ testedAt: new Date().toISOString(), appURL: ep.appURL, checks, counts: stats(state), browserErrors: errors }, null, 2));
  process.stdout.write(`[social-test] 全部 ${checks.length} 组实测通过。证据 ${evidence}\n`);
  await browser.close(); browser = undefined;
  if (process.argv.includes('--stop')) environment.stop();
  else process.stdout.write(`[social-test] 环境保留运行：${ep.appURL}/（按 Ctrl+C 停止）。\n`);
} catch (error) {
  const evidence = path.join(ep.runDir, 'evidence');
  await mkdir(evidence, { recursive: true });
  if (page && !page.isClosed()) await page.screenshot({ path: path.join(evidence, 'failure.png'), fullPage: true }).catch(() => {});
  await writeFile(path.join(evidence, 'failure.json'), JSON.stringify({ testedAt: new Date().toISOString(), checks, error: String(error) }, null, 2));
  if (browser) await browser.close();
  environment.stop();
  throw error;
}
