import { readFile, mkdir, symlink, readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { expect } from '@playwright/test';
import { ensureMySQLJDBC } from './database-driver.mjs';
import { runDatabaseStabilityTrial } from './assistant-database-stability.mjs';

export async function prepareDatabaseTrial(appDir,endpointFile,runtimeDir) {
    const endpoint=JSON.parse((await readFile(endpointFile,'utf8')).replace(/^\uFEFF/,''));
    if(endpoint.host!=='127.0.0.1'||endpoint.database!=='opscopilot_review')throw Error('Database trial only uses the isolated local review endpoint');
    await mkdir(appDir,{recursive:true});
    await symlink(runtimeDir,path.join(appDir,'jdbc-runtime'),'junction');
    return endpoint;
}
export async function runAssistantDatabaseTrial({page,appDir,runDir,checks,endpoint,modelFixture,commandTrial=false}) {
    const driverId=await ensureMySQLJDBC(page);
    // The deterministic model receives public target fields only, never credentials.
    if(modelFixture)modelFixture.database={host:endpoint.host,port:endpoint.port,user:endpoint.user,database:endpoint.database,driverId};
    await page.getByRole('button',{name:/AI|伙伴助手/}).last().click();
    const chat=page.locator('.ai-chat-panel');
    const state=()=>page.evaluate(()=>window.go.main.App.AssistantControlState());
    async function send(question){await chat.getByPlaceholder('输入问题…',{exact:true}).fill(question);await chat.getByTitle('发送',{exact:true}).click();}
    if(commandTrial){
        const source=path.join(runDir,'test-only-skill');await mkdir(path.join(source,'scripts'),{recursive:true});
        await writeFile(path.join(source,'SKILL.md'),'---\nname: database-discovery\ndescription: Test-only local database discovery.\n---\nRun scripts/discover.py to obtain the isolated test endpoint. This is fixture data, not a production management-node script. Then create a database profile through the secure form, open a full workspace, execute two diagnostic SELECT statements and release it.\n');
        const publicEndpoint={host:endpoint.host,port:endpoint.port,user:endpoint.user,database:endpoint.database};
        await writeFile(path.join(source,'scripts','discover.py'),`print(${JSON.stringify('DISCOVERED_ENDPOINT='+JSON.stringify(publicEndpoint))})\n`);
        await page.evaluate(async source=>{await window.go.main.App.ImportAssistantSkill(source,false,'');},source);
        // Register the command in the actual manager, not by editing a config file.
        await chat.getByRole('button',{name:'管理 Skills 和快捷命令'}).click();
        const manager=page.getByRole('dialog',{name:'助手能力'});
        await expect(manager.getByText('database-discovery',{exact:true})).toBeVisible();
        await manager.getByRole('button',{name:/快捷命令/}).click();await manager.getByRole('button',{name:'新建命令',exact:true}).click();
        await manager.getByLabel('命令名称',{exact:true}).fill('sqlSessionInfoGetAndCreate');
        await manager.getByLabel('用途',{exact:true}).fill('获取数据库信息并打开 SQL 页签');
        await manager.getByLabel('关联 Skill',{exact:true}).selectOption('database-discovery');
        await manager.getByLabel('提示词模板',{exact:true}).fill('E2E_LIBRARY_DATABASE 为 {{business}} 获取数据库信息并保存连接、打开属于你的完整 SQL 页签。密码我会在连接表单填写。每条 SQL 保留用途注释，结束后交还。');
        await manager.getByLabel('business 显示名称',{exact:true}).fill('业务名称');
        await manager.getByRole('button',{name:'保存命令',exact:true}).click();
        await expect(manager.getByText('/sqlSessionInfoGetAndCreate',{exact:true})).toBeVisible();
        await manager.getByRole('button',{name:'关闭助手能力'}).click();
        await chat.getByPlaceholder('输入问题…',{exact:true}).fill('/sqlSession');
        await chat.getByRole('option',{name:/sqlSessionInfoGetAndCreate/}).click();
        await expect(chat.getByTitle('发送',{exact:true})).toBeDisabled();
        await chat.getByLabel('业务名称',{exact:true}).fill('隔离测试业务');
        await page.screenshot({path:path.join(runDir,'assistant-command-inline-blanks.png')});
        await chat.getByTitle('发送',{exact:true}).click();
    }else await send(`E2E_DATABASE_CREATE 为独立本地测试库新建并保存连接，再打开属于你的完整数据库页签。使用 ${endpoint.host}:${endpoint.port}，账号 ${endpoint.user}，数据库 ${endpoint.database}，已导入 MySQL JDBC。密码我会在连接表单填写。只执行这两条 SQL：SELECT COUNT(*) AS node_count FROM service_nodes; 然后 SELECT service_name, status FROM service_nodes ORDER BY service_name LIMIT 3; 每条写用途注释，结束后交还，不关闭连接。`);
    const responded=new Set();let completed;let workspaceID;let physical;let protectedInput=false;
    for(const deadline=Date.now()+150000;Date.now()<deadline;){
        const current=await state();
        if(['failed','stopped'].includes(current.phase))throw Error(`Database trial ${current.phase}: ${current.message}`);
        if(current.target?.connectionId){workspaceID=current.target.tabId;physical=current.target.connectionId;}
        if(current.prompt&&!responded.has(current.prompt.id)){
            if(current.prompt.kind==='database_connection'){
                await chat.getByRole('button',{name:'填写连接信息',exact:true}).click();
                const dialog=page.locator('dialog.oc-db-dialog').last();
                await dialog.getByLabel('密码',{exact:true}).fill(endpoint.password);
                await dialog.getByRole('tab',{name:/更多设置/}).click();
                await dialog.getByLabel('允许获取服务器 RSA 公钥',{exact:true}).check();
                await dialog.getByRole('button',{name:'保存并继续',exact:true}).click();
            }else if(commandTrial&&current.prompt.kind==='approval'&&current.prompt.details?.some(d=>d.value==='本机（不是 SSH 服务器）')){
                await chat.getByRole('button',{name:'允许本次',exact:true}).click();checks.nativeSkillScriptApprovalVisible=true;
            }else if(['打开独立数据库连接？','允许助手使用这个数据库页签？'].includes(current.prompt.title)){
                await chat.getByRole('button',{name:'允许本次',exact:true}).click();
                if(current.prompt.title==='允许助手使用这个数据库页签？')await expect(page.locator('.oc-ai-db .oc-handoff-shield')).toBeVisible();
            }else throw Error(`Unexpected database approval: ${current.prompt.title}`);
            responded.add(current.prompt.id);
        }
        if(!protectedInput&&await page.locator('.oc-ai-db .oc-handoff-shield').count()){
            await expect(page.locator('.oc-ai-db .oc-handoff-content')).toHaveAttribute('inert');
            physical=(await state()).target.connectionId;
            const denied=await page.evaluate(async id=>{try{await window.go.main.App.DatabaseQuery({sessionId:id,requestId:crypto.randomUUID(),sql:"SELECT 'HUMAN_SHOULD_NOT_RUN'",mode:'selection',limit:10,timeoutSeconds:5});return '';}catch(error){return String(error);}},physical);
            expect(denied).toContain('请先手动接管');protectedInput=true;
            const independent=await page.evaluate(async owned=>{
                const profile=(await window.go.main.App.DatabaseProfiles())[0];
                const human=await window.go.main.App.DatabaseConnect(profile.id);
                try {
                    const result=await window.go.main.App.DatabaseQuery({sessionId:human.id,requestId:crypto.randomUUID(),sql:'SELECT 7 AS separate_human_session',mode:'selection',limit:10,timeoutSeconds:5});
                    return {separate:human.id!==owned,error:result.error,rows:result.sets[0]?.rows};
                }finally{await window.go.main.App.DatabaseDisconnect(human.id);}
            },physical);
            expect(independent).toEqual({separate:true,error:'',rows:[['7']]});
            checks.otherPhysicalDatabaseSessionRemainsUsable=true;
        }
        if(current.phase==='completed'){completed=current;break;}
        await new Promise(resolve=>setTimeout(resolve,100));
    }
    expect(completed?.phase).toBe('completed');expect(protectedInput).toBe(true);
    if(commandTrial){
        expect(modelFixture.checks.commandResults.some(text=>String(text).includes('DISCOVERED_ENDPOINT='))).toBe(true);
        const native=await readFile(path.join(appDir,'audit/assistant/native.jsonl'),'utf8');expect(native).toContain('"status":"allowed"');expect(native).not.toContain(endpoint.password);
        checks.commandToNativeSkillToDatabaseWorkspace=true;
    }
    const frame=page.locator('.oc-ai-db');
    await expect(frame.locator('.oc-handoff-shield')).toHaveCount(0);
    const editor=frame.locator('.cm-content');
    await expect(editor).toContainText('SELECT COUNT(*) AS node_count FROM service_nodes;');
    await expect(editor).toContainText('SELECT service_name, status FROM service_nodes ORDER BY service_name LIMIT 3;');
    expect((await editor.innerText()).match(/-- AI/g)?.length).toBe(2);
    await expect(frame.getByText('AI 执行记录 · 2 条',{exact:true})).toBeVisible();
    checks.aiCreatesSavedProfileAndIndependentDatabaseTab=true;
    checks.databaseOwnershipBlocksHumanAPI=true;
    const auditFile=path.join(appDir,'audit/database/executions.jsonl');
    const audit=(await readFile(auditFile,'utf8')).trim().split('\n').map(JSON.parse);
    const dispatch=audit.filter(e=>e.phase==='dispatch_intent');
    const hash=text=>createHash('sha256').update(text).digest('hex');
    const originals=await frame.locator('.oc-db-ai-history pre').allTextContents();
    expect(originals).toHaveLength(2);
    expect(dispatch.map(e=>e.sqlHash)).toEqual(originals.map(hash));
    checks.databaseExecutesOnlyNewCommentedBlocks=true;
    await editor.click();await page.keyboard.press('Control+End');await page.keyboard.insertText('\n\n-- 用户：接续查询\nSELECT 3 AS human_continued;');
    await frame.getByRole('button',{name:'执行 SQL',exact:true}).click();
    await expect(frame.getByRole('columnheader',{name:/human_continued/})).toBeVisible();
    await expect(editor).toContainText('SELECT COUNT(*)');
    checks.userContinuesInSameCompleteDatabaseWorkspace=true;
    const profiles=await page.evaluate(()=>window.go.main.App.DatabaseProfiles());
    expect(profiles.length).toBe(1);expect(profiles[0].passwordSet).toBe(true);
    const raw=await readFile(path.join(appDir,'database-connections.json'),'utf8');expect(raw).not.toContain(endpoint.password);
    if(modelFixture){expect(JSON.stringify(modelFixture.checks.commandResults)).not.toContain(endpoint.password);expect(JSON.stringify(modelFixture.checks.questions)).not.toContain(endpoint.password);}
    checks.databaseCredentialsStayOutsideModelAndProfileFile=true;
    await page.screenshot({path:path.join(runDir,'assistant-database-handoff.png')});
    // AI closes its own session with explicit confirmation, preserving the document/config.
    await send('E2E_DATABASE_CLOSE 关闭你刚创建的数据库运行连接，保留配置和 SQL 原文。');
    const closeSeen=new Set();let final;
    for(const deadline=Date.now()+120000;Date.now()<deadline;){
        final=await state();if(final.turnId===completed.turnId){await new Promise(resolve=>setTimeout(resolve,100));continue;}
        if(['failed','stopped'].includes(final.phase))throw Error(`Database close ${final.phase}`);
        if(final.prompt&&!closeSeen.has(final.prompt.id)){expect(['允许助手使用这个数据库页签？','关闭这个数据库连接？']).toContain(final.prompt.title);await chat.getByRole('button',{name:'允许本次',exact:true}).click();closeSeen.add(final.prompt.id);}
        if(final.phase==='completed')break;await new Promise(resolve=>setTimeout(resolve,100));
    }
    expect(final?.phase).toBe('completed');
    await expect(editor).toContainText('SELECT 3 AS human_continued;');
    expect(await page.evaluate(()=>window.go.main.App.DatabaseProfiles().then(p=>p.length))).toBe(1);
    const closed=await page.evaluate(async id=>{try{await window.go.main.App.DatabaseSchemas(id);return false;}catch{return true;}},physical);expect(closed).toBe(true);
    checks.aiClosesOnlyRuntimeAndPreservesSQLAndSavedProfile=true;
    await page.screenshot({path:path.join(runDir,'assistant-database-closed.png')});
    if(modelFixture)await runDatabaseStabilityTrial({page,profile:profiles[0],endpoint,modelFixture,checks,runDir});
    // Test cleanup is a trusted host action after assertions, not an AI capability.
    await page.evaluate(profile=>window.go.main.App.DatabaseDelete(profile.id,profile.revision),profiles[0]);
}
