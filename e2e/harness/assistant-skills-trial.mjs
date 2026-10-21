// Test-only business data. This fixture is never copied to a release package.
import { readFile, readdir, mkdir, writeFile } from 'node:fs/promises';
import { randomBytes, createHash } from 'node:crypto';
import path from 'node:path';
import { expect } from '@playwright/test';

export async function prepareBusinessSkills(root, appDir) {
  const logRoot = `/tmp/oc-business-${randomBytes(6).toString('hex')}`;
  const directory = path.join(appDir,'assistant-skills','order-detail-logs');
  await mkdir(path.join(directory,'references'),{recursive:true});
  const source=path.join(root,'e2e/fixtures/business-skills/order-detail-logs');
  await writeFile(path.join(directory,'SKILL.md'),await readFile(path.join(source,'SKILL.md')));
  await writeFile(path.join(directory,'references/deployment.md'),(await readFile(path.join(source,'references/deployment.md'),'utf8')).replace('/var/log/retail/orders-reader',logRoot));
  return {logRoot, cause:`ORDER_UPSTREAM_${randomBytes(6).toString('hex')}`,later:`UNRELATED_LATER_${randomBytes(6).toString('hex')}`};
}

async function auditEvents(appDir) {
  const directory=path.join(appDir,'audit/terminal');
  const batches=await Promise.all((await readdir(directory)).filter(name=>name.endsWith('.jsonl')).map(async name=>(await readFile(path.join(directory,name),'utf8')).trim().split('\n').filter(Boolean).map(line=>JSON.parse(line))));
  return batches.flat();
}

export async function runBusinessSkillsTrial({page,human,appDir,checks,business,modelFixture,realModel}) {
  const {logRoot,cause,later}=business;
  // Fixture creation is human setup through the original PTY, never model authority.
  await human("stty -echo; printf ECHO_; printf 'DISABLED\\n'");
  await expect(page.locator('.oc-handoff:visible .xterm-rows')).toContainText('ECHO_DISABLED');
  const logText=[`2026-10-21T08:41:12 ERROR request=req-17 code=UPSTREAM_TIMEOUT upstream=inventory ${cause}`,'2026-10-21T08:41:13 WARN request=req-17 detail=query failed','2026-10-21T08:41:14 INFO request=req-18 detail=ok',`2026-10-21T10:23:00 ERROR request=req-29 ${later}`,''].join('\n');
  const encoded=Buffer.from(logText).toString('base64');
  await human(`mkdir -p ${logRoot}; printf %s ${encoded} | base64 -d > ${logRoot}/orders-2026-10-21.log; printf healthy > ${logRoot}/orders-2026-10-20.log; printf BUSINESS_; printf LOGS_READY`);
  await expect(page.locator('.oc-handoff:visible .xterm-rows')).toContainText('BUSINESS_LOGS_READY');
  await human('stty echo');
  // Clear visible setup; the seed command itself was not echoed into PTY history.
  await human("printf '\\033[2J\\033[3J\\033[H'");
  const chat=page.locator('.ai-chat-panel');
  await chat.getByPlaceholder('输入问题…',{exact:true}).fill(`${realModel?'':'E2E_BUSINESS_DISCOVER '}订单详情页查询失败了。请先使用适用的业务 Skill 确认 Service 和日志目录，在当前节点列出日志文件，再问我故障时间；先不要读取任何日志正文。`);
  await chat.getByTitle('发送',{exact:true}).click();
  const state=()=>page.evaluate(()=>window.go.main.App.AssistantControlState());
  const answerConsent=async turn=>{
    const label=new Map([['允许助手查看当前终端？','允许本轮查看'],['允许助手接手当前终端？','允许本轮']]).get(turn.prompt?.title);
    if(label) { await chat.getByRole('button',{name:label,exact:true}).click();return true; }
    return false;
  };
  const seen=new Set();let first;let asked=false;
  for(const deadline=Date.now()+150000;Date.now()<deadline;) {
    first=await state();
    if(['failed','stopped','completed'].includes(first.phase)) throw Error(`Business discovery ended before asking: ${first.phase}`);
    if(first.prompt&&!seen.has(first.prompt.id)) {
      if(first.prompt.kind==='input') { asked=true;break; }
      if(!await answerConsent(first)) throw Error('Unexpected business discovery approval');
      seen.add(first.prompt.id);
    }
    await new Promise(resolve=>setTimeout(resolve,100));
  }
  expect(asked).toBe(true);
  const before=(await auditEvents(appDir)).filter(event=>event.phase==='result');
  const hash=text=>createHash('sha256').update(text).digest('hex');
  const lsCommands=[`ls ${logRoot}`,`ls -l ${logRoot}`,`ls -la ${logRoot}`,`command ls ${logRoot}`,`command ls -l ${logRoot}`,`command ls -la ${logRoot}`];
  expect(before.some(event=>lsCommands.map(hash).includes(event.commandHash)&&event.completion==='completed')).toBe(true);
  checks.businessSkillLocatesServiceAndListsLogs=true;
  const oldPrompt=first.prompt;
  // The real product queue stops and drains the old turn, then sends this correction.
  await chat.getByPlaceholder('输入问题…',{exact:true}).fill(`${realModel?'':'E2E_BUSINESS_CORRECT '}就是 orders-2026-10-21.log，不用再找其他文件。只读取前三行，都是 09:00 之前的内容；不要读取后面的行，也不要继续列目录。根据这几行告诉我 Service、请求号、错误码和上游，以及以 ORDER_UPSTREAM_ 开头的完整诊断标记。结束后交还终端。`);
  await chat.getByRole('button',{name:'加入队列',exact:true}).click();
  await chat.getByRole('button',{name:'立即发送',exact:true}).click();
  let final;const answered=new Set();
  for(const deadline=Date.now()+150000;Date.now()<deadline;) {
    final=await state();
    if(final.turnId===first.turnId) {await new Promise(resolve=>setTimeout(resolve,100));continue;}
    if(['failed','stopped'].includes(final.phase)) throw Error(`Business correction ${final.phase}`);
    if(final.phase==='completed') break;
    if(final.prompt&&!answered.has(final.prompt.id)) {
      if(!await answerConsent(final)) throw Error('Unexpected business correction approval');
      answered.add(final.prompt.id);
    }
    await new Promise(resolve=>setTimeout(resolve,100));
  }
  expect(final?.phase).toBe('completed');
  for(const text of [cause,'orders-reader','req-17','UPSTREAM_TIMEOUT','inventory']) expect(final.output).toContain(text);
  expect(final.output).not.toContain(later);
  expect(await page.evaluate(()=>window.go.main.App.TerminalHandoffStates().then(states=>states.some(state=>state.owned)))).toBe(false);
  const late=await page.evaluate(async ({turn,prompt})=>{try {await window.go.main.App.RespondAssistantInteraction(turn,prompt,{selected:['0'],text:''});return false;}catch{return true;}},{turn:first.turnId,prompt:oldPrompt.id});
  expect(late).toBe(true);
  const priorOperations=new Set(before.map(event=>event.operationId));
  const after=(await auditEvents(appDir)).filter(event=>event.phase==='result'&&!priorOperations.has(event.operationId));
  const expected=[`head -n 3 ${logRoot}/orders-2026-10-21.log`,`command head -n 3 ${logRoot}/orders-2026-10-21.log`].map(hash);
  expect(after.length).toBe(1);
  expect(expected).toContain(after[0].commandHash);
  expect(after[0].submission).toBe('submitted');expect(after[0].completion).toBe('completed');expect(after[0].exitCode).toBe(0);
  if(modelFixture) {
    expect(modelFixture.checks.skillLoaded).toBe(true);expect(modelFixture.checks.skillResourceRead).toBe(true);
    expect(modelFixture.checks.correctedEvidence).toBe(true);
    expect(modelFixture.checks.seedLeakedIntoObservation).not.toBe(true);
  }
  checks.businessCorrectionUsesLatestScopeAndRevokesOldQuestion=true;
  checks.businessConclusionMatchesActualLogAndAudit=true;
  await page.screenshot({path:path.join(appDir,'..','assistant-business-skill.png')});
}
