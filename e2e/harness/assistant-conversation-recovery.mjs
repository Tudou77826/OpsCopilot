// Uses real desktop/Chrys sessions. Model replies are deterministic; no SSH or
// write request is approved. Reload must cancel, not resume, the accepted turn.
import { expect } from '@playwright/test';
import assert from 'node:assert/strict';
import { access } from 'node:fs/promises';
import { randomBytes } from 'node:crypto';
import path from 'node:path';

export async function runConversationRecoveryTrial({page, appURL, modelFixture, checks, runDir}) {
  const delay = ms => new Promise(resolve=>setTimeout(resolve,ms));
  const chat = page.locator('.ai-chat-panel');
  const state = () => page.evaluate(()=>window.go.main.App.AssistantControlState());
  async function open() {
    if (!await chat.isVisible()) await page.getByRole('button',{name:'AI 助手',exact:true}).click();
    await expect(chat.getByPlaceholder('输入问题…',{exact:true})).toBeEnabled({timeout:20000});
  }
  async function submit(question) {
    const previous=await state();
    await chat.getByPlaceholder('输入问题…',{exact:true}).fill(question);
    await chat.getByRole('button',{name:'发送',exact:true}).click();
    await expect.poll(async()=>(await state()).turnId,{timeout:20000}).not.toBe(previous.turnId);
  }
  async function finish() {
    await expect(chat.getByRole('button',{name:'新建对话',exact:true})).toBeEnabled({timeout:45000});
    const s=await state(); assert.equal(s.phase,'completed',s.message); return s;
  }
  async function reload() {
    await page.reload();
    await page.waitForFunction(()=>!!window.go?.main?.App?.RestoreAssistantConversation);
    await open();
  }
  // Let the native WebView finish its initial (empty) recovery first.
  await delay(2000);
  await open();
  const nonce=`OC_SESSION_${randomBytes(8).toString('hex')}`;
  const question=`请记住本段对话的测试代号：${nonce}。只回复代号，不调用工具。`;
  await submit(question); await finish();
  await reload();
  await expect(chat.getByText(question,{exact:true})).toBeVisible();
  await expect(chat.getByText(nonce,{exact:true})).toBeVisible();
  checks.reloadRestoresOriginalMessages=true;
  await submit('刚才的测试代号是什么？只回复代号，不调用工具。');
  assert.ok((await finish()).output.includes(nonce));
  checks.reloadKeepsNativeModelContext=true;

  await submit('E2E_RECOVERY_SLOW 保持运行，等待刷新取消。');
  for (const question of ['E2E_RECOVERY_QUEUED_ONE','E2E_RECOVERY_QUEUED_TWO']) {
    await chat.getByPlaceholder('输入问题…',{exact:true}).fill(question);
    await chat.getByRole('button',{name:'加入队列',exact:true}).click();
  }
  await reload();
  await expect(chat.getByText('E2E_RECOVERY_SLOW 保持运行，等待刷新取消。',{exact:true})).toBeVisible();
  await expect(chat.getByText('本轮已停止，未自动续跑',{exact:true})).toBeVisible();
  await expect(chat.getByLabel('待发送消息')).toHaveCount(0);
  assert.equal((await state()).phase,'stopped');
  assert.ok(!modelFixture.checks.questions.some(q=>q.includes('E2E_RECOVERY_QUEUED')));
  checks.reloadStopsActiveTurnAndDropsUnsentQueue=true;
  await page.screenshot({path:path.join(runDir,'conversation-restored-stopped.png')});

  await submit('E2E_RECOVERY_APPROVAL 请求一次本地文件写入，等待确认时刷新。');
  await expect.poll(async()=>!!(await state()).prompt,{timeout:45000}).toBe(true);
  const before=await state();
  assert.equal(before.prompt.kind,'approval');
  assert.ok(before.prompt.title.includes('recovery-never-created.txt'));
  const forbidden=before.prompt.title;
  await reload();
  assert.equal((await state()).phase,'stopped');
  await expect(chat.getByRole('button',{name:'允许本次',exact:true})).toHaveCount(0);
  const rejected=await page.evaluate(async({turn,prompt})=>{
    try {await window.go.main.App.RespondAssistantInteraction(turn,prompt,{text:'',selected:['allow']});return false;}
    catch{return true;}
  },{turn:before.turnId,prompt:before.prompt.id});
  assert.equal(rejected,true);
  assert.equal(await access(forbidden).then(()=>true,()=>false),false);
  checks.reloadRevokesNativeApprovalWithoutWriting=true;
  const first=await page.evaluate(()=>window.go.main.App.RestoreAssistantConversation());
  await reload();
  const second=await page.evaluate(()=>window.go.main.App.RestoreAssistantConversation());
  assert.equal(first.id,second.id);
  assert.deepEqual(first.messages,second.messages);
  assert.equal(new Set(second.messages.map(m=>m.id)).size,second.messages.length);
  checks.repeatReloadDoesNotDuplicateOrReplay=true;

  await submit('之前的测试代号是什么？只回复代号，不调用工具。');
  assert.ok((await finish()).output.includes(nonce));
  await chat.getByRole('button',{name:'新建对话',exact:true}).click();
  await expect(chat.getByRole('heading',{name:'有什么需要一起排查？',exact:true})).toBeVisible();
  const empty=await page.evaluate(()=>window.go.main.App.RestoreAssistantConversation());
  assert.notEqual(empty.id,first.id); assert.deepEqual(empty.messages,[]);
  await submit('之前的测试代号是什么？只回复代号，不调用工具。');
  assert.ok(!(await finish()).output.includes(nonce));
  checks.newConversationClearsDisplayAndNativeContext=true;
  await page.screenshot({path:path.join(runDir,'conversation-new.png')});
  await page.goto(appURL); await open();
  await expect(chat.getByText(nonce,{exact:true})).toHaveCount(0);
  checks.oldDisplayDoesNotReturnAfterReset=true;
}
