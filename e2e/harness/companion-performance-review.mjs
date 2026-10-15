// Paired real-browser measurements. Reads the saved character; changes only this page's local UI.
import {chromium,expect} from '@playwright/test';
import {mkdir,writeFile} from 'node:fs/promises';
import path from 'node:path';
const evidence=path.resolve(process.argv[2]??'e2e/.runtime/companion-performance');
await mkdir(evidence,{recursive:true});
const browser=await chromium.launch({channel:'msedge',headless:true});
const context=await browser.newContext({viewport:{width:1041,height:833}});
const page=await context.newPage(),errors=[];
// Keep the idle/interaction samples in real time; accelerate only subsequent ambient waits.
await page.addInitScript(()=>{const original=window.setTimeout.bind(window);window.accelerateAmbient=false;window.setTimeout=(callback,delay,...args)=>original(callback,window.accelerateAmbient&&delay>=45000&&delay<=75000?3000:delay,...args);});
page.on('pageerror',error=>errors.push(error.message));
const cdp=await context.newCDPSession(page);
await cdp.send('Performance.enable');
let networkBytes=0;cdp.on('Network.loadingFinished',event=>{networkBytes+=event.encodedDataLength;});
await cdp.send('Network.enable');
const metrics=async()=>Object.fromEntries((await cdp.send('Performance.getMetrics')).metrics.map(value=>[value.name,value.value]));
try{
 await page.goto('http://127.0.0.1:34115/');
 const baseline=await page.evaluate(()=>window.go.main.App.CompanionSnapshot());
 await page.getByTestId('nav-icon-companion').click();
 const sprite=page.locator('.oc-assistant-companion-scene img.oc-pixel-sprite');
 await expect(sprite).toHaveAttribute('data-motion','running');
 await page.evaluate(()=>{window.companionPerf={changes:0,longTasks:[],phase:'between'};const observer=new MutationObserver(records=>{window.companionPerf.changes+=records.filter(record=>record.attributeName==='src').length;});observer.observe(document.querySelector('.oc-assistant-companion-scene img'),{attributes:true,attributeFilter:['src']});const longTasks=new PerformanceObserver(list=>window.companionPerf.longTasks.push(...list.getEntries().map(entry=>({duration:entry.duration,startTime:entry.startTime,phase:window.companionPerf.phase}))));longTasks.observe({type:'longtask',buffered:false});});
 const samples=[];
 async function phase(name,operation){
  await page.evaluate(name=>{window.companionPerf.phase=name;},name);
  const start=await metrics(),bytes=networkBytes,changes=await page.evaluate(()=>window.companionPerf.changes),time=performance.now();
  await operation();const end=await metrics();
  samples.push({name,wallMs:performance.now()-time,taskMs:(end.TaskDuration-start.TaskDuration)*1000,scriptMs:(end.ScriptDuration-start.ScriptDuration)*1000,layouts:end.LayoutCount-start.LayoutCount,styleRecalculations:end.RecalcStyleCount-start.RecalcStyleCount,networkBytes:networkBytes-bytes,poseChanges:(await page.evaluate(()=>window.companionPerf.changes))-changes,heapBytes:end.JSHeapUsedSize});
  await page.evaluate(()=>{window.companionPerf.phase='between';});
 }
 await phase('idle',()=>page.waitForTimeout(8500));
 await page.evaluate(()=>{window.accelerateAmbient=true;});
 await phase('interaction',async()=>{for(let i=0;i<4;i++){await page.locator('.oc-assistant-companion-scene .oc-companion-touch').click();await page.waitForTimeout(900);}});
 await expect(sprite).toHaveAttribute('data-action','idle',{timeout:5000});
 await expect(sprite).toHaveAttribute('data-action','walk',{timeout:60000});
 await expect(sprite).toHaveAttribute('data-motion','running');
 await phase('walking',()=>page.waitForTimeout(1000));
 const latencies=[];
 await phase('typing',async()=>{for(let i=0;i<20;i++){const start=performance.now();await page.getByPlaceholder('输入问题…').fill('性能实测输入 '+i);latencies.push(performance.now()-start);}});
 await page.getByPlaceholder('输入问题…').fill('');
 await phase('settings',async()=>{
  await page.getByRole('button',{name:'设置',exact:true}).click();
  await page.getByRole('navigation').getByText('伙伴与奖章',{exact:true}).click();
  await page.getByRole('button',{name:'修改名字',exact:true}).click();
  await expect(page.getByLabel('伙伴名称',{exact:true})).toBeVisible();
  await page.waitForTimeout(300);
 });
 await page.locator('.oc-companion-actions').getByRole('button',{name:'取消',exact:true}).click();
 await page.getByRole('button',{name:'关闭',exact:true}).click();
 await page.getByRole('button',{name:'收起伙伴展示',exact:true}).click();
 await phase('collapsed',()=>page.waitForTimeout(3000));
 latencies.sort((a,b)=>a-b);
 const detail=await page.evaluate(()=>window.companionPerf);
 expect(errors).toEqual([]);
 expect((await page.evaluate(()=>window.go.main.App.CompanionSnapshot())).pet).toEqual(baseline.pet);
 await writeFile(path.join(evidence,'result.json'),JSON.stringify({assetsVersion:'lumen-v8; other species v7',ambientWaitOnlyAcceleratedAfterIdle:true,character:{speciesId:baseline.pet?.speciesId,level:baseline.level,paletteId:baseline.pet?.paletteId},storedPetPreserved:true,samples,inputMedianMs:latencies[10],inputP95Ms:latencies[18],longTasks:detail.longTasks,pageErrors:errors},null,2));
 console.log(JSON.stringify({samples,inputP95Ms:latencies[18],longTasks:detail.longTasks}));
}finally{await context.close();await browser.close();}
