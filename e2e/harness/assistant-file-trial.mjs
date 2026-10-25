import assert from 'node:assert/strict';
import { writeFile, readFile, access } from 'node:fs/promises';
import path from 'node:path';
import { expect } from '@playwright/test';

const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
export async function runFileWorkspaceTrial({page,appDir,runDir,endpoint,modelFixture,checks}) {
  // Allow the native WebView to finish its initial attachment before the
  // headless test page claims presentation ownership.
  await delay(5000);
  await page.reload();
  await page.waitForFunction(()=>!!window.go?.main?.App?.FileWorkspacePoll);
  await page.getByRole('button',{name:'+ 新建连接',exact:true}).click();
  await page.getByRole('button',{name:'手动添加连接',exact:true}).click();
  await page.getByPlaceholder('连接名称',{exact:true}).fill('文件传输试用节点');
  for(const [label,value] of [['主机地址',endpoint.host],['端口',String(endpoint.port)],['用户名',endpoint.user],['密码',endpoint.password]]) await page.getByLabel(label,{exact:true}).fill(value);
  await page.getByRole('button',{name:/连接选中项/}).click();
  await expect(page.locator('.xterm-rows:visible')).toBeVisible({timeout:30000});
  const source=path.join(appDir,'fixture.zip'),download=path.join(appDir,'downloaded.zip');
  const bytes=Buffer.from('OPS_FILE_ROUNDTRIP\n'+Array(8192).fill('bundle-data').join(''));
  await writeFile(source,bytes);modelFixture.files={source,download};
  await page.getByRole('button',{name:/AI|伙伴助手/}).last().click();
  const chat=page.locator('.ai-chat-panel');
  await expect(chat).toBeVisible();
  const state=()=>page.evaluate(()=>window.go.main.App.AssistantControlState());
  let observer;
  async function submit(text){const before=await state();await chat.getByPlaceholder('输入问题…',{exact:true}).fill(text);await chat.getByRole('button',{name:'发送',exact:true}).click();await expect.poll(async()=>(await state()).turnId,{timeout:20000}).not.toBe(before.turnId);}
  async function drive(text,mode='allow'){
    await submit(text);let transferSeen=false;
    const deadline=Date.now()+90000;
    while(Date.now()<deadline){
      const s=await state();
      if(s.prompt){
        if (!observer) {
          const observerContext = await page.context().browser().newContext();
          observer = await observerContext.newPage();
          await observer.goto(page.url());
          await observer.waitForFunction(() => !!window.go?.main?.App?.WorkspaceRegisterPage);
          await delay(1000);
          const after = await state();
          assert.equal(after.turnId, s.turnId);
          assert.equal(after.phase, 'waiting');
          assert.equal(after.prompt?.id, s.prompt.id);
          checks.anotherPageCannotCancelOrStealSenderTurn = true;
        }
        const transfer=s.prompt.title==='确认这一次文件传输';
        if(transfer){transferSeen=true;await expect(page.locator('.oc-handoff-shield:visible')).toHaveAttribute('aria-label','AI 正在使用此文件页签');await expect(chat.getByText(text.includes('DOWNLOAD')?download:source,{exact:true})).toBeVisible();}
        if(transfer&&mode==='stop'){
          await chat.getByRole('button',{name:'停止本轮',exact:true}).click();
          const rejected=await page.evaluate(async({turn,prompt})=>{try{await window.go.main.App.RespondAssistantInteraction(turn,prompt,{text:'',selected:['allow']});return false}catch{return true}},{turn:s.turnId,prompt:s.prompt.id});assert.equal(rejected,true);break;
        }
        await chat.getByRole('button',{name:transfer&&mode==='deny'?'拒绝':'允许本次',exact:true}).click();
      }
      if(['completed','failed','stopped'].includes(s.phase)){assert.notEqual(s.phase,'failed',s.message);break;}
      await delay(200);
    }
    await expect.poll(async()=>['completed','stopped'].includes((await state()).phase),{timeout:20000}).toBe(true);
    assert.equal((await state()).phase,mode==='stop'?'stopped':'completed');
    await expect(page.locator('.oc-handoff-shield:visible')).toHaveCount(0);
    if (mode === 'allow' && !text.includes('BROWSE')) {
      const local = text.includes('DOWNLOAD') ? download : source;
      const observation = modelFixture.checks.commandResults.slice().reverse().map(value => {
        try { return JSON.parse(value).result?.tasks?.find(task => task.localPath === local); } catch { return undefined; }
      }).find(Boolean);
      assert.equal(observation?.status, 'done', observation?.message || 'Model never observed transfer completion');
    }
    return transferSeen;
  }
  await drive('E2E_FILE_BROWSE 浏览文件目录并交还。');
  await expect(page.getByRole('region',{name:'文件工作区 · 文件传输试用节点'})).toBeVisible();
  checks.fileTabPresentedAndReleased=true;
  assert.equal(await drive('E2E_FILE_UPLOAD 上传测试包并等待完成。'),true);
  checks.exactTransferApprovalKeepsFileOwnership=true;
  await page.screenshot({path:path.join(runDir,'file-upload-complete.png')});
  assert.equal(await drive('E2E_FILE_DOWNLOAD 下载刚才的测试包。'),true);
  assert.deepEqual(await readFile(download),bytes);checks.realSFTPRoundtripPreservesBytes=true;
  assert.ok(modelFixture.checks.questions[0].includes('remote-terminal'));
  assert.ok(modelFixture.checks.questions[0].includes(endpoint.host));
  checks.selectedRemoteContextReachesChrys = true;
  await drive('E2E_FILE_DENY 拒绝这次上传。','deny');checks.deniedTransferDoesNotStart=true;
  await drive('E2E_FILE_STOP 在确认上传时停止。','stop');checks.stopRevokesLateFileApproval=true;
  const requestsBeforeReload = modelFixture.checks.requests;
  await page.reload();
  await page.waitForFunction(() => !!window.go?.main?.App?.FileWorkspacePoll);
  const restoredTab = page.locator('.flexlayout__tab_button_content').filter({hasText:'文件 · 文件传输试用节点'}).first();
  await expect(restoredTab).toBeVisible({timeout:20000});
  await restoredTab.click();
  await expect(page.getByRole('region',{name:'文件工作区 · 文件传输试用节点'})).toBeVisible();
  await expect(page.getByText('传输队列',{exact:true})).toBeVisible();
  assert.equal(modelFixture.checks.requests,requestsBeforeReload);
  checks.reloadRestoresFileTabWithoutReplay = true;
  const client=await page.evaluate(()=>window.go.main.App.WorkspaceAttach());
  const snapshot=await page.evaluate(client=>window.go.main.App.FileWorkspacePoll(client,0),client);
  const workspace=snapshot.items.find(w=>!w.dismissed);
  assert.equal(workspace.ownerTurn,'');assert.equal(workspace.tasks.length,2);assert.ok(workspace.tasks.every(t=>t.status==='done'));
  assert.equal(await access(path.join(appDir,'never-created.zip')).then(()=>true,()=>false),false);
  checks.onlyConfirmedTransfersWereSubmitted=true;
  const audit=(await readFile(path.join(appDir,'audit','files','operations.jsonl'),'utf8')).trim().split('\n').map(JSON.parse);
  assert.equal(audit.filter(e=>e.action==='transfer_intent').length,2);
  assert.equal(audit.filter(e=>e.action==='transfer_result'&&e.status==='done').length,2);
  assert.ok(!JSON.stringify(audit).includes(source));checks.fileAuditContainsHashesAndOutcomes=true;
  await page.screenshot({path:path.join(runDir,'file-workspace-handed-back.png')});
}
