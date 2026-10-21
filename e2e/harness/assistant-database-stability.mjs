import { expect } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import path from 'node:path';

// Local model decisions are deterministic; ACP, host approvals, UI ownership,
// imported JDBC driver, physical sessions and MySQL transactions are real.
export async function runDatabaseStabilityTrial({page, profile, endpoint, modelFixture, checks, runDir}) {
    const table=`ai_handoff_${randomUUID().replaceAll('-','')}`;
    modelFixture.database={...modelFixture.database,table};
    const chat=page.locator('.ai-chat-panel');
    const state=()=>page.evaluate(()=>window.go.main.App.AssistantControlState());
    const visible=()=>page.locator('.oc-ai-db:visible');
    const observer=await page.evaluate(id=>window.go.main.App.DatabaseConnect(id),profile.id);
    const physicals=new Set();
    async function query(id,sql){
        const result=await page.evaluate(({id,sql,database})=>window.go.main.App.DatabaseQuery({sessionId:id,requestId:crypto.randomUUID(),sql,database,mode:'selection',limit:100,timeoutSeconds:30}),{id,sql,database:endpoint.database});
        expect(result.error).toBe('');return result;
    }
    async function rows(){return Number((await query(observer.id,`SELECT COUNT(*) AS n FROM ${table};`)).sets[0].rows[0][0]);}
    async function send(text){
        const before=(await state()).turnId;
        await chat.getByPlaceholder('输入问题…',{exact:true}).fill(text);
        await chat.getByTitle('发送',{exact:true}).click();
        await expect.poll(async()=>(await state()).turnId,{timeout:15000}).not.toBe(before);
    }
    async function drive({stopAtSQL=false,sqls=[]}={}){
        const seen=new Set();let latest;
        for(const end=Date.now()+90000;Date.now()<end;){
            latest=await state();
            if(latest.target?.connectionId)physicals.add(latest.target.connectionId);
            if(latest.prompt&&!seen.has(latest.prompt.id)){
                const p=latest.prompt;
                expect(['打开独立数据库连接？','允许助手使用这个数据库页签？','确认这一次数据库操作','关闭这个数据库连接？']).toContain(p.title);
                if(p.title==='确认这一次数据库操作'){
                    if(stopAtSQL)return latest;
                    expect(sqls).toContain(p.command);
                }
                await chat.getByRole('button',{name:'允许本次',exact:true}).click();seen.add(p.id);
            }
            if(['completed','failed','stopped'].includes(latest.phase))break;
            await new Promise(resolve=>setTimeout(resolve,80));
        }
        expect(latest.phase).toBe('completed');return latest;
    }
    async function appendAndRun(sql){
        const frame=visible();const editor=frame.locator('.cm-content');
        await editor.click();await page.keyboard.press('Control+End');
        await page.keyboard.insertText(`\n\n-- 用户：事务交接验证\n${sql}`);
        await frame.getByRole('button',{name:'执行 SQL',exact:true}).click();
        await expect(frame.getByRole('button',{name:'停止',exact:true})).toBeDisabled();
    }
    try{
        await query(observer.id,`CREATE TABLE ${table} (id INT PRIMARY KEY) ENGINE=InnoDB;`);
        await send('E2E_DATABASE_TRANSACTION 打开独立测试页签，开始事务，插入测试记录，保持未提交并交还。');
        const transaction=await drive({sqls:['START TRANSACTION;',`INSERT INTO ${table} (id) VALUES (1);`]});
        const physical=transaction.target.connectionId;
        expect(physical).not.toBe(observer.id);
        expect(await rows()).toBe(0);
        expect((await query(physical,`SELECT COUNT(*) AS n FROM ${table};`)).sets[0].rows).toEqual([['1']]);
        // Duplicating the inner SQL tab copies text, not execution or a session.
        const frame=visible();
        await frame.getByRole('navigation',{name:'数据库查询标签'}).getByRole('button',{name:/^AI 查询记录 ·/}).click({button:'right'});
        await page.getByRole('menuitem',{name:'复制查询',exact:true}).click();
        expect(await rows()).toBe(0);
        await expect(frame.locator('.cm-content')).toContainText(`INSERT INTO ${table}`);
        await appendAndRun('ROLLBACK;');
        expect((await query(physical,`SELECT COUNT(*) AS n FROM ${table};`)).sets[0].rows).toEqual([['0']]);
        checks.uncommittedAITransactionSurvivesReleaseAndUserRollsBackFromCopiedQuery=true;
        await appendAndRun('START TRANSACTION;');
        await appendAndRun(`INSERT INTO ${table} (id) VALUES (2);`);
        expect(await rows()).toBe(0);
        await send('E2E_DATABASE_CLOSE 关闭当前独立连接，不提交事务，保留原文。');
        await drive();
        await expect.poll(rows).toBe(0);
        checks.confirmedCloseNeverCommitsPendingTransaction=true;
        await send('E2E_DATABASE_PENDING 在独立页签写入隔离测试记录。');
        const held=await drive({stopAtSQL:true});
        expect(held.prompt.command).toBe(`INSERT INTO ${table} (id) VALUES (3);`);
        await chat.getByRole('button',{name:'停止本轮',exact:true}).click();
        await expect.poll(async()=>(await state()).phase).toBe('stopped');
        const late=await page.evaluate(async old=>{try{await window.go.main.App.RespondAssistantInteraction(old.turnId,old.prompt.id,{text:'',selected:['allow']});return false;}catch{return true;}},held);
        expect(late).toBe(true);expect(await rows()).toBe(0);
        await expect(visible().locator('.oc-handoff-shield')).toHaveCount(0);
        checks.stopRevokesPendingDatabaseApprovalAndPreservesOriginalSQL=true;
        await send('E2E_DATABASE_LONG 执行隔离长查询，等待人工接管。');
        await drive({stopAtSQL:true});
        await chat.getByRole('button',{name:'允许本次',exact:true}).click();
        await expect.poll(async()=>Number((await query(observer.id,"SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE ID <> CONNECTION_ID() AND INFO LIKE '%SLEEP(20)%';")).sets[0].rows[0][0]),{timeout:10000}).toBe(1);
        const running=await state();const longPhysical=running.target.connectionId;
        await visible().getByRole('button',{name:'手动接管',exact:true}).click();
        await expect.poll(async()=>(await state()).phase).toBe('stopped');
        await expect.poll(()=>page.evaluate(async id=>{try{await window.go.main.App.DatabaseSchemas(id);return false;}catch(error){return !String(error).includes('正在执行')&&!String(error).includes('请先手动接管');}},longPhysical),{timeout:12000}).toBe(true);
        await expect(visible().locator('.cm-content')).toContainText('SELECT SLEEP(20) AS long_query;');
        await expect(visible().locator('.oc-handoff-shield')).toHaveCount(0);
        await expect.poll(async()=>Number((await query(observer.id,"SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE ID <> CONNECTION_ID() AND INFO LIKE '%SLEEP(20)%';")).sets[0].rows[0][0])).toBe(0);
        expect(await rows()).toBe(0);
        checks.manualTakeoverCancelsRealJDBCQueryAndCleansOnlyItsSession=true;
        await visible().getByRole('button',{name:'连接数据库',exact:true}).click();
        await expect(visible().getByRole('button',{name:'断开数据库连接',exact:true})).toBeEnabled();
        await appendAndRun('SELECT 9 AS user_reconnected;');
        await expect(visible().getByRole('columnheader',{name:/user_reconnected/})).toBeVisible();
        checks.userReconnectsAfterCancellationWithoutReusingDeadSession=true;
        await page.screenshot({path:path.join(runDir,'assistant-database-stop.png')});
    }finally{
        const latest=await state();
        if(['running','waiting'].includes(latest.phase))await page.evaluate(id=>window.go.main.App.StopAssistantTurn(id),latest.turnId);
        for(const id of physicals){
            for(let attempt=0;attempt<20;attempt++){
                const closed=await page.evaluate(async id=>{try{await window.go.main.App.DatabaseDisconnect(id);return true;}catch{return false;}},id);
                if(closed)break;
                await new Promise(resolve=>setTimeout(resolve,100));
            }
        }
        try{await query(observer.id,`DROP TABLE IF EXISTS ${table};`);}finally{await page.evaluate(id=>window.go.main.App.DatabaseDisconnect(id),observer.id);}
    }
}
