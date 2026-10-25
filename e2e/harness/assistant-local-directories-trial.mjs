// Real Wails + Chrys + native local reads, deterministic model choices, and
// isolated local fixtures. No SSH connection or production directory is used.
import assert from 'node:assert/strict';
import { mkdir, writeFile, readFile } from 'node:fs/promises';
import { randomBytes } from 'node:crypto';
import path from 'node:path';
import { expect } from '@playwright/test';

export async function runLocalDirectoriesTrial({page,appDir,runDir,modelFixture,checks}) {
  const dirs=['project','logs','packages','business'].map(name=>path.join(appDir,'local-fixture',name));
  for(const dir of dirs)await mkdir(dir,{recursive:true});
  const markerA=`LOCAL_A_${randomBytes(8).toString('hex')}`,markerB=`LOCAL_B_${randomBytes(8).toString('hex')}`;
  const aFile=path.join(dirs[1],'README.txt'),bFile=path.join(dirs[3],'README.txt');
  await writeFile(aFile,markerA);await writeFile(bFile,markerB);
  modelFixture.localDirectories={paths:dirs.slice(1,3),readPath:aFile};
  const chat=page.locator('.ai-chat-panel'),dialog=page.getByRole('dialog',{name:'本地项目',exact:true});
  const state=()=>page.evaluate(()=>window.go.main.App.AssistantControlState());
  const projects=()=>page.evaluate(()=>window.go.main.App.AssistantLocalProjects());
  async function openChat(){if(!await chat.isVisible())await page.getByRole('button',{name:'AI 助手',exact:true}).click();await expect(chat.getByPlaceholder('输入问题…')).toBeEnabled({timeout:20000});}
  async function openProjects(){await chat.getByRole('button',{name:'本地项目',exact:true}).click();await expect(dialog).toBeVisible();}
  async function addDirectory(dir){await dialog.getByLabel('添加目录',{exact:true}).fill(dir);await dialog.getByRole('button',{name:'添加本地目录',exact:true}).click();await expect(dialog.getByText(dir,{exact:true})).toBeVisible();}
  async function createProject(name){await dialog.getByLabel('项目名称',{exact:true}).fill(name);await dialog.getByRole('button',{name:'保存项目',exact:true}).click();await expect(dialog.getByLabel('添加目录',{exact:true})).toBeVisible();return (await projects()).activeId;}
  await openChat();await openProjects();
  const bID=await createProject('B项目');await addDirectory(dirs[3]);
  await dialog.getByRole('button',{name:'新建项目',exact:true}).click();
  const aID=await createProject('A项目');await addDirectory(dirs[0]);
  await dialog.getByRole('button',{name:'关闭本地项目'}).click();
  async function drive(question,readPath,add=false){
    const before=await state();await chat.getByPlaceholder('输入问题…').fill(question);await chat.getByRole('button',{name:'发送',exact:true}).click();
    await expect.poll(async()=>(await state()).turnId).not.toBe(before.turnId);
    const answered=new Set();let completed=false;
    for(const deadline=Date.now()+90000;Date.now()<deadline;){
      const s=await state();if(s.phase==='failed')throw Error(s.message);
      if(s.prompt&&!answered.has(s.prompt.id)){
        assert.equal(s.prompt.kind,'approval');
        if(s.prompt.title==='将本地目录加入项目「A项目」'){
          assert.ok(add);assert.deepEqual(s.prompt.details.map(d=>path.normalize(d.value)),dirs.slice(1,3).map(d=>path.normalize(d)));
          assert.equal((await projects()).projects.find(p=>p.id===aID).directories.length,1);
          await chat.getByRole('button',{name:'添加到此项目',exact:true}).click();checks.agentAdditionBoundToExactProject=true;
        }else{
          const input=JSON.parse(s.prompt.command||'{}');assert.equal(path.normalize(input.path),path.normalize(readPath));
          assert.deepEqual(Object.keys(input).filter(k=>!['path','start_line','max_lines','max_tokens'].includes(k)),[]);
          await chat.getByRole('button',{name:'允许本次',exact:true}).click();
        }
        answered.add(s.prompt.id);
      }
      if(s.phase==='completed'){completed=true;break;}await new Promise(resolve=>setTimeout(resolve,150));
    }
    assert.ok(completed,'project turn timed out');
  }
  await drive('E2E_LOCAL_DIRS_ADD 追加两个本地目录到当前项目，并读取测试说明。',aFile,true);
  assert.ok(modelFixture.checks.commandResults.some(v=>String(v).includes(markerA)));
  let saved=await projects();assert.equal(saved.projects.find(p=>p.id===aID).directories.length,3);assert.equal(saved.projects.find(p=>p.id===bID).directories.length,1);
  checks.aThreeDirectoriesBOneDirectory=true;
  await openProjects();await dialog.getByLabel('当前项目').selectOption(bID);await expect(dialog.getByRole('listitem')).toHaveCount(1);await dialog.getByRole('button',{name:'关闭本地项目'}).click();
  modelFixture.localDirectories.readPath=bFile;
  await drive('E2E_LOCAL_DIRS_READ 读取当前项目的测试说明。',bFile);
  const latest=modelFixture.checks.questions.at(-1);assert.ok(latest.includes('B项目')&&latest.includes('business'));assert.ok(!latest.includes(dirs[0].replaceAll('\\','\\\\'))&&!latest.includes('A项目'));
  assert.ok(modelFixture.checks.commandResults.some(v=>String(v).includes(markerB)));checks.projectSwitchRefreshesModelContext=true;
  await openProjects();await dialog.getByLabel('当前项目').selectOption(aID);await expect(dialog.getByRole('listitem')).toHaveCount(3);await dialog.getByRole('button',{name:'关闭本地项目'}).click();
  await page.reload();await openChat();await openProjects();await expect(dialog.getByRole('listitem')).toHaveCount(3);
  await page.screenshot({path:path.join(runDir,'local-projects-restored.png')});await dialog.getByRole('button',{name:'关闭本地项目'}).click();
  checks.reloadPreservesSelectedProject=true;
  await chat.getByRole('button',{name:'新建对话',exact:true}).click();await expect(chat.getByRole('heading',{name:'有什么需要一起排查？'})).toBeVisible();
  saved=await projects();assert.equal(saved.activeId,aID);assert.equal(saved.projects.length,2);assert.equal(saved.projects.find(p=>p.id===aID).directories.length,3);
  const disk=JSON.parse(await readFile(path.join(appDir,'assistant-projects.json'),'utf8'));assert.deepEqual(disk,saved);
  checks.newConversationKeepsPersistedProjectGroups=true;
  await expect(page.locator('.oc-handoff-shield:visible')).toHaveCount(0);checks.noSSHOrTabLockRequired=true;
}
