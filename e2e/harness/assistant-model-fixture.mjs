// Deterministic LOCAL model responses exercise real Chrys, MCP and SSH PTY.
// This is a transport/lifecycle fixture, not evidence of model reasoning quality.
import { createServer } from 'node:http';

export async function startAssistantModelFixture() {
  const controls={database:undefined};
  const checks = { requests: 0, commandResults: [], errors: [], toolNames: [], questions: [] };
  const server = createServer(async (request, response) => {
    try {
      let body = '';
      for await (const chunk of request) { body += chunk; if (body.length > 2 ** 20) throw Error('fixture request too large'); }
      const data = JSON.parse(body);
      checks.requests++;
      const messages = data.messages || [];
      const lastUser = messages.findLastIndex(message => message.role === 'user' && !JSON.stringify(message.content).includes('<system-reminder>'));
      const question = JSON.stringify(messages[lastUser]?.content || '');
      if (checks.questions.at(-1) !== question) checks.questions.push(question);
      checks.toolNames = (data.tools || []).map(tool => tool.function?.name);
      const results = messages.slice(lastUser + 1).filter(message => message.role === 'tool');
      const missingTerminal = results.some(result => /ssh_required|ssh_unavailable/.test(String(result.content)));
      checks.resultCount = results.length;
      checks.commandResults.push(...results.map(result => result.content));
      const name = tool => data.tools.find(item => item.function.name === tool)?.function.name || tool;
      const step = (tool, args = {}) => ({ name: name(tool), arguments: JSON.stringify(args) });
      const actionScenario = question.includes('E2E_TERMINAL_') || /当前目录|\bpwd\b/i.test(question);
      const passiveOnly = /终端.*内容|屏幕|能看到|E2E_TERMINAL_SCREEN/.test(question);
      const plan = [];
      if (question.includes('E2E_RECOVERY_APPROVAL')) plan.push(step('write_file', {path:'recovery-never-created.txt',content:'must not be written'}));
      if (question.includes('E2E_RECOVERY_SLOW')) await new Promise(resolve => setTimeout(resolve, 12000));
      const commandScenario=question.includes('E2E_LIBRARY_DATABASE');
      const databaseScenario=commandScenario||/E2E_DATABASE_(CREATE|CLOSE|TRANSACTION|PENDING|LONG)/.test(question);
      const wire=index=>{try{return JSON.parse(results[index]?.content||'{}').result;}catch{return undefined;}};
      if(databaseScenario){
        let offset=0;
        if(commandScenario){plan.push(step('load_skill',{skill_name:'database-discovery'}),step('run_skill_script',{skill_name:'database-discovery',script_name:'scripts/discover.py'}));offset=2;}
        plan.push(step('database_list'));
        if(question.includes('E2E_DATABASE_CREATE')||commandScenario){
          const target=controls.database;
          let discovered;
          if(commandScenario&&results[1]){const text=String(results[1].content);const match=text.match(/DISCOVERED_ENDPOINT=(\{[^}]+\})/);if(match){try{discovered=JSON.parse(match[1].replaceAll('\\"','"'));}catch{}}if(!discovered)throw Error('Native Skill did not return the discovered endpoint');checks.discoveryFromNativeSkill=true;}
          plan.push(step('database_create',{name:'AI 本地查询验证',driver_id:target?.driverId||wire(offset)?.drivers?.[0]?.id,host:discovered?.host||target?.host||'127.0.0.1',port:discovered?.port||target?.port||33306,user:discovered?.user||target?.user||'ops_review',database:discovered?.database||target?.database||'opscopilot_review'}),step('database_open',{profile_id:wire(offset+1)?.id||'missing'}),step('database_take',{workspace_id:wire(offset+2)?.id||'missing'}),step('database_execute',{sql:'SELECT COUNT(*) AS node_count FROM service_nodes;',purpose:'核对业务节点数量'}),step('database_execute',{sql:'SELECT service_name, status FROM service_nodes ORDER BY service_name LIMIT 3;',purpose:'查看节点状态样本'}),step('database_release'));
        }else if(question.includes('E2E_DATABASE_TRANSACTION')||question.includes('E2E_DATABASE_PENDING')){
          const table=controls.database?.table||'missing';
          plan.push(step('database_open',{profile_id:wire(0)?.profiles?.[0]?.id||'missing'}),step('database_take',{workspace_id:wire(1)?.id||'missing'}));
          if(question.includes('E2E_DATABASE_TRANSACTION'))plan.push(step('database_execute',{sql:'START TRANSACTION;',purpose:'验证未提交事务可交还给人工'}),step('database_execute',{sql:`INSERT INTO ${table} (id) VALUES (1);`,purpose:'仅写入隔离测试记录，保持未提交'}),step('database_release'));
          else plan.push(step('database_execute',{sql:`INSERT INTO ${table} (id) VALUES (3);`,purpose:'验证停止后迟到批准不能继续写入'}),step('database_release'));
        }else if(question.includes('E2E_DATABASE_LONG')){
          const id=wire(0)?.workspaces?.find(w=>w.connected)?.id||'missing';
          plan.push(step('database_take',{workspace_id:id}),step('database_execute',{sql:'SELECT SLEEP(20) AS long_query;',purpose:'验证长查询期间手动接管和取消清理'}),step('database_release'));
        }else{
          const id=wire(0)?.workspaces?.find(w=>w.connected)?.id||'missing';
          plan.push(step('database_take',{workspace_id:id}),step('database_close',{workspace_id:id}));
        }
      }
      const businessScenario = /E2E_BUSINESS_(DISCOVER|CORRECT)/.test(question);
      const toolEvidence = results.map(result=>String(result.content)).join('\n');
      const logRoot = toolEvidence.match(/\/tmp\/oc-business-[a-f0-9]+/)?.[0];
      if (businessScenario) {
        plan.push(step('load_skill',{skill_name:'order-detail-logs'}),step('read_skill_resource',{skill_name:'order-detail-logs',resource_name:'references/deployment.md'}),step('terminal_context'),step('terminal_take'),step('terminal_context'));
        if(question.includes('E2E_BUSINESS_DISCOVER')) {
          plan.push(step('terminal_execute',{command:`command ls ${logRoot || '/missing'}`,purpose:'根据已加载的业务部署说明列出订单详情服务日志文件'}),step('terminal_release'),step('ask_user',{questions:[{question:'订单详情页大约在什么时间出错？',header:'故障时间',options:[{label:'今天上午',description:'先看今天上午的日志'},{label:'其他时间',description:'请补充具体时间'}]}]}));
        } else {
          plan.push(step('terminal_execute',{command:`command head -n 3 ${logRoot || '/missing'}/orders-2026-10-21.log`,purpose:'只检查用户指定文件的前三行，接受中断后的范围修正'}),step('terminal_release'));
        }
        checks.skillLoaded ||= toolEvidence.includes('订单详情日志排查');
        checks.skillResourceRead ||= !!logRoot;
        checks.correctedEvidence ||= question.includes('E2E_BUSINESS_CORRECT')&&/ORDER_UPSTREAM_[a-f0-9]+/.test(toolEvidence);
        if(results.length<=5&&/ORDER_UPSTREAM_[a-f0-9]+/.test(toolEvidence)) checks.seedLeakedIntoObservation=true;
      }
      const workspaceScenario = /E2E_WORKSPACE_(OPEN|SELECT|SWITCH)/.test(question);
      const inventory = (() => {try{return JSON.parse(results[0]?.content || '{}').result;}catch{return {};}})();
      if (workspaceScenario) {
        plan.push(step('workspace_list'));
        if (question.includes('E2E_WORKSPACE_OPEN')) {
          plan.push(step('workspace_open',{connection_id:inventory?.connections?.[0]?.id || 'missing'}));
        } else if (question.includes('E2E_WORKSPACE_SELECT')) {
          plan.push(step('workspace_select',{tab_id:inventory?.tabs?.find(tab=>tab.tabId!==inventory?.active?.tabId)?.tabId || 'missing'}));
        }
        plan.push(step('terminal_context'),step('terminal_take'),step('terminal_context'),step('terminal_execute',{command:'command pwd',purpose:'在明确选择的页签验证目录'}));
        if (question.includes('E2E_WORKSPACE_SWITCH')) {
          plan.push(step('workspace_select',{tab_id:inventory?.tabs?.find(tab=>tab.tabId!==inventory?.active?.tabId)?.tabId || 'missing'}),step('terminal_context'),step('terminal_take'),step('terminal_context'),step('terminal_execute',{command:'command pwd',purpose:'切换目标后使用新的授权读取目录'}));
        }
        plan.push(step('terminal_release'));
      }
      if (!databaseScenario && !businessScenario && !workspaceScenario && (actionScenario || passiveOnly)) plan.push(step('terminal_context'));
      if (!databaseScenario && !businessScenario && !workspaceScenario && actionScenario && !passiveOnly) {
        if (question.includes('E2E_TERMINAL_QUESTION')) plan.push(step('ask_user', {questions:[{question:'要检查哪一天的日志？',header:'日志日期',options:[{label:'今天',description:'只看今天'},{label:'昨天',description:'只看昨天'}]}]}));
        plan.push(step('terminal_take'), step('terminal_context'), step('terminal_execute', {command:question.includes('E2E_TERMINAL_APPROVAL') ? "command printf 'ACP_APPROVED_ONCE\\n'" : 'command pwd', purpose:'读取当前目录或打印本地测试标记，用于验证同一段终端操作'}));
        if (question.includes('E2E_TERMINAL_APPROVAL')) plan.push(step('terminal_context'), step('terminal_execute', {command:'command pwd',purpose:'继续检查当前目录，不改变占用状态'}), step('terminal_context'));
        plan.push(step('terminal_release'));
      }
      const next = missingTerminal ? undefined : plan[results.length];
      checks.nextTool = next?.name || 'final';
      checks.scenario = question;
      // Leave observable intervals for the UI to distinguish model inference
      // from a real input lease, and for testing release before the final reply.
      if ((next?.name === 'terminal_context' && results.length > 0) || next?.name==='database_execute' || !next) await new Promise(resolve => setTimeout(resolve, 2000));
      const remembered = JSON.stringify(messages.filter(message => message.role === 'user')).match(/OC_SESSION_[a-f0-9]+/)?.[0];
      const answer = workspaceScenario ? 'WORKSPACE_CHAIN_OK' : question.includes('E2E_LONG_READ') ? Array.from({length:30},(_,index)=>`### 检查项 ${index+1}\n\n这是本地界面回归的长回答，用来验证阅读历史时，新回复不会把用户强制带回底部。`).join('\n\n') : /测试代号/.test(question) ? remembered || '未提供代号' : missingTerminal ? '当前没有可用的 SSH 终端，可以继续聊天；需要操作时请先连接 SSH。' : passiveOnly ? '已读取当前终端屏幕，终端保持可操作。当前预览仍使用模拟模型。' : actionScenario ? '终端助手链路已完成。结果来自原终端。' : /名字|叫什么|who are you/i.test(question)
        ? '我是 OpsCopilot。当前预览使用模拟模型，用于体验终端接手、提问和审批流程。'
        : '当前预览使用模拟模型。可以输入“查看当前目录”体验终端操作流程。';
      const businessAnswer = `orders-reader 在 orders-2026-10-21.log 的 08:41:12 记录 request=req-17、code=UPSTREAM_TIMEOUT、upstream=inventory。实际日志标记：${toolEvidence.match(/ORDER_UPSTREAM_[a-f0-9]+/)?.[0] || '未观察到'}。已观察到上游调用超时，根因仍需更多证据，未修改配置。`;
      const message = next ? { role: 'assistant', content: null, tool_calls: [{ id: `fixture-${results.length}`, type: 'function', function: next }] } : { role: 'assistant', content: databaseScenario?'DATABASE_CHAIN_OK':businessScenario ? businessAnswer : answer };
      response.writeHead(200, { 'content-type': 'application/json' });
      response.end(JSON.stringify({ id: 'local-fixture', object: 'chat.completion', created: 1, model: 'fixture', choices: [{ index: 0, message, finish_reason: next ? 'tool_calls' : 'stop' }], usage: { prompt_tokens: 10, completion_tokens: 10, total_tokens: 20 } }));
    } catch (error) { checks.errors.push(String(error)); response.writeHead(500); response.end(JSON.stringify({ error: String(error) })); }
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  return { baseURL: `http://127.0.0.1:${server.address().port}/v1`, checks, get database(){return controls.database;},set database(value){controls.database=value;},close: () => server.close() };
}
