// Validate the production executable from a relocated, isolated directory.
// Release builds deliberately disable the WebView debugging transport, so this
// checks native startup, not UI automation. UI flows have their own E2E harness.
import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { cp, mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../..');
const input=process.argv.find(a=>a.startsWith('--package='))?.slice(10);
const icodeDirectory=process.argv.find(a=>a.startsWith('--icode-directory='))?.slice('--icode-directory='.length);
const expectedVersion=process.argv.find(a=>a.startsWith('--version='))?.slice('--version='.length);
assert.ok(input&&icodeDirectory&&expectedVersion,'Provide --package, --icode-directory and --version');
const runDir=path.join(root,'e2e/.runtime',`packaged-smoke-${Date.now()}`);
const appDir=path.join(runDir,'relocated path','黄区试用');
await mkdir(appDir,{recursive:true});
for(const name of ['opscopilot.exe','jdbc-runtime'])await cp(path.join(input,name),path.join(appDir,name),{recursive:true});
const info=spawnSync(path.join(appDir,'opscopilot.exe'),['--plugin-info'],{cwd:appDir,encoding:'utf8',windowsHide:true,timeout:15000});
assert.equal(info.status,0,info.stderr);const version=JSON.parse(info.stdout).version;
assert.equal(version,expectedVersion);
await mkdir(path.join(appDir,'documents'),{recursive:true});
await writeFile(path.join(appDir,'config.json'),JSON.stringify({experimental:{companion_enabled:false},docs:{dir:path.join(appDir,'documents')},scripts:{dir:path.join(appDir,'scripts')},log:{dir:path.join(appDir,'logs')},appearance:{theme:'dark'},llm:{BaseURL:'http://127.0.0.1:1/v1',APIKey:'',Model:'fixture'},patch_store:{enabled:false},session_share:{enabled:false}}));
await writeFile(path.join(appDir,'service-center.json'),JSON.stringify({baseUrl:'',choice:'disabled'}));
const env={...process.env};for(const key of Object.keys(env))if(/^(devserver|assetdir|OPSCOPILOT_.*)$/i.test(key))delete env[key];
env.PATH=`${icodeDirectory}${path.delimiter}${process.env.PATH||''}`;
const discovered=spawnSync('where.exe',['icode'],{env,encoding:'utf8',windowsHide:true,timeout:10000});
assert.equal(discovered.status,0);const executable=discovered.stdout.trim().split(/\r?\n/)[0];
assert.equal(path.normalize(executable).toLowerCase(),path.join(icodeDirectory,'icode.exe').toLowerCase());
const runtimeVersion=spawnSync(executable,['--version'],{env,encoding:'utf8',windowsHide:true,timeout:15000});
assert.equal(runtimeVersion.status,0);assert.equal(runtimeVersion.stdout.trim(),'0.30.1');
const app=spawn(path.join(appDir,'opscopilot.exe'),[],{cwd:appDir,env,windowsHide:true,stdio:['ignore','pipe','pipe']});
let log='';app.stdout.on('data',d=>{log+=d;});app.stderr.on('data',d=>{log+=d;});
const checks={productionVersion:true,installedICodeOnPATH:true};
try{
  let windowReady=false;
  for(const deadline=Date.now()+30000;Date.now()<deadline;){
    if(app.exitCode!==null)throw Error(`Production process exited: ${app.exitCode}`);
    // Background launch can hide the top-level window from MainWindowHandle.
    // Verify the WebView child belongs to this exact app process instead.
    const window=spawnSync('powershell.exe',['-NoProfile','-Command',`$webview=Get-CimInstance Win32_Process -Filter "ParentProcessId = ${app.pid}" | Where-Object Name -eq 'msedgewebview2.exe';[Console]::Write([string]([bool]$webview))`],{encoding:'utf8',windowsHide:true,timeout:5000});
    if(window.stdout.trim()==='True'&&log.includes('Environment created successfully')){windowReady=true;break;}
    await new Promise(resolve=>setTimeout(resolve,250));
  }
  assert.ok(windowReady,'Native WebView did not initialize');checks.nativeWebViewStartsAfterUnicodeRelocation=true;
  const java=spawn(path.join(appDir,'jdbc-runtime/runtime/bin/java.exe'),['-jar',path.join(appDir,'jdbc-runtime/jdbc-helper.jar')],{cwd:appDir,windowsHide:true,stdio:['pipe','pipe','pipe']});
  try{
    const reply=new Promise((resolve,reject)=>{let data=Buffer.alloc(0);const timer=setTimeout(()=>reject(Error('JDBC runtime timed out')),15000);java.once('error',error=>{clearTimeout(timer);reject(error);});java.stdout.on('data',chunk=>{data=Buffer.concat([data,chunk]);if(data.length>=4&&data.length>=4+data.readInt32BE(0)){clearTimeout(timer);resolve(JSON.parse(data.subarray(4,4+data.readInt32BE(0)).toString('utf8')));}});});
    const payload=Buffer.from(JSON.stringify({id:1,op:'runtime'})),header=Buffer.alloc(4);header.writeInt32BE(payload.length);java.stdin.write(Buffer.concat([header,payload]));
    const answer=await reply;assert.equal(answer.error,undefined);assert.equal(answer.data.legacyJaxb,true);assert.ok(answer.data.javaVersion.startsWith('21.'));checks.privateJavaAndJdbcHelper=true;
  }finally{java.stdin.end();java.kill();}
  await writeFile(path.join(runDir,'result.json'),JSON.stringify({version,checks,runDir},null,2));console.log('PACKAGED_SMOKE_PASSED',JSON.stringify(checks));console.log('EVIDENCE',runDir);
}catch(error){await writeFile(path.join(runDir,'failure.log'),`${error}\n${log}`);throw error;}
finally{if(app.exitCode===null)app.kill();}
