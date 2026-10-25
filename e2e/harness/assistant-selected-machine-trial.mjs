// Opt-in real-model verification of the selected remote machine context.
// All SSH activity stays inside the disposable localhost shell fixture.
import assert from 'node:assert/strict';
import { randomBytes } from 'node:crypto';
import { readFile, readdir } from 'node:fs/promises';
import path from 'node:path';
import { expect } from '@playwright/test';

const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
export async function runSelectedMachineTrial({page,endpoint,appDir,runDir,checks}) {
  await delay(5000); await page.reload();
  await page.waitForFunction(()=>!!window.go?.main?.App?.StartAssistantTurnOnPage);
  await page.getByRole('button',{name:'+ 新建连接',exact:true}).click();
  await page.getByRole('button',{name:'手动添加连接',exact:true}).click();
  await page.getByPlaceholder('连接名称',{exact:true}).fill('当前远端节点');
  for (const [label,value] of [['主机地址',endpoint.host],['端口',String(endpoint.port)],['用户名',endpoint.user],['密码',endpoint.password]]) await page.getByLabel(label,{exact:true}).fill(value);
  await page.getByRole('button',{name:/连接选中项/}).click();
  await expect(page.locator('.xterm-rows:visible')).toBeVisible({timeout:30000});
  const marker=`oc_remote_tmp_${randomBytes(8).toString('hex')}`;
  async function human(command) {
    await page.locator('.xterm-screen:visible').click();
    await page.keyboard.insertText(command); await page.keyboard.press('Enter');
  }
  await human(`touch /tmp/${marker}; printf 'REMOTE_SEED_READY\\n'`);
  await expect(page.locator('.xterm-rows:visible')).toContainText('REMOTE_SEED_READY');
  await page.getByRole('button',{name:'AI 助手',exact:true}).click();
  const chat=page.locator('.ai-chat-panel');
  await expect(chat.getByPlaceholder('输入问题…',{exact:true})).toBeEnabled({timeout:20000});
  await chat.getByPlaceholder('输入问题…',{exact:true}).fill('你看一下现在这个机器上，TMP目录下有什么内容？只读查看，列出条目名称即可。');
  await chat.getByRole('button',{name:'发送',exact:true}).click();
  const answered=new Set(); let done=false;
  for (const deadline=Date.now()+150000;Date.now()<deadline;) {
    const state=await page.evaluate(()=>window.go.main.App.AssistantControlState());
    if (state.phase==='failed'||state.phase==='stopped') throw Error(state.message);
    if (state.prompt&&!answered.has(state.prompt.id)) {
      // Consent is limited to the isolated remote fixture. Accept only the
      // reviewed listing command; native/local requests and other commands fail.
      assert.equal(state.prompt.kind,'approval');
      if (state.prompt.command) {
        assert.match(state.prompt.command.trim(),/^ls(?: -[1aAlht]+)* \/tmp\/?$/,'Unexpected remote command');
      } else {
        assert.ok(/^允许助手(接手当前终端|查看当前终端)/.test(state.prompt.title),`Unexpected prompt: ${state.prompt.title}`);
      }
      assert.ok(!state.prompt.details?.some(d=>d.label==='执行位置'&&d.value.includes('本机')));
      await chat.getByRole('button',{name:/允许本轮|允许读取|允许本次/,exact:true}).click();
      answered.add(state.prompt.id);
    }
    if (state.phase==='completed') {
      assert.ok(state.output?.includes(marker),'Answer does not include the isolated remote TMP entry');
      checks.realModelUsesSelectedRemoteTmp=true;done=true;break;
    }
    await delay(200);
  }
  assert.ok(done,'Real model did not complete');
  const auditDir=path.join(appDir,'audit','terminal');
  const events=(await Promise.all((await readdir(auditDir)).filter(n=>n.endsWith('.jsonl')).map(async n=>(await readFile(path.join(auditDir,n),'utf8')).trim().split('\n').filter(Boolean).map(JSON.parse)))).flat();
  assert.ok(events.some(e=>e.phase==='result'&&e.submission==='submitted'&&e.completion==='completed'));
  checks.remoteReadonlyCommandCompleted=true;
  await page.screenshot({path:path.join(runDir,'selected-remote-tmp.png')});
  await human(`rm -f /tmp/${marker}`);
}
