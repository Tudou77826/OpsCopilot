// Opt-in real-model trial. Only model settings are copied; SSH targets and
// terminal data always come from the disposable local PTY fixture.
import { readFile, readdir } from 'node:fs/promises';
import { createHash, randomBytes } from 'node:crypto';
import path from 'node:path';
import { expect } from '@playwright/test';

export async function loadTrialModel(filename) {
  if (!path.isAbsolute(filename)) throw Error('Real model config must be an absolute path.');
  let data;
  try { data = JSON.parse(await readFile(filename, 'utf8')); }
  catch { throw Error('Cannot read real model config.'); }
  const llm = data?.llm;
  const model = llm?.ComplexModel || llm?.Model || llm?.FastModel;
  if (typeof model !== 'string' || !model.trim() || typeof llm?.BaseURL !== 'string' ||
      (llm.APIKey !== undefined && typeof llm.APIKey !== 'string')) throw Error('Real model settings are incomplete.');
  let url;
  try { url = new URL(llm.BaseURL); } catch { throw Error('Invalid model service URL.'); }
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.hash) throw Error('Invalid model service URL.');
  // Explicit allowlist: never copy business connections or unrelated config.
  return { BaseURL: llm.BaseURL, APIKey: llm.APIKey || '', Model: model, ComplexModel: model };
}

async function auditEvents(appDir) {
  const directory = path.join(appDir, 'audit/terminal');
  const names = await readdir(directory);
  const batches = await Promise.all(names.filter(name => name.endsWith('.jsonl')).map(async name => {
    const lines = (await readFile(path.join(directory, name), 'utf8')).trim().split('\n').filter(Boolean);
    return lines.map(line => JSON.parse(line));
  }));
  return batches.flat();
}

export async function runRealModelTrial({ page, human, appDir, checks }) {
  const chat = page.locator('.ai-chat-panel');
  const frame = page.locator('.oc-handoff:visible');
  const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
  const state = () => page.evaluate(() => window.go.main.App.AssistantControlState());
  const owned = () => page.evaluate(() => window.go.main.App.TerminalHandoffStates().then(states => states.some(value => value.owned)));
  async function ask(question, allowedPrompts = new Map(), whileRunning = async () => {}) {
    const previous = await state();
    await chat.getByPlaceholder('输入问题…', { exact: true }).fill(question);
    await chat.getByTitle('发送', { exact: true }).click();
    const deadline = Date.now() + 150000;
    let turn;
    const answered = new Set();
    try {
      while (Date.now() < deadline) {
        turn = await state();
        if (!turn.turnId || turn.turnId === previous.turnId) { await delay(100); continue; }
        if (turn.phase === 'failed' || turn.phase === 'stopped') throw Error(`Real model turn ${turn.phase}: ${turn.message}`);
        if (turn.phase === 'completed') {
          expect(turn.output?.length).toBeGreaterThan(0);
          expect(await owned()).toBe(false);
          return turn;
        }
        await whileRunning(turn);
        if (turn.prompt && !answered.has(turn.prompt.id)) {
          const label = allowedPrompts.get(turn.prompt.title);
          // Never automatically approve a command produced by a real model.
          if (!label) throw Error(`Unexpected real model interaction: ${turn.prompt.kind}`);
          await expect(chat.getByRole('heading', { name: turn.prompt.title, exact: true })).toBeVisible();
          await chat.getByRole('button', { name: label, exact: true }).click();
          answered.add(turn.prompt.id);
        }
        await delay(100);
      }
      throw Error('Real model trial exceeded 150 seconds.');
    } catch (error) {
      if (turn?.turnId && turn.turnId !== previous.turnId) await page.evaluate(id => window.go.main.App.StopAssistantTurn(id), turn.turnId).catch(() => {});
      throw error;
    }
  }

  let humanTyped = false;
  const conversation = await ask('只做普通对话：17 加 25 等于多少？不要读取或操作终端。', new Map(), async () => {
    expect(await owned()).toBe(false);
    if (!humanTyped) {
      await expect(frame.locator('.oc-handoff-content')).not.toHaveAttribute('inert');
      await human("command printf 'REAL_CHAT_INPUT_ALLOWED\\n'");
      humanTyped = true;
    }
  });
  expect(conversation.output).toContain('42');
  expect(humanTyped).toBe(true);
  checks.realModelConversationWithoutOwnership = true;

  const marker = `REAL_MODEL_SCREEN_${randomBytes(8).toString('hex')}`;
  await human(`command printf '\\n${marker}\\n'`);
  await expect(frame.locator('.xterm-rows')).toContainText(marker);
  const consent = new Map([['允许助手查看当前终端？', '允许本轮查看']]);
  const screen = await ask('请告诉我当前终端最后一次输出的、以 REAL_MODEL_SCREEN_ 开头的完整标记。只查看现有屏幕，不执行任何命令。', consent, async () => expect(await owned()).toBe(false));
  expect(screen.output).toContain(marker);
  const screenAudit = (await auditEvents(appDir)).filter(event => event.target?.sessionId === screen.target.tabId);
  expect(screenAudit.some(event => event.phase === 'screen_read')).toBe(true);
  expect(screenAudit.some(event => event.phase === 'dispatch_intent')).toBe(false);
  checks.realModelReadsUnpromptedScreenMarkerWithoutOwnership = true;

  await human("cd /tmp; command printf 'REAL_DIRECTORY_READY\\n'");
  await expect(frame.locator('.xterm-rows')).toContainText('REAL_DIRECTORY_READY');
  let sawOwned = false;
  let releasedBeforeCompletion = false;
  consent.set('允许助手接手当前终端？', '允许本轮');
  const command = await ask('请在当前终端运行一次 command pwd，告诉我真实的当前目录。不要修改任何文件，也不要仅根据历史屏幕猜测。运行完成后交还终端，再解释结果。', consent, async () => {
    const current = await owned();
    if (sawOwned && !current) releasedBeforeCompletion = true;
    sawOwned ||= current;
  });
  const events = (await auditEvents(appDir)).filter(event => event.target?.sessionId === command.target.tabId && event.phase === 'result');
  const hash = text => createHash('sha256').update(text).digest('hex');
  const pwd = events.filter(event => [hash('command pwd'), hash('pwd')].includes(event.commandHash));
  expect(pwd.length).toBe(1);
  expect(pwd[0].submission).toBe('submitted');
  expect(pwd[0].completion).toBe('completed');
  expect(pwd[0].exitCode).toBe(0);
  expect(events.length).toBe(1);
  expect(command.output).toContain('/tmp');
  expect(sawOwned).toBe(true);
  expect(releasedBeforeCompletion).toBe(true);
  checks.realModelTakesExecutesAndReleases = true;
}
