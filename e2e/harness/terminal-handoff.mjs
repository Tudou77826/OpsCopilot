// Real Wails desktop + production SSH client + disposable Linux PTY.
// All configuration and credentials belong to the ignored local fixture.
import { spawn, spawnSync } from 'node:child_process';
import { mkdir, readdir, readFile, writeFile, symlink } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createServer } from 'node:net';
import { randomBytes } from 'node:crypto';
import { chromium, expect } from '@playwright/test';
import { startAssistantModelFixture } from './assistant-model-fixture.mjs';
import { loadTrialModel, runRealModelTrial } from './assistant-real-model.mjs';
import { prepareBusinessSkills, runBusinessSkillsTrial } from './assistant-skills-trial.mjs';
import { prepareDatabaseTrial, runAssistantDatabaseTrial } from './assistant-database-trial.mjs';
import { runFileWorkspaceTrial } from './assistant-file-trial.mjs';
import { runSelectedMachineTrial } from './assistant-selected-machine-trial.mjs';
import { runLocalDirectoriesTrial } from './assistant-local-directories-trial.mjs';
import { runConversationRecoveryTrial } from './assistant-conversation-recovery.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const frontendDist = process.argv.find(value => value.startsWith('--frontend-dist='))?.slice('--frontend-dist='.length) || path.join(root,'frontend/dist');
const runDir = path.join(root, 'e2e/.runtime', `handoff-desktop-${Date.now()}`);
const appDir = path.join(runDir, 'app');
const portArgument = process.argv.find(value => value.startsWith('--port='));
const appPort = portArgument ? Number(portArgument.slice('--port='.length)) : 34125;
if (!Number.isInteger(appPort) || appPort < 1 || appPort > 65535) throw Error('Use --port=<1..65535> for the isolated preview.');
const appURL = `http://127.0.0.1:${appPort}/`;
const endpointFile = path.join(runDir, 'ssh.json');
const processes = [];
const keep = process.argv.includes('--keep');
const previewOnly = process.argv.includes('--preview-only');
const skillsTrial = process.argv.includes('--skills-trial');
const databaseTrial=process.argv.includes('--database-trial');
const fileTrial=process.argv.includes('--file-trial');
const localDirectoriesTrial=process.argv.includes('--local-directories-trial');
const selectedMachineTrial=process.argv.includes('--selected-machine-trial');
const conversationTrial=process.argv.includes('--conversation-trial');
const jdbcRuntime=process.argv.find(v=>v.startsWith('--jdbc-runtime='))?.slice('--jdbc-runtime='.length);
if (previewOnly && !keep) throw Error('--preview-only requires --keep; it starts a preview without running assertions.');
const chrysExecutable = process.argv.find(value => value.startsWith('--icode='))?.slice('--icode='.length)
  || process.argv.find(value => value.startsWith('--chrys='))?.slice('--chrys='.length);
if (skillsTrial && (!chrysExecutable || previewOnly)) throw Error('--skills-trial requires Chrys and runs assertions.');
const realModelConfig = process.argv.find(value => value.startsWith('--real-model-config='))?.slice('--real-model-config='.length);
if (realModelConfig && !chrysExecutable) throw Error('Real model trials require --chrys=<absolute path>.');
const realModel = realModelConfig ? await loadTrialModel(realModelConfig) : undefined;
if(databaseTrial&&(!chrysExecutable||previewOnly))throw Error('--database-trial requires Chrys and assertions');
const modelFixture = chrysExecutable && !realModel ? await startAssistantModelFixture() : undefined;
if (conversationTrial && (!modelFixture || previewOnly || databaseTrial)) throw Error('--conversation-trial requires simulated Chrys and assertions, without --database-trial.');
if(fileTrial&&(!modelFixture||previewOnly||databaseTrial||conversationTrial))throw Error('--file-trial requires simulated Chrys and assertions');
if(localDirectoriesTrial&&(!modelFixture||previewOnly||databaseTrial||conversationTrial||fileTrial||selectedMachineTrial||skillsTrial))throw Error('--local-directories-trial requires simulated Chrys and assertions without other trials');
if(selectedMachineTrial&&(!realModel||previewOnly||databaseTrial||conversationTrial||fileTrial))throw Error('--selected-machine-trial requires real model configuration and assertions');
let stopping = false;
let cleaning = false;
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
async function until(fn, label) {
  const deadline = Date.now() + 60000;
  while (Date.now() < deadline) { try { const value = await fn(); if (value) return value; } catch {} await delay(200); }
  throw Error(`Timed out: ${label}`);
}
function build(exe, args, cwd = root, env = {}) {
  const result = spawnSync(exe, args, { cwd, env: {...process.env,...env}, stdio: 'inherit', windowsHide: true });
  if (result.error || result.status !== 0) throw Error(`Build failed: ${exe}`);
}
function launch(exe, args, cwd, env = {}) {
  const child = spawn(exe, args, { cwd, env: { ...process.env, ...env }, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] });
  processes.push(child);
  child.stdout.on('data', data => process.stdout.write(data));
  child.stderr.on('data', data => process.stderr.write(data));
  child.once('error', error => { console.error(`Trial child failed: ${error.message}`); stop(); process.exitCode = 1; });
  child.once('exit', code => { if (!stopping && !cleaning) { console.error(`Trial dependency exited (${code}); stopping the preview together.`); stop(); process.exitCode = 1; } });
  return child;
}
function stop() {
  if (stopping) return;
  stopping = true;
  modelFixture?.close();
  for (const child of processes) if (child.pid && child.exitCode === null) spawnSync('taskkill', ['/PID', String(child.pid), '/T', '/F'], { windowsHide: true, stdio: 'ignore' });
}
process.once('exit', stop);
process.once('SIGINT', () => { stop(); process.exit(130); });
process.once('SIGTERM', () => { stop(); process.exit(143); });
// Refuse an occupied port before launching anything. Otherwise readiness checks
// could accidentally exercise an unrelated preview already using this URL.
await new Promise((resolve, reject) => {
  const probe = createServer();
  probe.once('error', reject);
  probe.listen(appPort, '127.0.0.1', () => probe.close(error => error ? reject(error) : resolve()));
});
await mkdir(path.join(appDir, 'docs'), { recursive: true });
const business = skillsTrial ? await prepareBusinessSkills(root, appDir) : undefined;
const databaseEndpoint=databaseTrial?await prepareDatabaseTrial(appDir,process.argv.find(v=>v.startsWith('--database-endpoint='))?.slice('--database-endpoint='.length),jdbcRuntime):undefined;
if(previewOnly&&jdbcRuntime)await symlink(jdbcRuntime,path.join(appDir,'jdbc-runtime'),'junction');
await writeFile(path.join(appDir, 'config.json'), JSON.stringify({
  experimental: { companion_enabled: false },
  docs: { dir: path.join(appDir, 'docs') }, scripts: { dir: path.join(appDir, 'scripts') },
  log: { dir: path.join(appDir, 'logs') }, appearance: { theme: 'dark' },
  llm: realModel || { BaseURL: modelFixture?.baseURL || 'http://127.0.0.1:1/v1', APIKey: chrysExecutable ? 'local-fixture-key' : '', Model: chrysExecutable ? 'fixture' : 'no-ai-fixture', ComplexModel: chrysExecutable ? 'fixture' : undefined },
  patch_store: { enabled: false }, session_share: { enabled: false },
}));
const nativeConfig = path.join(appDir,'native-appdata','chrys');
if(chrysExecutable){
  const model=realModel || {BaseURL:modelFixture.baseURL,APIKey:'local-fixture-key',Model:'fixture'};
  await mkdir(path.join(nativeConfig,'models'),{recursive:true});
  await writeFile(path.join(nativeConfig,'settings.yaml'),'model.profile.active: fixture\n');
  await writeFile(path.join(nativeConfig,'models','fixture.yaml'),JSON.stringify({id:'fixture',name:'Trial model',provider:'openai',model_id:model.ComplexModel||model.Model,base_url:model.BaseURL,api_key:model.APIKey,stream:false,http_max_retries:0}));
}
await writeFile(path.join(appDir, 'service-center.json'), JSON.stringify({ baseUrl: '', choice: 'disabled' }));
if (!process.argv.includes('--reuse-frontend')) build('cmd.exe', ['/d', '/s', '/c', 'npm run build -- --logLevel warn'], path.join(root, 'frontend'), {VITE_TERMINAL_HANDOFF_LAB:previewOnly?'':'1'});
build('go', ['build', '-tags', 'dev', '-o', path.join(appDir, 'opscopilot-handoff.exe'), '.']);
const wslRoot = spawnSync('wsl.exe', ['-d', 'docker-desktop', '-e', 'wslpath', '-a', root], { encoding: 'utf8', env: { ...process.env, WSL_UTF8: '1' }, windowsHide: true });
const linuxRoot = wslRoot.stdout?.trim();
if (wslRoot.error || wslRoot.status !== 0 || !linuxRoot?.startsWith('/')) throw Error('Cannot resolve the isolated WSL fixture directory.');
launch('go', ['test', './experiments/terminalhandoff', '-run', '^TestDesktopFixture$', '-v', '-count=1', keep ? '-timeout=0' : '-timeout=21m'], root, {
  OPSCOPILOT_HANDOFF_FILES: fileTrial?'1':'', WSL_UTF8: '1', OPSCOPILOT_HANDOFF_FIXTURE: `${linuxRoot}/e2e/.runtime/terminal-handoff/pty-lab`,
  OPSCOPILOT_HANDOFF_SHELLS: '/bin/sh', OPSCOPILOT_HANDOFF_DESKTOP: endpointFile,
  OPSCOPILOT_HANDOFF_ADDR: '',
  OPSCOPILOT_HANDOFF_KEEP: keep ? '1' : '', OPSCOPILOT_HANDOFF_OWNER_PID: String(process.pid),
});
const endpoint = await until(async () => JSON.parse(await readFile(endpointFile, 'utf8')), 'SSH fixture');
launch(path.join(appDir, 'opscopilot-handoff.exe'), [], appDir, { devserver: `127.0.0.1:${appPort}`, assetdir: frontendDist, ...(chrysExecutable ? { APPDATA:path.dirname(nativeConfig), CHRYS_MODEL_PROFILE: '', OPSCOPILOT_ICODE_EXECUTABLE: chrysExecutable, OPSCOPILOT_CHRYS_EXECUTABLE: '' } : {}) });
await until(async () => (await fetch(appURL)).ok, 'desktop HTTP');
const browser = await chromium.launch({ channel: 'msedge', headless: true });
const page = await browser.newPage({ viewport: { width: 1360, height: 900 } });
const errors = [];
page.on('pageerror', error => errors.push(error.message));
const checks = {};
try {
  await page.goto(appURL);
  await page.waitForFunction(() => !!window.go?.main?.App?.TerminalHandoffStates);
  if(localDirectoriesTrial){
    await runLocalDirectoriesTrial({page,appDir,runDir,modelFixture,checks});
  }else if(selectedMachineTrial){
    await runSelectedMachineTrial({page,appDir,runDir,endpoint,checks});
  }else if(fileTrial){
    await runFileWorkspaceTrial({page,appDir,runDir,endpoint,modelFixture,checks});
  }else if(conversationTrial){
    await runConversationRecoveryTrial({page,appURL,modelFixture,checks,runDir});
  }else if(databaseTrial){
    // Wails also opens a native WebView. Let its initial attach finish, then
    // explicitly load the test page last so presentation receipts come from
    // the page whose controls and ownership shield are under assertion.
    await delay(2500);
    await page.reload();
    await page.waitForFunction(() => !!window.go?.main?.App?.TerminalHandoffStates);
    await runAssistantDatabaseTrial({page,appDir,runDir,checks,endpoint:databaseEndpoint,modelFixture,commandTrial:process.argv.includes('--command-trial')});
  }else{
  async function connect(name) {
    await page.getByRole('button', { name: '+ 新建连接', exact: true }).click();
    await page.getByRole('button', { name: '手动添加连接', exact: true }).click();
    await page.getByPlaceholder('连接名称', { exact: true }).fill(name);
    await page.getByLabel('主机地址', { exact: true }).fill(endpoint.host);
    await page.getByLabel('端口', { exact: true }).fill(String(endpoint.port));
    await page.getByLabel('用户名', { exact: true }).fill(endpoint.user);
    await page.getByLabel('密码', { exact: true }).fill(endpoint.password);
    await page.getByRole('button', { name: /连接选中项/ }).click();
    // An existing terminal can already have a visible handoff button. Wait for
    // the new tab to be added/selected before a later click selects an old tab.
    const addedTab = page.locator('.flexlayout__tab_button').filter({hasText:name}).first();
    await expect(addedTab).toBeVisible();
    await expect(addedTab).toHaveClass(/flexlayout__tab_button--selected/);
    await expect(page.locator('.oc-handoff:visible').getByRole('button', { name: /接手测试/ })).toBeVisible();
    // Fixture SSH acceptance precedes the actual interactive shell startup.
    await expect(page.locator('.oc-handoff:visible .xterm-rows')).toContainText('LAB>',{timeout:15000});
  }
  if (chrysExecutable && !previewOnly) {
    await page.getByRole('button', {name:/AI|伙伴助手/}).last().click();
    const chat = page.locator('.ai-chat-panel');
    await expect(chat.getByRole('heading',{name:'AI 助手',exact:true})).toHaveCount(1);
    await expect(page.locator('.sidebar-chat > div > .sidebar-header')).toHaveCount(0);
    await expect(chat.getByRole('button',{name:'添加上下文'})).toHaveCount(0);
    await expect(chat.getByLabel('助手模式')).toHaveCount(0);
    await expect(chat.getByText('知识问答',{exact:true})).toHaveCount(0);
    async function askWithoutSSH(question) {
      const previous = await page.evaluate(()=>window.go.main.App.AssistantControlState());
      await chat.getByPlaceholder('输入问题…',{exact:true}).fill(question);
      await chat.getByTitle('发送',{exact:true}).click();
      const result = await until(async()=>{
        const current=await page.evaluate(()=>window.go.main.App.AssistantControlState());
        if (!current.turnId || current.turnId===previous.turnId) return false;
        return current.prompt || ['completed','failed','stopped'].includes(current.phase) ? current : false;
      },'assistant without SSH');
      if (result.prompt) throw Error('No-SSH chat requested terminal authorization');
      if (result.phase!=='completed') throw Error(`No-SSH chat failed: ${result.message}`);
      if (!result.output || result.target?.tabId) throw Error('Conversation acquired a terminal dependency');
      if ((await page.evaluate(()=>window.go.main.App.TerminalHandoffStates())).length) throw Error('Chat created an SSH session');
      await expect(chat.getByRole('button',{name:'新建对话'})).toBeEnabled();
      return result;
    }
    await askWithoutSSH('你叫什么名字？');
    checks.chatWithoutSSH = true;
    const conversationMarker = `OC_SESSION_${randomBytes(8).toString('hex')}`;
    await askWithoutSSH(`请记住本段对话的测试代号：${conversationMarker}。只回复已记住，不调用工具。`);
    const recall = await askWithoutSSH('刚才给你的测试代号是什么？只回复完整代号，不调用工具。');
    if (!recall.output.includes(conversationMarker)) throw Error('ACP conversation lost previous message context');
    checks.conversationRetainsModelContext = true;
    if (modelFixture) {
      await askWithoutSSH('E2E_LONG_READ 只生成界面测试文字，不调用工具。');
      const editor = chat.getByPlaceholder('输入问题…',{exact:true});
      const history = chat.getByLabel('对话消息');
      await editor.fill('E2E_DRAFT_REPLY 请简短回答，不调用工具。');
      await chat.getByTitle('发送',{exact:true}).click();
      await expect(chat.getByRole('button',{name:'停止本轮',exact:true})).toBeVisible();
      await expect(editor).toBeEnabled();
      await editor.fill('本轮结束后保留的草稿');
      await history.evaluate(element=>{element.scrollTop=0;element.dispatchEvent(new Event('scroll'));});
      await expect(chat.getByRole('button',{name:'新建对话'})).toBeEnabled({timeout:45000});
      await expect(editor).toHaveValue('本轮结束后保留的草稿');
      if (await history.evaluate(element=>element.scrollTop)>30) throw Error('New response stole history scroll position');
      await expect(chat.getByRole('button',{name:'回到最新'})).toBeVisible();
      await page.screenshot({path:path.join(runDir,'assistant-reading-history.png')});
      await chat.getByRole('button',{name:'回到最新'}).click();
      if (!(await history.evaluate(element=>element.scrollHeight-element.clientHeight-element.scrollTop<56))) throw Error('Return to latest did not scroll to bottom');
      checks.busyEditorPreservesDraft = true;
      checks.readingHistoryDoesNotAutoScroll = true;
      await editor.fill('E2E_QUEUE_FIRST');
      await chat.getByTitle('发送',{exact:true}).click();
      await expect(chat.getByRole('button',{name:'停止本轮',exact:true})).toBeVisible();
      for (const question of ['E2E_QUEUE_SECOND', 'E2E_QUEUE_THIRD']) {
        await editor.fill(question);
        await editor.press('Enter');
      }
      await expect(chat.getByLabel('待发送消息')).toContainText('E2E_QUEUE_SECOND');
      await expect(chat.getByLabel('待发送消息')).toContainText('E2E_QUEUE_THIRD');
      await expect(chat.getByRole('button',{name:'新建对话'})).toBeEnabled({timeout:45000});
      const sequence = modelFixture.checks.questions.filter(question=>question.includes('E2E_QUEUE_'));
      if (sequence.length!==3 || !sequence[0].includes('FIRST') || !sequence[1].includes('SECOND') || !sequence[2].includes('THIRD')) throw Error('Queued messages did not execute in FIFO order');
      checks.messageQueueRunsSerially = true;
    }
    const unavailable = await askWithoutSSH('请读取当前终端屏幕，告诉我显示什么。如果没有可用 SSH 连接，请直接说明，不要猜测。');
    if (!/SSH|连接/.test(unavailable.output)) throw Error('Missing SSH tool requirement was not explained');
    checks.unavailableTerminalDoesNotBreakConversation = true;
    await page.screenshot({path:path.join(runDir,'assistant-without-ssh.png')});
    await chat.getByRole('button',{name:'新建对话'}).click();
    await expect(chat.getByRole('button',{name:'新建对话'})).toBeEnabled();
    const fresh = await askWithoutSSH('本段对话的测试代号是什么？如果本段对话没有提供测试代号，只回复“未提供代号”，不要猜测，不调用工具。');
    if (fresh.output.includes(conversationMarker) || !fresh.output.includes('未提供代号')) throw Error('New chat retained old model context');
    checks.newConversationClearsModelContext = true;
    await chat.getByRole('button',{name:'新建对话'}).click();
    await expect(chat.getByRole('button',{name:'新建对话'})).toBeEnabled();
    await page.getByRole('button', {name:/AI|伙伴助手/}).last().click();
  }
  if (!previewOnly) await connect('接手试用 · 原终端');
  const first = previewOnly ? undefined : (await page.evaluate(() => window.go.main.App.TerminalHandoffStates()))[0].sessionId;
  async function human(command) {
    await page.locator('.oc-handoff:visible .xterm-screen').click();
    // Paste one command through the real xterm input path. Sending individual
    // synthetic keys needlessly races Wails delivery on a loaded test machine.
    await page.keyboard.insertText(command);
    await page.keyboard.press('Enter');
  }
  async function selectTab(name) {
    const tab = page.locator('.flexlayout__tab_button').filter({ hasText: name }).first();
    await tab.locator('.flexlayout__tab_button_content').click();
    await expect(tab).toHaveClass(/flexlayout__tab_button--selected/);
  }
  if (!previewOnly) {
  await human(String.raw`cd /tmp; export OC_DEMO=retained; printf 'BEFORE_PID=%s\n' "$$"`);
  await expect(page.locator('.oc-handoff:visible .xterm-rows')).toContainText(/BEFORE_PID=\d+/);
  const beforeText = await page.locator('.oc-handoff:visible .xterm-rows').innerText();
  const originalPID = beforeText.match(/BEFORE_PID=(\d+)/)?.[1];
  if (!originalPID) throw Error('No original Shell PID');
  async function start(slow = false) {
    const frame = page.locator('.oc-handoff:visible');
    await frame.getByRole('button', { name: /接手测试/ }).click();
    await expect(frame.getByRole('button', { name: '开始测试' })).toBeDisabled();
    await frame.getByRole('radio', { name: slow ? /体验停止/ : /查看当前现场/ }).check();
    await frame.getByRole('checkbox').check();
    await frame.getByRole('button', { name: '开始测试' }).click();
  }
  await start();
  await expect(page.locator('.oc-handoff:visible [role=status]')).toContainText('测试完成', { timeout: 10000 });
  await expect(page.locator('.oc-handoff:visible .xterm-rows')).toContainText(`当前 Shell PID=${originalPID}`);
  await human(String.raw`printf 'AFTER=%s|%s\n' "$PWD" "$OC_DEMO"`);
  await expect(page.locator('.oc-handoff:visible .xterm-rows')).toContainText('AFTER=/tmp|retained');
  checks.sameShellAndEnvironment = true;
  await connect('接手试用 · 独立终端');
  await selectTab('接手试用 · 原终端');
  await start(true);
  await expect(page.getByLabel('接手期间终端由测试助手独占')).toBeVisible();
  await expect(page.locator('.oc-handoff-tab-badge:visible').first()).toContainText('接手测试中');
  await expect(page.locator('.oc-handoff:visible .xterm-rows')).toContainText('慢命令已开始');
  await page.screenshot({ path: path.join(runDir, 'handoff-active.png') });
  const controlled = await page.evaluate(()=>window.go.main.App.TerminalHandoffStates().then(states=>states.find(state=>state.owned)?.sessionId));
  if (controlled !== first) throw Error('Tab selection did not reach the intended terminal');
  const blocked = await page.evaluate(async id => {
    try { await window.go.main.App.SendCommand(id, 'FORBIDDEN_TEST_INPUT\n'); return false; } catch { return true; }
  }, first);
  if (!blocked) throw Error('Input gate accepted a competing writer');
  checks.competingInputBlocked = true;
  await selectTab('接手试用 · 独立终端');
  await human(String.raw`printf 'OTHER_TAB_WORKS\n'`);
  await expect(page.locator('.oc-handoff:visible .xterm-rows')).toContainText('OTHER_TAB_WORKS');
  checks.otherTabUnaffected = true;
  await selectTab('接手试用 · 原终端');
  await page.getByRole('button', { name: '手动接管', exact: true }).click();
  await expect(page.getByLabel('接手期间终端由测试助手独占')).toHaveCount(0);
  await expect(page.locator('.oc-handoff:visible [role=status]')).toContainText('远端命令可能仍在运行');
  await page.locator('.oc-handoff:visible .xterm-screen').click();
  await page.keyboard.press('Control+c');
  await expect(page.locator('.oc-handoff:visible [role=status]')).toContainText('可以继续操作');
  await human(String.raw`printf 'TAKEN_BACK=%s|%s\n' "$PWD" "$OC_DEMO"`);
  await expect(page.locator('.oc-handoff:visible .xterm-rows')).toContainText('TAKEN_BACK=/tmp|retained');
  checks.takeoverAndInterrupt = true;
  await start(true);
  await expect(page.getByLabel('接手期间终端由测试助手独占')).toBeVisible();
  await page.getByRole('button', { name: '停止', exact: true }).click();
  await expect(page.getByLabel('接手期间终端由测试助手独占')).toHaveCount(0);
  await page.locator('.oc-handoff:visible .xterm-screen').click();
  await page.keyboard.press('Control+c');
  checks.stopButton = true;
  await expect(page.locator('.oc-handoff-tab-badge')).toHaveCount(0);
  await page.screenshot({ path: path.join(runDir, 'handoff-released.png') });
  await expect(page.locator('.oc-handoff:visible [role=status]')).toContainText('可以继续操作');
  const oldConnection = await page.evaluate(async id => (await window.go.main.App.TerminalHandoffStates()).find(s => s.sessionId === id).connectionId, first);
  await human('exit');
  const reconnectButton = page.locator('.flexlayout__tab_button').filter({ hasText: '接手试用 · 原终端' }).first().getByRole('button', { name: '重连', exact: true });
  await expect(reconnectButton).toBeVisible();
  await reconnectButton.click();
  await expect(page.locator('.oc-handoff:visible').getByRole('button', { name: /接手测试/ })).toBeVisible();
  await page.waitForFunction(({ id, old }) => window.go.main.App.TerminalHandoffStates().then(states => states.some(s => s.sessionId === id && s.connectionId !== old)), { id: first, old: oldConnection });
  const staleConfirmationRejected = await page.evaluate(async ({ id, old }) => {
    try { await window.go.main.App.StartTerminalHandoffTest(id, 'inspect', true, old); return false; } catch { return true; }
  }, { id: first, old: oldConnection });
  if (!staleConfirmationRejected) throw Error('Reconnected terminal accepted old idle confirmation');
  await start();
  await expect(page.locator('.oc-handoff:visible [role=status]')).toContainText('测试完成');
  checks.reconnectRequiresFreshConfirmation = true;
  await start(true);
  await expect(page.getByLabel('接手期间终端由测试助手独占')).toBeVisible();
  await page.reload();
  await page.waitForFunction(() => !!window.go?.main?.App?.TerminalHandoffStates);
  await page.waitForFunction(id => window.go.main.App.TerminalHandoffStates().then(states => states.every(s => s.sessionId !== id || !s.owned)), first, { timeout: 5000 });
  checks.reloadReleasesOwnership = true;
  // Reload does not claim to terminate the remote process or restore all tabs.
  await page.evaluate(async () => {
    for (const state of await window.go.main.App.TerminalHandoffStates()) await window.go.main.App.CloseSession(state.sessionId);
  });
  await connect('接手试用 · 关闭页签');
  const closing = await page.evaluate(async () => (await window.go.main.App.TerminalHandoffStates()).find(s => s.phase === 'idle').sessionId);
  await start(true);
  await expect(page.getByLabel('接手期间终端由测试助手独占')).toBeVisible();
  await page.locator('.flexlayout__tab_button').filter({ hasText: '接手试用 · 关闭页签' }).first().locator('.flexlayout__tab_button_trailing').click();
  await page.waitForFunction(id => window.go.main.App.TerminalHandoffStates().then(states => states.every(s => s.sessionId !== id)), closing);
  await expect(page.locator('.oc-handoff-tab-badge')).toHaveCount(0);
  checks.closeTabReleasesOwnership = true;
  await connect('接手试用 · 授权确认');
  async function startApproval() {
    const frame = page.locator('.oc-handoff:visible');
    await frame.getByRole('button', { name: /接手测试/ }).click();
    await frame.getByRole('radio', { name: /体验操作确认/ }).check();
    await frame.getByRole('checkbox').check();
    await frame.getByRole('button', { name: '开始测试' }).click();
    await expect(frame.getByRole('button', { name: '允许本次' })).toBeVisible({ timeout: 10000 });
    await expect(frame.locator('.oc-handoff-content')).not.toHaveAttribute('inert');
    await expect(frame.locator('.oc-handoff-shield')).toHaveCount(0);
    return frame;
  }
  let approvalFrame = await startApproval();
  await page.screenshot({ path: path.join(runDir, 'approval-waiting.png') });
  await approvalFrame.getByRole('button', { name: '允许本次' }).click();
  await expect(approvalFrame.locator('[role=status]')).toContainText('授权体验完成', { timeout: 10000 });
  await expect(approvalFrame.locator('.xterm-rows')).toContainText('本次操作已获明确批准');
  checks.approvalAllowsOriginalCommand = true;
  approvalFrame = await startApproval();
  await approvalFrame.getByRole('button', { name: '拒绝', exact: true }).click();
  await expect(approvalFrame.locator('[role=status]')).toContainText('已拒绝');
  checks.approvalRejectsWithoutExecution = true;
  approvalFrame = await startApproval();
  await human(String.raw`printf 'CHANGED_%s\n' 'SITE'`);
  await expect(approvalFrame.locator('.xterm-rows')).toContainText('CHANGED_SITE');
  await page.waitForFunction(() => window.go.main.App.TerminalHandoffStates().then(states => states.some(s => s.ready)));
  await approvalFrame.getByRole('button', { name: '允许本次' }).click();
  await expect(approvalFrame.locator('[role=status]')).toContainText('现场已变化');
  checks.approvalRechecksManualChanges = true;
  approvalFrame = await startApproval();
  const oldWork = await page.evaluate(() => window.go.main.App.AssistantControlState());
  await approvalFrame.getByRole('button', { name: '停止本轮' }).click();
  await expect(approvalFrame.getByRole('button', { name: '允许本次' })).toHaveCount(0);
  const lateRejected = await page.evaluate(async old => {
    try { await window.go.main.App.RespondAssistantInteraction(old.turnId, old.prompt.id, { selected: ['allow'], text: '' }); return false; } catch { return true; }
  }, oldWork);
  if (!lateRejected) throw Error('Stopped approval accepted a late response');
  checks.approvalStopRejectsLateResponse = true;
  approvalFrame = await startApproval();
  await page.reload();
  await page.waitForFunction(() => !!window.go?.main?.App?.AssistantControlState);
  await page.waitForFunction(() => window.go.main.App.AssistantControlState().then(state => state.phase === 'stopped' && !state.prompt));
  checks.approvalReloadWithdrawsRequest = true;
  await page.evaluate(async () => { for (const state of await window.go.main.App.TerminalHandoffStates()) await window.go.main.App.CloseSession(state.sessionId); });
  await connect('接手试用 · 关闭审批页签');
  approvalFrame = await startApproval();
  await page.locator('.flexlayout__tab_button').filter({ hasText: '接手试用 · 关闭审批页签' }).first().locator('.flexlayout__tab_button_trailing').click();
  await page.waitForFunction(() => window.go.main.App.AssistantControlState().then(state => state.phase === 'stopped' && !state.prompt));
  checks.approvalCloseTabWithdrawsRequest = true;
  if (chrysExecutable) {
    await connect('终端助手 · 原命令行');
    // Product UI entry, not a direct arbitrary-command Wails call.
    await page.getByRole('button', {name:/AI|伙伴助手/}).last().click();
    if (skillsTrial) {
      await runBusinessSkillsTrial({page,human,appDir,checks,business,modelFixture,realModel});
    } else if (realModel) {
      await runRealModelTrial({ page, human, appDir, checks });
      await page.screenshot({path:path.join(runDir,'assistant-real-model.png')});
    } else {
    const chat = page.locator('.ai-chat-panel');
    await chat.getByPlaceholder('输入问题…', {exact:true}).fill('你叫什么名字？');
    await chat.getByTitle('发送', {exact:true}).click();
    await until(async()=>{const state=await page.evaluate(()=>window.go.main.App.AssistantControlState());return state.source==='assistant' && state.phase==='running';},'conversation is active');
    await expect(page.locator('.oc-handoff:visible .oc-handoff-shield')).toHaveCount(0);
    await expect(page.locator('.oc-handoff:visible .oc-handoff-content')).not.toHaveAttribute('inert');
    await human("command printf 'CHAT_INPUT_ALLOWED\\n'");
    await expect(page.locator('.oc-handoff:visible .xterm-rows')).toContainText('CHAT_INPUT_ALLOWED');
    await expect(chat.getByText('我是 OpsCopilot。当前预览使用模拟模型，用于体验终端接手、提问和审批流程。', {exact:true})).toBeVisible({timeout:45000});
    await expect(chat.getByRole('heading', {name:'允许助手接手当前终端？'})).toHaveCount(0);
    checks.chrysGeneralQuestionDoesNotTakeTerminal = true;
    checks.conversationKeepsTerminalUsable = true;
    await chat.getByPlaceholder('输入问题…', {exact:true}).fill('E2E_TERMINAL_SCREEN 看下终端有什么内容');
    await chat.getByTitle('发送', {exact:true}).click();
    await expect(chat.getByRole('heading',{name:'允许助手查看当前终端？'})).toBeVisible({timeout:45000});
    await chat.getByRole('button',{name:'允许本轮查看',exact:true}).click();
    await until(()=>modelFixture.checks.nextTool==='final' && modelFixture.checks.scenario?.includes('E2E_TERMINAL_SCREEN'),'passive screen snapshot');
    await expect(page.locator('.oc-handoff:visible .oc-handoff-shield')).toHaveCount(0);
    await human("command printf 'SCREEN_INPUT_ALLOWED\\n'");
    await expect(chat.getByText('已读取当前终端屏幕，终端保持可操作。当前预览仍使用模拟模型。',{exact:true})).toBeVisible({timeout:45000});
    checks.passiveScreenReadDoesNotOwnTerminal = true;
    async function askTerminal(question) {
      await chat.getByPlaceholder('输入问题…', {exact:true}).fill(question);
      await chat.getByTitle('发送',{exact:true}).click();
      await expect(chat.getByRole('heading',{name:'允许助手查看当前终端？'})).toBeVisible({timeout:45000});
      await chat.getByRole('button',{name:'允许本轮查看',exact:true}).click();
    }
    async function allowTake() {
      await expect(chat.getByRole('heading',{name:'允许助手接手当前终端？'})).toBeVisible({timeout:45000});
      await expect(page.locator('.oc-handoff:visible .oc-handoff-shield')).toHaveCount(0);
      await chat.getByRole('button',{name:'允许本轮',exact:true}).click();
    }
    await askTerminal('E2E_TERMINAL_READ 查看当前目录');
    await allowTake();
    await expect(page.locator('.oc-handoff:visible .oc-handoff-shield')).toBeVisible({timeout:45000});
    await until(async()=>{if(modelFixture.checks.nextTool!=='final')return false;const work=await page.evaluate(()=>window.go.main.App.AssistantControlState());const terms=await page.evaluate(()=>window.go.main.App.TerminalHandoffStates());return work.phase==='running' && terms.every(value=>!value.owned);},'model releases before replying');
    await expect(page.locator('.oc-handoff:visible .oc-handoff-shield')).toHaveCount(0);
    await human("command printf 'RELEASE_INPUT_ALLOWED\\n'");
    await expect(chat.getByText('终端助手链路已完成。结果来自原终端。',{exact:true})).toBeVisible({timeout:45000});
    checks.chrysMCPReadFromChat = true;
    checks.modelTakeAndReleaseIndependentOfConversation = true;
    await askTerminal('E2E_TERMINAL_QUESTION 需要先问我日志日期');
    await expect(chat.getByRole('heading',{name:'要检查哪一天的日志？'})).toBeVisible({timeout:45000});
    await expect(page.locator('.oc-handoff:visible .oc-handoff-shield')).toHaveCount(0);
    await chat.getByLabel('今天 · 只看今天').check();
    await chat.getByRole('button',{name:'提交回答',exact:true}).click();
    await allowTake();
    await expect(chat.getByText('终端助手链路已完成。结果来自原终端。',{exact:true})).toHaveCount(2,{timeout:45000});
    checks.chrysQuestionYieldsAndResumes = true;
    await askTerminal('E2E_TERMINAL_APPROVAL 打印测试标记');
    await allowTake();
    await expect(chat.getByRole('heading',{name:'确认这一次终端操作'})).toBeVisible({timeout:45000});
    await expect(page.locator('.oc-handoff:visible .oc-handoff-shield')).toBeVisible();
    await expect(chat.getByText('快速判定',{exact:true})).toBeVisible();
    await expect(chat.getByLabel('将要执行的命令')).toBeVisible();
    await chat.getByText('更多操作详情',{exact:true}).click();
    await expect(chat.getByText('AI 操作目的（供参考）',{exact:true})).toBeVisible();
    const approvalLease = await page.evaluate(()=>window.go.main.App.TerminalHandoffStates().then(states=>states.find(value=>value.owned)?.runId));
    await page.screenshot({path:path.join(runDir,'assistant-held-approval.png')});
    await chat.getByRole('button',{name:'允许本次',exact:true}).click();
    await until(()=>modelFixture.checks.resultCount>=4 && modelFixture.checks.nextTool==='terminal_context','approved command continues within lease');
    const continuingLease = await page.evaluate(()=>window.go.main.App.TerminalHandoffStates().then(states=>states.find(value=>value.owned)?.runId));
    if (!approvalLease || approvalLease!==continuingLease) throw Error('Approval or subsequent read replaced/released the lease');
    await expect(page.locator('.oc-handoff:visible .oc-handoff-content')).toHaveAttribute('inert');
    await expect(chat.getByText('终端助手链路已完成。结果来自原终端。',{exact:true})).toHaveCount(3,{timeout:45000});
    await expect(page.locator('.oc-handoff:visible .xterm-rows')).toContainText('ACP_APPROVED_ONCE');
    checks.chrysHostApprovalRunsOriginalCommand = true;
    checks.approvalAndFollowingCommandsKeepModelLease = true;
    await askTerminal('E2E_TERMINAL_APPROVAL_MANUAL 等待审批时手动接管');
    await allowTake();
    await expect(chat.getByRole('heading',{name:'确认这一次终端操作'})).toBeVisible({timeout:45000});
    const heldPending = await page.evaluate(()=>window.go.main.App.AssistantControlState());
    await page.locator('.oc-handoff:visible').getByRole('button',{name:'手动接管',exact:true}).click();
    await expect(page.locator('.oc-handoff:visible .oc-handoff-shield')).toHaveCount(0);
    await expect(chat.getByRole('heading',{name:'确认这一次终端操作'})).toHaveCount(0);
    const heldLate = await page.evaluate(async old=>{try{await window.go.main.App.RespondAssistantInteraction(old.turnId,old.prompt.id,{text:'',selected:['allow']});return false;}catch{return true;}},heldPending);
    if (!heldLate) throw Error('Manual takeover accepted late held approval');
    checks.manualTakeoverRevokesHeldApproval = true;
    await expect(chat.getByRole('button',{name:'新建对话'})).toBeEnabled({timeout:45000});
    await chat.getByPlaceholder('输入问题…',{exact:true}).fill('E2E_TERMINAL_READ 停止本轮测试');
    await chat.getByTitle('发送',{exact:true}).click();
    await expect(chat.getByRole('heading',{name:'允许助手查看当前终端？'})).toBeVisible({timeout:45000});
    const pending = await page.evaluate(()=>window.go.main.App.AssistantControlState());
    await chat.getByRole('button',{name:'停止本轮',exact:true}).first().click();
    await expect(chat.getByRole('heading',{name:'允许助手查看当前终端？'})).toHaveCount(0);
    const late = await page.evaluate(async old => {try {await window.go.main.App.RespondAssistantInteraction(old.turnId,old.prompt.id,{text:'',selected:['allow']});return false;}catch{return true;}},pending);
    if (!late) throw Error('late ACP consent revived a stopped turn');
    checks.chrysStopRejectsLateConsent = true;
    await expect(chat.getByRole('button',{name:'新建对话'})).toBeEnabled({timeout:45000});
    await askTerminal('E2E_TERMINAL_APPROVAL_QUEUE 等待审批期间立即发送补充要求');
    await allowTake();
    await expect(chat.getByRole('heading',{name:'确认这一次终端操作'})).toBeVisible({timeout:45000});
    const interrupted = await page.evaluate(()=>window.go.main.App.AssistantControlState());
    async function commandDispatchCount() {
      const directory = path.join(appDir,'audit','terminal');
      const files = (await readdir(directory)).filter(name=>name.endsWith('.jsonl'));
      const events = (await Promise.all(files.map(async name=>(await readFile(path.join(directory,name),'utf8')).trim().split('\n').filter(Boolean).map(line=>JSON.parse(line))))).flat();
      return events.filter(event=>event.phase==='dispatch_intent').length;
    }
    const commandsBefore = await commandDispatchCount();
    if (!commandsBefore) throw Error('Command audit baseline is missing');
    const editor = chat.getByPlaceholder('输入问题…',{exact:true});
    await editor.fill('E2E_QUEUE_CANCELLED 必须移除的消息');
    await editor.press('Enter');
    const override = '只看今天订单服务的异常日志，其他日期先不查。请记住本段对话的测试代号：OC_SESSION_0123456789abcdef。只回复代号，不调用工具。E2E_QUEUE_OVERRIDE';
    await editor.fill(override);
    await editor.press('Enter');
    await expect(chat.getByLabel('待发送消息')).toContainText('E2E_QUEUE_CANCELLED');
    await chat.getByRole('button',{name:'移除消息：E2E_QUEUE_CANCELLED 必须移除的消息',exact:true}).click();
    await page.setViewportSize({width:954,height:833});
    async function resizeSidebar(width) {
      const handle = await page.getByRole('separator',{name:'调整侧栏宽度'}).boundingBox();
      const current = await chat.boundingBox();
      if (!handle || !current) throw Error('Sidebar resize handle is missing');
      const x = handle.x + handle.width / 2, y = handle.y + 24;
      await page.mouse.move(x,y);
      await page.mouse.down();
      await page.mouse.move(x+current.width-width,y,{steps:6});
      await page.mouse.up();
      await expect.poll(async()=>Math.round((await chat.boundingBox()).width)).toBe(width);
    }
    for (const width of [320,520]) {
      await resizeSidebar(width);
      const surface = chat.locator('.ai-composer-surface');
      if (!(await surface.evaluate(element=>element.scrollWidth<=element.clientWidth+1))) throw Error('Composer overflowed sidebar');
      await expect(chat.getByRole('button',{name:'立即发送',exact:true})).toBeVisible();
      await expect(chat.getByRole('button',{name:'加入队列',exact:true})).toBeVisible();
      await expect(chat.getByRole('button',{name:'停止本轮',exact:true})).toBeVisible();
      await surface.screenshot({path:path.join(runDir,`assistant-composer-${width}-dark.png`)});
    }
    await page.getByRole('button',{name:'切换到亮色',exact:true}).click();
    await chat.locator('.ai-composer-surface').screenshot({path:path.join(runDir,'assistant-composer-520-light.png')});
    await page.getByRole('button',{name:'切换到暗色',exact:true}).click();
    await resizeSidebar(420);
    await page.setViewportSize({width:1360,height:900});
    checks.composerFitsNarrowAndWideSidebar = true;
    await page.screenshot({path:path.join(runDir,'assistant-message-queue.png')});
    await chat.getByRole('button',{name:'立即发送',exact:true}).click();
    await expect(chat.getByRole('heading',{name:'确认这一次终端操作'})).toHaveCount(0);
    await expect(page.locator('.oc-handoff:visible .oc-handoff-shield')).toHaveCount(0);
    await expect(chat.getByRole('button',{name:'新建对话'})).toBeEnabled({timeout:45000});
    const overrideState = await page.evaluate(()=>window.go.main.App.AssistantControlState());
    if (overrideState.turnId===interrupted.turnId || overrideState.phase!=='completed' || !overrideState.output.includes('OC_SESSION_0123456789abcdef')) throw Error('Immediate message did not complete in a fresh turn');
    const stale = await page.evaluate(async old=>{try{await window.go.main.App.RespondAssistantInteraction(old.turnId,old.prompt.id,{text:'',selected:['allow']});return false;}catch{return true;}},interrupted);
    if (!stale) throw Error('Immediate send accepted old command approval');
    if (modelFixture.checks.questions.some(question=>question.includes('E2E_QUEUE_CANCELLED'))) throw Error('Removed message reached model');
    const after = await commandDispatchCount();
    if (after!==commandsBefore) throw Error('Interrupted approval dispatched a command');
    await editor.fill('刚才的测试代号是什么？不调用工具。');
    await chat.getByTitle('发送',{exact:true}).click();
    await expect(chat.getByRole('button',{name:'新建对话'})).toBeEnabled({timeout:45000});
    const retained = await page.evaluate(()=>window.go.main.App.AssistantControlState());
    if (!retained.output?.includes('OC_SESSION_0123456789abcdef')) throw Error('Immediate send lost conversation context');
    checks.immediateSendRevokesOldApprovalAndLease = true;
    checks.immediateSendKeepsConversationContext = true;
    checks.removedQueueMessageNotDispatched = true;
    async function workspaceTrial(question, expectOpen = false, switches = false) {
      await chat.getByPlaceholder('输入问题…',{exact:true}).fill(question);
      await chat.getByTitle('发送',{exact:true}).click();
      if (expectOpen) {
        await expect(chat.getByRole('heading',{name:'打开这个 SSH 连接？',exact:true})).toBeVisible({timeout:45000});
        await chat.getByRole('button',{name:'打开连接',exact:true}).click();
      }
      await expect(chat.getByRole('heading',{name:'允许助手查看当前终端？'})).toBeVisible({timeout:45000});
      const current = await page.evaluate(()=>window.go.main.App.AssistantControlState());
      const active = page.locator('.flexlayout__tab_button--selected');
      if (!(await active.count())) throw Error('Workspace selected no visible tab');
      await chat.getByRole('button',{name:'允许本轮查看',exact:true}).click();
      await allowTake();
      if (switches) {
        await expect(chat.getByRole('heading',{name:'允许助手查看当前终端？'})).toBeVisible({timeout:45000});
        const changed = await page.evaluate(()=>window.go.main.App.AssistantControlState());
        if (current.target.tabId===changed.target.tabId || current.turnId!==changed.turnId) throw Error('Workspace did not switch target in the same turn');
        const states = await page.evaluate(()=>window.go.main.App.TerminalHandoffStates());
        if (states.some(state=>state.owned)) throw Error('Workspace switch retained old terminal ownership');
        await chat.getByRole('button',{name:'允许本轮查看',exact:true}).click();
        await allowTake();
      }
      await expect(chat.getByRole('button',{name:'新建对话'})).toBeEnabled({timeout:45000});
      const result = await page.evaluate(()=>window.go.main.App.AssistantControlState());
      if (result.phase!=='completed' || result.output!=='WORKSPACE_CHAIN_OK') throw Error(`Workspace trial failed: ${result.message}`);
      if ((await page.evaluate(()=>window.go.main.App.TerminalHandoffStates())).some(state=>state.owned)) throw Error('Workspace trial did not release ownership');
      return result;
    }
    const existingTabs = await page.evaluate(()=>window.go.main.App.TerminalHandoffStates());
    const opened = await workspaceTrial('E2E_WORKSPACE_OPEN 新开一个已保存的 SSH 终端，查看当前目录并交还。',true);
    if (existingTabs.some(tab=>tab.sessionId===opened.target.tabId)) throw Error('Open reused the original shell instead of creating a tab');
    if ((await page.evaluate(()=>window.go.main.App.TerminalHandoffStates())).length!==existingTabs.length+1) throw Error('Open created missing/duplicate terminals');
    checks.assistantOpensAndPresentsSavedSSH = true;
    await workspaceTrial('E2E_WORKSPACE_SELECT 选择另一个已有终端，查看目录并交还。');
    checks.assistantSelectsExistingTerminal = true;
    await workspaceTrial('E2E_WORKSPACE_SWITCH 在当前终端查看目录，然后切换另一个终端再查看目录，最后交还。',false,true);
    checks.targetSwitchRevokesOldLeaseAndRequiresFreshConsent = true;
    await page.screenshot({path:path.join(runDir,'assistant-workspace-tabs.png')});
    checks.modelFixtureRequests = modelFixture.checks.requests;
    await page.screenshot({path:path.join(runDir,'assistant-chat.png')});
    }
  }
  if (keep && !chrysExecutable) await connect('接手试用 · 操作确认体验');
  } else if (chrysExecutable) {
    await page.getByRole('button', {name:/AI|伙伴助手/}).last().click();
  }
  }
  if (errors.length) throw Error(errors.join('\n'));
  await writeFile(path.join(runDir, previewOnly ? 'preview.json' : 'result.json'), JSON.stringify({ modelMode: realModel ? 'real' : modelFixture ? 'simulated' : 'none', assertionsRun: !previewOnly, checks, errors, appURL, runDir }, null, 2));
  if (!previewOnly) console.log(`HANDOFF_PASSED ${JSON.stringify(checks)}`);
  console.log(`PREVIEW_READY ${appURL}\nEVIDENCE ${runDir}`);
} catch (error) {
  await page.screenshot({ path: path.join(runDir, 'failed.png') });
  await writeFile(path.join(runDir, 'failed.json'), JSON.stringify({ error: String(error), errors, modelFixture:modelFixture?.checks, body: await page.locator('body').innerText(), states: await page.evaluate(() => window.go.main.App.TerminalHandoffStates()) }, null, 2));
  console.error(`FAILED_EVIDENCE ${runDir}`);
  throw error;
} finally { await browser.close(); }
if (keep) await new Promise(() => {});
cleaning = true;
await writeFile(endpointFile + '.stop', 'stop');
stop();
